// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package river

import (
	"bytes"
	"math"
	"slices"
	"testing"

	"github.com/mdhender/mpg/internal/basin"
	"github.com/mdhender/mpg/internal/edges"
)

// defaultParams are the default class breaks (config river).
var defaultParams = Params{ThresholdKm2: 500, RiverKm2: 2000, MajorRiverKm2: 10_000}

// relEq reports whether a and b agree to a relative 1e-9.
func relEq(a, b float64) bool {
	return math.Abs(a-b) <= 1e-9*max(math.Abs(a), math.Abs(b), 1)
}

// uniform returns a flow of runoff mm per year on every cell and no lake
// overflow.
func uniform(in Input, runoff float64) Flow {
	return Flow{Runoff: slices.Repeat([]float64{runoff}, len(in.Land)), Overflow: make([]float64, len(in.Lakes))}
}

// accumulate builds the network, checks its invariants (checkNetwork), and
// checks that a second build is identical.
func accumulate(t *testing.T, in Input, tr *Tree, f Flow, p Params) *Network {
	t.Helper()
	n, err := Accumulate(in, tr, f, p)
	if err != nil {
		t.Fatal(err)
	}
	checkNetwork(t, in, tr, f, n)
	again, err := Accumulate(in, tr, f, p)
	if err != nil {
		t.Fatal(err)
	}
	b1, _ := n.AppendBinary(nil)
	b2, _ := again.AppendBinary(nil)
	if !bytes.Equal(b1, b2) {
		t.Error("two accumulations differ")
	}
	return n
}

// bruteDrainage recomputes every corner's drainage cell by cell: each land
// cell's area is added to every corner on its path down the tree, and on
// through the lakes its water reaches (an outlet continues from its outlet
// corner, a direct drain into its lake); each lake cell's area starts at
// its lake. It returns the corners' drainage and each lake's.
func bruteDrainage(in Input, tr *Tree) (corner, lake []float64) {
	m := in.Mesh
	corner = make([]float64, len(m.Corners))
	lake = make([]float64, len(in.Lakes))
	var intoLake func(l int, a float64)
	var down func(k int, a float64)
	down = func(k int, a float64) {
		for {
			corner[k] += a
			if tr.Down[k] == None {
				break
			}
			k = tr.Down[k]
		}
		if tr.Terminal[k] == Lake {
			intoLake(tr.Water[k], a)
		}
	}
	intoLake = func(l int, a float64) {
		lake[l] += a
		switch o := tr.Lakes[l]; o.Drain {
		case Outlet:
			down(o.Corner, a)
		case DirectLake:
			intoLake(o.Into, a)
		}
	}
	for c := range m.Cells {
		switch {
		case in.Land[c]:
			down(tr.Lowest(m, c), m.Area(c))
		case in.Lake[c] != None:
			intoLake(in.Lake[c], m.Area(c))
		}
	}
	return corner, lake
}

