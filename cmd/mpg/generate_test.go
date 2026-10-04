// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/mdhender/mpg/internal/config"
)

// generate runs "mpg generate args..." and returns the exit code, stdout,
// and stderr.
func generate(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(append([]string{"generate"}, args...), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// readConfig reads and decodes dir/config.json, returning it and its bytes.
func readConfig(t *testing.T, dir string) (config.Config, []byte) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return cfg, b
}

func TestGenerateSeed42(t *testing.T) {
	want, err := os.ReadFile("../../internal/config/testdata/example.json")
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "new", "dir")
	code, stdout, stderr := generate(t, "--seed", "42", "--output", out)
	if code != 0 {
		t.Fatalf("exit %d; stderr %q", code, stderr)
	}
	_, got := readConfig(t, out)
	if !bytes.Equal(got, want) {
		t.Errorf("config.json differs from testdata/example.json:\n%s", got)
	}
	if !strings.Contains(stderr, "stopped: stage 5 cells not implemented yet") {
		t.Errorf("stderr = %q, want a not-implemented stop", stderr)
	}
	cfg, _ := readConfig(t, out)
	hash, _ := cfg.Hash()
	for _, s := range []string{hash, "stages  config, layout, elevation, mesh\n", "33333 playable"} {
		if !strings.Contains(stdout, s) {
			t.Errorf("stdout = %q, want it to contain %q", stdout, s)
		}
	}

	// Running again overwrites config.json with the same bytes.
	if code, _, stderr := generate(t, "--seed", "42", "--output", out); code != 0 {
		t.Fatalf("second run exit %d; stderr %q", code, stderr)
	}
	if _, again := readConfig(t, out); !bytes.Equal(again, want) {
		t.Error("second run changed config.json")
	}
}

func TestGenerateConfigRoundTrip(t *testing.T) {
	in := "../../internal/config/testdata/example.json"
	want, err := os.ReadFile(in)
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if code, _, stderr := generate(t, "--config", in, "--output", out); code != 0 {
		t.Fatalf("exit %d; stderr %q", code, stderr)
	}
	if _, got := readConfig(t, out); !bytes.Equal(got, want) {
		t.Errorf("round trip changed config.json:\n%s", got)
	}
}

func TestGenerateOverrides(t *testing.T) {
	in := "../../internal/config/testdata/example.json"
	f, err := os.Open(in)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	base, err := config.Decode(f)
	if err != nil {
		t.Fatal(err)
	}

	// Seed alone: derived values stay valid and unchanged.
	out := t.TempDir()
	if code, _, stderr := generate(t, "--config", in, "--seed", "18446744073709551615", "--output", out); code != 0 {
		t.Fatalf("seed override: exit %d; stderr %q", code, stderr)
	}
	cfg, _ := readConfig(t, out)
	if cfg.Seed != config.Seed(18446744073709551615) {
		t.Errorf("seed = %d", cfg.Seed)
	}
	cfg.Seed = base.Seed
	if !reflect.DeepEqual(cfg, base) {
		t.Errorf("seed override changed other fields: %+v", cfg)
	}

	// Land cells and aspect change the sizes.
	out = t.TempDir()
	if code, _, stderr := generate(t, "--config", in, "--land-cells", "20000", "--aspect", "16:9", "--output", out); code != 0 {
		t.Fatalf("size override: exit %d; stderr %q", code, stderr)
	}
	cfg, _ = readConfig(t, out)
	if cfg.World.LandCells != 20000 || cfg.World.Aspect != "16:9" || cfg.World.PlayableCells != 66667 {
		t.Errorf("world = %+v", cfg.World)
	}
	if cfg.World.WidthKm == base.World.WidthKm || cfg.World.HeightKm == base.World.HeightKm {
		t.Errorf("sizes unchanged: %+v", cfg.World)
	}
	if cfg.Seed != base.Seed {
		t.Errorf("seed = %d, want the file's %d", cfg.Seed, base.Seed)
	}

	// Without --config, overrides apply to the defaults.
	out = t.TempDir()
	if code, _, stderr := generate(t, "--aspect", "square", "--output", out); code != 0 {
		t.Fatalf("default override: exit %d; stderr %q", code, stderr)
	}
	if cfg, _ = readConfig(t, out); cfg.World.AspectRatio != 1 || cfg.Seed != 0 {
		t.Errorf("world = %+v, seed %d", cfg.World, cfg.Seed)
	}

	// --preset picks the layout preset.
	out = t.TempDir()
	if code, _, stderr := generate(t, "--preset", "islands", "--stop-after", "config", "--output", out); code != 0 {
		t.Fatalf("preset override: exit %d; stderr %q", code, stderr)
	}
	if cfg, _ = readConfig(t, out); cfg.Layout.Preset != "islands" {
		t.Errorf("preset = %q, want islands", cfg.Layout.Preset)
	}
	if code, _, stderr := generate(t, "--preset", "isles", "--output", t.TempDir()); code != 1 || !strings.Contains(stderr, "layout.preset") {
		t.Errorf("bad preset: exit %d, stderr %q", code, stderr)
	}
}

