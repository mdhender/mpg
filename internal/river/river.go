// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package river

import (
	"errors"
	"fmt"

	"github.com/mdhender/mpg/internal/basin"
	"github.com/mdhender/mpg/internal/mesh"
)

// None marks the absence of a corner, edge, lake or cell.
const None = -1

// Terminal is what a land corner touches: nothing but land, the sea, a lake,
// or a dry sink.
type Terminal uint8

// The terminal kinds, in the order a corner is assigned them.
const (
	// Interior is a land corner touching only land.
	Interior Terminal = iota
	// Ocean is a corner touching the sea: an ocean or rim cell.
	Ocean
	// Lake is a corner touching a lake or inland sea and not the sea.
	Lake
	// Sink is a dry-sink corner touching no water.
	Sink
)

// String returns the kind's name.
func (k Terminal) String() string {
	switch k {
	case Interior:
		return "interior"
	case Ocean:
		return "ocean"
	case Lake:
		return "lake"
	case Sink:
		return "sink"
	}
	return fmt.Sprintf("Terminal(%d)", uint8(k))
}

// LakeIn is one lake of the input.
type LakeIn struct {
	// SurfaceM is the lake's surface level, which decides which of two
	// lakes meeting at a corner takes it (the lower).
	SurfaceM float64
	// Spill is the lake's spill corner when it overflows (basin
	// Lake.Outlet), or None for a closed lake.
	Spill int
	// Pass is the pass cell of an overflowing lake (the spill cell of its
	// basin), or None.
	Pass int
}

// Dest is where water ends: the sea, a lake (ID its lake id), or a dry
// sink (ID its corner). A Dest of kind Interior is unknown: the other
// model's water stopped somewhere this package does not name.
type Dest struct {
	Kind Terminal
	ID   int
}

// Input is what the tree is built from. Every slice is indexed by cell id
// except Lakes (by lake id) and Sinks.
type Input struct {
	Mesh *mesh.Mesh
	// Altitude is each cell's altitude (cells.Stats.Altitude).
	Altitude []float64
	// Land marks the land cells after lakes. Rim cells are never land.
	Land []bool
	// Lake is each cell's lake id, or None. A cell that is neither land nor
	// lake is sea (ocean or rim).
	Lake []int
	// Lakes lists the lakes.
	Lakes []LakeIn
	// Sinks lists the dry-sink corners (each playa's lowest corner).
	Sinks []int
	// Catchment and FinalCatchment are optional: each cell's destination
	// in the cell-level model (S28: basin Lakes.Sink), immediately and
	// after following lake overflow. With them, Build reports how far the
	// tree agrees (Tree.Agreement). Only land cells are read.
	Catchment, FinalCatchment []Dest
}

// Drain is how an overflowing lake's water leaves it.
type Drain uint8

// The drains.
const (
	// Closed: the lake does not overflow.
	Closed Drain = iota
	// Outlet: down the tree from the lake's outlet corner.
	Outlet
	// DirectSea: the spill corner (or the outlet corner chosen on the pass
	// cell) touches the sea; no river.
	DirectSea
	// DirectLake: the spill corner belongs to another, lower lake (Into),
	// or the outlet corner chosen on the pass cell lies between this lake
	// and another with no way over land; no river.
	DirectLake
)

// String returns the drain's name.
func (d Drain) String() string {
	switch d {
	case Closed:
		return "closed"
	case Outlet:
		return "outlet"
	case DirectSea:
		return "direct-sea"
	case DirectLake:
		return "direct-lake"
	}
	return fmt.Sprintf("Drain(%d)", uint8(d))
}

// LakeOut is how a lake drains in the tree.
type LakeOut struct {
	Drain Drain
	// Corner is the outlet corner (Drain Outlet), or None.
	Corner int
	// Into is the lake drained into (Drain DirectLake), or None.
	Into int
	// AtSpill reports an outlet at the spill corner itself; otherwise it is
	// a shore corner of the pass cell, or, with Fallback, the corner the
	// fallback chose.
	AtSpill, Fallback bool
}

