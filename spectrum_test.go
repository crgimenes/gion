package gion

import "testing"

// tone renders a steady test tone long enough for several analysis frames.
func tone(freq float64) []int16 {
	p := Params{Wave: Sine, Freq: freq, Sustain: 0.5, Gain: 0.8}
	return p.Render(DefaultRate)
}

// argmax returns the index of the largest value in the row.
func argmax(row []float64) int {
	best := 0
	for i, v := range row {
		if v > row[best] {
			best = i
		}
	}
	return best
}

func TestSpectrogramLocatesTone(t *testing.T) {
	const bins = 64
	// With a 1024-sample window at 44100Hz, each of the 64 bins spans
	// 8 raw bins of ~43Hz: bin = freq / (43.066 * 8).
	cases := []struct {
		freq float64
		bin  int
	}{
		{440, 1},
		{4400, 12},
	}
	for _, tc := range cases {
		grid := Spectrogram(tone(tc.freq), 8, bins)
		if len(grid) != 8 || len(grid[0]) != bins {
			t.Fatalf("%gHz: grid is %dx%d", tc.freq, len(grid), len(grid[0]))
		}
		got := argmax(grid[4]) // mid-sound frame, away from edge padding
		if got != tc.bin {
			t.Fatalf("%gHz: peak at bin %d, want %d", tc.freq, got, tc.bin)
		}
	}
}

func TestSpectrogramNormalizedRange(t *testing.T) {
	grid := Spectrogram(tone(880), 6, 32)
	peak := 0.0
	for _, row := range grid {
		for _, v := range row {
			if v < 0 || v > 1 {
				t.Fatalf("value out of range: %v", v)
			}
			if v > peak {
				peak = v
			}
		}
	}
	if peak != 1 {
		t.Fatalf("peak should normalize to 1, got %v", peak)
	}
}

func TestSpectrogramIsDeterministic(t *testing.T) {
	s := Explosion(9).Render(DefaultRate)
	a := Spectrogram(s, 16, 48)
	b := Spectrogram(s, 16, 48)
	for f := range a {
		for i := range a[f] {
			if a[f][i] != b[f][i] {
				t.Fatalf("frame %d bin %d differs", f, i)
			}
		}
	}
}

func TestSpectrogramDegenerateInputs(t *testing.T) {
	if Spectrogram(nil, 8, 8) != nil {
		t.Fatal("empty samples should give a nil grid")
	}
	if Spectrogram([]int16{1, 2, 3}, 0, 8) != nil {
		t.Fatal("zero frames should give a nil grid")
	}
	// Fewer samples than one window: zero-padded, still a valid grid.
	grid := Spectrogram(make([]int16, 100), 4, 8)
	if len(grid) != 4 {
		t.Fatalf("short input: got %d frames", len(grid))
	}
}
