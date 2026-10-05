// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mdhender/mpg/internal/playermap"
	"github.com/mdhender/mpg/internal/render"
	"github.com/mdhender/mpg/world"
)

// renderStage runs "mpg render-stage args..." and returns the exit code,
// stdout, and stderr.
func renderStage(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(append([]string{"render-stage"}, args...), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func readPNG(t *testing.T, path string) (image.Image, render.Meta) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	meta, err := render.ReadMeta(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return img, meta
}

func TestRenderStageUsage(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "p.png")
	for _, tc := range []struct {
		args []string
		msg  string
	}{
		{[]string{"--output", out}, "want exactly one PATH"},
		{[]string{"--output", out, "a", "b"}, "want exactly one PATH"},
		{[]string{dir}, "--output is required"},
		{[]string{"--output", out, "--scale", "2", "--width", "100", dir}, "exclusive"},
		{[]string{"--output", out, "--scale", "0", dir}, "--scale"},
		{[]string{"--output", out, "--scale", "-1", dir}, "--scale"},
		{[]string{"--output", out, "--width", "0", dir}, "--width"},
		{[]string{"--output", out, "--window", "1,2,3", dir}, "--window"},
		{[]string{"--output", out, "--window", "1,2,3,x", dir}, "--window"},
		{[]string{"--output", out, "--window", "1,2,0,4", dir}, "--window"},
		{[]string{"--output", out, "--window", "1,2,NaN,4", dir}, "--window"},
		{[]string{"--no-such-flag", dir}, "flag provided but not defined"},
	} {
		code, _, stderr := renderStage(t, tc.args...)
		if code != 2 || !strings.Contains(stderr, tc.msg) {
			t.Errorf("render-stage %q: exit %d, stderr %q; want exit 2 mentioning %q", tc.args, code, stderr, tc.msg)
		}
	}
	if code, _, stderr := renderStage(t, "--output", out, filepath.Join(dir, "missing")); code != 1 || !strings.Contains(stderr, "missing") {
		t.Errorf("missing world: exit %d, stderr %q", code, stderr)
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("a failed render wrote its output")
	}
}

func TestParseWindow(t *testing.T) {
	got, err := parseWindow(" -12.5, 0,300 ,1e2")
	if err != nil || !slices.Equal(got, []float64{-12.5, 0, 300, 100}) {
		t.Errorf("parseWindow = %v, %v", got, err)
	}
	for _, s := range []string{"", "1,2,3", "1,2,3,4,5", "a,2,3,4", "1,2,-3,4", "1,2,3,0", "1,Inf,3,4"} {
		if v, err := parseWindow(s); err == nil {
			t.Errorf("parseWindow(%q) = %v, want an error", s, v)
		}
	}
}

// TestRenderStage draws a generated world whole and through windows,
// including one across the seam, and checks each window is the crop of the
// whole map, the provenance, and that the world's directory and file both
// work.
func TestRenderStage(t *testing.T) {
	wdir := smallWorld(t, 5)
	w, err := readWorld(filepath.Join(wdir, world.File))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	fullPath := filepath.Join(dir, "sub", "full.png")
	code, stdout, stderr := renderStage(t, "--scale", "1.5", "--output", fullPath, wdir)
	if code != 0 {
		t.Fatalf("exit %d; stderr %q", code, stderr)
	}
	l, err := playermap.NewLattice(w, 1.5)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"render  " + fullPath, fmt.Sprintf("map     %d x %d px at 1.5 px/km", l.Width, l.Height), "window  full", "pixels  "} {
		if !strings.Contains(stdout, s) {
			t.Errorf("stdout lacks %q:\n%s", s, stdout)
		}
	}
	fullImg, meta := readPNG(t, fullPath)
	if meta.Stage != "player" || meta.ConfigHash != w.Meta.ConfigHash {
		t.Errorf("meta = %+v", meta)
	}
	extra := map[string]string{}
	for _, x := range meta.Extra {
		extra[x.Key] = x.Value
	}
	if extra["mpg:seed"] != "5" || extra["mpg:scale"] != "1.5" || extra["mpg:window-km"] != "full" ||
		extra["mpg:map-px"] != fmt.Sprintf("%d,%d", l.Width, l.Height) {
		t.Errorf("extra = %v", extra)
	}
	full := image.NewRGBA(fullImg.Bounds())
	for y := range full.Rect.Dy() {
		for x := range full.Rect.Dx() {
			full.Set(x, y, fullImg.At(x, y))
		}
	}

	W, H := w.Meta.WidthKm, w.Meta.HeightKm
	for k, win := range [][4]float64{{W - 100, 0.3 * H, 300, 0.4 * H}, {-120, 0, 250, 150}, {W / 4, H / 3, 200, 100}} {
		path := filepath.Join(dir, fmt.Sprintf("w%d.png", k))
		arg := fmt.Sprintf("%v,%v,%v,%v", win[0], win[1], win[2], win[3])
		code, stdout, stderr := renderStage(t, "--scale", "1.5", "--window", arg, "--output", path, filepath.Join(wdir, world.File))
		if code != 0 {
			t.Fatalf("window %s: exit %d; stderr %q", arg, code, stderr)
		}
		f, err := l.Window(win[0], win[1], win[2], win[3])
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stdout, fmt.Sprintf("frame %d,%d,%d,%d px", f.X, f.Y, f.Width, f.Height)) {
			t.Errorf("window %s: stdout %q", arg, stdout)
		}
		img, meta := readPNG(t, path)
		crop, err := playermap.Crop(full, l, f)
		if err != nil {
			t.Fatal(err)
		}
		if render.PixelHash(img) != render.PixelHash(crop) {
			t.Errorf("window %s: differs from the crop of the full render", arg)
		}
		if meta.Stage != "player" || !slices.Contains(meta.Extra, render.Text{Key: "mpg:window-km", Value: arg}) {
			t.Errorf("window %s: meta %+v", arg, meta)
		}
	}

	// --width sets the full map's width.
	path := filepath.Join(dir, "width.png")
	if code, _, stderr := renderStage(t, "--width", "200", "--output", path, wdir); code != 0 {
		t.Fatalf("--width: exit %d; stderr %q", code, stderr)
	}
	if img, _ := readPNG(t, path); img.Bounds().Dx() != 200 {
		t.Errorf("--width 200 gave %v", img.Bounds())
	}
	// A window off the map is an error.
	if code, _, stderr := renderStage(t, "--window", fmt.Sprintf("0,%v,10,10", H), "--output", path, wdir); code != 1 || !strings.Contains(stderr, "does not fit") {
		t.Errorf("window off the map: exit %d, stderr %q", code, stderr)
	}
}

