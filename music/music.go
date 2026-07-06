// Package music generates short, perfectly-looping chiptune tracks from a
// small deterministic parameter set, rendered entirely through the gion
// synthesis core: every note is a gion.Params scheduled on a step grid and
// mixed into one buffer. The same Params always produce the same samples, so
// a game can ship a seed instead of an audio file.
//
// The design follows the classic NES conventions: a square lead built from a
// short motif arranged in an AABA form over a chord progression, a triangle
// bass on chord roots, and noise percussion on euclidean patterns. Mood picks
// scale, progression, tempo and timbre together.
package music

import (
	"math"
	"math/rand/v2"

	"github.com/crgimenes/gion"
)

// Mood selects the musical character: scale, chord progression, default
// tempo, lead timbre and swing.
type Mood int

const (
	Upbeat Mood = iota
	Heroic
	Dark
	Chill
	Battle
	Boss
)

// Moods maps the mood names to their values, for CLIs and UIs.
var Moods = map[string]Mood{
	"upbeat": Upbeat,
	"heroic": Heroic,
	"dark":   Dark,
	"chill":  Chill,
	"battle": Battle,
	"boss":   Boss,
}

// moodDef is one row of the style table: everything a Mood decides. The
// drive flag splits the table into two families: melodic moods (walking
// motif, sparse kick, breathing cadences — menus, towns, cutscenes) and
// drive moods (dense riff, galloping eighth-note bass, four-on-the-floor
// kick, 16th hats — action, shmups, bosses).
type moodDef struct {
	scale            []int     // semitone offsets from the root, one octave
	progs            [][4]int  // chord progressions (roots as scale degrees); the seed picks one
	tempoLo, tempoHi float64   // the seed lands anywhere in the range; Params.Tempo overrides exactly
	duties           []float64 // lead duty bank; the seed picks the timbre
	duty             float64   // resolved per track in Render, not set in the table
	swing            float64   // fraction of a step added to off-beat onsets
	drive            bool      // action style instead of melodic style
}

// moods is the style table. A mood is a territory, not a track: the seed
// picks the progression, the tempo inside a wide range, the lead timbre from
// the duty bank, the form and the grooves — that spread is what keeps two
// rolls from resembling each other. Battle rides the phrygian scale and Boss
// the harmonic minor, the two classic tension colors of action chiptune.
var moods = map[Mood]moodDef{
	Upbeat: {scale: []int{0, 2, 4, 5, 7, 9, 11}, tempoLo: 128, tempoHi: 168, duties: []float64{0.5, 0.25},
		progs: [][4]int{{0, 4, 5, 3}, {0, 3, 4, 3}, {0, 5, 3, 4}}},
	Heroic: {scale: []int{0, 2, 4, 5, 7, 9, 10}, tempoLo: 120, tempoHi: 155, duties: []float64{0.5, 0.25},
		progs: [][4]int{{0, 6, 3, 4}, {0, 3, 6, 4}, {0, 4, 6, 3}}},
	Dark: {scale: []int{0, 2, 3, 5, 7, 8, 10}, tempoLo: 95, tempoHi: 135, duties: []float64{0.25, 0.125},
		progs: [][4]int{{0, 5, 2, 6}, {0, 3, 4, 3}, {0, 2, 3, 4}}},
	Chill: {scale: []int{0, 2, 4, 7, 9}, tempoLo: 80, tempoHi: 104, duties: []float64{0.25, 0.5}, swing: 0.33,
		progs: [][4]int{{0, 3, 1, 4}, {0, 2, 3, 1}, {0, 4, 3, 1}}},
	Battle: {scale: []int{0, 1, 3, 5, 7, 8, 10}, tempoLo: 155, tempoHi: 200, duties: []float64{0.25, 0.125, 0.5}, drive: true,
		progs: [][4]int{{0, 1, 5, 6}, {0, 1, 0, 6}, {0, 2, 1, 6}}},
	Boss: {scale: []int{0, 2, 3, 5, 7, 8, 11}, tempoLo: 140, tempoHi: 180, duties: []float64{0.5, 0.25}, drive: true,
		progs: [][4]int{{0, 3, 5, 4}, {0, 5, 3, 4}, {0, 3, 0, 4}}},
}