// Agreement compares the tree's catchments with the cell-level model's,
// over land cells (each assigned to its lowest corner).
type Agreement struct {
	// Cells counts the land cells and AreaKm2 their area; Differ and
	// DifferKm2 those whose immediate destination differs, FinalDiffer
	// and FinalDifferKm2 those whose final destination (after lake
	// overflow) differs.
	Cells, Differ, FinalDiffer         int
	AreaKm2, DifferKm2, FinalDifferKm2 float64
}

// Tree is the corner drainage tree. Slices are indexed by corner id
// except Lakes (by lake id) and Order.
type Tree struct {
	// Height is each corner's height: the mean altitude of its cells
	// (basin.CornerHeight).
	Height []float64
	// Land marks the corners in the graph: those touching a land cell.
	Land []bool
	// Terminal is each land corner's kind, and Water the lake of a Lake
	// corner (None otherwise).
	Terminal []Terminal
	Water    []int
	// Level is each land corner's flood level: the lowest level at which
	// its water reaches a terminal (at least its height).
	Level []float64
	// Down and DownEdge are each land corner's downstream corner and the
	// land–land edge to it, or None where water ends: at a terminal (the
	// sea, a lake shore, a dry sink), except a lake's outlet.
	Down, DownEdge []int
	// Order lists the land corners in the order the flood resolved them:
	// every corner after its downstream corner, and an overflowing lake's
	// shore corners after the corner its water continues from (its outlet,
	// or the spill corner it drains straight through). Accumulating in
	// reverse order therefore sees every lake's inflow before its outflow.
	Order []int
	// Lakes is each lake's drain.
	Lakes []LakeOut
	// Fallbacks counts the outlets chosen by the fallback (rule d).
	Fallbacks int
	// Agreement is set when the input has catchments.
	Agreement *Agreement
}

// Build builds the drainage tree. See the package documentation.
func Build(in Input) (*Tree, error) {
	if err := in.check(); err != nil {
		return nil, err
	}
	b := newBuilder(in)
	if err := b.flood(); err != nil {
		return nil, err
	}
	t := b.t
	if in.Catchment != nil {
		a := t.agreement(in)
		t.Agreement = &a
	}
	return t, nil
}

func (in *Input) check() error {
	m := in.Mesh
	if m == nil {
		return errors.New("river: no mesh")
	}
	n := len(m.Cells)
	if len(in.Altitude) != n || len(in.Land) != n || len(in.Lake) != n {
		return fmt.Errorf("river: %d cells, but %d altitudes, %d land flags, %d lake ids", n, len(in.Altitude), len(in.Land), len(in.Lake))
	}
	for c, l := range in.Lake {
		if l != None && (l < 0 || l >= len(in.Lakes)) {
			return fmt.Errorf("river: cell %d: lake %d out of range", c, l)
		}
		if l != None && in.Land[c] {
			return fmt.Errorf("river: cell %d is land and lake %d", c, l)
		}
		if in.Land[c] && m.Cells[c].Rim {
			return fmt.Errorf("river: rim cell %d is land", c)
		}
	}
	for l, k := range in.Lakes {
		if k.Spill == None {
			continue
		}
		if k.Spill < 0 || k.Spill >= len(m.Corners) || k.Pass < 0 || k.Pass >= n {
			return fmt.Errorf("river: lake %d: spill corner %d or pass cell %d out of range", l, k.Spill, k.Pass)
		}
	}
	for _, k := range in.Sinks {
		if k < 0 || k >= len(m.Corners) {
			return fmt.Errorf("river: sink corner %d out of range", k)
		}
	}
	for _, d := range [][]Dest{in.Catchment, in.FinalCatchment} {
		if d != nil && len(d) != n {
			return fmt.Errorf("river: %d cells, but %d catchments", n, len(d))
		}
	}
	if (in.Catchment == nil) != (in.FinalCatchment == nil) {
		return errors.New("river: catchments need both immediate and final")
	}
	return nil
}

// link is a land–land edge seen from one of its corners.
type link struct{ corner, edge int }

