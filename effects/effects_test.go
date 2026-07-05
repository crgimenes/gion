package effects

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/crgimenes/gion"
)

func TestFormatParseRoundTrip(t *testing.T) {
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
	got, err := Parse(Format([]Entry{{Name: "boom \"big\"", Params: p}}))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("entries: %d", len(got))
	}
	if got[0].Name != "boom 'big'" {
		t.Fatalf("name: %q", got[0].Name)
	}
	if got[0].Params != p {
		t.Fatalf("params round trip:\n got %+v\nwant %+v", got[0].Params, p)
	}
}

func TestParseMultipleAndUnknownField(t *testing.T) {
	src := Format([]Entry{{Name: "a", Params: gion.Params{Freq: 100, Gain: 0.5}}}) +
		"(effect \"b\" (tuple \"Freq\" 200) (tuple \"FutureField\" 1))\n"
	got, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("entries: %d", len(got))
	}
	if got[1].Name != "b" || got[1].Params.Freq != 200 {
		t.Fatalf("second entry: %+v", got[1])
	}
}

func TestParseRejectsMalformedField(t *testing.T) {
	_, err := Parse("(effect \"x\" (tuple \"Freq\"))\n")
	if err == nil || !strings.Contains(err.Error(), "x") {
		t.Fatalf("want a field arity error naming the effect, got %v", err)
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "effects"+Ext)
	list := []Entry{
		{Name: "coin", Params: gion.Params{Wave: gion.Square, Freq: 1000, Gain: 0.5}},
		{Name: "boom", Params: gion.Params{Wave: gion.Noise, Decay: 0.8, Gain: 0.6, Seed: 7}},
	}
	err := Save(path, list)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != list[0] || got[1] != list[1] {
		t.Fatalf("round trip:\n got %+v\nwant %+v", got, list)
	}

	// An emptied list saves to an empty file that loads back as empty.
	err = Save(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("cleared file: %d entries", len(got))
	}
}

func TestParseEmpty(t *testing.T) {
	got, err := Parse("")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("entries: %d", len(got))
	}
}
