// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package measure

import (
	"cmp"
	"slices"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/world"
)

// chokepoint is one strait, neck or pass found on a graph.
type chokepoint struct {
	// a and b are the landmasses joined, a ≤ b; a == b for a strait within
	// one landmass, a neck, or a pass.
	a, b int
	// width is the narrowest crossing in cells, and cells the
	// chokepoint's cells, ascending.
	width int
	cells []int
}

// export returns c as measures.json lists it.
func (c chokepoint) export() world.Chokepoint {
	lms := []int{c.a}
	if c.b != c.a {
		lms = append(lms, c.b)
	}
	return world.Chokepoint{Width: c.width, Landmasses: lms, Cells: slices.Clone(c.cells)}
}

// hit is one cell on a qualifying crossing: the landmass pair (or the
// chain) the crossing belongs to, the cell, and the crossing's width.
type hit struct {
	a, b, cell, width int
}

// compareHits orders hits by pair, then cell, then width.
func compareHits(x, y hit) int {
	return cmp.Or(cmp.Compare(x.a, y.a), cmp.Compare(x.b, y.b), cmp.Compare(x.cell, y.cell), cmp.Compare(x.width, y.width))
}

// collect sorts hits, keeps each (pair, cell)'s narrowest, and groups each
// pair's cells by adjacency into chokepoints, ordered by pair and then
// lowest cell. A chokepoint's width is the narrowest of its cells.
func collect(g *graph, hits []hit, mark *scratch) []chokepoint {
	slices.SortFunc(hits, compareHits)
	hits = slices.CompactFunc(hits, func(x, y hit) bool { return x.a == y.a && x.b == y.b && x.cell == y.cell })
	width := make(map[int]int) // cell → width within the current pair; read by key only
	var out []chokepoint
	for lo := 0; lo < len(hits); {
		hi := lo
		clear(width)
		var members []int
		for ; hi < len(hits) && hits[hi].a == hits[lo].a && hits[hi].b == hits[lo].b; hi++ {
			members = append(members, hits[hi].cell)
			width[hits[hi].cell] = hits[hi].width
		}
		for _, set := range groups(g, members, mark) {
			w := width[set[0]]
			for _, c := range set[1:] {
				w = min(w, width[c])
			}
			out = append(out, chokepoint{a: hits[lo].a, b: hits[lo].b, width: w, cells: set})
		}
		lo = hi
	}
	return out
}

// entry is a cell reached from a shore, with its steps (cells crossed,
// itself included).
type entry struct{ cell, dist int }

// crossings joins two shores' reach lists (each ascending by cell): it
// appends a hit for every cell on a crossing of at most limit cells, that
// is every cell both reach with dist sum − 1 ≤ limit.
func crossings(hits []hit, a, b int, ra, rb []entry, limit int) []hit {
	for i, j := 0, 0; i < len(ra) && j < len(rb); {
		switch {
		case ra[i].cell < rb[j].cell:
			i++
		case ra[i].cell > rb[j].cell:
			j++
		default:
			if w := ra[i].dist + rb[j].dist - 1; w <= limit {
				hits = append(hits, hit{a: a, b: b, cell: ra[i].cell, width: w})
			}
			i++
			j++
		}
	}
	return hits
}

// straits finds the straits of g (adapted from wgvc seas.go
// assignStraits). From every coastal land cell a breadth-first search
// through playable water records, for each water cell within k cells, the
// cells crossed. Two shores a and b reaching a common water cell x make a
// crossing through x of width d_a(x) + d_b(x) − 1; it qualifies when that
// is at most k and a and b lie on different landmasses, or on one landmass
// with no land path of at most detour steps between them. The water cells
// on qualifying crossings, grouped by landmass pair and water adjacency,
// are the straits.
func straits(g *graph, lm landmasses, k, detour int) []chokepoint {
	n := g.cells()
	s, land := newScratch(n), newScratch(n)
	water := func(c int) bool { return g.water[c] }
	isLand := func(c int) bool { return g.land[c] }
	// reach[a] lists, ascending by cell, the water cells within k of
	// shore a.
	reach := make([][]entry, n)
	for a := range n {
		if !g.coastal(a) {
			continue
		}
		var r []entry
		s.gen++
		gen := s.gen
		q := s.queue[:0]
		for _, j := range g.nbr[a] {
			if g.water[j] {
				s.stamp[j], s.dist[j] = gen, 1
				q = append(q, j)
			}
		}
		for t := 0; t < len(q); t++ {
			x := q[t]
			r = append(r, entry{x, s.dist[x]})
			if s.dist[x] >= k {
				continue
			}
			for _, j := range g.nbr[x] {
				if s.stamp[j] != gen && water(j) {
					s.stamp[j], s.dist[j] = gen, s.dist[x]+1
					q = append(q, j)
				}
			}
		}
		s.queue = q
		slices.SortFunc(r, func(x, y entry) int { return cmp.Compare(x.cell, y.cell) })
		reach[a] = r
	}
	var hits []hit
	var shores []int
	for a := range n {
		if reach[a] == nil {
			continue
		}
		s.gen++
		shores = shores[:0]
		for _, e := range reach[a] {
			for _, b := range g.nbr[e.cell] {
				if b > a && g.land[b] && s.stamp[b] != s.gen {
					s.stamp[b] = s.gen
					shores = append(shores, b)
				}
			}
		}
		slices.Sort(shores)
		searched := false
		for _, b := range shores {
			A, B := lm.id[a], lm.id[b]
			if A == B {
				if !searched {
					land.search(g, a, detour, isLand)
					searched = true
				}
				if land.reached(b) {
					continue
				}
			}
			hits = crossings(hits, min(A, B), max(A, B), reach[a], reach[b], k)
		}
	}
	return collect(g, hits, s)
}

