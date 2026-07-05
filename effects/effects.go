// Package effects reads and writes gion effects documents. A document is a
// filo script with the extension Ext where each line declares one named
// effect:
//
//	(effect "coin" (tuple "Freq" 1100) (tuple "Decay" 0.25) (tuple "Gain" 0.5))
//
// The gion app edits these documents; a game consumes them with Load (or
// Parse over an embedded file) and renders each Entry's Params at runtime:
//
//	list, err := effects.Load("sounds.gion")
//	samples := list[0].Params.Render(gion.DefaultRate)
//
// Zero-valued fields are omitted on write and default to zero on read, so a
// document round-trips exactly; unknown fields are ignored on read, so an
// older binary still loads a document written by a newer one.
package effects

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/crgimenes/filo"
	"github.com/crgimenes/gion"
)

// Ext is the document extension. The content is the filo language (like
// kutta's .afoil scenes), but the specific extension names the document type,
// so the OS can associate it with the app.
const Ext = ".gion"

// Entry is one named effect in a document.
type Entry struct {
	Name   string
	Params gion.Params
}

// Load reads and parses the document at path.
func Load(path string) ([]Entry, error) {
	// #nosec G304 G703 -- the path is the caller's own document.
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(string(src))
}

// Save writes the whole document, one form per line.
func Save(path string, entries []Entry) error {
	return os.WriteFile(path, []byte(Format(entries)), 0o600) // #nosec G304 -- caller-chosen path
}

// Parse evaluates a document as a filo script: each (effect ...) form appends
// one entry.
func Parse(src string) ([]Entry, error) {
	if strings.TrimSpace(src) == "" {
		return nil, nil // filo rejects empty scripts; an empty document is fine
	}
	var out []Entry
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

// Format serializes a document, one form per line.
func Format(entries []Entry) string {
	var b strings.Builder
	for _, e := range entries {
		b.WriteString(line(e))
	}
	return b.String()
}

// parseEffect decodes one builtin call: the name, then field tuples in any
// order.
func parseEffect(args []filo.Value) (Entry, error) {
	var e Entry
	if len(args) == 0 {
		return e, fmt.Errorf("effect: missing name")
	}
	name, err := args[0].AsString()
	if err != nil {
		return e, fmt.Errorf("effect name: %w", err)
	}
	e.Name = name
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
		setParam(&e.Params, key, num)
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

// line serializes one effect as a single filo form. Zero fields are omitted:
// absent means the zero value on load, so the round trip is exact.
func line(e Entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "(effect %q", cleanName(e.Name))
	field := func(key string, v float64) {
		if v != 0 {
			fmt.Fprintf(&b, " (tuple %q %s)", key, strconv.FormatFloat(v, 'g', -1, 64))
		}
	}
	p := e.Params
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
