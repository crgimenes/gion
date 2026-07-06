// Command gion is the sound-effect workbench over the gion library: preset
// buttons roll seeded variations, sliders shape the parameters, the result
// plays as soon as a drag is released, and the current sound can be saved as
// a WAV file. The synthesis lives in the library; this is a thin
// immediate-mode front end.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"math"
	mrand "math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/crgimenes/gion"
	fx "github.com/crgimenes/gion/effects"
	"github.com/crgimenes/gion/music"
	ui "github.com/crgimenes/minigui"
	"github.com/crgimenes/native/filedialog"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	winW = 940
	winH = 468
	rate = gion.DefaultRate

	// sideW is the width of the command column: its buttons, the name field
	// and the effect list all share it, so the column reads as one block.
	sideW = 150
	mainX = 186 // slider column, right of the command column
)

// presetOrder fixes the button layout; the Presets map has no stable order.
var presetOrder = []string{"pickup", "laser", "explosion", "powerup", "hit", "jump", "blip"}

// waves pairs each oscillator with its toggle label, in display order.
var waves = []struct {
	wave  gion.Wave
	label string
}{
	{gion.Square, "square"},
	{gion.Saw, "saw"},
	{gion.Triangle, "triangle"},
	{gion.Sine, "sine"},
	{gion.Noise, "noise"},
}

// moodOrder pairs each music mood with its toggle label, in display order.
var moodOrder = []struct {
	mood  music.Mood
	label string
}{
	{music.Upbeat, "upbeat"},
	{music.Heroic, "heroic"},
	{music.Dark, "dark"},
	{music.Chill, "chill"},
	{music.Battle, "battle"},
	{music.Boss, "boss"},
}

// muteOrder pairs each instrument mute bit with its toggle label.
var muteOrder = []struct {
	bit   int
	label string
}{
	{music.MuteLead, "lead"},
	{music.MuteEcho, "echo"},
	{music.MuteBass, "bass"},
	{music.MuteKick, "kick"},
	{music.MuteSnare, "snare"},
	{music.MuteHat, "hat"},
}

// moodLabel names a mood for status lines and generated track names.
func moodLabel(m music.Mood) string {
	for _, e := range moodOrder {
		if e.mood == m {
			return e.label
		}
	}
	return "music"
}

type app struct {
	gui  ui.Context // slider column
	side ui.Context // command column: buttons, name field, effect list
	actx *audio.Context

	preset  string
	seed    int64
	params  gion.Params
	samples []int16
	player  *audio.Player
	edited  bool // a parameter changed this frame; refresh the view live
	dirty   bool // a slider moved since the last playback; play on release
	err     string

	// The list: built-in families first, then the working document — the
	// user's effects and music tracks, saved whole via the native dialogs.
	effects  []fx.Entry
	musics   []fx.MusicEntry
	listSel  int
	fxName   string
	savePath string // current document; empty until the first save or open

	// Music editing state: when musicMode is on, the parameter column edits
	// mparams and the sound is a looping track instead of an effect.
	musicMode bool
	mparams   music.Params

	// Visualization state: the waterfall grid and its camera.
	view3D         bool
	spec           [][]float64
	yaw, pitch     float64
	vizDrag        bool
	lastMX, lastMY float64
}

func (a *app) Update() error {
	in := ui.InputFromEbiten()
	a.sideColumn(in)
	a.gui.Begin(in, mainX, 12)
	if a.musicMode {
		a.moodRow()
		a.musicSliderRows()
	} else {
		a.waveRow()
		a.sliderRows()
	}
	if a.err != "" {
		a.gui.Label("error: " + a.err)
	}
	a.gui.End()
	a.updateViz(in)

	// A document dropped onto the window opens like a double-click.
	files := ebiten.DroppedFiles()
	if files != nil {
		a.openDropped(files)
	}

	// Delete or Backspace removes the selected saved effect, as long as no
	// text field owns the keyboard.
	if !a.side.HasFocus() && !a.gui.HasFocus() &&
		(inpututil.IsKeyJustPressed(ebiten.KeyBackspace) || inpututil.IsKeyJustPressed(ebiten.KeyDelete)) {
		a.deleteSelected()
	}

	// The view follows the sliders live; the sound only plays on release, so
	// a drag does not spam half-finished sounds. Music renders whole tracks,
	// too heavy per frame, so it also waits for the release.
	if a.edited {
		a.edited = false
		if !a.musicMode {
			a.rebuild()
		}
	}
	if a.dirty && !in.MouseDown {
		a.dirty = false
		if a.musicMode {
			a.rebuild()
		}
		a.play()
	}
	return nil
}