// necks finds the necks of g (adapted from wgvc seas.go assignNecks): the
// cuts of at most k land cells whose removal leaves two regions of at
// least minRegion cells. Width-1 cuts are articulation points (Tarjan's
// algorithm with subtree sizes); a cut of w ≥ 2 cells is a path of w
// adjacent land cells whose two end cells touch playable water, tested by
// removal, and counts only when no smaller subset of it is already a cut.
// Cuts that share or touch a cell merge into one neck, of the narrowest
// width, which must still split its landmass. Necks are ordered by
// landmass and then lowest cell.
func necks(g *graph, lm landmasses, k, minRegion int) []chokepoint {
	n := g.cells()
	t := &cutTester{g: g, lm: lm, minRegion: minRegion, removed: newScratch(n), s: newScratch(n)}
	var cuts []cutSet
	isCut := make(map[cutKey]bool) // read by key only
	for _, c := range articulationNecks(g, lm, minRegion) {
		cuts = append(cuts, cutSet{members: []int{c}, width: 1})
		isCut[keyOf([]int{c})] = true
	}
	tested := make(map[cutKey]bool) // read by key only
	path := make([]int, 0, k)
	var extend func(w int)
	extend = func(w int) {
		last := path[len(path)-1]
		if len(path) == w {
			if last < path[0] || !g.coastal(last) {
				return
			}
			members := slices.Sorted(slices.Values(path))
			key := keyOf(members)
			if tested[key] || containsCut(members, isCut) {
				return
			}
			tested[key] = true
			if t.splits(members, true) {
				cuts = append(cuts, cutSet{members: members, width: w})
				isCut[key] = true
			}
			return
		}
		for _, j := range g.nbr[last] {
			if g.land[j] && !slices.Contains(path, j) {
				path = append(path, j)
				extend(w)
				path = path[:len(path)-1]
			}
		}
	}
	for w := 2; w <= k; w++ {
		for a := range n {
			if g.coastal(a) {
				path = append(path[:0], a)
				extend(w)
			}
		}
	}
	slices.SortFunc(cuts, func(x, y cutSet) int {
		return cmp.Or(cmp.Compare(x.members[0], y.members[0]), cmp.Compare(x.width, y.width), slices.Compare(x.members, y.members))
	})
	var out []chokepoint
	for _, grp := range mergeTouchingCuts(g, cuts) {
		if t.splits(grp.members, false) {
			id := lm.id[grp.members[0]]
			out = append(out, chokepoint{a: id, b: id, width: grp.width, cells: grp.members})
		}
	}
	slices.SortFunc(out, func(x, y chokepoint) int {
		return cmp.Or(cmp.Compare(x.a, y.a), cmp.Compare(x.cells[0], y.cells[0]))
	})
	return out
}

// cutSet is a candidate or merged cut: its cells, ascending, and width.
type cutSet struct {
	members []int
	width   int
}

// cutKey is a cut's cells, ascending, padded with −1.
type cutKey [config.MaxChokepointCells]int

func keyOf(members []int) cutKey {
	var k cutKey
	for i := range k {
		k[i] = -1
	}
	copy(k[:], members)
	return k
}

