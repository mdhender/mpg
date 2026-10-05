// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package basin

import (
	"cmp"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/mdhender/mpg/internal/mesh"
)

// None marks the absence of a basin, depression, or parent: a cell in no
// basin, or a top-level basin, whose parent is the sea.
const None = -1

// Basin is one closed depression of the cell graph: a connected set of
// cells, all lower than its spill level, that water can leave only by
// rising to the spill level. The same type holds a depression of the full
// hierarchy (Result.Depressions) and a basin that passed the minimum depth
// (Result.Basins); the ids in Parent and Children refer to the slice the
// value is in.
type Basin struct {
	// Parent is the depression that holds this one, the one it overflows
	// into, or None for a top-level depression, which overflows to the sea.
	Parent int
	// Children lists the depressions nested directly in this one,
	// ascending. Each has a lower id than its parent.
	Children []int
	// Bottom is the lowest cell (by altitude, then cell id) and BottomM its
	// altitude.
	Bottom  int
	BottomM float64
	// SpillM is the spill level: the altitude of SpillCell, the lowest
	// level at which water standing in the depression escapes it.
	SpillM float64
	// DepthM is SpillM − BottomM, always positive.
	DepthM float64
	// SpillCell is the pass: the cell, outside this depression and at its
	// spill level, over which its water leaves. For a nested depression it
	// is a cell of the parent; for a top-level one, a land cell that drains
	// freely to the sea.
	SpillCell int
	// SpillEdge is the edge between SpillCell and a cell of the depression
	// that ends at SpillCorner (the lowest such edge id).
	SpillEdge int
	// SpillCorner is the one corner through which overflow leaves: of the
	// corners of the edges between the pass cells and the depression, the
	// lowest by corner height (the mean altitude of the cells that meet
	// there), ties to the lower corner id.
	SpillCorner int
	// Size counts the depression's cells, those of the depressions nested
	// in it included.
	Size int
	// Cells lists, ascending, the cells of this depression that are in no
	// child of it. For a basin it includes the cells of every shallower
	// depression merged into it.
	Cells []int
}

// Result is the basin hierarchy of a cell graph.
type Result struct {
	// MinDepthM is the minimum depth a depression needs to be a basin.
	MinDepthM float64
	// Depressions is the full hierarchy, every depression the flood found,
	// in the order they formed (a local minimum flooded, or depressions
	// meeting at a pass): children before their parents.
	Depressions []Basin
	// Basins is the hierarchy after the minimum depth: the depressions at
	// least MinDepthM deep, in the same order, renumbered. A shallower
	// depression nested in a basin is merged into it (its cells join the
	// basin's Cells); a shallower top-level depression, with everything in
	// it, is no basin at all.
	Basins []Basin
	// BasinOf maps a depression id to its basin id, or None for a
	// depression shallower than MinDepthM.
	BasinOf []int
	// Depression and Of give each cell's innermost depression and innermost
	// basin, or None.
	Depression, Of []int
	// RouteM is each cell's height for routing: for a cell whose
	// innermost depression is shallower than the minimum, the spill level
	// of the outermost depression around it that is shallower too (the
	// top-level one, for a cell in no basin; the one merged into a basin,
	// for a cell of a basin), so every shallow depression routes as flat
	// ground at its spill level; for every other cell its altitude.
	// Altitudes themselves are never changed.
	RouteM []float64
}

// Labels of union-find roots that are not depressions.
const (
	unlabeled = -1 // a flat being processed
	sea       = -2 // the sea: the seed cells and everything joined to them
)

// Find returns the basin hierarchy of m's cells with altitudes alt, seeded
// from the cells marked in seed (the sea: ocean and rim cells), with
// minimum depth minDepthM. See the package documentation for the
// algorithm. alt is read only. It fails when alt or seed does not match m,
// an altitude is not finite, minDepthM is negative or not finite, no cell
// is a seed, or some cell cannot reach a seed.
func Find(m *mesh.Mesh, alt []float64, seed []bool, minDepthM float64) (*Result, error) {
	n := len(m.Cells)
	switch {
	case len(alt) != n:
		return nil, fmt.Errorf("basin: %d altitudes for %d cells", len(alt), n)
	case len(seed) != n:
		return nil, fmt.Errorf("basin: %d seed flags for %d cells", len(seed), n)
	case !(minDepthM >= 0) || math.IsInf(minDepthM, 1):
		return nil, fmt.Errorf("basin: minimum depth %v m must be non-negative and finite", minDepthM)
	}
	for i, v := range alt {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("basin: cell %d altitude %v is not finite", i, v)
		}
	}
	f := newFlood(m, alt)
	if err := f.seed(seed); err != nil {
		return nil, err
	}
	f.run(seed)
	for d, dep := range f.deps {
		if dep.Parent == None && dep.SpillCell == None {
			return nil, fmt.Errorf("basin: depression %d (bottom cell %d) never reaches a seed cell", d, dep.Bottom)
		}
	}
	return f.result(minDepthM), nil
}

