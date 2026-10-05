// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package river

import (
	"bytes"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/mdhender/mpg/internal/basin"
	"github.com/mdhender/mpg/internal/mesh"
	"github.com/mdhender/mpg/internal/topo"
)

// hexDX is the site spacing of the hand-built fixtures in km.
const hexDX = 10.0

// grid is a hand-built fixture (after package basin's): a hexagonal lattice
// of rows × cols cells (odd rows shifted half a cell east), whose top and
// bottom rows are rim cells, read from a text map. Cell (r, c) has id
// r·cols + c; columns wrap east–west. Tokens:
//
//	~       a rim cell (sea, altitude 0)
//	o       an ocean cell (sea, altitude −100)
//	aN, bN  a cell of lake 0 or 1 at altitude N
//	N       a land cell at altitude N
type grid struct {
	rows, cols int
	in         Input
}

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
	nl := 0
	for r, l := range lines {
		if len(l) != cols {
			t.Fatalf("row %d has %d cells, want %d", r, len(l), cols)
		}
		for c, v := range l {
			x := (float64(c) + 0.25 + 0.5*float64(r%2)) * hexDX
			sites = append(sites, topo.Point{X: x, Y: (float64(r) + 0.5) * dy})
			alt, land, lake := 0.0, false, None
			switch {
			case v == "~":
			case v == "o":
				alt = -100
			case v[0] == 'a' || v[0] == 'b':
				lake = int(v[0] - 'a')
				nl = max(nl, lake+1)
				alt = parse(t, v[1:])
			default:
				alt, land = parse(t, v), true
			}
			g.in.Altitude = append(g.in.Altitude, alt)
			g.in.Land = append(g.in.Land, land)
			g.in.Lake = append(g.in.Lake, lake)
		}
	}
	if g.in.Mesh, err = mesh.Build(cyl, sites); err != nil {
		t.Fatal(err)
	}
	for i, c := range g.in.Mesh.Cells {
		if c.Rim != (lines[i/cols][i%cols] == "~") {
			t.Fatalf("cell %d: rim %v", i, c.Rim)
		}
	}
	g.in.Lakes = make([]LakeIn, nl)
	for l := range g.in.Lakes {
		g.in.Lakes[l] = LakeIn{SurfaceM: float64(l), Spill: None, Pass: None}
	}
	return g
}