// containsCut reports whether a proper, non-empty subset of members
// (ascending) is already a cut.
func containsCut(members []int, isCut map[cutKey]bool) bool {
	full := 1<<len(members) - 1
	sub := make([]int, 0, len(members))
	for mask := 1; mask < full; mask++ {
		sub = sub[:0]
		for i, c := range members {
			if mask&(1<<i) != 0 {
				sub = append(sub, c)
			}
		}
		if isCut[keyOf(sub)] {
			return true
		}
	}
	return false
}

// cutTester tests removals from a landmass.
type cutTester struct {
	g          *graph
	lm         landmasses
	minRegion  int
	removed, s *scratch
}

// localReach bounds the first, local search of a removal test: a removal
// whose neighbors all reconnect within this many steps splits nothing, and
// the whole landmass is searched only otherwise.
const localReach = 10

// splits reports whether removing members (one landmass's cells) leaves at
// least two regions of that landmass, the two largest each of at least
// minRegion cells when requireMinimum is set.
func (t *cutTester) splits(members []int, requireMinimum bool) bool {
	g, id := t.g, t.lm.id[members[0]]
	t.removed.gen++
	gen := t.removed.gen
	for _, c := range members {
		t.removed.stamp[c] = gen
	}
	open := func(c int) bool { return t.lm.id[c] == id && t.removed.stamp[c] != gen }
	var boundary []int
	for _, c := range members {
		for _, j := range g.nbr[c] {
			if open(j) && !slices.Contains(boundary, j) {
				boundary = append(boundary, j)
			}
		}
	}
	if len(boundary) < 2 {
		return false
	}
	slices.Sort(boundary)
	t.s.search(g, boundary[0], localReach, open)
	connected := true
	for _, b := range boundary[1:] {
		if !t.s.reached(b) {
			connected = false
			break
		}
	}
	if connected {
		return false
	}
	// Every region left touches the boundary, since the landmass was
	// connected; label them from it.
	var sizes []int
	t.s.gen++
	seen := t.s.gen
	q := t.s.queue[:0]
	for _, b := range boundary {
		if t.s.stamp[b] == seen {
			continue
		}
		t.s.stamp[b] = seen
		q = append(q[:0], b)
		for k := 0; k < len(q); k++ {
			for _, j := range g.nbr[q[k]] {
				if t.s.stamp[j] != seen && open(j) {
					t.s.stamp[j] = seen
					q = append(q, j)
				}
			}
		}
		sizes = append(sizes, len(q))
	}
	t.s.queue = q
	if len(sizes) < 2 {
		return false
	}
	slices.SortFunc(sizes, func(x, y int) int { return cmp.Compare(y, x) })
	return !requireMinimum || sizes[1] >= t.minRegion
}

// articulationNecks returns, ascending, the land cells whose removal
// leaves two regions of their landmass of at least minRegion cells each:
// the articulation points of the land graph, found by an iterative
// depth-first search (Tarjan) from each landmass's lowest cell, visiting
// neighbors in ascending order.
func articulationNecks(g *graph, lm landmasses, minRegion int) []int {
	n := g.cells()
	disc, low, sub := make([]int, n), make([]int, n), make([]int, n)
	for i := range disc {
		disc[i] = -1
	}
	// Per cell: the separated children's total size and the two largest.
	sepSum, sep1, sep2, sepN := make([]int, n), make([]int, n), make([]int, n), make([]int, n)
	type frame struct{ v, parent, next int }
	var cutsOut []int
	clock := 0
	for id, root := range lm.first {
		total := lm.size[id]
		stack := []frame{{v: root, parent: -1}}
		disc[root], low[root], sub[root] = clock, clock, 1
		clock++
		for len(stack) > 0 {
			f := &stack[len(stack)-1]
			v := f.v
			if f.next < len(g.nbr[v]) {
				w := g.nbr[v][f.next]
				f.next++
				if !g.land[w] || w == f.parent {
					continue
				}
				if disc[w] >= 0 {
					low[v] = min(low[v], disc[w])
					continue
				}
				disc[w], low[w], sub[w] = clock, clock, 1
				clock++
				stack = append(stack, frame{v: w, parent: v})
				continue
			}
			// v is finished: judge it, then report to its parent.
			parent := f.parent
			stack = stack[:len(stack)-1]
			r1, r2 := sep1[v], sep2[v]
			regions := sepN[v]
			if parent >= 0 && regions > 0 {
				rest := total - 1 - sepSum[v]
				regions++
				if rest > r1 {
					r1, r2 = rest, r1
				} else if rest > r2 {
					r2 = rest
				}
			}
			if regions >= 2 && r2 >= minRegion {
				cutsOut = append(cutsOut, v)
			}
			if p := parent; p >= 0 {
				sub[p] += sub[v]
				low[p] = min(low[p], low[v])
				if p == root || low[v] >= disc[p] {
					sepSum[p] += sub[v]
					sepN[p]++
					if s := sub[v]; s > sep1[p] {
						sep1[p], sep2[p] = s, sep1[p]
					} else if s > sep2[p] {
						sep2[p] = s
					}
				}
			}
		}
	}
	slices.Sort(cutsOut)
	return cutsOut
}

