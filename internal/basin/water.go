// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package basin

import (
	"cmp"
	"encoding/binary"
	"fmt"
	"math"
	"slices"

	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/internal/mesh"
)

// Params are the water balance's settings (config group basin).
type Params struct {
	// SeepageMM is the seepage from every lake cell, in mm per year
	// (basin.seepage_mm).
	SeepageMM float64
	// SaltEvapShare is the least share of a closed lake's losses that
	// evaporation must take for the lake to be salt (basin.salt_evap_share).
	SaltEvapShare float64
	// InlandSeaMinCells is the size from which a lake is an inland sea
	// (basin.inland_sea_min_cells).
	InlandSeaMinCells int
}

// Climate is the per-cell climate the water balance reads, each in mm per
// year and indexed by cell id (package climate's Result fields of the same
// names).
type Climate struct {
	Precipitation, PET, Runoff []float64
}

// State is a basin's water state.
type State uint8

// The basin states.
const (
	// Dry: the basin holds no water of its own. A nested basin's children
	// may still hold lakes.
	Dry State = iota
	// Partial: the basin's water stands below its spill level.
	Partial
	// Full: the basin is filled to its spill level and overflows.
	Full
)

var stateNames = [...]string{"dry", "partial", "full"}

// String returns the state's name.
func (s State) String() string {
	if int(s) < len(stateNames) {
		return stateNames[s]
	}
	return fmt.Sprintf("State(%d)", uint8(s))
}

// Kind is a lake's kind.
type Kind uint8

// The lake kinds.
const (
	// KindLake is a lake of 1 to InlandSeaMinCells − 1 cells.
	KindLake Kind = iota
	// KindInlandSea is a lake of InlandSeaMinCells cells or more.
	KindInlandSea
)

var kindNames = [...]string{"lake", "inland-sea"}

// String returns the kind's name, as world.json spells the water kind.
func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return fmt.Sprintf("Kind(%d)", uint8(k))
}

// Water is one basin's outcome of the water balance. Volumes are in
// mm·km² per year (10³ m³ per year).
type Water struct {
	// State is the basin's water state.
	State State
	// LevelM is the basin's water level: the highest altitude it filled,
	// its spill level when full, its bottom's altitude when dry.
	LevelM float64
	// Filled lists the cells this basin filled itself, in fill order; the
	// cells of its children, filled by them, are not repeated.
	Filled []int
	// Inflow is the runoff of the catchment cells that drain to this
	// basin directly (Lakes.Sink).
	Inflow float64
	// Overflow is the water that left through the spill corner.
	Overflow float64
	// OverflowTo is where overflow goes, or would go: the parent, for a
	// nested basin; for a top-level basin, the basin its spill drains to,
	// or None for the sea.
	OverflowTo int
	// OverflowVia is, for a top-level basin, the cell outside it down
	// whose descent (Lakes.Down) its overflow runs from the spill: the
	// lowest cell, the sea first, next to the flat at the spill level
	// around its spill cell. It is None for a nested basin, whose overflow
	// stays in its parent.
	OverflowVia int
	// Remainder is the water the basin received but could not place: too
	// little to fill its next cell (the whole inflow of a playa).
	Remainder float64
	// Lake is the lake holding this basin's water, or None.
	Lake int
}

// Lake is one lake: a connected set of cells with one surface level, the
// water of one basin and every basin nested in it. Volumes are in mm·km²
// per year.
type Lake struct {
	// Basin is the outermost basin whose water the lake is.
	Basin int
	// Cells lists the lake's cells, ascending.
	Cells []int
	// SurfaceM is the surface level: the highest altitude among its cells,
	// or the basin's spill level when full.
	SurfaceM float64
	// Kind is KindInlandSea from InlandSeaMinCells cells, else KindLake.
	Kind Kind
	// Full reports whether the basin is filled to its spill level.
	Full bool
	// Salt reports a closed lake (no overflow) whose evaporation is at
	// least SaltEvapShare of its evaporation and seepage, and positive.
	Salt bool
	// Runoff is the runoff of the land cells draining into the lake;
	// Precipitation falls on the lake; Received is overflow from other
	// lakes and basins.
	Runoff, Precipitation, Received float64
	// Evaporation (the lake cells' PET) and Seepage leave the lake;
	// Overflow leaves through Outlet; Remainder is water the lake could
	// not place (less than its next cell needs).
	Evaporation, Seepage, Overflow, Remainder float64
	// Outlet is the basin's spill corner when the lake overflows, or None.
	Outlet int
}

