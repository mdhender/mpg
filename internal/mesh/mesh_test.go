// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package mesh

import (
	"bytes"
	"math"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/mdhender/mpg/internal/seed"
	"github.com/mdhender/mpg/internal/topo"
)

// seamSites is the seam fixture on a 12 × 10 km cylinder (rim 1 km): site 0
// sits just east of x = 0, sites 1 and 2 just west of x = W, so cell 0
// spans the seam and borders cells 1 and 2 across it. The rest fill the
// cylinder.
var seamSites = []topo.Point{
	{X: 0.3, Y: 5.0},  // 0: spans x = 0
	{X: 11.2, Y: 3.6}, // 1: across the seam, north-west of 0
	{X: 11.0, Y: 6.6}, // 2: across the seam, south-west of 0
	{X: 1.8, Y: 3.2},  // 3
	{X: 1.9, Y: 6.9},  // 4
	{X: 4.5, Y: 2.0},  // 5
	{X: 4.0, Y: 5.2},  // 6
	{X: 4.6, Y: 8.3},  // 7
	{X: 7.0, Y: 1.5},  // 8
	{X: 7.2, Y: 4.8},  // 9
	{X: 6.8, Y: 8.0},  // 10
	{X: 9.3, Y: 2.2},  // 11
	{X: 9.0, Y: 5.1},  // 12
	{X: 9.2, Y: 8.6},  // 13
	{X: 0.5, Y: 1.0},  // 14
	{X: 0.4, Y: 9.0},  // 15
	{X: 2.9, Y: 0.8},  // 16
	{X: 11.6, Y: 0.6}, // 17: rim cell across the seam from 14
}

