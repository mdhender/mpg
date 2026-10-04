// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package mesh

import (
	"bytes"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/mdhender/mpg/internal/topo"
)

// hexLattice returns a triangular lattice of sites, spacing s km, in rows
// rows of cols sites, odd rows offset by s/2, and the cylinder it tiles
// exactly (rim 3 km). Its Voronoi cells are regular hexagons, clipped at
// y = 0 and y = H, with every corner 3-way and every interior edge s/√3 km.
// lattice(r, c) is the index of the site in row r, column c.
func hexLattice(t *testing.T, cols, rows int, s float64) (cyl topo.Cylinder, sites []topo.Point) {
	t.Helper()
	dy := s * math.Sqrt(3) / 2
	for r := range rows {
		for c := range cols {
			sites = append(sites, topo.Point{X: s*float64(c) + s/2*float64(r%2) + s/4, Y: dy * (float64(r) + 0.5)})
		}
	}
	return cylinder(t, s*float64(cols), dy*float64(rows), 3), sites
}

// shortEdges returns the ids of m's edges shorter than km.
func shortEdges(m *Mesh, km float64) []int {
	var out []int
	for e := range m.Edges {
		if m.EdgeLength(e) < km {
			out = append(out, e)
		}
	}
	return out
}

// collapse returns Collapse(m, minKm, cap), failing the test on an error,
// and checks that m was not modified.
func collapse(t *testing.T, m *Mesh, minKm float64, cap int) *Mesh {
	t.Helper()
	before, _ := m.AppendBinary(nil)
	c, err := Collapse(m, minKm, cap)
	if err != nil {
		t.Fatal(err)
	}
	if after, _ := m.AppendBinary(nil); !bytes.Equal(before, after) {
		t.Fatal("Collapse modified its input")
	}
	return c
}

// singleFixture is a 10 × 12 hexagonal lattice (spacing 6 km) with the two
// sites across the edge between cells 54 and 55 pulled toward it until the
// four sites are nearly on one circle: that edge is 0.75 km, and every other
// edge at least 2.5 km.
func singleFixture(t *testing.T, shift float64) (topo.Cylinder, []topo.Point) {
	cyl, sites := hexLattice(t, 10, 12, 6)
	a := sites[54]
	sites[45].Y = a.Y - 3.4 // row 4, column 5: across the edge to the north
	sites[65].Y = a.Y + 3.4 // row 6, column 5: to the south
	for i := range sites {
		sites[i].X = cyl.WrapX(sites[i].X + shift)
	}
	return cyl, sites
}

