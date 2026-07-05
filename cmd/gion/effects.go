package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/crgimenes/filo"
	"github.com/crgimenes/gion"
)

// fileExt is the effects document extension. The content is the filo
// language (like kutta's .afoil scenes), but the specific extension names the
// document type, so the OS can associate it with the app.
const fileExt = ".gion"

// fxEntry is one effect in the working list. The list is the document: it is
// saved whole to a file the consuming game evaluates to get every effect by
// name.
type fxEntry struct {
	name   string
	params gion.Params
}

// readEffectsFile loads a saved effects list.
func readEffectsFile(path string) ([]fxEntry, error) {
	// #nosec G304 G703 -- the path is the user's own: the native dialog or the
	// document argument on the command line.
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseLibrary(string(src))
}

// writeEffectsFile saves the whole effects list, one form per line.
func writeEffectsFile(path string, effects []fxEntry) error {
	var b strings.Builder
	for _, e := range effects {
		b.WriteString(effectLine(e.name, e.params))
	}
	return os.WriteFile(path, []byte(b.String()), 0o600) // #nosec G304 -- user-chosen save path
}

// parseLibrary evaluates the file as a filo script: each line is an
// (effect "name" (tuple "Field" value) ...) form appending one entry.
func parseLibrary(src string) ([]fxEntry, error) {
	if strings.TrimSpace(src) == "" {
		return nil, nil // filo rejects empty scripts; an empty list is fine
	}
	var out []fxEntry
	f := filo.New()
	defer f.Close()
	err := f.RegisterBuiltin("effect", func(_ context.Context, args []filo.Value) (filo.Value, error) {
		e, err := parseEffect(args)
		if err != nil {
			return filo.VBool(false), err
		}
		out = append(out, e)
		return filo.VBool(true), nil
	})
	if err != nil {
		return nil, err
	}
	err = f.DoString(src)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// parseEffect decodes one builtin call: the name, then field tuples in any
// order. Unknown fields are skipped, so an older binary still reads a file
// written by a newer one.
func parseEffect(args []filo.Value) (fxEntry, error) {
	var e fxEntry
	if len(args) == 0 {
		return e, fmt.Errorf("effect: missing name")
	}
	name, err := args[0].AsString()
	if err != nil {
		return e, fmt.Errorf("effect name: %w", err)
	}
	e.name = name
	for _, v := range args[1:] {
		kv, err := v.AsTuple()
		if err != nil {
			return e, fmt.Errorf("effect %q: %w", name, err)
		}
		if len(kv) != 2 {
			return e, fmt.Errorf("effect %q: field tuples take a name and a value", name)
		}
		key, err := kv[0].AsString()
		if err != nil {
			return e, fmt.Errorf("effect %q: %w", name, err)
		}
		num, err := kv[1].AsNumber()
		if err != nil {
			return e, fmt.Errorf("effect %q, field %s: %w", name, key, err)
		}
		setParam(&e.params, key, num)
	}
	return e, nil
}

// setParam maps a serialized field name onto Params.
func setParam(p *gion.Params, key string, v float64) {
	switch key {
	case "Wave":
		p.Wave = gion.Wave(int(v))
	case "Freq":
		p.Freq = v
	case "FreqSlide":
		p.FreqSlide = v
	case "FreqLimit":
		p.FreqLimit = v
	case "Attack":
		p.Attack = v
	case "Sustain":
		p.Sustain = v
	case "Punch":
		p.Punch = v
	case "Decay":
		p.Decay = v
	case "Duty":
		p.Duty = v
	case "Vibrato":
		p.Vibrato = v
	case "VibratoHz":
		p.VibratoHz = v
	case "ArpMult":
		p.ArpMult = v
	case "ArpDelay":
		p.ArpDelay = v
	case "LowPass":
		p.LowPass = v
	case "Bits":
		p.Bits = int(v)
	case "Gain":
		p.Gain = v
	case "Seed":
		p.Seed = int64(v)
	}
}

// effectLine serializes one effect as a single filo form. Zero fields are
// omitted: absent means the zero value on load, so the round trip is exact.
func effectLine(name string, p gion.Params) string {
	var b strings.Builder
	fmt.Fprintf(&b, "(effect %q", cleanName(name))
	field := func(key string, v float64) {
		if v != 0 {
			fmt.Fprintf(&b, " (tuple %q %s)", key, strconv.FormatFloat(v, 'g', -1, 64))
		}
	}
	field("Wave", float64(p.Wave))
	field("Freq", p.Freq)
	field("FreqSlide", p.FreqSlide)
	field("FreqLimit", p.FreqLimit)
	field("Attack", p.Attack)
	field("Sustain", p.Sustain)
	field("Punch", p.Punch)
	field("Decay", p.Decay)
	field("Duty", p.Duty)
	field("Vibrato", p.Vibrato)
	field("VibratoHz", p.VibratoHz)
	field("ArpMult", p.ArpMult)
	field("ArpDelay", p.ArpDelay)
	field("LowPass", p.LowPass)
	field("Bits", float64(p.Bits))
	field("Gain", p.Gain)
	field("Seed", float64(p.Seed))
	b.WriteString(")\n")
	return b.String()
}

// cleanName keeps the stored name on one line and free of quote escapes.
func cleanName(name string) string {
	name = strings.ReplaceAll(name, `"`, "'")
	name = strings.ReplaceAll(name, "\n", " ")
	return name
}
