package music

// The mix contract, learned from crg's ear ("one instrument drowns the
// others" -- it was the bass): the lead owns the top of the mix. Each voice
// renders in isolation and its level is asserted against the lead's.

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestVoiceBalanceKeepsLeadOnTop(t *testing.T) {
	rate := 44100
	for _, tc := range []struct {
		name string
		mood Mood
	}{{"battle", Battle}, {"upbeat", Upbeat}} {
		md := moods[tc.mood]
		root := 220.0
		gain := 0.6
		rng := rand.New(rand.NewPCG(scramble(3), musicStream))
		md.duty = md.duties[rng.IntN(len(md.duties))]
		tempo := md.tempoLo + (md.tempoHi-md.tempoLo)*rng.Float64()
		root *= math.Pow(2, float64(rng.IntN(13)-6)/12)
		stepDur := 60 / tempo / 4
		bars := 16
		total := int(math.Round(stepDur * float64(stepsPerBar*bars) * float64(rate)))
		score := compose(rng, md, bars)

		rms := map[voice]float64{}
		peaks := map[voice]int32{}
		for v, label := range map[voice]string{
			leadVoice: "lead", echoVoice: "echo", bassVoice: "bass",
			kickVoice: "kick", snareVoice: "snare", hatVoice: "hat",
		} {
			mix := make([]int32, total)
			count := 0
			for _, n := range score {
				if n.voice != v {
					continue
				}
				count++
				off := int(math.Round(float64(n.step) * stepDur * float64(rate)))
				addWrap(mix, n.voiceParams(md, root, stepDur, gain, mixLevels{1, 1, 1}).Render(rate), off)
			}
			sum := 0.0
			peak := int32(0)
			for _, s := range mix {
				sum += float64(s) * float64(s)
				if s > peak {
					peak = s
				}
				if -s > peak {
					peak = -s
				}
			}
			rms[v] = math.Sqrt(sum / float64(total))
			peaks[v] = peak
			t.Logf("%s %-5s notes=%3d rms=%6.0f peak=%6d", tc.name, label, count, rms[v], peak)
		}
		// The triangle is a soft timbre: its peak may graze the lead's, but
		// not run past it the way the original drowning bass did (1.28x).
		if float64(peaks[bassVoice]) >= 1.15*float64(peaks[leadVoice]) {
			t.Errorf("%s: bass peak %d too far above the lead peak %d", tc.name, peaks[bassVoice], peaks[leadVoice])
		}
		if rms[bassVoice] >= rms[leadVoice] {
			t.Errorf("%s: bass rms %.0f must stay under the lead rms %.0f", tc.name, rms[bassVoice], rms[leadVoice])
		}
		// A drum transient may peak slightly above the lead (high peak, low
		// RMS is what a kick is), but not by much and never in sustained
		// energy.
		if float64(peaks[kickVoice]) >= 1.2*float64(peaks[leadVoice]) {
			t.Errorf("%s: kick peak %d too far above the lead peak %d", tc.name, peaks[kickVoice], peaks[leadVoice])
		}
		if rms[kickVoice] >= rms[leadVoice] {
			t.Errorf("%s: kick rms %.0f must stay under the lead rms %.0f", tc.name, rms[kickVoice], rms[leadVoice])
		}
		if rms[hatVoice] >= rms[leadVoice]/2 {
			t.Errorf("%s: hat rms %.0f must stay well under the lead rms %.0f", tc.name, rms[hatVoice], rms[leadVoice])
		}
		// The other edge, from crg's "the lead muffles the rest": leading is
		// not drowning.
		if rms[leadVoice] >= 2*rms[bassVoice] {
			t.Errorf("%s: lead rms %.0f drowns the bass rms %.0f (limit 2x)", tc.name, rms[leadVoice], rms[bassVoice])
		}
	}
}

func TestLeadStaysOutOfWhistleRange(t *testing.T) {
	// Also from crg's ear: in-tune high squares ("apitos") mask everything
	// else. The lead melody must stay in the singable register, below
	// ~1.25kHz, across every mood and a spread of seeds.
	for name, mood := range Moods {
		md := moods[mood]
		for seed := int64(1); seed <= 10; seed++ {
			rng := rand.New(rand.NewPCG(scramble(seed), musicStream))
			_ = rng.IntN(len(md.duties)) // the duty draw, consumed as Render does
			_ = rng.Float64()            // the tempo draw
			root := 220 * math.Pow(2, float64(rng.IntN(13)-6)/12)
			for _, n := range compose(rng, md, 16) {
				if n.voice != leadVoice && n.voice != echoVoice {
					continue
				}
				f := degreeFreq(root, md.scale, n.degree)
				if f > 1250 {
					t.Fatalf("%s seed %d: lead note at %.0fHz is in whistle range", name, seed, f)
				}
			}
		}
	}
}
