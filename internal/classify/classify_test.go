// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package classify

import (
	"bytes"
	"image/color"
	"math"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"testing"

	"github.com/mdhender/mpg/internal/cells"
	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/mesh"
	"github.com/mdhender/mpg/internal/topo"
)

// hm are hmz2ter's rules, which the hand-built cases below are written
// against.
var hm = Rules{
	FlatsBelowM: 20, PlainsBelowM: 60, RollingPlainsBelowM: 150, HillsBelowM: 350,
	PlateauMinAltitudeM: 500, PlateauBelowM: 150, VolcanicRadiusKm: 25,
	ShallowMaxCells: 12, OpenMaxCells: 19,
}

func TestLandformRule(t *testing.T) {
	for _, tc := range []struct {
		relief, height float64
		want           Landform
	}{
		{0, 0, Flats},
		{19.9, 100, Flats},
		{20, 100, Plains},
		{59.9, -300, Plains}, // a dry basin floor below sea level is land like any other
		{60, 100, RollingPlains},
		{149.9, 499.9, RollingPlains},
		{150, 100, Hills},
		{349.9, 100, Hills},
		{350, 100, Mountains},
		{2000, 3000, Mountains},
		{149.9, 500, Plateaus}, // the plateau rule wins over the relief class
		{10, 1500, Plateaus},
		{150, 500, Hills}, // relief at the plateau limit: not a plateau
		{100, 499.9, RollingPlains},
	} {
		if got := hm.Landform(tc.relief, tc.height); got != tc.want {
			t.Errorf("Landform(relief %v, height %v) = %s, want %s", tc.relief, tc.height, got, tc.want)
		}
	}
}

func TestDepthRule(t *testing.T) {
	for steps, want := range map[int]Depth{1: Shallow, 12: Shallow, 13: Open, 19: Open, 20: Deep, 500: Deep} {
		if got := hm.Depth(steps); got != want {
			t.Errorf("Depth(%d) = %s, want %s", steps, got, want)
		}
	}
}

func TestNames(t *testing.T) {
	var got []string
	for _, l := range Landforms {
		got = append(got, l.String())
	}
	want := []string{"salt-water", "fresh-water", "flats", "plains", "rolling-plains", "hills", "mountains", "plateaus", "volcanic-highlands"}
	if !slices.Equal(got, want) {
		t.Errorf("landform names %q, want %q", got, want)
	}
	for _, l := range LandLandforms {
		if !l.IsLand() {
			t.Errorf("%s is not land", l)
		}
	}
	if SaltWater.IsLand() || FreshWater.IsLand() || LandformNone.IsLand() {
		t.Error("water or unset is land")
	}
	if Shallow.String() != "shallow" || Open.String() != "open" || Deep.String() != "deep" || DepthNone.String() != "" {
		t.Error("depth names")
	}
	if Landform(200).String() != "Landform(200)" || Depth(9).String() != "Depth(9)" {
		t.Error("out-of-range names")
	}
}

func TestRulesOfDefault(t *testing.T) {
	c := config.Default()
	r := RulesOf(c)
	d := c.Classify
	if r.FlatsBelowM != d.FlatsBelowM || r.HillsBelowM != d.HillsBelowM || r.VolcanicRadiusKm != 25 || r.OpenMaxCells != d.OpenMaxCells {
		t.Errorf("RulesOf = %+v, config %+v", r, d)
	}
}

// smallWorld returns a resolved config and mesh of about land/0.3 playable
// cells, with a raster of the same cylinder for renders.
func smallWorld(t testing.TB, seed uint64, aspect string, land int) (*mesh.Mesh, *field.Field) {
	t.Helper()
	c := config.Default()
	c.Seed = config.Seed(seed)
	c.World.Aspect = aspect
	c.World.LandCells = land
	c.Rim.FalloffCells = 4
	if err := c.Resolve(); err != nil {
		t.Fatal(err)
	}
	m, err := mesh.New(c)
	if err != nil {
		t.Fatal(err)
	}
	f, err := field.FromConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	return m, f
}