// mergeTouchingCuts unions the cuts (in order) that share or neighbor a
// cell and returns the merged sets in order of their first cut, each with
// its cells ascending and its narrowest width.
func mergeTouchingCuts(g *graph, cuts []cutSet) []cutSet {
	parent := make([]int, len(cuts))
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
	owner := make([]int, g.cells())
	for i := range owner {
		owner[i] = -1
	}
	for i, c := range cuts {
		for _, m := range c.members {
			for _, near := range append([]int{m}, g.nbr[m]...) {
				if o := owner[near]; o >= 0 {
					if ri, ro := find(i), find(o); ri != ro {
						parent[max(ri, ro)] = min(ri, ro)
					}
				}
			}
		}
		for _, m := range c.members {
			owner[m] = i
		}
	}
	var out []cutSet
	index := make([]int, len(cuts))
	for i, c := range cuts {
		r := find(i)
		if r == i {
			index[i] = len(out)
			out = append(out, cutSet{width: c.width})
		}
		o := &out[index[r]]
		o.width = min(o.width, c.width)
		o.members = append(o.members, c.members...)
	}
	for i := range out {
		slices.Sort(out[i].members)
		out[i].members = slices.Compact(out[i].members)
	}
	return out
}

// chains labels g's mountain chains: the connected sets of mountain cells
// with at least minCells cells, ids by lowest cell. It returns each cell's
// chain (−1 for none) and each chain's size.
func chains(g *graph, minCells int) (chain, size []int) {
	n := g.cells()
	chain = make([]int, n)
	seen := make([]bool, n)
	for i := range chain {
		chain[i] = -1
	}
	var q []int
	for i := range n {
		if !g.mountain[i] || seen[i] {
			continue
		}
		seen[i] = true
		q = append(q[:0], i)
		for k := 0; k < len(q); k++ {
			for _, j := range g.nbr[q[k]] {
				if g.mountain[j] && !seen[j] {
					seen[j] = true
					q = append(q, j)
				}
			}
		}
		if len(q) >= minCells {
			for _, c := range q {
				chain[c] = len(size)
			}
			size = append(size, len(q))
		}
	}
	return chain, size
}

// passes finds the passes of g's mountain chains. A flank cell is land in
// no chain. From each flank cell a, and each chain it enters by an edge of
// |grade| at most limit (tenths of a percent), a breadth-first search
// through that chain's cells, over edges of |grade| at most limit, records
// the chain cells within maxCells. Flank cells a and b reaching a common
// chain cell x (b also by low edges) make a route of d_a(x) + d_b(x) − 1
// chain cells; it qualifies when that is at most maxCells and no land path
// of at most detour steps avoiding the chain joins a and b. The chain cells
// on qualifying routes, grouped per chain by adjacency, are the passes,
// ordered by chain and then lowest cell; each pass's landmass is its
// chain's.
func passes(g *graph, lm landmasses, chain []int, maxCells int, limit float64, detour int) []chokepoint {
	n := g.cells()
	s, around := newScratch(n), newScratch(n)
	low := func(x, k int) bool { return float64(g.grade[x][k]) <= limit }
	flank := func(c int) bool { return g.land[c] && chain[c] < 0 }
	type key struct{ a, ch int }
	var keys []key
	var reach [][]entry
	for a := range n {
		if !flank(a) {
			continue
		}
		var touched []int
		for k, j := range g.nbr[a] {
			if chain[j] >= 0 && low(a, k) && !slices.Contains(touched, chain[j]) {
				touched = append(touched, chain[j])
			}
		}
		slices.Sort(touched)
		for _, ch := range touched {
			s.gen++
			gen := s.gen
			q := s.queue[:0]
			for k, j := range g.nbr[a] {
				if chain[j] == ch && low(a, k) {
					s.stamp[j], s.dist[j] = gen, 1
					q = append(q, j)
				}
			}
			var r []entry
			for t := 0; t < len(q); t++ {
				x := q[t]
				r = append(r, entry{x, s.dist[x]})
				if s.dist[x] >= maxCells {
					continue
				}
				for k, j := range g.nbr[x] {
					if s.stamp[j] != gen && chain[j] == ch && low(x, k) {
						s.stamp[j], s.dist[j] = gen, s.dist[x]+1
						q = append(q, j)
					}
				}
			}
			s.queue = q
			slices.SortFunc(r, func(x, y entry) int { return cmp.Compare(x.cell, y.cell) })
			keys = append(keys, key{a, ch})
			reach = append(reach, r)
		}
	}
	find := func(a, ch int) int {
		i, ok := slices.BinarySearchFunc(keys, key{a, ch}, func(x, y key) int {
			return cmp.Or(cmp.Compare(x.a, y.a), cmp.Compare(x.ch, y.ch))
		})
		if !ok {
			return -1
		}
		return i
	}
	var hits []hit
	var others []int
	for i, kk := range keys {
		a, ch := kk.a, kk.ch
		s.gen++
		others = others[:0]
		for _, e := range reach[i] {
			for k, b := range g.nbr[e.cell] {
				if b > a && flank(b) && low(e.cell, k) && s.stamp[b] != s.gen {
					s.stamp[b] = s.gen
					others = append(others, b)
				}
			}
		}
		if len(others) == 0 {
			continue
		}
		slices.Sort(others)
		around.search(g, a, detour, func(c int) bool { return g.land[c] && chain[c] != ch })
		for _, b := range others {
			if around.reached(b) {
				continue
			}
			if j := find(b, ch); j >= 0 {
				hits = crossings(hits, ch, ch, reach[i], reach[j], maxCells)
			}
		}
	}
	out := collect(g, hits, s)
	for i := range out {
		id := lm.id[out[i].cells[0]]
		out[i].a, out[i].b = id, id
	}
	return out
}