// Playa is a basin with no stable water level: dry land, surface playa on
// its bottom cell, and a dry sink for rivers.
type Playa struct {
	// Basin is the basin, Cell its bottom cell, and Corner the dry sink:
	// the cell's lowest corner by corner height, ties to the lower id.
	Basin, Cell, Corner int
}

// Lakes is the water balance of a basin hierarchy.
type Lakes struct {
	// Params are the settings it was computed with.
	Params Params
	// Down is each cell's downstream neighbor by steepest descent on the
	// routing height (Result.RouteM), flats resolved toward their exits;
	// None for sea cells and pits.
	Down []int
	// Sink is the basin each cell's runoff reaches: the basin of the pit
	// its descent ends in; None for cells that drain to the sea and for
	// sea cells.
	Sink []int
	// Stray counts the cells whose descent ends in a pit in no basin; their
	// runoff is counted as reaching the sea. It is 0 on every world seen.
	Stray int
	// Water is each basin's outcome, by basin id.
	Water []Water
	// Lakes lists the lakes in the order of their basin ids.
	Lakes []Lake
	// Lake is each cell's lake, or None.
	Lake []int
	// Playas lists the playas in the order of their basin ids.
	Playas []Playa
	// Inflow is the water entering the basins: the runoff of the land
	// cells draining to them plus the precipitation on the lakes.
	// Evaporation, Seepage, ToSea (overflow reaching the sea) and Unplaced
	// (every basin's remainder) are where it goes. Residual is
	// (Inflow − the four) / Inflow, 0 with no inflow.
	Inflow, Evaporation, Seepage, ToSea, Unplaced, Residual float64
}

// ConservationTolerance is the largest relative water-balance residual the
// tests accept, for the whole map and for each lake.
const ConservationTolerance = 1e-9

// Counts returns the number of lakes (KindLake), inland seas, salt lakes and
// seas, and full basins.
func (l *Lakes) Counts() (lakes, seas, salt, full int) {
	for _, k := range l.Lakes {
		if k.Kind == KindInlandSea {
			seas++
		} else {
			lakes++
		}
		if k.Salt {
			salt++
		}
	}
	for _, w := range l.Water {
		if w.State == Full {
			full++
		}
	}
	return
}

// Cells returns the number of lake cells.
func (l *Lakes) Cells() int {
	n := 0
	for _, k := range l.Lakes {
		n += len(k.Cells)
	}
	return n
}

// Balance runs the water balance (see the package documentation) on the
// basin hierarchy r of m's cells with altitudes alt, sea cells seed (as
// given to Find), cell areas area in km², climate cl and settings p. It
// reads its inputs and modifies none of them.
func Balance(m *mesh.Mesh, alt []float64, seed []bool, area []float64, r *Result, cl Climate, p Params) (*Lakes, error) {
	n := len(m.Cells)
	switch {
	case len(alt) != n || len(seed) != n || len(area) != n || len(r.Of) != n:
		return nil, fmt.Errorf("basin: balance inputs do not match %d cells", n)
	case len(cl.Precipitation) != n || len(cl.PET) != n || len(cl.Runoff) != n:
		return nil, fmt.Errorf("basin: climate does not match %d cells", n)
	case !(p.SeepageMM >= 0) || math.IsInf(p.SeepageMM, 1):
		return nil, fmt.Errorf("basin: seepage %v mm must be non-negative and finite", p.SeepageMM)
	case !(p.SaltEvapShare >= 0 && p.SaltEvapShare <= 1):
		return nil, fmt.Errorf("basin: salt evaporation share %v must be in [0, 1]", p.SaltEvapShare)
	case p.InlandSeaMinCells < 1:
		return nil, fmt.Errorf("basin: inland sea minimum %d cells must be positive", p.InlandSeaMinCells)
	}
	for c := range n {
		for _, v := range [...]float64{alt[c], area[c], cl.Precipitation[c], cl.PET[c], cl.Runoff[c]} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return nil, fmt.Errorf("basin: cell %d has a value %v that is not finite", c, v)
			}
		}
	}
	w := &water{m: m, alt: alt, seed: seed, area: area, r: r, cl: cl, p: p}
	w.run()
	return w.res, nil
}

