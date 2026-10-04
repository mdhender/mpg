// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package mesh

import (
	"cmp"
	"container/heap"
	"fmt"
	"slices"

	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/internal/topo"
)

// StretchMargin is how far past the minimum a stretch lengthens an edge, as
// a fraction of the minimum: a stretched edge is minKm·(1 + StretchMargin)
// long, so rounding in its recomputed length cannot leave it short.
const StretchMargin = 1e-6

// collapseOpsPerEdge bounds Collapse's work: it fails after this many
// collapses and stretches per edge of the input.
const collapseOpsPerEdge = 16

// Collapse returns m with no edge shorter than minKm and no cell with more
// than degreeCap neighbors: the short-edge collapse and the degree cap (see
// the package documentation). m is not modified. The result is a new graph
// with canonical ids; its Collapses, Stretches, DegreeCapHits, MaxShiftKm,
// and Stretched report what was done, and GhostMarginKm and LloydCV are
// carried over.
//
// It fails, rather than return a bad mesh, when the work does not finish
// within its bound, when a stretch would move a corner off the map, when no
// edge of a cell over the cap may be collapsed, and when a cell's polygon
// ends up not simple or without positive area.
func Collapse(m *Mesh, minKm float64, degreeCap int) (*Mesh, error) {
	if !(minKm >= 0) || !finite(minKm) {
		return nil, fmt.Errorf("mesh: minimum edge %v km", minKm)
	}
	if degreeCap < 3 {
		return nil, fmt.Errorf("mesh: degree cap %d is below 3", degreeCap)
	}
	w := newWorkMesh(m, minKm)
	for e := range w.edges {
		w.push(e)
	}
	for {
		if err := w.drain(); err != nil {
			return nil, err
		}
		hit := -1
		for i := range w.polys {
			if w.neighborCount(i) > degreeCap {
				hit = i
				break
			}
		}
		if hit < 0 {
			break
		}
		best, bl := -1, 0.0
		for _, e := range w.sides[hit] { // ascending ids win ties below
			if w.edges[e].cells[1] == Boundary || !w.canCollapse(e) {
				continue
			}
			if l := w.length(e); best < 0 || l < bl || (l == bl && e < best) {
				best, bl = e, l
			}
		}
		if best < 0 {
			return nil, fmt.Errorf("mesh: cell %d has %d neighbors, over the cap of %d, and no edge the cap may collapse", hit, w.neighborCount(hit), degreeCap)
		}
		if err := w.collapse(best); err != nil {
			return nil, err
		}
		w.capHits++
	}
	return w.finish(m)
}

// workEdge is an edge of the mesh being collapsed. Its ids are the input
// mesh's; dead edges stay in place.
type workEdge struct {
	c         [2]int // corners
	cells     [2]int // as Edge.Cells
	alive     bool
	stretched bool
	ver       int // bumped whenever the edge's length may have changed
}

// workMesh is a mutable copy of a mesh's graph for Collapse. Corner and edge
// ids are the input's; a merged-away corner is dead (no cells).
type workMesh struct {
	cyl   topo.Cylinder
	minKm float64
	sites []topo.Point

	pos    []topo.Point
	orig   []topo.Point // the input corner positions
	bnd    []bool
	merged []int   // the corner a dead corner merged into, or its own id
	cCells [][]int // corner → cells, ascending
	cEdges [][]int // corner → edges, ascending
	polys  [][]int // cell → corners, clockwise
	sides  [][]int // cell → edges: sides[i][k] joins polys[i][k] to polys[i][k+1]
	edges  []workEdge

	q     edgeQueue
	ops   int
	limit int

	collapses, stretches, capHits int
}

func newWorkMesh(m *Mesh, minKm float64) *workMesh {
	w := &workMesh{cyl: m.cyl, minKm: minKm, limit: collapseOpsPerEdge * len(m.Edges)}
	for _, c := range m.Cells {
		w.sites = append(w.sites, c.Site)
		w.polys = append(w.polys, slices.Clone(c.Corners))
		w.sides = append(w.sides, slices.Clone(c.Edges))
	}
	for i, k := range m.Corners {
		w.pos = append(w.pos, k.Point)
		w.bnd = append(w.bnd, k.Boundary)
		w.merged = append(w.merged, i)
		w.cCells = append(w.cCells, slices.Clone(k.Cells))
		w.cEdges = append(w.cEdges, slices.Clone(k.Edges))
	}
	w.orig = slices.Clone(w.pos)
	for _, e := range m.Edges {
		w.edges = append(w.edges, workEdge{c: e.Corners, cells: e.Cells, alive: true})
	}
	return w
}

