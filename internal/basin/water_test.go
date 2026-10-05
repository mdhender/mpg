// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package basin

import (
	"bytes"
	"math"
	"slices"
	"strings"
	"testing"
)

// chainMap is the chained-basins fixture: two top-level basins side by
// side in a 900 m upland. D (cells at 200, 250 and 250 m, row 3, columns
// 2–4) spills at its 600 m pass (row 3, column 1) onto the 500 m strip of
// column 0, which runs to the rim (the sea). U (one cell at 300 m, row 3,
// column 7) spills at its 700 m pass (row 3, column 6) onto a 650 m slope
// (row 3, column 5) that falls into D. So U's overflow runs into D, and
// D's to the sea.
const chainMap = `
~   ~   ~   ~   ~   ~   ~   ~   ~   ~
500 900 900 900 900 900 900 900 900 900
500 900 900 900 900 900 900 900 900 900
500 600 200 250 250 650 700 300 900 900
500 900 900 900 900 900 900 900 900 900
500 900 900 900 900 900 900 900 900 900
~   ~   ~   ~   ~   ~   ~   ~   ~   ~
`

// sinkMap is the dry-basin and one-cell-lake fixture: low land at 100 m
// that drains to the rim, with a ring of 300 m hills around a floor below
// sea level (the rim's 0 m): a cell at −50 m (row 3, column 3) and one at
// −20 m (row 3, column 4). The ring's one gap is a pass at 250 m (row 3,
// column 5).
const sinkMap = `
~   ~   ~   ~   ~   ~   ~   ~   ~   ~
100 100 100 100 100 100 100 100 100 100
100 100 300 300 300 300 100 100 100 100
100 100 300 -50 -20 250 100 100 100 100
100 100 300 300 300 300 100 100 100 100
100 100 100 100 100 100 100 100 100 100
~   ~   ~   ~   ~   ~   ~   ~   ~   ~
`

// cellAreas returns the fixture's cell areas.
func (g *grid) cellAreas() []float64 {
	a := make([]float64, len(g.m.Cells))
	for i := range a {
		a[i] = g.m.Area(i)
	}
	return a
}

// humid returns a climate in which every cell's actual evaporation equals
// its PET (500 mm), so a lake cell costs exactly its seepage, with runoff
// runoff(c) on each cell.
func humid(n int, runoff func(c int) float64) Climate {
	cl := Climate{Precipitation: make([]float64, n), PET: make([]float64, n), Runoff: make([]float64, n)}
	for c := range n {
		cl.Runoff[c] = runoff(c)
		cl.PET[c] = 500
		cl.Precipitation[c] = 500 + cl.Runoff[c]
	}
	return cl
}

// balance runs Find and Balance on the fixture and checks conservation.
func (g *grid) balance(t *testing.T, cl Climate, p Params) (*Result, *Lakes) {
	t.Helper()
	r := g.find(t, 50)
	l, err := Balance(g.m, g.alt, g.seed, g.cellAreas(), r, cl, p)
	if err != nil {
		t.Fatal(err)
	}
	checkConservation(t, l)
	return r, l
}

// checkConservation checks the whole map's and every lake's balance.
func checkConservation(t *testing.T, l *Lakes) {
	t.Helper()
	if math.Abs(l.Residual) > ConservationTolerance {
		t.Errorf("residual %v: inflow %v, evaporation %v, seepage %v, to sea %v, unplaced %v",
			l.Residual, l.Inflow, l.Evaporation, l.Seepage, l.ToSea, l.Unplaced)
	}
	for k := range l.Lakes {
		if r := l.Lakes[k].Residual(); math.Abs(r) > ConservationTolerance {
			t.Errorf("lake %d: residual %v: %+v", k, r, l.Lakes[k])
		}
	}
}

// catchment returns the cells whose runoff reaches basin b.
func catchment(l *Lakes, b int) []int {
	var cs []int
	for c, s := range l.Sink {
		if s == b {
			cs = append(cs, c)
		}
	}
	return cs
}

// near reports whether a and b agree to 1e-9 relative.
func near(a, b float64) bool { return math.Abs(a-b) <= 1e-9*max(math.Abs(a), math.Abs(b), 1) }