// water is the state of one Balance.
type water struct {
	m    *mesh.Mesh
	alt  []float64
	seed []bool
	area []float64
	r    *Result
	cl   Climate
	p    Params

	res *Lakes
	// tin and tout are each basin's interval in a depth-first order of
	// the hierarchy: basin x is in b's subtree when tin[b] ≤ tin[x] and
	// tout[x] ≤ tout[b].
	tin, tout []int
	cost      []float64 // per cell: what turning it into lake costs
	filled    []bool    // per cell
	by        []int     // per cell: the basin that filled it, or None
	bs        []wbasin
	toSea     []float64 // per basin: its overflow that reached the sea
	moves     []move
}

// wbasin is one basin's working state.
type wbasin struct {
	started  bool // filling its own cells (all children full)
	full     bool
	fullKids int
	budget   float64
	level    float64
	front    cellHeap
	target   int // top-level overflow target, once computed
	targeted bool
}

// move is water landing in a basin's budget, or passing through a full
// one, from the overflow of basin from.
type move struct {
	from, to int
	w        float64
}

func (w *water) run() {
	r, n := w.r, len(w.m.Cells)
	nb := len(r.Basins)
	w.res = &Lakes{
		Params: w.p,
		Water:  make([]Water, nb),
		Lake:   make([]int, n),
	}
	w.descend()
	w.order()
	// The cost of a lake cell: it stops running off (R), takes its
	// precipitation (P), and loses PET and seepage, per unit area:
	// PET − (P − R) + S = PET − AET + S, at least S.
	w.cost = make([]float64, n)
	for c := range n {
		aet := w.cl.Precipitation[c] - w.cl.Runoff[c]
		w.cost[c] = fmath.Mul(w.area[c], w.cl.PET[c]-aet+w.p.SeepageMM)
	}
	w.filled = make([]bool, n)
	w.by = make([]int, n)
	for c := range n {
		w.by[c] = None
	}
	w.bs = make([]wbasin, nb)
	w.toSea = make([]float64, nb)
	for b := range nb {
		w.bs[b].level = r.Basins[b].BottomM
		w.res.Water[b].OverflowTo = r.Basins[b].Parent
		w.res.Water[b].OverflowVia = None
		w.res.Water[b].Lake = None
	}
	// Direct inflow, in cell order, waits in each basin's budget.
	for c := range n {
		if b := w.res.Sink[c]; b != None {
			v := fmath.Mul(w.cl.Runoff[c], w.area[c])
			w.res.Water[b].Inflow += v
			w.bs[b].budget += v
		}
	}
	// Top-level trees by descending spill level, ties to the lower id;
	// within a tree, children before parents (id order).
	var tops []int
	for b := range nb {
		if r.Basins[b].Parent == None {
			tops = append(tops, b)
		}
	}
	slices.SortFunc(tops, func(a, b int) int {
		return cmp.Or(cmp.Compare(r.Basins[b].SpillM, r.Basins[a].SpillM), cmp.Compare(a, b))
	})
	for _, t := range tops {
		for b := range nb {
			if w.within(t, b) {
				w.settle(b)
			}
		}
	}
	w.collect()
}

