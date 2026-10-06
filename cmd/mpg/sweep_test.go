// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mdhender/mpg/internal/basin"
	"github.com/mdhender/mpg/internal/climate"
	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/mesh"
	"github.com/mdhender/mpg/internal/pipeline"
	"github.com/mdhender/mpg/internal/render"
	"github.com/mdhender/mpg/world"
)

// sweep runs "mpg sweep args..." and returns the exit code, stdout, and
// stderr.
func sweep(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(append([]string{"sweep"}, args...), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestParseSeeds(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []uint64
	}{
		{"7", []uint64{7}},
		{"1-4", []uint64{1, 2, 3, 4}},
		{"1,5,9-12", []uint64{1, 5, 9, 10, 11, 12}},
		{" 3 , 2 ", []uint64{3, 2}},
		{"5-5", []uint64{5}},
		{"0-255", nil}, // 256 seeds: checked by length below
		{"18446744073709551614-18446744073709551615", []uint64{18446744073709551614, 18446744073709551615}},
	} {
		got, err := parseSeeds(tc.in)
		if err != nil {
			t.Errorf("parseSeeds(%q): %v", tc.in, err)
			continue
		}
		if tc.want == nil {
			if len(got) != maxSweepSeeds {
				t.Errorf("parseSeeds(%q) gave %d seeds", tc.in, len(got))
			}
			continue
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("parseSeeds(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
	for _, in := range []string{
		"", ",", "1,", "1,,2", "-1", "1-", "3-1", "x", "0x10", "+1", "1.5",
		"18446744073709551616", "1-2-3", "0-256", "0-18446744073709551615",
		"1-200,201-300", "1,1", "1-5,3",
	} {
		if got, err := parseSeeds(in); err == nil {
			t.Errorf("parseSeeds(%q) = %v, want an error", in, got)
		}
	}
}

func TestParseSweepStages(t *testing.T) {
	registry := pipeline.Stages()
	cols, err := parseSweepStages("layout,config,3,mesh:area", registry)
	if err != nil {
		t.Fatal(err)
	}
	want := []sweepStage{{"layout", 1, ""}, {"config", 0, ""}, {"elevation", 2, ""}, {"mesh:area", 3, "area"}}
	if !slices.Equal(cols, want) {
		t.Errorf("cols = %+v, want %+v", cols, want)
	}
	for _, in := range []string{"bogus", "noise", "layout,", "layout,layout", "elevation,3", "mesh:", "mesh:Area", "0"} {
		if _, err := parseSweepStages(in, registry); err == nil {
			t.Errorf("parseSweepStages(%q) succeeded", in)
		}
	}
	if _, err := parseSweepStages("bogus", registry); !strings.Contains(err.Error(), "elevation") {
		t.Errorf("unknown-stage error %q does not list the stages", err)
	}

	// Every stage is implemented, measures included.
	for _, tc := range []struct {
		in   string
		last int
		ok   bool
	}{{"config", 0, true}, {"layout,config", 1, true}, {"config,elevation", 2, true}, {"elevation,mesh", 3, true}, {"mesh,cells", 4, true}, {"cells:relief", 4, true}, {"cells,sea-level", 5, true}, {"sea-level", 5, true}, {"sea-level,climate", 6, true}, {"climate", 6, true}, {"climate:mask", 6, true}, {"climate:precip,climate:moisture,climate:pet,climate:runoff,climate:aridity", 6, true}, {"climate,basins", 7, true}, {"basins:depth", 7, true}, {"basins:lakes", 7, true}, {"basins,land-target", 8, true}, {"land-target:lakes", 8, true},
		{"classify", 10, true}, {"sea-level,classify", 10, true}, {"rivers,classify", 10, true}, {"rivers", 9, true}, {"rivers:catchments", 9, true}, {"measures", 12, true}, {"classify,edges", 11, true}, {"edges:passability", 11, true}, {"edges,measures", 12, true}} {
		cols, err := parseSweepStages(tc.in, registry)
		if err != nil {
			t.Fatal(err)
		}
		last, err := checkImplemented(cols, registry)
		if (err == nil) != tc.ok || last != tc.last {
			t.Errorf("checkImplemented(%q) = %d, %v", tc.in, last, err)
		}
	}

	// A stage that is not implemented cannot be a column, nor lie before
	// one, unless it is deferred.
	partial := pipeline.Stages()
	partial[12].Run = nil
	for _, tc := range []struct {
		in       string
		deferred bool
		last     int
		msg      string
	}{
		{"measures", false, -1, "stage 13 measures is not implemented yet"},
		{"edges,export", false, -1, "sweep cannot run through stage 14 export"},
		{"edges,export", true, 13, ""},
		{"measures", true, -1, "stage 13 measures is not implemented yet"},
	} {
		partial[12].Deferred = tc.deferred
		cols, err := parseSweepStages(tc.in, partial)
		if err != nil {
			t.Fatal(err)
		}
		last, err := checkImplemented(cols, partial)
		if last != tc.last || (tc.msg == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), tc.msg)) {
			t.Errorf("partial registry, deferred %v: checkImplemented(%q) = %d, %v; want %d, %q", tc.deferred, tc.in, last, err, tc.last, tc.msg)
		}
	}
}

func TestSweepErrors(t *testing.T) {
	out := filepath.Join(t.TempDir(), "sheet.png")
	for _, tc := range []struct {
		args []string
		code int
		msg  string
	}{
		{[]string{"--stage", "layout", "--output", out}, 2, "--seeds is required"},
		{[]string{"--seeds", "1", "--output", out}, 2, "--stage is required"},
		{[]string{"--seeds", "1", "--stage", "layout"}, 2, "--output is required"},
		{[]string{"--seeds", "2-1", "--stage", "layout", "--output", out}, 2, "reversed"},
		{[]string{"--seeds", "1", "--stage", "bogus", "--output", out}, 2, "sea-level"},
		{[]string{"--seeds", "1", "--stage", "layout", "--tile", "8", "--output", out}, 2, "--tile"},
		{[]string{"--seeds", "1", "--stage", "layout", "--aspect", "square,", "--output", out}, 2, "--aspect"},
		{[]string{"--seeds", "1", "--stage", "layout", "--output", out, "extra"}, 2, "unexpected"},
		{[]string{"--seeds", "1", "--stage", "layout", "--aspect", "squarish", "--output", out}, 1, "world.aspect"},
		{[]string{"--seeds", "1", "--stage", "layout", "--config", "no-such-file.json", "--output", out}, 1, "no-such-file"},
	} {
		code, _, stderr := sweep(t, tc.args...)
		if code != tc.code || !strings.Contains(stderr, tc.msg) {
			t.Errorf("sweep %q: exit %d, stderr %q; want exit %d mentioning %q", tc.args, code, stderr, tc.code, tc.msg)
		}
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("sheet written on an error: %v", err)
	}
}

// sweepPixelHash pins the small sweep below: seeds 1 and 7, the elevation
// render and the (render-less) config stage, cinematic and square, 600 land
// cells, 64-pixel tiles. Row labels carry the layout preset.
const sweepPixelHash = "b75c1f27c0fd8adbf1c168b641d126061843fc87bd454206fff4bbf05a7bee56"

func TestSweepSheet(t *testing.T) {
	dir := t.TempDir()
	args := []string{"--seeds", "1,7", "--stage", "elevation,config", "--aspect", "cinematic,square",
		"--land-cells", "600", "--tile", "64"}
	var hashes []string
	for n := range 2 {
		out := filepath.Join(dir, "sub", strings.Repeat("x", n+1)+".png")
		code, stdout, stderr := sweep(t, append(args, "--output", out)...)
		if code != 0 {
			t.Fatalf("exit %d; stderr %q", code, stderr)
		}
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		hash := render.PixelHash(img)
		hashes = append(hashes, hash)
		if !strings.Contains(stdout, "pixels  "+hash) {
			t.Errorf("stdout %q does not report the pixel hash", stdout)
		}

		// Layout: label column, two 64-pixel tile columns; four rows whose
		// tile heights follow each world's shape.
		cfg := config.Default()
		cfg.World.LandCells = 600
		var tileHs []int
		for _, aspect := range []string{"cinematic", "square"} {
			c := cfg
			c.World.Aspect = aspect
			if err := c.Resolve(); err != nil {
				t.Fatal(err)
			}
			h := int(float64(64)*c.World.HeightKm/c.World.WidthKm + 0.5)
			tileHs = append(tileHs, h, h)
		}
		labelW := len("continents") * render.LabelAdvance // the preset is the longest label line
		wantW := 8 + labelW + 8 + 2*(64+8)
		wantH := 8 + render.LabelHeight + 4
		for _, h := range tileHs {
			wantH += h + 2*render.LabelHeight + 2 + 8
		}
		if img.Bounds() != image.Rect(0, 0, wantW, wantH) {
			t.Errorf("sheet is %v, want %d x %d (tile heights %v)", img.Bounds(), wantW, wantH, tileHs)
		}

		meta, err := render.ReadMeta(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		base := config.Default()
		base.World.LandCells = 600
		if err := base.Resolve(); err != nil {
			t.Fatal(err)
		}
		baseHash, _ := base.Hash()
		if meta.Stage != "sweep" || meta.ConfigHash != baseHash {
			t.Errorf("meta = %+v, want stage sweep and base hash %s", meta, baseHash)
		}
		extra := map[string]string{}
		for _, e := range meta.Extra {
			extra[e.Key] = e.Value
		}
		for k, v := range map[string]string{
			"mpg:seeds": "1,7", "mpg:stages": "elevation,config", "mpg:aspects": "cinematic,square", "mpg:presets": "continents", "mpg:tile-width": "64",
		} {
			if extra[k] != v {
				t.Errorf("%s = %q, want %q", k, extra[k], v)
			}
		}
		if rows := strings.Split(extra["mpg:rows"], "\n"); len(rows) != 4 || !strings.HasPrefix(rows[0], "1 cinematic continents ") || !strings.HasPrefix(rows[3], "7 square continents ") {
			t.Errorf("mpg:rows = %q", extra["mpg:rows"])
		}
	}
	if hashes[0] != hashes[1] {
		t.Errorf("two runs gave pixel hashes %s and %s", hashes[0], hashes[1])
	}
	if hashes[0] != sweepPixelHash {
		t.Errorf("pixel hash = %s, want %s", hashes[0], sweepPixelHash)
	}
}

// TestSweepPresets checks that --preset adds a block of rows per preset
// within each aspect, and that the layout stage renders in a sweep.
func TestSweepPresets(t *testing.T) {
	out := filepath.Join(t.TempDir(), "presets.png")
	code, stdout, stderr := sweep(t, "--seeds", "3,4", "--stage", "layout", "--preset", "pangaea,islands",
		"--aspect", "square", "--land-cells", "2000", "--tile", "64", "--output", out)
	if code != 0 {
		t.Fatalf("exit %d; stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "presets pangaea, islands") {
		t.Errorf("stdout %q does not list the presets", stdout)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := render.ReadMeta(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	var rows []string
	for _, e := range meta.Extra {
		if e.Key == "mpg:rows" {
			rows = strings.Split(e.Value, "\n")
		}
	}
	want := []string{"3 square pangaea ", "4 square pangaea ", "3 square islands ", "4 square islands "}
	if len(rows) != len(want) {
		t.Fatalf("rows = %q", rows)
	}
	for k, w := range want {
		if !strings.HasPrefix(rows[k], w) {
			t.Errorf("row %d = %q, want prefix %q", k, rows[k], w)
		}
	}
	if code, _, stderr := sweep(t, "--seeds", "1", "--stage", "layout", "--preset", "isles", "--output", out); code != 1 || !strings.Contains(stderr, "layout.preset") {
		t.Errorf("bad preset: exit %d, stderr %q", code, stderr)
	}
	if code, _, stderr := sweep(t, "--seeds", "1", "--stage", "layout", "--preset", "islands,islands", "--output", out); code != 2 || !strings.Contains(stderr, "--preset") {
		t.Errorf("repeated preset: exit %d, stderr %q", code, stderr)
	}
}

// TestSweepSeaLevel checks that a sweep runs through the sea level stage
// and the climate (temperature, mask, precipitation and aridity), the basins, and on to classification and
// the edges, past the deferred stages, and
// gives the same sheet twice.
func TestSweepSeaLevel(t *testing.T) {
	dir := t.TempDir()
	var hashes []string
	for _, name := range []string{"a.png", "b.png"} {
		code, stdout, stderr := sweep(t, "--seeds", "1,2", "--stage", "cells,sea-level,climate,climate:mask,climate:precip,climate:aridity,basins,basins:depth,basins:lakes,classify,edges,edges:passability", "--land-cells", "600",
			"--tile", "64", "--output", filepath.Join(dir, name))
		if code != 0 {
			t.Fatalf("exit %d; stderr %q", code, stderr)
		}
		if !strings.Contains(stdout, "stages  cells, sea-level, climate, climate:mask, climate:precip, climate:aridity, basins, basins:depth, basins:lakes, classify, edges, edges:passability") || !strings.Contains(stderr, "basins: ") || !strings.Contains(stderr, " lake cells; ") || !strings.Contains(stderr, "climate: rim temperature ") || !strings.Contains(stderr, "sea-level: level ") ||
			!strings.Contains(stderr, "classify: land: ") || !strings.Contains(stderr, "edges: direction error ") {
			t.Errorf("stdout %q, stderr %q", stdout, stderr)
		}
		_, h, _ := strings.Cut(stdout, "pixels  ")
		hashes = append(hashes, h)
	}
	if hashes[0] != hashes[1] {
		t.Errorf("two sweeps differ: %q, %q", hashes[0], hashes[1])
	}
}

// TestClimateCaption checks the climate tile captions by variant on a
// three-cell world: a rim cell and two land cells.
func TestClimateCaption(t *testing.T) {
	cells := []mesh.Cell{{Rim: true}, {}, {}}
	r := &climate.Result{
		Ocean:         []bool{true, false, false},
		Temperature:   []float64{-20, 10, 20},
		Precipitation: []float64{150, 300, 1200},
		PET:           []float64{0, 589.3, 1178.6},
		Runoff:        []float64{150, 20, 400},
		Moisture:      []float64{1, 0.25, 0.75},
		Aridity:       []float64{10, 300 / 589.3, 1200 / 1178.6},
	}
	for _, tc := range []struct{ variant, want string }{
		{"", "rim -20..-20 land 10..20 ~20 C"},
		{"mask", "rim -20..-20 land 10..20 ~20 C"},
		{"precip", "land P 300/300/1200 mm"},
		{"pet", "land PET 589/589/1179 mm"},
		{"runoff", "land R 20/20/400 mm"},
		{"moisture", "land moisture 0.25/0.25/0.75"},
		{"aridity", "ha0 a0 sa0 ds50 hu50 %"},
	} {
		if got := climateCaption(cells, r, tc.variant); got != tc.want {
			t.Errorf("variant %q: caption %q, want %q", tc.variant, got, tc.want)
		}
	}
}

// TestLakesCaption checks the basins:lakes tile caption on a hand-made
// water balance: a lake, a salt inland sea, a playa, and a full basin.
func TestLakesCaption(t *testing.T) {
	l := &basin.Lakes{
		Water:  []basin.Water{{State: basin.Full}, {State: basin.Partial}, {State: basin.Dry}},
		Lakes:  []basin.Lake{{Cells: []int{1, 2}}, {Cells: make([]int, 20), Kind: basin.KindInlandSea, Salt: true}},
		Playas: []basin.Playa{{Basin: 2}},
	}
	if got, want := lakesCaption(l), "lk 1 sea 1 salt 1 pl 1 full 1 cells 22"; got != want {
		t.Errorf("caption %q, want %q", got, want)
	}
}

// TestBasinsCaption checks the basins tile caption on a hand-made
// hierarchy: a basin nested in another, and a cell in each.
func TestBasinsCaption(t *testing.T) {
	r := &basin.Result{
		Depressions: make([]basin.Basin, 3),
		Basins:      []basin.Basin{{Parent: 1}, {Parent: basin.None}},
		Of:          []int{basin.None, 0, 1, basin.None},
	}
	if got, want := basinsCaption(r), "dep 3 bas 2 nest 1 cells 2"; got != want {
		t.Errorf("caption %q, want %q", got, want)
	}
}

func TestMeasuresCaption(t *testing.T) {
	m := &world.Measures{Checks: make([]world.CheckResult, 14), ReportsFailed: 2, GatesFailed: 1}
	if got, want := measuresCaption(m), "ok 11/14 rpt 2 gate 1"; got != want {
		t.Errorf("measuresCaption = %q, want %q", got, want)
	}
}

func TestChokepointCaption(t *testing.T) {
	m := &world.Measures{}
	m.Landmasses = world.LandmassMeasures{Count: 28, Continents: 5, Islands: 7, Islets: 16}
	m.Chokepoints = world.ChokepointMeasures{Straits: 19, StraitsMajor: 5, Necks: 5, Passes: 29}
	if got, want := chokepointCaption(m), "LM 28 (5c/7i/16.) str 19/5 neck 5 pass 29"; got != want {
		t.Errorf("chokepointCaption = %q, want %q", got, want)
	}
}

// TestMeasuresTileCaption checks a measures tile's caption lines: the
// usability and rivers line and the features line (S35) follow the
// chokepoint and check lines.
func TestMeasuresTileCaption(t *testing.T) {
	m := &world.Measures{Checks: make([]world.CheckResult, 14)}
	m.Usability = world.UsabilityMeasures{HabitableShare: 0.877, WetlandShare: 0.026, CoastCells: 3, CoastWithinShare: 0.56}
	m.Rivers = world.RiverMeasures{EdgesPerLandCell: 0.201, Mouths: 234, LongestEdges: 35}
	m.Features = world.FeatureMeasures{Depressions: 320, Basins: 62, DryBasins: 2, Playas: 2, GlacierCells: 9, IceFieldCells: 2, Volcanoes: 4, Hotspots: 6}
	got := measuresTileCaption(m)
	want := []string{chokepointCaption(m), measuresCaption(m), "hab 88% wet 3% c3 56% riv .20 mo 234 L35", "dep 320 bas 62 dry 2 pl 2 ice 11 v 4/6"}
	if !slices.Equal(got, want) {
		t.Errorf("measuresTileCaption = %q, want %q", got, want)
	}
	m.Usability.CoastCells = 5
	m.Rivers.EdgesPerLandCell = 1.234
	if got := usabilityCaption(m); got != "hab 88% wet 3% c5 56% riv 1.23 mo 234 L35" {
		t.Errorf("usabilityCaption = %q", got)
	}
	for _, line := range got {
		if len(line) > 320/render.LabelAdvance {
			t.Errorf("caption %q is wider than a default tile", line)
		}
	}
}

// TestSweepMeasures checks that a measures column runs, that its tiles
// get five caption lines (the feature, river and usability measures among
// them: S35's "measures appear in sweep tile labels"), and that a failed
// gate does not stop the sweep.
func TestSweepMeasures(t *testing.T) {
	path := smallConfig(t, 1, []config.Check{{Measure: "land.cells", Op: "<", Value: 0, Mode: config.ModeGate}})
	dir := t.TempDir()
	height := func(stages string) int {
		t.Helper()
		out := filepath.Join(dir, strings.ReplaceAll(stages, ",", "-")+".png")
		code, stdout, stderr := sweep(t, "--config", path, "--seeds", "1,2", "--stage", stages, "--tile", "64", "--output", out)
		if code != 0 || !strings.Contains(stdout, "stages  "+strings.ReplaceAll(stages, ",", ", ")+"\n") {
			t.Fatalf("exit %d; stdout %q; stderr %q", code, stdout, stderr)
		}
		f, err := os.Open(out)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		cfg, err := png.DecodeConfig(f)
		if err != nil {
			t.Fatal(err)
		}
		return cfg.Height
	}
	// Two rows, each with three caption lines more than an edges sheet's
	// two.
	if a, b := height("edges"), height("edges,measures"); b-a != 2*3*render.LabelHeight {
		t.Errorf("sheet heights %d and %d: want %d more with a measures column", a, b, 2*3*render.LabelHeight)
	}
}
