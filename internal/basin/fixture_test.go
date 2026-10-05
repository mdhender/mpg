// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package basin

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/mdhender/mpg/internal/mesh"
	"github.com/mdhender/mpg/internal/topo"
)

// hexDX is the site spacing of the hand-built fixtures in km.
const hexDX = 10.0

// grid is a hand-built fixture: a hexagonal lattice of rows × cols cells
// (odd rows shifted half a cell east), whose top and bottom rows are rim
// cells, with each cell's altitude given in the rows of a text map. Cell
// (r, c) has id r·cols + c. In "odd-r" offset coordinates a cell's
// neighbors are (r, c ± 1) and, in rows r ± 1, columns c − 1 and c for an
// even r, c and c + 1 for an odd r; columns wrap east-west.
type grid struct {
	m          *mesh.Mesh
	rows, cols int
	alt        []float64
	seed       []bool
}

// newGrid builds the lattice and reads its altitudes from text: one line per
// row, north first, the cells' altitudes in meters separated by spaces;
// "~" marks a rim cell (altitude 0 and a seed).
func newGrid(t *testing.T, text string) *grid {
	t.Helper()
	var lines [][]string
	for l := range strings.SplitSeq(strings.TrimSpace(text), "\n") {
		lines = append(lines, strings.Fields(l))
	}
	rows, cols := len(lines), len(lines[0])
	dy := hexDX * math.Sqrt(3) / 2
	cyl, err := topo.New(float64(cols)*hexDX, float64(rows)*dy, dy, 0)
	if err != nil {
		t.Fatal(err)
	}
	g := &grid{rows: rows, cols: cols}
	var sites []topo.Point
	for r, l := range lines {
		if len(l) != cols {
			t.Fatalf("row %d has %d cells, want %d", r, len(l), cols)
		}
		for c, v := range l {
			x := (float64(c) + 0.25 + 0.5*float64(r%2)) * hexDX
			sites = append(sites, topo.Point{X: x, Y: (float64(r) + 0.5) * dy})
			if v == "~" {
				g.alt = append(g.alt, 0)
				g.seed = append(g.seed, true)
				continue
			}
			a, err := strconv.ParseFloat(v, 64)
			if err != nil {
				t.Fatal(err)
			}
			g.alt = append(g.alt, a)
			g.seed = append(g.seed, false)
		}
	}
	if g.m, err = mesh.Build(cyl, sites); err != nil {
		t.Fatal(err)
	}
	for i, c := range g.m.Cells {
		if c.Rim != g.seed[i] {
			t.Fatalf("cell %d: rim %v, want %v", i, c.Rim, g.seed[i])
		}
	}
	return g
}

// id returns cell (r, c)'s id.
func (g *grid) id(r, c int) int { return r*g.cols + c }

// find runs Find on the fixture, failing the test on an error.
func (g *grid) find(t *testing.T, minDepth float64) *Result {
	t.Helper()
	r, err := Find(g.m, g.alt, g.seed, minDepth)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// spillAt returns the lowest corner (by CornerHeight, then id) of the edges
// between pass cell p and the cells in set, the lowest such edge ending at
// it, and that edge's cell p: what Find must choose as the spill of a
// depression with cells set over the pass flat {p}.
func (g *grid) spillAt(t *testing.T, flat, set []int) (cell, edge, corner int) {
	t.Helper()
	cell, edge, corner = None, None, None
	bestH := 0.0
	for _, p := range flat {
		for _, e := range g.m.Cells[p].Edges {
			if !slices.Contains(set, g.m.Edges[e].Other(p)) {
				continue
			}
			for _, k := range g.m.Edges[e].Corners {
				h := CornerHeight(g.m, g.alt, k)
				if corner == None || h < bestH || h == bestH && (k < corner || k == corner && e < edge) {
					cell, edge, corner, bestH = p, e, k, h
				}
			}
		}
	}
	if corner == None {
		t.Fatalf("no edge between %v and %v", flat, set)
	}
	return cell, edge, corner
}
