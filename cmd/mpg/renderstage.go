// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mdhender/mpg/internal/playermap"
	"github.com/mdhender/mpg/internal/render"
	"github.com/mdhender/mpg/world"
)

// runRenderStage draws the player-style map of a world.json, whole or a
// window of it, from the file alone (package playermap). PATH is the file
// or the directory holding it. The window is x,y,w,h in km from the
// northwest corner; x is taken modulo W, so a window may cross the seam.
// The PNG records the stage ("player"), the world's config hash, its seed,
// the scale, and the frame. Exit codes: 0 on success, 1 when the world
// cannot be read, is invalid, or does not fit the window, 2 on a usage
// error.
func runRenderStage(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("render-stage", stderr)
	windowFlag := fs.String("window", "", "draw only the window `x,y,w,h` in km (x modulo W; default the whole map)")
	scale := fs.Float64("scale", 0, "`pixels` per km (default 1)")
	width := fs.Int("width", 0, "full map width in `pixels`, setting the scale (instead of --scale)")
	output := fs.String("output", "", "write the PNG to `file` (required)")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: mpg render-stage [--window x,y,w,h] [--scale px/km | --width px] --output FILE PATH")
		fmt.Fprintln(stderr, "PATH is a world.json file or a directory holding one.")
		fs.PrintDefaults()
	}
	if code, stop := parse(fs, args); stop {
		return code
	}
	usageErr := func(format string, args ...any) int {
		fmt.Fprintf(stderr, "mpg render-stage: "+format+"\n", args...)
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "mpg render-stage: want exactly one PATH")
		fs.Usage()
		return 2
	}
	if *output == "" {
		return usageErr("--output is required")
	}
	if isSet(fs, "scale") && isSet(fs, "width") {
		return usageErr("--scale and --width are exclusive")
	}
	if isSet(fs, "scale") && (!(*scale > 0) || math.IsInf(*scale, 0)) {
		return usageErr("--scale %v is not positive", *scale)
	}
	if isSet(fs, "width") && (*width < 1 || *width > playermap.MaxMapPixels) {
		return usageErr("--width %d outside [1, %d]", *width, playermap.MaxMapPixels)
	}
	var win []float64
	if isSet(fs, "window") {
		var err error
		if win, err = parseWindow(*windowFlag); err != nil {
			return usageErr("--window: %v", err)
		}
	}

	fail := func(format string, args ...any) int {
		fmt.Fprintf(stderr, "mpg render-stage: "+format+"\n", args...)
		return 1
	}
	path := fs.Arg(0)
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		path = filepath.Join(path, world.File)
	}
	w, err := readWorld(path)
	if err != nil {
		return fail("%v", err)
	}
	if err := world.Validate(w); err != nil {
		return fail("%s: invalid world\n%v", path, err)
	}
	s := playermap.DefaultScale
	switch {
	case isSet(fs, "scale"):
		s = *scale
	case isSet(fs, "width"):
		s = float64(*width) / w.Meta.WidthKm
	}
	l, err := playermap.NewLattice(w, s)
	if err != nil {
		return fail("%v", err)
	}
	f, windowText := l.Full(), "full"
	if win != nil {
		if f, err = l.Window(win[0], win[1], win[2], win[3]); err != nil {
			return fail("%v", err)
		}
		windowText = *windowFlag
	}
	start := time.Now()
	img, err := playermap.Render(w, l, f)
	if err != nil {
		return fail("%v", err)
	}
	elapsed := time.Since(start)
	frameText := fmt.Sprintf("%d,%d,%d,%d", f.X, f.Y, f.Width, f.Height)
	meta := render.Meta{
		Stage:      "player",
		ConfigHash: w.Meta.ConfigHash,
		Extra: []render.Text{
			{Key: "mpg:seed", Value: w.Meta.Seed},
			{Key: "mpg:scale", Value: strconv.FormatFloat(s, 'g', -1, 64)},
			{Key: "mpg:map-px", Value: fmt.Sprintf("%d,%d", l.Width, l.Height)},
			{Key: "mpg:window-km", Value: windowText},
			{Key: "mpg:frame-px", Value: frameText},
		},
	}
	if dir := filepath.Dir(*output); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fail("%v", err)
		}
	}
	if err := render.WritePNGFile(*output, img, meta); err != nil {
		return fail("%v", err)
	}
	fmt.Fprintf(stdout, "render  %s (%d x %d px)\n", *output, f.Width, f.Height)
	fmt.Fprintf(stdout, "world   %s, config %s\n", path, w.Meta.ConfigHash)
	fmt.Fprintf(stdout, "map     %d x %d px at %g px/km\n", l.Width, l.Height, s)
	fmt.Fprintf(stdout, "window  %s (frame %s px)\n", windowText, frameText)
	fmt.Fprintf(stdout, "pixels  %s\n", render.PixelHash(img))
	fmt.Fprintf(stderr, "render-stage: drawn in %.2fs\n", elapsed.Seconds())
	return 0
}

// parseWindow parses "x,y,w,h": four finite decimal numbers in km.
func parseWindow(s string) ([]float64, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return nil, fmt.Errorf("%q: want x,y,w,h in km", s)
	}
	v := make([]float64, 4)
	for k, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, fmt.Errorf("%q: %q is not a finite number", s, p)
		}
		v[k] = f
	}
	if !(v[2] > 0) || !(v[3] > 0) {
		return nil, fmt.Errorf("%q: width and height must be positive", s)
	}
	return v, nil
}
