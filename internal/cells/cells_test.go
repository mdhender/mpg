// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package cells

import (
	"bytes"
	"image"
	"math"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"testing"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/mesh"
	"github.com/mdhender/mpg/internal/render"
	"github.com/mdhender/mpg/internal/topo"
)

// handField returns the hand-built raster: an 8 × 4 km cylinder at 1 km
// spacing (8 columns at x = 0.5 … 7.5, 4 rows at y = 0.5 … 3.5) holding
// 10·i + j at column i, row j.
func handField(t testing.TB) *field.Field {
	t.Helper()
	cyl, err := topo.New(8, 4, 0.5, 0)
	if err != nil {
		t.Fatal(err)
	}
	f, err := field.New(cyl, 1)
	if err != nil {
		t.Fatal(err)
	}
	if f.NX() != 8 || f.NY() != 4 {
		t.Fatalf("hand field is %d × %d, want 8 × 4", f.NX(), f.NY())
	}
	f.SetFunc(func(i, j int, _ topo.Point) float64 { return float64(10*i + j) })
	return f
}

// TestHandBuiltMedians checks the statistics of four hand-placed sites over
// the hand-built field against values worked out by hand.
//
// Sites A (0.2, 1), B (4.2, 1), C (0.2, 3), D (4.2, 3). The east-west
// boundaries between them are x = 2.2 and x = 6.2, and the north-south one
// is y = 2, so A takes columns 6, 7, 0, 1 of rows 0 and 1 (its cell crosses
// the seam), B columns 2 to 5 of rows 0 and 1, and C and D the same columns
// of rows 2 and 3. A's eight values sorted are 0 1 10 11 60 61 70 71: the
// nearest-rank median (rank ⌈4⌉ = 4) is 11, p5 (rank 1) 0, p95 (rank
// ⌈7.6⌉ = 8) 71, so relief 71. B's are 20 21 30 31 40 41 50 51: median 31,
// relief 51 − 20 = 31. C and D are A and B plus 2 in every value.
func TestHandBuiltMedians(t *testing.T) {
	f := handField(t)
	sites := []topo.Point{{X: 0.2, Y: 1}, {X: 4.2, Y: 1}, {X: 0.2, Y: 3}, {X: 4.2, Y: 3}}
	s, err := FromSites(f, sites)
	if err != nil {
		t.Fatal(err)
	}
	if want := []float64{11, 31, 13, 33}; !slices.Equal(s.Altitude, want) {
		t.Errorf("Altitude = %v, want %v", s.Altitude, want)
	}
	if want := []float64{71, 31, 71, 31}; !slices.Equal(s.Relief, want) {
		t.Errorf("Relief = %v, want %v", s.Relief, want)
	}
	if want := []float64{0.5, 0.5, -0.5, -0.5}; !slices.Equal(s.Latitude, want) {
		t.Errorf("Latitude = %v, want %v", s.Latitude, want)
	}
	if want := []int{8, 8, 8, 8}; !slices.Equal(s.Samples, want) || s.Empty != 0 {
		t.Errorf("Samples = %v, Empty = %d, want %v and 0", s.Samples, s.Empty, want)
	}
	wantOwner := []int{
		0, 0, 1, 1, 1, 1, 0, 0,
		0, 0, 1, 1, 1, 1, 0, 0,
		2, 2, 3, 3, 3, 3, 2, 2,
		2, 2, 3, 3, 3, 3, 2, 2,
	}
	if !slices.Equal(s.Owner, wantOwner) {
		t.Errorf("Owner = %v, want %v", s.Owner, wantOwner)
	}

	// One site takes all 32 samples; its statistics are the field's own
	// percentiles.
	one, err := FromSites(f, []topo.Point{{X: 3, Y: 2}})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := field.Percentiles(f.Values(), 5, 50, 95)
	if one.Altitude[0] != p[1] || one.Relief[0] != p[2]-p[0] || one.Samples[0] != 32 {
		t.Errorf("single site: altitude %v relief %v samples %d, want %v %v 32", one.Altitude[0], one.Relief[0], one.Samples[0], p[1], p[2]-p[0])
	}
}