// checkNetwork checks a network's invariants:
//
//   - every corner's and lake's drainage is the cell-by-cell sum
//     (bruteDrainage), to a relative 1e-9, and drainage never falls
//     downstream;
//   - the drainage arriving at the sea, dry sinks, closed lakes and lakes
//     draining straight to the sea is the land and lake area (area is
//     conserved);
//   - an outlet's volume is at least its lake's overflow;
//   - a tree edge's upstream corner is Up and its class its drainage's;
//     every other edge has no class;
//   - a river edge is land–land (never a coast or shore) and no corner of
//     it touches the rim;
//   - the polylines are ordered by first edge id; each runs down the tree,
//     with its edges' classes, never falling; every river edge is on
//     exactly one;
//   - a polyline ends at a terminal, which is a mouth, or at a corner
//     inside another polyline, where it is not the main inflow: the
//     continuing one has the larger drainage (ties to the lower corner id)
//     or is a lake's outlet whose lake passes on at least as much;
//   - the mouths are exactly the polylines' terminal ends.
func checkNetwork(t *testing.T, in Input, tr *Tree, f Flow, n *Network) {
	t.Helper()
	m := in.Mesh
	bc, bl := bruteDrainage(in, tr)
	for k := range m.Corners {
		if !relEq(n.Drainage[k], bc[k]) {
			t.Fatalf("corner %d: drainage %v, cell by cell %v", k, n.Drainage[k], bc[k])
		}
		if d := tr.Down[k]; tr.Land[k] && d != None && n.Drainage[d] < n.Drainage[k] {
			t.Errorf("corner %d: drainage falls downstream (%v to %v)", k, n.Drainage[k], n.Drainage[d])
		}
	}
	var area, arrived float64
	for c := range m.Cells {
		if in.Land[c] || in.Lake[c] != None {
			area += m.Area(c)
		}
	}
	for l, o := range tr.Lakes {
		if !relEq(n.LakeDrainage[l], bl[l]) {
			t.Errorf("lake %d: drainage %v, cell by cell %v", l, n.LakeDrainage[l], bl[l])
		}
		switch o.Drain {
		case Closed, DirectSea:
			arrived += n.LakeDrainage[l]
		case Outlet:
			if n.Volume[o.Corner] < f.Overflow[l] {
				t.Errorf("lake %d: outlet volume %v below its overflow %v", l, n.Volume[o.Corner], f.Overflow[l])
			}
		}
	}
	for k, land := range tr.Land {
		if land && tr.Down[k] == None && (tr.Terminal[k] == Ocean || tr.Terminal[k] == Sink) {
			arrived += n.Drainage[k]
		}
	}
	if !relEq(arrived, area) {
		t.Errorf("drainage arriving %v, land and lake area %v", arrived, area)
	}
	for e := range m.Edges {
		k := n.Up[e]
		if k == None {
			if n.Class[e] != edges.RiverNone {
				t.Errorf("edge %d off the tree has class %v", e, n.Class[e])
			}
			continue
		}
		if tr.DownEdge[k] != e || n.Class[e] != n.Params.Class(n.Drainage[k]) || n.EdgeDrainage(e) != n.Drainage[k] {
			t.Errorf("edge %d: up %d, class %v for drainage %v", e, k, n.Class[e], n.Drainage[k])
		}
		if n.Class[e] == edges.RiverNone {
			continue
		}
		ed := &m.Edges[e]
		if ed.OnBoundary() || !in.Land[ed.Cells[0]] || !in.Land[ed.Cells[1]] {
			t.Errorf("river edge %d not land–land", e)
		}
		for _, x := range ed.Corners {
			if slices.ContainsFunc(m.Corners[x].Cells, func(c int) bool { return m.Cells[c].Rim }) {
				t.Errorf("river edge %d touches the rim at corner %d", e, x)
			}
		}
	}
	on := make([]int, len(m.Edges))
	within := make([]int, len(m.Corners)) // the polyline continuing through a corner
	for k := range within {
		within[k] = None
	}
	for i, p := range n.Paths {
		if i > 0 && p.Edges[0] <= n.Paths[i-1].Edges[0] {
			t.Errorf("path %d: first edge %d not after path %d's", i, p.Edges[0], i-1)
		}
		if len(p.Corners) != len(p.Edges)+1 || len(p.Classes) != len(p.Edges) {
			t.Fatalf("path %d: %d corners, %d edges, %d classes", i, len(p.Corners), len(p.Edges), len(p.Classes))
		}
		for j, e := range p.Edges {
			x := p.Corners[j]
			if tr.DownEdge[x] != e || tr.Down[x] != p.Corners[j+1] || p.Classes[j] != n.Class[e] || n.Class[e] == edges.RiverNone {
				t.Errorf("path %d step %d: corner %d edge %d class %v", i, j, x, e, p.Classes[j])
			}
			if j > 0 && p.Classes[j] < p.Classes[j-1] {
				t.Errorf("path %d: class falls at edge %d", i, e)
			}
			on[e]++
			within[x] = i
		}
	}
	for e, c := range n.Class {
		if c != edges.RiverNone && on[e] != 1 {
			t.Errorf("river edge %d on %d paths", e, on[e])
		}
	}
	lakeAt := make([]float64, len(m.Corners))
	outlet := make([]bool, len(m.Corners))
	for l, o := range tr.Lakes {
		if o.Drain == Outlet {
			lakeAt[o.Corner] += n.LakeDrainage[l]
			outlet[o.Corner] = true
		}
	}
	ends := make([]bool, len(m.Corners))
	for i, p := range n.Paths {
		last := p.Corners[len(p.Corners)-1]
		prev := p.Corners[len(p.Corners)-2]
		if tr.Down[last] == None {
			ends[last] = true
			continue
		}
		j := within[last]
		if j == None {
			t.Errorf("path %d ends at corner %d, neither a terminal nor on another path", i, last)
			continue
		}
		q := n.Paths[j]
		at := slices.Index(q.Corners, last)
		if at == 0 {
			if !outlet[last] || lakeAt[last] < n.Drainage[prev] {
				t.Errorf("path %d ends at corner %d, where path %d starts without a larger lake", i, last, j)
			}
			continue
		}
		main := q.Corners[at-1]
		if n.Drainage[main] < n.Drainage[prev] || n.Drainage[main] == n.Drainage[prev] && main > prev {
			t.Errorf("path %d (from corner %d) ends at confluence %d, but path %d (from corner %d) is smaller", i, prev, last, j, main)
		}
	}
	for k := range m.Corners {
		if n.Mouth[k] != ends[k] {
			t.Errorf("corner %d: mouth %v, a path ends at this terminal %v", k, n.Mouth[k], ends[k])
		}
		if n.Mouth[k] && tr.Terminal[k] == Interior {
			t.Errorf("mouth %d is not terminal", k)
		}
	}
}

