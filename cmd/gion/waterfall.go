package main

import (
	"image"
	"image/color"
	"math"

	"github.com/crgimenes/gion"
	ui "github.com/crgimenes/minigui"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Spectrogram resolution and the shared visualization rect, right of the
// slider column.
const (
	specFrames = 80
	specBins   = 64

	vizX, vizY = 520.0, 12.0
	vizW, vizH = 404.0, 440.0
)

// The relief spins freely; only the tilt is clamped so the view stays above
// the plane.
const (
	autoSpin = 0.003 // radians per tick of idle rotation
	pitchMin = 0.12
	pitchMax = 1.35
)

// updateViz spins the relief slowly when idle and lets a drag inside the view
// steer the camera: horizontal movement turns it, vertical movement tilts it.
func (a *app) updateViz(in ui.Input) {
	inViz := in.MouseX >= vizX && in.MouseX < vizX+vizW && in.MouseY >= vizY && in.MouseY < vizY+vizH
	if in.MouseClicked && inViz {
		a.vizDrag = true
	}
	if !in.MouseDown {
		a.vizDrag = false
	}
	if a.vizDrag {
		a.yaw += (in.MouseX - a.lastMX) * 0.008
		a.pitch = clamp(a.pitch+(in.MouseY-a.lastMY)*0.005, pitchMin, pitchMax)
	} else {
		a.yaw += autoSpin
	}
	a.lastMX, a.lastMY = in.MouseX, in.MouseY
}

// cam is the frame's precomputed yaw/pitch rotation and screen mapping.
type cam struct {
	cy, sy, cp, sp float64
	cx, cyc, scale float64
}

// project maps relief coordinates (x across frequency, y up, z across time,
// all roughly -1..1) through the camera to screen pixels: yaw around the
// vertical axis, a downward pitch so the relief is seen from above, then a
// perspective divide.
func (c cam) project(x, y, z float64) (float32, float32) {
	px := x*c.cy + z*c.sy
	pz := -x*c.sy + z*c.cy
	py := y*c.cp + pz*c.sp
	pz = pz*c.cp - y*c.sp
	s := 3 / (3.6 + pz)
	return float32(c.cx + px*s*c.scale), float32(c.cyc - py*s*c.scale)
}

// drawEmptyNotice explains a blank panel: a silent parameter set is a state,
// not a rendering failure.
func (a *app) drawEmptyNotice(screen *ebiten.Image) {
	msg := "sound is empty (0 samples)\ncheck durations and SlideStop"
	ebitenutil.DebugPrintAt(screen, msg, int(vizX)+16, int(vizY+vizH/2)-16)
}

// drawWaterfall paints the spectrogram as a 3D relief: time recedes into the
// screen, frequency runs across, amplitude rises. Slices are drawn far to
// near, colored by intensity, with the playing slice highlighted.
func (a *app) drawWaterfall(screen *ebiten.Image) {
	st := a.gui.Style()
	vector.StrokeRect(screen, vizX, vizY, vizW, vizH, 1, st.Border, false)
	if len(a.samples) == 0 {
		a.drawEmptyNotice(screen)
		return
	}
	if len(a.spec) == 0 {
		return
	}
	clip := screen.SubImage(image.Rect(vizX+1, vizY+1, vizX+vizW-1, vizY+vizH-1)).(*ebiten.Image)

	c := cam{
		cy: math.Cos(a.yaw), sy: math.Sin(a.yaw),
		cp: math.Cos(a.pitch), sp: math.Sin(a.pitch),
		cx:    vizX + vizW/2,
		cyc:   vizY + vizH*0.55,
		scale: math.Min(vizW, vizH) * 0.42,
	}

	playFrame := -1
	if a.player != nil && a.player.IsPlaying() && len(a.samples) > 0 {
		frac := a.player.Position().Seconds() / (float64(len(a.samples)) / rate)
		playFrame = int(frac * float64(len(a.spec)))
		playFrame = int(clamp(float64(playFrame), 0, float64(len(a.spec)-1)))
	}

	frames := len(a.spec)
	bins := len(a.spec[0])
	// Painter's order follows the turn: slice depth is dominated by z*cos(yaw),
	// so past the quarter turn the traversal flips, and the relief can spin the
	// whole way around.
	flip := c.cy < 0
	for i := range frames {
		f := i
		if flip {
			f = frames - 1 - i
		}
		// Frame 0 (the start of the sound) sits at z=+1; playback then runs
		// toward z=-1.
		z := 1.0
		if frames > 1 {
			z = 1 - 2*float64(f)/float64(frames-1)
		}
		width := float32(1)
		if f == playFrame {
			width = 2
		}
		px, py := c.project(-1, a.spec[f][0]*0.9, z)
		for b := 1; b < bins; b++ {
			x := -1 + 2*float64(b)/float64(bins-1)
			amp := a.spec[f][b]
			qx, qy := c.project(x, amp*0.9, z)
			if prev := a.spec[f][b-1]; prev > amp {
				amp = prev
			}
			col := heatColor(amp)
			if f == playFrame {
				col = st.Focus
			}
			vector.StrokeLine(clip, px, py, qx, qy, width, col, true)
			px, py = qx, qy
		}
	}
}

// heatStops is the amplitude color ramp, quiet to loud: deep navy, blue, cyan,
// green, yellow, hot white. The classic audio-waterfall palette.
var heatStops = []color.RGBA{
	{0x14, 0x1c, 0x48, 0xff},
	{0x20, 0x50, 0xc0, 0xff},
	{0x20, 0xc0, 0xd0, 0xff},
	{0x40, 0xe8, 0x60, 0xff},
	{0xf8, 0xe0, 0x40, 0xff},
	{0xff, 0xff, 0xe0, 0xff},
}

// heatColor maps an amplitude in 0..1 onto the ramp, blending between the two
// surrounding stops.
func heatColor(t float64) color.RGBA {
	t = clamp(t, 0, 1) * float64(len(heatStops)-1)
	i := int(t)
	if i >= len(heatStops)-1 {
		return heatStops[len(heatStops)-1]
	}
	return lerpColor(heatStops[i], heatStops[i+1], t-float64(i))
}

// lerpColor blends a into b by t in 0..1.
func lerpColor(a, b color.RGBA, t float64) color.RGBA {
	mix := func(x, y uint8) uint8 {
		return uint8(float64(x) + (float64(y)-float64(x))*t)
	}
	return color.RGBA{mix(a.R, b.R), mix(a.G, b.G), mix(a.B, b.B), 0xff}
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// spectrogram recomputes the waterfall grid for the current samples.
func (a *app) spectrogram() {
	a.spec = gion.Spectrogram(a.samples, specFrames, specBins)
}
