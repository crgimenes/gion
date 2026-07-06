package music

import "math/rand/v2"

// This file is the composer: it turns one deterministic random stream into a
// scheduled score, applying the small set of rules that keep generated music
// from sounding random — everything on the scale, chord tones on strong
// beats, one motif reused with variations, an AABA form, stable percussion
// with a fill every fourth bar.

// motif is one bar of melody material: onset steps and a scale degree per
// onset, before any per-bar transposition.
type motif struct {
	onsets  []int
	degrees []int
}

// driveKicks is the kick-pattern bank for drive moods; the seed picks the
// groove: straight four, pushed, or broken.
var driveKicks = [][]int{
	{0, 4, 8, 12},
	{0, 4, 8, 10, 12},
	{0, 6, 8, 14},
}

// driveSnares is the snare-pattern bank for drive moods.
var driveSnares = [][]int{
	{4, 12},
	{4, 10, 12},
	{4, 12, 15},
}

// melodicKicks is the kick bank for melodic moods.
var melodicKicks = [][]int{
	{0, 8},
	{0, 8, 10},
	{0, 6, 8},
}

// forms is the bank of song forms: one motif index per 4-bar phrase. Three
// motifs exist per track — A, a mutation of A, and an independent C — so the
// form choice reshapes the whole arc of the piece.
var forms = [][4]int{
	{0, 0, 1, 0}, // AABA
	{0, 1, 0, 1}, // ABAB
	{0, 0, 1, 2}, // AABC
	{0, 1, 2, 0}, // ABCA
}

// compose builds the whole score for the given number of bars.
func compose(rng *rand.Rand, md moodDef, bars int) []note {
	scaleLen := len(md.scale)

	// The seed picks this track's harmonic identity from the mood's bank.
	prog := md.progs[rng.IntN(len(md.progs))]

	// Three motifs and a form. A is the anchor, B mutates it, C is drawn
	// fresh for real contrast; the form decides how they alternate. Drive
	// moods use dense riffs instead of walking motifs.
	make1 := makeMotif
	if md.drive {
		make1 = makeRiff
	}
	motifA := make1(rng, scaleLen)
	motifs := [3]motif{motifA, mutateMotif(rng, motifA), make1(rng, scaleLen)}
	form := forms[rng.IntN(len(forms))]

	// The one-channel echo is itself a per-seed trait of melodic tracks.
	echo := !md.drive && rng.Float64() < 0.6

	// Bass rhythm: drive moods gallop on constant eighths, with the figure
	// drawn from a small bank; melodic moods ride a euclidean pattern
	// anchored on the downbeat. The color is drawn once, reused every bar.
	var bassOnsets []int
	if md.drive {
		bassOnsets = euclid(8, stepsPerBar, 0)
	} else {
		bassOnsets = euclid(4+rng.IntN(2), stepsPerBar, 0)
	}
	bassColor := make([]int, len(bassOnsets))
	if md.drive {
		style := rng.IntN(3)
		// Some seeds ride the bass an octave higher: a big, cheap timbre shift.
		lift := rng.IntN(2) * scaleLen
		for i := range bassColor {
			bassColor[i] = lift
			switch {
			case style == 0 && i%4 == 3:
				bassColor[i] += scaleLen // octave pop on the last eighth of each beat
			case style == 1 && i%2 == 1:
				bassColor[i] += scaleLen // octave bounce
			case style == 2 && i%2 == 1:
				bassColor[i] += 4 // root-fifth pump
			}
		}
	} else {
		for i := range bassColor {
			r := rng.Float64()
			switch {
			case i == 0 || r < 0.6:
				bassColor[i] = 0 // the root carries the bar
			case r < 0.85:
				bassColor[i] = 2 // chord third/fifth region
			default:
				bassColor[i] = scaleLen // octave up
			}
		}
	}

	// Percussion: drive moods draw kick and snare grooves from the banks and
	// run 16th hats; melodic moods keep the classic sparse grid. Drawn once,
	// repeated every bar.
	kicks := melodicKicks[rng.IntN(len(melodicKicks))]
	snares := []int{4, 12}
	hats := euclid(3+rng.IntN(6), stepsPerBar, rng.IntN(2))
	if md.drive {
		kicks = driveKicks[rng.IntN(len(driveKicks))]
		snares = driveSnares[rng.IntN(len(driveSnares))]
		// From sparse ride to a full 16th wall: the hat density is a large
		// part of how "busy" a track feels.
		hats = euclid(4+rng.IntN(13), stepsPerBar, 0)
	}
	perSeed := rng.Int64()

	var score []note
	for bar := range bars {
		chord := prog[bar%len(prog)]
		base := bar * stepsPerBar

		m := motifs[form[(bar/4)%len(form)]]
		// Drive moods never stop to breathe: the riff hammers through the
		// cadence bars too.
		cadence := !md.drive && bar%4 == 3
		score = append(score, melodyBar(m, base, chord, scaleLen, cadence, md.drive, echo)...)

		for i, s := range bassOnsets {
			score = append(score, note{voice: bassVoice, step: base + s, dur: barGap(bassOnsets, i), degree: chord + bassColor[i]})
		}

		fill := bar%4 == 3
		for _, s := range kicks {
			score = append(score, note{voice: kickVoice, step: base + s, dur: 1, seed: perSeed})
		}
		for _, s := range snares {
			score = append(score, note{voice: snareVoice, step: base + s, dur: 1, seed: perSeed})
		}
		for _, s := range hats {
			score = append(score, note{voice: hatVoice, step: base + s, dur: 1, seed: perSeed})
		}
		if fill {
			// Every fourth bar earns a short roll into the next phrase.
			for s := 12; s < stepsPerBar; s++ {
				score = append(score, note{voice: hatVoice, step: base + s, dur: 1, seed: perSeed})
			}
			score = append(score, note{voice: snareVoice, step: base + 14, dur: 1, seed: perSeed})
		}
	}
	return score
}