// builder holds the flood's state.
type builder struct {
	in  Input
	t   *Tree
	adj [][]link
	// lakesAt lists the lakes each corner touches, ascending; shore lists
	// each lake's land corners, ascending.
	lakesAt [][]int
	shore   [][]int
	// eligible[l] lists the corners that may become lake l's outlet.
	eligible [][]int
	released []bool
	// directAt lists, per corner, the lakes that drain straight through it.
	directAt map[int][]int
	visited  []bool
	resolved []bool
	rank     []int
	held     []bool // reached but held
	heldAt   []float64
	nHeld    int
	q        queue
	seq      int
}

func newBuilder(in Input) *builder {
	m := in.Mesh
	nk := len(m.Corners)
	t := &Tree{
		Height:   make([]float64, nk),
		Land:     make([]bool, nk),
		Terminal: make([]Terminal, nk),
		Water:    make([]int, nk),
		Level:    make([]float64, nk),
		Down:     make([]int, nk),
		DownEdge: make([]int, nk),
		Lakes:    make([]LakeOut, len(in.Lakes)),
	}
	b := &builder{
		in:       in,
		t:        t,
		adj:      make([][]link, nk),
		lakesAt:  make([][]int, nk),
		shore:    make([][]int, len(in.Lakes)),
		eligible: make([][]int, len(in.Lakes)),
		released: make([]bool, len(in.Lakes)),
		directAt: map[int][]int{},
		visited:  make([]bool, nk),
		resolved: make([]bool, nk),
		rank:     make([]int, nk),
		held:     make([]bool, nk),
		heldAt:   make([]float64, nk),
	}
	sink := make([]bool, nk)
	for _, k := range in.Sinks {
		sink[k] = true
	}
	for k, cr := range m.Corners {
		t.Height[k] = basin.CornerHeight(m, in.Altitude, k)
		t.Down[k], t.DownEdge[k], t.Water[k], b.rank[k] = None, None, None, None
		sea := false
		for _, c := range cr.Cells {
			switch {
			case in.Land[c]:
				t.Land[k] = true
			case in.Lake[c] != None:
				b.lakesAt[k] = insert(b.lakesAt[k], in.Lake[c])
			default:
				sea = true
			}
		}
		if !t.Land[k] {
			continue
		}
		for _, l := range b.lakesAt[k] {
			b.shore[l] = append(b.shore[l], k)
		}
		switch {
		case sea:
			t.Terminal[k] = Ocean
		case len(b.lakesAt[k]) > 0:
			t.Terminal[k] = Lake
			w := b.lakesAt[k][0]
			for _, l := range b.lakesAt[k][1:] {
				if in.Lakes[l].SurfaceM < in.Lakes[w].SurfaceM {
					w = l
				}
			}
			t.Water[k] = w
		case sink[k]:
			t.Terminal[k] = Sink
		}
	}
	for e, ed := range m.Edges {
		if ed.OnBoundary() || !in.Land[ed.Cells[0]] || !in.Land[ed.Cells[1]] {
			continue
		}
		a, c := ed.Corners[0], ed.Corners[1]
		b.adj[a] = append(b.adj[a], link{c, e})
		b.adj[c] = append(b.adj[c], link{a, e})
	}
	b.outlets()
	return b
}

// insert adds l to the ascending list s if absent.
func insert(s []int, l int) []int {
	for i, x := range s {
		if x == l {
			return s
		}
		if x > l {
			s = append(s, 0)
			copy(s[i+1:], s[i:])
			s[i] = l
			return s
		}
	}
	return append(s, l)
}

// touches reports whether corner k touches lake l.
func (b *builder) touches(k, l int) bool {
	for _, x := range b.lakesAt[k] {
		if x == l {
			return true
		}
	}
	return false
}

