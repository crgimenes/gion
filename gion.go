// Package gion synthesizes 8-bit style game sound effects from a small,
// serializable set of parameters. Rendering is deterministic: the same Params
// always produce the same samples, so a game can ship tiny presets and
// generate the audio at runtime instead of shipping files. Inspired by
// sfxr/bfxr, not a port.
package gion

import (
	"math"
	"math/rand/v2"
)

// Wave selects the oscillator shape.
type Wave int

const (
	Square Wave = iota
	Saw
	Triangle
	Sine
	Noise
)

// DefaultRate is the sample rate used when a caller passes a rate <= 0.
const DefaultRate = 44100

// Rendering guards: sliders and hand-edited presets can ask for absurd values,
// so durations are capped and the frequency is kept under a safe ceiling.
const (
	maxDuration = 10.0    // seconds for the whole sound
	maxFreq     = 20000.0 // Hz
)

// pcgStream fixes the second PCG word so a Seed alone identifies the noise
// stream ("gion" in ASCII).
const pcgStream = 0x67696f6e

// Params describes one sound effect. The zero value renders silence; start
// from a preset and tweak. All fields are plain data, so a Params can be
// serialized (e.g. JSON) and shipped with a game.
type Params struct {
	Wave Wave

	Freq      float64 // starting frequency in Hz
	FreqSlide float64 // frequency change in Hz per second
	FreqLimit float64 // stop the sound when a downward slide crosses this; 0 = never

	Attack  float64 // seconds fading in
	Sustain float64 // seconds at full level
	Punch   float64 // extra level at the start of the sustain, fading across it (0..1)
	Decay   float64 // seconds fading out

	Duty      float64 // square duty cycle (0..1]; 0 means 0.5
	Vibrato   float64 // vibrato depth in Hz
	VibratoHz float64 // vibrato speed in cycles per second
	ArpMult   float64 // multiply the frequency by this once, after ArpDelay; 0 = off
	ArpDelay  float64 // seconds before the arpeggio jump
	LowPass   float64 // one-pole low-pass cutoff in Hz; 0 = off
	Bits      int     // quantize the output to this many bits (1..15); 0 = off

	Gain float64 // output level (0..1)
	Seed int64   // noise stream seed
}

// Render synthesizes the sound as mono 16-bit samples at the given rate
// (DefaultRate when rate <= 0). The output depends only on Params, never on
// the clock or global randomness.
func (p Params) Render(rate int) []int16 {
	if rate <= 0 {
		rate = DefaultRate
	}
	attack := clampDur(p.Attack)
	sustain := clampDur(p.Sustain)
	decay := clampDur(p.Decay)
	total := attack + sustain + decay
	if total > maxDuration {
		k := maxDuration / total
		attack *= k
		sustain *= k
		decay *= k
		total = maxDuration
	}
	n := int(total * float64(rate))
	out := make([]int16, 0, n)

	// #nosec G115 G404 -- reinterpreting the seed's bits is the intent, and the
	// deterministic generator is the feature (same Params, same sound), not a
	// security boundary.
	rng := rand.New(rand.NewPCG(uint64(p.Seed), pcgStream))
	dt := 1 / float64(rate)
	duty := p.Duty
	if duty <= 0 || duty > 1 {
		duty = 0.5
	}
	freq := p.Freq
	phase := 0.0
	noiseVal := rng.Float64()*2 - 1
	lp := 0.0
	arpDone := false

	for i := range n {
		t := float64(i) * dt

		freq += p.FreqSlide * dt
		if p.FreqLimit > 0 && freq < p.FreqLimit {
			break
		}
		if !arpDone && p.ArpMult != 0 && t >= p.ArpDelay {
			freq *= p.ArpMult
			arpDone = true
		}
		f := freq + p.Vibrato*math.Sin(2*math.Pi*p.VibratoHz*t)
		if f < 0 {
			f = 0
		}
		if f > maxFreq {
			f = maxFreq
		}
		phase += f * dt
		if phase >= 1 {
			phase -= math.Floor(phase)
			// 8-bit style noise: hold one random level per oscillator period.
			noiseVal = rng.Float64()*2 - 1
		}

		v := oscSample(p.Wave, phase, duty, noiseVal)
		v *= envelope(t, attack, sustain, decay, p.Punch)
		if p.LowPass > 0 {
			alpha := dt / (1/(2*math.Pi*p.LowPass) + dt)
			lp += alpha * (v - lp)
			v = lp
		}
		if p.Bits > 0 && p.Bits < 16 {
			levels := float64(uint(1) << p.Bits)
			v = math.Round(v*levels) / levels
		}
		v *= p.Gain
		if v > 1 {
			v = 1
		}
		if v < -1 {
			v = -1
		}
		out = append(out, int16(v*math.MaxInt16))
	}
	return out
}

// oscSample evaluates one oscillator shape at the given phase in [0, 1).
func oscSample(w Wave, phase, duty, noiseVal float64) float64 {
	switch w {
	case Square:
		if phase < duty {
			return 1
		}
		return -1
	case Saw:
		return 2*phase - 1
	case Triangle:
		return 1 - 4*math.Abs(phase-0.5)
	case Sine:
		return math.Sin(2 * math.Pi * phase)
	case Noise:
		return noiseVal
	}
	return 0
}

// envelope shapes the amplitude at time t: a linear attack, a sustain that
// starts punched above full level and settles to it, and a linear decay.
func envelope(t, attack, sustain, decay, punch float64) float64 {
	switch {
	case t < attack:
		return t / attack
	case t < attack+sustain:
		k := (t - attack) / sustain
		return 1 + punch*(1-k)
	default:
		if decay <= 0 {
			return 0
		}
		k := (t - attack - sustain) / decay
		if k > 1 {
			return 0
		}
		return 1 - k
	}
}

// clampDur keeps a duration non-negative, so a bad preset degrades to a
// shorter sound instead of a panic or a huge allocation.
func clampDur(d float64) float64 {
	if d < 0 || math.IsNaN(d) {
		return 0
	}
	return d
}
