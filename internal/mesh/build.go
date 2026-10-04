// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package mesh

import (
	"cmp"
	"fmt"
	"math"
	"slices"

	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/internal/mesh/voronoi"
	"github.com/mdhender/mpg/internal/topo"
)

// MergeKm is the corner merge distance: polygon vertices within this
// distance of each other (across the seam too) are one corner. The sweep
// computes a corner near the seam twice, once from the sites and once from
// their ghosts, and the two copies differ by rounding (far below a
// micrometre at world sizes); an edge shorter than MergeKm disappears, and
// its two cells meet only at the merged corner.
const MergeKm = 1e-6

// ghostMarginCells is the first ghost band tried, in mean site spacings.
const ghostMarginCells = 4

// Build returns the Voronoi mesh of sites on cyl. Every site must have X in
// [0, W) and Y in (0, H), and no two may coincide. Cell i is the cell of
// sites[i].
//
// It fails when a cell would wrap around the whole cylinder (a world only a
// few cells around), and when the sweep's output is inconsistent, which the
// checks here turn into an error rather than a bad graph.
func Build(cyl topo.Cylinder, sites []topo.Point) (*Mesh, error) {
	rings, margin, err := cellRings(cyl, sites)
	if err != nil {
		return nil, err
	}
	m, err := assemble(cyl, sites, rings)
	if err != nil {
		return nil, err
	}
	m.GhostMarginKm = margin
	return m, nil
}

// cellRings checks the sites and returns each site's swept polygon,
// clockwise and unwrapped about the site, with the ghost band that proved
// them exact: the sweep run with a band widened until it is (see the
// package documentation).
func cellRings(cyl topo.Cylinder, sites []topo.Point) (rings [][]seg, margin float64, err error) {
	if err := checkSites(cyl, sites); err != nil {
		return nil, 0, err
	}
	w := cyl.W()
	margin = min(ghostMarginCells*math.Sqrt(w*cyl.H()/float64(len(sites))), w)
	for {
		rings, ok, err := sweep(cyl, sites, margin)
		if err != nil {
			return nil, 0, err
		}
		if ok {
			return rings, margin, nil
		}
		if margin == w {
			return nil, 0, fmt.Errorf("mesh: a cell is too wide for the cylinder (%v km around, %d sites); use more sites", w, len(sites))
		}
		margin = min(2*margin, w)
	}
}

// checkSites checks the sites' ranges and that no two coincide.
func checkSites(cyl topo.Cylinder, sites []topo.Point) error {
	if len(sites) == 0 {
		return fmt.Errorf("mesh: no sites")
	}
	for i, s := range sites {
		if !(s.X >= 0 && s.X < cyl.W()) || !(s.Y > 0 && s.Y < cyl.H()) {
			return fmt.Errorf("mesh: site %d at (%v, %v) is outside [0, %v) × (0, %v)", i, s.X, s.Y, cyl.W(), cyl.H())
		}
	}
	order := byPosition(sites)
	for k := 1; k < len(order); k++ {
		if a, b := order[k-1], order[k]; sites[a] == sites[b] {
			return fmt.Errorf("mesh: sites %d and %d coincide at (%v, %v)", min(a, b), max(a, b), sites[a].X, sites[a].Y)
		}
	}
	return nil
}

// byPosition returns the indices of pts ordered by Y, then X, then index.
func byPosition(pts []topo.Point) []int {
	order := make([]int, len(pts))
	for i := range order {
		order[i] = i
	}
	slices.SortFunc(order, func(a, b int) int {
		return cmp.Or(cmp.Compare(pts[a].Y, pts[b].Y), cmp.Compare(pts[a].X, pts[b].X), cmp.Compare(a, b))
	})
	return order
}

// seg is one side of a swept cell polygon: it runs from P (unwrapped, as the
// sweep computed it) to the next seg's P, and Across is the site on its
// other side, or Boundary.
type seg struct {
	P      topo.Point
	Across int
}