// queued is an entry in the edge queue: edge e had length l at version ver.
type queued struct {
	l   float64
	e   int
	ver int
}

// edgeQueue is a min-heap of edges by length, then edge id.
type edgeQueue []queued

func (q edgeQueue) Len() int { return len(q) }
func (q edgeQueue) Less(i, j int) bool {
	return cmp.Or(cmp.Compare(q[i].l, q[j].l), cmp.Compare(q[i].e, q[j].e)) < 0
}
func (q edgeQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *edgeQueue) Push(x any)   { *q = append(*q, x.(queued)) }
func (q *edgeQueue) Pop() any {
	old := *q
	x := old[len(old)-1]
	*q = old[:len(old)-1]
	return x
}

func (w *workMesh) length(e int) float64 {
	c := w.edges[e].c
	return w.cyl.Distance(w.pos[c[0]], w.pos[c[1]])
}

// push queues edge e at its current length, invalidating earlier entries.
func (w *workMesh) push(e int) {
	w.edges[e].ver++
	heap.Push(&w.q, queued{l: w.length(e), e: e, ver: w.edges[e].ver})
}

// pushAt queues every edge at corner c.
func (w *workMesh) pushAt(c int) {
	for _, e := range w.cEdges[c] {
		w.push(e)
	}
}