// sideColumn is the command column: uniform buttons, the effect name field,
// the file operations, and the effect list — built-in families first, then
// the working document, so the catalog can grow without crowding the screen.
func (a *app) sideColumn(in ui.Input) {
	a.side.Begin(in, 16, 12)
	// Buttons come in half-width pairs so the whole command set fits the
	// column: sound actions first, then the file operations.
	a.side.SetItemWidth((sideW - 4) / 2)
	if a.side.Button("s.play", "Play") {
		a.play()
	}
	a.side.SameLine()
	if a.side.Button("s.roll", "Roll") {
		a.roll()
	}
	if a.side.Button("s.mutate", "Mutate") {
		a.mutate()
	}
	a.side.SameLine()
	if a.side.Button("s.wav", "Save WAV") {
		a.save()
	}
	if a.side.Button("s.open", "Open") {
		a.openFile()
	}
	a.side.SameLine()
	if a.side.Button("s.save", "Save") {
		a.saveFile()
	}
	if a.side.Button("s.saveas", "Save As") {
		a.saveFileAs()
	}
	a.side.SameLine()
	if a.side.Button("s.clear", "Clear") {
		a.clearList()
	}
	a.side.SetItemWidth(sideW)
	a.side.Label("effect name:")
	a.side.TextField("s.name", &a.fxName)
	a.side.SetItemWidth((sideW - 4) / 2)
	if a.side.Button("s.add", "Add FX") {
		a.addEffect()
	}
	a.side.SameLine()
	if a.side.Button("s.addmus", "Add Music") {
		a.addMusic()
	}
	a.side.SetItemWidth(sideW)
	a.side.Label("effects:")
	changed, clicked, _ := a.side.ListWithIcons("s.list", a.listItems(), &a.listSel, 0)
	if changed {
		a.selectEffect(a.listSel)
	}
	if !changed && clicked >= 0 {
		// A second click on the selected entry just plays it again.
		a.play()
	}
	seed := a.seed
	if a.musicMode {
		seed = a.mparams.Seed
	}
	status := fmt.Sprintf("%s %d %.2fs", a.preset, seed, float64(len(a.samples))/rate)
	a.side.Label(truncate(status, 25))
	a.side.End()
}

// truncate caps a label to n runes so a long effect name stays inside the
// command column.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "~"
}

// listItems is the shared catalog: families to roll variations from, then
// the user's saved effects, then the music tracks.
func (a *app) listItems() []string {
	items := make([]string, 0, len(presetOrder)+len(a.effects)+len(a.musics))
	items = append(items, presetOrder...)
	for _, e := range a.effects {
		items = append(items, e.Name)
	}
	for _, m := range a.musics {
		items = append(items, "m: "+m.Name)
	}
	return items
}

// normalizeMix materializes the mixer defaults (0 means 1.0 in the library)
// so the sliders show the real levels and saves store what is heard.
func normalizeMix(p *music.Params) {
	if p.LeadVol == 0 {
		p.LeadVol = 1
	}
	if p.BassVol == 0 {
		p.BassVol = 1
	}
	if p.DrumVol == 0 {
		p.DrumVol = 1
	}
}

// musicStart is the list index of the first music entry.
func (a *app) musicStart() int {
	return len(presetOrder) + len(a.effects)
}

