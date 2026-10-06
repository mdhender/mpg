// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package measure

import (
	"errors"
	"slices"

	"github.com/mdhender/mpg/internal/classify"
)

// graph is the cell graph the landmass, chokepoint and pass measures walk:
// each cell's neighbors (ascending) with the |grade| of the edge to each,
// and which cells are land, playable water, and mountains. Tests build
// small graphs by hand.
type graph struct {
	// nbr lists each cell's neighbors, ascending; grade[i][k] is the
	// |incline| of the edge to nbr[i][k], in tenths of a percent.
	nbr   [][]int
	grade [][]int
	// land marks the land cells (after lakes), water the playable water
	// cells (ocean, lake, inland sea; never the rim), and mountain the
	// land cells whose landform is mountains.
	land, water, mountain []bool
}

// newGraph builds the graph of in's mesh: its neighbors, the edge data's
// inclines, the land mask, and the landforms. It checks that the
// classification matches the mesh.
func newGraph(in Input) (*graph, error) {
	m := in.Mesh
	n := len(m.Cells)
	if len(in.Classes.Landform) != n || len(in.Classes.Biome) != n || len(in.Classes.Surface) != n || len(in.Edges.Cells) != n {
		return nil, errors.New("measure: landforms or edge data do not match the mesh")
	}
	g := &graph{
		nbr:      make([][]int, n),
		grade:    make([][]int, n),
		land:     slices.Clone(in.Land),
		water:    make([]bool, n),
		mountain: make([]bool, n),
	}
	for i, c := range m.Cells {
		g.nbr[i] = c.Neighbors
		g.grade[i] = make([]int, len(c.Neighbors))
		for _, h := range in.Edges.Cells[i] {
			k := slices.Index(c.Neighbors, h.Neighbor)
			if k < 0 {
				return nil, errors.New("measure: a half-edge's neighbor is not a mesh neighbor")
			}
			g.grade[i][k] = int(h.Incline.Abs())
		}
		g.water[i] = !c.Rim && !in.Land[i]
		g.mountain[i] = in.Land[i] && in.Classes.Landform[i] == classify.Mountains
	}
	return g, nil
}

// cells returns the number of cells.
func (g *graph) cells() int { return len(g.nbr) }

// coastal reports whether land cell c touches playable water.
func (g *graph) coastal(c int) bool {
	if !g.land[c] {
		return false
	}
	for _, j := range g.nbr[c] {
		if g.water[j] {
			return true
		}
	}
	return false
}

// scratch is a breadth-first search's work arrays, reused across searches:
// a cell is reached in the current search when its stamp equals gen, so
// nothing needs clearing between searches.
type scratch struct {
	gen   int
	stamp []int
	dist  []int
	queue []int
}

func newScratch(n int) *scratch {
	return &scratch{stamp: make([]int, n), dist: make([]int, n)}
}

// search reaches every cell joined to src by a path of at most depth steps
// whose cells after src all satisfy ok, recording each one's steps from
// src, and returns the cells reached in breadth-first order (src first).
// Neighbors are visited in ascending order. The result is valid until the
// next search.
func (s *scratch) search(g *graph, src, depth int, ok func(int) bool) []int {
	s.gen++
	s.stamp[src], s.dist[src] = s.gen, 0
	q := append(s.queue[:0], src)
	for k := 0; k < len(q); k++ {
		x := q[k]
		if s.dist[x] >= depth {
			continue
		}
		for _, j := range g.nbr[x] {
			if s.stamp[j] != s.gen && ok(j) {
				s.stamp[j], s.dist[j] = s.gen, s.dist[x]+1
				q = append(q, j)
			}
		}
	}
	s.queue = q
	return q
}

// reached reports whether the last search reached c.
func (s *scratch) reached(c int) bool { return s.stamp[c] == s.gen }

// groups splits members (ascending, no repeats) into the sets connected
// through neighbors among themselves, each ascending, ordered by lowest
// cell. mark is a scratch whose stamps it uses.
func groups(g *graph, members []int, mark *scratch) [][]int {
	mark.gen++
	in := mark.gen
	for _, c := range members {
		mark.stamp[c] = in
	}
	mark.gen++
	seen := mark.gen
	var out [][]int
	for _, s := range members {
		if mark.stamp[s] == seen {
			continue
		}
		mark.stamp[s] = seen
		set := []int{s}
		for k := 0; k < len(set); k++ {
			for _, j := range g.nbr[set[k]] {
				if mark.stamp[j] == in {
					mark.stamp[j] = seen
					set = append(set, j)
				}
			}
		}
		slices.Sort(set)
		out = append(out, set)
	}
	return out
}