// TestHandBuiltSeam checks that a cell straddling the seam takes the
// samples on both sides of it: a site at x = 7.9 (just west of the seam)
// with its rival at x = 3.9 owns columns 6, 7, 0 and 1, and a site at
// x = 0 exactly does the same with its rival at x = 4.
func TestHandBuiltSeam(t *testing.T) {
	f := handField(t)
	for _, tc := range []struct {
		name  string
		sites []topo.Point
	}{
		{"west of seam", []topo.Point{{X: 7.9, Y: 2}, {X: 3.9, Y: 2}}},
		{"on seam", []topo.Point{{X: 0, Y: 2}, {X: 4, Y: 2}}},
	} {
		s, err := FromSites(f, tc.sites)
		if err != nil {
			t.Fatal(err)
		}
		for j := range f.NY() {
			for i := range f.NX() {
				want := 1
				if i <= 1 || i >= 6 {
					want = 0
				}
				if got := s.Owner[j*f.NX()+i]; got != want {
					t.Errorf("%s: sample (%d, %d) owned by %d, want %d", tc.name, i, j, got, want)
				}
			}
		}
		if tc.name == "west of seam" {
			// Columns 6, 7, 0, 1 of every row: 60..63, 70..73, 0..3,
			// 10..13. Sorted, rank 8 of 16 is 13; p95 rank 16 is 73.
			if s.Altitude[0] != 13 || s.Relief[0] != 73 {
				t.Errorf("%s: seam cell altitude %v relief %v, want 13 and 73", tc.name, s.Altitude[0], s.Relief[0])
			}
		}
	}
}

// TestTieBreak checks that equally near sites go to the lower index: two
// sites at the same point (the second gets no sample and falls back to the
// field at its site), and samples exactly half way between two sites.
func TestTieBreak(t *testing.T) {
	f := handField(t)
	sites := []topo.Point{{X: 0, Y: 2}, {X: 4, Y: 2}, {X: 0, Y: 2}}
	s, err := FromSites(f, sites)
	if err != nil {
		t.Fatal(err)
	}
	// Site 2 ties site 0 at every sample, so it loses every one.
	if s.Samples[2] != 0 || s.Empty != 1 {
		t.Fatalf("duplicate site got %d samples (Empty %d), want 0 (1)", s.Samples[2], s.Empty)
	}
	if want := f.Sample(0, 2); s.Altitude[2] != want || s.Relief[2] != 0 || s.Latitude[2] != 0 {
		t.Errorf("empty cell: altitude %v relief %v latitude %v, want %v, 0, 0", s.Altitude[2], s.Relief[2], s.Latitude[2], want)
	}
	if s.Samples[0]+s.Samples[1] != f.Len() {
		t.Errorf("samples %v do not add up to %d", s.Samples, f.Len())
	}

	// Distinct sites at x = 1.5 and x = 7.5 are both exactly 1 km from
	// column 0 (x = 0.5), one of them across the seam; the sample goes to
	// the lower index whichever side that is.
	for _, sites := range [][]topo.Point{
		{{X: 1.5, Y: 2}, {X: 7.5, Y: 2}},
		{{X: 7.5, Y: 2}, {X: 1.5, Y: 2}},
	} {
		s, err := FromSites(f, sites)
		if err != nil {
			t.Fatal(err)
		}
		if got := s.Owner[0]; got != 0 {
			t.Errorf("sites %v: equidistant sample owned by %d, want 0", sites, got)
		}
		cyl := f.Cylinder()
		p := f.Point(0, 0)
		if BruteNearest(cyl, sites, p) != 0 || s.Owner[0] != BruteNearest(cyl, sites, p) {
			t.Errorf("sites %v: brute force disagrees", sites)
		}
	}
}

// smallWorld returns a resolved 600-land-cell world (narrow falloff so it
// fits), its mesh, and a synthetic field over it.
func smallWorld(t testing.TB, seed uint64, aspect string, land int) (config.Config, *mesh.Mesh, *field.Field) {
	t.Helper()
	c := config.Default()
	c.Seed = config.Seed(seed)
	c.World.Aspect = aspect
	c.World.LandCells = land
	if land < 1000 {
		c.Rim.FalloffCells = 4
	}
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
	w := c.World.WidthKm
	f.SetFunc(func(_, _ int, p topo.Point) float64 {
		return 2000*math.Sin(2*math.Pi*p.X/w) + 0.5*p.Y
	})
	return c, m, f
}

