package music

import (
	"math"
	"testing"

	"github.com/crgimenes/gion"
)

func TestRenderIsDeterministic(t *testing.T) {
	p := New(Dark, 7)
	a := p.Render(gion.DefaultRate)
	b := p.Render(gion.DefaultRate)
	if len(a) == 0 {
		t.Fatal("no samples")
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("sample %d differs", i)
		}
	}
}

func TestRenderLengthFollowsTempoAndBars(t *testing.T) {
	// An explicit tempo is honored exactly; the default draws anywhere in
	// the mood's range, so the loop length lands between the range bounds.
	exact := Params{Seed: 5, Mood: Upbeat, Tempo: 150, Gain: 0.6}
	want := int(math.Round(60.0 / 150 / 4 * 16 * 16 * gion.DefaultRate))
	if got := len(exact.Render(gion.DefaultRate)); got != want {
		t.Fatalf("explicit tempo: got %d samples, want %d", got, want)
	}

	got := len(New(Upbeat, 1).Render(gion.DefaultRate))
	lo := int(60.0 / 168 * 4 * 16 * gion.DefaultRate)
	hi := int(60.0 / 128 * 4 * 16 * gion.DefaultRate)
	if got < lo || got > hi {
		t.Fatalf("default tempo: got %d, want within [%d, %d]", got, lo, hi)
	}

	slow := Params{Seed: 5, Mood: Upbeat, Tempo: 120, Gain: 0.6}
	fast := Params{Seed: 5, Mood: Upbeat, Tempo: 180, Gain: 0.6}
	if len(slow.Render(gion.DefaultRate)) <= len(fast.Render(gion.DefaultRate)) {
		t.Fatal("a slower tempo must render a longer loop")
	}
}

func TestSeedsChangeIdentity(t *testing.T) {
	// Two rolls of the same mood should differ in pace or key, not just in
	// melodic detail: assert the tempo breathing shows up as a length change
	// across a handful of seeds.
	base := len(New(Battle, 1).Render(gion.DefaultRate))
	varied := false
	for seed := int64(2); seed <= 5; seed++ {
		if len(New(Battle, seed).Render(gion.DefaultRate)) != base {
			varied = true
			break
		}
	}
	if !varied {
		t.Fatal("seeds 1..5 all rendered identical lengths; tempo breathing is not applied")
	}
}

func TestRenderIsAudibleAcrossMoodsAndSeeds(t *testing.T) {
	for name, mood := range Moods {
		out := New(mood, 3).Render(gion.DefaultRate)
		peak := int16(0)
		for _, s := range out {
			if s > peak {
				peak = s
			}
		}
		if peak < math.MaxInt16/8 {
			t.Fatalf("%s: peak too quiet: %d", name, peak)
		}
	}
}

func TestSeedsAndMoodsDiffer(t *testing.T) {
	a := New(Upbeat, 1).Render(gion.DefaultRate)
	b := New(Upbeat, 2).Render(gion.DefaultRate)
	if equalSamples(a, b) {
		t.Fatal("different seeds should give different tracks")
	}
	c := New(Dark, 1).Render(gion.DefaultRate)
	if len(a) == len(c) && equalSamples(a, c) {
		t.Fatal("different moods should give different tracks")
	}
}

func equalSamples(a, b []int16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestEuclidClassicPatterns(t *testing.T) {
	got := euclid(3, 8, 0)
	want := []int{0, 3, 6} // the tresillo
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("E(3,8): got %v, want %v", got, want)
	}
	if n := len(euclid(5, 8, 0)); n != 5 {
		t.Fatalf("E(5,8): %d onsets, want 5", n)
	}
	if n := len(euclid(16, 16, 0)); n != 16 {
		t.Fatalf("E(16,16): %d onsets, want 16", n)
	}
}

func TestDegreeFreq(t *testing.T) {
	major := []int{0, 2, 4, 5, 7, 9, 11}
	if got := degreeFreq(220, major, 0); got != 220 {
		t.Fatalf("tonic: %v", got)
	}
	if got := degreeFreq(220, major, 7); math.Abs(got-440) > 1e-9 {
		t.Fatalf("octave: %v", got)
	}
	if got := degreeFreq(220, major, -7); math.Abs(got-110) > 1e-9 {
		t.Fatalf("octave down: %v", got)
	}
	penta := []int{0, 2, 4, 7, 9}
	if got := degreeFreq(220, penta, 5); math.Abs(got-440) > 1e-9 {
		t.Fatalf("pentatonic octave: %v", got)
	}
}

func TestMuteSilencesInstruments(t *testing.T) {
	p := New(Battle, 9)
	full := p.Render(gion.DefaultRate)

	p.Mute = MuteLead | MuteEcho | MuteBass | MuteKick | MuteSnare | MuteHat
	silent := p.Render(gion.DefaultRate)
	if len(silent) != len(full) {
		t.Fatal("muting must not change the loop length")
	}
	for i, s := range silent {
		if s != 0 {
			t.Fatalf("all-muted track has signal at sample %d", i)
		}
	}

	p.Mute = MuteHat
	noHat := p.Render(gion.DefaultRate)
	if equalSamples(noHat, full) {
		t.Fatal("muting the hat should change the audio")
	}
}
