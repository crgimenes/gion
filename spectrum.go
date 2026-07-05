package gion

import "math"

// fftSize is the analysis window: ~23ms at DefaultRate, fine enough time
// resolution for game-length sounds while keeping the transform cheap.
const fftSize = 1024

// Spectrogram slices the samples into frames spread evenly across the sound
// and returns a frames x bins grid of spectral magnitudes, normalized to 0..1
// and log-compressed so quiet detail stays visible next to the peaks. Each bin
// keeps the peak of its frequency range, which preserves narrow tones. It is
// the data behind the app's waterfall view, exported so a game can drive its
// own visualizer from the same sounds.
func Spectrogram(samples []int16, frames, bins int) [][]float64 {
	if len(samples) == 0 || frames < 1 || bins < 1 {
		return nil
	}
	if bins > fftSize/2 {
		bins = fftSize / 2
	}
	hop := 1
	if frames > 1 && len(samples) > fftSize {
		hop = max((len(samples)-fftSize)/(frames-1), 1)
	}

	// Hann window, computed once per call.
	window := make([]float64, fftSize)
	for i := range window {
		window[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(fftSize-1))
	}

	grid := make([][]float64, frames)
	re := make([]float64, fftSize)
	im := make([]float64, fftSize)
	peak := 0.0
	group := (fftSize / 2) / bins
	for f := range frames {
		start := f * hop
		for i := range fftSize {
			v := 0.0
			if start+i < len(samples) {
				v = float64(samples[start+i]) / (math.MaxInt16 + 1)
			}
			re[i] = v * window[i]
			im[i] = 0
		}
		fft(re, im)

		row := make([]float64, bins)
		for b := range bins {
			m := 0.0
			for k := b * group; k < (b+1)*group; k++ {
				mag := math.Hypot(re[k], im[k])
				if mag > m {
					m = mag
				}
			}
			row[b] = m
			if m > peak {
				peak = m
			}
		}
		grid[f] = row
	}

	if peak == 0 {
		return grid // silence: all zeros, already normalized
	}
	// Log compression over ~40dB: x in 0..1 -> log1p(99x)/log1p(99).
	den := math.Log1p(99)
	for _, row := range grid {
		for b, m := range row {
			row[b] = math.Log1p(99*m/peak) / den
		}
	}
	return grid
}

// fft is an in-place iterative radix-2 transform. Thirty lines beat a
// dependency for the single power-of-two window this package needs.
func fft(re, im []float64) {
	n := len(re)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j |= bit
		if i < j {
			re[i], re[j] = re[j], re[i]
			im[i], im[j] = im[j], im[i]
		}
	}
	for length := 2; length <= n; length <<= 1 {
		ang := -2 * math.Pi / float64(length)
		wr, wi := math.Cos(ang), math.Sin(ang)
		for start := 0; start < n; start += length {
			cr, ci := 1.0, 0.0
			for k := start; k < start+length/2; k++ {
				m := k + length/2
				tr := re[m]*cr - im[m]*ci
				ti := re[m]*ci + im[m]*cr
				re[m], im[m] = re[k]-tr, im[k]-ti
				re[k], im[k] = re[k]+tr, im[k]+ti
				cr, ci = cr*wr-ci*wi, cr*wi+ci*wr
			}
		}
	}
}
