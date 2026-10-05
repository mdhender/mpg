// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package basin

import (
	"bytes"
	"math"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// nestedMap is the nested-basin fixture. Inside a wall (900 m) lie two
// pits: A (cells at 100 and 200 m) and B (a flat floor at 300 m), divided
// by a wall broken only by the pass at 400 m (row 4, column 4). The outer
// wall's one gap is the pass at 600 m (row 4, column 7), which opens onto
// the 500 m land that drains to the sea (the rim rows).
//
// Expected: A and B are 300 m and 100 m deep, overflowing at the 400 m pass
// into their parent P, which spills at 600 m to the sea, 500 m deep.
const nestedMap = `
~   ~   ~   ~   ~   ~   ~   ~   ~   ~   ~   ~
500 500 500 500 500 500 500 500 500 500 500 500
500 900 900 900 900 900 900 900 500 500 500 500
500 900 100 200 900 300 300 900 500 500 500 500
500 900 200 200 400 300 300 600 500 500 500 500
500 900 200 200 900 300 300 900 500 500 500 500
500 900 900 900 900 900 900 900 500 500 500 500
500 500 500 500 500 500 500 500 500 500 500 500
500 500 500 500 500 500 500 500 500 500 500 500
~   ~   ~   ~   ~   ~   ~   ~   ~   ~   ~   ~
`

// cellsOf returns the ids of the cells (r, c) for r in rows, c in cols,
// ascending.
func (g *grid) cellsOf(rows, cols []int) []int {
	var ids []int
	for _, r := range rows {
		for _, c := range cols {
			ids = append(ids, g.id(r, c))
		}
	}
	slices.Sort(ids)
	return ids
}

func span(lo, hi int) []int {
	var v []int
	for i := lo; i <= hi; i++ {
		v = append(v, i)
	}
	return v
}

// checkBasin compares basin b of bs with the expected fields.
func checkBasin(t *testing.T, name string, bs []Basin, b int, want Basin) {
	t.Helper()
	got := bs[b]
	if got.Parent != want.Parent || !slices.Equal(got.Children, want.Children) ||
		got.Bottom != want.Bottom || got.BottomM != want.BottomM || got.SpillM != want.SpillM || got.DepthM != want.DepthM ||
		got.SpillCell != want.SpillCell || got.SpillEdge != want.SpillEdge || got.SpillCorner != want.SpillCorner ||
		got.Size != want.Size || !slices.Equal(got.Cells, want.Cells) {
		t.Errorf("%s (%d):\n got %+v\nwant %+v", name, b, got, want)
	}
}

// TestNestedBasin is the nested-basin fixture: an inner basin overflows into
// its outer one, which spills to the sea.
func TestNestedBasin(t *testing.T) {
	g := newGrid(t, nestedMap)
	r := g.find(t, 50)
	a := g.cellsOf(span(3, 5), span(2, 3))
	b := g.cellsOf(span(3, 5), span(5, 6))
	pass, gap := g.id(4, 4), g.id(4, 7)
	ab := slices.Sorted(slices.Values(append(append(slices.Clone(a), b...), pass)))

	if len(r.Depressions) != 3 || len(r.Basins) != 3 {
		t.Fatalf("%d depressions, %d basins; want 3, 3", len(r.Depressions), len(r.Basins))
	}
	// A closes first (the pass at 400 m is the first flat to touch both),
	// then B, in id order; P is made at the pass.
	sc, se, sk := g.spillAt(t, []int{pass}, a)
	checkBasin(t, "A", r.Basins, 0, Basin{Parent: 2, Bottom: g.id(3, 2), BottomM: 100, SpillM: 400, DepthM: 300,
		SpillCell: sc, SpillEdge: se, SpillCorner: sk, Size: 6, Cells: a})
	sc, se, sk = g.spillAt(t, []int{pass}, b)
	checkBasin(t, "B", r.Basins, 1, Basin{Parent: 2, Bottom: g.id(3, 5), BottomM: 300, SpillM: 400, DepthM: 100,
		SpillCell: sc, SpillEdge: se, SpillCorner: sk, Size: 6, Cells: b})
	sc, se, sk = g.spillAt(t, []int{gap}, ab)
	checkBasin(t, "P", r.Basins, 2, Basin{Parent: None, Children: []int{0, 1}, Bottom: g.id(3, 2), BottomM: 100, SpillM: 600, DepthM: 500,
		SpillCell: sc, SpillEdge: se, SpillCorner: sk, Size: 13, Cells: []int{pass}})
	if sc != gap || r.Basins[0].SpillCell != pass || r.Basins[1].SpillCell != pass {
		t.Errorf("spill cells %d, %d, %d; want the passes %d, %d, %d", r.Basins[0].SpillCell, r.Basins[1].SpillCell, sc, pass, pass, gap)
	}
	// The spill corner ends the spill edge, and the edge joins the pass to
	// the basin.
	for k, bs := range r.Basins {
		e := g.m.Edges[bs.SpillEdge]
		if e.Corners[0] != bs.SpillCorner && e.Corners[1] != bs.SpillCorner {
			t.Errorf("basin %d: spill corner %d not on spill edge %d", k, bs.SpillCorner, bs.SpillEdge)
		}
		if o := e.Other(bs.SpillCell); o < 0 || r.Of[o] != k && (r.Of[o] < 0 || r.Basins[r.Of[o]].Parent != k) {
			t.Errorf("basin %d: spill edge %d does not join the pass %d to the basin", k, bs.SpillEdge, bs.SpillCell)
		}
	}
	for k := range r.Basins {
		if want := []int{1, 1, 0}[k]; r.Nesting(k) != want {
			t.Errorf("Nesting(%d) = %d, want %d", k, r.Nesting(k), want)
		}
	}
	for c := range g.m.Cells {
		want := None
		switch {
		case slices.Contains(a, c):
			want = 0
		case slices.Contains(b, c):
			want = 1
		case c == pass:
			want = 2
		}
		if r.Of[c] != want || r.Depression[c] != want {
			t.Errorf("cell %d: basin %d, depression %d; want %d", c, r.Of[c], r.Depression[c], want)
		}
		if r.RouteM[c] != g.alt[c] {
			t.Errorf("cell %d: RouteM %v, want its altitude %v", c, r.RouteM[c], g.alt[c])
		}
	}
	if !slices.Equal(r.BasinOf, []int{0, 1, 2}) {
		t.Errorf("BasinOf = %v", r.BasinOf)
	}
}

// TestBelowMinimum is the below-minimum fixture: a shallow nested sub-basin
// merges into its parent, a shallow top-level depression is no basin and
// routes as flat ground at its spill level, a depression exactly the
// minimum deep counts, and a deep parent of two shallow pits keeps their
// cells and no children.
func TestBelowMinimum(t *testing.T) {
	// B's floor is 380 m: 20 m below the 400 m pass. Row 7, column 10 is a
	// one-cell pit 20 m below the land around it.
	text := strings.Replace(nestedMap, "300", "380", -1)
	lines := strings.Split(strings.TrimSpace(text), "\n")
	f := strings.Fields(lines[7])
	f[10] = "480"
	lines[7] = strings.Join(f, " ")
	g := newGrid(t, strings.Join(lines, "\n"))
	a := g.cellsOf(span(3, 5), span(2, 3))
	b := g.cellsOf(span(3, 5), span(5, 6))
	pass, pit := g.id(4, 4), g.id(7, 10)

	r := g.find(t, 50)
	if len(r.Depressions) != 4 {
		t.Fatalf("%d depressions, want 4 (A, B, the pit, P)", len(r.Depressions))
	}
	// Depressions are numbered as they form: A at 100 m, B at 380 m, P at
	// the 400 m pass, the pit at 480 m.
	if !slices.Equal(r.BasinOf, []int{0, None, 1, None}) {
		t.Fatalf("BasinOf = %v, want [0 -1 1 -1]: A, B dropped, P, the pit dropped", r.BasinOf)
	}
	if d := r.Depressions[1]; d.DepthM != 20 || d.Parent != 2 {
		t.Errorf("B: %+v, want 20 m deep in P", d)
	}
	if d := r.Depressions[3]; d.DepthM != 20 || d.Parent != None || d.SpillM != 500 || !slices.Equal(d.Cells, []int{pit}) {
		t.Errorf("pit: %+v, want 20 m deep, spilling at 500 m to the sea", d)
	}
	pb := r.Basins[1]
	wantCells := slices.Sorted(slices.Values(append(slices.Clone(b), pass)))
	if pb.Parent != None || !slices.Equal(pb.Children, []int{0}) || pb.Size != 13 || !slices.Equal(pb.Cells, wantCells) || pb.DepthM != 500 {
		t.Errorf("P: %+v, want children [0], size 13, B's cells and the pass its own", pb)
	}
	for _, c := range b {
		if r.Of[c] != 1 || r.Depression[c] != 1 {
			t.Errorf("B cell %d: basin %d, depression %d; want 1, 1", c, r.Of[c], r.Depression[c])
		}
		// B is shallow, merged into P: it routes as flat ground at its
		// spill, the 400 m pass.
		if r.RouteM[c] != 400 || g.alt[c] != 380 {
			t.Errorf("B cell %d: RouteM %v, altitude %v; want 400, 380", c, r.RouteM[c], g.alt[c])
		}
	}
	for _, c := range a {
		if r.RouteM[c] != g.alt[c] {
			t.Errorf("A cell %d: RouteM %v, want its altitude %v", c, r.RouteM[c], g.alt[c])
		}
	}
	if r.Of[pit] != None || r.Depression[pit] != 3 || r.RouteM[pit] != 500 || g.alt[pit] != 480 {
		t.Errorf("pit: basin %d, depression %d, RouteM %v, altitude %v; want none, 3, 500, 480", r.Of[pit], r.Depression[pit], r.RouteM[pit], g.alt[pit])
	}

	// At exactly B's depth B counts; above it, it does not.
	if r := g.find(t, 20); len(r.Basins) != 4 {
		t.Errorf("minimum 20 m: %d basins, want 4", len(r.Basins))
	}
	if r := g.find(t, math.Nextafter(20, 21)); len(r.Basins) != 2 {
		t.Errorf("minimum just over 20 m: %d basins, want 2", len(r.Basins))
	}
	// A minimum of 0 keeps every depression; one deeper than P keeps none.
	if r := g.find(t, 0); len(r.Basins) != 4 {
		t.Errorf("minimum 0: %d basins, want 4", len(r.Basins))
	}
	if r := g.find(t, 501); len(r.Basins) != 0 || slices.ContainsFunc(r.Of, func(b int) bool { return b != None }) {
		t.Errorf("minimum 501 m: %d basins, want none", len(r.Basins))
	} else if r.RouteM[a[0]] != 600 || r.RouteM[pass] != 600 {
		t.Errorf("minimum 501 m: RouteM %v, %v; want P's spill, 600", r.RouteM[a[0]], r.RouteM[pass])
	}

	// Two shallow pits in a deep parent: both merge, leaving no children.
	text = strings.Replace(strings.Replace(nestedMap, "300", "380", -1), "100 200", "370 380", 1)
	text = strings.Replace(text, "200 200", "380 380", -1)
	g = newGrid(t, text)
	r = g.find(t, 50)
	if len(r.Depressions) != 3 || len(r.Basins) != 1 {
		t.Fatalf("shallow pits: %d depressions, %d basins; want 3, 1", len(r.Depressions), len(r.Basins))
	}
	if p := r.Basins[0]; len(p.Children) != 0 || len(p.Cells) != 13 || p.BottomM != 370 || p.DepthM != 230 {
		t.Errorf("shallow pits: %+v, want 13 own cells, no children, 230 m deep", p)
	}
}

// TestPlateaus checks equal altitudes: a flat floor is one depression whose
// bottom is its lowest cell id; a flat pass of several cells closes both
// pits at its level, with no zero-depth depression; and a flat that touches
// the sea drains every depression it touches to the sea.
func TestPlateaus(t *testing.T) {
	// The pass is two cells at 400 m (rows 3 and 4, column 4).
	text := strings.Replace(nestedMap, "100 200 900 300", "100 200 400 300", 1)
	g := newGrid(t, text)
	r := g.find(t, 0)
	flat := []int{g.id(3, 4), g.id(4, 4)}
	if len(r.Depressions) != 3 {
		t.Fatalf("%d depressions, want 3", len(r.Depressions))
	}
	for k, d := range r.Depressions {
		if !(d.DepthM > 0) {
			t.Errorf("depression %d is %v m deep", k, d.DepthM)
		}
	}
	a := g.cellsOf(span(3, 5), span(2, 3))
	b := g.cellsOf(span(3, 5), span(5, 6))
	for k, set := range [][]int{a, b} {
		sc, se, sk := g.spillAt(t, flat, set)
		d := r.Depressions[k]
		if d.SpillM != 400 || d.Parent != 2 || d.SpillCell != sc || d.SpillEdge != se || d.SpillCorner != sk {
			t.Errorf("depression %d: %+v, want spill 400 m at cell %d edge %d corner %d", k, d, sc, se, sk)
		}
	}
	if d := r.Depressions[1]; d.Bottom != g.id(3, 5) {
		t.Errorf("B's bottom %d, want %d, the lowest id on its flat floor", d.Bottom, g.id(3, 5))
	}
	if p := r.Depressions[2]; !slices.Equal(p.Cells, flat) || p.SpillM != 600 {
		t.Errorf("P: %+v, want the flat pass as its own cells, spilling at 600 m", p)
	}

	// Open the outer wall's gap down to 500 m, the level of the land
	// outside: P's pass is now a flat joined to the sea.
	g = newGrid(t, strings.Replace(nestedMap, "300 300 600", "300 300 500", 1))
	r = g.find(t, 0)
	if p := r.Depressions[2]; p.SpillM != 500 || p.SpillCell != g.id(4, 7) || p.Parent != None {
		t.Errorf("P: %+v, want spill 500 m at the gap %d", p, g.id(4, 7))
	}
}

// TestAltitudesUnchanged checks that Find never modifies the altitudes, on
// the fixtures and (in world_test.go) on real worlds.
func TestAltitudesUnchanged(t *testing.T) {
	g := newGrid(t, nestedMap)
	before := slices.Clone(g.alt)
	r := g.find(t, 50)
	if !slices.Equal(bits(before), bits(g.alt)) {
		t.Error("Find changed the altitudes")
	}
	// RouteM is a copy, not the altitudes.
	r.RouteM[0] = -1
	if g.alt[0] == -1 {
		t.Error("RouteM aliases the altitudes")
	}
}

func bits(v []float64) []uint64 {
	b := make([]uint64, len(v))
	for i, x := range v {
		b[i] = math.Float64bits(x)
	}
	return b
}

func TestFindErrors(t *testing.T) {
	g := newGrid(t, nestedMap)
	noSea := make([]bool, len(g.seed))
	nan := slices.Clone(g.alt)
	nan[20] = math.NaN()
	// An island of sea in a ring of land, seeding only part of the world:
	// every other cell drains to it, so Find succeeds even without the rim.
	part := make([]bool, len(g.seed))
	part[g.id(7, 9)] = true
	for _, tc := range []struct {
		name  string
		alt   []float64
		seed  []bool
		depth float64
	}{
		{"short alt", g.alt[1:], g.seed, 50},
		{"short seed", g.alt, g.seed[1:], 50},
		{"NaN", nan, g.seed, 50},
		{"no sea", g.alt, noSea, 50},
		{"negative depth", g.alt, g.seed, -1},
		{"NaN depth", g.alt, g.seed, math.NaN()},
		{"Inf depth", g.alt, g.seed, math.Inf(1)},
	} {
		if _, err := Find(g.m, tc.alt, tc.seed, tc.depth); err == nil {
			t.Errorf("%s: no error", tc.name)
		}
	}
	if _, err := Find(g.m, g.alt, part, 50); err != nil {
		t.Errorf("one sea cell: %v", err)
	}
}

// TestDeterministic runs Find twice on a fixture and checks the encodings
// match.
func TestDeterministic(t *testing.T) {
	g := newGrid(t, nestedMap)
	a, _ := g.find(t, 50).AppendBinary(nil)
	b, _ := g.find(t, 50).AppendBinary(nil)
	if !bytes.Equal(a, b) {
		t.Error("two runs differ")
	}
}

// TestNoFusedMultiplyAdd compiles this package for the architectures whose Go
// compilers fuse a*b+c into one instruction and fails if any fused
// multiply-add or multiply-subtract appears in its code.
func TestNoFusedMultiplyAdd(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles for several architectures")
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool not found")
	}
	fusedOp := regexp.MustCompile(`\tV?FN?M(ADD|SUB)`)
	const pkg = "github.com/mdhender/mpg/internal/basin"
	for _, tg := range []struct{ arch, env string }{{"arm64", ""}, {"amd64", "GOAMD64=v3"}} {
		t.Run(tg.arch, func(t *testing.T) {
			cmd := exec.Command(goTool, "build", "-gcflags="+pkg+"=-S", pkg)
			cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+tg.arch, "CGO_ENABLED=0")
			if tg.env != "" {
				cmd.Env = append(cmd.Env, tg.env)
			}
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("go build %s: %v\n%s", pkg, err, out)
			}
			if m := fusedOp.FindAllString(string(out), -1); len(m) != 0 {
				t.Errorf("%s: %d fused multiply-add instructions in %s", tg.arch, len(m), pkg)
			}
		})
	}
}
