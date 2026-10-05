// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package basin_test

import (
	"bytes"
	"cmp"
	"container/heap"
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/mdhender/mpg/internal/basin"
	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/mesh"
	"github.com/mdhender/mpg/internal/pipeline"
)

// world runs the pipeline through the sea level stage and returns the mesh,
// the altitudes, and the seed cells (rim and ocean). land < 1000 makes a
// small world with a narrow falloff.
func world(t *testing.T, seed uint64, aspect, preset string, land int) (*mesh.Mesh, []float64, []bool) {
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
	last, err := pipeline.Lookup(stages, "sea-level")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.Run(ctx, stages, last); err != nil {
		t.Fatal(err)
	}
	m, sl := ctx.Products.Mesh, ctx.Products.SeaLevel
	seed2 := make([]bool, len(m.Cells))
	for i, cell := range m.Cells {
		seed2[i] = cell.Rim || sl.Ocean[i]
	}
	return m, ctx.Products.Cells.Altitude, seed2
}

// cellHeap is a priority queue of cells by level, then cell id: wgvc's
// cornerHeap pattern over cells.
type cellItem struct {
	cell  int
	level float64
}

type cellHeap []cellItem

func (h cellHeap) Len() int { return len(h) }
func (h cellHeap) Less(i, j int) bool {
	if h[i].level != h[j].level {
		return h[i].level < h[j].level
	}
	return h[i].cell < h[j].cell
}
func (h cellHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *cellHeap) Push(x any)   { *h = append(*h, x.(cellItem)) }
func (h *cellHeap) Pop() any {
	old := *h
	item := old[len(old)-1]
	*h = old[:len(old)-1]
	return item
}

// priorityFlood is the classic priority flood from the sea (Barnes et al.
// 2014): every seed cell starts at −∞; the lowest queued cell is taken and
// each unvisited neighbor queued at the larger of its altitude and the
// taken cell's level. A cell's level is the lowest water surface at which
// it drains to the sea: its altitude, or the spill level of the outermost
// depression holding it.
func priorityFlood(m *mesh.Mesh, alt []float64, seed []bool) []float64 {
	level := make([]float64, len(alt))
	done := make([]bool, len(alt))
	h := &cellHeap{}
	for i, s := range seed {
		if s {
			done[i] = true
			level[i] = math.Inf(-1)
			heap.Push(h, cellItem{i, math.Inf(-1)})
		}
	}
	for h.Len() > 0 {
		it := heap.Pop(h).(cellItem)
		for _, nb := range m.Cells[it.cell].Neighbors {
			if !done[nb] {
				done[nb] = true
				level[nb] = max(alt[nb], it.level)
				heap.Push(h, cellItem{nb, level[nb]})
			}
		}
	}
	return level
}

// TestWorlds checks the hierarchy of real worlds:
//
//   - the altitudes are never modified;
//   - against an independent priority flood from the sea: a cell is in a
//     depression exactly when the flood raises it above its altitude, and
//     then the flood's level is the spill level of its outermost
//     depression;
//   - every depression's cells are all below its spill level, its spill
//     cell is outside it at the spill level, its spill edge joins the
//     spill cell to one of its cells and ends at the spill corner, its
//     parent spills strictly higher, its children are lower ids, its size
//     adds up, and its depth is positive;
//   - the basins are the depressions at least the minimum deep, a kept
//     depression's parent is kept, and each cell's basin is its innermost
//     kept depression;
//   - two runs agree byte for byte.
func TestWorlds(t *testing.T) {
	for _, tc := range []struct {
		seed           uint64
		aspect, preset string
		land           int
	}{
		{42, "cinematic", "continents", 10_000},
		{7, "square", "pangaea", 10_000},
		{3, "cinematic", "archipelago", 2_000},
		{5, "portrait", "pangaea", 800},
	} {
		t.Run(fmt.Sprintf("seed%d-%s-%s-%d", tc.seed, tc.aspect, tc.preset, tc.land), func(t *testing.T) {
			if testing.Short() && tc.land > 2_000 {
				t.Skip("large world")
			}
			t.Parallel()
			m, alt, seed := world(t, tc.seed, tc.aspect, tc.preset, tc.land)
			before := slices.Clone(alt)
			r, err := basin.Find(m, alt, seed, 50)
			if err != nil {
				t.Fatal(err)
			}
			for i := range alt {
				if math.Float64bits(alt[i]) != math.Float64bits(before[i]) {
					t.Fatalf("Find changed cell %d's altitude", i)
				}
			}
			checkWorld(t, m, alt, seed, r)
			again, err := basin.Find(m, alt, seed, 50)
			if err != nil {
				t.Fatal(err)
			}
			a, _ := r.AppendBinary(nil)
			b, _ := again.AppendBinary(nil)
			if !bytes.Equal(a, b) {
				t.Error("two runs differ")
			}
			t.Logf("%d depressions, %d basins, %d cells in basins", len(r.Depressions), len(r.Basins), countIn(r.Of))
		})
	}
}