// TestParams checks the class breaks and the discharge conversion by hand.
func TestParams(t *testing.T) {
	p := defaultParams
	for _, tc := range []struct {
		a    float64
		want edges.RiverClass
	}{
		{0, edges.RiverNone}, {499.99, edges.RiverNone}, {500, edges.Stream}, {1999.9, edges.Stream},
		{2000, edges.River}, {9999, edges.River}, {10_000, edges.MajorRiver}, {1e6, edges.MajorRiver},
	} {
		if got := p.Class(tc.a); got != tc.want {
			t.Errorf("Class(%v) = %v, want %v", tc.a, got, tc.want)
		}
	}
	if got := Discharge(31_557.6); got != 1 {
		t.Errorf("Discharge(31557.6 mm·km²/yr) = %v m³/s, want 1", got)
	}
	for _, bad := range []Params{{0, 2000, 10_000}, {500, 500, 10_000}, {500, 2000, 1000}, {500, 2000, math.Inf(1)}, {math.NaN(), 2000, 10_000}} {
		if bad.check() == nil {
			t.Errorf("%v accepted", bad)
		}
	}
}

// TestSinkDrainage: in a closed bowl around a playa, the dry sink's
// drainage is the area of every cell whose water ends there (the playa and
// its six neighbors at least), its volume that area times the runoff, and
// the edges into it carry their upstream corner's drainage, classed by it.
func TestSinkDrainage(t *testing.T) {
	g := newGrid(t, `
~   ~   ~   ~   ~   ~   ~   ~
o   o   o   o   o   o   o   o
300 300 300 300 300 300 300 300
300 300 250 250 300 300 300 300
300 300 250 10  250 300 300 300
300 300 250 250 300 300 300 300
300 300 300 300 300 300 300 300
o   o   o   o   o   o   o   o
~   ~   ~   ~   ~   ~   ~   ~`)
	m := g.in.Mesh
	playa := g.id(4, 3)
	sink := None
	for _, k := range m.Cells[playa].Corners {
		if sink == None || basin.CornerHeight(m, g.in.Altitude, k) < basin.CornerHeight(m, g.in.Altitude, sink) {
			sink = k
		}
	}
	g.in.Sinks = []int{sink}
	tr := g.build(t)
	hex := m.Area(playa) // every cell of the lattice is a regular hexagon
	if want := math.Sqrt(3) / 2 * hexDX * hexDX; !relEq(hex, want) {
		t.Fatalf("cell area %v, want %v", hex, want)
	}
	cells := 0
	for c, land := range g.in.Land {
		if land && tr.Dest(tr.Lowest(m, c)) == (Dest{Sink, sink}) {
			cells++
		}
	}
	if cells < 7 {
		t.Fatalf("%d cells drain to the sink, want the bowl's 7 at least", cells)
	}
	p := Params{ThresholdKm2: 2.5 * hex, RiverKm2: 3.5 * hex, MajorRiverKm2: 6.5 * hex}
	n := accumulate(t, g.in, tr, uniform(g.in, 100), p)
	if want := float64(cells) * hex; !relEq(n.Drainage[sink], want) {
		t.Errorf("sink drainage %v, want %d cells × %v = %v", n.Drainage[sink], cells, hex, want)
	}
	if want := 100 * n.Drainage[sink]; !relEq(n.Volume[sink], want) {
		t.Errorf("sink volume %v, want %v", n.Volume[sink], want)
	}
	if !n.Mouth[sink] {
		t.Error("the sink is not a mouth")
	}
	for k, d := range tr.Down {
		if d != sink {
			continue
		}
		e := tr.DownEdge[k]
		if n.EdgeDrainage(e) != n.Drainage[k] || !relEq(n.EdgeDischarge(e), n.Volume[k]*1000/SecondsPerYear) {
			t.Errorf("edge %d into the sink: drainage %v, discharge %v; corner %d %v, %v", e, n.EdgeDrainage(e), n.EdgeDischarge(e), k, n.Drainage[k], n.Volume[k])
		}
	}
	s := n.Stats(g.in, tr)
	if s.Ends[Sink] == 0 || s.Mouths == 0 || s.RiverEdges == 0 {
		t.Errorf("no river ends at the sink: %+v", s)
	}
}

