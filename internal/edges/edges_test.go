// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package edges

import (
	"math"
	"slices"
	"testing"

	"github.com/mdhender/mpg/internal/mesh"
	"github.com/mdhender/mpg/internal/topo"
)

func TestGrade(t *testing.T) {
	for _, tc := range []struct {
		from, to, dist float64
		want           Incline
	}{
		{0, 0, 9, 0},
		{100, 115, 1, 15},    // 15 m over 1 km: 1.5%
		{0, 1350, 9, 150},    // 1,350 m over 9 km: 15.0%
		{1350, 0, 9, -150},   // and back
		{0, 2000, 9, 222},    // the design's 2,000 m example: about 22%
		{0, 0.25, 0.1, 3},    // 2.5 tenths rounds away from zero
		{0, -0.25, 0.1, -3},  // symmetrically
		{0, 0.249, 0.1, 2},   //
		{0, -0.01, 1, 0},     // rounds to zero, never −0 (an integer)
		{0, 9000, 9, 1000},   // exactly at the cap
		{0, 50000, 9, 1000},  // capped
		{50000, 0, 9, -1000}, // capped below
		{-4000, 3000, 0.5, 1000},
	} {
		got := Grade(tc.from, tc.to, tc.dist)
		if got != tc.want {
			t.Errorf("Grade(%v, %v, %v) = %d, want %d", tc.from, tc.to, tc.dist, got, tc.want)
		}
		if back := Grade(tc.to, tc.from, tc.dist); back != -got {
			t.Errorf("Grade(%v, %v, %v) = %d, not the negation of %d", tc.to, tc.from, tc.dist, back, got)
		}
	}
	if s := Incline(150).String(); s != "+15.0%" {
		t.Errorf("String = %q", s)
	}
	if s := Incline(-3).String(); s != "-0.3%" {
		t.Errorf("String = %q", s)
	}
	if Incline(-1000).Abs() != MaxIncline || Incline(-1000).Percent() != -100 {
		t.Error("Abs or Percent")
	}
}

func TestNames(t *testing.T) {
	if Ocean.String() != "ocean" || InlandSea.String() != "inland-sea" || Lake.String() != "lake" || WaterNone.String() != "" {
		t.Error("water names")
	}
	if Stream.String() != "stream" || River.String() != "river" || MajorRiver.String() != "major-river" || RiverNone.String() != "" {
		t.Error("river names")
	}
	if Water(9).String() != "Water(9)" || RiverClass(9).String() != "RiverClass(9)" {
		t.Error("out-of-range names")
	}
}

// grid is a hand-made world: a square grid of 10 × 10 sites 10 km apart on
// a 100 × 100 km cylinder with a 10 km rim, so rows 0 and 9 are rim cells.
// Four sites share every Voronoi vertex, so the diagonal neighbors touch
// only at a point: each cell has exactly 4 neighbors, due N, E, S and W.
// Altitude rises 100 m per row southward; columns 0 to 2 are ocean.
type grid struct {
	m     *mesh.Mesh
	alt   []float64
	water []Water
}

