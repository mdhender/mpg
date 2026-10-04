// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package mesh

import (
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/topo"
)

// TestLloydCVFalls is S16's "done when": the cell-area coefficient of
// variation is reported and falls with more passes. On real default-config
// meshes it must fall strictly from pass to pass, 0 through 4, and every
// relaxed mesh must keep the S15 invariants.
func TestLloydCVFalls(t *testing.T) {
	const passes = 4
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
			cyl := cylinderOf(t, c)
			sites, err := Sites(c, cyl)
			if err != nil {
				t.Fatal(err)
			}
			relaxed, cv, err := Lloyd(cyl, sites, passes)
			if err != nil {
				t.Fatal(err)
			}
			if len(cv) != passes {
				t.Fatalf("%d CVs for %d passes", len(cv), passes)
			}
			m, err := Build(cyl, relaxed)
			if err != nil {
				t.Fatal(err)
			}
			checkMesh(t, m)
			st := m.Stats(c.Province.AreaKm2, c.Mesh.MinEdgeKm)
			cvs := append(slices.Clone(cv), st.AreaCV)
			t.Logf("area CV by pass 0..%d: %.4f", passes, cvs)
			for k := 1; k < len(cvs); k++ {
				if !(cvs[k] < cvs[k-1]) {
					t.Errorf("area CV after %d passes %.5f, not below %.5f after %d", k, cvs[k], cvs[k-1], k-1)
				}
			}
			// The CV Lloyd reports for the unrelaxed sites is the
			// unrelaxed mesh's.
			m0, err := Build(cyl, sites)
			if err != nil {
				t.Fatal(err)
			}
			if got := m0.Stats(c.Province.AreaKm2, 0).AreaCV; math.Abs(got-cv[0]) > 1e-9 {
				t.Errorf("pass 0 CV %v, but the unrelaxed mesh's is %v", cv[0], got)
			}
		})
	}
}

