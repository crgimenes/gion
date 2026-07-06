package effects

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/crgimenes/gion"
	"github.com/crgimenes/gion/music"
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
	doc, err := Parse(Format(Document{Effects: []Entry{{Name: "boom \"big\"", Params: p}}}))
	if err != nil {
		t.Fatal(err)
	}
	got := doc.Effects
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
	src := Format(Document{Effects: []Entry{{Name: "a", Params: gion.Params{Freq: 100, Gain: 0.5}}}}) +
		"(effect \"b\" (tuple \"Freq\" 200) (tuple \"FutureField\" 1))\n"
	doc, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Effects) != 2 {
		t.Fatalf("entries: %d", len(doc.Effects))
	}
	if doc.Effects[1].Name != "b" || doc.Effects[1].Params.Freq != 200 {
		t.Fatalf("second entry: %+v", doc.Effects[1])
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
	doc := Document{
		Effects: []Entry{
			{Name: "coin", Params: gion.Params{Wave: gion.Square, Freq: 1000, Gain: 0.5}},
			{Name: "boom", Params: gion.Params{Wave: gion.Noise, Decay: 0.8, Gain: 0.6, Seed: 7}},
		},
		Music: []MusicEntry{
			{Name: "stage 1", Params: music.Params{Mood: music.Battle, Seed: 7, Tempo: 180, Bars: 8, Gain: 0.6, Mute: music.MuteHat | music.MuteEcho}},
		},
	}
	err := Save(path, doc)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Effects) != 2 || got.Effects[0] != doc.Effects[0] || got.Effects[1] != doc.Effects[1] {
		t.Fatalf("effects round trip:\n got %+v\nwant %+v", got.Effects, doc.Effects)
	}
	if len(got.Music) != 1 || got.Music[0] != doc.Music[0] {
		t.Fatalf("music round trip:\n got %+v\nwant %+v", got.Music, doc.Music)
	}

	// An emptied document saves to an empty file that loads back as empty.
	err = Save(path, Document{})
	if err != nil {
		t.Fatal(err)
	}
	got, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Empty() {
		t.Fatalf("cleared file: %+v", got)
	}
}

func TestParseEmpty(t *testing.T) {
	doc, err := Parse("")
	if err != nil {
		t.Fatal(err)
	}
	if !doc.Empty() {
		t.Fatalf("entries: %+v", doc)
	}
}