// flood is the state of one Find.
type flood struct {
	m   *mesh.Mesh
	alt []float64
	// up is the union-find forest over cells: up[c] is c's parent, or −1
	// before c is flooded. size is a root's tree size, label its depression
	// id, sea, or unlabeled.
	up, size, label []int
	// mark[c] is the generation (altitude group) in which c was flooded.
	mark []int
	// dep[c] is the depression c's own cells belong to, or None.
	dep  []int
	deps []Basin
}

func newFlood(m *mesh.Mesh, alt []float64) *flood {
	n := len(m.Cells)
	f := &flood{
		m: m, alt: alt,
		up:    make([]int, n),
		size:  make([]int, n),
		label: make([]int, n),
		mark:  make([]int, n),
		dep:   make([]int, n),
	}
	for i := range n {
		f.up[i] = -1
		f.dep[i] = None
	}
	return f
}

// seed floods every seed cell at once, as one sea component.
func (f *flood) seed(seed []bool) error {
	root := -1
	for i, s := range seed {
		if !s {
			continue
		}
		if root < 0 {
			root = i
			f.up[i], f.size[i], f.label[i] = i, 0, sea
		}
		f.up[i] = root
		f.size[root]++
	}
	if root < 0 {
		return errors.New("basin: no seed cell")
	}
	return nil
}

func (f *flood) find(c int) int {
	r := c
	for f.up[r] != r {
		r = f.up[r]
	}
	for f.up[c] != r {
		f.up[c], c = r, f.up[c]
	}
	return r
}

// union joins the trees of roots a and b and returns the new root, which
// keeps a's label.
func (f *flood) union(a, b int) int {
	if a == b {
		return a
	}
	l := f.label[a]
	if f.size[a] < f.size[b] {
		a, b = b, a
	}
	f.up[b] = a
	f.size[a] += f.size[b]
	f.label[a] = l
	return a
}

// run floods the non-seed cells in ascending (altitude, id) order, a group
// of equal altitudes at a time.
func (f *flood) run(seed []bool) {
	order := make([]int, 0, len(seed))
	for i, s := range seed {
		if !s {
			order = append(order, i)
		}
	}
	slices.SortFunc(order, func(a, b int) int {
		return cmp.Or(cmp.Compare(f.alt[a], f.alt[b]), cmp.Compare(a, b))
	})
	flatOf := make([]int, len(seed)) // a group root's flat index, by generation
	stamp := make([]int, len(seed))
	var flats [][]int
	var roots, deps []int
	gen := 0
	for i := 0; i < len(order); {
		a := f.alt[order[i]]
		j := i
		for j < len(order) && f.alt[order[j]] == a {
			j++
		}
		group := order[i:j]
		i = j
		gen++
		for _, c := range group {
			f.up[c], f.size[c], f.label[c], f.mark[c] = c, 1, unlabeled, gen
		}
		// The flats: the group's cells joined through each other.
		for _, c := range group {
			for _, nb := range f.m.Cells[c].Neighbors {
				if f.mark[nb] == gen && f.up[nb] >= 0 {
					f.union(f.find(c), f.find(nb))
				}
			}
		}
		flats = flats[:0]
		for _, c := range group { // ascending id: each flat in order of its lowest cell
			r := f.find(c)
			if stamp[r] != gen {
				stamp[r], flatOf[r] = gen, len(flats)
				flats = append(flats, nil)
			}
			flats[flatOf[r]] = append(flats[flatOf[r]], c)
		}
		for _, flat := range flats {
			// The components already flooded that the flat touches.
			roots, deps = roots[:0], deps[:0]
			toSea := false
			for _, c := range flat {
				for _, nb := range f.m.Cells[c].Neighbors {
					if f.up[nb] < 0 || f.mark[nb] == gen {
						continue
					}
					r := f.find(nb)
					if slices.Contains(roots, r) {
						continue
					}
					roots = append(roots, r)
					if l := f.label[r]; l == sea {
						toSea = true
					} else {
						deps = append(deps, l)
					}
				}
			}
			slices.Sort(deps)
			label := unlabeled
			switch {
			case len(roots) == 0:
				// A local minimum: a new depression.
				label = len(f.deps)
				f.deps = append(f.deps, Basin{
					Parent: None, Bottom: flat[0], BottomM: a,
					SpillCell: None, SpillEdge: None, SpillCorner: None,
				})
			case toSea:
				// Every depression the flat touches overflows to the sea.
				for _, d := range deps {
					f.close(d, flat, gen, a)
				}
				label = sea
			case len(deps) == 1:
				// The flat raises the one depression it touches.
				label = deps[0]
			default:
				// Two or more depressions meet at the flat: each closes,
				// overflowing into a new depression that holds them all.
				label = len(f.deps)
				p := Basin{Parent: None, Children: slices.Clone(deps), SpillCell: None, SpillEdge: None, SpillCorner: None}
				for k, d := range deps {
					f.close(d, flat, gen, a)
					f.deps[d].Parent = label
					if c := f.deps[d]; k == 0 || c.BottomM < p.BottomM || c.BottomM == p.BottomM && c.Bottom < p.Bottom {
						p.Bottom, p.BottomM = c.Bottom, c.BottomM
					}
				}
				f.deps = append(f.deps, p)
			}
			if label >= 0 {
				f.deps[label].Cells = append(f.deps[label].Cells, flat...)
				for _, c := range flat {
					f.dep[c] = label
				}
			}
			r := f.find(flat[0])
			for _, o := range roots {
				r = f.union(r, o)
			}
			f.label[r] = label
		}
	}
}