// TestLloydPasses checks that passes compose: two passes are one pass run
// twice, and zero passes return a copy of the sites.
func TestLloydPasses(t *testing.T) {
	c := resolved(t, 5, "cinematic", 300)
	cyl := cylinderOf(t, c)
	sites, err := Sites(c, cyl)
	if err != nil {
		t.Fatal(err)
	}
	same, cv, err := Lloyd(cyl, sites, 0)
	if err != nil || len(cv) != 0 || !slices.Equal(same, sites) {
		t.Fatalf("0 passes: %d CVs, err %v, equal %v", len(cv), err, slices.Equal(same, sites))
	}
	same[0].X = -1
	if sites[0].X == -1 {
		t.Error("0 passes returned the input slice, not a copy")
	}
	two, cv2, err := Lloyd(cyl, sites, 2)
	if err != nil {
		t.Fatal(err)
	}
	one, cvA, err := Lloyd(cyl, sites, 1)
	if err != nil {
		t.Fatal(err)
	}
	again, cvB, err := Lloyd(cyl, one, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(two, again) || !slices.Equal(cv2, append(cvA, cvB...)) {
		t.Error("two passes differ from one pass run twice")
	}
	if slices.Equal(one, sites) {
		t.Error("a pass did not move the sites")
	}
	// New applies the configured passes, then the collapse.
	m, err := Build(cyl, two)
	if err != nil {
		t.Fatal(err)
	}
	if m, err = Collapse(m, c.Mesh.MinEdgeKm, c.Mesh.DegreeCap); err != nil {
		t.Fatal(err)
	}
	a, _ := m.AppendBinary(nil)
	b, _ := build(t, c).AppendBinary(nil)
	if c.Mesh.LloydPasses != 2 || !slices.Equal(a, b) {
		t.Errorf("New (lloyd_passes %d) differs from Collapse(Build) after 2 passes", c.Mesh.LloydPasses)
	}
}

// TestLloydSeamFixture relaxes the seam fixture: the relaxed mesh keeps every
// invariant and is still the Voronoi diagram of its sites, and rotating the
// fixture around the cylinder before relaxing gives the same graph.
func TestLloydSeamFixture(t *testing.T) {
	cyl := cylinder(t, 12, 10, 1)
	relax := func(sites []topo.Point) *Mesh {
		t.Helper()
		r, cv, err := Lloyd(cyl, sites, 3)
		if err != nil {
			t.Fatal(err)
		}
		m, err := Build(cyl, r)
		if err != nil {
			t.Fatal(err)
		}
		m.LloydCV = cv
		return m
	}
	base := relax(seamSites)
	checkMesh(t, base)
	checkVoronoi(t, base, 1e-9)
	if st := base.Stats(1, 0); !(st.AreaCV < base.LloydCV[0]) {
		t.Errorf("relaxed fixture CV %v, not below the unrelaxed %v", st.AreaCV, base.LloydCV[0])
	}
	for k := 1; k < 24; k++ {
		shift := float64(k) * cyl.W() / 24
		sites := make([]topo.Point, len(seamSites))
		for i, s := range seamSites {
			sites[i] = topo.Point{X: cyl.WrapX(s.X + shift), Y: s.Y}
		}
		m := relax(sites)
		checkMesh(t, m)
		for i := range m.Cells {
			if !slices.Equal(m.Cells[i].Neighbors, base.Cells[i].Neighbors) {
				t.Fatalf("shift %v: cell %d neighbors %v, want %v", shift, i, m.Cells[i].Neighbors, base.Cells[i].Neighbors)
			}
			if a, b := m.Area(i), base.Area(i); math.Abs(a-b) > 1e-9 {
				t.Fatalf("shift %v: cell %d area %v, want %v", shift, i, a, b)
			}
			if d := cyl.Distance(m.Cells[i].Site, topo.Point{X: cyl.WrapX(base.Cells[i].Site.X + shift), Y: base.Cells[i].Site.Y}); d > 1e-9 {
				t.Fatalf("shift %v: relaxed site %d is %v km from the shifted base site", shift, i, d)
			}
		}
	}
}

// TestMeanArea checks the area report on default-config meshes: the cells
// tile the fixed W × H cylinder, so the areas sum to W·H, the mean is W·H/n,
// and with n = round(W·H/A) it is within 1/(2n) of A, relative.
func TestMeanArea(t *testing.T) {
	for _, tc := range []struct {
		seed   uint64
		aspect string
	}{{42, "cinematic"}, {7, "portrait"}} {
		c := resolved(t, tc.seed, tc.aspect, 0)
		m := build(t, c)
		w, h, a := c.World.WidthKm, c.World.HeightKm, c.Province.AreaKm2
		n := float64(len(m.Cells))
		st := m.Stats(a, c.Mesh.MinEdgeKm)
		if rel := math.Abs(st.AreaMean-w*h/n) / (w * h / n); rel > 1e-9 {
			t.Errorf("%s: mean area %v, want W·H/n = %v", tc.aspect, st.AreaMean, w*h/n)
		}
		if math.Abs(st.AreaDev) > 1/(2*n)+1e-9 {
			t.Errorf("%s: mean area deviates %v from A, more than 1/(2n) = %v", tc.aspect, st.AreaDev, 1/(2*n))
		}
		if math.Abs(st.AreaDev-(st.AreaMean/a-1)) > 1e-15 || st.AreaMinA != st.AreaMin/a || st.AreaMaxA != st.AreaMax/a {
			t.Errorf("%s: inconsistent area report %+v", tc.aspect, st)
		}
		if len(m.LloydCV) != c.Mesh.LloydPasses {
			t.Errorf("%s: %d Lloyd CVs for %d passes", tc.aspect, len(m.LloydCV), c.Mesh.LloydPasses)
		}
		t.Logf("%s: n %d, mean %.4f km² (A %.4f, deviation %+.2e), CV %.4f, range %.3f A to %.3f A, %d edges below %.2f km (shortest %.3g)",
			tc.aspect, len(m.Cells), st.AreaMean, a, st.AreaDev, st.AreaCV, st.AreaMinA, st.AreaMaxA, st.ShortEdges, c.Mesh.MinEdgeKm, st.EdgeMin)
	}
}

func TestLloydErrors(t *testing.T) {
	cyl := cylinder(t, 12, 10, 1)
	for _, tc := range []struct {
		name   string
		sites  []topo.Point
		passes int
		want   string
	}{
		{"negative passes", seamSites, -1, "-1 Lloyd passes"},
		{"duplicate", []topo.Point{{X: 1, Y: 5}, {X: 3, Y: 2}, {X: 1, Y: 5}}, 1, "sites 0 and 2 coincide"},
		{"duplicate, 0 passes", []topo.Point{{X: 1, Y: 5}, {X: 3, Y: 2}, {X: 1, Y: 5}}, 0, "sites 0 and 2 coincide"},
		{"NaN", []topo.Point{{X: math.NaN(), Y: 5}}, 1, "outside"},
	} {
		_, _, err := Lloyd(cyl, tc.sites, tc.passes)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err %v, want one containing %q", tc.name, err, tc.want)
		}
	}
}

// cylinderOf returns c's cylinder, failing the test on an error.
func cylinderOf(t testing.TB, c config.Config) topo.Cylinder {
	t.Helper()
	cyl, err := topo.New(c.World.WidthKm, c.World.HeightKm, c.Rim.Km, c.Rim.FalloffKm)
	if err != nil {
		t.Fatal(err)
	}
	return cyl
}
