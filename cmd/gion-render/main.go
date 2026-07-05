// Command gion-render writes a preset's sound to a WAV file: the ear-first
// check of the synth core. Render, listen, tweak.
//
//	gion-render [-seed N] [-rate HZ] [-o file.wav] <preset>
//	gion-render [-seed N] [-rate HZ] [-o dir] -all
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/crgimenes/gion"
)

func main() {
	rate := flag.Int("rate", gion.DefaultRate, "sample rate in Hz")
	seed := flag.Int64("seed", 1, "variation seed; the same seed is the same sound")
	out := flag.String("o", "", "output file (default <preset>.wav), or directory with -all")
	all := flag.Bool("all", false, "render every preset")
	flag.Parse()

	if *all {
		err := renderAll(*out, *seed, *rate)
		if err != nil {
			fatal(err)
		}
		return
	}

	name := flag.Arg(0)
	fn, ok := gion.Presets[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "usage: gion-render [-seed N] [-rate HZ] [-o file.wav] <preset>\npresets: %v\n", presetNames())
		os.Exit(2)
	}
	path := *out
	if path == "" {
		path = name + ".wav"
	}
	err := renderFile(path, fn(*seed), *rate)
	if err != nil {
		fatal(err)
	}
}

// renderAll writes one WAV per preset into dir (current directory when empty),
// a quick full-catalog ear pass.
func renderAll(dir string, seed int64, rate int) error {
	for _, name := range presetNames() {
		path := filepath.Join(dir, name+".wav")
		err := renderFile(path, gion.Presets[name](seed), rate)
		if err != nil {
			return err
		}
	}
	return nil
}

func renderFile(path string, p gion.Params, rate int) error {
	// #nosec G304 G302 -- the path is the user's own -o flag, and a rendered
	// WAV is a shareable asset, so world-readable 0644 is the right mode.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	samples := p.Render(rate)
	err = gion.WriteWAV(f, rate, samples)
	if err != nil {
		_ = f.Close() // the write error is the one worth reporting
		return err
	}
	err = f.Close()
	if err != nil {
		return err
	}
	fmt.Printf("%s  %.2fs\n", path, float64(len(samples))/float64(rate))
	return nil
}

func presetNames() []string {
	names := make([]string, 0, len(gion.Presets))
	for name := range gion.Presets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "gion-render:", err)
	os.Exit(1)
}
