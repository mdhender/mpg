// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package mesh

import (
	"cmp"
	"math"
	"slices"
	"testing"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/topo"
)

// resolved returns the default config with the given seed, aspect, and land
// cell count (0 for the default), resolved. Below 1,000 land cells it
// narrows the falloff band so the world fits.
func resolved(t testing.TB, seed uint64, aspect string, landCells int) config.Config {
	t.Helper()
	c := config.Default()
	c.Seed = config.Seed(seed)
	c.World.Aspect = aspect
	if landCells > 0 {
		c.World.LandCells = landCells
	}
	if landCells > 0 && landCells < 1_000 {
		c.Rim.FalloffCells = 4 // the default 12 do not fit a world this small
	}
	if err := c.Resolve(); err != nil {
		t.Fatal(err)
	}
	return c
}

// build returns New(c), failing the test on an error.
func build(t testing.TB, c config.Config) *Mesh {
	t.Helper()
	m, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// voronoiOf returns c's mesh before the collapse: the Voronoi graph of its
// relaxed sites, as New builds it before calling Collapse.
func voronoiOf(t testing.TB, c config.Config) *Mesh {
	t.Helper()
	cyl := cylinderOf(t, c)
	sites, err := Sites(c, cyl)
	if err != nil {
		t.Fatal(err)
	}
	relaxed, cv, err := Lloyd(cyl, sites, c.Mesh.LloydPasses)
	if err != nil {
		t.Fatal(err)
	}
	m, err := Build(cyl, relaxed)
	if err != nil {
		t.Fatal(err)
	}
	m.LloydCV = cv
	return m
}

// cylinder returns topo.New's cylinder, failing the test on an error.
func cylinder(t testing.TB, w, h, rim float64) topo.Cylinder {
	t.Helper()
	c, err := topo.New(w, h, rim, 0)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// checkMesh checks every structural invariant of m: ranges and finiteness,
// canonical id order, each cell polygon closed and clockwise with positive
// area, each edge with exactly two distinct cells or one cell on the rim
// boundary, neighbor symmetry with a shared edge behind every neighbor
// relation, corner and edge back-references, the Euler characteristic of the
// annulus (V − E + F = 0), and the cell areas summing to W × H.
func checkMesh(t *testing.T, m *Mesh) {
	t.Helper()
	cyl := m.Cylinder()
	w, h := cyl.W(), cyl.H()
	fail := func(format string, args ...any) {
		t.Helper()
		t.Fatalf(format, args...)
	}

	// Corners: in range, finite, canonical order, boundary flag.
	for c, k := range m.Corners {
		p := k.Point
		if !(p.X >= 0 && p.X < w) || !(p.Y >= 0 && p.Y <= h) {
			fail("corner %d at (%v, %v) is outside [0, %v) × [0, %v]", c, p.X, p.Y, w, h)
		}
		if k.Boundary != (p.Y == 0 || p.Y == h) {
			fail("corner %d at y %v has Boundary %v", c, p.Y, k.Boundary)
		}
		if c > 0 {
			q := m.Corners[c-1].Point
			if cmp.Or(cmp.Compare(q.Y, p.Y), cmp.Compare(q.X, p.X)) >= 0 {
				fail("corners %d and %d are out of order: (%v, %v), (%v, %v)", c-1, c, q.X, q.Y, p.X, p.Y)
			}
		}
		if len(k.Cells) == 0 {
			fail("corner %d touches no cell", c)
		}
		if want := 3; !k.Boundary && len(k.Cells) < want {
			fail("interior corner %d touches %d cells, want at least %d", c, len(k.Cells), want)
		}
		if !slices.IsSorted(k.Cells) || !slices.IsSorted(k.Edges) {
			fail("corner %d lists are not sorted", c)
		}
		for _, i := range k.Cells {
			if !slices.Contains(m.Cells[i].Corners, c) {
				fail("corner %d lists cell %d, whose polygon does not pass through it", c, i)
			}
		}
		for _, e := range k.Edges {
			if !slices.Contains(m.Edges[e].Corners[:], c) {
				fail("corner %d lists edge %d, which does not end at it", c, e)
			}
		}
	}

	// Edges: two distinct cells, or one on the boundary; canonical order.
	for e, edge := range m.Edges {
		a, b := edge.Corners[0], edge.Corners[1]
		if a == b {
			fail("edge %d joins corner %d to itself", e, a)
		}
		if edge.OnBoundary() {
			ya, yb := m.Corners[a].Point.Y, m.Corners[b].Point.Y
			if ya != yb || (ya != 0 && ya != h) {
				fail("boundary edge %d runs from y %v to y %v, not along y = 0 or y = H", e, ya, yb)
			}
		} else if !(edge.Cells[0] >= 0 && edge.Cells[0] < edge.Cells[1]) {
			fail("edge %d has cells %v, want two distinct cells, lower first", e, edge.Cells)
		}
		if e > 0 {
			p := m.Edges[e-1].Corners
			pk := [2]int{min(p[0], p[1]), max(p[0], p[1])}
			k := [2]int{min(a, b), max(a, b)}
			if cmp.Or(cmp.Compare(pk[0], k[0]), cmp.Compare(pk[1], k[1])) >= 0 {
				fail("edges %d and %d are out of order", e-1, e)
			}
		}
		if !slices.Contains(m.Corners[a].Edges, e) || !slices.Contains(m.Corners[b].Edges, e) {
			fail("edge %d is missing from its corners' lists", e)
		}
		if l := m.EdgeLength(e); !(l > 0) || math.IsInf(l, 0) {
			fail("edge %d has length %v", e, l)
		}
	}

	// Cells: closed clockwise polygons; edges and neighbors consistent.
	seen := make([]int, len(m.Edges))
	var total float64
	for i, cell := range m.Cells {
		n := len(cell.Corners)
		if n < 3 || len(cell.Edges) != n {
			fail("cell %d has %d corners and %d edges", i, n, len(cell.Edges))
		}
		if !(cell.Site.X >= 0 && cell.Site.X < w && cell.Site.Y > 0 && cell.Site.Y < h) {
			fail("cell %d site (%v, %v) out of range", i, cell.Site.X, cell.Site.Y)
		}
		if cell.Corners[0] != slices.Min(cell.Corners) {
			fail("cell %d polygon does not start at its lowest corner", i)
		}
		var nbs []int
		for k, e := range cell.Edges {
			a, b := cell.Corners[k], cell.Corners[(k+1)%n]
			edge := m.Edges[e]
			seen[e]++
			switch i {
			case edge.Cells[0]:
				if edge.Corners != [2]int{a, b} {
					fail("cell %d side %d runs %d→%d but edge %d (its Cells[0] side) runs %v", i, k, a, b, e, edge.Corners)
				}
			case edge.Cells[1]:
				if edge.Corners != [2]int{b, a} {
					fail("cell %d side %d runs %d→%d but edge %d runs %v; want the reverse", i, k, a, b, e, edge.Corners)
				}
			default:
				fail("cell %d lists edge %d, whose cells are %v", i, e, edge.Cells)
			}
			if o := edge.Other(i); o != Boundary {
				nbs = append(nbs, o)
			}
		}
		slices.Sort(nbs)
		nbs = slices.Compact(nbs)
		if !slices.Equal(nbs, cell.Neighbors) {
			fail("cell %d neighbors %v, but its edges give %v", i, cell.Neighbors, nbs)
		}
		for _, j := range cell.Neighbors {
			if j == i || !slices.Contains(m.Cells[j].Neighbors, i) {
				fail("cell %d lists neighbor %d, which does not list it back", i, j)
			}
		}
		a := m.Area(i)
		if !(a > 0) || math.IsInf(a, 0) {
			fail("cell %d has area %v", i, a)
		}
		total += a
		g := m.Centroid(i)
		if !(g.X >= 0 && g.X < w && g.Y > 0 && g.Y < h) {
			fail("cell %d centroid (%v, %v) out of range", i, g.X, g.Y)
		}
	}
	for e, k := range seen {
		want := 2
		if m.Edges[e].OnBoundary() {
			want = 1
		}
		if k != want {
			fail("edge %d appears in %d cell polygons, want %d", e, k, want)
		}
	}
	if chi := len(m.Corners) - len(m.Edges) + len(m.Cells); chi != 0 {
		fail("V − E + F = %d, want 0 for the annulus", chi)
	}
	if rel := math.Abs(total-w*h) / (w * h); rel > 1e-9 {
		fail("cell areas sum to %v, want W × H = %v (relative error %g)", total, w*h, rel)
	}
}

// checkVoronoi checks m against the definition by brute force: every corner
// is equidistant (within tol) from the sites of the cells that meet there,
// and no site is nearer to it; every non-boundary edge's midpoint is
// equidistant from its two cells' sites, and no site is nearer. It is
// O(corners × sites), for small meshes.
func checkVoronoi(t *testing.T, m *Mesh, tol float64) {
	t.Helper()
	cyl := m.Cylinder()
	nearest := func(p topo.Point) float64 {
		d := math.Inf(1)
		for _, c := range m.Cells {
			d = min(d, cyl.Distance(p, c.Site))
		}
		return d
	}
	for c, k := range m.Corners {
		near := nearest(k.Point)
		for _, i := range k.Cells {
			if d := cyl.Distance(k.Point, m.Cells[i].Site); math.Abs(d-near) > tol {
				t.Fatalf("corner %d is %v km from cell %d's site, but the nearest site is %v km", c, d, i, near)
			}
		}
	}
	for e, edge := range m.Edges {
		if edge.OnBoundary() {
			continue
		}
		a, b := m.Corners[edge.Corners[0]].Point, m.Corners[edge.Corners[1]].Point
		dx, dy := cyl.Delta(a, b)
		mid := topo.Point{X: cyl.WrapX(a.X + dx/2), Y: a.Y + dy/2}
		near := nearest(mid)
		for _, i := range edge.Cells {
			if d := cyl.Distance(mid, m.Cells[i].Site); math.Abs(d-near) > tol {
				t.Fatalf("edge %d's midpoint is %v km from cell %d's site, but the nearest site is %v km", e, d, i, near)
			}
		}
	}
}