// sweep runs Fortune's sweep over the sites plus ghost copies within margin
// of the seam, in the box [−margin, W + margin] × [0, H], and returns each
// site's polygon, clockwise. ok is false when the band was too narrow to
// be sure of every cell (see the package documentation).
func sweep(cyl topo.Cylinder, sites []topo.Point, margin float64) (rings [][]seg, ok bool, err error) {
	w, h := cyl.W(), cyl.H()
	n := len(sites)
	vs := make([]voronoi.Vertex, 0, n+n/4)
	var ghostOf []int
	for _, s := range sites {
		vs = append(vs, voronoi.Vertex{X: s.X, Y: s.Y})
	}
	for i, s := range sites { // east copies, of the sites near x = 0
		if s.X < margin {
			vs = append(vs, voronoi.Vertex{X: s.X + w, Y: s.Y})
			ghostOf = append(ghostOf, i)
		}
	}
	for i, s := range sites { // west copies, of the sites near x = W
		if s.X >= w-margin {
			vs = append(vs, voronoi.Vertex{X: s.X - w, Y: s.Y})
			ghostOf = append(ghostOf, i)
		}
	}
	d, err := voronoi.ComputeDiagram(vs, voronoi.NewBBox(-margin, w+margin, 0, h), true)
	if err != nil {
		return nil, false, fmt.Errorf("mesh: %w", err)
	}

	rings = make([][]seg, n)
	for _, c := range d.Cells {
		if c.Index >= n {
			continue // a ghost's cell
		}
		ring := make([]seg, 0, len(c.Halfedges))
		for _, he := range c.Halfedges {
			p := he.GetStartpoint()
			across := Boundary
			if o := he.Edge.GetOtherCell(c); o != nil {
				across = o.Index
				if across >= n {
					across = ghostOf[across-n]
				}
			}
			ring = append(ring, seg{P: topo.Point{X: p.X, Y: p.Y}, Across: across})
		}
		rings[c.Index] = ring
	}

	for i, ring := range rings {
		if len(ring) < 3 {
			return nil, false, fmt.Errorf("mesh: the sweep gave site %d a polygon of %d sides", i, len(ring))
		}
		s := sites[i]
		// The sweep's cell is exact if no site left out of it is closer to
		// any of its vertices than its own site is. The nearest left-out
		// sites are at x ≥ W + margin and x < −margin.
		for _, sg := range ring {
			p := sg.P
			if !finite(p.X) || !finite(p.Y) {
				return nil, false, fmt.Errorf("mesh: the sweep gave site %d a non-finite vertex", i)
			}
			r := fmath.Hypot(p.X-s.X, p.Y-s.Y)
			if !(p.X+r < w+margin && p.X-r > -margin) {
				return nil, false, nil
			}
		}
		if signedArea2(s, ring) < 0 {
			reverse(ring)
		}
	}
	return rings, true, nil
}

// signedArea2 returns twice the signed area of the ring, relative to s:
// positive when the ring runs clockwise on the map (x east, y south).
func signedArea2(s topo.Point, ring []seg) float64 {
	var sum float64
	for k, a := range ring {
		b := ring[(k+1)%len(ring)].P
		sum += fmath.Mul(a.P.X-s.X, b.Y-s.Y) - fmath.Mul(b.X-s.X, a.P.Y-s.Y)
	}
	return sum
}

// reverse reverses the ring's direction in place: the vertices run the
// other way, and each side keeps the site across it.
func reverse(ring []seg) {
	n := len(ring)
	across := make([]int, n)
	for k := range ring {
		across[k] = ring[k].Across
	}
	slices.Reverse(ring)
	// The side from new vertex j to j+1 is the old side from vertex
	// n−2−j to n−1−j.
	for j := range ring {
		ring[j].Across = across[fmath.FloorMod(n-2-j, n)]
	}
}

// halfEdge is one side of one cell polygon in corner ids.
type halfEdge struct {
	lo, hi int // the corners, lower first
	a, b   int // the corners in the cell's clockwise order
	cell   int
	across int
	k      int // the side's position in the cell's polygon
}

