// Package gion synthesizes 8-bit style game sound effects from a small,
// serializable set of parameters. Rendering is deterministic: the same Params
// always produce the same samples, so a game can ship tiny presets and
// generate the audio at runtime instead of shipping files. Inspired by
// sfxr/bfxr, not a port.
//
// The implementation, and the documentation of each symbol, live in package core.
package gion

import (
	"io"

	"github.com/crgimenes/gion/core"
)

type (
	Params = core.Params
	Wave   = core.Wave
)

const DefaultRate = core.DefaultRate

const (
	Square   = core.Square
	Saw      = core.Saw
	Triangle = core.Triangle
	Sine     = core.Sine
	Noise    = core.Noise
)

var (
	ErrTooLong = core.ErrTooLong
	Presets    = core.Presets
)

func Blip(seed int64) Params { return core.Blip(seed) }

func Explosion(seed int64) Params { return core.Explosion(seed) }

func Hit(seed int64) Params { return core.Hit(seed) }

func Jump(seed int64) Params { return core.Jump(seed) }

func Laser(seed int64) Params { return core.Laser(seed) }

func Pickup(seed int64) Params { return core.Pickup(seed) }

func Powerup(seed int64) Params { return core.Powerup(seed) }

func Mutate(p Params, seed int64) Params { return core.Mutate(p, seed) }

func Spectrogram(samples []int16, frames, bins int) [][]float64 {
	return core.Spectrogram(samples, frames, bins)
}

func WriteWAV(w io.Writer, rate int, samples []int16) error { return core.WriteWAV(w, rate, samples) }
