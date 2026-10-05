// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package mesh

import (
	"fmt"
	"image"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/mdhender/mpg/internal/elevation"
	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/render"
	"github.com/mdhender/mpg/internal/topo"
)

// rimSites is the rim fixture on a 12 × 10 km cylinder: five rows of six
// sites, 2 km apart, at y = 1, 3, 5, 7, 9, the rows staggered by 1 km, so
// every corner off the boundary is 3-way. Row r holds cells 6r to 6r + 5.
func rimSites() []topo.Point {
	var sites []topo.Point
	for r := range 5 {
		for k := range 6 {
			sites = append(sites, topo.Point{X: float64(2*k + (r+1)%2), Y: float64(2*r + 1)})
		}
	}
	return sites
}

// fixtureLimits are limits every rim fixture mesh meets: A = 120/30 km².
var fixtureLimits = Limits{AreaKm2: 4, MinEdgeKm: 0.5, AreaMin: 0.5, AreaMax: 1.6, DegreeCap: 8}

// TestRimFixture is S18's rim fixture: on a hand-built cylinder whose rim
// rows are known, exactly the cells of those rows are flagged Rim and
// Impassable; no playable cell touches y = 0 or y = H; every boundary edge
// belongs to a rim cell; every edge of a rim cell is impassable and every
// edge between playable cells passable; and the checks pass, with the rim
// depth the number of rim rows.
func TestRimFixture(t *testing.T) {
	for _, tc := range []struct {
		rim      float64
		rimRows  []int // rows whose cells are rim
		depth    int
		playable int
	}{
		{rim: 2, rimRows: []int{0, 4}, depth: 1, playable: 18},
		{rim: 4, rimRows: []int{0, 1, 3, 4}, depth: 2, playable: 6},
	} {
		t.Run(fmt.Sprintf("rim%v", tc.rim), func(t *testing.T) {
			m, err := Build(cylinder(t, 12, 10, tc.rim), rimSites())
			if err != nil {
				t.Fatal(err)
			}
			checkMesh(t, m)
			h := m.Cylinder().H()
			for i, c := range m.Cells {
				want := slices.Contains(tc.rimRows, i/6)
				if c.Rim != want || c.Impassable != want || m.InRim(i) != want {
					t.Errorf("cell %d (row %d): Rim %v, Impassable %v, InRim %v; want %v", i, i/6, c.Rim, c.Impassable, m.InRim(i), want)
				}
				if c.Rim {
					continue
				}
				for _, k := range c.Corners {
					if y := m.Corners[k].Point.Y; y == 0 || y == h {
						t.Errorf("playable cell %d has corner %d on y = %v", i, k, y)
					}
				}
			}
			boundary := 0
			for e, edge := range m.Edges {
				a, b := edge.Cells[0], edge.Cells[1]
				if edge.OnBoundary() {
					boundary++
					if !m.Cells[a].Rim {
						t.Errorf("boundary edge %d belongs to playable cell %d", e, a)
					}
				}
				rim := m.Cells[a].Rim || b == Boundary || m.Cells[b].Rim
				if m.Passable(e) == rim {
					t.Errorf("edge %d between cells %d and %d: Passable %v", e, a, b, m.Passable(e))
				}
			}
			if boundary != 12 {
				t.Errorf("%d boundary edges, want 12 (6 cells on each edge)", boundary)
			}
			r := m.Check(fixtureLimits)
			if err := r.Err(); err != nil {
				t.Fatal(err)
			}
			if r.PlayableCells != tc.playable || r.RimCells != 30-tc.playable {
				t.Errorf("%d playable and %d rim cells, want %d and %d", r.PlayableCells, r.RimCells, tc.playable, 30-tc.playable)
			}
			if r.RimDepthMin != tc.depth || r.RimDepthMax != tc.depth {
				t.Errorf("rim depth %d to %d, want %d", r.RimDepthMin, r.RimDepthMax, tc.depth)
			}
			if r.CornerCells[3] != len(m.Corners)-r.BoundaryCorners || r.BoundaryCornerCells[2] != r.BoundaryCorners {
				t.Errorf("corner histograms %v, on the edge %v; want every corner 3-way, 2-way on the edge", r.CornerCells, r.BoundaryCornerCells)
			}
		})
	}
}

// TestRimSeal checks that the rim-seal check catches a rim too thin to keep
// the playable cells off the map edge: with a 0.5 km rim no site is in it,
// so the rows on y = 1 and y = 9 are playable and touch the boundary.
func TestRimSeal(t *testing.T) {
	m, err := Build(cylinder(t, 12, 10, 0.5), rimSites())
	if err != nil {
		t.Fatal(err)
	}
	r := m.Check(fixtureLimits)
	seal := result(t, r, CheckRimSeal)
	if seal.Bad != 12 || !slices.Equal(seal.First, []int{0, 1, 2, 3, 4}) {
		t.Errorf("rim-seal: %d bad, first %v; want 12, first [0 1 2 3 4]", seal.Bad, seal.First)
	}
	if r.OK() || r.RimCells != 0 || r.RimDepthMin != 0 {
		t.Errorf("OK %v, %d rim cells, depth %d; want a failure, none, 0", r.OK(), r.RimCells, r.RimDepthMin)
	}
	for _, c := range r.Checks {
		if c.Name != CheckRimSeal && !c.Pass() {
			t.Errorf("check %s failed too: %d bad", c.Name, c.Bad)
		}
	}
	if err := r.Err(); err == nil || !strings.Contains(err.Error(), "rim-seal") {
		t.Errorf("Err %v, want one naming rim-seal", err)
	}
}