// selectEffect loads the list selection: families re-derive from the current
// seed, saved effects and tracks load their stored parameters verbatim.
func (a *app) selectEffect(i int) {
	if i < 0 {
		return
	}
	if i < len(presetOrder) {
		a.musicMode = false
		a.preset = presetOrder[i]
		a.params = gion.Presets[a.preset](a.seed)
		a.rebuild()
		a.play()
		return
	}
	j := i - len(presetOrder)
	if j < len(a.effects) {
		a.musicMode = false
		a.preset = a.effects[j].Name
		a.params = a.effects[j].Params
		a.rebuild()
		a.play()
		return
	}
	k := i - a.musicStart()
	if k >= len(a.musics) {
		return
	}
	a.musicMode = true
	a.preset = a.musics[k].Name
	a.mparams = a.musics[k].Params
	normalizeMix(&a.mparams)
	a.rebuild()
	a.play()
}

// newSeed draws a fresh random seed. Exploration is meant to surprise — the
// runtime seeds the global generator with real entropy, so every launch and
// every roll differs; determinism lives in the seed VALUE, which is shown,
// stored in documents and reproduces the exact sound anywhere.
func newSeed() int64 {
	return int64(mrand.IntN(1000)) // #nosec G404 -- exploration, not security
}

// roll gives the current sound a fresh variation: a family re-derives all
// parameters from the new seed, a saved effect or track gets a new seed only.
func (a *app) roll() {
	a.seed = newSeed()
	if a.musicMode {
		a.mparams.Seed = a.seed
		a.rebuild()
		a.play()
		return
	}
	a.params.Seed = a.seed
	if a.listSel < len(presetOrder) {
		a.params = gion.Presets[presetOrder[a.listSel]](a.seed)
	}
	a.rebuild()
	a.play()
}

// mutate replaces the current effect with a small deterministic sibling —
// the exploration move: keep what the sound is, drift how it sounds. A music
// track has no fine-grained fields to drift, so it just rolls.
func (a *app) mutate() {
	if a.musicMode {
		a.roll()
		return
	}
	a.seed = newSeed()
	a.params = gion.Mutate(a.params, a.seed)
	a.rebuild()
	a.play()
}

// addEffect appends the current effect to the list under the typed name (or
// a generated one) and selects the new entry. The list only touches disk
// through Save/Save As. Committing immediately auditions a mutated sibling,
// so repeated Add clicks build a family instead of duplicating one entry.
func (a *app) addEffect() {
	name := strings.TrimSpace(a.fxName)
	if name == "" {
		name = fmt.Sprintf("%s-%d", a.preset, a.seed)
	}
	a.effects = append(a.effects, fx.Entry{Name: name, Params: a.params})
	a.musicMode = false
	a.listSel = len(presetOrder) + len(a.effects) - 1
	a.preset = name

	a.seed = newSeed()
	a.params = gion.Mutate(a.params, a.seed)
	a.rebuild()
	a.play()
}

// addMusic appends the current track parameters to the list under the typed
// name (or a generated one) and switches to music editing. What was heard is
// exactly what is saved; the commit then rolls and auditions the next
// candidate, so repeated Add clicks yield new tracks, not duplicates.
func (a *app) addMusic() {
	name := strings.TrimSpace(a.fxName)
	if name == "" {
		name = fmt.Sprintf("%s-%d", moodLabel(a.mparams.Mood), a.mparams.Seed)
	}
	a.musics = append(a.musics, fx.MusicEntry{Name: name, Params: a.mparams})
	a.musicMode = true
	a.listSel = a.musicStart() + len(a.musics) - 1
	a.preset = name

	a.mparams.Seed = newSeed()
	a.rebuild()
	a.play()
}