// TestCollapseFixture is S17's "done when" fixture: a single short edge
// collapses into a 4-way corner at its midpoint, and its two cells, which
// now touch only there, are no longer neighbors. Rotating the fixture
// around the cylinder, so the seam cuts through the collapse, changes
// nothing.
func TestCollapseFixture(t *testing.T) {
	const minKm = 1.5
	cyl, sites := singleFixture(t, 0)
	m, err := Build(cyl, sites)
	if err != nil {
		t.Fatal(err)
	}
	short := shortEdges(m, 2.5)
	if len(short) != 1 || m.Edges[short[0]].Cells != [2]int{54, 55} {
		t.Fatalf("edges under 2.5 km: %v; want only the edge between cells 54 and 55", short)
	}
	e := m.Edges[short[0]]
	pa, pb := m.Corners[e.Corners[0]].Point, m.Corners[e.Corners[1]].Point
	dx, dy := cyl.Delta(pa, pb)
	mid := topo.Point{X: cyl.WrapX(pa.X + dx/2), Y: pa.Y + dy/2}

	c := collapse(t, m, minKm, 8)
	checkMesh(t, c)
	if c.Collapses != 1 || c.Stretches != 0 || c.DegreeCapHits != 0 || len(c.Stretched) != 0 {
		t.Errorf("%d collapses, %d stretches, %d cap hits, stretched %v; want 1, 0, 0, none", c.Collapses, c.Stretches, c.DegreeCapHits, c.Stretched)
	}
	if len(c.Corners) != len(m.Corners)-1 || len(c.Edges) != len(m.Edges)-1 {
		t.Errorf("%d corners, %d edges; want one fewer of each than %d, %d", len(c.Corners), len(c.Edges), len(m.Corners), len(m.Edges))
	}
	if slices.Contains(c.Cells[54].Neighbors, 55) || slices.Contains(c.Cells[55].Neighbors, 54) {
		t.Error("cells 54 and 55 touch only at a point but are still neighbors")
	}
	var four []int
	for k, corner := range c.Corners {
		if len(corner.Cells) == 4 {
			four = append(four, k)
		}
	}
	if len(four) != 1 {
		t.Fatalf("4-way corners %v, want one", four)
	}
	k := c.Corners[four[0]]
	if k.Point != mid || !slices.Equal(k.Cells, []int{45, 54, 55, 65}) {
		t.Errorf("4-way corner at %v touching %v; want the midpoint %v, touching 45, 54, 55, 65", k.Point, k.Cells, mid)
	}
	// Every other corner is where it was.
	for _, corner := range m.Corners {
		if corner.Point == pa || corner.Point == pb {
			continue
		}
		if !slices.ContainsFunc(c.Corners, func(x Corner) bool { return x.Point == corner.Point }) {
			t.Fatalf("corner at %v moved", corner.Point)
		}
	}
	if want := cyl.Distance(pa, pb) / 2; math.Abs(c.MaxShiftKm-want) > 1e-12 {
		t.Errorf("max shift %v km, want half the edge, %v", c.MaxShiftKm, want)
	}
	// The cells' sides: 54 and 55 each lose one; 45 and 65 each gain one
	// corner, but no side.
	for i, d := range map[int]int{54: -1, 55: -1, 45: 0, 65: 0} {
		if got, want := len(c.Cells[i].Corners), len(m.Cells[i].Corners)+d; got != want {
			t.Errorf("cell %d has %d sides, want %d", i, got, want)
		}
	}

	for s := 1; s < 24; s++ {
		shift := float64(s) * cyl.W() / 24
		cyl, sites := singleFixture(t, shift)
		ms, err := Build(cyl, sites)
		if err != nil {
			t.Fatal(err)
		}
		cs := collapse(t, ms, minKm, 8)
		checkMesh(t, cs)
		if cs.Collapses != 1 || cs.Stretches != 0 {
			t.Fatalf("shift %v: %d collapses, %d stretches", shift, cs.Collapses, cs.Stretches)
		}
		for i := range cs.Cells {
			if !slices.Equal(cs.Cells[i].Neighbors, c.Cells[i].Neighbors) {
				t.Fatalf("shift %v: cell %d neighbors %v, want %v", shift, i, cs.Cells[i].Neighbors, c.Cells[i].Neighbors)
			}
			if a, b := cs.Area(i), c.Area(i); math.Abs(a-b) > 1e-9 {
				t.Fatalf("shift %v: cell %d area %v, want %v", shift, i, a, b)
			}
		}
	}
}

// pentagonFixture is the lattice with the sites within 7 km of (31.5, 31)
// replaced by five sites nearly on a circle of radius 4.5 km: the middle of
// the pentagon has two short edges, 0.09 and 0.35 km, that share a corner;
// every other edge is at least 2.2 km.
func pentagonFixture(t *testing.T) (topo.Cylinder, []topo.Point) {
	cyl, lattice := hexLattice(t, 10, 12, 6)
	p0 := topo.Point{X: 31.5, Y: 31}
	var sites []topo.Point
	for _, s := range lattice {
		if math.Hypot(s.X-p0.X, s.Y-p0.Y) > 7 {
			sites = append(sites, s)
		}
	}
	for k, eps := range []float64{0, 0.02, -0.01, 0.03, -0.02} {
		a := (90 + 72*float64(k)) * math.Pi / 180
		r := 4.5 * (1 + eps)
		sites = append(sites, topo.Point{X: p0.X + r*math.Cos(a), Y: p0.Y + r*math.Sin(a)})
	}
	return cyl, sites
}

// TestStretchFixture checks the guard: of two short edges that share a
// corner, the shorter collapses into a 4-way corner; the other would make a
// 5-way corner, so it is stretched to the minimum instead, and its cells
// stay neighbors.
func TestStretchFixture(t *testing.T) {
	const minKm = 1
	cyl, sites := pentagonFixture(t)
	m, err := Build(cyl, sites)
	if err != nil {
		t.Fatal(err)
	}
	short := shortEdges(m, 2)
	if len(short) != 2 {
		t.Fatalf("edges under 2 km: %v, want two", short)
	}
	s0, s1 := m.Edges[short[0]], m.Edges[short[1]]
	if m.EdgeLength(short[0]) > m.EdgeLength(short[1]) {
		s0, s1 = s1, s0
	}
	shared := 0
	for _, a := range s0.Corners {
		if slices.Contains(s1.Corners[:], a) {
			shared++
		}
	}
	if shared != 1 {
		t.Fatalf("the short edges %v and %v share %d corners, want 1", s0.Corners, s1.Corners, shared)
	}

	c := collapse(t, m, minKm, 8)
	checkMesh(t, c)
	if c.Collapses != 1 || c.Stretches != 1 || len(c.Stretched) != 1 {
		t.Fatalf("%d collapses, %d stretches, stretched %v; want 1, 1, one edge", c.Collapses, c.Stretches, c.Stretched)
	}
	x, y := s0.Cells[0], s0.Cells[1]
	if slices.Contains(c.Cells[x].Neighbors, y) {
		t.Errorf("cells %d and %d, across the collapsed edge, are still neighbors", x, y)
	}
	st := c.Edges[c.Stretched[0]]
	if st.Cells != s1.Cells {
		t.Errorf("stretched edge between cells %v, want %v", st.Cells, s1.Cells)
	}
	if l := c.EdgeLength(c.Stretched[0]); !(l >= minKm && l <= minKm*(1+2*StretchMargin)) {
		t.Errorf("stretched edge is %v km, want just over %v", l, minKm)
	}
	if !slices.Contains(c.Cells[s1.Cells[0]].Neighbors, s1.Cells[1]) {
		t.Errorf("cells %v, across the stretched edge, are no longer neighbors", s1.Cells)
	}
	checkCollapsed(t, c, minKm, 8)
	// The stretch is along the perpendicular bisector of the two sites.
	a, b := c.Corners[st.Corners[0]].Point, c.Corners[st.Corners[1]].Point
	ex, ey := cyl.Delta(a, b)
	sx, sy := cyl.Delta(c.Cells[st.Cells[0]].Site, c.Cells[st.Cells[1]].Site)
	if dot := (ex*sx + ey*sy) / (math.Hypot(ex, ey) * math.Hypot(sx, sy)); math.Abs(dot) > 1e-9 {
		t.Errorf("stretched edge is not perpendicular to its sites (cosine %v)", dot)
	}
}