// Mute bits: set a bit in Params.Mute to silence that instrument — for
// isolating a voice in the workbench, or shipping a reduced arrangement. The
// bit order matches the internal voice order.
const (
	MuteLead = 1 << iota
	MuteEcho
	MuteBass
	MuteKick
	MuteSnare
	MuteHat
)

// Params describes one track. Zero fields fall back to the mood's defaults
// (or the package defaults for Bars, Root and Gain); use New for a ready set.
type Params struct {
	Seed  int64
	Mood  Mood
	Tempo float64 // BPM; 0 = the mood's tempo
	Bars  int     // loop length; 0 = 16 (an AABA of 4-bar phrases)
	Root  float64 // tonic frequency in Hz; 0 = 220 (A3)
	Gain  float64 // output level (0..1); 0 = 0.6
	Mute  int     // bitmask of silenced instruments (MuteLead, MuteBass, ...)

	// Mixer levels per instrument group, multiplying the built-in balance.
	// 0 means 1.0 (unchanged); the workbench exposes them as sliders so the
	// mix can be set by ear and saved with the track.
	LeadVol float64
	BassVol float64
	DrumVol float64
}

// New returns the default track parameters for a mood and seed.
func New(mood Mood, seed int64) Params {
	return Params{Seed: seed, Mood: mood, Gain: 0.6}
}

// stepsPerBar is the tracker grid: 16th notes in 4/4.
const stepsPerBar = 16

// musicStream fixes the second PCG word ("musi" in ASCII), so a Seed alone
// identifies the composition stream.
const musicStream = 0x6d757369