// drain collapses or stretches the queued edges shorter than minKm, shortest
// first, until none is left.
func (w *workMesh) drain() error {
	for w.q.Len() > 0 {
		it := heap.Pop(&w.q).(queued)
		ed := &w.edges[it.e]
		if !ed.alive || it.ver != ed.ver || !(it.l < w.minKm) {
			continue
		}
		var err error
		if w.canCollapse(it.e) {
			err = w.collapse(it.e)
		} else {
			err = w.stretch(it.e)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// other returns the cell across edge e from cell i.
func (w *workMesh) other(e, i int) int {
	if c := w.edges[e].cells; c[0] == i {
		return c[1]
	} else {
		return c[0]
	}
}

// neighborCount returns the number of distinct cells across cell i's edges.
func (w *workMesh) neighborCount(i int) int {
	nbs := make([]int, 0, len(w.sides[i]))
	for _, e := range w.sides[i] {
		if o := w.other(e, i); o != Boundary {
			nbs = append(nbs, o)
		}
	}
	slices.Sort(nbs)
	return len(slices.Compact(nbs))
}

// canCollapse reports whether edge e may be collapsed: the merged corner
// would touch at most 4 cells, and each cell beside the edge would keep at
// least 4 sides and 3 neighbors.
func (w *workMesh) canCollapse(e int) bool {
	ed := w.edges[e]
	a, b := ed.c[0], ed.c[1]
	n := len(w.cCells[a])
	for _, c := range w.cCells[b] {
		if _, found := slices.BinarySearch(w.cCells[a], c); !found {
			n++
		}
	}
	if n > 4 {
		return false
	}
	for _, i := range ed.cells {
		if i == Boundary {
			continue
		}
		if len(w.polys[i]) < 5 {
			return false
		}
		nb := w.neighborCount(i)
		if o := w.other(e, i); o != Boundary {
			shared := 0
			for _, f := range w.sides[i] {
				if w.other(f, i) == o {
					shared++
				}
			}
			if shared == 1 {
				nb--
			}
		}
		if nb < 3 {
			return false
		}
	}
	return true
}

// collapse merges edge e's corners into the lower id, at the edge's wrapped
// midpoint (on the boundary if either end is), and removes the edge.
func (w *workMesh) collapse(e int) error {
	if err := w.count(); err != nil {
		return err
	}
	ed := &w.edges[e]
	a, b := ed.c[0], ed.c[1]
	keep, gone := min(a, b), max(a, b)
	pa, pb := w.pos[a], w.pos[b]
	dx, dy := w.cyl.Delta(pa, pb)
	p := topo.Point{X: w.cyl.WrapX(pa.X + fmath.Mul(dx, 0.5)), Y: pa.Y + fmath.Mul(dy, 0.5)}
	switch {
	case w.bnd[a] && w.bnd[b]:
		if pa.Y != pb.Y {
			return fmt.Errorf("mesh: edge %d joins the north and south boundaries", e)
		}
		p.Y = pa.Y
	case w.bnd[a]:
		p.Y = pa.Y
	case w.bnd[b]:
		p.Y = pb.Y
	}

	ed.alive = false
	for _, i := range ed.cells {
		if i == Boundary {
			continue
		}
		// Side k runs from polys[i][k] to polys[i][k+1], the edge's two
		// ends. Dropping the side and its first corner keeps sides[i][k]
		// joining polys[i][k] to polys[i][k+1]; the corner that stays is
		// renamed to keep below.
		k := slices.Index(w.sides[i], e)
		w.sides[i] = slices.Delete(w.sides[i], k, k+1)
		w.polys[i] = slices.Delete(w.polys[i], k, k+1)
	}
	drop := func(s []int, v int) []int { return slices.DeleteFunc(s, func(x int) bool { return x == v }) }
	w.cEdges[a] = drop(w.cEdges[a], e)
	w.cEdges[b] = drop(w.cEdges[b], e)
	for _, f := range w.cEdges[gone] {
		for k, c := range w.edges[f].c {
			if c == gone {
				w.edges[f].c[k] = keep
			}
		}
	}
	for _, i := range w.cCells[gone] {
		for k, c := range w.polys[i] {
			if c == gone {
				w.polys[i][k] = keep
			}
		}
	}
	w.cEdges[keep] = slices.Sorted(slices.Values(slices.Concat(w.cEdges[keep], w.cEdges[gone])))
	w.cCells[keep] = slices.Compact(slices.Sorted(slices.Values(slices.Concat(w.cCells[keep], w.cCells[gone]))))
	w.cEdges[gone], w.cCells[gone] = nil, nil
	w.bnd[keep] = w.bnd[a] || w.bnd[b]
	w.merged[gone] = keep
	w.pos[keep] = p
	w.collapses++
	w.pushAt(keep)
	return nil
}

// stretch lengthens edge e to minKm·(1 + StretchMargin) along the
// perpendicular bisector of its two cells' sites (along x for an edge on the
// boundary or between two boundary corners), symmetrically about its
// midpoint; a boundary corner stays put and its partner moves the whole way.
func (w *workMesh) stretch(e int) error {
	if err := w.count(); err != nil {
		return err
	}
	ed := &w.edges[e]
	a, b := ed.c[0], ed.c[1]
	target := fmath.Mul(w.minKm, 1+StretchMargin)
	dx, dy := 1.0, 0.0
	if ed.cells[1] != Boundary && !(w.bnd[a] && w.bnd[b]) {
		sx, sy := w.cyl.Delta(w.sites[ed.cells[0]], w.sites[ed.cells[1]])
		dx, dy = -sy, sx
	}
	n := fmath.Hypot(dx, dy)
	dx, dy = dx/n, dy/n
	ex, ey := w.cyl.Delta(w.pos[a], w.pos[b])
	if fmath.Mul(dx, ex)+fmath.Mul(dy, ey) < 0 {
		dx, dy = -dx, -dy
	}
	at := func(p topo.Point, t float64) topo.Point {
		return topo.Point{X: w.cyl.WrapX(p.X + fmath.Mul(t, dx)), Y: p.Y + fmath.Mul(t, dy)}
	}
	pa, pb := w.pos[a], w.pos[b]
	mid := topo.Point{X: w.cyl.WrapX(pa.X + fmath.Mul(ex, 0.5)), Y: pa.Y + fmath.Mul(ey, 0.5)}
	switch {
	case w.bnd[a] && w.bnd[b]:
		w.pos[a], w.pos[b] = at(mid, -target/2), at(mid, target/2)
	case w.bnd[a]:
		w.pos[b] = at(pa, target)
	case w.bnd[b]:
		w.pos[a] = at(pb, -target)
	default:
		w.pos[a], w.pos[b] = at(mid, -target/2), at(mid, target/2)
	}
	h := w.cyl.H()
	for _, c := range []int{a, b} {
		if y := w.pos[c].Y; !w.bnd[c] && !(y > 0 && y < h) {
			return fmt.Errorf("mesh: stretching edge %d would move corner %d off the map (y %v)", e, c, y)
		}
	}
	ed.stretched = true
	w.stretches++
	w.pushAt(a)
	w.pushAt(b)
	return nil
}

// count counts one collapse or stretch against the bound.
func (w *workMesh) count() error {
	w.ops++
	if w.ops > w.limit {
		return fmt.Errorf("mesh: the short-edge collapse did not finish in %d steps", w.limit)
	}
	return nil
}

// finish renumbers the surviving graph canonically, builds and checks it,
// and fills in the report.
func (w *workMesh) finish(in *Mesh) (*Mesh, error) {
	var live []int
	for c := range w.pos {
		if w.cCells[c] != nil {
			live = append(live, c)
		}
	}
	pos := make([]topo.Point, len(live))
	for k, c := range live {
		pos[k] = w.pos[c]
	}
	id := make([]int, len(w.pos))
	corners := make([]Corner, len(live))
	for n, k := range byPosition(pos) {
		c := live[k]
		id[c] = n
		corners[n] = Corner{Point: w.pos[c], Boundary: w.bnd[c]}
	}
	polys := make([][]int, len(w.polys))
	across := make([][]int, len(w.polys))
	for i, poly := range w.polys {
		polys[i] = make([]int, len(poly))
		across[i] = make([]int, len(poly))
		for k, c := range poly {
			polys[i][k] = id[c]
			across[i][k] = w.other(w.sides[i][k], i)
		}
	}
	m, err := graph(w.cyl, w.sites, corners, polys, across)
	if err != nil {
		return nil, fmt.Errorf("%w (after the short-edge collapse)", err)
	}
	for i := range m.Cells {
		q := m.Offsets(i)
		if a, _ := areaCentroid(q); !(a > 0) || !finite(a) {
			return nil, fmt.Errorf("mesh: cell %d has area %v after the short-edge collapse", i, a)
		}
		if !simplePolygon(q) {
			return nil, fmt.Errorf("mesh: cell %d's polygon is not simple after the short-edge collapse", i)
		}
	}
	for e := range m.Edges {
		if l := m.EdgeLength(e); l < w.minKm {
			return nil, fmt.Errorf("mesh: edge %d is %v km after the short-edge collapse, below %v", e, l, w.minKm)
		}
	}
	for _, ed := range w.edges {
		if !ed.alive || !ed.stretched {
			continue
		}
		a, b := id[ed.c[0]], id[ed.c[1]]
		for _, f := range m.Corners[a].Edges {
			if c := m.Edges[f].Corners; c == [2]int{a, b} || c == [2]int{b, a} {
				m.Stretched = append(m.Stretched, f)
				break
			}
		}
	}
	slices.Sort(m.Stretched)
	for c, p := range w.orig {
		r := c
		for w.merged[r] != r {
			r = w.merged[r]
		}
		m.MaxShiftKm = max(m.MaxShiftKm, w.cyl.Distance(p, w.pos[r]))
	}
	m.Collapses, m.Stretches, m.DegreeCapHits = w.collapses, w.stretches, w.capHits
	m.GhostMarginKm = in.GhostMarginKm
	m.LloydCV = slices.Clone(in.LloydCV)
	return m, nil
}

// simplePolygon reports whether no two non-adjacent sides of the closed
// polygon q meet.
func simplePolygon(q []topo.Point) bool {
	n := len(q)
	for i := range n {
		for j := i + 2; j < n; j++ {
			if i == 0 && j == n-1 {
				continue // adjacent across the closing vertex
			}
			if segmentsMeet(q[i], q[(i+1)%n], q[j], q[(j+1)%n]) {
				return false
			}
		}
	}
	return true
}

// orient returns twice the signed area of the triangle a, b, c.
func orient(a, b, c topo.Point) float64 {
	return fmath.Mul(b.X-a.X, c.Y-a.Y) - fmath.Mul(b.Y-a.Y, c.X-a.X)
}

// segmentsMeet reports whether the closed segments ab and cd share a point.
func segmentsMeet(a, b, c, d topo.Point) bool {
	d1, d2 := orient(c, d, a), orient(c, d, b)
	d3, d4 := orient(a, b, c), orient(a, b, d)
	if ((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) && ((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0)) {
		return true
	}
	on := func(p, q, r topo.Point) bool { // r on segment pq, given collinear
		return min(p.X, q.X) <= r.X && r.X <= max(p.X, q.X) && min(p.Y, q.Y) <= r.Y && r.Y <= max(p.Y, q.Y)
	}
	return (d1 == 0 && on(c, d, a)) || (d2 == 0 && on(c, d, b)) || (d3 == 0 && on(a, b, c)) || (d4 == 0 && on(a, b, d))
}
