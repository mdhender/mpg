// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package pipeline

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mdhender/mpg/internal/cells"
	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/elevation"
	"github.com/mdhender/mpg/internal/layout"
	"github.com/mdhender/mpg/internal/mesh"
	"github.com/mdhender/mpg/internal/render"
	"github.com/mdhender/mpg/internal/seed"
)

func TestStagesTable(t *testing.T) {
	want := []string{
		"config", "layout", "elevation", "mesh", "cells", "sea-level", "climate",
		"basins", "land-target", "rivers", "classify", "edges", "measures", "export",
	}
	stages := Stages()
	if len(stages) != len(want) {
		t.Fatalf("len(Stages()) = %d, want %d", len(stages), len(want))
	}
	for i, st := range stages {
		if st.Number != i+1 || st.Name != want[i] {
			t.Errorf("stage %d = %s, want %d %s", i, st, i+1, want[i])
		}
		// StageFile panics on a bad name.
		_ = render.StageFile(st.Number, st.Name, "")
	}
	if !stages[0].Implemented() {
		t.Error("config stage is not implemented")
	}
}

func TestLookup(t *testing.T) {
	stages := Stages()
	for s, want := range map[string]int{"config": 0, "1": 0, "elevation": 2, "3": 2, "export": 13, "14": 13} {
		if i, err := Lookup(stages, s); err != nil || i != want {
			t.Errorf("Lookup(%q) = %d, %v; want %d", s, i, err, want)
		}
	}
	for _, s := range []string{"bogus", "0", "15", "", "Config"} {
		_, err := Lookup(stages, s)
		if err == nil || !strings.Contains(err.Error(), "sea-level") {
			t.Errorf("Lookup(%q) error = %v, want one listing the stage names", s, err)
		}
	}
}

// fake returns a test registry: a, b and c implemented, d not, e
// implemented. Each implemented stage appends its name to *ran.
func fake(ran *[]string) []Stage {
	rec := func(c *Context) error {
		*ran = append(*ran, c.Stage().Name)
		return nil
	}
	return []Stage{
		{1, "a", rec}, {2, "b", rec}, {3, "c", rec}, {4, "d", nil}, {5, "e", rec},
	}
}