// outlets decides each overflowing lake's drain rule: (a) direct when its
// spill corner touches the sea or belongs to another lake; (b) the spill
// corner when it has a land–land edge to a corner off the lake; (c) else
// the lake's shore corners on its pass cell.
func (b *builder) outlets() {
	in, t, m := b.in, b.t, b.in.Mesh
	for l, k := range in.Lakes {
		out := &t.Lakes[l]
		out.Corner, out.Into = None, None
		if k.Spill == None {
			continue
		}
		s := k.Spill
		switch {
		case t.Land[s] && t.Terminal[s] == Ocean:
			out.Drain = DirectSea
			b.directAt[s] = append(b.directAt[s], l)
			continue
		case t.Land[s] && t.Terminal[s] == Lake && t.Water[s] != l:
			out.Drain, out.Into = DirectLake, t.Water[s]
			b.directAt[s] = append(b.directAt[s], l)
			continue
		}
		out.Drain = Outlet
		if t.Land[s] {
			for _, x := range b.adj[s] {
				if !b.touches(x.corner, l) {
					b.eligible[l] = []int{s}
					break
				}
			}
		}
		if b.eligible[l] != nil {
			continue
		}
		if in.Land[k.Pass] {
			for _, c := range m.Cells[k.Pass].Corners {
				if b.touches(c, l) {
					b.eligible[l] = insert(b.eligible[l], c)
				}
			}
		}
		if b.eligible[l] == nil {
			b.eligible[l] = []int{} // only the fallback can open it
		}
	}
}

// root reports whether corner k ends water from the start: the sea, a dry
// sink, or the shore of a closed lake.
func (b *builder) root(k int) bool {
	switch b.t.Terminal[k] {
	case Ocean, Sink:
		return true
	case Lake:
		return b.in.Lakes[b.t.Water[k]].Spill == None
	}
	return false
}

func (b *builder) overflows(l int) bool { return b.in.Lakes[l].Spill != None }

func (b *builder) isEligible(l, k int) bool {
	for _, x := range b.eligible[l] {
		if x == k {
			return true
		}
	}
	return false
}

// holds reports whether corner k is held: it touches an overflowing lake
// not yet released, and is neither that lake's possible outlet nor the
// spill corner it drains straight through.
func (b *builder) holds(k int) bool {
	if b.root(k) {
		return false
	}
	for _, l := range b.lakesAt[k] {
		if !b.overflows(l) || b.released[l] {
			continue
		}
		if b.isEligible(l, k) || b.t.Lakes[l].Drain != Outlet && b.in.Lakes[l].Spill == k {
			continue
		}
		return true
	}
	return false
}

func (b *builder) push(k int, level float64) {
	b.visited[k] = true
	if b.held[k] {
		b.held[k] = false
		b.nHeld--
	}
	b.t.Level[k] = level
	b.q.push(item{level, b.seq, k})
	b.seq++
}

// reach offers corner k at level: queued, or held.
func (b *builder) reach(k int, level float64) {
	if b.visited[k] {
		return
	}
	if b.holds(k) {
		if !b.held[k] {
			b.held[k] = true
			b.nHeld++
			b.heldAt[k] = level
		} else {
			b.heldAt[k] = min(b.heldAt[k], level)
		}
		return
	}
	b.push(k, level)
}

// release opens lake l: its shore corners are queued at the level its water
// leaves by, from.
func (b *builder) release(l, from int) {
	b.released[l] = true
	for _, k := range b.shore[l] {
		b.reach(k, max(b.t.Height[k], b.t.Level[from]))
	}
}