// within reports whether basin x is in basin b's subtree.
func (w *water) within(b, x int) bool { return w.tin[b] <= w.tin[x] && w.tout[x] <= w.tout[b] }

// inRegion reports whether cell c is in basin b or a basin nested in it.
func (w *water) inRegion(b, c int) bool {
	x := w.r.Of[c]
	return x != None && w.within(b, x)
}

// order numbers the hierarchy depth first.
func (w *water) order() {
	nb := len(w.r.Basins)
	w.tin, w.tout = make([]int, nb), make([]int, nb)
	k := 0
	var visit func(b int)
	visit = func(b int) {
		w.tin[b] = k
		k++
		for _, ch := range w.r.Basins[b].Children {
			visit(ch)
		}
		w.tout[b] = k
		k++
	}
	for b := range nb {
		if w.r.Basins[b].Parent == None {
			visit(b)
		}
	}
}

// lower reports whether cell a is lower than cell b for routing: the sea
// lowest, then by routing height, then by id.
func (w *water) lower(a, b int) bool {
	sa, sb := w.seed[a], w.seed[b]
	if sa != sb {
		return sa
	}
	if !sa {
		if ha, hb := w.r.RouteM[a], w.r.RouteM[b]; ha != hb {
			return ha < hb
		}
	}
	return a < b
}

// descend sets Down and Sink: steepest descent on the routing height, with
// flats drained breadth first toward their exits.
func (w *water) descend() {
	m, r, n := w.m, w.r, len(w.m.Cells)
	h := r.RouteM
	down := make([]int, n)
	for c := range n {
		down[c] = None
	}
	flatOf := make([]int, n)
	for c := range n {
		flatOf[c] = None
	}
	var flat, queue []int
	pit := make([]bool, n)
	for c := range n {
		if w.seed[c] || flatOf[c] != None {
			continue
		}
		// The flat: c and the non-sea cells joined to it at its height.
		flat = append(flat[:0], c)
		flatOf[c] = c
		for i := 0; i < len(flat); i++ {
			for _, y := range m.Cells[flat[i]].Neighbors {
				if !w.seed[y] && flatOf[y] == None && h[y] == h[c] {
					flatOf[y] = c
					flat = append(flat, y)
				}
			}
		}
		slices.Sort(flat)
		// Exits drain to their lowest lower neighbor.
		queue = queue[:0]
		for _, x := range flat {
			best := None
			for _, y := range m.Cells[x].Neighbors {
				if flatOf[y] == c {
					continue
				}
				if (w.seed[y] || h[y] < h[x]) && (best == None || w.lower(y, best)) {
					best = y
				}
			}
			if best != None {
				down[x] = best
				queue = append(queue, x)
			}
		}
		if len(queue) == 0 {
			pit[flat[0]] = true
			queue = append(queue, flat[0])
		}
		// The rest drain breadth first toward the exits. Cells of the flat
		// are marked as reached by setting flatOf to −2 − c.
		for _, x := range queue {
			flatOf[x] = -2 - c
		}
		for i := 0; i < len(queue); i++ {
			x := queue[i]
			for _, y := range m.Cells[x].Neighbors {
				if flatOf[y] == c {
					flatOf[y] = -2 - c
					down[y] = x
					queue = append(queue, y)
				}
			}
		}
	}
	// Sinks: follow each cell's descent to the sea or a pit.
	sink := make([]int, n)
	done := make([]bool, n)
	stray := make([]bool, n) // the descent ends in a pit in no basin
	var path []int
	for c := range n {
		if w.seed[c] {
			sink[c], done[c] = None, true
		}
	}
	for c := range n {
		path = path[:0]
		x := c
		for !done[x] && !pit[x] {
			path = append(path, x)
			x = down[x]
		}
		if !done[x] { // a pit
			sink[x], stray[x], done[x] = r.Of[x], r.Of[x] == None, true
		}
		for _, y := range path {
			sink[y], stray[y], done[y] = sink[x], stray[x], true
		}
	}
	for c := range n {
		if stray[c] {
			w.res.Stray++
		}
	}
	w.res.Down, w.res.Sink = down, sink
}

