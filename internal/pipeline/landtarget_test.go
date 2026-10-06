// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package pipeline

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mdhender/mpg/internal/basin"
	"github.com/mdhender/mpg/internal/cells"
	"github.com/mdhender/mpg/internal/climate"
	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/elevation"
)

// runTo runs a world through the named stage and returns its context.
func runTo(t *testing.T, cfg config.Config, stage, renders string) *Context {
	t.Helper()
	c, err := NewContext(cfg, t.TempDir(), renders, nil)
	if err != nil {
		t.Fatal(err)
	}
	last, err := Lookup(Stages(), stage)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Run(c, Stages(), last); err != nil {
		t.Fatal(err)
	}
	return c
}

// checkLandTarget checks a land target against its own record: the land is
// counted after lakes, the record's reason and met flag agree with the
// deviation, and the pass count is saved.
func checkLandTarget(t *testing.T, c *Context) {
	t.Helper()
	p := c.Products
	lt, m := p.Target, p.Mesh
	r := lt.Search
	land, lake, ocean, dry := 0, 0, 0, 0
	area := 0.0
	for i, cell := range m.Cells {
		isLake := lt.Lakes.Lake[i] != basin.None
		switch {
		case cell.Rim:
			if lt.Land[i] || isLake {
				t.Fatalf("rim cell %d is land or lake", i)
			}
		case lt.Flood.Ocean[i]:
			ocean++
			if lt.Land[i] || isLake {
				t.Fatalf("ocean cell %d is land or lake", i)
			}
		case isLake:
			lake++
			if lt.Land[i] {
				t.Fatalf("lake cell %d counted as land", i)
			}
		default:
			if !lt.Land[i] {
				t.Fatalf("playable cell %d is neither ocean, lake nor land", i)
			}
			land++
			area += m.Area(i)
			if p.Cells.Altitude[i] <= lt.Flood.Level {
				dry++
			}
		}
	}
	best := r.Result()
	if land != lt.LandCells || lake != lt.LakeCells || ocean != lt.OceanCells || dry != lt.DryBasinCells || area != lt.LandAreaKm2 {
		t.Errorf("counts %d land, %d lake, %d ocean, %d dry, %v km²; recorded %d, %d, %d, %d, %v",
			land, lake, ocean, dry, area, lt.LandCells, lt.LakeCells, lt.OceanCells, lt.DryBasinCells, lt.LandAreaKm2)
	}
	if best.Land != land || best.Lake != lake || best.Level != lt.Flood.Level || lt.Flood.LandCells-lake != land {
		t.Errorf("best probe %+v, flood land %d: land after lakes is %d with %d lake cells", best, lt.Flood.LandCells, land, lake)
	}
	dev := land - r.Target
	met := 100*max(dev, -dev) <= cells.TolerancePercent*r.Target
	switch {
	case r.Met != met || lt.Met() != met:
		t.Errorf("met %v, but land %d of %d", r.Met, land, r.Target)
	case dev == 0 && r.Reason != cells.ReasonExact,
		dev != 0 && met && r.Reason != cells.ReasonWithinTolerance,
		!met && r.Reason != cells.ReasonUnreachable && r.Reason != cells.ReasonBudgetExhausted:
		t.Errorf("reason %s for land %d of %d ± %d", r.Reason, land, r.Target, r.Tolerance)
	}
	if r.Target != c.Config.World.LandCells || len(r.Trace) == 0 || len(r.Trace) > r.Budget {
		t.Errorf("target %d, %d probes of %d", r.Target, len(r.Trace), r.Budget)
	}
	if r.Expected != p.Lakes.Cells() {
		t.Errorf("first probe expected %d lake cells, the basins stage found %d", r.Expected, p.Lakes.Cells())
	}
	if lt.ClimatePasses != ClimatePasses || lt.Climate == nil {
		t.Errorf("%d climate passes, final climate %v", lt.ClimatePasses, lt.Climate != nil)
	}
	if pp := p.PrePass; pp == nil || pp.LakeCells < 0 || pp.LandShare != elevation.LandShareWithLakes(c.Config, pp.LakeCells) {
		t.Errorf("pre-pass %+v", pp)
	}
}