// deleteSelected removes the selected saved effect or track from the list;
// the built-in families are not deletable.
func (a *app) deleteSelected() {
	j := a.listSel - len(presetOrder)
	if j < 0 {
		return
	}
	if j < len(a.effects) {
		a.effects = slices.Delete(a.effects, j, j+1)
	} else {
		k := j - len(a.effects)
		if k >= len(a.musics) {
			return
		}
		a.musics = slices.Delete(a.musics, k, k+1)
	}
	if a.listSel >= len(presetOrder)+len(a.effects)+len(a.musics) && a.listSel > 0 {
		a.listSel--
	}
}

// clearList drops every saved entry, keeping the current sound and file path.
func (a *app) clearList() {
	a.effects = nil
	a.musics = nil
	if a.listSel >= len(presetOrder) {
		a.listSel = 0
	}
}

// openFile loads an effects document chosen in the native dialog, replacing
// the working list.
func (a *app) openFile() {
	var path string
	ebiten.RunOnMainThread(func() {
		path = filedialog.Open(filedialog.Options{
			Title:      "Open effects (" + fx.Ext + ")",
			Extensions: []string{fx.Ext[1:]},
		})
	})
	if path == "" {
		return // cancelled
	}
	a.openPath(path)
}

// openPath loads the document at path — from the dialog, the command line
// (which is how an OS file association hands over a double-clicked file), or
// anywhere else the path is known.
func (a *app) openPath(path string) {
	doc, err := fx.Load(path)
	if err != nil {
		a.err = err.Error()
		return
	}
	a.adoptDoc(doc)
	a.savePath = path
	a.setTitle()
}

// openDropped loads a document dropped onto the window. The drop API hides
// the real path, so the next Save asks for one.
func (a *app) openDropped(files fs.FS) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), fx.Ext) {
			continue
		}
		src, err := fs.ReadFile(files, e.Name())
		if err != nil {
			a.err = err.Error()
			return
		}
		doc, err := fx.Parse(string(src))
		if err != nil {
			a.err = err.Error()
			return
		}
		a.adoptDoc(doc)
		a.savePath = ""
		ebiten.SetWindowTitle("gion — " + e.Name())
		return
	}
}

// adoptDoc installs a loaded document and shows its first entry, without
// playing anything: opening a file should be silent.
func (a *app) adoptDoc(doc fx.Document) {
	a.effects = doc.Effects
	a.musics = doc.Music
	a.err = ""
	if len(doc.Effects) > 0 {
		a.musicMode = false
		a.listSel = len(presetOrder)
		a.preset = doc.Effects[0].Name
		a.params = doc.Effects[0].Params
		a.rebuild()
		return
	}
	if len(doc.Music) > 0 {
		a.musicMode = true
		a.listSel = a.musicStart()
		a.preset = doc.Music[0].Name
		a.mparams = doc.Music[0].Params
		normalizeMix(&a.mparams)
		a.rebuild()
	}
}

// saveFile writes the whole list to the current document, or asks for a path
// on the first save.
func (a *app) saveFile() {
	if a.savePath == "" {
		a.saveFileAs()
		return
	}
	err := fx.Save(a.savePath, fx.Document{Effects: a.effects, Music: a.musics})
	if err != nil {
		a.err = err.Error()
		return
	}
	a.err = ""
}

// saveFileAs asks for a path in the native dialog and writes the whole list.
func (a *app) saveFileAs() {
	name := "effects" + fx.Ext
	if a.savePath != "" {
		name = filepath.Base(a.savePath)
	}
	var path string
	ebiten.RunOnMainThread(func() {
		path = filedialog.Save(filedialog.Options{
			Title:      "Save effects (" + fx.Ext + ")",
			Filename:   name,
			Extensions: []string{fx.Ext[1:]},
		})
	})
	if path == "" {
		return // cancelled
	}
	if !strings.HasSuffix(path, fx.Ext) {
		path += fx.Ext
	}
	err := fx.Save(path, fx.Document{Effects: a.effects, Music: a.musics})
	if err != nil {
		a.err = err.Error()
		return
	}
	a.savePath = path
	a.err = ""
	a.setTitle()
}

