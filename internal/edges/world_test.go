// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package edges_test

import (
	"bytes"
	"github.com/mdhender/mpg/internal/basin"
	"image"
	"slices"
	"testing"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/edges"
	"github.com/mdhender/mpg/internal/mesh"
	"github.com/mdhender/mpg/internal/pipeline"
)

// world runs the pipeline through the edge stage on a small world and
// returns its context.
func world(t *testing.T, seed uint64, aspect, preset string, land int) *pipeline.Context {
	t.Helper()
	c := config.Default()
	c.Seed = config.Seed(seed)
	c.World.Aspect = aspect
	c.Layout.Preset = preset
	c.World.LandCells = land
	if land < 1000 {
		c.Rim.FalloffCells = 4
	}
	ctx, err := pipeline.NewContext(c, t.TempDir(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	stages := pipeline.Stages()
	last, err := pipeline.Lookup(stages, "edges")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.Run(ctx, stages, last); err != nil {
		t.Fatal(err)
	}
	if ctx.Products.Edges == nil {
		t.Fatal("no edge data")
	}
	return ctx
}

// waterOf returns the cells' water kinds as the edge stage sets them: the
// land-target stage's ocean, lakes and inland seas.
func waterOf(ctx *pipeline.Context) []edges.Water {
	t := ctx.Products.Target
	water := make([]edges.Water, len(ctx.Products.Mesh.Cells))
	for i, o := range t.Flood.Ocean {
		switch k := t.Lakes.Lake[i]; {
		case o:
			water[i] = edges.Ocean
		case k != basin.None && t.Lakes.Lakes[k].Kind == basin.KindInlandSea:
			water[i] = edges.InlandSea
		case k != basin.None:
			water[i] = edges.Lake
		}
	}
	return water
}

// TestWorlds checks the edge data of real meshes: every cell's half-edges
// match its neighbors, with unique directions in compass order whose
// bearings run clockwise, and each the brute-force assignment's choice;
// incline A→B is exactly −(B→A) for every edge and equals Grade; coast and
// passability follow their rules.
func TestWorlds(t *testing.T) {
	for _, tc := range []struct {
		seed           uint64
		aspect, preset string
		land           int
	}{
		{1, "cinematic", "continents", 600},
		{2, "square", "archipelago", 600},
		{3, "cinematic", "islands", 900},
		{42, "square", "pangaea", 2000},
	} {
		ctx := world(t, tc.seed, tc.aspect, tc.preset, tc.land)
		m, d := ctx.Products.Mesh, ctx.Products.Edges
		alt := ctx.Products.Cells.Altitude
		water := waterOf(ctx)
		cyl := m.Cylinder()
		coast := 0
		for i, c := range m.Cells {
			hs := d.Cells[i]
			if len(hs) != len(c.Neighbors) || len(hs) > edges.MaxDegree {
				t.Fatalf("seed %d cell %d: %d half-edges for %d neighbors", tc.seed, i, len(hs), len(c.Neighbors))
			}
			var nb []int
			var bearings []float64
			var dirs []edges.Direction
			for k, h := range hs {
				nb = append(nb, h.Neighbor)
				bearings = append(bearings, h.Bearing)
				dirs = append(dirs, h.Direction)
				if k > 0 && h.Direction <= hs[k-1].Direction {
					t.Fatalf("seed %d cell %d: directions %v not unique and in compass order", tc.seed, i, dirs)
				}
				if h.Bearing != cyl.Bearing(c.Site, m.Cells[h.Neighbor].Site) || h.Error != edges.AngularError(h.Bearing, h.Direction) {
					t.Fatalf("seed %d cell %d: bearing or error", tc.seed, i)
				}
			}
			slices.Sort(nb)
			if !slices.Equal(nb, c.Neighbors) {
				t.Fatalf("seed %d cell %d: half-edge neighbors %v, cell neighbors %v", tc.seed, i, nb, c.Neighbors)
			}
			// Clockwise: in list order the bearings rise, except for at
			// most one step down where the list passes north.
			down := 0
			for k := range bearings {
				if bearings[(k+1)%len(bearings)] < bearings[k] {
					down++
				}
			}
			if len(bearings) > 1 && down != 1 {
				t.Fatalf("seed %d cell %d: bearings %v (directions %v) are not clockwise", tc.seed, i, bearings, dirs)
			}
			want, err := edges.Assign(bearings)
			if err != nil || !slices.Equal(want, dirs) {
				t.Fatalf("seed %d cell %d: directions %v, Assign %v (%v)", tc.seed, i, dirs, want, err)
			}
		}
		for e, me := range m.Edges {
			ed := d.Edges[e]
			if ed.Passable != m.Passable(e) {
				t.Errorf("seed %d edge %d: passable %v", tc.seed, e, ed.Passable)
			}
			if ed.Coast {
				coast++
			}
			if me.OnBoundary() {
				if ed.Passable || ed.Coast || ed.Incline != 0 {
					t.Errorf("seed %d boundary edge %d: %+v", tc.seed, e, ed)
				}
				continue
			}
			a, b := me.Cells[0], me.Cells[1]
			ha, oka := half(d, a, e)
			hb, okb := half(d, b, e)
			if !oka || !okb || ha.Neighbor != b || hb.Neighbor != a {
				t.Fatalf("seed %d edge %d: half-edges missing", tc.seed, e)
			}
			if ha.Incline != -hb.Incline || ha.Incline != ed.Incline || ed.Incline.Abs() > edges.MaxIncline {
				t.Fatalf("seed %d edge %d: incline %d / %d / %d", tc.seed, e, ed.Incline, ha.Incline, hb.Incline)
			}
			if g := edges.Grade(alt[a], alt[b], cyl.Distance(m.Cells[a].Site, m.Cells[b].Site)); g != ed.Incline {
				t.Fatalf("seed %d edge %d: incline %d, Grade %d", tc.seed, e, ed.Incline, g)
			}
			rim := m.Cells[a].Rim || m.Cells[b].Rim
			wantCoast := !rim && (water[a] == edges.WaterNone) != (water[b] == edges.WaterNone)
			wantWater := edges.WaterNone
			if wantCoast {
				wantWater = max(water[a], water[b])
			}
			if ed.Coast != wantCoast || ed.Water != wantWater {
				t.Errorf("seed %d edge %d: coast %v %s, want %v", tc.seed, e, ed.Coast, ed.Water, wantCoast)
			}
			if ed.River != ctx.Products.Network.Class[e] {
				t.Errorf("seed %d edge %d: river %v, the river stage's %v", tc.seed, e, ed.River, ctx.Products.Network.Class[e])
			}
			if ed.River != edges.RiverNone && (rim || ed.Coast || water[a] != edges.WaterNone || water[b] != edges.WaterNone) {
				t.Errorf("seed %d edge %d: river %v not land–land", tc.seed, e, ed.River)
			}
		}
		st := ctx.Products.EdgeStats
		if st == nil || st.Coast != coast || st.LandRim != 0 || st.HalfEdges == 0 {
			t.Errorf("seed %d: stats %+v", tc.seed, st)
		}
		t.Logf("seed %d %s %s: %v", tc.seed, tc.aspect, tc.preset, st.Lines())
	}
}

func half(d *edges.Data, c, e int) (edges.HalfEdge, bool) {
	for _, h := range d.Cells[c] {
		if h.Edge == e {
			return h, true
		}
	}
	return edges.HalfEdge{}, false
}

// TestDeterminism builds the edge data twice from the same inputs, and
// checks the encoding and the renders are identical.
func TestDeterminism(t *testing.T) {
	ctx := world(t, 7, "cinematic", "continents", 600)
	m, alt, water := ctx.Products.Mesh, ctx.Products.Cells.Altitude, waterOf(ctx)
	a, err := ctx.Products.Edges.AppendBinary(nil)
	if err != nil {
		t.Fatal(err)
	}
	d, err := edges.Build(m, alt, water, ctx.Products.Network.Class)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := d.AppendBinary(nil)
	if !bytes.Equal(a, b) {
		t.Error("edge data differs between builds")
	}
	f := ctx.Products.Elevation
	for name, draw := range map[string]func() *image.RGBA{
		"incline":     func() *image.RGBA { return edges.InclineRender(f, m, d, water) },
		"passability": func() *image.RGBA { return edges.PassabilityRender(f, m, d, water) },
		"compass":     func() *image.RGBA { return edges.CompassRender(m, d, water) },
	} {
		x, y := draw(), draw()
		if !bytes.Equal(x.Pix, y.Pix) || x.Rect.Empty() {
			t.Errorf("%s render differs between calls or is empty", name)
		}
		if name != "compass" {
			s := mesh.RenderScale(f, m)
			if x.Rect.Dx() != f.NX()*s || x.Rect.Dy() != f.NY()*s {
				t.Errorf("%s render is %v", name, x.Rect)
			}
		}
	}
}