func countIn(of []int) int {
	n := 0
	for _, b := range of {
		if b != basin.None {
			n++
		}
	}
	return n
}

func checkWorld(t *testing.T, m *mesh.Mesh, alt []float64, seed []bool, r *basin.Result) {
	t.Helper()
	fill := priorityFlood(m, alt, seed)
	// The outermost depression of each cell.
	top := func(d int) int {
		for r.Depressions[d].Parent != basin.None {
			d = r.Depressions[d].Parent
		}
		return d
	}
	for c := range m.Cells {
		d := r.Depression[c]
		switch {
		case seed[c]:
			if d != basin.None {
				t.Errorf("seed cell %d in depression %d", c, d)
			}
		case d == basin.None:
			if fill[c] != alt[c] {
				t.Errorf("cell %d: in no depression, but the flood raises it from %v to %v", c, alt[c], fill[c])
			}
		default:
			if s := r.Depressions[top(d)].SpillM; fill[c] != s || !(alt[c] < s) {
				t.Errorf("cell %d (%v m): flood level %v, outermost spill %v", c, alt[c], fill[c], s)
			}
		}
	}
	in := func(c, d int) bool {
		for e := r.Depression[c]; e != basin.None; e = r.Depressions[e].Parent {
			if e == d {
				return true
			}
		}
		return false
	}
	for d, dep := range r.Depressions {
		if !(dep.DepthM > 0) || dep.DepthM != dep.SpillM-dep.BottomM || alt[dep.Bottom] != dep.BottomM || in(dep.Bottom, d) == false {
			t.Errorf("depression %d: bad depth or bottom: %+v", d, dep)
		}
		if alt[dep.SpillCell] != dep.SpillM || in(dep.SpillCell, d) {
			t.Errorf("depression %d: spill cell %d at %v m, spill %v m, inside %v", d, dep.SpillCell, alt[dep.SpillCell], dep.SpillM, in(dep.SpillCell, d))
		}
		e := m.Edges[dep.SpillEdge]
		if o := e.Other(dep.SpillCell); o < 0 || !in(o, d) || (e.Corners[0] != dep.SpillCorner && e.Corners[1] != dep.SpillCorner) {
			t.Errorf("depression %d: spill edge %d, corner %d, cell %d do not fit", d, dep.SpillEdge, dep.SpillCorner, dep.SpillCell)
		}
		size := len(dep.Cells)
		for _, c := range dep.Children {
			size += r.Depressions[c].Size
			if c >= d || r.Depressions[c].Parent != d {
				t.Errorf("depression %d: child %d", d, c)
			}
		}
		if size != dep.Size {
			t.Errorf("depression %d: size %d, want %d", d, dep.Size, size)
		}
		for _, c := range dep.Cells {
			if !(alt[c] < dep.SpillM) || r.Depression[c] != d {
				t.Errorf("depression %d: cell %d at %v m, spill %v m", d, c, alt[c], dep.SpillM)
			}
		}
		if p := dep.Parent; p != basin.None && !(r.Depressions[p].SpillM > dep.SpillM) {
			t.Errorf("depression %d spills at %v m, its parent %d at %v m", d, dep.SpillM, p, r.Depressions[p].SpillM)
		}
		kept := dep.DepthM >= r.MinDepthM
		if b := r.BasinOf[d]; kept != (b != basin.None) {
			t.Errorf("depression %d, %v m deep: basin %d", d, dep.DepthM, b)
		}
		if p := dep.Parent; kept && p != basin.None && r.BasinOf[p] == basin.None {
			t.Errorf("depression %d kept, its parent %d not", d, p)
		}
	}
	for c := range m.Cells {
		want := basin.None
		for d := r.Depression[c]; d != basin.None; d = r.Depressions[d].Parent {
			if b := r.BasinOf[d]; b != basin.None {
				want = b
				break
			}
		}
		if r.Of[c] != want {
			t.Errorf("cell %d: basin %d, want %d", c, r.Of[c], want)
		}
		if want == basin.None && r.Depression[c] != basin.None {
			if r.RouteM[c] != fill[c] {
				t.Errorf("cell %d: RouteM %v, want the flood level %v", c, r.RouteM[c], fill[c])
			}
		} else if r.RouteM[c] != alt[c] {
			t.Errorf("cell %d: RouteM %v, want its altitude %v", c, r.RouteM[c], alt[c])
		}
	}
	for b, bs := range r.Basins {
		for _, c := range bs.Cells {
			if r.Of[c] != b {
				t.Errorf("basin %d lists cell %d of basin %d", b, c, r.Of[c])
			}
		}
	}
}