// settle fills basin b with what it holds when its children are full, or
// passes a parent's own inflow (a pit among its own cells) to its children.
func (w *water) settle(b int) {
	x := &w.bs[b]
	if x.full {
		return
	}
	if w.allFull(b) {
		w.start(b)
		w.fill(b)
		return
	}
	if v := x.budget; v > 0 {
		x.budget = 0
		w.give(b, v, None)
	}
}

// allFull reports whether every child of b is full (true for a leaf).
func (w *water) allFull(b int) bool { return w.bs[b].fullKids == len(w.r.Basins[b].Children) }

// start begins b's own fill: from its bottom cell for a leaf, or from the
// shore of its full children.
func (w *water) start(b int) {
	x := &w.bs[b]
	if x.started {
		return
	}
	x.started = true
	bs := &w.r.Basins[b]
	if len(bs.Children) == 0 {
		x.front.push(w.alt, bs.Bottom)
		return
	}
	for _, ch := range bs.Children {
		x.level = max(x.level, w.r.Basins[ch].SpillM)
	}
	for _, c := range w.regionCells(b) {
		if !w.filled[c] {
			continue
		}
		for _, y := range w.m.Cells[c].Neighbors {
			if !w.filled[y] && w.inRegion(b, y) {
				x.front.push(w.alt, y)
			}
		}
	}
}

// regionCells returns the cells of b and every basin nested in it, in no
// particular order.
func (w *water) regionCells(b int) []int {
	var out []int
	var walk func(b int)
	walk = func(b int) {
		out = append(out, w.r.Basins[b].Cells...)
		for _, ch := range w.r.Basins[b].Children {
			walk(ch)
		}
	}
	walk(b)
	return out
}

// fill fills b's cells, lowest first, while its budget covers the next
// cell's cost; when no cell is left it is full and overflows.
func (w *water) fill(b int) {
	x := &w.bs[b]
	for {
		for x.front.len() > 0 && w.filled[x.front.top()] {
			x.front.pop(w.alt)
		}
		if x.front.len() == 0 {
			x.full = true
			x.level = w.r.Basins[b].SpillM
			v := x.budget
			x.budget = 0
			if p := w.r.Basins[b].Parent; p != None {
				w.bs[p].fullKids++
			}
			w.route(b, v)
			return
		}
		c := x.front.top()
		d := w.cost[c]
		if x.budget < d {
			return
		}
		x.front.pop(w.alt)
		x.budget -= d
		w.filled[c] = true
		w.by[c] = b
		w.res.Water[b].Filled = append(w.res.Water[b].Filled, c)
		x.level = max(x.level, w.alt[c])
		for _, y := range w.m.Cells[c].Neighbors {
			if !w.filled[y] && w.inRegion(b, y) {
				x.front.push(w.alt, y)
			}
		}
	}
}

// give hands water v, the overflow of basin from (None for direct
// inflow), to basin b, which is not full: b fills with it once its
// children are full; until then it goes to the child with the lowest water
// level, ties to the lower id.
func (w *water) give(b int, v float64, from int) {
	if w.allFull(b) {
		w.moves = append(w.moves, move{from, b, v})
		w.start(b)
		w.bs[b].budget += v
		w.fill(b)
		return
	}
	best := None
	for _, ch := range w.r.Basins[b].Children {
		if w.bs[ch].full {
			continue
		}
		if best == None || w.bs[ch].level < w.bs[best].level {
			best = ch
		}
	}
	w.give(best, v, from)
}