// TestNestedOverflow is the overflow fixture on the nested basins (A and B
// in P, which spills to the sea; see nestedMap). In a humid climate a lake
// cell costs its seepage, so the runoff placed on each catchment decides
// how many cells fill.
func TestNestedOverflow(t *testing.T) {
	g := newGrid(t, nestedMap)
	n := len(g.m.Cells)
	p := Params{SeepageMM: 100, SaltEvapShare: 0.5, InlandSeaMinCells: 20}
	_, l0 := g.balance(t, humid(n, func(int) float64 { return 0 }), p)
	ca, cb := catchment(l0, 0), catchment(l0, 1)
	if len(ca) < 6 || len(cb) < 6 || len(catchment(l0, 2)) != 0 {
		t.Fatalf("catchments A %v, B %v, P %v: want A and B to hold their cells, P none", ca, cb, catchment(l0, 2))
	}
	// The sea's land drains to the sea; every cell of A and B to its own.
	for c := range n {
		if !g.seed[c] && l0.Sink[c] == None && g.alt[c] < 500 {
			t.Errorf("cell %d at %v m drains to the sea", c, g.alt[c])
		}
	}
	a := g.cellAreas()[g.id(4, 2)]
	pass := g.id(4, 4)

	t.Run("A overflows into B", func(t *testing.T) {
		// A gets 6.5 cells' worth (fills, 0.5 over); B 3.2 (with A's 0.5,
		// 3 cells and 0.7 left over).
		rA, rB := 650/float64(len(ca)), 320/float64(len(cb))
		r, l := g.balance(t, humid(n, func(c int) float64 {
			switch l0.Sink[c] {
			case 0:
				return rA
			case 1:
				return rB
			}
			return 0
		}), p)
		if s := []State{l.Water[0].State, l.Water[1].State, l.Water[2].State}; !slices.Equal(s, []State{Full, Partial, Dry}) {
			t.Fatalf("states %v, want full, partial, dry", s)
		}
		if len(l.Lakes) != 2 {
			t.Fatalf("%d lakes, want 2", len(l.Lakes))
		}
		la, lb := l.Lakes[0], l.Lakes[1]
		if la.Basin != 0 || lb.Basin != 1 || len(la.Cells) != 6 || len(lb.Cells) != 3 {
			t.Errorf("lakes %+v, %+v: want A's 6 cells and 3 of B's", la, lb)
		}
		if !la.Full || la.SurfaceM != 400 || la.Outlet != r.Basins[0].SpillCorner || la.Salt {
			t.Errorf("A: %+v, want full at 400 m, fresh, out through its spill corner", la)
		}
		if lb.Full || lb.SurfaceM != 300 || lb.Outlet != None || !lb.Salt {
			t.Errorf("B: %+v, want partial at 300 m, closed and salt", lb)
		}
		if !near(la.Overflow, 50*a) || !near(lb.Received, 50*a) || !near(lb.Remainder, 70*a) {
			t.Errorf("A overflows %v, B receives %v and keeps %v; want %v, %v, %v", la.Overflow, lb.Received, lb.Remainder, 50*a, 50*a, 70*a)
		}
		if l.ToSea != 0 || l.Water[0].OverflowTo != 2 || l.Lake[pass] != None {
			t.Errorf("to sea %v, A overflows to %d, pass lake %d; want 0, P, none", l.ToSea, l.Water[0].OverflowTo, l.Lake[pass])
		}
	})

	t.Run("P fills and overflows to the sea", func(t *testing.T) {
		// Ten cells of runoff per catchment cell: everything fills, P's 13
		// cells cost 13 units, and the rest leaves through P's spill.
		r, l := g.balance(t, humid(n, func(c int) float64 {
			if l0.Sink[c] != None {
				return 1000
			}
			return 0
		}), p)
		for b, want := range []State{Full, Full, Full} {
			if l.Water[b].State != want {
				t.Errorf("basin %d: %v, want %v", b, l.Water[b].State, want)
			}
		}
		if len(l.Lakes) != 1 {
			t.Fatalf("%d lakes, want 1", len(l.Lakes))
		}
		k := l.Lakes[0]
		in := 1000 * a * float64(len(ca)+len(cb))
		want := in - 13*100*a
		if k.Basin != 2 || len(k.Cells) != 13 || k.SurfaceM != 600 || !k.Full || k.Salt || k.Kind != KindLake ||
			k.Outlet != r.Basins[2].SpillCorner || !near(k.Overflow, want) || !near(l.ToSea, want) {
			t.Errorf("lake %+v (to sea %v): want P's 13 cells at 600 m, fresh, overflowing %v through P's spill corner", k, l.ToSea, want)
		}
		if !slices.Contains(k.Cells, pass) {
			t.Error("the pass is not in the lake")
		}
		if l.Water[2].OverflowTo != None || l.Water[2].OverflowVia != g.id(4, 8) {
			t.Errorf("P overflows to %d via %d, want the sea via the land past its gap %d", l.Water[2].OverflowTo, l.Water[2].OverflowVia, g.id(4, 8))
		}
		// 13 cells make an inland sea from 13.
		p13 := p
		p13.InlandSeaMinCells = 13
		if _, l := g.balance(t, humid(n, func(c int) float64 { return 1000 * float64(btoi(l0.Sink[c] != None)) }), p13); l.Lakes[0].Kind != KindInlandSea {
			t.Errorf("minimum 13: kind %v, want inland sea", l.Lakes[0].Kind)
		}
	})

	t.Run("lowest sibling first", func(t *testing.T) {
		// Only A gets water, enough for A and two cells more: they go to B,
		// the one child not full.
		_, l := g.balance(t, humid(n, func(c int) float64 { return 800 * float64(btoi(l0.Sink[c] == 0)) / float64(len(ca)) }), p)
		if l.Water[0].State != Full || l.Water[1].State != Partial || len(l.Water[1].Filled) != 2 || l.Water[2].State != Dry {
			t.Errorf("A %v, B %v with %d cells, P %v; want full, partial with 2, dry", l.Water[0].State, l.Water[1].State, len(l.Water[1].Filled), l.Water[2].State)
		}
	})
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// TestChainedOverflow is the chained fixture: a top-level basin overflows
// through its spill corner into another top-level basin, not the sea.
func TestChainedOverflow(t *testing.T) {
	g := newGrid(t, chainMap)
	n := len(g.m.Cells)
	p := Params{SeepageMM: 100, SaltEvapShare: 0.5, InlandSeaMinCells: 20}
	r, l0 := g.balance(t, humid(n, func(int) float64 { return 0 }), p)
	if len(r.Basins) != 2 || r.Basins[0].Parent != None || r.Basins[1].Parent != None {
		t.Fatalf("basins %+v: want two top-level basins", r.Basins)
	}
	d, u := r.Of[g.id(3, 2)], r.Of[g.id(3, 7)]
	if d == None || u == None || d == u || r.Basins[u].SpillM != 700 || r.Basins[d].SpillM != 600 {
		t.Fatalf("D %d, U %d: want two basins spilling at 600 and 700 m", d, u)
	}
	if l0.Water[u].OverflowTo != d || l0.Water[u].OverflowVia != g.id(3, 5) || l0.Water[d].OverflowTo != None {
		t.Errorf("U overflows to %d via %d, D to %d; want D via the slope %d, the sea", l0.Water[u].OverflowTo, l0.Water[u].OverflowVia, l0.Water[d].OverflowTo, g.id(3, 5))
	}
	a := g.cellAreas()[g.id(3, 7)]
	cu := catchment(l0, u)

	// U gets 1.5 cells' worth: it fills, and half a cell runs into D, which
	// gets nothing else: D's bottom cell needs a whole one, so D stays a
	// playa with U's overflow as its remainder.
	_, l := g.balance(t, humid(n, func(c int) float64 { return 150 * float64(btoi(l0.Sink[c] == u)) / float64(len(cu)) }), p)
	if l.Water[u].State != Full || l.Water[d].State != Dry || !near(l.Water[d].Remainder, 50*a) || len(l.Playas) != 1 || l.Playas[0].Basin != d {
		t.Errorf("U %v, D %v keeping %v, playas %+v; want full, dry keeping %v, D a playa", l.Water[u].State, l.Water[d].State, l.Water[d].Remainder, l.Playas, 50*a)
	}
	if len(l.Lakes) != 1 || !near(l.Lakes[0].Overflow, 50*a) || l.Lakes[0].Outlet != r.Basins[u].SpillCorner {
		t.Errorf("lakes %+v: want U overflowing %v through its spill corner", l.Lakes, 50*a)
	}

	// Plenty: U overflows into D, which fills and overflows to the sea.
	_, l = g.balance(t, humid(n, func(c int) float64 { return 1000 * float64(btoi(l0.Sink[c] != None)) }), p)
	if len(l.Lakes) != 2 || l.Water[u].State != Full || l.Water[d].State != Full {
		t.Fatalf("lakes %+v; want U and D full", l.Lakes)
	}
	lu, ld := l.Lakes[l.Water[u].Lake], l.Lakes[l.Water[d].Lake]
	if !near(ld.Received, lu.Overflow) || lu.Overflow <= 0 || !near(ld.Overflow, l.ToSea) || ld.Overflow <= lu.Overflow {
		t.Errorf("U overflows %v, D receives %v and sends %v of %v to the sea", lu.Overflow, ld.Received, ld.Overflow, l.ToSea)
	}
}

// TestDryBasin is the dry-basin fixture: a basin below sea level in a dry
// climate has no stable level, so it stays dry land, a playa on its bottom
// cell with a dry sink at that cell's lowest corner, and its altitudes are
// untouched.
func TestDryBasin(t *testing.T) {
	g := newGrid(t, sinkMap)
	n := len(g.m.Cells)
	before := slices.Clone(g.alt)
	cl := Climate{Precipitation: make([]float64, n), PET: make([]float64, n), Runoff: make([]float64, n)}
	for c := range n {
		cl.Precipitation[c], cl.PET[c], cl.Runoff[c] = 200, 1500, 1
	}
	r, l := g.balance(t, cl, Params{SeepageMM: 50, SaltEvapShare: 0.5, InlandSeaMinCells: 20})
	bottom := g.id(3, 3)
	if len(r.Basins) != 1 || r.Basins[0].Bottom != bottom || g.alt[bottom] >= 0 {
		t.Fatalf("basins %+v: want one, its bottom %d below sea level", r.Basins, bottom)
	}
	if len(l.Lakes) != 0 || l.Water[0].State != Dry || len(l.Playas) != 1 {
		t.Fatalf("lakes %+v, state %v, playas %+v: want a dry basin", l.Lakes, l.Water[0].State, l.Playas)
	}
	best, bestH := None, 0.0
	for _, k := range g.m.Cells[bottom].Corners {
		if h := CornerHeight(g.m, g.alt, k); best == None || h < bestH || h == bestH && k < best {
			best, bestH = k, h
		}
	}
	if pl := l.Playas[0]; pl != (Playa{Basin: 0, Cell: bottom, Corner: best}) {
		t.Errorf("playa %+v, want basin 0, cell %d, sink corner %d", pl, bottom, best)
	}
	if w := l.Water[0]; w.LevelM != -50 || !near(w.Remainder, w.Inflow) || w.Inflow <= 0 || !near(l.Unplaced, w.Inflow) {
		t.Errorf("water %+v (unplaced %v): want level −50 m, the whole inflow kept", w, l.Unplaced)
	}
	if !slices.Equal(g.alt, before) {
		t.Error("altitudes changed")
	}
}

// TestOneCellLake is the one-cell-lake fixture: the basin of TestDryBasin
// with enough water for its lowest cell but not its second.
func TestOneCellLake(t *testing.T) {
	g := newGrid(t, sinkMap)
	n := len(g.m.Cells)
	p := Params{SeepageMM: 100, SaltEvapShare: 0.5, InlandSeaMinCells: 20}
	_, l0 := g.balance(t, humid(n, func(int) float64 { return 0 }), p)
	cs := catchment(l0, 0)
	a := g.cellAreas()[g.id(3, 3)]
	_, l := g.balance(t, humid(n, func(c int) float64 { return 150 * float64(btoi(l0.Sink[c] == 0)) / float64(len(cs)) }), p)
	bottom := g.id(3, 3)
	if len(l.Lakes) != 1 || !slices.Equal(l.Lakes[0].Cells, []int{bottom}) || len(l.Playas) != 0 {
		t.Fatalf("lakes %+v, playas %+v: want one lake of the bottom cell", l.Lakes, l.Playas)
	}
	k := l.Lakes[0]
	if k.SurfaceM != -50 || k.Full || k.Kind != KindLake || !k.Salt || !near(k.Remainder, 50*a) || l.Lake[bottom] != 0 || l.Lake[g.id(3, 4)] != None {
		t.Errorf("lake %+v: want one partial salt lake at −50 m keeping %v", k, 50*a)
	}
	// One cell is an inland sea when the minimum is 1.
	p.InlandSeaMinCells = 1
	if _, l := g.balance(t, humid(n, func(c int) float64 { return 150 * float64(btoi(l0.Sink[c] == 0)) / float64(len(cs)) }), p); l.Lakes[0].Kind != KindInlandSea {
		t.Errorf("minimum 1: kind %v, want inland sea", l.Lakes[0].Kind)
	}
}

// TestSalt checks the salt rule: a closed lake is salt when evaporation
// takes at least the share of its losses; an overflowing lake never is; a
// lake with no evaporation (frozen) never is.
func TestSalt(t *testing.T) {
	g := newGrid(t, sinkMap)
	n := len(g.m.Cells)
	p := Params{SeepageMM: 100, SaltEvapShare: 0.5, InlandSeaMinCells: 20}
	_, l0 := g.balance(t, humid(n, func(int) float64 { return 0 }), p)
	cs := catchment(l0, 0)
	run := func(share, seep, pet, runoff, deficit float64) Lake {
		t.Helper()
		cl := humid(n, func(c int) float64 { return runoff * float64(btoi(l0.Sink[c] == 0)) / float64(len(cs)) })
		for c := range n {
			cl.PET[c] = pet
			cl.Precipitation[c] = pet - deficit + cl.Runoff[c]
		}
		_, l := g.balance(t, cl, Params{SeepageMM: seep, SaltEvapShare: share, InlandSeaMinCells: 20})
		if len(l.Lakes) != 1 {
			t.Fatalf("%d lakes, want 1", len(l.Lakes))
		}
		return l.Lakes[0]
	}
	// PET 500, seepage 100: evaporation is 5/6 of the losses. With no
	// seepage, a deficit (PET above actual evaporation) keeps the lake
	// from filling.
	for _, tc := range []struct {
		name                              string
		share, seep, pet, runoff, deficit float64
		salt                              bool
	}{
		{"share 0.5", 0.5, 100, 500, 150, 0, true},
		{"share 5/6 exactly", 500.0 / 600, 100, 500, 150, 0, true},
		{"share 0.9", 0.9, 100, 500, 150, 0, false},
		{"share 1, no seepage", 1, 0, 500, 150, 100, true},
		{"overflowing", 0, 100, 500, 1e6, 0, false},
		{"frozen", 0, 100, 0, 150, 0, false},
	} {
		if k := run(tc.share, tc.seep, tc.pet, tc.runoff, tc.deficit); k.Salt != tc.salt {
			t.Errorf("%s: salt %v, want %v (%+v)", tc.name, k.Salt, tc.salt, k)
		}
	}
}

// TestBalanceErrors checks Balance's input checks.
func TestBalanceErrors(t *testing.T) {
	g := newGrid(t, sinkMap)
	n := len(g.m.Cells)
	r := g.find(t, 50)
	ok := humid(n, func(int) float64 { return 1 })
	p := Params{SeepageMM: 50, SaltEvapShare: 0.5, InlandSeaMinCells: 20}
	nan := humid(n, func(int) float64 { return 1 })
	nan.PET[3] = math.NaN()
	for _, tc := range []struct {
		name string
		area []float64
		cl   Climate
		p    Params
	}{
		{"short area", g.cellAreas()[1:], ok, p},
		{"short climate", g.cellAreas(), Climate{Precipitation: ok.Precipitation, PET: ok.PET[1:], Runoff: ok.Runoff}, p},
		{"NaN PET", g.cellAreas(), nan, p},
		{"negative seepage", g.cellAreas(), ok, Params{SeepageMM: -1, SaltEvapShare: 0.5, InlandSeaMinCells: 20}},
		{"share 2", g.cellAreas(), ok, Params{SeepageMM: 50, SaltEvapShare: 2, InlandSeaMinCells: 20}},
		{"inland sea 0", g.cellAreas(), ok, Params{SeepageMM: 50, SaltEvapShare: 0.5}},
	} {
		if _, err := Balance(g.m, g.alt, g.seed, tc.area, r, tc.cl, tc.p); err == nil {
			t.Errorf("%s: no error", tc.name)
		}
	}
}

// TestBalanceDeterministic runs Balance twice and compares the encodings.
func TestBalanceDeterministic(t *testing.T) {
	g := newGrid(t, nestedMap)
	n := len(g.m.Cells)
	cl := humid(n, func(c int) float64 { return float64(c % 7 * 40) })
	p := Params{SeepageMM: 50, SaltEvapShare: 0.5, InlandSeaMinCells: 20}
	_, a := g.balance(t, cl, p)
	_, b := g.balance(t, cl, p)
	x, _ := a.AppendBinary(nil)
	y, _ := b.AppendBinary(nil)
	if !bytes.Equal(x, y) {
		t.Error("two runs differ")
	}
	if !strings.Contains(Full.String()+Partial.String()+Dry.String()+KindInlandSea.String(), "inland-sea") {
		t.Error("names")
	}
}

// tieMap is the same-level overflow fixture (S29): two top-level basins, A
// (one cell at 200 m, row 3, column 2) and B (one cell at 300 m, row 3,
// column 5), close at one pass flat at 600 m (row 3, columns 3 and 4),
// which also touches a 500 m slope (row 2, column 4) running by a 400 m
// cell (row 1, column 4) to the rim. Across the flat each basin's lowest
// outside neighbor is the other basin, so with both full their overflow
// would pass back and forth for ever (the S28 balance recursed until the
// stack overflowed). The water must leave the flat for the sea instead.
const tieMap = `
~   ~   ~   ~   ~   ~   ~   ~
900 900 900 900 400 900 900 900
900 900 900 900 500 900 900 900
900 900 200 600 600 300 900 900
900 900 900 900 900 900 900 900
900 900 900 900 900 900 900 900
~   ~   ~   ~   ~   ~   ~   ~
`

// TestSameLevelOverflow is the same-level overflow fixture: A fills and
// overflows into B across their shared pass; B, once full, sends its
// overflow (A's included) to the sea down the slope, not back to A.
func TestSameLevelOverflow(t *testing.T) {
	g := newGrid(t, tieMap)
	n := len(g.m.Cells)
	p := Params{SeepageMM: 100, SaltEvapShare: 0.5, InlandSeaMinCells: 20}
	r := g.find(t, 50)
	a, b := r.Of[g.id(3, 2)], r.Of[g.id(3, 5)]
	if len(r.Basins) != 2 || a == None || b == None || a == b ||
		r.Basins[a].Parent != None || r.Basins[b].Parent != None || r.Basins[a].SpillM != 600 || r.Basins[b].SpillM != 600 {
		t.Fatalf("basins %+v: want two top-level basins spilling at 600 m", r.Basins)
	}
	_, l := g.balance(t, humid(n, func(int) float64 { return 1000 }), p)
	if l.Water[a].State != Full || l.Water[b].State != Full {
		t.Fatalf("A %v, B %v: want both full", l.Water[a].State, l.Water[b].State)
	}
	first, second := a, b // the lower id settles first and overflows into the other
	if b < a {
		first, second = b, a
	}
	if l.Water[first].OverflowTo != second {
		t.Errorf("first basin overflows to %d, want the other basin %d", l.Water[first].OverflowTo, second)
	}
	if l.Water[second].OverflowTo != None || l.Water[second].OverflowVia != g.id(2, 4) {
		t.Errorf("second basin overflows to %d via %d, want the sea via the slope %d", l.Water[second].OverflowTo, l.Water[second].OverflowVia, g.id(2, 4))
	}
	if len(l.Lakes) != 2 || l.ToSea <= 0 || !near(l.ToSea, l.Water[second].Overflow) {
		t.Errorf("lakes %d, to sea %v, second basin's overflow %v", len(l.Lakes), l.ToSea, l.Water[second].Overflow)
	}
}