// world holds hand-set altitudes and reliefs over a small mesh.
type world struct {
	m        *mesh.Mesh
	alt, rel []float64
}

func newWorld(m *mesh.Mesh, alt, rel float64) *world {
	w := &world{m: m, alt: make([]float64, len(m.Cells)), rel: make([]float64, len(m.Cells))}
	for i := range w.alt {
		w.alt[i], w.rel[i] = alt, rel
	}
	return w
}

func (w *world) classify(t *testing.T, level float64, peaks []topo.Point, r Rules) *Result {
	t.Helper()
	res, err := Classify(w.m, w.alt, w.rel, cells.Classify(w.m, w.alt, level), nil, peaks, r)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// interior returns playable cells none of whose neighbors is a rim cell,
// starting near the middle of the cell list.
func interior(m *mesh.Mesh) []int {
	var out []int
	for k := range len(m.Cells) {
		i := (len(m.Cells)/2 + k) % len(m.Cells)
		if !m.Cells[i].Rim && !slices.ContainsFunc(m.Cells[i].Neighbors, func(j int) bool { return m.Cells[j].Rim }) {
			out = append(out, i)
		}
	}
	return out
}

// TestClassifyLandforms checks the landform of hand-set land cells, a dry
// basin floor, ocean, and the rim, with altitudes measured from the sea
// level rather than from 0 m.
func TestClassifyLandforms(t *testing.T) {
	m, _ := smallWorld(t, 1, "cinematic", 600)
	const level = 100
	w := newWorld(m, level+50, 10) // land: flats, 50 m above sea level
	in := interior(m)
	type want struct {
		alt, rel float64
		lf       Landform
	}
	cases := []want{
		{level + 50, 30, Plains},
		{level + 50, 100, RollingPlains},
		{level + 50, 200, Hills},
		{level + 50, 400, Mountains},
		{level + 600, 100, Plateaus},
		{level + 450, 100, RollingPlains}, // 550 m above 0 m but only 450 m above sea level
		{level - 300, 25, Plains},         // a dry basin floor, enclosed by land
	}
	for k, c := range cases {
		w.alt[in[10*k]], w.rel[in[10*k]] = c.alt, c.rel
	}
	// Ocean: every cell beside the rim and every rim cell, below sea level.
	for i, c := range m.Cells {
		if c.Rim || slices.ContainsFunc(c.Neighbors, func(j int) bool { return m.Cells[j].Rim }) {
			w.alt[i] = level - 1000
		}
	}
	res := w.classify(t, level, nil, hm)
	for k, c := range cases {
		if got := res.Landform[in[10*k]]; got != c.lf {
			t.Errorf("case %d (altitude %v, relief %v): %s, want %s", k, c.alt, c.rel, got, c.lf)
		}
		if res.Depth[in[10*k]] != DepthNone || res.SeaSteps[in[10*k]] != 0 {
			t.Errorf("case %d: land with depth %s, sea steps %d", k, res.Depth[in[10*k]], res.SeaSteps[in[10*k]])
		}
	}
	flats := 0
	for i, c := range m.Cells {
		switch {
		case c.Rim:
			if res.Landform[i] != SaltWater || res.Depth[i] != Deep || res.SeaSteps[i] != -1 {
				t.Errorf("rim cell %d: %s %s %d; want deep salt water, no steps", i, res.Landform[i], res.Depth[i], res.SeaSteps[i])
			}
		case w.alt[i] == level-1000:
			if res.Landform[i] != SaltWater || res.Depth[i] == DepthNone {
				t.Errorf("ocean cell %d: %s %s", i, res.Landform[i], res.Depth[i])
			}
		case res.Landform[i] == Flats:
			flats++
		}
		if res.Landform[i] == LandformNone {
			t.Errorf("cell %d unset", i)
		}
	}
	counts, total := res.LandHistogram()
	if counts[0] != flats || total != flats+len(cases) {
		t.Errorf("histogram %v total %d; want %d flats of %d land", counts, total, flats, flats+len(cases))
	}
	if got := res.Count(m, Flats, true); got != flats {
		t.Errorf("Count(flats) = %d, want %d", got, flats)
	}
	shares := res.LandShares(true)
	if len(shares) != len(LandLandforms) || shares[1] != "pl0" {
		t.Errorf("compact shares %q", shares)
	}
}

// TestVolcanoFlag checks that a peak in a land cell flags that cell (the
// cell of the nearest site) and one in a sea cell flags nothing.
func TestVolcanoFlag(t *testing.T) {
	m, _ := smallWorld(t, 2, "square", 600)
	w := newWorld(m, 100, 500)
	in := interior(m)
	sea, land := in[0], in[len(in)/2]
	w.alt[sea] = -500
	peakLand := m.Cells[land].Site
	peakLand.X += 0.3 // off the site, still nearest to it
	peakSea := m.Cells[sea].Site
	res, err := Classify(m, w.alt, w.rel, oceanAt(m, sea), nil, []topo.Point{peakLand, peakSea}, hm)
	if err != nil {
		t.Fatal(err)
	}
	if res.VolcanoCell[0] != land || !res.VolcanoLand[0] || !res.Volcano[land] {
		t.Errorf("land peak: cell %d (want %d), land %v, flag %v", res.VolcanoCell[0], land, res.VolcanoLand[0], res.Volcano[land])
	}
	if res.Landform[sea] != SaltWater || res.VolcanoLand[1] || res.Volcano[sea] {
		t.Errorf("sea peak: cell %s, land %v, flag %v", res.Landform[sea], res.VolcanoLand[1], res.Volcano[sea])
	}
	if res.VolcanoCell[1] != sea {
		t.Errorf("sea peak in cell %d, want %d", res.VolcanoCell[1], sea)
	}
	if res.Volcanoes() != 1 {
		t.Errorf("Volcanoes() = %d, want 1", res.Volcanoes())
	}
	for i, v := range res.Volcano {
		if v && i != land {
			t.Errorf("cell %d flagged", i)
		}
	}
	// The volcano cell agrees with the sample assignment's nearest site.
	sites := make([]topo.Point, len(m.Cells))
	for i, c := range m.Cells {
		sites[i] = c.Site
	}
	if b := cells.BruteNearest(m.Cylinder(), sites, peakLand); b != land {
		t.Errorf("brute-force nearest site %d, want %d", b, land)
	}
}

// TestVolcanicHighlandsSeam checks the radius rule with the wrapped
// distance: with every cell a plateau, exactly the cells whose sites lie
// within the radius of a volcano's peak, across the seam too, become
// volcanic highlands, and a hotspot whose peak is in the sea makes none.
func TestVolcanicHighlandsSeam(t *testing.T) {
	m, _ := smallWorld(t, 3, "cinematic", 600)
	cyl := m.Cylinder()
	w := newWorld(m, 1000, 50) // every playable cell a plateau
	// A peak 2 km west of the seam, at mid height.
	peak := topo.Point{X: cyl.W() - 2, Y: cyl.H() / 2}
	r := hm
	r.VolcanicRadiusKm = 30
	res := w.classify(t, 0, []topo.Point{peak}, r)
	if !res.VolcanoLand[0] {
		t.Fatal("the peak is not on land")
	}
	east := 0
	for i, c := range m.Cells {
		if c.Rim {
			continue
		}
		near := cyl.Distance(c.Site, peak) <= r.VolcanicRadiusKm
		want := Plateaus
		if near {
			want = VolcanicHighlands
			if c.Site.X < cyl.W()/2 {
				east++
			}
		}
		if res.Landform[i] != want {
			t.Errorf("cell %d at (%.1f, %.1f), %.1f km from the peak: %s, want %s",
				i, c.Site.X, c.Site.Y, cyl.Distance(c.Site, peak), res.Landform[i], want)
		}
	}
	if east == 0 {
		t.Error("no volcanic highland across the seam")
	}

	// The same peak in the sea: no volcano, no highlands.
	w.alt[res.VolcanoCell[0]] = -100
	res2, err := Classify(m, w.alt, w.rel, oceanAt(m, res.VolcanoCell[0]), nil, []topo.Point{peak}, r)
	if err != nil {
		t.Fatal(err)
	}
	if res2.VolcanoLand[0] || res2.Count(m, VolcanicHighlands, true) != 0 {
		t.Errorf("sea peak: volcano %v, %d highlands", res2.VolcanoLand[0], res2.Count(m, VolcanicHighlands, true))
	}
}

// oceanAt returns a flood at sea level 0 in which every playable cell is
// land except cell c, which is ocean.
func oceanAt(m *mesh.Mesh, c int) *cells.Flood {
	n := len(m.Cells)
	f := &cells.Flood{Ocean: make([]bool, n), Basin: make([]bool, n), Land: make([]bool, n)}
	for i, cell := range m.Cells {
		if !cell.Rim {
			f.Land[i] = i != c
			f.Ocean[i] = i == c
		}
	}
	return f
}

// TestDepthSteps checks the sea steps and depth bands by breadth-first
// distance over cell adjacency from a single land cell, with bands of one
// and two steps: the land cell's neighbors are shallow, their other
// neighbors open, the rest deep; every salt-water cell's steps are one more
// than its nearest neighbor's; and the rim is deep, never stepped through.
func TestDepthSteps(t *testing.T) {
	m, _ := smallWorld(t, 4, "square", 600)
	w := newWorld(m, -1000, 0)
	land := interior(m)[0]
	w.alt[land] = 100
	r := hm
	r.ShallowMaxCells, r.OpenMaxCells = 1, 2
	res := w.classify(t, 0, nil, r)
	ring1 := m.Cells[land].Neighbors
	for i, c := range m.Cells {
		switch {
		case c.Rim:
			if res.SeaSteps[i] != -1 || res.Depth[i] != Deep {
				t.Errorf("rim cell %d: steps %d, %s", i, res.SeaSteps[i], res.Depth[i])
			}
			continue
		case i == land:
			if res.SeaSteps[i] != 0 || res.Depth[i] != DepthNone {
				t.Errorf("land cell: steps %d, %s", res.SeaSteps[i], res.Depth[i])
			}
			continue
		}
		s := res.SeaSteps[i]
		if s < 1 {
			t.Errorf("cell %d: steps %d", i, s)
			continue
		}
		best := math.MaxInt
		for _, j := range c.Neighbors {
			if !m.Cells[j].Rim && res.SeaSteps[j] >= 0 {
				best = min(best, res.SeaSteps[j])
			}
		}
		if s != best+1 {
			t.Errorf("cell %d: steps %d, nearest neighbor %d", i, s, best)
		}
		want := Deep
		switch {
		case slices.Contains(ring1, i):
			want = Shallow
			if s != 1 {
				t.Errorf("neighbor %d: steps %d", i, s)
			}
		case s == 2:
			want = Open
		}
		if res.Depth[i] != want {
			t.Errorf("cell %d at %d steps: %s, want %s", i, s, res.Depth[i], want)
		}
	}

	// With no land at all, every salt-water cell is deep and unreached.
	w.alt[land] = -1000
	res = w.classify(t, 0, nil, r)
	for i := range m.Cells {
		if res.SeaSteps[i] != -1 || res.Depth[i] != Deep {
			t.Fatalf("no land: cell %d steps %d, %s", i, res.SeaSteps[i], res.Depth[i])
		}
	}
}

// TestDepthCornerContact checks that salt water touching land only at a
// 4-way corner left by a collapsed short edge is two steps from it, not
// one: the two cells across the collapsed edge are not neighbors.
func TestDepthCornerContact(t *testing.T) {
	m, _ := smallWorld(t, 2, "square", 600)
	if m.Collapses == 0 {
		t.Fatal("the test mesh has no collapsed edge")
	}
	tried := 0
	for _, corner := range m.Corners {
		if len(corner.Cells) != 4 || slices.ContainsFunc(corner.Cells, func(i int) bool { return m.Cells[i].Rim }) {
			continue
		}
		p, q := -1, -1
		for a, i := range corner.Cells {
			for _, j := range corner.Cells[a+1:] {
				if !slices.Contains(m.Cells[i].Neighbors, j) {
					p, q = i, j
				}
			}
		}
		if p < 0 {
			continue
		}
		w := newWorld(m, -1000, 0)
		w.alt[p] = 100
		res := w.classify(t, 0, nil, hm)
		if res.SeaSteps[q] != 2 {
			t.Errorf("cells %d and %d meet only at a corner: %d is %d steps from land %d, want 2", p, q, q, res.SeaSteps[q], p)
		}
		tried++
		if tried == 5 {
			break
		}
	}
	if tried == 0 {
		t.Fatal("no 4-way corner to test")
	}
}

// TestDeterminism classifies a world twice and compares the encodings.
func TestDeterminism(t *testing.T) {
	m, _ := smallWorld(t, 5, "cinematic", 600)
	w := newWorld(m, 0, 0)
	for i, c := range m.Cells {
		w.alt[i] = 1500*math.Sin(2*math.Pi*c.Site.X/m.Cylinder().W()) + 2*c.Site.Y - 600
		w.rel[i] = float64((i * 37) % 700)
	}
	peaks := []topo.Point{{X: 10, Y: 300}, {X: 400, Y: 200}, {X: 700, Y: 250}}
	a, _ := w.classify(t, 0, peaks, hm).AppendBinary(nil)
	b, _ := w.classify(t, 0, peaks, hm).AppendBinary(nil)
	if !bytes.Equal(a, b) {
		t.Error("two classifications differ")
	}
	if len(a) != 8+len(m.Cells)*11+8+len(peaks)*9+8 {
		t.Errorf("encoding is %d bytes", len(a))
	}
}

func TestClassifyErrors(t *testing.T) {
	m, _ := smallWorld(t, 1, "square", 600)
	w := newWorld(m, 100, 10)
	f := cells.Classify(m, w.alt, 0)
	if _, err := Classify(m, w.alt[1:], w.rel, f, nil, nil, hm); err == nil {
		t.Error("short altitudes accepted")
	}
	w.rel[3] = math.NaN()
	if _, err := Classify(m, w.alt, w.rel, f, nil, nil, hm); err == nil {
		t.Error("NaN relief accepted")
	}
	w.rel[3] = -1
	if _, err := Classify(m, w.alt, w.rel, f, nil, nil, hm); err == nil {
		t.Error("negative relief accepted")
	}
}

// TestRender checks the landform render's size, a volcano mark at its
// cell's site, and the ice rim.
func TestRender(t *testing.T) {
	m, f := smallWorld(t, 2, "square", 600)
	w := newWorld(m, 100, 500)
	land := interior(m)[0]
	res := w.classify(t, 0, []topo.Point{m.Cells[land].Site}, hm)
	img := LandformRender(f, m, res)
	base := mesh.CellRender(f, m, func(int) color.RGBA { return color.RGBA{} })
	if img.Rect != base.Rect {
		t.Fatalf("render %v, want %v", img.Rect, base.Rect)
	}
	s := float64(mesh.RenderScale(f, m))
	site := m.Cells[land].Site
	x, y := int(site.X*s/f.PitchX()), int(site.Y*s/f.PitchY())
	if got := img.RGBAAt(x, y); got != volcanoInk {
		t.Errorf("pixel at the volcano's site %v, want %v", got, volcanoInk)
	}
	rim := slices.IndexFunc(m.Cells, func(c mesh.Cell) bool { return c.Rim })
	p := m.Cells[rim].Site
	if got := img.RGBAAt(int(p.X*s/f.PitchX()), int(p.Y*s/f.PitchY())); got != mesh.IceColor {
		t.Errorf("rim cell %d's site pixel %v, want ice", rim, got)
	}
	if LandformColor(SaltWater, Deep) == LandformColor(SaltWater, Shallow) || LandformColor(Hills, DepthNone) == LandformColor(Mountains, DepthNone) {
		t.Error("palette repeats")
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
	const pkg = "github.com/mdhender/mpg/internal/classify"
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