// route sends the overflow v of full basin b on: to its parent, or from a
// top-level basin through its spill to the sea or the basin downstream.
func (w *water) route(b int, v float64) {
	w.res.Water[b].Overflow += v
	if p := w.r.Basins[b].Parent; p != None {
		if w.bs[p].full {
			// Passing through a full basin (overflow from upstream).
			w.moves = append(w.moves, move{b, p, v})
			w.route(p, v)
			return
		}
		w.give(p, v, b)
		return
	}
	t := w.target(b)
	if t == None {
		w.toSea[b] += v
		return
	}
	if w.bs[t].full {
		// Pass through a full basin (a tie in spill level).
		w.moves = append(w.moves, move{b, t, v})
		w.route(t, v)
		return
	}
	w.give(t, v, b)
}

// target returns where top-level basin b's overflow goes: across the flat
// of its spill cell (outside b) to the lowest lower cell next to it, the
// sea first, and down that cell's descent. None is the sea.
func (w *water) target(b int) int {
	x := &w.bs[b]
	if x.targeted {
		return x.target
	}
	x.targeted = true
	bs := &w.r.Basins[b]
	h := w.r.RouteM
	a := bs.SpillM
	seen := map[int]bool{bs.SpillCell: true}
	queue := []int{bs.SpillCell}
	best := None
	for i := 0; i < len(queue); i++ {
		for _, y := range w.m.Cells[queue[i]].Neighbors {
			if seen[y] || w.inRegion(b, y) {
				continue
			}
			switch {
			case w.seed[y] || h[y] < a:
				if best == None || w.lower(y, best) {
					best = y
				}
			case h[y] == a:
				seen[y] = true
				queue = append(queue, y)
			}
		}
	}
	x.target = None
	if best != None && !w.seed[best] {
		x.target = w.res.Sink[best]
	}
	if x.target != None && w.within(b, x.target) {
		x.target = None // cannot happen: the spill leads out of b
	}
	w.res.Water[b].OverflowTo = x.target
	w.res.Water[b].OverflowVia = best
	return x.target
}