// capFixture is the lattice with the sites within 9 km of (31.5, 31)
// replaced by a site there and ten around it at 5 km: the middle cell has
// ten neighbors.
func capFixture(t *testing.T) (topo.Cylinder, []topo.Point, int) {
	cyl, lattice := hexLattice(t, 10, 12, 6)
	p0 := topo.Point{X: 31.5, Y: 31}
	var sites []topo.Point
	for _, s := range lattice {
		if math.Hypot(s.X-p0.X, s.Y-p0.Y) > 9 {
			sites = append(sites, s)
		}
	}
	mid := len(sites)
	sites = append(sites, p0)
	for k := range 10 {
		a := (36*float64(k) + 3*float64(k%3)) * math.Pi / 180
		sites = append(sites, topo.Point{X: p0.X + 5*math.Cos(a), Y: p0.Y + 5*math.Sin(a)})
	}
	return cyl, sites, mid
}

// TestDegreeCapFixture checks the degree cap: with no short edges to
// collapse, the cell with ten neighbors loses its shortest collapsible edges
// until it has the cap, each collapse a 4-way corner.
func TestDegreeCapFixture(t *testing.T) {
	cyl, sites, mid := capFixture(t)
	m, err := Build(cyl, sites)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(m.Cells[mid].Neighbors); n != 10 {
		t.Fatalf("the middle cell has %d neighbors, want 10", n)
	}
	for _, tc := range []struct{ cap, hits int }{{10, 0}, {9, 1}, {8, 2}, {7, 3}} {
		c := collapse(t, m, 0, tc.cap)
		checkMesh(t, c)
		if c.DegreeCapHits != tc.hits || c.Collapses != tc.hits || c.Stretches != 0 {
			t.Errorf("cap %d: %d cap hits, %d collapses, %d stretches; want %d, %d, 0", tc.cap, c.DegreeCapHits, c.Collapses, c.Stretches, tc.hits, tc.hits)
		}
		if n := len(c.Cells[mid].Neighbors); n != min(10, tc.cap) {
			t.Errorf("cap %d: the middle cell has %d neighbors", tc.cap, n)
		}
		st := c.Stats(1, 0)
		if st.NeighborMax > tc.cap || st.CornerCells[4] != tc.hits {
			t.Errorf("cap %d: most neighbors %d, %d 4-way corners; want ≤ %d, %d", tc.cap, st.NeighborMax, st.CornerCells[4], tc.cap, tc.hits)
		}
	}
}

func TestCollapseErrors(t *testing.T) {
	m := seamMesh(t)
	for _, tc := range []struct {
		min  float64
		cap  int
		want string
	}{
		{-1, 8, "minimum edge"},
		{math.NaN(), 8, "minimum edge"},
		{math.Inf(1), 8, "minimum edge"},
		{1, 2, "degree cap 2"},
	} {
		if _, err := Collapse(m, tc.min, tc.cap); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Collapse(%v, %d): err %v, want one containing %q", tc.min, tc.cap, err, tc.want)
		}
	}
}

