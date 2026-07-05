package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/crgimenes/gion"
)

func TestEffectLineRoundTrip(t *testing.T) {
	p := gion.Params{
		Wave:      gion.Noise,
		Freq:      1234.5,
		FreqSlide: -6000,
		FreqLimit: 100,
		Attack:    0.01,
		Sustain:   0.2,
		Punch:     0.5,
		Decay:     0.75,
		Duty:      0.3,
		Vibrato:   40,
		VibratoHz: 9.5,
		ArpMult:   1.5,
		ArpDelay:  0.06,
		LowPass:   1800,
		Bits:      8,
		Gain:      0.6,
		Seed:      42,
	}
	got, err := parseLibrary(effectLine("boom \"big\"", p))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("entries: %d", len(got))
	}
	if got[0].name != "boom 'big'" {
		t.Fatalf("name: %q", got[0].name)
	}
	if got[0].params != p {
		t.Fatalf("params round trip:\n got %+v\nwant %+v", got[0].params, p)
	}
}

func TestParseLibraryMultipleAndUnknownField(t *testing.T) {
	src := effectLine("a", gion.Params{Freq: 100, Gain: 0.5}) +
		"(effect \"b\" (tuple \"Freq\" 200) (tuple \"FutureField\" 1))\n"
	got, err := parseLibrary(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("entries: %d", len(got))
	}
	if got[1].name != "b" || got[1].params.Freq != 200 {
		t.Fatalf("second entry: %+v", got[1])
	}
}

func TestParseLibraryRejectsMalformedField(t *testing.T) {
	_, err := parseLibrary("(effect \"x\" (tuple \"Freq\"))\n")
	if err == nil || !strings.Contains(err.Error(), "x") {
		t.Fatalf("want a field arity error naming the effect, got %v", err)
	}
}

func TestEffectsFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "effects.filo")
	list := []fxEntry{
		{name: "coin", params: gion.Params{Wave: gion.Square, Freq: 1000, Gain: 0.5}},
		{name: "boom", params: gion.Params{Wave: gion.Noise, Decay: 0.8, Gain: 0.6, Seed: 7}},
	}
	err := writeEffectsFile(path, list)
	if err != nil {
		t.Fatal(err)
	}
	got, err := readEffectsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != list[0] || got[1] != list[1] {
		t.Fatalf("round trip:\n got %+v\nwant %+v", got, list)
	}

	// An emptied list saves to an empty file that loads back as empty.
	err = writeEffectsFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err = readEffectsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("cleared file: %d entries", len(got))
	}
}

func TestParseLibraryEmpty(t *testing.T) {
	got, err := parseLibrary("")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("entries: %d", len(got))
	}
}