// TestLakePassThrough: an overflowing one-cell lake passes its drainage
// (the drainage reaching its shore, plus its own area) on at its outlet,
// whose volume is the injected overflow plus what the tree brings to the
// outlet corner itself; the river leaving it starts at the outlet and
// reaches the sea. Closed, the same lake ends its drainage.
func TestLakePassThrough(t *testing.T) {
	const text = `
~   ~   ~   ~   ~   ~   ~   ~
o   o   o   o   o   o   o   o
300 300 300 300 300 300 300 300
300 300 250 200 300 300 300 300
300 300 250 a90 100 50  20  o
300 300 250 200 300 300 300 300
300 300 300 300 300 300 300 300
o   o   o   o   o   o   o   o
~   ~   ~   ~   ~   ~   ~   ~`
	g := newGrid(t, text)
	g.overflow(t, 0, g.id(4, 4))
	tr := g.build(t)
	m := g.in.Mesh
	out := tr.Lakes[0]
	if out.Drain != Outlet {
		t.Fatalf("lake 0 drains %v", out.Drain)
	}
	lakeCell := g.id(4, 3)
	hex := m.Area(lakeCell)
	f := uniform(g.in, 100)
	const overflow = 12_345.0
	f.Overflow[0] = overflow
	p := Params{ThresholdKm2: 0.5 * hex, RiverKm2: 4 * hex, MajorRiverKm2: 30 * hex}
	n := accumulate(t, g.in, tr, f, p)
	shore := 0
	for c, land := range g.in.Land {
		if land && tr.Dest(tr.Lowest(m, c)) == (Dest{Lake, 0}) {
			shore++
		}
	}
	if shore == 0 {
		t.Fatal("no cell drains into the lake")
	}
	if want := float64(shore+1) * hex; !relEq(n.LakeDrainage[0], want) {
		t.Errorf("lake drainage %v, want (%d shore cells + the lake) × %v = %v", n.LakeDrainage[0], shore, hex, want)
	}
	if want := 100 * float64(shore) * hex; !relEq(n.LakeInflow[0], want) {
		t.Errorf("lake inflow %v, want %v", n.LakeInflow[0], want)
	}
	k := out.Corner
	local, upA, upV := 0.0, 0.0, 0.0
	for c, land := range g.in.Land {
		if land && tr.Lowest(m, c) == k {
			local += m.Area(c)
		}
	}
	for j, d := range tr.Down {
		if d == k {
			upA += n.Drainage[j]
			upV += n.Volume[j]
		}
	}
	if want := n.LakeDrainage[0] + local + upA; !relEq(n.Drainage[k], want) {
		t.Errorf("outlet drainage %v, want %v", n.Drainage[k], want)
	}
	if want := overflow + 100*local + upV; !relEq(n.Volume[k], want) {
		t.Errorf("outlet volume %v, want overflow %v + %v local + %v upstream", n.Volume[k], overflow, 100*local, upV)
	}
	starts := slices.IndexFunc(n.Paths, func(p Path) bool { return p.Corners[0] == k })
	if starts < 0 {
		t.Fatal("no river starts at the outlet")
	}
	path := n.Paths[starts]
	if end := path.Corners[len(path.Corners)-1]; tr.Terminal[end] != Ocean || !n.Mouth[end] {
		t.Errorf("the outlet's river ends at corner %d (%v), not a mouth on the sea", end, tr.Terminal[end])
	}
	if s := n.Stats(g.in, tr); s.OutletSources != 1 {
		t.Errorf("%d rivers from outlets, want 1", s.OutletSources)
	}

	// Closed: the lake keeps its drainage.
	g = newGrid(t, text)
	tr = g.build(t)
	n = accumulate(t, g.in, tr, uniform(g.in, 100), p)
	shore = 0
	for c, land := range g.in.Land {
		if land && tr.Dest(tr.Lowest(m, c)) == (Dest{Lake, 0}) {
			shore++
		}
	}
	if want := float64(shore+1) * hex; !relEq(n.LakeDrainage[0], want) {
		t.Errorf("closed lake drainage %v, want %v", n.LakeDrainage[0], want)
	}
	for _, p := range n.Paths {
		if touchesLake(g.in, p.Corners[0], 0) {
			t.Errorf("a river starts at shore corner %d of a closed lake", p.Corners[0])
		}
	}
}