// makeMotif draws one bar of melody: a euclidean onset rhythm (density and
// rotation are part of the seed's identity) and a weighted random walk over
// scale degrees, mostly stepwise, centered an octave above the root.
func makeMotif(rng *rand.Rand, scaleLen int) motif {
	onsets := euclid(4+rng.IntN(5), stepsPerBar, rng.IntN(3))
	degrees := make([]int, len(onsets))
	deg := scaleLen // start on the tonic, one octave up
	for i := range degrees {
		deg += walkStep(rng)
		deg = clampDegree(deg, scaleLen)
		degrees[i] = deg
	}
	return motif{onsets: onsets, degrees: degrees}
}

// makeRiff draws one bar of drive-mood melody in one of three rhythm
// archetypes — machine gun (eighths with 16th doubles), 16th burst, or
// syncopated off-beats — hammering chord tones and repeated notes in a tight
// register: a riff, not a tune.
func makeRiff(rng *rand.Rand, scaleLen int) motif {
	var onsets []int
	switch rng.IntN(3) {
	case 0: // machine gun
		for s := 0; s < stepsPerBar; s += 2 {
			onsets = append(onsets, s)
			if rng.Float64() < 0.3 {
				onsets = append(onsets, s+1) // 16th double for urgency
			}
		}
	case 1: // 16th burst
		onsets = euclid(10+rng.IntN(4), stepsPerBar, 0)
	default: // syncopated: the downbeat anchors, the rest pushes off-beat
		onsets = append(onsets, 0)
		for s := 2; s < stepsPerBar; s += 2 {
			if rng.Float64() < 0.75 {
				onsets = append(onsets, s+1)
			} else {
				onsets = append(onsets, s)
			}
		}
	}
	degrees := make([]int, len(onsets))
	// The register itself is part of the seed's identity: some riffs sit on
	// the tonic, others ride higher.
	deg := scaleLen + (rng.IntN(3)-1)*2
	for i := range degrees {
		r := rng.Float64()
		switch {
		case r < 0.35:
			// Hold the note: the repeated-note pedal is the drive signature.
		case r < 0.60:
			deg += 2 // chord-tone leaps keep the riff harmonic
		case r < 0.80:
			deg -= 2
		case r < 0.90:
			deg += 1
		default:
			deg -= 1
		}
		deg = clampDegree(deg, scaleLen)
		degrees[i] = deg
	}
	return motif{onsets: onsets, degrees: degrees}
}

// walkStep draws one melodic interval in scale steps: mostly seconds, the
// occasional third, rarely a repeat.
func walkStep(rng *rand.Rand) int {
	r := rng.Float64()
	switch {
	case r < 0.30:
		return -1
	case r < 0.60:
		return 1
	case r < 0.72:
		return -2
	case r < 0.84:
		return 2
	case r < 0.92:
		return 0
	case r < 0.96:
		return -3
	default:
		return 3
	}
}

// clampDegree keeps the melody inside a singable octave-and-a-half above the
// root, reflecting steps that run past the edges.
func clampDegree(deg, scaleLen int) int {
	lo, hi := scaleLen/2, scaleLen*2
	if deg < lo {
		return lo + (lo - deg)
	}
	if deg > hi {
		return hi - (deg - hi)
	}
	return deg
}