// TestLocatorMatchesBruteForce checks the bucket grid against comparing
// every site, for every raster sample of small meshes (all of them) and a
// stride through a default-size one.
func TestLocatorMatchesBruteForce(t *testing.T) {
	type world struct {
		seed   uint64
		aspect string
		land   int
		stride int
	}
	worlds := []world{{1, "cinematic", 600, 1}, {2, "square", 600, 1}, {3, "portrait", 600, 1}}
	if !testing.Short() {
		worlds = append(worlds, world{42, "cinematic", 0, 31})
	}
	for _, w := range worlds {
		c := config.Default()
		land := w.land
		if land == 0 {
			land = c.World.LandCells
		}
		_, m, f := smallWorld(t, w.seed, w.aspect, land)
		cyl := m.Cylinder()
		sites := make([]topo.Point, len(m.Cells))
		for i, cell := range m.Cells {
			sites[i] = cell.Site
		}
		loc, err := NewLocator(cyl, sites)
		if err != nil {
			t.Fatal(err)
		}
		checked := 0
		for k := 0; k < f.Len(); k += w.stride {
			p := f.Point(k%f.NX(), k/f.NX())
			if got, want := loc.Nearest(p), BruteNearest(cyl, sites, p); got != want {
				t.Fatalf("seed %d %s: sample %d at %v: grid %d, brute force %d", w.seed, w.aspect, k, p, got, want)
			}
			checked++
		}
		// Points on and next to the seam, and at the poles.
		for _, p := range []topo.Point{{X: 0, Y: 0}, {X: 0, Y: cyl.H()}, {X: math.Nextafter(cyl.W(), 0), Y: cyl.H() / 2}, {X: 0, Y: cyl.H() / 3}} {
			if got, want := loc.Nearest(p), BruteNearest(cyl, sites, p); got != want {
				t.Errorf("seed %d %s: point %v: grid %d, brute force %d", w.seed, w.aspect, p, got, want)
			}
		}
		t.Logf("seed %d %s: %d cells, %d samples checked", w.seed, w.aspect, len(sites), checked)
	}
}

// TestSeamCells checks, on a real mesh, that the cells whose polygons cross
// the seam collect samples from both sides of it (x < W/2 and x ≥ W/2), and
// that no sample is far from its cell's site, as it would be if distance
// ignored the wrap.
func TestSeamCells(t *testing.T) {
	for _, w := range []struct {
		seed   uint64
		aspect string
	}{{7, "cinematic"}, {11, "square"}, {13, "portrait"}} {
		c, m, f := smallWorld(t, w.seed, w.aspect, 600)
		s, err := Compute(f, m)
		if err != nil {
			t.Fatal(err)
		}
		cyl := m.Cylinder()
		wkm := cyl.W()
		east := make([]int, len(m.Cells)) // samples at x < W/2
		west := make([]int, len(m.Cells)) // samples at x ≥ W/2
		far := 0.0
		for k, owner := range s.Owner {
			p := f.Point(k%f.NX(), k/f.NX())
			if p.X < wkm/2 {
				east[owner]++
			} else {
				west[owner]++
			}
			far = max(far, cyl.Distance(p, m.Cells[owner].Site))
		}
		side := math.Sqrt(c.Province.AreaKm2)
		if far > 1.5*side {
			t.Errorf("a sample lies %.1f km from its cell's site, more than 1.5 cell sides (%.1f km)", far, 1.5*side)
		}
		seam, wide, both := 0, 0, 0
		for i := range m.Cells {
			lo, hi := math.Inf(1), math.Inf(-1)
			for _, p := range m.Polygon(i) {
				lo, hi = min(lo, p.X), max(hi, p.X)
			}
			if lo >= 0 && hi < wkm {
				continue
			}
			seam++
			// The polygon's extent on each side of the seam; one that reaches
			// a raster pitch into both sides (past the first sample column, half
			// a pitch from the seam) must hold samples on both.
			a, b := -lo, hi // site east of the seam
			if hi >= wkm {
				a, b = wkm-lo, hi-wkm // site west of the seam
			}
			if min(a, b) < f.PitchX() {
				continue
			}
			wide++
			if east[i] > 0 && west[i] > 0 {
				both++
			} else {
				t.Errorf("seam cell %d reaches %.1f and %.1f km into the two sides but has samples %d east, %d west", i, a, b, east[i], west[i])
			}
		}
		if wide == 0 {
			t.Fatalf("%d seam cells, none reaching well into both sides", seam)
		}
		t.Logf("seed %d %s: %d seam cells, %d reaching well into both sides, %d with samples on both; farthest sample %.2f km from its site", w.seed, w.aspect, seam, wide, both, far)
		if s.Empty != 0 {
			t.Errorf("%d cells have no sample", s.Empty)
		}
	}
}