// TestShoreRivers: with every tree edge that carries water a river, none
// runs along the coast or beside water (checkNetwork), and every river ends
// at a mouth on the sea.
func TestShoreRivers(t *testing.T) {
	g := newGrid(t, `
~   ~   ~   ~   ~   ~   ~   ~   ~   ~
o   o   o   o   o   o   o   o   o   o
o   o   o   o   o   o   o   o   o   o
o   o   o   400 o   o   o   o   o   o
o   o   o   400 o   o   o   o   o   o
190 180 170 160 150 140 130 120 110 100
290 280 270 260 250 240 230 220 210 200
390 380 370 360 350 340 330 320 310 300
o   o   o   o   o   o   o   o   o   o
~   ~   ~   ~   ~   ~   ~   ~   ~   ~`)
	tr := g.build(t)
	n := accumulate(t, g.in, tr, uniform(g.in, 300), Params{ThresholdKm2: 1e-9, RiverKm2: 1, MajorRiverKm2: 2})
	s := n.Stats(g.in, tr)
	if s.RiverEdges == 0 {
		t.Fatalf("stats %+v: no rivers", s)
	}
	for e, k := range n.Up {
		if k != None && n.Class[e] == edges.RiverNone && n.Drainage[k] != 0 {
			t.Errorf("edge %d carries %v km² but is no river", e, n.Drainage[k])
		}
	}
	if s.Ends[Lake]+s.Ends[Sink] != 0 || s.Ends[Ocean] == 0 {
		t.Errorf("ends %v: want every river ending at the sea or a confluence", s.Ends)
	}
}

// TestSeamRiver: the valley's river crosses the east–west seam as one
// polyline and reaches the sea.
func TestSeamRiver(t *testing.T) {
	g := newGrid(t, `
~   ~   ~   ~   ~   ~   ~   ~   ~   ~
900 900 900 900 900 900 900 900 900 900
150 160 170 180 190 900 900 110 130 140
50  60  70  80  90  900 900 o   30  40
150 160 170 180 190 900 900 110 130 140
900 900 900 900 900 900 900 900 900 900
~   ~   ~   ~   ~   ~   ~   ~   ~   ~`)
	tr := g.build(t)
	m := g.in.Mesh
	hex := m.Area(g.id(3, 1))
	n := accumulate(t, g.in, tr, uniform(g.in, 300), Params{ThresholdKm2: 2.5 * hex, RiverKm2: 6 * hex, MajorRiverKm2: 12 * hex})
	half := m.Cylinder().W() / 2
	found := false
	for _, p := range n.Paths {
		cross := false
		for j := range p.Edges {
			if math.Abs(m.Corners[p.Corners[j]].Point.X-m.Corners[p.Corners[j+1]].Point.X) > half {
				cross = true
			}
		}
		if !cross {
			continue
		}
		found = true
		end := p.Corners[len(p.Corners)-1]
		if !n.Mouth[end] || tr.Terminal[end] != Ocean {
			t.Errorf("the seam river ends at corner %d (%v)", end, tr.Terminal[end])
		}
		if len(p.Edges) < 3 {
			t.Errorf("the seam river has %d edges", len(p.Edges))
		}
	}
	if !found {
		t.Fatal("no river crosses the seam")
	}
	if s := n.Stats(g.in, tr); s.Seam == 0 {
		t.Errorf("stats count %d seam crossings", s.Seam)
	}
}