// chokepointMeasures measures the straits, necks and passes of g.
func chokepointMeasures(g *graph, lm landmasses, c config.Measures) (world.ChokepointMeasures, chokeMap) {
	k := c.Chokepoints.MaxCells
	st := straits(g, lm, k, c.Chokepoints.DetourCells)
	nk := necks(g, lm, k, c.Chokepoints.NeckMinRegionCells)
	chain, size := chains(g, c.Passes.ChainMinCells)
	ps := passes(g, lm, chain, c.Passes.MaxCells, c.Passes.MaxGradePercent*10, c.Passes.DetourCells)
	out := world.ChokepointMeasures{
		MaxCells:     k,
		Straits:      len(st),
		Necks:        len(nk),
		Chains:       len(size),
		Passes:       len(ps),
		StraitWidths: make([]int, k),
		NeckWidths:   make([]int, k),
		StraitList:   make([]world.Chokepoint, 0, len(st)),
		NeckList:     make([]world.Chokepoint, 0, len(nk)),
		PassList:     make([]world.Chokepoint, 0, len(ps)),
	}
	isletMax := c.Landmass.IsletMaxCells
	for _, x := range st {
		if x.a == x.b {
			out.StraitsWithin++
		} else {
			out.StraitsBetween++
		}
		if lm.size[x.a] > isletMax && lm.size[x.b] > isletMax {
			out.StraitsMajor++
		}
		out.StraitWidths[x.width-1]++
		out.StraitList = append(out.StraitList, x.export())
	}
	// A water cell can lie in straits of two landmass pairs; count it once.
	out.StraitCells = distinctCells(st)
	for _, x := range nk {
		out.NeckCells += len(x.cells)
		out.NeckWidths[x.width-1]++
		out.NeckList = append(out.NeckList, x.export())
	}
	for _, s := range size {
		out.ChainCells += s
	}
	withPass := make([]bool, len(size))
	for _, x := range ps {
		out.PassCells += len(x.cells)
		withPass[chain[x.cells[0]]] = true
		out.PassList = append(out.PassList, x.export())
	}
	for _, w := range withPass {
		if w {
			out.ChainsWithPass++
		}
	}
	return out, chokeMap{chain: chain, straits: st, necks: nk, passes: ps}
}

// chokeMap is what the landmass and chokepoint render draws besides the
// landmasses: each cell's chain, and the chokepoints.
type chokeMap struct {
	chain                  []int
	straits, necks, passes []chokepoint
}

// distinctCells counts the cells of cs, each once.
func distinctCells(cs []chokepoint) int {
	var all []int
	for _, c := range cs {
		all = append(all, c.cells...)
	}
	slices.Sort(all)
	return len(slices.Compact(all))
}