// TestCheckFailures checks that each limit check reports its offenders.
func TestCheckFailures(t *testing.T) {
	m, err := Build(cylinder(t, 12, 10, 2), rimSites())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		edit  func(l *Limits, m *Mesh)
		check string
		bad   int
	}{
		{"edge", func(l *Limits, _ *Mesh) { l.MinEdgeKm = 100 }, CheckEdgeLength, len(m.Edges)},
		{"area", func(l *Limits, _ *Mesh) { l.AreaMax = 0.9 * l.AreaMin }, CheckArea, len(m.Cells)},
		{"cap", func(l *Limits, _ *Mesh) { l.DegreeCap = 2 }, CheckNeighbors, len(m.Cells)},
		{"flag", func(_ *Limits, m *Mesh) { m.Cells[3].Impassable = false }, CheckRimFlags, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := fixtureLimits
			mm := *m
			mm.Cells = slices.Clone(m.Cells)
			tc.edit(&l, &mm)
			r := mm.Check(l)
			c := result(t, r, tc.check)
			if c.Bad != tc.bad || len(c.First) != min(tc.bad, firstIDs) || !slices.IsSorted(c.First) {
				t.Errorf("%s: %d bad, first %v; want %d", tc.check, c.Bad, c.First, tc.bad)
			}
			if err := r.Err(); err == nil || !strings.Contains(err.Error(), tc.check) {
				t.Errorf("Err %v, want one naming %s", err, tc.check)
			}
		})
	}
}

// result returns r's result for the named check.
func result(t *testing.T, r Report, name string) CheckResult {
	t.Helper()
	k := slices.IndexFunc(r.Checks, func(c CheckResult) bool { return c.Name == name })
	if k < 0 {
		t.Fatalf("no check %s", name)
	}
	return r.Checks[k]
}

// TestCheckSweep is S18's "done when" on real meshes: the mesh checks pass
// across a seed sweep, 8 seeds × 2 aspects at 2,000 land cells, and, unless
// -short, the default worlds of seed 42 cinematic and seed 7 square. The
// rim is at least two cells deep everywhere (the default rim is 4 cells).
func TestCheckSweep(t *testing.T) {
	type world struct {
		seed   uint64
		aspect string
		land   int
	}
	var worlds []world
	for s := uint64(1); s <= 8; s++ {
		for _, a := range []string{"cinematic", "square"} {
			worlds = append(worlds, world{s, a, 2_000})
		}
	}
	if !testing.Short() {
		worlds = append(worlds, world{42, "cinematic", 0}, world{7, "square", 0})
	}
	for _, w := range worlds {
		t.Run(fmt.Sprintf("seed%d-%s-%d", w.seed, w.aspect, w.land), func(t *testing.T) {
			t.Parallel()
			c := resolved(t, w.seed, w.aspect, w.land)
			r := build(t, c).Check(LimitsOf(c))
			if err := r.Err(); err != nil {
				t.Fatal(err)
			}
			if r.RimDepthMin < 2 {
				t.Errorf("rim only %d cells deep in places", r.RimDepthMin)
			}
			t.Log("\n" + strings.Join(r.Lines()[len(r.Checks):], "\n"))
		})
	}
}

// TestRenders checks the mesh renders: deterministic pixels at
// StageRender's size; in the stage render the rim cells drawn as ice; in
// the short-edge render a red mark at every stretched edge.
func TestRenders(t *testing.T) {
	c := resolved(t, 3, "cinematic", 600)
	m := build(t, c)
	if len(m.Stretched) == 0 {
		t.Fatal("the fixture world has no stretched edge")
	}
	f, err := field.FromConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	f.SetFunc(func(_, _ int, p topo.Point) float64 { return 2000 * math.Sin(p.X/50) * math.Sin(p.Y/40) })
	base := elevation.Render(f, 0)
	s := RenderScale(f, m)
	size := image.Rect(0, 0, f.NX()*s, f.NY()*s)
	cv, _ := newCanvas(f, m)
	renders := map[string]func() *image.RGBA{
		"stage": func() *image.RGBA { return StageRender(base, f, m) },
		"area":  func() *image.RGBA { return AreaRender(f, m, c.Province.AreaKm2) },
		"short": func() *image.RGBA { return ShortRender(base, f, m) },
	}
	for _, name := range []string{"stage", "area", "short"} {
		a, b := renders[name](), renders[name]()
		if a.Rect != size || render.PixelHash(a) != render.PixelHash(b) {
			t.Errorf("%s render: size %v (want %v), or two renders differ", name, a.Rect, size)
		}
	}
	stage := StageRender(base, f, m)
	for i, cell := range m.Cells {
		p := cv.px(cell.Site)
		got := stage.RGBAAt(int(p.X), int(p.Y))
		if cell.Rim && got != iceFill && got != iceEdge {
			t.Fatalf("rim cell %d's site pixel is %v, want ice", i, got)
		}
	}
	short := ShortRender(base, f, m)
	for _, e := range m.Stretched {
		p := cv.px(m.edgeMidpoint(e))
		r := int(cv.cellPx() / 2)
		if got := short.RGBAAt(int(p.X)+r, int(p.Y)); got != stretchInk {
			t.Errorf("stretched edge %d: pixel beside its midpoint is %v, want the red mark", e, got)
		}
	}
}