func parse(t *testing.T, s string) float64 {
	t.Helper()
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// id returns cell (r, c)'s id.
func (g *grid) id(r, c int) int { return r*g.cols + c }

// overflow makes lake l overflow over pass cell p, with S27's spill corner:
// of the edges between p and the lake, the lowest end by corner height,
// ties to the lower id.
func (g *grid) overflow(t *testing.T, l, p int) int {
	t.Helper()
	m := g.in.Mesh
	best := None
	for _, e := range m.Cells[p].Edges {
		if o := m.Edges[e].Other(p); o == mesh.Boundary || g.in.Lake[o] != l {
			continue
		}
		for _, k := range m.Edges[e].Corners {
			h, hb := 0.0, 0.0
			h = basin.CornerHeight(m, g.in.Altitude, k)
			if best != None {
				hb = basin.CornerHeight(m, g.in.Altitude, best)
			}
			if best == None || h < hb || h == hb && k < best {
				best = k
			}
		}
	}
	if best == None {
		t.Fatalf("pass %d does not touch lake %d", p, l)
	}
	g.in.Lakes[l].Spill, g.in.Lakes[l].Pass = best, p
	return best
}

// build builds the tree, checks the invariants every tree must meet, and
// checks that a second build is identical.
func (g *grid) build(t *testing.T) *Tree {
	t.Helper()
	tr, err := Build(g.in)
	if err != nil {
		t.Fatal(err)
	}
	checkTree(t, g.in, tr)
	again, err := Build(g.in)
	if err != nil {
		t.Fatal(err)
	}
	b1, _ := tr.AppendBinary(nil)
	b2, _ := again.AppendBinary(nil)
	if !bytes.Equal(b1, b2) {
		t.Error("two builds differ")
	}
	return tr
}

// touchesLake reports whether corner k touches a cell of lake l.
func touchesLake(in Input, k, l int) bool {
	return slices.ContainsFunc(in.Mesh.Corners[k].Cells, func(c int) bool { return in.Lake[c] == l })
}

// isOutlet reports whether k is some lake's outlet corner.
func isOutlet(tr *Tree, k int) bool {
	return slices.ContainsFunc(tr.Lakes, func(l LakeOut) bool { return l.Corner == k })
}

// checkTree checks the tree's invariants:
//
//   - the graph is the corners touching land; rim corners are never in it;
//   - every land corner's path down the tree ends, within as many steps as
//     there are corners (acyclic), at a terminal corner;
//   - every tree edge is a land–land edge joining the corner to its
//     downstream corner, never on the boundary nor beside the rim;
//   - levels never rise downstream, and every level is at least the
//     corner's height;
//   - Order holds every land corner once, each after its downstream corner;
//   - a terminal corner has no downstream corner unless it is a lake
//     outlet that is not itself a root;
//   - an outlet's path never touches its own lake again, and the lakes'
//     drains form no loop.
func checkTree(t *testing.T, in Input, tr *Tree) {
	t.Helper()
	m := in.Mesh
	rank := make([]int, len(m.Corners))
	for k := range rank {
		rank[k] = None
	}
	for i, k := range tr.Order {
		if rank[k] != None {
			t.Fatalf("corner %d twice in Order", k)
		}
		rank[k] = i
	}
	for k, cr := range m.Corners {
		land := slices.ContainsFunc(cr.Cells, func(c int) bool { return in.Land[c] })
		if land != tr.Land[k] {
			t.Fatalf("corner %d: land %v, want %v", k, tr.Land[k], land)
		}
		if !land {
			continue
		}
		if slices.ContainsFunc(cr.Cells, func(c int) bool { return m.Cells[c].Rim }) && tr.Terminal[k] != Ocean {
			t.Errorf("corner %d touches the rim but is %v", k, tr.Terminal[k])
		}
		if rank[k] == None {
			t.Fatalf("land corner %d not in Order", k)
		}
		if tr.Level[k] < tr.Height[k] {
			t.Errorf("corner %d: level %v below height %v", k, tr.Level[k], tr.Height[k])
		}
		x, n := k, 0
		for ; tr.Down[x] != None; n++ {
			if n > len(m.Corners) {
				t.Fatalf("corner %d: cycle", k)
			}
			x = tr.Down[x]
		}
		if tr.Terminal[x] == Interior {
			t.Errorf("corner %d ends at interior corner %d", k, x)
		}
		d := tr.Down[k]
		if d == None {
			continue
		}
		if tr.Terminal[k] != Interior && !isOutlet(tr, k) {
			t.Errorf("terminal corner %d (%v) has downstream corner %d", k, tr.Terminal[k], d)
		}
		e := &m.Edges[tr.DownEdge[k]]
		if e.OnBoundary() || !in.Land[e.Cells[0]] || !in.Land[e.Cells[1]] {
			t.Errorf("corner %d: tree edge %d is not land–land", k, tr.DownEdge[k])
		}
		if c := e.Corners; c != [2]int{k, d} && c != [2]int{d, k} {
			t.Errorf("corner %d: edge %d joins %v, not %d and %d", k, tr.DownEdge[k], c, k, d)
		}
		if tr.Level[d] > tr.Level[k] {
			t.Errorf("corner %d: level rises downstream (%v to %v)", k, tr.Level[k], tr.Level[d])
		}
		if rank[d] > rank[k] {
			t.Errorf("corner %d resolved before its downstream corner %d", k, d)
		}
	}
	for l, out := range tr.Lakes {
		if (out.Drain == Closed) != (in.Lakes[l].Spill == None) {
			t.Errorf("lake %d: drain %v, spill %d", l, out.Drain, in.Lakes[l].Spill)
		}
		// Following the lakes' drains from l ends at the sea, a closed
		// lake or a dry sink, without returning to a lake already seen.
		seen := map[int]bool{}
		for d := (Dest{Lake, l}); d.Kind == Lake; {
			if seen[d.ID] {
				t.Errorf("lake %d: drains in a loop through lake %d", l, d.ID)
				break
			}
			seen[d.ID] = true
			switch o := tr.Lakes[d.ID]; o.Drain {
			case Closed:
				d = Dest{Ocean, None}
			case DirectSea:
				d = Dest{Ocean, None}
			case DirectLake:
				if o.Into == d.ID || o.Into == None {
					t.Errorf("lake %d drains into lake %d", d.ID, o.Into)
				}
				d = Dest{Lake, o.Into}
			case Outlet:
				d = tr.Dest(o.Corner)
			}
		}
		if out.Drain != Outlet {
			continue
		}
		if out.Corner == None || !touchesLake(in, out.Corner, l) {
			t.Errorf("lake %d: outlet %d not on its shore", l, out.Corner)
			continue
		}
		for x := tr.Down[out.Corner]; x != None; x = tr.Down[x] {
			if touchesLake(in, x, l) {
				t.Errorf("lake %d: outlet path re-enters the lake at corner %d", l, x)
			}
		}
	}
}

// TestShore: the coast falls steadily eastward, so a drainage graph that
// allowed shore edges would run its water along the coast. Here no tree
// edge has water beside it, every shore corner ends water, and a peninsula
// one cell wide has no tree edge along it.
func TestShore(t *testing.T) {
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
	m := g.in.Mesh
	edges := 0
	for k, d := range tr.Down {
		if d == None {
			continue
		}
		edges++
		for _, c := range m.Edges[tr.DownEdge[k]].Cells {
			if !g.in.Land[c] {
				t.Errorf("tree edge %d beside water cell %d", tr.DownEdge[k], c)
			}
		}
	}
	if edges == 0 {
		t.Fatal("no tree edges")
	}
	for k, land := range tr.Land {
		if land && tr.Terminal[k] != Interior && tr.Down[k] != None {
			t.Errorf("shore corner %d has downstream corner %d", k, tr.Down[k])
		}
	}
	tip, neck := g.id(3, 3), g.id(4, 3)
	for _, k := range m.Cells[tip].Corners {
		if tr.Terminal[k] != Ocean || tr.Down[k] != None {
			t.Errorf("peninsula corner %d: %v, down %d", k, tr.Terminal[k], tr.Down[k])
		}
	}
	for _, e := range m.Cells[tip].Edges {
		if m.Edges[e].Other(tip) == neck && slices.Contains(tr.DownEdge, e) {
			t.Errorf("peninsula edge %d is a tree edge", e)
		}
	}
}

// TestSeam: a valley falls westward from column 4 across the east–west
// seam to its only sea, in column 7; a ridge in columns 5 and 6 stops a
// shortcut east. Its water crosses the seam on a tree edge and reaches
// that sea.
func TestSeam(t *testing.T) {
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
	half := m.Cylinder().W() / 2
	crossing := 0
	for k, d := range tr.Down {
		if d != None && math.Abs(m.Corners[k].Point.X-m.Corners[d].Point.X) > half {
			crossing++
		}
	}
	if crossing == 0 {
		t.Error("no tree edge crosses the seam")
	}
	sea := g.id(3, 7)
	for _, c := range []int{g.id(3, 1), g.id(3, 2), g.id(2, 2), g.id(4, 3)} {
		e := tr.End(tr.Lowest(m, c))
		if tr.Terminal[e] != Ocean || !slices.Contains(m.Corners[e].Cells, sea) {
			t.Errorf("cell %d: water ends at corner %d (%v), not at the sea cell %d", c, e, tr.Terminal[e], sea)
		}
	}
}

// TestOutletAtSpill (rule b): a one-cell lake overflows over its pass cell;
// the spill corner has a land–land edge leading off the lake, so it is the
// outlet; its path runs to the sea without touching the lake, the other
// shore corners end water there, and the slopes above the lake drain into
// it and finally to the sea.
func TestOutletAtSpill(t *testing.T) {
	g := newGrid(t, `
~   ~   ~   ~   ~   ~   ~   ~
o   o   o   o   o   o   o   o
300 300 300 300 300 300 300 300
300 300 250 200 300 300 300 300
300 300 250 a90 100 50  20  o
300 300 250 200 300 300 300 300
300 300 300 300 300 300 300 300
o   o   o   o   o   o   o   o
~   ~   ~   ~   ~   ~   ~   ~`)
	spill := g.overflow(t, 0, g.id(4, 4))
	tr := g.build(t)
	out := tr.Lakes[0]
	if out.Drain != Outlet || out.Corner != spill || !out.AtSpill || out.Fallback {
		t.Fatalf("lake 0: %+v, want an outlet at the spill corner %d", out, spill)
	}
	if d := tr.Dest(out.Corner); d != (Dest{Ocean, None}) {
		t.Errorf("outlet path ends at %+v, want the sea", d)
	}
	for _, k := range g.in.Mesh.Cells[g.id(4, 3)].Corners {
		if k != spill && tr.Down[k] != None {
			t.Errorf("shore corner %d has downstream corner %d", k, tr.Down[k])
		}
	}
	k := tr.Lowest(g.in.Mesh, g.id(3, 2))
	if d, f := tr.Dest(k), tr.Final(k); d != (Dest{Lake, 0}) || f != (Dest{Ocean, None}) {
		t.Errorf("slope above the lake: dest %+v, final %+v; want lake 0, then the sea", d, f)
	}
}

// TestOutletOnPass (rule c): a two-cell lake's spill corner touches both
// lake cells and the pass cell, so it has no land–land edge; the outlet is
// another shore corner of the pass cell.
func TestOutletOnPass(t *testing.T) {
	g := newGrid(t, `
~   ~   ~   ~   ~   ~   ~   ~
o   o   o   o   o   o   o   o
300 300 300 300 300 300 300 300
300 300 250 a80 300 300 300 300
300 300 250 a90 100 50  20  o
300 300 250 200 300 300 300 300
300 300 300 300 300 300 300 300
o   o   o   o   o   o   o   o
~   ~   ~   ~   ~   ~   ~   ~`)
	pass := g.id(4, 4)
	spill := g.overflow(t, 0, pass)
	m := g.in.Mesh
	n := 0
	for _, c := range m.Corners[spill].Cells {
		if g.in.Lake[c] == 0 {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("spill corner %d touches %d lake cells, want 2", spill, n)
	}
	tr := g.build(t)
	out := tr.Lakes[0]
	if out.Drain != Outlet || out.Corner == spill || out.AtSpill || out.Fallback ||
		!slices.Contains(m.Cells[pass].Corners, out.Corner) || !touchesLake(g.in, out.Corner, 0) {
		t.Fatalf("lake 0: %+v, want an outlet on pass cell %d other than spill corner %d", out, pass, spill)
	}
	if tr.Down[spill] != None {
		t.Errorf("spill corner %d has downstream corner %d", spill, tr.Down[spill])
	}
	if d := tr.Dest(out.Corner); d != (Dest{Ocean, None}) {
		t.Errorf("outlet path ends at %+v, want the sea", d)
	}
}

// TestOutletFallback (rule d): a lake whose pass cell does not touch it and
// whose spill corner has no land–land edge has no possible outlet, so the
// flood stalls with its shore held and opens the lowest held corner.
func TestOutletFallback(t *testing.T) {
	g := newGrid(t, `
~   ~   ~   ~   ~   ~   ~   ~
o   o   o   o   o   o   o   o
300 300 300 300 300 300 300 300
300 300 250 a80 300 300 300 300
300 300 250 a90 100 50  20  o
300 300 250 200 300 300 300 300
300 300 300 300 300 300 300 300
o   o   o   o   o   o   o   o
~   ~   ~   ~   ~   ~   ~   ~`)
	spill := g.overflow(t, 0, g.id(4, 4))
	g.in.Lakes[0].Pass = g.id(2, 0)
	tr := g.build(t)
	out := tr.Lakes[0]
	if tr.Fallbacks != 1 || !out.Fallback || out.Drain != Outlet || out.Corner == None {
		t.Fatalf("lake 0: %+v, %d fallbacks; want one fallback outlet", out, tr.Fallbacks)
	}
	if out.Corner == spill {
		t.Errorf("fallback chose the spill corner %d, which has no land–land edge", spill)
	}
}

// TestDrySink: a playa cell at the bottom of a closed basin; every corner of
// the playa cell ends at its dry sink, which ends water.
func TestDrySink(t *testing.T) {
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
		if h := basin.CornerHeight(m, g.in.Altitude, k); sink == None || h < basin.CornerHeight(m, g.in.Altitude, sink) {
			sink = k
		}
	}
	g.in.Sinks = []int{sink}
	tr := g.build(t)
	if tr.Terminal[sink] != Sink || tr.Down[sink] != None {
		t.Fatalf("sink %d: %v, down %d", sink, tr.Terminal[sink], tr.Down[sink])
	}
	for _, k := range m.Cells[playa].Corners {
		if d := tr.Dest(k); d != (Dest{Sink, sink}) {
			t.Errorf("playa corner %d ends at %+v, want the sink %d", k, d, sink)
		}
	}
}

// TestFlat: a plateau of equal cells drains through a low column. Inside
// it, where no height decides, each corner's path to the plateau's edge is
// as short as a breadth-first search from the edge says (first in, first
// out among equal levels), so the flow does not comb along corner ids.
func TestFlat(t *testing.T) {
	g := newGrid(t, `
~   ~   ~   ~   ~   ~   ~   ~   ~   ~
o   o   o   o   o   o   o   o   o   o
500 500 500 500 500 500 500 500 500 500
500 100 100 100 100 100 100 100 50  o
500 100 100 100 100 100 100 100 50  o
500 100 100 100 100 100 100 100 50  o
500 100 100 100 100 100 100 100 50  o
500 100 100 100 100 100 100 100 50  o
500 500 500 500 500 500 500 500 500 500
o   o   o   o   o   o   o   o   o   o
~   ~   ~   ~   ~   ~   ~   ~   ~   ~`)
	tr := g.build(t)
	const flat = 100.0
	var region []int
	for k, land := range tr.Land {
		if land && tr.Terminal[k] == Interior && tr.Level[k] == flat && tr.Height[k] == flat {
			region = append(region, k)
		}
	}
	if len(region) < 20 {
		t.Fatalf("flat has %d corners", len(region))
	}
	// Breadth-first search from the flat's exits: corners at the flat's
	// level whose downstream corner is lower.
	dist := map[int]int{}
	var queue []int
	for _, k := range tr.Order {
		if tr.Level[k] == flat && tr.Down[k] != None && tr.Level[tr.Down[k]] < flat {
			dist[k] = 0
			queue = append(queue, k)
		}
	}
	m := g.in.Mesh
	for len(queue) > 0 {
		k := queue[0]
		queue = queue[1:]
		for _, e := range m.Corners[k].Edges {
			ed := &m.Edges[e]
			if ed.OnBoundary() || !g.in.Land[ed.Cells[0]] || !g.in.Land[ed.Cells[1]] {
				continue
			}
			j := ed.Corners[0] + ed.Corners[1] - k
			if _, ok := dist[j]; !ok && tr.Terminal[j] == Interior && tr.Level[j] == flat {
				dist[j] = dist[k] + 1
				queue = append(queue, j)
			}
		}
	}
	for _, k := range region {
		hops := 0
		for x := k; tr.Level[tr.Down[x]] == flat; x = tr.Down[x] {
			hops++
		}
		if d, ok := dist[k]; !ok || hops != d {
			t.Errorf("flat corner %d: %d hops to the edge, shortest %d", k, hops, d)
		}
	}
}

// TestInputErrors checks that malformed inputs are refused.
func TestInputErrors(t *testing.T) {
	g := newGrid(t, `
~   ~   ~
100 100 100
~   ~   ~`)
	for name, mod := range map[string]func(in *Input){
		"no mesh":    func(in *Input) { in.Mesh = nil },
		"short land": func(in *Input) { in.Land = in.Land[:1] },
		"rim land":   func(in *Input) { in.Land = slices.Repeat([]bool{true}, len(in.Land)) },
		"bad lake":   func(in *Input) { in.Lake = slices.Repeat([]int{5}, len(in.Lake)) },
		"bad sink":   func(in *Input) { in.Sinks = []int{-2} },
		"half catch": func(in *Input) { in.Catchment = make([]Dest, len(in.Land)) },
	} {
		in := g.in
		in.Land, in.Lake = slices.Clone(in.Land), slices.Clone(in.Lake)
		mod(&in)
		if _, err := Build(in); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}
