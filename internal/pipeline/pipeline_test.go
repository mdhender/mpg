// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package pipeline

import (
	"bytes"
	"errors"
	"image"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mdhender/mpg/internal/config"
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
	if err != nil || res.NotImplemented == nil || res.NotImplemented.Name != "layout" {
		t.Errorf("full run = %+v, %v; want a stop at layout", res, err)
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