// setTitle shows the current document in the window title.
func (a *app) setTitle() {
	title := "gion"
	if a.savePath != "" {
		title += " — " + filepath.Base(a.savePath)
	}
	ebiten.SetWindowTitle(title)
}

// waveRow is one line of toggles: the oscillator shapes, then the two view
// modes.
func (a *app) waveRow() {
	for i, w := range waves {
		if i > 0 {
			a.gui.SameLine()
		}
		if a.gui.Toggle(ui.ID("w."+w.label), w.label, a.params.Wave == w.wave) {
			a.params.Wave = w.wave
			a.rebuild()
			a.play()
		}
	}
	a.viewToggles()
}

// moodRow is the music counterpart of waveRow: one line of mood toggles plus
// the view modes.
func (a *app) moodRow() {
	for i, m := range moodOrder {
		if i > 0 {
			a.gui.SameLine()
		}
		if a.gui.Toggle(ui.ID("md."+m.label), m.label, a.mparams.Mood == m.mood) {
			a.mparams.Mood = m.mood
			a.rebuild()
			a.play()
		}
	}
	a.viewToggles()
}

// viewToggles appends the 3D/2D view switch to the current row.
func (a *app) viewToggles() {
	a.gui.SameLine()
	if a.gui.Toggle("act.3d", "3D", a.view3D) {
		a.view3D = true
	}
	a.gui.SameLine()
	if a.gui.Toggle("act.2d", "2D", !a.view3D) {
		a.view3D = false
	}
}

// musicSliderRows exposes the track parameters when a music entry is being
// edited. Rendering happens on release (see Update), so dragging stays light.
func (a *app) musicSliderRows() {
	a.slider("Tempo", &a.mparams.Tempo, 60, 220, "%.0f")

	bars := float64(a.mparams.Bars)
	if a.slider("Bars", &bars, 4, 32, "%.0f") {
		a.mparams.Bars = int(math.Round(bars))
	}
	seed := float64(a.mparams.Seed)
	if a.slider("Seed", &seed, 0, 999, "%.0f") {
		a.mparams.Seed = int64(math.Round(seed))
	}
	a.slider("Root", &a.mparams.Root, 55, 880, "%.0f")
	a.slider("Gain", &a.mparams.Gain, 0, 1, "%.2f")
	a.slider("Lead", &a.mparams.LeadVol, 0.05, 1.5, "%.2f")
	a.slider("Bass", &a.mparams.BassVol, 0.05, 1.5, "%.2f")
	a.slider("Drums", &a.mparams.DrumVol, 0.05, 1.5, "%.2f")
	a.muteRow()
}

// muteRow lets each instrument be silenced alone — to hear which voice
// misbehaves, or to ship a reduced arrangement (a track can sound better
// without a voice; disabling one is a legitimate arrangement decision). A
// lit toggle means the instrument is sounding.
func (a *app) muteRow() {
	for i, m := range muteOrder {
		if i > 0 {
			a.gui.SameLine()
		}
		on := a.mparams.Mute&m.bit == 0
		if a.gui.Toggle(ui.ID("mu."+m.label), m.label, on) {
			a.mparams.Mute ^= m.bit
			// The mute set is part of the saved arrangement: flipping it on
			// a selected track writes through to the entry, so Save — and
			// the game regenerating from the document — honor it.
			k := a.listSel - a.musicStart()
			if k >= 0 && k < len(a.musics) {
				a.musics[k].Params.Mute = a.mparams.Mute
			}
			a.rebuild()
			a.play()
		}
	}
}