// collect forms the lakes, playas and totals from the final state.
func (w *water) collect() {
	m, r, res, n := w.m, w.r, w.res, len(w.m.Cells)
	nb := len(r.Basins)
	for b := range nb {
		if r.Basins[b].Parent == None {
			w.target(b) // so OverflowTo and OverflowVia say where overflow would go
		}
	}
	for b := range nb {
		if r.Basins[b].Parent == None {
			w.target(b) // so OverflowTo and OverflowVia say where overflow would go
		}
	}
	wet := func(b int) bool { return w.bs[b].full || len(res.Water[b].Filled) > 0 }
	root := make([]int, nb) // the lake root of a wet basin, else itself
	for b := nb - 1; b >= 0; b-- {
		root[b] = b
		if p := r.Basins[b].Parent; p != None && wet(b) && wet(p) {
			root[b] = root[p]
		}
	}
	lakeOf := make([]int, nb)
	for b := range nb {
		lakeOf[b] = None
		bw := &res.Water[b]
		x := &w.bs[b]
		bw.LevelM, bw.Remainder = x.level, x.budget
		switch {
		case x.full:
			bw.State = Full
		case len(bw.Filled) > 0:
			bw.State = Partial
		default:
			bw.State, bw.LevelM = Dry, r.Basins[b].BottomM
		}
		if wet(b) && root[b] == b {
			lakeOf[b] = len(res.Lakes)
			res.Lakes = append(res.Lakes, Lake{Basin: b, SurfaceM: bw.LevelM, Full: x.full, Remainder: x.budget, Outlet: None})
		}
	}
	for b := range nb {
		if wet(b) {
			res.Water[b].Lake = lakeOf[root[b]]
		}
	}
	for c := range n {
		res.Lake[c] = None
		if b := w.by[c]; b != None {
			k := lakeOf[root[b]]
			res.Lake[c] = k
			res.Lakes[k].Cells = append(res.Lakes[k].Cells, c)
		}
	}
	// Per-lake volumes, in cell order.
	for c := range n {
		s := res.Sink[c]
		if s == None {
			continue
		}
		if k := res.Lake[c]; k != None {
			l := &res.Lakes[k]
			l.Precipitation += fmath.Mul(w.cl.Precipitation[c], w.area[c])
			l.Evaporation += fmath.Mul(w.cl.PET[c], w.area[c])
			l.Seepage += fmath.Mul(w.p.SeepageMM, w.area[c])
			continue
		}
		if wet(s) {
			res.Lakes[lakeOf[root[s]]].Runoff += fmath.Mul(w.cl.Runoff[c], w.area[c])
		}
	}
	// Moves between lakes, in the order made.
	for _, mv := range w.moves {
		if mv.from == None {
			// A dry parent's own inflow, passed to a child.
			if wet(mv.to) {
				res.Lakes[lakeOf[root[mv.to]]].Runoff += mv.w
			}
			continue
		}
		src := lakeOf[root[mv.from]]
		dst := None
		if wet(mv.to) {
			dst = lakeOf[root[mv.to]]
		}
		if src == dst {
			continue
		}
		res.Lakes[src].Overflow += mv.w
		if dst != None {
			res.Lakes[dst].Received += mv.w
		}
	}
	for b := range nb {
		if v := w.toSea[b]; v != 0 {
			res.Lakes[lakeOf[root[b]]].Overflow += v
		}
	}
	for k := range res.Lakes {
		l := &res.Lakes[k]
		if len(l.Cells) >= w.p.InlandSeaMinCells {
			l.Kind = KindInlandSea
		}
		if l.Overflow > 0 {
			l.Outlet = r.Basins[l.Basin].SpillCorner
		}
		l.Salt = l.Overflow == 0 && l.Evaporation > 0 &&
			l.Evaporation >= fmath.Mul(w.p.SaltEvapShare, l.Evaporation+l.Seepage)
	}
	// Playas: leaf basins left dry.
	for b := range nb {
		if len(r.Basins[b].Children) != 0 || wet(b) {
			continue
		}
		c := r.Basins[b].Bottom
		best, bestH := None, 0.0
		for _, k := range m.Cells[c].Corners {
			h := CornerHeight(m, w.alt, k)
			if best == None || h < bestH || h == bestH && k < best {
				best, bestH = k, h
			}
		}
		res.Playas = append(res.Playas, Playa{Basin: b, Cell: c, Corner: best})
	}
	// Totals, in cell and basin order.
	for c := range n {
		if res.Sink[c] == None {
			continue
		}
		if res.Lake[c] != None {
			res.Inflow += fmath.Mul(w.cl.Precipitation[c], w.area[c])
			res.Evaporation += fmath.Mul(w.cl.PET[c], w.area[c])
			res.Seepage += fmath.Mul(w.p.SeepageMM, w.area[c])
		} else {
			res.Inflow += fmath.Mul(w.cl.Runoff[c], w.area[c])
		}
	}
	for b := range nb {
		res.ToSea += w.toSea[b]
		res.Unplaced += w.bs[b].budget
	}
	if res.Inflow > 0 {
		res.Residual = (res.Inflow - res.Evaporation - res.Seepage - res.ToSea - res.Unplaced) / res.Inflow
	}
}

// Residual returns lake l's relative balance residual: (in − out) / in,
// with in its runoff, precipitation and received water, and out its
// evaporation, seepage, overflow and remainder; 0 with nothing in.
func (l *Lake) Residual() float64 {
	in := l.Runoff + l.Precipitation + l.Received
	if in == 0 {
		return 0
	}
	return (in - l.Evaporation - l.Seepage - l.Overflow - l.Remainder) / in
}

// cellHeap is a min-heap of cells by altitude, then id.
type cellHeap struct{ c []int }

func (h *cellHeap) len() int { return len(h.c) }
func (h *cellHeap) top() int { return h.c[0] }

func cellLess(alt []float64, a, b int) bool {
	if alt[a] != alt[b] {
		return alt[a] < alt[b]
	}
	return a < b
}

