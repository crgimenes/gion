package music

// The distinguishability contract, from crg's ear ("every roll sounds like
// the previous track"): tracks of the same mood must not be spectrally
// near-identical across seeds. Baseline before the identity axes: battle
// 0.879, upbeat 0.734 average pairwise spectrogram correlation.

import (
	"math"
	"testing"

	"github.com/crgimenes/gion"
)

func specVec(m Mood, seed int64) []float64 {
	s := New(m, seed).Render(gion.DefaultRate)
	grid := gion.Spectrogram(s, 48, 40)
	var v []float64
	for _, row := range grid {
		v = append(v, row...)
	}
	return v
}

func pearson(a, b []float64) float64 {
	n := float64(len(a))
	var sa, sb, saa, sbb, sab float64
	for i := range a {
		sa += a[i]
		sb += b[i]
		saa += a[i] * a[i]
		sbb += b[i] * b[i]
		sab += a[i] * b[i]
	}
	num := sab - sa*sb/n
	den := math.Sqrt((saa - sa*sa/n) * (sbb - sb*sb/n))
	return num / den
}

func TestTracksAreDistinguishable(t *testing.T) {
	for _, mood := range []Mood{Battle, Upbeat} {
		var vecs [][]float64
		for seed := int64(1); seed <= 4; seed++ {
			vecs = append(vecs, specVec(mood, seed))
		}
		sum, cnt := 0.0, 0
		for i := range vecs {
			for j := i + 1; j < len(vecs); j++ {
				c := pearson(vecs[i], vecs[j])
				sum += c
				cnt++
			}
		}
		avg := sum / float64(cnt)
		t.Logf("mood %v: avg pairwise spectrogram correlation seeds 1-4 = %.3f", mood, avg)
		// The metric floors around ~0.7: the constant percussion texture of a
		// genre correlates regardless of the tune. The guard catches the real
		// pathology (the 0.879 baseline, when sequential seeds shared tempo,
		// key and timbre through PCG's correlated first draws).
		if avg > 0.85 {
			t.Errorf("mood %v: tracks too similar across seeds (avg correlation %.3f > 0.85)", mood, avg)
		}
	}
}