func newTestContext(t *testing.T, renders bool) *Context {
	t.Helper()
	cfg := config.Default()
	cfg.Seed = 42
	dir := t.TempDir()
	rd := ""
	if renders {
		rd = filepath.Join(dir, "renders")
	}
	c, err := NewContext(cfg, filepath.Join(dir, "out"), rd, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func names(stages []Stage) []string {
	var s []string
	for _, st := range stages {
		s = append(s, st.Name)
	}
	return s
}

func TestRunOrderAndStops(t *testing.T) {
	for _, tc := range []struct {
		stopAfter int
		want      []string
		notImpl   string
	}{
		{-1, []string{"a", "b", "c"}, "d"},
		{0, []string{"a"}, ""},
		{1, []string{"a", "b"}, ""},
		{2, []string{"a", "b", "c"}, ""},
		{3, []string{"a", "b", "c"}, "d"},
		{4, []string{"a", "b", "c"}, "d"},
	} {
		var ran []string
		c := newTestContext(t, false)
		res, err := Run(c, fake(&ran), tc.stopAfter)
		if err != nil {
			t.Fatalf("stopAfter %d: %v", tc.stopAfter, err)
		}
		if !slices.Equal(ran, tc.want) || !slices.Equal(names(res.Ran), tc.want) {
			t.Errorf("stopAfter %d: ran %q, Ran %q, want %q", tc.stopAfter, ran, names(res.Ran), tc.want)
		}
		got := ""
		if res.NotImplemented != nil {
			got = res.NotImplemented.Name
		}
		if got != tc.notImpl {
			t.Errorf("stopAfter %d: NotImplemented = %q, want %q", tc.stopAfter, got, tc.notImpl)
		}
		if fi, err := os.Stat(c.OutputDir); err != nil || !fi.IsDir() {
			t.Errorf("output dir not created: %v", err)
		}
	}
	if _, err := Run(newTestContext(t, false), fake(new([]string)), 5); err == nil {
		t.Error("Run with stopAfter past the end succeeded")
	}
}

func TestRunStageError(t *testing.T) {
	boom := errors.New("boom")
	var ran []string
	stages := fake(&ran)
	stages[1].Run = func(*Context) error { return boom }
	res, err := Run(newTestContext(t, false), stages, -1)
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "2 b") {
		t.Errorf("err = %v, want boom naming stage 2 b", err)
	}
	if !slices.Equal(names(res.Ran), []string{"a"}) {
		t.Errorf("Ran = %q, want [a]", names(res.Ran))
	}
}

func TestRender(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	draw := func(c *Context) error {
		if err := c.Render("", img); err != nil {
			return err
		}
		return c.Render("area", img)
	}
	stages := []Stage{{Number: 4, Name: "mesh", Run: draw}}

	c := newTestContext(t, true)
	if _, err := Run(c, stages, -1); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"04-mesh.png", "04-mesh-area.png"} {
		b, err := os.ReadFile(filepath.Join(c.RendersDir, name))
		if err != nil {
			t.Fatal(err)
		}
		meta, err := render.ReadMeta(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		if want := strings.TrimSuffix(name, ".png"); meta.Stage != want || meta.ConfigHash != c.ConfigHash {
			t.Errorf("%s meta = %+v, want stage %q hash %q", name, meta, want, c.ConfigHash)
		}
	}
	entries, _ := os.ReadDir(c.OutputDir)
	if len(entries) != 0 {
		t.Errorf("render stage wrote %d files to the output dir", len(entries))
	}

	// Without a renders dir, Render writes nothing.
	c = newTestContext(t, false)
	if _, err := Run(c, stages, -1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(c.OutputDir), "renders")); !os.IsNotExist(err) {
		t.Errorf("renders dir exists without RendersDir: %v", err)
	}
}

