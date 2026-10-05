// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package export_test

import (
	"bytes"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/export"
	"github.com/mdhender/mpg/internal/pipeline"
	"github.com/mdhender/mpg/internal/seed"
	"github.com/mdhender/mpg/world"
)

// generate runs the whole pipeline on a small world and returns its
// context.
func generate(t *testing.T, s uint64, aspect, preset string, land int) *pipeline.Context {
	t.Helper()
	c := config.Default()
	c.Seed = config.Seed(s)
	c.World.Aspect = aspect
	c.Layout.Preset = preset
	c.World.LandCells = land
	c.Rim.FalloffCells = 4
	ctx, err := pipeline.NewContext(c, t.TempDir(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := pipeline.Run(ctx, pipeline.Stages(), -1)
	if err != nil {
		t.Fatal(err)
	}
	if res.NotImplemented != nil || ctx.Products.World == nil {
		t.Fatalf("run stopped at %v without a world", res.NotImplemented)
	}
	return ctx
}

var worlds = []struct {
	seed           uint64
	aspect, preset string
	land           int
}{
	{3, "cinematic", "archipelago", 600},
	{7, "square", "continents", 400},
	{42, "cinematic", "islands", 600},
}

// TestWorlds checks small generated worlds: the file on disk is the
// product's bytes, it validates, decodes and re-encodes to the same bytes,
// and its contents follow the export rules.
func TestWorlds(t *testing.T) {
	seamCoasts, fourWay := 0, 0
	for _, tc := range worlds {
		ctx := generate(t, tc.seed, tc.aspect, tc.preset, tc.land)
		w, b := ctx.Products.World, ctx.Products.WorldBytes
		disk, err := os.ReadFile(filepath.Join(ctx.OutputDir, world.File))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(disk, b) {
			t.Error("world.json on disk differs from the product's bytes")
		}
		if err := world.Validate(w); err != nil {
			t.Fatal(err)
		}
		back, err := world.DecodeBytes(b)
		if err != nil {
			t.Fatal(err)
		}
		if again, err := back.Bytes(); err != nil || !bytes.Equal(again, b) {
			t.Errorf("decode and re-encode changed the bytes (%v)", err)
		}
		m, sl := ctx.Products.Mesh, ctx.Products.SeaLevel
		if len(w.Cells) != len(m.Cells) || len(w.Corners) != len(m.Corners) || len(w.Edges) != len(m.Edges) {
			t.Fatal("world and mesh sizes differ")
		}
		if w.Outcomes.LandCells != sl.LandCells || w.Outcomes.SeaLevelM != sl.Level || len(w.Outcomes.Trace) != len(sl.Trace) {
			t.Error("outcomes differ from the sea level stage")
		}
		if want := []string{"land-target", "rivers", "measures"}; !slices.Equal(w.Outcomes.Deferred, want) {
			t.Errorf("deferred %q, want %q", w.Outcomes.Deferred, want)
		}
		hash, _ := ctx.Config.Hash()
		if w.Meta.ConfigHash != hash || w.Meta.WidthKm != ctx.Config.World.WidthKm {
			t.Error("meta does not match the config")
		}
		// Edge seeds are the edge-noise stream's values in edge id order.
		r := seed.Rand(uint64(ctx.Config.Seed), export.EdgeNoiseStream, export.EdgeNoiseVersion)
		for i := range w.Edges {
			if v := r.Uint32(); w.Edges[i].Seed != v {
				t.Fatalf("edge %d seed %d, want %d", i, w.Edges[i].Seed, v)
			}
		}
		// Geometry is on the precision grid.
		for i := range w.Corners {
			p := w.Corners[i].Point
			for _, v := range []float64{p.X, p.Y} {
				if v != 0 && v != w.Meta.HeightKm && math.Abs(v*1e6-math.Round(v*1e6)) > 1e-6 {
					t.Fatalf("corner %d coordinate %v is not on the 1e-6 km grid", i, v)
				}
			}
		}
		// Every playable cell's exits match the edge stage's half-edges.
		for i, hs := range ctx.Products.Edges.Cells {
			for k, h := range hs {
				x := w.Cells[i].Exits[k]
				if x.Edge != h.Edge || x.Neighbor != h.Neighbor || x.InclinePermille != int(h.Incline) || string(x.Direction) != h.Direction.String() {
					t.Fatalf("cell %d exit %d %+v, half-edge %+v", i, k, x, h)
				}
			}
		}
		if len(w.Coastlines) == 0 {
			t.Error("no coastlines")
		}
		for _, cl := range w.Coastlines {
			if !cl.Closed {
				t.Errorf("open coastline from edge %d", cl.Edges[0])
			}
			for k := range cl.Edges {
				p, q := w.Corners[cl.Corners[k]].Point, w.Corners[cl.Corners[k+1]].Point
				if math.Abs(p.X-q.X) > w.Meta.WidthKm/2 {
					seamCoasts++
				}
			}
		}
		for i := range w.Corners {
			k := &w.Corners[i]
			n := 0
			for _, e := range k.Edges {
				if w.Edges[e].Coast {
					n++
				}
			}
			if n == 4 {
				fourWay++
			}
		}
	}
	// The worlds exercise a coastline across the seam and corners where
	// land and water alternate (four coast edges: the 4-way corner of a
	// collapsed short edge), where the chain must hug its own land.
	if seamCoasts == 0 {
		t.Error("no coastline crosses the seam in the test worlds")
	}
	if fourWay == 0 {
		t.Error("no corner with four coast edges in the test worlds")
	}
	t.Logf("%d coast edges across the seam, %d corners with four coast edges", seamCoasts, fourWay)
}

func TestDeterminism(t *testing.T) {
	a := generate(t, 7, "square", "continents", 400).Products.WorldBytes
	b := generate(t, 7, "square", "continents", 400).Products.WorldBytes
	if !bytes.Equal(a, b) {
		t.Error("two runs of the same config gave different world.json")
	}
	c := generate(t, 8, "square", "continents", 400).Products.WorldBytes
	if bytes.Equal(a, c) {
		t.Error("two seeds gave the same world.json")
	}
}

// TestValidateCorrupt corrupts a valid world one way at a time and checks
// that Validate reports each.
func TestValidateCorrupt(t *testing.T) {
	good := generate(t, 3, "cinematic", "archipelago", 600).Products.WorldBytes
	fresh := func() *world.World {
		w, err := world.DecodeBytes(good)
		if err != nil {
			t.Fatal(err)
		}
		return w
	}
	g := fresh()
	land, ocean, rim := -1, -1, -1
	for i := range g.Cells {
		c := &g.Cells[i]
		switch {
		case c.HasFlag(world.FlagRim) && rim < 0:
			rim = i
		case c.Landform.IsLand() && land < 0 && len(c.Exits) >= 3:
			land = i
		case c.Water == world.Ocean && ocean < 0:
			ocean = i
		}
	}
	coast := slices.IndexFunc(g.Edges, func(e world.Edge) bool { return e.Coast })
	inner := slices.IndexFunc(g.Edges, func(e world.Edge) bool { return e.Passable && e.InclinePermille != 0 })
	boundary := slices.IndexFunc(g.Edges, func(e world.Edge) bool { return e.OnBoundary() })
	terminal := slices.IndexFunc(g.Corners, func(k world.Corner) bool { return k.HasFlag(world.FlagTerminal) })
	for _, tc := range []struct {
		name string
		f    func(w *world.World)
		msg  string
	}{
		{"schema", func(w *world.World) { w.Schema = 3 }, "schema 3"},
		{"seed", func(w *world.World) { w.Meta.Seed = "007" }, "meta.seed"},
		{"config hash", func(w *world.World) { w.Meta.ConfigHash = "abc" }, "config_hash"},
		{"units", func(w *world.World) { w.Meta.Units.Length = "mi" }, "meta.units"},
		{"codebook", func(w *world.World) { w.Codebooks.Landforms = w.Codebooks.Landforms[1:] }, "codebooks"},
		{"cell id", func(w *world.World) { w.Cells[5].ID = 6 }, "cells[5].id"},
		{"corner out of range", func(w *world.World) { w.Cells[land].Corners[0] = len(w.Corners) }, "out of range"},
		{"landform", func(w *world.World) { w.Cells[land].Landform = "swamp" }, "not in the codebook"},
		{"land with depth", func(w *world.World) { w.Cells[land].Depth = world.Deep }, "with depth"},
		{"water kind", func(w *world.World) { w.Cells[ocean].Water = world.Lake }, "water kind"},
		{"altitude", func(w *world.World) { w.Cells[land].AltitudeM = math.NaN() }, "altitude"},
		{"rim not impassable", func(w *world.World) { w.Cells[rim].Flags = []world.CellFlag{world.FlagRim} }, "impassable"},
		{"flag order", func(w *world.World) { slices.Reverse(w.Cells[rim].Flags) }, "codebook order"},
		{"volcano at sea", func(w *world.World) { w.Cells[ocean].Flags = append(w.Cells[ocean].Flags, world.FlagVolcano) }, "volcano on"},
		{"coast flag", func(w *world.World) {
			c := &w.Cells[w.Edges[coast].Cells[0]]
			c.Flags = slices.DeleteFunc(c.Flags, func(f world.CellFlag) bool { return f == world.FlagCoast })
		}, "coast flag"},
		{"site off map", func(w *world.World) { w.Cells[land].Site.X = w.Meta.WidthKm }, "off the map"},
		{"counterclockwise", func(w *world.World) {
			c := &w.Cells[land]
			slices.Reverse(c.Polygon)
		}, "does not reach corner"},
		{"offset", func(w *world.World) { w.Cells[land].Polygon[1].X += 0.01 }, "does not reach corner"},
		{"bbox", func(w *world.World) { w.Cells[land].BBox.Max.Y -= 0.5 }, "bbox"},
		{"side", func(w *world.World) { s := w.Cells[land].Sides; s[0], s[1] = s[1], s[0] }, "side"},
		{"duplicate direction", func(w *world.World) { x := w.Cells[land].Exits; x[1].Direction = x[0].Direction }, "clockwise order"},
		{"exit edge", func(w *world.World) { x := w.Cells[land].Exits; x[0].Edge = x[1].Edge }, "which joins"},
		{"missing exit", func(w *world.World) { w.Cells[land].Exits = w.Cells[land].Exits[1:] }, "does not list"},
		{"exit incline", func(w *world.World) { w.Cells[land].Exits[0].InclinePermille += 3 }, "incline"},
		{"edge incline", func(w *world.World) { w.Edges[inner].InclinePermille = -w.Edges[inner].InclinePermille }, "incline"},
		{"bearing", func(w *world.World) { x := &w.Cells[land].Exits[0]; x.BearingDeg = math.Mod(x.BearingDeg+30, 360) }, "bearing"},
		{"direction error", func(w *world.World) { w.Cells[land].Exits[0].ErrorDeg += 1 }, "error"},
		{"corner height", func(w *world.World) { w.Corners[terminal].HeightM += 1 }, "mean altitude"},
		{"terminal flag", func(w *world.World) { w.Corners[terminal].Flags = nil }, "terminal flag"},
		{"corner cells", func(w *world.World) { w.Corners[terminal].Cells = w.Corners[terminal].Cells[1:] }, "polygons through it"},
		{"edge length", func(w *world.World) { w.Edges[inner].LengthKm += 0.001 }, "length"},
		{"passable rim", func(w *world.World) { w.Edges[boundary].Passable = true }, "rim boundary"},
		{"impassable", func(w *world.World) { w.Edges[inner].Passable = false }, "passable"},
		{"coast", func(w *world.World) { w.Edges[coast].Coast = false; w.Edges[coast].Water = "" }, "coast"},
		{"river at sea", func(w *world.World) { w.Edges[coast].River = world.Stream }, "river"},
		{"coastline gap", func(w *world.World) { cl := &w.Coastlines[0]; cl.Edges = cl.Edges[1:]; cl.Corners = cl.Corners[1:] }, "coastline"},
		{"coastline reversed", func(w *world.World) {
			cl := &w.Coastlines[0]
			slices.Reverse(cl.Edges)
			slices.Reverse(cl.Corners)
		}, "water on its right"},
		{"land count", func(w *world.World) { w.Outcomes.LandCells-- }, "outcomes"},
		{"met", func(w *world.World) { w.Outcomes.Met = !w.Outcomes.Met }, "met"},
		{"sea level", func(w *world.World) { w.Outcomes.SeaLevelM = -1e5 }, "above the sea level"},
		{"reason", func(w *world.World) { w.Outcomes.Reason = "luck" }, "reason"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := fresh()
			tc.f(w)
			err := world.Validate(w)
			if _, ok := errors.AsType[*world.Invalid](err); !ok {
				t.Fatalf("Validate = %v, want an *Invalid", err)
			}
			if !strings.Contains(err.Error(), tc.msg) {
				t.Errorf("Validate = %v\nwant a problem mentioning %q", err, tc.msg)
			}
		})
	}
}

// TestNoFusedMultiplyAdd compiles this package for the architectures whose Go
// compilers fuse a*b+c into one instruction and fails if any fused
// multiply-add or multiply-subtract appears in its code.
func TestNoFusedMultiplyAdd(t *testing.T) {
	noFMA(t, "github.com/mdhender/mpg/internal/export")
}

func noFMA(t *testing.T, pkg string) {
	if testing.Short() {
		t.Skip("compiles for several architectures")
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool not found")
	}
	fusedOp := regexp.MustCompile(`\tV?FN?M(ADD|SUB)`)
	for _, tg := range []struct{ arch, env string }{{"arm64", ""}, {"amd64", "GOAMD64=v3"}} {
		t.Run(tg.arch, func(t *testing.T) {
			cmd := exec.Command(goTool, "build", "-gcflags="+pkg+"=-S", pkg)
			cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+tg.arch, "CGO_ENABLED=0")
			if tg.env != "" {
				cmd.Env = append(cmd.Env, tg.env)
			}
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("go build %s: %v\n%s", pkg, err, out)
			}
			if m := fusedOp.FindAllString(string(out), -1); len(m) != 0 {
				t.Errorf("%s: %d fused multiply-add instructions in %s", tg.arch, len(m), pkg)
			}
		})
	}
}