func TestGenerateBadConfig(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(in, []byte(`{"schema": 1, "world": {"land_cell": 5000}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	code, _, stderr := generate(t, "--config", in, "--output", out)
	if code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	if !strings.Contains(stderr, "land_cell") || !strings.Contains(stderr, "land_cells") {
		t.Errorf("stderr = %q, want the field and a suggestion", stderr)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("output dir created for a bad config: %v", err)
	}

	// An invalid override fails as a config error too.
	if code, _, stderr := generate(t, "--aspect", "squarish", "--output", out); code != 1 || !strings.Contains(stderr, "world.aspect") {
		t.Errorf("bad aspect: exit %d, stderr %q", code, stderr)
	}
}

func TestGenerateStopAfter(t *testing.T) {
	for _, stage := range []string{"config", "1"} {
		out := t.TempDir()
		code, stdout, stderr := generate(t, "--seed", "42", "--stop-after", stage, "--output", out)
		if code != 0 {
			t.Fatalf("--stop-after %s: exit %d; stderr %q", stage, code, stderr)
		}
		if !strings.Contains(stderr, "stopped after stage 1 config") || strings.Contains(stderr, "not implemented") {
			t.Errorf("--stop-after %s: stderr = %q", stage, stderr)
		}
		if !strings.Contains(stdout, "stages  config\n") {
			t.Errorf("--stop-after %s: stdout = %q", stage, stdout)
		}
		readConfig(t, out)
	}

	// Stopping after a later stage still stops at the first unimplemented one.
	code, _, stderr := generate(t, "--stop-after", "sea-level", "--output", t.TempDir())
	if code != 0 || !strings.Contains(stderr, "stage 5 cells not implemented yet") {
		t.Errorf("--stop-after sea-level: exit %d, stderr %q", code, stderr)
	}
}

func TestGenerateUsageErrors(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out")
	for _, args := range [][]string{
		{"--stop-after", "bogus", "--output", out},
		{},
		{"--seed", "-1", "--output", out},
		{"--seed", "0x10", "--output", out},
		{"--output", out, "extra"},
	} {
		code, _, stderr := generate(t, args...)
		if code != 2 {
			t.Errorf("generate %q: exit %d, want 2; stderr %q", args, code, stderr)
		}
	}
	if _, _, stderr := generate(t, "--stop-after", "bogus", "--output", out); !strings.Contains(stderr, "sea-level") {
		t.Errorf("bogus stage: stderr %q does not list the stages", stderr)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("output dir created on a usage error: %v", err)
	}
}

func TestGenerateRendersDir(t *testing.T) {
	dir := t.TempDir()
	renders := filepath.Join(dir, "renders")
	if code, _, stderr := generate(t, "--renders", renders, "--output", filepath.Join(dir, "out")); code != 0 {
		t.Fatalf("exit %d; stderr %q", code, stderr)
	}
	entries, err := os.ReadDir(renders)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if !slices.Equal(names, []string{"02-layout.png", "03-elevation.png", "04-mesh-area.png", "04-mesh.png"}) {
		t.Errorf("renders = %q, want the layout, elevation, mesh area and mesh renders", names)
	}
}
