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
	"github.com/mdhender/mpg/internal/measure"
	"github.com/mdhender/mpg/world"
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
	if !strings.Contains(stderr, "elevation: pre-pass (stages 3–8, no lake allowance)") {
		t.Errorf("stderr = %q, want the elevation pre-pass logged", stderr)
	}
	if strings.Contains(stderr, "stopped") {
		t.Errorf("stderr = %q, want a run to the end", stderr)
	}
	if strings.Contains(stderr, "skipped") || strings.Contains(stderr, "check failed") {
		t.Errorf("stderr = %q, want no stage skipped and no check failed", stderr)
	}
	if !strings.Contains(stderr, "measures: checks   14 checks: 14 pass, 0 report failed, 0 gates failed\n") {
		t.Errorf("stderr = %q, want the measures summary logged", stderr)
	}
	for _, f := range []string{world.File, world.MeasuresFile, measure.SummaryFile} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Error(err)
		}
	}
	if code, vout, verr := validate(t, out); code != 0 {
		t.Errorf("validate: exit %d, stdout %q, stderr %q", code, vout, verr)
	}
	cfg, _ := readConfig(t, out)
	hash, _ := cfg.Hash()
	for _, s := range []string{hash, "stages  config, layout, elevation, mesh, cells, sea-level, climate, basins, land-target, rivers, classify, edges, measures, export\n", "33333 playable", "checks  14 checks: 14 pass, 0 report failed, 0 gates failed\n"} {
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

	// Stopping after the measures stage writes the measures but not
	// world.json.
	out := t.TempDir()
	code, stdout, stderr := generate(t, "--stop-after", "measures", "--config", smallConfig(t, 1, nil), "--output", out)
	if code != 0 || strings.Contains(stderr, "skipped") ||
		!strings.Contains(stderr, "stopped after stage 13 measures") || !strings.Contains(stdout, "stages  config, layout, elevation, mesh, cells, sea-level, climate, basins, land-target, rivers, classify, edges, measures\n") ||
		!strings.Contains(stdout, "checks  14 checks:") {
		t.Errorf("--stop-after measures: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	for f, want := range map[string]bool{world.MeasuresFile: true, measure.SummaryFile: true, world.File: false} {
		if _, err := os.Stat(filepath.Join(out, f)); (err == nil) != want {
			t.Errorf("--stop-after measures: %s written %v, want %v", f, err == nil, want)
		}
	}
}

// smallConfig writes a small world's config (300 land cells) with seed s
// and, when checks is not nil, those checks, and returns its path.
func smallConfig(t *testing.T, s uint64, checks []config.Check) string {
	t.Helper()
	cfg := config.Default()
	cfg.Seed = config.Seed(s)
	cfg.World.LandCells = 300
	cfg.Rim.FalloffCells = 4
	if checks != nil {
		cfg.Measures.Checks = checks
	}
	if err := cfg.Resolve(); err != nil {
		t.Fatal(err)
	}
	b, err := cfg.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "in.json")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestGenerateChecks checks the exit codes of the configured checks: a
// failed gate exits 3 with every output written; a failed report-only check
// exits 0; both are listed on stderr, and the verdict on stdout.
func TestGenerateChecks(t *testing.T) {
	gate := config.Check{Measure: "land.cells", Op: ">", Value: 1e9, Mode: config.ModeGate}
	report := config.Check{Measure: "directions.error_max_deg", Op: "<", Value: 0, Mode: config.ModeReport}
	pass := config.Check{Measure: "land.met", Op: "==", Value: 1, Mode: config.ModeGate}

	out := t.TempDir()
	code, stdout, stderr := generate(t, "--config", smallConfig(t, 2, []config.Check{pass, gate, report}), "--output", out)
	if code != 3 {
		t.Fatalf("failed gate: exit %d, want 3; stderr %q", code, stderr)
	}
	for _, s := range []string{"mpg generate: gate check failed: land.cells > 1e+09", "mpg generate: report check failed: directions.error_max_deg < 0", "mpg generate: 1 gate checks failed\n", "measures: check    FAIL  gate   land.cells > 1e+09"} {
		if !strings.Contains(stderr, s) {
			t.Errorf("failed gate: stderr lacks %q", s)
		}
	}
	if !strings.Contains(stdout, "checks  3 checks: 1 pass, 1 report failed, 1 gates failed\n") || !strings.Contains(stdout, "export\n") {
		t.Errorf("failed gate: stdout %q", stdout)
	}
	for _, f := range []string{"config.json", world.File, world.MeasuresFile, measure.SummaryFile} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Errorf("failed gate: %v", err)
		}
	}
	b, err := os.ReadFile(filepath.Join(out, world.MeasuresFile))
	if err != nil {
		t.Fatal(err)
	}
	m, err := world.DecodeMeasuresBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	if m.Pass || m.GatesFailed != 1 || m.ReportsFailed != 1 || len(m.Checks) != 3 || !m.Checks[0].Pass {
		t.Errorf("failed gate: measures.json checks %+v", m.Checks)
	}
	if code, vout, verr := validate(t, out); code != 0 {
		t.Errorf("failed gate: validate: exit %d, stdout %q, stderr %q", code, vout, verr)
	}

	// The same world with only the failing report check exits 0.
	out2 := t.TempDir()
	code, stdout, stderr = generate(t, "--config", smallConfig(t, 2, []config.Check{report}), "--output", out2)
	if code != 0 || !strings.Contains(stdout, "checks  1 checks: 0 pass, 1 report failed, 0 gates failed\n") ||
		!strings.Contains(stderr, "mpg generate: report check failed: directions.error_max_deg < 0") || strings.Contains(stderr, "gate checks failed") {
		t.Errorf("failed report check: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}

	// An unknown measure fails when the config resolves, before any stage.
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte(`{"schema": 1, "measures": {"checks": [{"measure": "land.cell", "op": "<=", "value": 1, "mode": "gate"}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out3 := t.TempDir()
	code, _, stderr = generate(t, "--config", path, "--output", out3)
	if code != 1 || !strings.Contains(stderr, `measures.checks[0].measure "land.cell" is not a measure; did you mean "land.cells"?`) {
		t.Errorf("unknown measure: exit %d, stderr %q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(out3, "config.json")); err == nil {
		t.Error("unknown measure: config.json written")
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
	if !slices.Equal(names, []string{"02-layout.png", "03-elevation.png", "04-mesh-area.png", "04-mesh-short.png", "04-mesh.png", "05-cells-relief.png", "05-cells.png", "06-sea-level.png", "07-climate-aridity.png", "07-climate-mask.png", "07-climate-moisture.png", "07-climate-pet.png", "07-climate-precip.png", "07-climate-runoff.png", "07-climate.png", "08-basins-depth.png", "08-basins-lakes.png", "08-basins.png", "09-land-target-aridity.png", "09-land-target-lakes.png", "09-land-target-precip.png", "09-land-target.png", "10-rivers-catchments.png", "10-rivers-tree.png", "10-rivers.png", "11-classify-biome.png", "11-classify-surface.png", "11-classify-wetness.png", "11-classify.png", "12-edges-compass.png", "12-edges-passability.png", "12-edges.png", "13-measures.png", "14-export.png"}) {
		t.Errorf("renders = %q, want the layout, elevation, mesh area, mesh short-edge, mesh, cell relief, cell altitude, sea level, aridity, climate mask, moisture, PET, precipitation, runoff, temperature, basin depth, lakes, basins, land-target final aridity, lakes and precipitation, search trace, river catchments, drainage tree, rivers by class, biome, surface, wetness, landform, compass, passability, incline, landmass and chokepoint, and player map renders", names)
	}
}