// TestLandTargetMeetsTarget runs the land-target search with the basins
// inside it on worlds of every layout, and checks that each meets the 1%
// target or reports it unmet, with land counted after lakes and the reason
// consistent with the deviation; and that lakes are present.
func TestLandTargetMeetsTarget(t *testing.T) {
	type row struct {
		seed           config.Seed
		aspect, preset string
	}
	rows := []row{
		{1, "cinematic", "continents"}, {2, "square", "continents"}, {3, "cinematic", "pangaea"},
		{7, "cinematic", "pangaea"}, {4, "square", "pangaea"}, {5, "cinematic", "archipelago"},
	}
	land := 0 // default
	if testing.Short() {
		land, rows = 2_000, rows[:3]
	}
	for _, r := range rows {
		t.Run(fmt.Sprintf("seed%d-%s-%s", r.seed, r.aspect, r.preset), func(t *testing.T) {
			t.Parallel()
			cfg := config.Default()
			cfg.Seed, cfg.World.Aspect, cfg.Layout.Preset = r.seed, r.aspect, r.preset
			if land > 0 {
				cfg.World.LandCells = land
			}
			c := runTo(t, cfg, "land-target", "")
			checkLandTarget(t, c)
			lt := c.Products.Target
			if lt.LakeCells == 0 || len(lt.Lakes.Lakes) == 0 {
				t.Errorf("no lakes")
			}
			if !lt.Met() {
				t.Errorf("unmet: %d land of %d, %s", lt.LandCells, lt.Search.Target, lt.Search.Reason)
			}
			t.Logf("level %.1f m: %d land after %d lake cells (%+d), %s in %d probes; pre-pass %d lake cells",
				lt.Flood.Level, lt.LandCells, lt.LakeCells, lt.Search.Deviation(), lt.Search.Reason, len(lt.Search.Trace), c.Products.PrePass.LakeCells)
		})
	}
}

// TestLandTargetUnmet checks that an unreachable target is reported, not an
// error: more land than there are playable cells.
func TestLandTargetUnmet(t *testing.T) {
	cfg := config.Default()
	cfg.Seed = 3
	cfg.World.LandCells = 600
	cfg.Rim.FalloffCells = 4
	c := runTo(t, cfg, "basins", "")
	p := c.Products
	big := c.Config
	big.World.LandCells = len(p.Mesh.Cells) + 100
	lt, err := SearchLand(p.Mesh, p.Cells.Altitude, p.Climate, &big, 0)
	if err != nil {
		t.Fatal(err)
	}
	r := lt.Search
	if r.Met || r.Reason != cells.ReasonUnreachable || lt.LandCells != r.Result().Land || lt.LandCells >= big.World.LandCells {
		t.Errorf("met %v, reason %s, land %d of %d", r.Met, r.Reason, lt.LandCells, big.World.LandCells)
	}
}

// TestLandTargetStage checks the stage's product, renders and log, and that
// the export records the search and the passes.
func TestLandTargetStage(t *testing.T) {
	cfg := config.Default()
	cfg.Seed = 42
	cfg.World.LandCells = 600
	cfg.Rim.FalloffCells = 4
	dir := t.TempDir()
	c, err := NewContext(cfg, filepath.Join(dir, "out"), filepath.Join(dir, "renders"), nil)
	if err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	c.Log = &log
	if _, err := Run(c, Stages(), -1); err != nil {
		t.Fatal(err)
	}
	checkLandTarget(t, c)
	for _, name := range []string{"09-land-target.png", "09-land-target-lakes.png", "09-land-target-precip.png", "09-land-target-aridity.png"} {
		if _, err := os.Stat(filepath.Join(c.RendersDir, name)); err != nil {
			t.Error(err)
		}
	}
	for _, s := range []string{"elevation: pre-pass (stages 3–8, no lake allowance): sea level ", "land-target: target 600 land cells", "after lakes", "2 climate passes"} {
		if !strings.Contains(log.String(), s) {
			t.Errorf("log lacks %q:\n%s", s, log.String())
		}
	}
	lt, o := c.Products.Target, c.Products.World.Outcomes
	if o.LandCells != lt.LandCells || o.ClimatePasses != ClimatePasses || o.SeaLevelM != lt.Flood.Level ||
		o.ExpectedLakeCells != lt.Search.Expected || o.PrePassLakeCells != c.Products.PrePass.LakeCells ||
		o.DatumLandShare != c.Products.PrePass.LandShare || len(o.Trace) != len(lt.Search.Trace) ||
		o.LakeCells+o.InlandSeaCells != lt.LakeCells {
		t.Errorf("outcomes %+v do not record the land target", o)
	}
	for k, p := range o.Trace {
		if q := lt.Search.Trace[k]; p.Land != q.Land || p.Lake != q.Lake || p.LevelM != q.Level {
			t.Errorf("outcome probe %d %+v, search %+v", k, p, q)
		}
	}
}

// TestLandTargetDeterministic runs the same world twice, the elevation
// pre-pass included, and compares the products bit for bit.
func TestLandTargetDeterministic(t *testing.T) {
	cfg := config.Default()
	cfg.Seed = 6
	cfg.Layout.Preset = "pangaea"
	cfg.World.LandCells = 2_000
	var encs [][]byte
	var pps []PrePass
	for range 2 {
		c := runTo(t, cfg, "land-target", "")
		b, _ := c.Products.Target.AppendBinary(nil)
		for _, v := range c.Products.Elevation.Values() {
			b = binary.LittleEndian.AppendUint64(b, math.Float64bits(v))
		}
		encs = append(encs, b)
		pps = append(pps, *c.Products.PrePass)
	}
	if !bytes.Equal(encs[0], encs[1]) || pps[0] != pps[1] {
		t.Errorf("two runs differ: pre-pass %+v and %+v", pps[0], pps[1])
	}
}