// mutateMotif derives the B-phrase motif: the same rhythm with the melodic
// contour shifted up a third and a couple of degrees redrawn.
func mutateMotif(rng *rand.Rand, m motif) motif {
	out := motif{onsets: m.onsets, degrees: make([]int, len(m.degrees))}
	copy(out.degrees, m.degrees)
	for i := range out.degrees {
		out.degrees[i] += 2
		if rng.Float64() < 0.25 {
			out.degrees[i] += walkStep(rng)
		}
	}
	return out
}

// melodyBar places one motif into a bar: diatonic transposition to the bar's
// chord, chord tones repaired onto the strong steps, a thinner ending on
// cadence bars, and — when the track's seed opted in — a ghost echo in the
// gaps (the one-channel fake delay). Drive bars stay staccato: short notes,
// no holds, no echo.
func melodyBar(m motif, base, chord, scaleLen int, cadence, drive, echo bool) []note {
	var out []note
	for i, s := range m.onsets {
		if cadence && s > 8 {
			break // cadence bars breathe: nothing after the held note
		}
		deg := m.degrees[i] + chord
		if s == 0 || s == 8 {
			deg = nearestChordTone(deg, chord, scaleLen)
		}
		dur := barGap(m.onsets, i)
		if drive && dur > 2 {
			dur = 2 // staccato: the gaps are part of the machine-gun feel
		}
		if cadence && s == lastOnsetAtOrBefore(m.onsets, 8) {
			deg = nearestChordTone(deg, chord, scaleLen)
			dur = stepsPerBar - s // hold the cadence note to the bar line
		}
		// Final register guard at the point of emission: chord transposition
		// and B-phrase mutation can push past the motif's range, and a high
		// square is a whistle that masks the mix. Whole-octave shifts keep
		// both the scale degree and the chord membership intact.
		for deg > scaleLen*2 {
			deg -= scaleLen
		}
		for deg < scaleLen/2 {
			deg += scaleLen
		}
		out = append(out, note{voice: leadVoice, step: base + s, dur: dur, degree: deg})

		// A short note followed by silence earns a ghost repeat two steps
		// later, quieter — the classic single-channel echo.
		if echo && !cadence && !drive && dur <= 2 && gapAfter(m.onsets, i) >= 4 {
			out = append(out, note{voice: echoVoice, step: base + s + 2, dur: dur, degree: deg})
		}
	}
	return out
}

// nearestChordTone snaps a degree to the closest tone of the chord built on
// the given root degree (root, third and fifth as stacked scale steps),
// searching the chord tones across nearby octaves of the scale.
func nearestChordTone(deg, chord, scaleLen int) int {
	best := deg
	bestDist := 1 << 30
	for _, t := range []int{chord, chord + 2, chord + 4} {
		for oct := -2; oct <= 3; oct++ {
			tone := t + oct*scaleLen
			dist := deg - tone
			if dist < 0 {
				dist = -dist
			}
			if dist < bestDist {
				bestDist = dist
				best = tone
			}
		}
	}
	return best
}

// barGap is the duration from onset i to the next onset (or the bar line),
// capped so held notes stay in scale with the groove.
func barGap(onsets []int, i int) int {
	gap := stepsPerBar - onsets[i]
	if i+1 < len(onsets) {
		gap = onsets[i+1] - onsets[i]
	}
	if gap > 4 {
		gap = 4
	}
	return gap
}

// gapAfter is the raw distance from onset i to the next onset or bar line.
func gapAfter(onsets []int, i int) int {
	if i+1 < len(onsets) {
		return onsets[i+1] - onsets[i]
	}
	return stepsPerBar - onsets[i]
}

// lastOnsetAtOrBefore returns the last onset step <= limit.
func lastOnsetAtOrBefore(onsets []int, limit int) int {
	last := onsets[0]
	for _, s := range onsets {
		if s <= limit {
			last = s
		}
	}
	return last
}

// euclid spreads k onsets as evenly as possible over n steps (the Bresenham
// formulation of the Bjorklund algorithm), rotated by rot steps. E(3,8) is
// the tresillo, E(5,8) the cinquillo.
func euclid(k, n, rot int) []int {
	if k <= 0 || n <= 0 {
		return nil
	}
	if k > n {
		k = n
	}
	var onsets []int
	for i := range n {
		// Fire where the Bresenham accumulator crosses an integer. i==0 is
		// spelled out because Go's integer division of (i-1)*k rounds toward
		// zero, not down.
		fire := i == 0
		if i > 0 && i*k/n != (i-1)*k/n {
			fire = true
		}
		if fire {
			onsets = append(onsets, (i+rot)%n)
		}
	}
	// Rotation can break ordering; normalize so callers see ascending steps.
	for i := 1; i < len(onsets); i++ {
		for j := i; j > 0 && onsets[j] < onsets[j-1]; j-- {
			onsets[j], onsets[j-1] = onsets[j-1], onsets[j]
		}
	}
	return onsets
}
