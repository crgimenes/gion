package gion

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestRenderIsDeterministic(t *testing.T) {
	p := Params{Wave: Noise, Freq: 800, Sustain: 0.1, Decay: 0.1, Gain: 0.5, Seed: 7}
	a := p.Render(DefaultRate)
	b := p.Render(DefaultRate)
	if len(a) == 0 {
		t.Fatal("noise render produced no samples")
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("sample %d differs: %d vs %d", i, a[i], b[i])
		}
	}
}

func TestRenderDurationMatchesEnvelope(t *testing.T) {
	p := Params{Wave: Square, Freq: 440, Attack: 0.1, Sustain: 0.2, Decay: 0.2, Gain: 0.5}
	got := len(p.Render(DefaultRate))
	want := int(0.5 * DefaultRate)
	if got != want {
		t.Fatalf("duration: got %d samples, want %d", got, want)
	}
}

func TestRenderAttackStartsSilent(t *testing.T) {
	p := Params{Wave: Square, Freq: 440, Attack: 0.1, Sustain: 0.1, Decay: 0.1, Gain: 0.5}
	out := p.Render(DefaultRate)
	if out[0] != 0 {
		t.Fatalf("first sample should be silent under an attack, got %d", out[0])
	}
	peak := int16(0)
	for _, s := range out {
		if s > peak {
			peak = s
		}
	}
	if peak < math.MaxInt16/4 {
		t.Fatalf("peak too quiet: %d", peak)
	}
}

func TestRenderFreqLimitStopsEarly(t *testing.T) {
	p := Params{Wave: Square, Freq: 400, FreqSlide: -2000, FreqLimit: 100, Sustain: 1, Gain: 0.5}
	out := p.Render(DefaultRate)
	// The slide crosses the limit at 0.15s, well before the 1s sustain.
	stop := int(0.16 * DefaultRate)
	if len(out) == 0 || len(out) > stop {
		t.Fatalf("freq limit should stop the sound near 0.15s, got %d samples", len(out))
	}
}

func TestRenderCapsRunawayDuration(t *testing.T) {
	p := Params{Wave: Square, Freq: 440, Sustain: 1e9, Gain: 0.5}
	out := p.Render(DefaultRate)
	if len(out) > 10*DefaultRate {
		t.Fatalf("duration cap failed: %d samples", len(out))
	}
}

func TestRenderZeroValueIsSilence(t *testing.T) {
	var p Params
	out := p.Render(DefaultRate)
	if len(out) != 0 {
		t.Fatalf("zero-value Params should render nothing, got %d samples", len(out))
	}
}

func TestPresetsRenderAudibly(t *testing.T) {
	for name, fn := range Presets {
		out := fn(42).Render(DefaultRate)
		if len(out) == 0 {
			t.Fatalf("%s: no samples", name)
		}
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

func TestPresetSeedsVary(t *testing.T) {
	if Pickup(1) == Pickup(2) {
		t.Fatal("different seeds should give different variations")
	}
	if Pickup(3) != Pickup(3) {
		t.Fatal("the same seed should give the same variation")
	}
}

func TestWriteWAVHeader(t *testing.T) {
	var buf bytes.Buffer
	err := WriteWAV(&buf, DefaultRate, []int16{0, 100})
	if err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	if len(b) != 48 {
		t.Fatalf("file length: got %d, want 48", len(b))
	}
	if string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" || string(b[36:40]) != "data" {
		t.Fatalf("bad chunk markers: %q %q %q", b[0:4], b[8:12], b[36:40])
	}
	if got := binary.LittleEndian.Uint32(b[4:8]); got != 40 {
		t.Fatalf("riff size: got %d, want 40", got)
	}
	if got := binary.LittleEndian.Uint32(b[24:28]); got != DefaultRate {
		t.Fatalf("rate: got %d", got)
	}
	if got := binary.LittleEndian.Uint32(b[40:44]); got != 4 {
		t.Fatalf("data size: got %d, want 4", got)
	}
	if got := int16(binary.LittleEndian.Uint16(b[46:48])); got != 100 {
		t.Fatalf("second sample: got %d, want 100", got)
	}
}

func TestMutateIsDeterministicAndKeepsCharacter(t *testing.T) {
	p := Laser(5)
	a := Mutate(p, 9)
	b := Mutate(p, 9)
	if a != b {
		t.Fatal("the same seed should give the same sibling")
	}
	if a == Mutate(p, 10) {
		t.Fatal("different seeds should give different siblings")
	}
	if a.Wave != p.Wave || a.Gain != p.Gain || a.Seed != p.Seed {
		t.Fatalf("wave/gain/seed must not mutate: %+v", a)
	}
	if (a.FreqSlide < 0) != (p.FreqSlide < 0) {
		t.Fatalf("relative drift must keep the slide direction: %v vs %v", a.FreqSlide, p.FreqSlide)
	}
}

func TestMutateLeavesZeroFieldsOff(t *testing.T) {
	p := Params{Wave: Square, Freq: 500, Sustain: 0.1, Gain: 0.5}
	for seed := int64(1); seed <= 20; seed++ {
		m := Mutate(p, seed)
		if m.Vibrato != 0 || m.ArpMult != 0 || m.LowPass != 0 || m.Attack != 0 {
			t.Fatalf("seed %d: an off field was switched on: %+v", seed, m)
		}
	}
}