// TestComputeInvariants checks counts, ranges and determinism on a real
// mesh: every sample is owned, the counts add up, altitude lies within the
// cell's own sample range, latitude matches the site, nothing is NaN, and
// two runs encode identically.
func TestComputeInvariants(t *testing.T) {
	_, m, f := smallWorld(t, 5, "square", 600)
	s, err := Compute(f, m)
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, n := range s.Samples {
		total += n
	}
	if total != f.Len() || len(s.Owner) != f.Len() || s.Len() != len(m.Cells) {
		t.Fatalf("samples %d, owners %d, cells %d; want %d, %d, %d", total, len(s.Owner), s.Len(), f.Len(), f.Len(), len(m.Cells))
	}
	values := f.Values()
	lo := make([]float64, s.Len())
	hi := make([]float64, s.Len())
	for i := range lo {
		lo[i], hi[i] = math.Inf(1), math.Inf(-1)
	}
	for k, c := range s.Owner {
		lo[c], hi[c] = min(lo[c], values[k]), max(hi[c], values[k])
	}
	cyl := m.Cylinder()
	for i := range s.Len() {
		a, r := s.Altitude[i], s.Relief[i]
		if math.IsNaN(a) || math.IsInf(a, 0) || math.IsNaN(r) || r < 0 || r > hi[i]-lo[i] {
			t.Fatalf("cell %d: altitude %v relief %v (samples %v to %v)", i, a, r, lo[i], hi[i])
		}
		if s.Samples[i] > 0 && (a < lo[i] || a > hi[i]) {
			t.Fatalf("cell %d: altitude %v outside its samples %v to %v", i, a, lo[i], hi[i])
		}
		if s.Latitude[i] != cyl.Latitude(m.Cells[i].Site.Y) {
			t.Fatalf("cell %d: latitude %v", i, s.Latitude[i])
		}
	}
	again, err := Compute(f, m)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := s.AppendBinary(nil)
	b, _ := again.AppendBinary(nil)
	if !bytes.Equal(a, b) {
		t.Error("two runs encode differently")
	}

	// A field on another cylinder is refused.
	if _, err := Compute(handField(t), m); err == nil {
		t.Error("Compute accepted a field on another cylinder")
	}
	if _, err := FromSites(f, nil); err == nil {
		t.Error("FromSites accepted no sites")
	}
	if _, err := FromSites(f, []topo.Point{{X: cyl.W(), Y: 1}}); err == nil {
		t.Error("FromSites accepted a site at x = W")
	}
}

// TestRenders checks the cell renders: the mesh render's size, the same
// pixels twice, rim cells' sites drawn as ice, and a playable cell's site
// drawn in its altitude's color.
func TestRenders(t *testing.T) {
	_, m, f := smallWorld(t, 3, "cinematic", 600)
	s, err := Compute(f, m)
	if err != nil {
		t.Fatal(err)
	}
	scale := mesh.RenderScale(f, m)
	size := image.Rect(0, 0, f.NX()*scale, f.NY()*scale)
	alt := AltitudeRender(f, m, s, 0)
	rel := ReliefRender(f, m, s)
	if alt.Rect != size || rel.Rect != size {
		t.Fatalf("render sizes %v and %v, want %v", alt.Rect, rel.Rect, size)
	}
	if render.PixelHash(alt) != render.PixelHash(AltitudeRender(f, m, s, 0)) {
		t.Error("two altitude renders differ")
	}
	sx := float64(scale) / f.PitchX()
	sy := float64(scale) / f.PitchY()
	rimSeen, landSeen := false, false
	for i, cell := range m.Cells {
		x, y := int(cell.Site.X*sx), int(cell.Site.Y*sy)
		got := alt.RGBAAt(x, y)
		switch {
		case cell.Rim:
			rimSeen = true
			if got != mesh.IceColor {
				// The site pixel can sit under a blended outline only if
				// the site is within a pixel of an edge; rare, so allow it
				// by checking a neighbor too.
				if alt.RGBAAt(x+1, y) != mesh.IceColor && alt.RGBAAt(x, y+1) != mesh.IceColor {
					t.Errorf("rim cell %d's site pixel %v, want ice %v", i, got, mesh.IceColor)
				}
			}
		case !landSeen && got == AltitudeColor(s.Altitude[i], 0):
			landSeen = true
		}
	}
	if !rimSeen || !landSeen {
		t.Errorf("rim seen %v, a playable cell in its altitude color seen %v", rimSeen, landSeen)
	}
	if AltitudeColor(10, 0) != render.Land.At(10) || AltitudeColor(0, 0) != render.Water.At(0) || AltitudeColor(-50, 0) != render.Water.At(50) {
		t.Error("AltitudeColor does not split at sea level like render.Hypsometric")
	}
}

// TestNoFusedMultiplyAdd compiles this package for the architectures whose Go
// backends contract a*b + c and checks the code holds no fused instruction.
func TestNoFusedMultiplyAdd(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles for several architectures")
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool not found")
	}
	fusedOp := regexp.MustCompile(`\tV?FN?M(ADD|SUB)`)
	const pkg = "github.com/mdhender/mpg/internal/cells"
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