func (h *cellHeap) push(alt []float64, c int) {
	h.c = append(h.c, c)
	i := len(h.c) - 1
	for i > 0 {
		p := (i - 1) / 2
		if !cellLess(alt, h.c[i], h.c[p]) {
			break
		}
		h.c[i], h.c[p] = h.c[p], h.c[i]
		i = p
	}
}

func (h *cellHeap) pop(alt []float64) {
	last := len(h.c) - 1
	h.c[0] = h.c[last]
	h.c = h.c[:last]
	i := 0
	for {
		l, r, s := 2*i+1, 2*i+2, i
		if l < last && cellLess(alt, h.c[l], h.c[s]) {
			s = l
		}
		if r < last && cellLess(alt, h.c[r], h.c[s]) {
			s = r
		}
		if s == i {
			return
		}
		h.c[i], h.c[s] = h.c[s], h.c[i]
		i = s
	}
}

// AppendBinary appends the water balance's canonical encoding to b; it is
// the input to the golden hashes. Integers are little-endian uint64 (None
// as its two's complement), floats their IEEE 754 bits as little-endian
// uint64, booleans one byte. In order:
//
//	SeepageMM, SaltEvapShare                       floats
//	InlandSeaMinCells                              integer
//	number of cells                                integer
//	per cell: Down, Sink, Lake                     integers
//	Stray                                          integer
//	number of basins                               integer
//	per basin: State                               integer
//	    LevelM                                     float
//	    number filled, then each                   integers
//	    Inflow, Overflow                           floats
//	    OverflowTo, OverflowVia                    integers
//	    Remainder                                  float
//	    Lake                                       integer
//	number of lakes                                integer
//	per lake: Basin                                integer
//	    number of cells, then each                 integers
//	    SurfaceM                                   float
//	    Kind                                       integer
//	    Full, Salt                                 booleans
//	    Runoff, Precipitation, Received,
//	    Evaporation, Seepage, Overflow, Remainder  floats
//	    Outlet                                     integer
//	number of playas                               integer
//	per playa: Basin, Cell, Corner                 integers
//	Inflow, Evaporation, Seepage, ToSea,
//	Unplaced, Residual                             floats
//
// The error is always nil; the signature is encoding.BinaryAppender's.
func (l *Lakes) AppendBinary(b []byte) ([]byte, error) {
	fl := func(vs ...float64) {
		for _, v := range vs {
			b = binary.LittleEndian.AppendUint64(b, math.Float64bits(v))
		}
	}
	in := func(vs ...int) {
		for _, v := range vs {
			b = binary.LittleEndian.AppendUint64(b, uint64(int64(v)))
		}
	}
	bo := func(v bool) {
		if v {
			b = append(b, 1)
		} else {
			b = append(b, 0)
		}
	}
	fl(l.Params.SeepageMM, l.Params.SaltEvapShare)
	in(l.Params.InlandSeaMinCells, len(l.Down))
	for c := range l.Down {
		in(l.Down[c], l.Sink[c], l.Lake[c])
	}
	in(l.Stray, len(l.Water))
	for _, x := range l.Water {
		in(int(x.State))
		fl(x.LevelM)
		in(len(x.Filled))
		in(x.Filled...)
		fl(x.Inflow, x.Overflow)
		in(x.OverflowTo, x.OverflowVia)
		fl(x.Remainder)
		in(x.Lake)
	}
	in(len(l.Lakes))
	for _, k := range l.Lakes {
		in(k.Basin, len(k.Cells))
		in(k.Cells...)
		fl(k.SurfaceM)
		in(int(k.Kind))
		bo(k.Full)
		bo(k.Salt)
		fl(k.Runoff, k.Precipitation, k.Received, k.Evaporation, k.Seepage, k.Overflow, k.Remainder)
		in(k.Outlet)
	}
	in(len(l.Playas))
	for _, p := range l.Playas {
		in(p.Basin, p.Cell, p.Corner)
	}
	fl(l.Inflow, l.Evaporation, l.Seepage, l.ToSea, l.Unplaced, l.Residual)
	return b, nil
}