func TestConfigStage(t *testing.T) {
	c := newTestContext(t, false)
	res, err := Run(c, Stages(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.NotImplemented != nil || !slices.Equal(names(res.Ran), []string{"config"}) {
		t.Errorf("result = %+v", res)
	}
	got, err := os.ReadFile(filepath.Join(c.OutputDir, ConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../config/testdata/example.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("config.json differs from the seed-42 example:\n%s", got)
	}

	// The full registry stops at the first unimplemented stage.
	res, err = Run(newTestContext(t, false), Stages(), -1)
	if err != nil || res.NotImplemented == nil || res.NotImplemented.Name != "climate" {
		t.Errorf("full run = %+v, %v; want a stop at climate", res, err)
	}
}

func TestNewContext(t *testing.T) {
	cfg := config.Default()
	cfg.World.Aspect = "squarish"
	if _, err := NewContext(cfg, t.TempDir(), "", nil); err == nil {
		t.Error("NewContext accepted an invalid config")
	}
	if _, err := NewContext(config.Default(), "", "", nil); err == nil {
		t.Error("NewContext accepted an empty output dir")
	}
	c := newTestContext(t, false)
	if h, _ := c.Config.Hash(); h != c.ConfigHash || h == "" {
		t.Errorf("ConfigHash = %q, want %q", c.ConfigHash, h)
	}
	s1, s2 := c.Seed("elevation", "1")
	w1, w2 := seed.Derive(42, "elevation", "1")
	if s1 != w1 || s2 != w2 {
		t.Error("Seed does not match seed.Derive")
	}
	if c.Rand("warp", "1").Uint64() != seed.Rand(42, "warp", "1").Uint64() {
		t.Error("Rand does not match seed.Rand")
	}
}

func TestRenderSink(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	draw := func(c *Context) error {
		if err := c.Render("", img); err != nil {
			return err
		}
		return c.Render("area", img)
	}
	stages := []Stage{{Number: 4, Name: "mesh", Run: draw}}
	for _, renders := range []bool{false, true} {
		c := newTestContext(t, renders)
		var got []string
		c.Sink = func(st Stage, variant string, m image.Image) error {
			if m != img {
				t.Error("sink got a different image")
			}
			got = append(got, st.String()+"/"+variant)
			return nil
		}
		if _, err := Run(c, stages, -1); err != nil {
			t.Fatal(err)
		}
		if want := []string{"4 mesh/", "4 mesh/area"}; !slices.Equal(got, want) {
			t.Errorf("renders %v: sink saw %q, want %q", renders, got, want)
		}
		if renders {
			if _, err := os.Stat(filepath.Join(c.RendersDir, "04-mesh-area.png")); err != nil {
				t.Errorf("sink stopped the file write: %v", err)
			}
		}
	}

	// A sink error fails the stage and skips the write.
	boom := errors.New("boom")
	c := newTestContext(t, true)
	c.Sink = func(Stage, string, image.Image) error { return boom }
	if _, err := Run(c, stages, -1); !errors.Is(err, boom) {
		t.Errorf("err = %v, want boom", err)
	}
	if _, err := os.Stat(filepath.Join(c.RendersDir, "04-mesh.png")); !os.IsNotExist(err) {
		t.Errorf("render written after a sink error: %v", err)
	}
}

func TestLayoutStage(t *testing.T) {
	c := newTestContext(t, true)
	var variants []string
	c.Sink = func(st Stage, variant string, img image.Image) error {
		variants = append(variants, st.String()+"/"+variant)
		return nil
	}
	res, err := Run(c, Stages(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(names(res.Ran), []string{"config", "layout"}) {
		t.Errorf("ran %q", names(res.Ran))
	}
	l, bias := c.Products.Layout, c.Products.Bias
	if l == nil || bias == nil {
		t.Fatal("layout stage left its products empty")
	}
	if l.Preset != "continents" || l.Masses < 3 || l.Masses > 5 {
		t.Errorf("layout = %s with %d masses", l.Preset, l.Masses)
	}
	want, err := layout.New(c.Config)
	if err != nil {
		t.Fatal(err)
	}
	f, err := want.Bias(c.Config.Raster.SpacingKm)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.Values(), bias.Values()) || !slices.Equal(want.Attractors, l.Attractors) {
		t.Error("stage products differ from layout.New")
	}
	if !slices.Equal(variants, []string{"2 layout/"}) {
		t.Errorf("renders %q, want the layout render", variants)
	}
	if _, err := os.Stat(filepath.Join(c.RendersDir, "02-layout.png")); err != nil {
		t.Error(err)
	}
}

func TestElevationStage(t *testing.T) {
	c := newTestContext(t, true)
	var variants []string
	c.Sink = func(st Stage, variant string, img image.Image) error {
		variants = append(variants, st.String()+"/"+variant)
		return nil
	}
	res, err := Run(c, Stages(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(names(res.Ran), []string{"config", "layout", "elevation"}) {
		t.Errorf("ran %q", names(res.Ran))
	}
	f := c.Products.Elevation
	if f == nil {
		t.Fatal("elevation stage left its product empty")
	}
	e, err := elevation.New(c.Config, c.Products.Bias)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := e.Field()
	if !slices.Equal(f.Values(), want.Values()) {
		t.Error("stage product differs from elevation.New")
	}
	if hs := c.Products.Hotspots; len(hs) == 0 || !slices.Equal(hs, e.Hotspots()) {
		t.Errorf("stage hotspots %+v, want elevation.New's (seed 42 draws some)", hs)
	}
	if !slices.Equal(variants, []string{"2 layout/", "3 elevation/"}) {
		t.Errorf("renders %q, want the layout and elevation renders", variants)
	}
	if _, err := os.Stat(filepath.Join(c.RendersDir, "03-elevation.png")); err != nil {
		t.Error(err)
	}
}

func TestMeshStage(t *testing.T) {
	c := newTestContext(t, true)
	var variants []string
	c.Sink = func(st Stage, variant string, img image.Image) error {
		variants = append(variants, st.String()+"/"+variant)
		return nil
	}
	res, err := Run(c, Stages(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(names(res.Ran), []string{"config", "layout", "elevation", "mesh"}) {
		t.Errorf("ran %q", names(res.Ran))
	}
	m := c.Products.Mesh
	if m == nil {
		t.Fatal("mesh stage left its product empty")
	}
	want, err := mesh.New(c.Config)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := m.AppendBinary(nil)
	b, _ := want.AppendBinary(nil)
	if !bytes.Equal(a, b) {
		t.Error("stage product differs from mesh.New")
	}
	if !slices.Equal(variants, []string{"2 layout/", "3 elevation/", "4 mesh/", "4 mesh/area", "4 mesh/short"}) {
		t.Errorf("renders %q, want the layout, elevation, mesh, mesh area and mesh short-edge renders", variants)
	}
	if _, err := os.Stat(filepath.Join(c.RendersDir, "04-mesh.png")); err != nil {
		t.Error(err)
	}
}

// TestCellsStage runs the pipeline through the cell statistics stage and
// checks its product against cells.Compute and its renders.
func TestCellsStage(t *testing.T) {
	c := newTestContext(t, true)
	var variants []string
	c.Sink = func(st Stage, variant string, img image.Image) error {
		variants = append(variants, st.String()+"/"+variant)
		return nil
	}
	last, err := Lookup(Stages(), "cells")
	if err != nil {
		t.Fatal(err)
	}
	res, err := Run(c, Stages(), last)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(names(res.Ran), []string{"config", "layout", "elevation", "mesh", "cells"}) {
		t.Errorf("ran %q", names(res.Ran))
	}
	s := c.Products.Cells
	if s == nil {
		t.Fatal("cells stage left its product empty")
	}
	want, err := cells.Compute(c.Products.Elevation, c.Products.Mesh)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := s.AppendBinary(nil)
	b, _ := want.AppendBinary(nil)
	if !bytes.Equal(a, b) {
		t.Error("stage product differs from cells.Compute")
	}
	if got := variants[len(variants)-2:]; !slices.Equal(got, []string{"5 cells/", "5 cells/relief"}) {
		t.Errorf("renders %q, want the cell altitude and relief renders last", variants)
	}
	for _, name := range []string{"05-cells.png", "05-cells-relief.png"} {
		if _, err := os.Stat(filepath.Join(c.RendersDir, name)); err != nil {
			t.Error(err)
		}
	}
}

// TestSeaLevelStage runs the pipeline through the sea level stage and
// checks its product against cells.Search, its render, and its log.
func TestSeaLevelStage(t *testing.T) {
	c := newTestContext(t, true)
	var log bytes.Buffer
	c.Log = &log
	var variants []string
	c.Sink = func(st Stage, variant string, img image.Image) error {
		variants = append(variants, st.String()+"/"+variant)
		return nil
	}
	last, err := Lookup(Stages(), "sea-level")
	if err != nil {
		t.Fatal(err)
	}
	res, err := Run(c, Stages(), last)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(names(res.Ran), []string{"config", "layout", "elevation", "mesh", "cells", "sea-level"}) {
		t.Errorf("ran %q", names(res.Ran))
	}
	sl := c.Products.SeaLevel
	if sl == nil {
		t.Fatal("sea level stage left its product empty")
	}
	want, err := cells.Search(c.Products.Mesh, c.Products.Cells.Altitude, c.Config.World.LandCells)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := sl.AppendBinary(nil)
	b, _ := want.AppendBinary(nil)
	if !bytes.Equal(a, b) {
		t.Error("stage product differs from cells.Search")
	}
	if !sl.Met || sl.Target != 10_000 {
		t.Errorf("seed 42: %d land of %d, reason %s", sl.LandCells, sl.Target, sl.Reason)
	}
	if got := variants[len(variants)-1]; got != "6 sea-level/" {
		t.Errorf("renders %q, want the sea level render last", variants)
	}
	if _, err := os.Stat(filepath.Join(c.RendersDir, "06-sea-level.png")); err != nil {
		t.Error(err)
	}
	for _, s := range []string{"sea-level: target 10000 land cells ± 100", string(sl.Reason), "dry basin"} {
		if !strings.Contains(log.String(), s) {
			t.Errorf("log lacks %q:\n%s", s, log.String())
		}
	}
}

// TestSeaLevelMeetsTarget runs the sea-level search on default worlds
// (smaller ones with -short) for seeds 1 to 8 at the cinematic and square
// aspects, and archipelago and pangaea at cinematic: every one meets the
// land target within 1% of N.
func TestSeaLevelMeetsTarget(t *testing.T) {
	type row struct {
		aspect, preset string
	}
	rows := []row{{"cinematic", "continents"}, {"square", "continents"}, {"cinematic", "archipelago"}, {"cinematic", "pangaea"}}
	land := 0 // default
	if testing.Short() {
		land = 2_000
	}
	for _, r := range rows {
		for seed := range uint64(8) {
			t.Run(fmt.Sprintf("%s-%s-%d", r.aspect, r.preset, seed+1), func(t *testing.T) {
				t.Parallel()
				cfg := config.Default()
				cfg.Seed = config.Seed(seed + 1)
				cfg.World.Aspect = r.aspect
				cfg.Layout.Preset = r.preset
				if land > 0 {
					cfg.World.LandCells = land
				}
				c, err := NewContext(cfg, t.TempDir(), "", nil)
				if err != nil {
					t.Fatal(err)
				}
				last, _ := Lookup(Stages(), "sea-level")
				if _, err := Run(c, Stages(), last); err != nil {
					t.Fatal(err)
				}
				sl := c.Products.SeaLevel
				if !sl.Met || 100*max(sl.Deviation(), -sl.Deviation()) > sl.Target {
					t.Errorf("land %d of %d (%+.2f%%), reason %s, %d probes", sl.LandCells, sl.Target, sl.DeviationPercent(), sl.Reason, len(sl.Trace))
				}
				t.Logf("land %d of %d (%+.2f%%), %d dry basin cells, level %.2f m, %s in %d probes",
					sl.LandCells, sl.Target, sl.DeviationPercent(), sl.BasinCells, sl.Level, sl.Reason, len(sl.Trace))
			})
		}
	}
}

// TestMeshStageCheckFails checks that a mesh failing the mesh checks stops
// the run with an error naming the check, still renders, and leaves no
// product for the later stages: area bounds of exactly A fail every cell.
func TestMeshStageCheckFails(t *testing.T) {
	cfg := config.Default()
	cfg.Seed = 42
	cfg.World.LandCells = 2_000
	cfg.Mesh.AreaMin, cfg.Mesh.AreaMax = 1, 1
	dir := t.TempDir()
	c, err := NewContext(cfg, filepath.Join(dir, "out"), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	var variants []string
	c.Sink = func(st Stage, variant string, img image.Image) error {
		variants = append(variants, st.String()+"/"+variant)
		return nil
	}
	_, err = Run(c, Stages(), 3)
	if err == nil || !strings.Contains(err.Error(), "mesh check area failed") {
		t.Fatalf("Run: %v, want the area check's failure", err)
	}
	if c.Products.Mesh != nil {
		t.Error("a mesh that failed its checks was left as the stage product")
	}
	if !slices.Contains(variants, "4 mesh/short") {
		t.Errorf("renders %q, want the mesh renders before the failure", variants)
	}
}