// permuted returns a copy of m with cell i renumbered perm[i], and the
// altitudes and seeds to match.
func permuted(m *mesh.Mesh, alt []float64, seed []bool, perm []int) (*mesh.Mesh, []float64, []bool) {
	pm := *m
	n := len(m.Cells)
	pm.Cells = make([]mesh.Cell, n)
	palt := make([]float64, n)
	pseed := make([]bool, n)
	for i, c := range m.Cells {
		c.Neighbors = make([]int, len(m.Cells[i].Neighbors))
		for k, nb := range m.Cells[i].Neighbors {
			c.Neighbors[k] = perm[nb]
		}
		slices.Sort(c.Neighbors)
		pm.Cells[perm[i]] = c
		palt[perm[i]] = alt[i]
		pseed[perm[i]] = seed[i]
	}
	pm.Edges = slices.Clone(m.Edges)
	for e := range pm.Edges {
		for k, c := range pm.Edges[e].Cells {
			if c != mesh.Boundary {
				pm.Edges[e].Cells[k] = perm[c]
			}
		}
	}
	pm.Corners = slices.Clone(m.Corners)
	for k := range pm.Corners {
		cs := make([]int, len(m.Corners[k].Cells))
		for j, c := range m.Corners[k].Cells {
			cs[j] = perm[c]
		}
		slices.Sort(cs)
		pm.Corners[k].Cells = cs
	}
	return &pm, palt, pseed
}

// TestRenumbering checks that the hierarchy does not depend on cell ids:
// with the cells of a real world numbered in reverse, the depressions are
// the same sets of cells with the same spill levels, depths, and nesting.
// Only the choices among equals (a bottom cell, a spill cell) may differ.
func TestRenumbering(t *testing.T) {
	m, alt, seed := world(t, 3, "cinematic", "archipelago", 2_000)
	n := len(m.Cells)
	perm := make([]int, n)
	for i := range perm {
		perm[i] = n - 1 - i
	}
	pm, palt, pseed := permuted(m, alt, seed, perm)
	a, err := basin.Find(m, alt, seed, 50)
	if err != nil {
		t.Fatal(err)
	}
	b, err := basin.Find(pm, palt, pseed, 50)
	if err != nil {
		t.Fatal(err)
	}
	type key struct {
		cells          string
		spill, depth   float64
		parent         string
		kept           bool
		bottomM        float64
		spillCellAltsM float64
	}
	// all returns every cell of depression d, ascending, mapped by f.
	describe := func(r *basin.Result, alt []float64, f func(int) int) []key {
		all := func(d int) []int {
			var cs []int
			for c, e := range r.Depression {
				for ; e != basin.None; e = r.Depressions[e].Parent {
					if e == d {
						cs = append(cs, f(c))
						break
					}
				}
			}
			slices.Sort(cs)
			return cs
		}
		var keys []key
		for d, dep := range r.Depressions {
			k := key{cells: fmt.Sprint(all(d)), spill: dep.SpillM, depth: dep.DepthM, kept: r.BasinOf[d] != basin.None,
				bottomM: dep.BottomM, spillCellAltsM: alt[dep.SpillCell]}
			if dep.Parent != basin.None {
				k.parent = fmt.Sprint(all(dep.Parent))
			}
			keys = append(keys, k)
		}
		slices.SortFunc(keys, func(x, y key) int { return cmp.Compare(x.cells, y.cells) })
		return keys
	}
	inverse := make([]int, n)
	for i, p := range perm {
		inverse[p] = i
	}
	ka := describe(a, alt, func(c int) int { return c })
	kb := describe(b, palt, func(c int) int { return inverse[c] })
	if !slices.Equal(ka, kb) {
		t.Errorf("renumbered hierarchy differs: %d vs %d depressions", len(ka), len(kb))
	}
	if len(ka) == 0 {
		t.Error("no depressions to compare")
	}
}