// TestConfluence splits a hand-built tree by main stem: two equal
// tributaries meet at corner 2 (the lower id continues), a smaller one
// joins at corner 3, and corner 4 ends them at a terminal; then corner 2 is
// a lake's outlet whose lake passes on as much, so the lake continues and
// both tributaries end there.
func TestConfluence(t *testing.T) {
	// Corners 0 and 1 → 2 → 3 → 4 (terminal); 5 → 3. Edge e joins
	// corner e to its downstream corner (edge 5 is 5 → 3).
	tr := &Tree{
		Land:     slices.Repeat([]bool{true}, 6),
		Terminal: []Terminal{Interior, Interior, Interior, Interior, Ocean, Interior},
		Down:     []int{2, 2, 3, 4, None, 3},
		DownEdge: []int{0, 1, 2, 3, None, 5},
	}
	split := func(lake float64) *Network {
		n := &Network{
			Params:   defaultParams,
			Drainage: []float64{600, 600, 1300 + lake, 2100 + lake, 2100 + lake, 500},
			Class:    make([]edges.RiverClass, 6),
			Mouth:    make([]bool, 6),
		}
		for k, e := range tr.DownEdge {
			if e != None {
				n.Class[e] = n.Params.Class(n.Drainage[k])
			}
		}
		outlets := make([][]int, 6)
		if lake > 0 {
			n.LakeDrainage = []float64{lake}
			outlets[2] = []int{0}
		}
		if err := n.paths(tr, outlets); err != nil {
			t.Fatal(err)
		}
		return n
	}
	n := split(0)
	want := []Path{
		{Corners: []int{0, 2, 3, 4}, Edges: []int{0, 2, 3}, Classes: []edges.RiverClass{edges.Stream, edges.Stream, edges.River}},
		{Corners: []int{1, 2}, Edges: []int{1}, Classes: []edges.RiverClass{edges.Stream}},
		{Corners: []int{5, 3}, Edges: []int{5}, Classes: []edges.RiverClass{edges.Stream}},
	}
	if !equalPaths(n.Paths, want) {
		t.Errorf("paths %+v, want %+v", n.Paths, want)
	}
	if !slices.Equal(n.Mouth, []bool{false, false, false, false, true, false}) {
		t.Errorf("mouths %v", n.Mouth)
	}
	n = split(600)
	want = []Path{
		{Corners: []int{0, 2}, Edges: []int{0}, Classes: []edges.RiverClass{edges.Stream}},
		{Corners: []int{1, 2}, Edges: []int{1}, Classes: []edges.RiverClass{edges.Stream}},
		{Corners: []int{2, 3, 4}, Edges: []int{2, 3}, Classes: []edges.RiverClass{edges.Stream, edges.River}},
		{Corners: []int{5, 3}, Edges: []int{5}, Classes: []edges.RiverClass{edges.Stream}},
	}
	if !equalPaths(n.Paths, want) {
		t.Errorf("with the lake: paths %+v, want %+v", n.Paths, want)
	}
}

func equalPaths(a, b []Path) bool {
	return slices.EqualFunc(a, b, func(p, q Path) bool {
		return slices.Equal(p.Corners, q.Corners) && slices.Equal(p.Edges, q.Edges) && slices.Equal(p.Classes, q.Classes)
	})
}

// TestAccumulateErrors checks that malformed inputs are refused.
func TestAccumulateErrors(t *testing.T) {
	g := newGrid(t, `
~   ~   ~
o   100 o
~   ~   ~`)
	tr := g.build(t)
	f := uniform(g.in, 100)
	for name, mod := range map[string]func(f *Flow, p *Params){
		"short runoff":    func(f *Flow, p *Params) { f.Runoff = f.Runoff[:1] },
		"negative runoff": func(f *Flow, p *Params) { f.Runoff = slices.Repeat([]float64{-1}, len(f.Runoff)) },
		"NaN runoff":      func(f *Flow, p *Params) { f.Runoff = slices.Repeat([]float64{math.NaN()}, len(f.Runoff)) },
		"overflows":       func(f *Flow, p *Params) { f.Overflow = []float64{1} },
		"breaks":          func(f *Flow, p *Params) { p.RiverKm2 = p.ThresholdKm2 },
	} {
		f2, p := f, defaultParams
		mod(&f2, &p)
		if _, err := Accumulate(g.in, tr, f2, p); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}
