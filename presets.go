package gion

import "math/rand/v2"

// Presets maps each preset name to its generator, so CLIs and UIs can offer
// the catalog without hardcoding it.
var Presets = map[string]func(seed int64) Params{
	"blip":      Blip,
	"explosion": Explosion,
	"hit":       Hit,
	"jump":      Jump,
	"laser":     Laser,
	"pickup":    Pickup,
	"powerup":   Powerup,
}

// Each preset derives a deterministic variation from seed: the same seed is
// the same sound forever, and a new seed is a sibling of the same family.

// Pickup is the classic coin: a short square blip that jumps up a musical
// interval after a moment.
func Pickup(seed int64) Params {
	rng := presetRNG(seed)
	return Params{
		Wave:     Square,
		Freq:     span(rng, 900, 1400),
		Sustain:  span(rng, 0.05, 0.09),
		Punch:    span(rng, 0.3, 0.6),
		Decay:    span(rng, 0.15, 0.35),
		Duty:     0.5,
		ArpMult:  span(rng, 1.3, 1.6),
		ArpDelay: span(rng, 0.05, 0.09),
		Gain:     0.5,
		Seed:     seed,
	}
}

// Laser is a fast downward sweep on a thin pulse.
func Laser(seed int64) Params {
	rng := presetRNG(seed)
	freq := span(rng, 1200, 2400)
	return Params{
		Wave:      Square,
		Freq:      freq,
		FreqSlide: -span(rng, 4000, 9000),
		FreqLimit: span(rng, 180, 320),
		Sustain:   span(rng, 0.08, 0.16),
		Punch:     span(rng, 0.1, 0.3),
		Decay:     span(rng, 0.05, 0.15),
		Duty:      span(rng, 0.15, 0.35),
		Gain:      0.5,
		Seed:      seed,
	}
}

// Explosion is low-passed noise sliding down with a long tail.
func Explosion(seed int64) Params {
	rng := presetRNG(seed)
	return Params{
		Wave:      Noise,
		Freq:      span(rng, 700, 1400),
		FreqSlide: -span(rng, 400, 900),
		Sustain:   span(rng, 0.1, 0.2),
		Punch:     span(rng, 0.4, 0.7),
		Decay:     span(rng, 0.5, 1.1),
		LowPass:   span(rng, 900, 2200),
		Gain:      0.6,
		Seed:      seed,
	}
}

// Powerup rises: a square sweeping up with a light vibrato.
func Powerup(seed int64) Params {
	rng := presetRNG(seed)
	return Params{
		Wave:      Square,
		Freq:      span(rng, 500, 900),
		FreqSlide: span(rng, 1500, 3500),
		Sustain:   span(rng, 0.25, 0.45),
		Decay:     span(rng, 0.15, 0.35),
		Duty:      0.5,
		Vibrato:   span(rng, 20, 60),
		VibratoHz: span(rng, 8, 14),
		Gain:      0.5,
		Seed:      seed,
	}
}

// Hit is a short burst of falling noise for impacts and damage.
func Hit(seed int64) Params {
	rng := presetRNG(seed)
	return Params{
		Wave:      Noise,
		Freq:      span(rng, 1500, 3000),
		FreqSlide: -span(rng, 6000, 12000),
		Sustain:   span(rng, 0.02, 0.05),
		Punch:     span(rng, 0.3, 0.6),
		Decay:     span(rng, 0.08, 0.18),
		Gain:      0.5,
		Seed:      seed,
	}
}

// Jump is a soft square sweeping up, longer than a blip and without punch.
func Jump(seed int64) Params {
	rng := presetRNG(seed)
	return Params{
		Wave:      Square,
		Freq:      span(rng, 300, 600),
		FreqSlide: span(rng, 900, 2200),
		Sustain:   span(rng, 0.12, 0.22),
		Decay:     span(rng, 0.1, 0.2),
		Duty:      span(rng, 0.3, 0.5),
		Gain:      0.5,
		Seed:      seed,
	}
}

// Blip is the minimal UI tick: a very short steady tone.
func Blip(seed int64) Params {
	rng := presetRNG(seed)
	wave := Square
	if rng.Float64() < 0.5 {
		wave = Sine
	}
	return Params{
		Wave:    wave,
		Freq:    span(rng, 700, 1600),
		Sustain: span(rng, 0.02, 0.04),
		Decay:   span(rng, 0.03, 0.07),
		Duty:    0.5,
		Gain:    0.5,
		Seed:    seed,
	}
}

// Mutate returns a deterministic sibling of p: each non-zero numeric field
// has a coin-flip chance of drifting up to ±10%, relative to its value, so
// the sound keeps its character — a mutated laser is still that laser. Fields
// that are off (zero) stay off, and the wave, bit depth, gain and noise seed
// are left alone. The same p and seed always produce the same sibling.
//
// Besides sculpting sounds in the editor, this is useful at runtime: mutate a
// base effect with a varying seed and repeated explosions, hits or footsteps
// stop sounding identical, at zero asset cost.
func Mutate(p Params, seed int64) Params {
	rng := presetRNG(seed)
	drift := func(v *float64) {
		if *v == 0 {
			return
		}
		if rng.Float64() < 0.5 {
			return
		}
		*v *= 1 + (rng.Float64()*2-1)*0.1
	}
	drift(&p.Freq)
	drift(&p.FreqSlide)
	drift(&p.FreqLimit)
	drift(&p.Attack)
	drift(&p.Sustain)
	drift(&p.Punch)
	drift(&p.Decay)
	drift(&p.Duty)
	if p.Duty > 0.95 {
		p.Duty = 0.95 // keep the pulse audible; Render treats >1 as the default
	}
	drift(&p.Vibrato)
	drift(&p.VibratoHz)
	drift(&p.ArpMult)
	drift(&p.ArpDelay)
	drift(&p.LowPass)
	return p
}

// presetRNG is the deterministic source behind a preset variation.
func presetRNG(seed int64) *rand.Rand {
	// #nosec G404 -- the deterministic generator is the feature (same seed,
	// same variation), not a security boundary.
	return rand.New(rand.NewPCG(scramble(seed), pcgStream))
}

// span picks a value in [lo, hi) from the preset's random stream.
func span(rng *rand.Rand, lo, hi float64) float64 {
	return lo + rng.Float64()*(hi-lo)
}