// TestRenderStageInvalidWorld checks that a world that fails validation is
// not drawn.
func TestRenderStageInvalidWorld(t *testing.T) {
	wdir := smallWorld(t, 6)
	w, err := readWorld(filepath.Join(wdir, world.File))
	if err != nil {
		t.Fatal(err)
	}
	w.Cells[0].Landform = "lava"
	b, err := w.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(t.TempDir(), world.File)
	if err := os.WriteFile(bad, b, 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := renderStage(t, "--output", filepath.Join(t.TempDir(), "p.png"), bad); code != 1 || !strings.Contains(stderr, "invalid") {
		t.Errorf("invalid world: exit %d, stderr %q", code, stderr)
	}
}

// TestSweepExport checks that sweep draws the export column, the player
// map.
func TestSweepExport(t *testing.T) {
	out := filepath.Join(t.TempDir(), "sheet.png")
	code, stdout, stderr := sweep(t, "--seeds", "1,2", "--stage", "classify,export", "--land-cells", "600", "--tile", "64", "--output", out)
	if code != 0 {
		t.Fatalf("exit %d; stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "stages  classify, export") || !strings.Contains(stderr, "export: world.json: ") {
		t.Errorf("stdout %q, stderr %q", stdout, stderr)
	}
	if _, meta := readPNG(t, out); meta.Stage != "sweep" {
		t.Errorf("meta = %+v", meta)
	}
}