// checkCollapsed checks the mesh checks the collapse must make achievable:
// no edge shorter than minKm; every cell off the rim with 3 to cap
// neighbors, and every rim cell (InRim) 1 to cap; every corner off the rim
// boundary touching 3 or 4 cells, and every corner on it 2 to 4.
func checkCollapsed(t *testing.T, m *Mesh, minKm float64, cap int) {
	t.Helper()
	for e := range m.Edges {
		if l := m.EdgeLength(e); l < minKm {
			t.Fatalf("edge %d is %v km, below %v", e, l, minKm)
		}
	}
	for i, c := range m.Cells {
		lo := 3
		if m.InRim(i) {
			lo = 1
		}
		if n := len(c.Neighbors); n < lo || n > cap {
			t.Fatalf("cell %d (rim %v) has %d neighbors, want %d to %d", i, m.InRim(i), n, lo, cap)
		}
	}
	for k, c := range m.Corners {
		lo := 3
		if c.Boundary {
			lo = 2
		}
		if n := len(c.Cells); n < lo || n > 4 {
			t.Fatalf("corner %d (boundary %v) touches %d cells, want %d to 4", k, c.Boundary, n, lo)
		}
	}
}

// TestCollapseRealMeshes is S17's "done when" on real meshes: after New, no
// edge is shorter than min_edge_km, every cell off the rim has 3 to 8
// neighbors (rim cells 1 to 8), every corner touches 3 or 4 cells (2 to 4
// on the rim boundary), and every cell's area is within the configured
// bounds. Collapsing again changes nothing.
func TestCollapseRealMeshes(t *testing.T) {
	type world struct {
		seed   uint64
		aspect string
		land   int
	}
	worlds := []world{{42, "cinematic", 0}, {7, "square", 0}, {1, "portrait", 2_000}, {3, "4:1", 600}}
	if !testing.Short() {
		for s := uint64(100); s < 112; s++ {
			worlds = append(worlds, world{s, []string{"cinematic", "square", "portrait", "widescreen"}[s%4], []int{2_000, 5_000, 10_000}[s%3]})
		}
	}
	for _, w := range worlds {
		t.Run(fmt.Sprintf("seed%d-%s-%d", w.seed, w.aspect, w.land), func(t *testing.T) {
			t.Parallel()
			c := resolved(t, w.seed, w.aspect, w.land)
			m := build(t, c)
			checkMesh(t, m)
			checkCollapsed(t, m, c.Mesh.MinEdgeKm, c.Mesh.DegreeCap)
			st := m.Stats(c.Province.AreaKm2, c.Mesh.MinEdgeKm)
			if st.ShortEdges != 0 || st.Collapses == 0 || st.CornerCells[4] == 0 {
				t.Errorf("%d short edges, %d collapses, %d 4-way corners", st.ShortEdges, st.Collapses, st.CornerCells[4])
			}
			if st.AreaMinA < c.Mesh.AreaMin || st.AreaMaxA > c.Mesh.AreaMax {
				t.Errorf("areas %.3f A to %.3f A, outside %v to %v", st.AreaMinA, st.AreaMaxA, c.Mesh.AreaMin, c.Mesh.AreaMax)
			}
			again := collapse(t, m, c.Mesh.MinEdgeKm, c.Mesh.DegreeCap)
			a, _ := m.AppendBinary(nil)
			b, _ := again.AppendBinary(nil)
			if !bytes.Equal(a, b) || again.Collapses != 0 || again.Stretches != 0 {
				t.Errorf("collapsing again changed the mesh (%d collapses, %d stretches)", again.Collapses, again.Stretches)
			}
			t.Logf("%d cells: %d collapses, %d stretches, %d cap hits, max shift %.2f km; corners 3/4 %d/%d; neighbors %d–%d (off the rim %d–%d); shortest edge %.3f km; areas %.3f–%.3f A",
				st.Cells, st.Collapses, st.Stretches, st.DegreeCapHits, st.MaxShiftKm, st.CornerCells[3], st.CornerCells[4],
				st.NeighborMin, st.NeighborMax, st.PlayableNeighborMin, st.PlayableNeighborMax, st.EdgeMin, st.AreaMinA, st.AreaMaxA)
		})
	}
}

// TestCollapseDisabled checks min_edge_fraction 0: nothing is collapsed or
// stretched, and New's mesh is the Voronoi graph.
func TestCollapseDisabled(t *testing.T) {
	c := resolved(t, 5, "cinematic", 300)
	c.Mesh.MinEdgeFraction = 0
	c.ClearDerived()
	if err := c.Resolve(); err != nil {
		t.Fatal(err)
	}
	m := build(t, c)
	if m.Collapses != 0 || m.Stretches != 0 {
		t.Errorf("%d collapses, %d stretches with the collapse disabled", m.Collapses, m.Stretches)
	}
	a, _ := m.AppendBinary(nil)
	b, _ := voronoiOf(t, c).AppendBinary(nil)
	if !bytes.Equal(a, b) {
		t.Error("with the collapse disabled, New differs from the Voronoi graph")
	}
}