// close ends depression d at the flat, flooded in generation gen at
// altitude a: its spill level is a, and its spill corner the lowest corner
// of the edges between the flat and d's cells (ties to the lower corner
// id), its spill edge the lowest such edge ending there, and its spill
// cell that edge's cell in the flat.
func (f *flood) close(d int, flat []int, gen int, a float64) {
	// A depression is created with its flat, so its own cells are never
	// empty; its children's cells share its union-find root.
	root := f.find(f.deps[d].Cells[0])
	best := Basin{SpillCell: None, SpillEdge: None, SpillCorner: None}
	bestH := 0.0
	for _, c := range flat {
		for _, e := range f.m.Cells[c].Edges {
			o := f.m.Edges[e].Other(c)
			if o == mesh.Boundary || f.up[o] < 0 || f.mark[o] == gen || f.find(o) != root {
				continue
			}
			for _, k := range f.m.Edges[e].Corners {
				h := f.cornerHeight(k)
				if best.SpillCorner == None || h < bestH || h == bestH && (k < best.SpillCorner || k == best.SpillCorner && e < best.SpillEdge) {
					best.SpillCell, best.SpillEdge, best.SpillCorner, bestH = c, e, k, h
				}
			}
		}
	}
	dep := &f.deps[d]
	dep.SpillM = a
	dep.DepthM = a - dep.BottomM
	dep.SpillCell, dep.SpillEdge, dep.SpillCorner = best.SpillCell, best.SpillEdge, best.SpillCorner
}

// cornerHeight returns corner k's height: the mean altitude of the cells
// that meet there, summed in ascending cell id order.
func (f *flood) cornerHeight(k int) float64 {
	cs := f.m.Corners[k].Cells
	s := 0.0
	for _, c := range cs {
		s += f.alt[c]
	}
	return s / float64(len(cs))
}

// CornerHeight returns corner k of m's height for altitudes alt: the mean
// altitude of the cells that meet there, summed in ascending cell id order.
func CornerHeight(m *mesh.Mesh, alt []float64, k int) float64 {
	return (&flood{m: m, alt: alt}).cornerHeight(k)
}