// sliderRows exposes every Params field. The labels are padded for the
// monospaced debug font, so the sliders line up as a column.
func (a *app) sliderRows() {
	a.slider("Freq", &a.params.Freq, 20, 3000, "%.0f")
	a.slider("Slide", &a.params.FreqSlide, -10000, 10000, "%.0f")
	a.slider("SlideStop", &a.params.FreqLimit, 0, 2000, "%.0f")
	a.slider("Attack", &a.params.Attack, 0, 1, "%.2f")
	a.slider("Sustain", &a.params.Sustain, 0, 2, "%.2f")
	a.slider("Punch", &a.params.Punch, 0, 1, "%.2f")
	a.slider("Decay", &a.params.Decay, 0, 2, "%.2f")
	a.slider("Duty", &a.params.Duty, 0.05, 0.95, "%.2f")
	a.slider("Vibrato", &a.params.Vibrato, 0, 200, "%.0f")
	a.slider("VibratoHz", &a.params.VibratoHz, 0, 30, "%.1f")
	a.slider("ArpMult", &a.params.ArpMult, 0, 4, "%.2f")
	a.slider("ArpDelay", &a.params.ArpDelay, 0, 0.5, "%.2f")
	a.slider("LowPass", &a.params.LowPass, 0, 8000, "%.0f")
	a.slider("Gain", &a.params.Gain, 0, 1, "%.2f")

	bits := float64(a.params.Bits)
	if a.slider("Bits", &bits, 0, 15, "%.0f") {
		a.params.Bits = int(math.Round(bits))
	}
}

// slider is one labeled row: a fixed-width name and value, then the track.
// The trailing spaces clear the knob, which overhangs the track start by its
// radius at the low end.
func (a *app) slider(name string, v *float64, lo, hi float64, format string) bool {
	a.gui.Label(fmt.Sprintf("%-10s %8s  ", name, fmt.Sprintf(format, *v)))
	a.gui.SameLine()
	changed := a.gui.Slider(ui.ID("sl."+name), v, lo, hi)
	if changed {
		a.edited = true
		a.dirty = true
	}
	return changed
}

// rebuild re-renders the samples and the waterfall grid from the current
// parameters — the effect or the music track, whichever is being edited.
func (a *app) rebuild() {
	if a.musicMode {
		a.samples = a.mparams.Render(rate)
	} else {
		a.samples = a.params.Render(rate)
	}
	a.spectrogram()
}

// play starts the current sound from the top, replacing any running playback
// so a burst of tweaks does not stack echoes.
func (a *app) play() {
	if a.player != nil {
		_ = a.player.Close()
	}
	if len(a.samples) == 0 {
		return
	}
	a.player = a.actx.NewPlayerFromBytes(stereo16(a.samples))
	a.player.Play()
}

// save asks for a path with the native dialog and writes the current sound.
func (a *app) save() {
	name := fmt.Sprintf("%s-%d.wav", a.preset, a.seed)
	var path string
	ebiten.RunOnMainThread(func() {
		path = filedialog.Save(filedialog.Options{
			Title:      "Save sound (.wav)",
			Filename:   name,
			Extensions: []string{"wav"},
		})
	})
	if path == "" {
		return // cancelled
	}
	if !strings.HasSuffix(path, ".wav") {
		path += ".wav"
	}
	// #nosec G304 -- user-chosen save path from the native dialog.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		a.err = err.Error()
		return
	}
	err = gion.WriteWAV(f, rate, a.samples)
	if err != nil {
		_ = f.Close() // the write error is the one worth reporting
		a.err = err.Error()
		return
	}
	err = f.Close()
	if err != nil {
		a.err = err.Error()
		return
	}
	a.err = ""
}

func (a *app) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{0x06, 0x08, 0x0c, 0xff})
	if a.view3D {
		a.drawWaterfall(screen)
	} else {
		a.drawWave(screen, vizX, vizY, vizW, vizH)
	}
	a.side.Render(screen)
	a.gui.Render(screen)
	a.debugShot(screen)
}

// debugShot is a development aid: with GION_SHOT set, frame 30 is dumped to
// that path and the app exits. With GION_SHOT_FRAMES=N it dumps a numbered
// sequence instead (one frame every other tick, for README gifs) and exits
// after the N-th.
var (
	shotFrame int
	shotCount int
)