func seamMesh(t *testing.T) *Mesh {
	t.Helper()
	m, err := Build(cylinder(t, 12, 10, 1), seamSites)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// TestSeamFixture is S15's "done when": the cell spanning x = 0 has
// consistent neighbors, and every edge has exactly two cells or the rim
// boundary (checkMesh). The cell's polygon closes, runs clockwise with
// positive area when unwrapped about its site, and reaches both sides of
// the seam; each of its neighbor relations is a shared edge, symmetric.
func TestSeamFixture(t *testing.T) {
	m := seamMesh(t)
	checkMesh(t, m)
	checkVoronoi(t, m, 1e-9)

	c := m.Cells[0]
	poly := m.Polygon(0)
	minX := slices.MinFunc(poly, func(a, b topo.Point) int { return cmpF(a.X, b.X) }).X
	maxX := slices.MaxFunc(poly, func(a, b topo.Point) int { return cmpF(a.X, b.X) }).X
	if !(minX < 0 && maxX > 0) {
		t.Fatalf("cell 0's unwrapped polygon spans x %v to %v; want it across x = 0", minX, maxX)
	}
	// Its corners west of the seam are stored wrapped, near x = W.
	west := 0
	for _, id := range c.Corners {
		if x := m.Corners[id].Point.X; x > 6 {
			west++
		}
	}
	if west == 0 {
		t.Error("no corner of cell 0 is stored on the west side of the seam (x near W)")
	}
	for _, j := range []int{1, 2} {
		if !slices.Contains(c.Neighbors, j) || !slices.Contains(m.Cells[j].Neighbors, 0) {
			t.Errorf("cells 0 and %d are not neighbors across the seam: %v, %v", j, c.Neighbors, m.Cells[j].Neighbors)
		}
	}
	// Each neighbor is behind exactly one of cell 0's edges, and that edge
	// lists cell 0 and the neighbor.
	for _, j := range c.Neighbors {
		n := 0
		for _, e := range c.Edges {
			if m.Edges[e].Other(0) == j {
				n++
				if !slices.Contains(m.Cells[j].Edges, e) {
					t.Errorf("edge %d between cells 0 and %d is missing from cell %d", e, j, j)
				}
			}
		}
		if n != 1 {
			t.Errorf("cells 0 and %d share %d edges, want 1", j, n)
		}
	}
	if a := m.Area(0); !(a > 1) {
		t.Errorf("cell 0 area %v", a)
	}
	g := m.Centroid(0)
	if !(g.X < 1 || g.X > 11) {
		t.Errorf("cell 0 centroid x %v, want near the seam", g.X)
	}

	// Rim cells 14 and 17 meet across the seam and both reach y = 0.
	if !slices.Contains(m.Cells[14].Neighbors, 17) {
		t.Errorf("rim cells 14 and 17 are not neighbors across the seam")
	}
	for _, i := range []int{14, 16, 17, 15} {
		if !hasBoundaryEdge(m, i) {
			t.Errorf("rim cell %d has no rim boundary edge", i)
		}
	}
	if hasBoundaryEdge(m, 0) {
		t.Error("cell 0 has a rim boundary edge")
	}
}

func hasBoundaryEdge(m *Mesh, i int) bool {
	for _, e := range m.Cells[i].Edges {
		if m.Edges[e].OnBoundary() {
			return true
		}
	}
	return false
}

func cmpF(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// TestSeamShift rotates the fixture around the cylinder so the seam cuts
// through every part of it in turn. The graph must not care where the seam
// is: the same neighbor sets and, within rounding, the same cell areas.
func TestSeamShift(t *testing.T) {
	base := seamMesh(t)
	cyl := base.Cylinder()
	for k := 1; k < 48; k++ {
		shift := float64(k) * cyl.W() / 48
		sites := make([]topo.Point, len(seamSites))
		for i, s := range seamSites {
			sites[i] = topo.Point{X: cyl.WrapX(s.X + shift), Y: s.Y}
		}
		m, err := Build(cyl, sites)
		if err != nil {
			t.Fatalf("shift %v: %v", shift, err)
		}
		checkMesh(t, m)
		for i := range m.Cells {
			if !slices.Equal(m.Cells[i].Neighbors, base.Cells[i].Neighbors) {
				t.Fatalf("shift %v: cell %d neighbors %v, want %v", shift, i, m.Cells[i].Neighbors, base.Cells[i].Neighbors)
			}
			if a, b := m.Area(i), base.Area(i); math.Abs(a-b) > 1e-9 {
				t.Fatalf("shift %v: cell %d area %v, want %v", shift, i, a, b)
			}
		}
		if len(m.Corners) != len(base.Corners) || len(m.Edges) != len(base.Edges) {
			t.Fatalf("shift %v: %d corners, %d edges; want %d, %d", shift, len(m.Corners), len(m.Edges), len(base.Corners), len(base.Edges))
		}
	}
}

// TestRealMeshes checks the invariants on default-config meshes across
// seeds and aspects: every edge two cells or the rim boundary, neighbor
// symmetry, corners touching cells, areas summing to W × H.
func TestRealMeshes(t *testing.T) {
	for _, tc := range []struct {
		seed   uint64
		aspect string
		land   int
	}{
		{42, "cinematic", 0},
		{7, "square", 0},
		{1, "portrait", 2_000},
		{3, "4:1", 600},
	} {
		t.Run(tc.aspect, func(t *testing.T) {
			t.Parallel()
			c := resolved(t, tc.seed, tc.aspect, tc.land)
			m := build(t, c)
			checkMesh(t, m)
			if want := SiteCount(c.World.WidthKm, c.World.HeightKm, c.Province.AreaKm2); len(m.Cells) != want {
				t.Errorf("%d cells, want %d", len(m.Cells), want)
			}
			// Seam cells exist and are whole: some cell's polygon crosses
			// x = 0 or x = W.
			crossing := 0
			for i := range m.Cells {
				for _, p := range m.Polygon(i) {
					if p.X < 0 || p.X >= c.World.WidthKm {
						crossing++
						break
					}
				}
			}
			if crossing == 0 {
				t.Error("no cell spans the seam")
			}
			t.Logf("%d cells, %d corners, %d edges; %d span the seam; ghost band %.1f km",
				len(m.Cells), len(m.Corners), len(m.Edges), crossing, m.GhostMarginKm)
		})
	}
}

// TestBruteForce checks a small real mesh against the Voronoi definition.
func TestBruteForce(t *testing.T) {
	m := build(t, resolved(t, 5, "cinematic", 300))
	checkMesh(t, m)
	checkVoronoi(t, m, 1e-6)
}

// TestRegularGrid builds the degenerate jitter-0 grid, where four sites
// share every circle: corners are 4-way, and the cells that touch only
// there are not neighbors.
func TestRegularGrid(t *testing.T) {
	cyl := cylinder(t, 120, 60, 5)
	sites := JitteredGrid(cyl, 72, 0, seed.Rand(0, Stage, "test"))
	m, err := Build(cyl, sites)
	if err != nil {
		t.Fatal(err)
	}
	checkMesh(t, m)
	checkVoronoi(t, m, 1e-9)
	for c, k := range m.Corners {
		if want := 4; !k.Boundary && len(k.Cells) != want {
			t.Errorf("corner %d touches %d cells, want %d", c, len(k.Cells), want)
		}
	}
	for i, c := range m.Cells {
		if want := 4; len(c.Neighbors) != want && !hasBoundaryEdge(m, i) {
			t.Errorf("cell %d has %d neighbors, want %d", i, len(c.Neighbors), want)
		}
	}
}

func TestDeterminism(t *testing.T) {
	enc := func(m *Mesh) []byte {
		b, err := m.AppendBinary(nil)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	a := enc(build(t, resolved(t, 42, "cinematic", 2_000)))
	b := enc(build(t, resolved(t, 42, "cinematic", 2_000)))
	if !bytes.Equal(a, b) {
		t.Error("the same seed built different meshes")
	}
	if c := enc(build(t, resolved(t, 43, "cinematic", 2_000))); bytes.Equal(a, c) {
		t.Error("seeds 42 and 43 built the same mesh")
	}
}

func TestJitteredGrid(t *testing.T) {
	cyl := cylinder(t, 2536, 1133, 36)
	for _, n := range []int{1, 2, 3, 7, 100, 999, 35591} {
		for _, jitter := range []float64{0, 0.8, math.Nextafter(1, 0)} {
			sites := JitteredGrid(cyl, n, jitter, seed.Rand(1, Stage, Version))
			if len(sites) != n {
				t.Fatalf("n %d: %d sites", n, len(sites))
			}
			for i, s := range sites {
				if !(s.X >= 0 && s.X < cyl.W() && s.Y > 0 && s.Y < cyl.H()) {
					t.Fatalf("n %d jitter %v: site %d at (%v, %v) out of range", n, jitter, i, s.X, s.Y)
				}
			}
			if err := checkSites(cyl, sites); err != nil {
				t.Fatalf("n %d jitter %v: %v", n, jitter, err)
			}
		}
	}
	// Near-square boxes: the default world's rows are about √A apart.
	sites := JitteredGrid(cyl, 35591, 0, seed.Rand(1, Stage, Version))
	var ys []float64
	for _, s := range sites {
		ys = append(ys, s.Y)
	}
	rows := len(slices.Compact(ys))
	if dy := cyl.H() / float64(rows); math.Abs(dy-8.99) > 0.1 {
		t.Errorf("%d rows, %v km apart; want about √A = 8.99", rows, dy)
	}
}

func TestBuildErrors(t *testing.T) {
	cyl := cylinder(t, 12, 10, 1)
	for _, tc := range []struct {
		name  string
		sites []topo.Point
		want  string
	}{
		{"none", nil, "no sites"},
		{"x = W", []topo.Point{{X: 12, Y: 5}}, "outside"},
		{"y = 0", []topo.Point{{X: 1, Y: 0}}, "outside"},
		{"NaN", []topo.Point{{X: math.NaN(), Y: 5}}, "outside"},
		{"duplicate", []topo.Point{{X: 1, Y: 5}, {X: 3, Y: 2}, {X: 1, Y: 5}}, "sites 0 and 2 coincide"},
		{"too narrow", []topo.Point{{X: 1, Y: 5}, {X: 3, Y: 2}}, "too"},
	} {
		_, err := Build(cyl, tc.sites)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err %v, want one containing %q", tc.name, err, tc.want)
		}
	}
}

// TestNoFusedMultiplyAdd compiles this package for the architectures whose Go
// backends contract a*b + c and checks the code holds no fused instruction.
func TestNoFusedMultiplyAdd(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles for several architectures")
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool not found")
	}
	fusedOp := regexp.MustCompile(`\tV?FN?M(ADD|SUB)`)
	const pkg = "github.com/mdhender/mpg/internal/mesh"
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

// TestGhostBand checks the ghost band's certificate: a band too narrow to
// prove the seam cells is rejected, and every accepted band, up to full
// ghost copies (margin W), gives the same graph.
func TestGhostBand(t *testing.T) {
	cyl := cylinder(t, 12, 10, 1)
	if _, ok, err := sweep(cyl, seamSites, 0.05); err != nil || ok {
		t.Errorf("a 0.05 km band: ok %v, err %v; want it rejected", ok, err)
	}
	base := seamMesh(t)
	want, _ := base.AppendBinary(nil)
	for _, margin := range []float64{6, 8, 12} {
		rings, ok, err := sweep(cyl, seamSites, margin)
		if err != nil || !ok {
			t.Fatalf("margin %v: ok %v, err %v", margin, ok, err)
		}
		m, err := assemble(cyl, seamSites, rings)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := m.AppendBinary(nil)
		if !bytes.Equal(got, want) {
			t.Errorf("margin %v gives a different graph from Build's %v", margin, base.GhostMarginKm)
		}
	}

	// A world with cells far wider than the mean site spacing needs the
	// band widened: three sites in a row on a 300 × 4 km strip have cells
	// about 100 km wide, against a first band of 4 × √(1200/3) = 80 km.
	wide := cylinder(t, 300, 4, 0.5)
	sites := []topo.Point{{X: 10, Y: 1.5}, {X: 110, Y: 2.5}, {X: 210, Y: 2}}
	m, err := Build(wide, sites)
	if err != nil {
		t.Fatal(err)
	}
	checkMesh(t, m)
	checkVoronoi(t, m, 1e-9)
	if first := ghostMarginCells * math.Sqrt(300*4/3.0); !(m.GhostMarginKm > first) {
		t.Errorf("ghost band %v km, want it widened past the first %v km", m.GhostMarginKm, first)
	}
}