// result applies the minimum depth to the hierarchy.
func (f *flood) result(minDepthM float64) *Result {
	n := len(f.m.Cells)
	r := &Result{
		MinDepthM:   minDepthM,
		Depressions: f.deps,
		BasinOf:     make([]int, len(f.deps)),
		Depression:  f.dep,
		Of:          make([]int, n),
		RouteM:      slices.Clone(f.alt),
	}
	// Sizes, children before parents.
	for d := range r.Depressions {
		dep := &r.Depressions[d]
		dep.Size += len(dep.Cells)
		slices.Sort(dep.Cells)
		if p := dep.Parent; p != None {
			r.Depressions[p].Size += dep.Size
		}
	}
	for d, dep := range r.Depressions {
		r.BasinOf[d] = None
		if dep.DepthM >= minDepthM {
			r.BasinOf[d] = len(r.Basins)
			b := dep
			b.Children, b.Cells = nil, nil
			r.Basins = append(r.Basins, b)
		}
	}
	// owner is the basin a depression's own cells belong to: its own, or
	// its nearest kept ancestor's; shallow, for a depression shallower than
	// the minimum, its outermost ancestor that is shallower too (itself
	// when its parent is kept or it is top-level). Parents come after
	// children, so walk down from the end.
	owner := make([]int, len(r.Depressions))
	shallow := make([]int, len(r.Depressions))
	for d := len(r.Depressions) - 1; d >= 0; d-- {
		dep := r.Depressions[d]
		owner[d], shallow[d] = r.BasinOf[d], None
		if owner[d] == None {
			shallow[d] = d
		}
		if p := dep.Parent; p != None {
			if owner[d] == None {
				owner[d] = owner[p]
				if r.BasinOf[p] == None {
					shallow[d] = shallow[p]
				}
			}
		}
	}
	for b := range r.Basins {
		bs := &r.Basins[b]
		if bs.Parent != None {
			// A kept depression's parent is at least as deep, so kept.
			bs.Parent = r.BasinOf[bs.Parent]
			r.Basins[bs.Parent].Children = append(r.Basins[bs.Parent].Children, b)
		}
	}
	for c := range n {
		r.Of[c] = None
		d := r.Depression[c]
		if d == None {
			continue
		}
		if b := owner[d]; b != None {
			r.Of[c] = b
			r.Basins[b].Cells = append(r.Basins[b].Cells, c)
		}
		if s := shallow[d]; s != None {
			r.RouteM[c] = r.Depressions[s].SpillM
		}
	}
	return r
}

// Nesting returns how many basins hold basin b: 0 for a top-level basin.
func (r *Result) Nesting(b int) int {
	k := 0
	for p := r.Basins[b].Parent; p != None; p = r.Basins[p].Parent {
		k++
	}
	return k
}

// AppendBinary appends the result's canonical encoding to b; it is the
// input to the golden hashes. Integers are little-endian uint64 (None as
// its two's complement); floats their IEEE 754 bits as little-endian
// uint64. In order:
//
//	MinDepthM                                  float
//	number of depressions                      integer
//	per depression, then (after its count)
//	per basin, in id order:
//	    Parent                                 integer
//	    number of children, then each          integers
//	    Bottom                                 integer
//	    BottomM, SpillM, DepthM                floats
//	    SpillCell, SpillEdge, SpillCorner      integers
//	    Size                                   integer
//	    number of cells, then each             integers
//	number of basins                           integer
//	per depression: BasinOf                    integer
//	number of cells                            integer
//	per cell: Depression, Of                   integers
//	          RouteM                           float
//
// The error is always nil; the signature is encoding.BinaryAppender's.
func (r *Result) AppendBinary(b []byte) ([]byte, error) {
	fl := func(v float64) { b = binary.LittleEndian.AppendUint64(b, math.Float64bits(v)) }
	in := func(v int) { b = binary.LittleEndian.AppendUint64(b, uint64(int64(v))) }
	list := func(bs []Basin) {
		in(len(bs))
		for _, x := range bs {
			in(x.Parent)
			in(len(x.Children))
			for _, c := range x.Children {
				in(c)
			}
			in(x.Bottom)
			fl(x.BottomM)
			fl(x.SpillM)
			fl(x.DepthM)
			in(x.SpillCell)
			in(x.SpillEdge)
			in(x.SpillCorner)
			in(x.Size)
			in(len(x.Cells))
			for _, c := range x.Cells {
				in(c)
			}
		}
	}
	fl(r.MinDepthM)
	list(r.Depressions)
	list(r.Basins)
	for _, v := range r.BasinOf {
		in(v)
	}
	in(len(r.Of))
	for c := range r.Of {
		in(r.Depression[c])
		in(r.Of[c])
		fl(r.RouteM[c])
	}
	return b, nil
}