func (a *app) debugShot(screen *ebiten.Image) {
	path := os.Getenv("GION_SHOT")
	if path == "" {
		return
	}
	frames := 1
	n, err := strconv.Atoi(os.Getenv("GION_SHOT_FRAMES"))
	if err == nil && n > 0 {
		frames = n
	}
	shotFrame++
	if shotFrame < 30 {
		return
	}
	if (shotFrame-30)%2 != 0 {
		return
	}
	out := path
	if frames > 1 {
		out = fmt.Sprintf("%s-%04d.png", strings.TrimSuffix(path, ".png"), shotCount)
	}
	img := image.NewRGBA(screen.Bounds())
	screen.ReadPixels(img.Pix)
	// #nosec G304 G703 -- debug-only, path from the developer's own env var.
	f, err := os.Create(out)
	if err != nil {
		panic(err)
	}
	err = png.Encode(f, img)
	if err != nil {
		panic(err)
	}
	err = f.Close()
	if err != nil {
		panic(err)
	}
	shotCount++
	if shotCount >= frames {
		os.Exit(0)
	}
}

// drawWave paints the rendered samples as a min/max column per pixel, the
// usual waveform thumbnail.
func (a *app) drawWave(screen *ebiten.Image, x, y, w, h float64) {
	st := a.gui.Style()
	vector.StrokeRect(screen, float32(x), float32(y), float32(w), float32(h), 1, st.Border, false)
	n := len(a.samples)
	if n == 0 {
		a.drawEmptyNotice(screen)
		return
	}
	mid := y + h/2
	scale := (h/2 - 2) / math.MaxInt16
	cols := int(w) - 2
	for i := range cols {
		lo, hi := int16(math.MaxInt16), int16(math.MinInt16)
		for j := i * n / cols; j < (i+1)*n/cols; j++ {
			s := a.samples[j]
			if s < lo {
				lo = s
			}
			if s > hi {
				hi = s
			}
		}
		if lo > hi {
			continue // empty bucket when there are fewer samples than columns
		}
		cx := float32(x + 1 + float64(i))
		y0 := float32(mid - float64(hi)*scale)
		y1 := float32(mid-float64(lo)*scale) + 1
		vector.StrokeLine(screen, cx, y0, cx, y1, 1, st.ButtonOn, false)
	}
}

func (a *app) Layout(int, int) (int, int) {
	return winW, winH
}

func main() {
	seed := newSeed()
	a := &app{
		actx:   audio.NewContext(rate),
		preset: "pickup",
		seed:   seed,
		params: gion.Pickup(seed),
		view3D: true,
		pitch:  0.55,
		// Explicit track defaults, so the music sliders show real values
		// instead of "0 = mood default" and documents save what is heard.
		mparams: music.Params{Seed: seed, Mood: music.Battle, Bars: 16, Root: 220, Gain: 0.6, LeadVol: 1, BassVol: 1, DrumVol: 1},
	}
	st := ui.DefaultStyle()
	st.FieldW = sideW
	a.side.SetStyle(st)

	st = ui.DefaultStyle()
	st.FieldW = 190 // keeps the knob's overhang clear of the view frame
	a.gui.SetStyle(st)
	a.rebuild()

	// A document on the command line opens at startup; this is also how the
	// OS hands over a double-clicked file once the extension is associated.
	if len(os.Args) > 1 {
		a.openPath(os.Args[1])
	}

	ebiten.SetWindowSize(winW, winH)
	ebiten.SetWindowTitle("gion")
	setWindowIcon()
	err := ebiten.RunGame(a)
	if err != nil {
		panic(err)
	}
}

// stereo16 packs mono samples into the 16-bit little-endian stereo stream the
// audio context expects, duplicating each sample into both channels.
func stereo16(samples []int16) []byte {
	b := make([]byte, len(samples)*4)
	for i, v := range samples {
		u := uint16(v) // #nosec G115 -- reinterpreting the sample's bits is the encoding
		lo, hi := byte(u&0xff), byte(u>>8)
		b[i*4], b[i*4+1], b[i*4+2], b[i*4+3] = lo, hi, lo, hi
	}
	return b
}