func (b *builder) flood() error {
	t, m := b.t, b.in.Mesh
	for k := range m.Corners {
		if t.Land[k] && b.root(k) {
			b.push(k, t.Height[k])
		}
	}
	for {
		if b.q.len() == 0 {
			if b.nHeld == 0 {
				break
			}
			b.fallback()
		}
		k := b.q.pop().corner
		b.rank[k] = len(t.Order)
		t.Order = append(t.Order, k)
		b.resolved[k] = true
		var becomes []int
		for _, l := range b.lakesAt[k] {
			if b.overflows(l) && !b.released[l] && t.Lakes[l].Drain == Outlet && b.isEligible(l, k) {
				becomes = append(becomes, l)
			}
		}
		if !b.root(k) && (t.Terminal[k] != Lake || len(becomes) > 0) {
			best := link{None, None}
			for _, x := range b.adj[k] {
				if !b.resolved[x.corner] {
					continue
				}
				if best.corner == None || t.Level[x.corner] < t.Level[best.corner] ||
					t.Level[x.corner] == t.Level[best.corner] && b.rank[x.corner] < b.rank[best.corner] {
					best = x
				}
			}
			if best.corner == None && len(becomes) == 0 {
				return fmt.Errorf("river: corner %d has no resolved neighbor", k)
			}
			t.Down[k], t.DownEdge[k] = best.corner, best.edge
		}
		for _, l := range becomes {
			out := &t.Lakes[l]
			switch {
			case t.Down[k] != None:
				out.Corner = k
				out.AtSpill = k == b.in.Lakes[l].Spill
			case t.Terminal[k] == Ocean:
				// The outlet has no way over land: it touches the sea.
				out.Drain = DirectSea
			default:
				// The outlet has no way over land: it lies between this
				// lake and another (a 4-way corner), or on a closed lake.
				into := b.other(k, l)
				if into == None {
					return fmt.Errorf("river: lake %d: outlet %d leads nowhere", l, k)
				}
				out.Drain, out.Into = DirectLake, into
			}
			b.release(l, k)
		}
		for _, l := range b.directAt[k] {
			if !b.released[l] {
				b.release(l, k)
			}
		}
		for _, x := range b.adj[k] {
			b.reach(x.corner, max(t.Height[x.corner], t.Level[k]))
		}
	}
	for k := range m.Corners {
		if t.Land[k] && !b.resolved[k] {
			return fmt.Errorf("river: land corner %d not reached", k)
		}
	}
	return nil
}

// other returns the lake other than l that corner k drains into: its own
// lake when that is not l, else the lowest-id other lake it touches, or
// None.
func (b *builder) other(k, l int) int {
	if w := b.t.Water[k]; w != None && w != l {
		return w
	}
	for _, x := range b.lakesAt[k] {
		if x != l {
			return x
		}
	}
	return None
}

// fallback (rule d) opens the held corner with the lowest (level, id) as
// the outlet of every overflowing lake holding it.
func (b *builder) fallback() {
	k := None
	for c, h := range b.held {
		if h && (k == None || b.heldAt[c] < b.heldAt[k]) {
			k = c
		}
	}
	for _, l := range b.lakesAt[k] {
		if b.overflows(l) && !b.released[l] && !b.isEligible(l, k) {
			b.t.Lakes[l].Drain = Outlet
			b.t.Lakes[l].Fallback = true
			b.t.Lakes[l].Into = None
			b.eligible[l] = insert(b.eligible[l], k)
		}
	}
	b.t.Fallbacks++
	b.push(k, b.heldAt[k])
}

// item is a queued corner: by level, then queue order (first in, first out
// among equal levels).
type item struct {
	level  float64
	seq    int
	corner int
}

func (a item) less(c item) bool { return a.level < c.level || a.level == c.level && a.seq < c.seq }

// queue is a binary min-heap of items.
type queue struct{ h []item }

func (q *queue) len() int { return len(q.h) }

func (q *queue) push(x item) {
	q.h = append(q.h, x)
	for i := len(q.h) - 1; i > 0; {
		p := (i - 1) / 2
		if !q.h[i].less(q.h[p]) {
			break
		}
		q.h[i], q.h[p] = q.h[p], q.h[i]
		i = p
	}
}

func (q *queue) pop() item {
	top := q.h[0]
	n := len(q.h) - 1
	q.h[0] = q.h[n]
	q.h = q.h[:n]
	for i := 0; ; {
		l, r, s := 2*i+1, 2*i+2, i
		if l < n && q.h[l].less(q.h[s]) {
			s = l
		}
		if r < n && q.h[r].less(q.h[s]) {
			s = r
		}
		if s == i {
			break
		}
		q.h[i], q.h[s] = q.h[s], q.h[i]
		i = s
	}
	return top
}