// TestPrePassAllowance checks that the elevation stage's datum allows for
// the pre-pass's lake cells, and that the pre-pass is the run with no
// allowance: its lake count is the basins stage's on a run whose datum is
// the plain land fraction.
func TestPrePassAllowance(t *testing.T) {
	cfg := config.Default()
	cfg.Seed = 2
	cfg.World.LandCells = 2_000
	c := runTo(t, cfg, "elevation", "")
	pp := c.Products.PrePass
	if pp == nil || pp.LakeCells == 0 {
		t.Fatalf("pre-pass %+v: want some lake cells", pp)
	}
	want := cfg.World.LandFraction * float64(cfg.World.LandCells+pp.LakeCells) / float64(cfg.World.LandCells)
	if d := pp.LandShare - want; d > 1e-12 || d < -1e-12 {
		t.Errorf("datum land share %v, want %v", pp.LandShare, want)
	}
	// The same stages by hand, with no allowance.
	pc := &Context{Config: c.Config, ConfigHash: c.ConfigHash, Log: c.Log, prepass: true}
	pc.Products.Layout, pc.Products.Bias = c.Products.Layout, c.Products.Bias
	for _, st := range Stages()[2:8] {
		pc.stage = st
		if err := st.Run(pc); err != nil {
			t.Fatal(err)
		}
	}
	if got := pc.Products.Lakes.Cells(); got != pp.LakeCells || pc.Products.SeaLevel.Level != pp.SeaLevelM {
		t.Errorf("pre-pass %+v; the stages with no allowance give %d lake cells at %v m", pp, got, pc.Products.SeaLevel.Level)
	}
	if share := elevation.LandShare(pc.Products.Elevation, 0); share >= elevation.LandShare(c.Products.Elevation, 0) {
		t.Errorf("no-allowance land share %v not below the allowed field's %v", share, elevation.LandShare(c.Products.Elevation, 0))
	}
}

// TestFinalClimateInlandSeas checks the final climate pass on a world with
// lakes and inland seas: inland-sea cells are open water that recharges the
// air (moisture 1, no lift), lake cells are not, the mask follows, and
// making the inland seas sources never dries a sample and wets some.
func TestFinalClimateInlandSeas(t *testing.T) {
	cfg := config.Default()
	cfg.Seed = 3
	cfg.Layout.Preset = "pangaea"
	c := runTo(t, cfg, "land-target", "")
	p := c.Products
	lt, m := p.Target, p.Mesh
	sea := lt.InlandSeas()
	r := lt.Climate
	seas, lakes := 0, 0
	for i, cell := range m.Cells {
		want := cell.Rim || lt.Flood.Ocean[i] || sea[i]
		if r.Ocean[i] != want {
			t.Fatalf("cell %d: open water %v, want %v", i, r.Ocean[i], want)
		}
		switch {
		case sea[i]:
			seas++
			if r.Moisture[i] != 1 || r.LiftM[i] != 0 {
				t.Errorf("inland-sea cell %d: moisture %v, lift %v", i, r.Moisture[i], r.LiftM[i])
			}
		case lt.Lakes.Lake[i] != basin.None:
			lakes++
		}
	}
	if seas == 0 || lakes == 0 {
		t.Fatalf("%d inland-sea and %d lake cells: the fixture needs both", seas, lakes)
	}
	for k, o := range p.Cells.Owner {
		if r.Mask[k] != r.Ocean[o] {
			t.Fatalf("sample %d: mask %v, cell %d open water %v", k, r.Mask[k], o, r.Ocean[o])
		}
	}
	// Lakes do not recharge: the same pass with no inland seas as sources.
	model, err := climate.NewModel(c.Config.Climate)
	if err != nil {
		t.Fatal(err)
	}
	none, err := climate.ComputeFinal(p.Elevation, m, p.Cells, lt.Flood, make([]bool, len(m.Cells)), model, uint64(c.Config.Seed))
	if err != nil {
		t.Fatal(err)
	}
	wetter := 0
	for i := range m.Cells {
		if r.Ocean[i] {
			continue
		}
		if r.Moisture[i] < none.Moisture[i]-1e-12 {
			t.Fatalf("cell %d: moisture %v with inland seas as sources, %v without", i, r.Moisture[i], none.Moisture[i])
		}
		if r.Moisture[i] > none.Moisture[i]+1e-9 {
			wetter++
		}
		if r.Temperature[i] != none.Temperature[i] {
			t.Fatalf("cell %d: sources changed the temperature", i)
		}
	}
	if wetter == 0 {
		t.Error("inland seas wet no land")
	}
	// Lake and inland-sea cells take their altitude's height, as land.
	for i, cell := range m.Cells {
		if cell.Rim || lt.Lakes.Lake[i] == basin.None {
			continue
		}
		want := max(p.Cells.Altitude[i]-lt.Flood.Level, 0)
		if r.HeightM[i] != want {
			t.Fatalf("lake cell %d: height %v, want %v", i, r.HeightM[i], want)
		}
	}
	t.Logf("%d inland-sea and %d lake cells; %d land and lake cells wetter with the inland seas as sources", seas, lakes, wetter)
}
