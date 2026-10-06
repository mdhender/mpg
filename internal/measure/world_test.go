// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package measure_test

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/mdhender/mpg/internal/basin"
	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/measure"
	"github.com/mdhender/mpg/internal/pipeline"
	"github.com/mdhender/mpg/world"
)

// run runs the pipeline through the measures stage on a small world with
// cfg's checks and returns its context and output directory.
func run(t *testing.T, seed uint64, preset string, checks []config.Check) (*pipeline.Context, string) {
	t.Helper()
	c := config.Default()
	c.Seed = config.Seed(seed)
	c.Layout.Preset = preset
	c.World.LandCells = 600
	c.Rim.FalloffCells = 4
	if checks != nil {
		c.Measures.Checks = checks
	}
	out := t.TempDir()
	ctx, err := pipeline.NewContext(c, out, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	stages := pipeline.Stages()
	last, err := pipeline.Lookup(stages, "measures")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.Run(ctx, stages, last); err != nil {
		t.Fatal(err)
	}
	if ctx.Products.Measures == nil {
		t.Fatal("no measures")
	}
	return ctx, out
}

func sum(xs []int) int {
	n := 0
	for _, x := range xs {
		n += x
	}
	return n
}

// TestWorldMeasures checks the measures of small worlds against the stage
// products they come from, and that the stage writes the report and the
// summary.
func TestWorldMeasures(t *testing.T) {
	for _, tc := range []struct {
		seed   uint64
		preset string
	}{{1, "continents"}, {2, "archipelago"}, {3, "pangaea"}} {
		ctx, out := run(t, tc.seed, tc.preset, nil)
		p := ctx.Products
		m := p.Measures
		tg, es := p.Target, p.EdgeStats
		if m.Schema != world.MeasuresSchemaVersion || m.ConfigHash != ctx.ConfigHash || m.Seed != strconv.FormatUint(tc.seed, 10) {
			t.Errorf("seed %d: header %+v", tc.seed, m)
		}
		l := m.Land
		if l.Cells != tg.LandCells || l.TargetCells != tg.Search.Target || l.Met != tg.Met() ||
			l.DeviationCells != tg.LandCells-tg.Search.Target || l.AreaKm2 != tg.LandAreaKm2 {
			t.Errorf("seed %d: land %+v", tc.seed, l)
		}
		me := m.Mesh
		if me.PlayableCells != es.Playable || sum(me.NeighborsPlayable) != es.Playable || sum(me.NeighborsLand) != tg.LandCells ||
			me.Cells != len(p.Mesh.Cells) || me.DegreeCapHits != p.Mesh.DegreeCapHits {
			t.Errorf("seed %d: mesh counts %+v", tc.seed, me)
		}
		if !(me.EdgeMinKm >= me.MinEdgeKm && me.EdgeP5Km >= me.EdgeMinKm) || !(me.LandCellAreaMinA <= me.LandCellAreaMeanA && me.LandCellAreaMeanA <= me.LandCellAreaMaxA) {
			t.Errorf("seed %d: mesh lengths and areas %+v", tc.seed, me)
		}
		d := m.Directions
		if d.ErrorP95Deg != es.ErrorP95 || d.NotOpposite != es.NotOpposite || !(d.ErrorMeanDeg <= d.ErrorMaxDeg && d.ErrorP95Deg <= d.ErrorMaxDeg) {
			t.Errorf("seed %d: directions %+v", tc.seed, d)
		}
		g := m.Grades
		if sum(g.LandLand) != g.LandEdges || sum(g.Playable) != es.Pairs || g.LandMaxPercent != float64(es.MaxLandGrade)/10 || g.LandP95Percent > g.LandMaxPercent {
			t.Errorf("seed %d: grades %+v", tc.seed, g)
		}
		w := m.Water
		lakes, seas := 0, 0
		for _, lk := range tg.Lakes.Lakes {
			if lk.Kind == basin.KindInlandSea {
				seas++
			} else {
				lakes++
			}
		}
		if w.OceanCells != tg.OceanCells || w.LakeCells+w.InlandSeaCells != tg.LakeCells || w.Lakes != lakes || w.InlandSeas != seas ||
			w.CoastEdges != es.Coast || w.LandRimEdges != es.LandRim {
			t.Errorf("seed %d: water %+v", tc.seed, w)
		}
		lm := m.Landmasses
		cells := 0
		for _, x := range lm.List {
			cells += x.Cells
		}
		if lm.Count != len(lm.List) || lm.Continents+lm.Islands+lm.Islets != lm.Count || sum(lm.Sizes) != lm.Count || cells != tg.LandCells ||
			lm.LargestShare <= 0 || lm.LargestShare > 1 {
			t.Errorf("seed %d: landmasses %+v", tc.seed, lm)
		}
		cp := m.Chokepoints
		if cp.Straits != len(cp.StraitList) || cp.StraitsBetween+cp.StraitsWithin != cp.Straits || cp.StraitsMajor > cp.Straits ||
			sum(cp.StraitWidths) != cp.Straits || cp.Necks != len(cp.NeckList) || sum(cp.NeckWidths) != cp.Necks ||
			cp.Passes != len(cp.PassList) || cp.ChainsWithPass > cp.Chains {
			t.Errorf("seed %d: chokepoints %+v", tc.seed, cp)
		}
		for _, x := range cp.StraitList {
			for _, c := range x.Cells {
				if tg.Land[c] || p.Mesh.Cells[c].Rim {
					t.Errorf("seed %d: strait cell %d is not playable water", tc.seed, c)
				}
			}
		}
		for _, x := range slices.Concat(cp.NeckList, cp.PassList) {
			for _, c := range x.Cells {
				if !tg.Land[c] {
					t.Errorf("seed %d: neck or pass cell %d is not land", tc.seed, c)
				}
			}
		}
		if tg.LandCells+tg.OceanCells+tg.LakeCells != es.Playable {
			t.Errorf("seed %d: land, ocean and lakes do not cover the playable cells", tc.seed)
		}
		// The files: measures.json decodes to the report, and
		// measures.txt is the summary.
		b, err := os.ReadFile(filepath.Join(out, world.MeasuresFile))
		if err != nil {
			t.Fatal(err)
		}
		want, err := m.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(b, want) {
			t.Errorf("seed %d: measures.json is not the report", tc.seed)
		}
		if _, err := world.DecodeMeasuresBytes(b); err != nil {
			t.Errorf("seed %d: %v", tc.seed, err)
		}
		txt, err := os.ReadFile(filepath.Join(out, measure.SummaryFile))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(txt, measure.Summary(m)) {
			t.Errorf("seed %d: measures.txt is not the summary", tc.seed)
		}
	}
}

// TestMeasuresByteStable checks that the same config gives the same
// measures.json bytes on a fresh run.
func TestMeasuresByteStable(t *testing.T) {
	_, a := run(t, 5, "islands", nil)
	_, b := run(t, 5, "islands", nil)
	ba, err := os.ReadFile(filepath.Join(a, world.MeasuresFile))
	if err != nil {
		t.Fatal(err)
	}
	bb, err := os.ReadFile(filepath.Join(b, world.MeasuresFile))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ba, bb) {
		t.Error("measures.json differs between runs of one config")
	}
	if bytes.Contains(ba, []byte(a)) || bytes.Contains(ba, []byte(os.TempDir())) {
		t.Error("measures.json holds a path")
	}
}

// TestGateIsNotAStageError checks that a failing gate leaves the stage
// successful (so export still runs) and is recorded in the report.
func TestGateIsNotAStageError(t *testing.T) {
	ctx, _ := run(t, 1, "continents", []config.Check{
		{Measure: "land.cells", Op: "<", Value: 0, Mode: config.ModeGate},
		{Measure: "water.lakes", Op: ">=", Value: 0, Mode: config.ModeReport},
	})
	m := ctx.Products.Measures
	if m.Pass || m.GatesFailed != 1 || m.ReportsFailed != 0 || m.Checks[0].Pass || !m.Checks[1].Pass {
		t.Errorf("checks %+v", m.Checks)
	}
}