// assemble merges the polygons' vertices into corners and builds the graph.
func assemble(cyl topo.Cylinder, sites []topo.Point, rings [][]seg) (*Mesh, error) {
	var pts []topo.Point // every polygon vertex, x wrapped
	first := make([]int, len(rings)+1)
	for i, ring := range rings {
		first[i] = len(pts)
		for _, sg := range ring {
			pts = append(pts, topo.Point{X: cyl.WrapX(sg.P.X), Y: sg.P.Y})
		}
	}
	first[len(rings)] = len(pts)

	cornerOf, corners := mergeCorners(cyl, pts)

	m := &Mesh{cyl: cyl, Cells: make([]Cell, len(sites)), Corners: corners}
	var hes []halfEdge
	acrossOf := make([][]int, len(sites))
	for i, ring := range rings {
		var ids, across []int
		for k, sg := range ring {
			c := cornerOf[first[i]+k]
			next := cornerOf[first[i]+(k+1)%len(ring)]
			if c != next { // a side shorter than MergeKm is dropped
				ids = append(ids, c)
				across = append(across, sg.Across)
			}
		}
		if len(ids) < 3 {
			return nil, fmt.Errorf("mesh: cell %d collapsed to %d corners", i, len(ids))
		}
		if sorted := slices.Sorted(slices.Values(ids)); len(slices.Compact(sorted)) != len(ids) {
			return nil, fmt.Errorf("mesh: cell %d passes through a corner twice", i)
		}
		start := slices.Index(ids, slices.Min(ids))
		ids = append(ids[start:], ids[:start]...)
		across = append(across[start:], across[:start]...)
		for k, a := range ids {
			b := ids[(k+1)%len(ids)]
			if across[k] == i {
				return nil, fmt.Errorf("mesh: cell %d borders itself across the seam; the cylinder is too narrow for its cells", i)
			}
			hes = append(hes, halfEdge{lo: min(a, b), hi: max(a, b), a: a, b: b, cell: i, across: across[k], k: k})
		}
		m.Cells[i] = Cell{Site: sites[i], Corners: ids, Edges: make([]int, len(ids))}
		acrossOf[i] = across
	}

	// Group the half-edges by their corners: each group is one edge, with
	// two cells that agree about each other, or one rim cell on the
	// boundary.
	slices.SortFunc(hes, func(x, y halfEdge) int {
		return cmp.Or(cmp.Compare(x.lo, y.lo), cmp.Compare(x.hi, y.hi), cmp.Compare(x.cell, y.cell), cmp.Compare(x.k, y.k))
	})
	for g := 0; g < len(hes); {
		end := g + 1
		for end < len(hes) && hes[end].lo == hes[g].lo && hes[end].hi == hes[g].hi {
			end++
		}
		group := hes[g:end]
		e := len(m.Edges)
		t := group[0]
		switch len(group) {
		case 1:
			ya, yb := corners[t.a].Point.Y, corners[t.b].Point.Y
			if t.across != Boundary || !corners[t.a].Boundary || ya != yb {
				return nil, fmt.Errorf("mesh: the edge from corner %d to %d has only cell %d (across %d) but is not on the rim boundary", t.a, t.b, t.cell, t.across)
			}
			m.Edges = append(m.Edges, Edge{Cells: [2]int{t.cell, Boundary}, Corners: [2]int{t.a, t.b}})
		case 2:
			u := group[1]
			if t.across != u.cell || u.across != t.cell || t.cell == u.cell || t.a != u.b || t.b != u.a {
				return nil, fmt.Errorf("mesh: cells %d and %d disagree about the edge from corner %d to %d", t.cell, u.cell, t.a, t.b)
			}
			m.Edges = append(m.Edges, Edge{Cells: [2]int{t.cell, u.cell}, Corners: [2]int{t.a, t.b}})
		default:
			return nil, fmt.Errorf("mesh: %d cell sides join corners %d and %d", len(group), t.lo, t.hi)
		}
		for _, x := range group {
			m.Cells[x.cell].Edges[x.k] = e
		}
		g = end
	}

	for i := range m.Cells {
		var nbs []int
		for _, a := range acrossOf[i] {
			if a != Boundary {
				nbs = append(nbs, a)
			}
		}
		slices.Sort(nbs)
		m.Cells[i].Neighbors = slices.Clip(slices.Compact(nbs))
		for _, c := range m.Cells[i].Corners {
			m.Corners[c].Cells = append(m.Corners[c].Cells, i) // i ascends
		}
	}
	for e, edge := range m.Edges {
		for _, c := range edge.Corners {
			m.Corners[c].Edges = append(m.Corners[c].Edges, e) // e ascends
		}
	}
	for c, corner := range m.Corners {
		if len(corner.Cells) == 0 {
			return nil, fmt.Errorf("mesh: corner %d touches no cell", c)
		}
	}
	return m, nil
}

// mergeCorners merges the points (x wrapped) that lie within MergeKm of
// each other, across the seam too, into corners. It returns each point's
// corner id and the corners, numbered by Y, then X. A corner's position is
// that of its lowest-numbered point, with Y set to exactly 0 or H when it
// is within MergeKm of either.
func mergeCorners(cyl topo.Cylinder, pts []topo.Point) (cornerOf []int, corners []Corner) {
	parent := make([]int, len(pts))
	for i := range parent {
		parent[i] = i
	}
	find := func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	// Points are bucketed on a MergeKm grid, wrapped east–west; a point
	// can only merge with points in its own or the eight surrounding
	// buckets. The map is only looked up, never iterated, and points are
	// visited in index order, so the result does not depend on it.
	type bucket struct{ x, y int64 }
	nbx := int64(math.Ceil(cyl.W() / MergeKm))
	grid := make(map[bucket][]int, len(pts))
	for i, p := range pts {
		bx := min(int64(math.Floor(p.X/MergeKm)), nbx-1)
		by := int64(math.Floor(p.Y / MergeKm))
		for dx := int64(-1); dx <= 1; dx++ {
			for dy := int64(-1); dy <= 1; dy++ {
				for _, j := range grid[bucket{fmath.FloorMod(bx+dx, nbx), by + dy}] {
					if cyl.Distance(p, pts[j]) <= MergeKm {
						ri, rj := find(i), find(j)
						parent[max(ri, rj)] = min(ri, rj)
					}
				}
			}
		}
		b := bucket{bx, by}
		grid[b] = append(grid[b], i)
	}

	var roots []int
	for i := range pts {
		if find(i) == i {
			roots = append(roots, i)
		}
	}
	h := cyl.H()
	pos := make([]topo.Point, len(roots))
	for k, r := range roots {
		p := pts[r]
		switch {
		case math.Abs(p.Y) <= MergeKm:
			p.Y = 0
		case math.Abs(p.Y-h) <= MergeKm:
			p.Y = h
		}
		pos[k] = p
	}
	order := byPosition(pos)
	id := make([]int, len(pts)) // corner id by root point
	corners = make([]Corner, len(roots))
	for c, k := range order {
		id[roots[k]] = c
		corners[c] = Corner{Point: pos[k], Boundary: pos[k].Y == 0 || pos[k].Y == h}
	}
	cornerOf = make([]int, len(pts))
	for i := range pts {
		cornerOf[i] = id[find(i)]
	}
	return cornerOf, corners
}

// finite reports whether v is finite.
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