// scramble spreads the seed across the state space with a splitmix64
// finalizer. PCG's first draws correlate badly between small sequential
// seeds — rolls 1, 2, 3 were landing on the same tempo, timbre and key.
func scramble(seed int64) uint64 {
	z := uint64(seed) + 0x9E3779B97F4A7C15 // #nosec G115 -- bit reinterpretation is the intent
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// Render composes and synthesizes the track as mono 16-bit samples at the
// given rate (gion.DefaultRate when rate <= 0). The sample count is derived
// from tempo and bars alone, and note tails wrap around to the start of the
// buffer, so the result loops seamlessly by construction.
func (p Params) Render(rate int) []int16 {
	if rate <= 0 {
		rate = gion.DefaultRate
	}
	md, ok := moods[p.Mood]
	if !ok {
		md = moods[Upbeat]
	}
	bars := p.Bars
	if bars <= 0 {
		bars = 16
	}
	root := p.Root
	if root <= 0 {
		root = 220
	}
	gain := p.Gain
	if gain <= 0 {
		gain = 0.6
	}

	// #nosec G404 -- the deterministic generator is the feature, not a
	// security boundary.
	rng := rand.New(rand.NewPCG(scramble(p.Seed), musicStream))

	// Per-seed identity, drawn in a fixed order: the lead timbre from the
	// duty bank, the tempo anywhere in the mood's range, and the key up to a
	// tritone away. An explicit Params.Tempo is honored exactly.
	md.duty = md.duties[rng.IntN(len(md.duties))]
	tempoDraw := rng.Float64()
	tempo := p.Tempo
	if tempo <= 0 {
		tempo = md.tempoLo + (md.tempoHi-md.tempoLo)*tempoDraw
	}
	root *= math.Pow(2, float64(rng.IntN(13)-6)/12)

	stepDur := 60 / tempo / 4 // seconds per 16th step
	total := int(math.Round(stepDur * float64(stepsPerBar*bars) * float64(rate)))
	if total <= 0 {
		return nil
	}

	score := compose(rng, md, bars)
	lv := mixLevels{lead: vol(p.LeadVol), bass: vol(p.BassVol), drum: vol(p.DrumVol)}

	mix := make([]int32, total)
	for _, n := range score {
		if p.Mute&(1<<int(n.voice)) != 0 {
			continue // this instrument is muted; composition stays identical
		}
		start := float64(n.step)
		if md.swing > 0 && n.step%2 == 1 {
			start += md.swing
		}
		off := int(math.Round(start * stepDur * float64(rate)))
		voice := n.voiceParams(md, root, stepDur, gain, lv)
		addWrap(mix, voice.Render(rate), off)
	}

	out := make([]int16, total)
	for i, v := range mix {
		if v > math.MaxInt16 {
			v = math.MaxInt16
		}
		if v < math.MinInt16 {
			v = math.MinInt16
		}
		out[i] = int16(v)
	}
	return out
}

// addWrap mixes a note into the master buffer at the given offset. A tail
// running past the end wraps to the start, which is what keeps the loop
// boundary click-free.
func addWrap(mix []int32, s []int16, off int) {
	n := len(mix)
	for i, v := range s {
		mix[(off+i)%n] += int32(v)
	}
}

// voice identifies which instrument plays a note.
type voice int

const (
	leadVoice voice = iota
	echoVoice       // the lead's ghost repeat, quieter
	bassVoice
	kickVoice
	snareVoice
	hatVoice
)

// note is one scheduled event: a step on the grid, a duration in steps, and
// either a scale degree (pitched voices) or a percussion instrument.
type note struct {
	voice  voice
	step   int
	dur    int
	degree int
	seed   int64 // noise stream for percussion, drawn during composition
}

// mixLevels are the resolved per-group volume multipliers.
type mixLevels struct {
	lead, bass, drum float64
}

// vol resolves a mixer field: zero means unchanged.
func vol(v float64) float64 {
	if v <= 0 {
		return 1
	}
	return v
}

// voiceParams builds the gion.Params that synthesize this note. Percussion
// values follow the FamiTracker conventions: noise period low for the kick,
// mid for the snare, high for the hat, all with fast-dying envelopes.
func (n note) voiceParams(md moodDef, root, stepDur, gain float64, lv mixLevels) gion.Params {
	dur := float64(n.dur) * stepDur
	switch n.voice {
	case leadVoice, echoVoice:
		// Mix rule, both edges: the lead leads (above the bass) but does not
		// drown — its RMS stays under twice the bass's. "Lead on top" overdone
		// was crg's "the lead muffles everything" complaint.
		g := 0.22 * gain * lv.lead
		if n.voice == echoVoice {
			g *= 0.4
		}
		// The motif already sits an octave up in scale degrees, so the base
		// stays at the root: the melody lands in the 300-900Hz range where a
		// square sings instead of whistling. The gentle low-pass shaves the
		// upper harmonics that pierce far above their RMS ("apitos").
		p := gion.Params{
			Wave:    gion.Square,
			Freq:    degreeFreq(root, md.scale, n.degree),
			Sustain: dur * 0.55,
			Punch:   0.15,
			Decay:   dur * 0.4,
			Duty:    md.duty,
			LowPass: 3200,
			Gain:    g,
		}
		if n.dur >= 4 {
			// Vibrato only on held notes, entering as the note sustains.
			p.Vibrato = p.Freq * 0.01
			p.VibratoHz = 6
		}
		return p
	case bassVoice:
		return gion.Params{
			Wave:    gion.Triangle,
			Freq:    degreeFreq(root/2, md.scale, n.degree),
			Sustain: dur * 0.7,
			Decay:   0.05,
			Gain:    0.24 * gain * lv.bass,
		}
	case kickVoice:
		return gion.Params{
			Wave: gion.Noise, Freq: 220, FreqSlide: -600,
			Punch: 0.6, Decay: 0.12, Gain: 0.3 * gain * lv.drum, Seed: n.seed,
		}
	case snareVoice:
		return gion.Params{
			Wave: gion.Noise, Freq: 1400, FreqSlide: -900,
			Punch: 0.4, Decay: 0.09, Gain: 0.25 * gain * lv.drum, Seed: n.seed,
		}
	case hatVoice:
		// High noise cuts through far above its RMS; keep it low.
		return gion.Params{
			Wave: gion.Noise, Freq: 7000,
			Decay: 0.03, Gain: 0.14 * gain * lv.drum, Seed: n.seed,
		}
	}
	return gion.Params{}
}

// degreeFreq maps a scale degree (any range; negatives allowed) to a
// frequency, walking whole octaves outside the base one.
func degreeFreq(root float64, scale []int, degree int) float64 {
	n := len(scale)
	oct := degree / n
	idx := degree % n
	if idx < 0 {
		idx += n
		oct--
	}
	semi := float64(scale[idx] + 12*oct)
	return root * math.Pow(2, semi/12)
}