func newGrid(t *testing.T) *grid {
	t.Helper()
	cyl, err := topo.New(100, 100, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	var sites []topo.Point
	for j := range 10 {
		for i := range 10 {
			sites = append(sites, topo.Point{X: 5 + 10*float64(i), Y: 5 + 10*float64(j)})
		}
	}
	m, err := mesh.Build(cyl, sites)
	if err != nil {
		t.Fatal(err)
	}
	g := &grid{m: m, alt: make([]float64, len(sites)), water: make([]Water, len(sites))}
	for k := range sites {
		i, j := k%10, k/10
		g.alt[k] = 100 * float64(j)
		if i < 3 {
			g.water[k] = Ocean
		}
	}
	return g
}

func TestBuildGrid(t *testing.T) {
	g := newGrid(t)
	d, err := Build(g.m, g.alt, g.water, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, c := range g.m.Cells {
		i, j := k%10, k/10
		hs := d.Cells[k]
		var dirs []Direction
		for _, h := range hs {
			dirs = append(dirs, h.Direction)
		}
		want := []Direction{N, E, S, W}
		switch j {
		case 0:
			want = []Direction{E, S, W} // the rim boundary has no half-edge
		case 9:
			want = []Direction{N, E, W}
		}
		if !slices.Equal(dirs, want) {
			t.Fatalf("cell %d (%d, %d): directions %v, want %v", k, i, j, dirs, want)
		}
		for _, h := range hs {
			ni, nj := h.Neighbor%10, h.Neighbor/10
			var wantN, wantG int
			switch h.Direction {
			case N:
				wantN, wantG = k-10, -10 // 100 m down over 10 km: −1.0%
			case S:
				wantN, wantG = k+10, 10
			case E:
				wantN = j*10 + (i+1)%10
			case W:
				wantN = j*10 + (i+9)%10 // across the seam from column 0
			}
			if h.Neighbor != wantN || int(h.Incline) != wantG {
				t.Errorf("cell %d %s: neighbor %d (%d, %d) incline %d, want %d, %d", k, h.Direction, h.Neighbor, ni, nj, h.Incline, wantN, wantG)
			}
			if h.Error > 1e-9 {
				t.Errorf("cell %d %s: bearing %v, error %v", k, h.Direction, h.Bearing, h.Error)
			}
			me := g.m.Edges[h.Edge]
			if me.Other(k) != h.Neighbor {
				t.Errorf("cell %d: half-edge edge %d does not join it to %d", k, h.Edge, h.Neighbor)
			}
			e := d.Edges[h.Edge]
			if e.Passable != (!c.Rim && !g.m.Cells[h.Neighbor].Rim) {
				t.Errorf("cell %d %s: passable %v", k, h.Direction, e.Passable)
			}
			// Coasts: between columns 2 and 3, and 9 and 0 across the
			// seam, off the rim.
			coast := !c.Rim && !g.m.Cells[h.Neighbor].Rim && (g.water[k] == Ocean) != (g.water[h.Neighbor] == Ocean)
			if e.Coast != coast || (coast && e.Water != Ocean) || (!coast && e.Water != WaterNone) {
				t.Errorf("cell %d %s: coast %v water %s, want %v", k, h.Direction, e.Coast, e.Water, coast)
			}
		}
	}
	coast := 0
	for e, me := range g.m.Edges {
		if me.OnBoundary() {
			if d.Edges[e].Passable || d.Edges[e].Coast || d.Edges[e].Incline != 0 {
				t.Errorf("boundary edge %d: %+v", e, d.Edges[e])
			}
		}
		if d.Edges[e].Coast {
			coast++
		}
	}
	if coast != 16 { // 8 playable rows × 2 shores
		t.Errorf("%d coast edges, want 16", coast)
	}
	st := Summarize(g.m, d, g.water)
	if st.Playable != 80 || st.LandCells != 56 || st.Neighbors[4] != 80 || st.Coast != 16 || st.ErrorMax > 1e-9 ||
		st.NotOpposite != 0 || st.NaiveCollisions != 0 || st.LandRim != 14 || st.Boundary != 20 {
		t.Errorf("stats %+v", st)
	}
	if h, ok := d.Toward(55, S); !ok || h.Neighbor != 65 {
		t.Errorf("Toward(55, S) = %+v, %v", h, ok)
	}
	if _, ok := d.Toward(5, N); ok {
		t.Error("a north rim cell has a half-edge north")
	}
}

func TestBuildErrors(t *testing.T) {
	g := newGrid(t)
	if _, err := Build(g.m, g.alt[1:], g.water, nil); err == nil {
		t.Error("Build accepted short altitudes")
	}
	if _, err := Build(g.m, g.alt, g.water, make([]RiverClass, 3)); err == nil {
		t.Error("Build accepted short river classes")
	}
	alt := slices.Clone(g.alt)
	alt[7] = math.NaN()
	if _, err := Build(g.m, alt, g.water, nil); err == nil {
		t.Error("Build accepted a NaN altitude")
	}
	water := slices.Clone(g.water)
	water[3] = numWaters
	if _, err := Build(g.m, g.alt, water, nil); err == nil {
		t.Error("Build accepted a bad water kind")
	}
	river := make([]RiverClass, len(g.m.Edges))
	river[0] = MajorRiver
	d, err := Build(g.m, g.alt, g.water, river)
	if err != nil || d.Edges[0].River != MajorRiver {
		t.Errorf("river class not carried: %v", err)
	}
}
