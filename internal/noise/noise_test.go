// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package noise

import (
	"math"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/internal/render"
	"github.com/mdhender/mpg/internal/topo"
)

// The test world is 512 × 256 km at 2 km spacing: 256 × 128 samples. Every
// size is a power of two, so x + k·pitch and x ± W are exact and shifting by
// whole columns can be checked bit for bit.
func testWorld(t *testing.T) (Cylinder, *field.Field) {
	t.Helper()
	c, err := topo.New(512, 256, 2, 6)
	if err != nil {
		t.Fatalf("topo.New: %v", err)
	}
	f, err := field.New(c, 2)
	if err != nil {
		t.Fatalf("field.New: %v", err)
	}
	if f.NX() != 256 || f.NY() != 128 || f.PitchX() != 2 {
		t.Fatalf("field is %d × %d at pitch %v, want 256 × 128 at 2", f.NX(), f.NY(), f.PitchX())
	}
	return NewCylinder(c), f
}

// Ladders scaled to the 512 km test world.
var (
	testFBM    = FBM{Octaves: 6, Lacunarity: 2, Gain: 0.5, WavelengthKm: 128}
	testRidged = Ridged{Octaves: 5, Lacunarity: 2, Gain: 0.5, WavelengthKm: 96, Weight: 2}
	testWarp   = Warp{StrengthKm: 24, FBM: FBM{Octaves: 3, Lacunarity: 2, Gain: 0.5, WavelengthKm: 128}}
)

// composite is the seam-proof field: warp the point, then mix fBm with ridges
// rescaled to [−1, 1]. Each part draws on its own stream.
func composite(s Source, m Cylinder, x, y float64) float64 {
	wx, wy := testWarp.Apply(s.Stream(0), m, x, y)
	q := m.Point(wx, wy)
	a := testFBM.Sample(s.Stream(1), q)
	r := fmath.MulAdd(2, testRidged.Sample(s.Stream(2), q), -1)
	return fmath.MulAdd(0.65, a, fmath.Mul(0.35, r))
}

// compositeField fills a field with composite sampled shift columns east:
// sample (i, j) holds the composite at X(i) + shift·PitchX.
func compositeField(t *testing.T, s Source, shift int) *field.Field {
	t.Helper()
	m, f := testWorld(t)
	dx := float64(shift) * f.PitchX()
	Fill(f, func(x, y float64) float64 { return composite(s, m, x+dx, y) })
	return f
}

var testSource = New(42, "noise-test")

func same(a, b float64) bool { return math.Float64bits(a) == math.Float64bits(b) }

// TestSeamContinuity is the milestone 1 proof, numerically: the step across
// the seam (column NX−1 to column 0) is statistically like the step between
// any two adjacent interior columns.
func TestSeamContinuity(t *testing.T) {
	f := compositeField(t, testSource, 0)
	nx, ny := f.NX(), f.NY()

	pairStats := func(i int) (maxD, meanD float64) {
		for j := range ny {
			d := math.Abs(f.At(i+1, j) - f.At(i, j)) // At wraps i+1 = NX to 0
			maxD = max(maxD, d)
			meanD += d
		}
		return maxD, meanD / float64(ny)
	}
	seamMax, seamMean := pairStats(nx - 1)
	var maxes, means []float64
	for i := range nx - 1 {
		mx, mn := pairStats(i)
		maxes = append(maxes, mx)
		means = append(means, mn)
	}
	slices.Sort(maxes)
	slices.Sort(means)
	var sum float64
	for _, v := range means {
		sum += v
	}
	interiorMean := sum / float64(len(means))
	rank := func(sorted []float64, v float64) float64 {
		k, _ := slices.BinarySearch(sorted, v)
		return float64(k) / float64(len(sorted))
	}
	t.Logf("seam: max |Δ| %.5f, mean |Δ| %.5f", seamMax, seamMean)
	t.Logf("interior column pairs: mean |Δ| %.5f (min %.5f, median %.5f, max %.5f); max |Δ| median %.5f, largest %.5f",
		interiorMean, means[0], means[len(means)/2], means[len(means)-1], maxes[len(maxes)/2], maxes[len(maxes)-1])
	t.Logf("seam ranks among %d interior pairs: mean at %.0f%%, max at %.0f%%; seam/interior mean ratio %.3f",
		len(means), 100*rank(means, seamMean), 100*rank(maxes, seamMax), seamMean/interiorMean)

	if seamMean > means[len(means)-1] || seamMean < means[0] {
		t.Errorf("seam mean |Δ| %v outside the interior range [%v, %v]", seamMean, means[0], means[len(means)-1])
	}
	if seamMax > maxes[len(maxes)-1] {
		t.Errorf("seam max |Δ| %v exceeds every interior pair's max (largest %v)", seamMax, maxes[len(maxes)-1])
	}
	if r := seamMean / interiorMean; r > 2 || r < 0.5 {
		t.Errorf("seam/interior mean |Δ| ratio %v, want within [0.5, 2]", r)
	}
}

// TestPeriodicBits checks that x, x + W, and x − W give identical bits in
// every function of the package.
func TestPeriodicBits(t *testing.T) {
	m, f := testWorld(t)
	w := m.Topo().W()
	s := testSource
	for _, i := range []int{0, 1, 63, 127, 128, 200, 254, 255} {
		for _, j := range []int{0, 5, 40, 64, 100, 127} {
			x, y := f.X(i), f.Y(j)
			for _, xx := range []float64{x + w, x - w, x + 3*w} {
				p, q := m.Point(x, y), m.Point(xx, y)
				if p != q {
					t.Fatalf("Point(%v) = %v, Point(%v) = %v", x, p, xx, q)
				}
				if a, b := testFBM.Sample(s, p), testFBM.Sample(s, q); !same(a, b) {
					t.Fatalf("fBm at x %v = %v, at %v = %v", x, a, xx, b)
				}
				ax, ay := testWarp.Apply(s, m, x, y)
				bx, by := testWarp.Apply(s, m, xx, y)
				if !same(ax, bx) || !same(ay, by) {
					t.Fatalf("Warp at x %v = (%v, %v), at %v = (%v, %v)", x, ax, ay, xx, bx, by)
				}
				if a, b := composite(s, m, x, y), composite(s, m, xx, y); !same(a, b) {
					t.Fatalf("composite at x %v = %v, at %v = %v", x, a, xx, b)
				}
			}
		}
	}
}

// TestShiftEqualsRoll checks that a field sampled k columns east is the
// original field rolled k columns west, bit for bit, including shifts that
// carry columns across the seam.
func TestShiftEqualsRoll(t *testing.T) {
	f := compositeField(t, testSource, 0)
	for _, k := range []int{1, 37, 128, 255, -3} {
		g := compositeField(t, testSource, k)
		for j := range f.NY() {
			for i := range f.NX() {
				if a, b := g.At(i, j), f.At(i+k, j); !same(a, b) {
					t.Fatalf("shift %d: (%d, %d) = %v, original (%d, %d) = %v", k, i, j, a, i+k, j, b)
				}
			}
		}
	}
}

// goldenComposite is the PixelHash of the composite field rendered on the
// Gray ramp. It must match on amd64 and arm64; change it only with Version.
const goldenComposite = "2031ebba6c6cc056d0ecfec2350daa7a48d69132c00b633c2c66609d82604380"

// TestSeamRender pins the composite render and, when MPG_RENDER_DIR is set,
// writes it and the field rolled by half the width (the seam in the middle of
// the image) for visual inspection.
func TestSeamRender(t *testing.T) {
	f := compositeField(t, testSource, 0)
	img := render.Sequential(f, render.Gray)
	if got := render.PixelHash(img); got != goldenComposite {
		t.Errorf("PixelHash = %s, want %s", got, goldenComposite)
	}
	rolled := compositeField(t, testSource, f.NX()/2)
	if dir := os.Getenv("MPG_RENDER_DIR"); dir != "" {
		for _, out := range []struct {
			name, variant string
			field         *field.Field
		}{
			{"noise", "seam-original", f},
			{"noise", "seam-rolled", rolled},
		} {
			for _, rr := range []struct {
				ramp   render.Ramp
				suffix string
			}{{render.Gray, ""}, {render.Viridis, "-viridis"}} {
				name := out.name + "-" + out.variant + rr.suffix
				path := filepath.Join(dir, name+".png")
				meta := render.Meta{Stage: name, ConfigHash: "test"}
				if err := render.WritePNGFile(path, render.Sequential(out.field, rr.ramp), meta); err != nil {
					t.Fatal(err)
				}
				t.Logf("wrote %s", path)
			}
		}
	}
}

func TestNoise3Bits(t *testing.T) {
	s := NewSource(0x0123456789abcdef, 0xfedcba9876543210)
	cases := []struct {
		x, y, z float64
		want    uint64
	}{
		{0, 0, 0, 0x0000000000000000},                       // 0
		{0.5, 0.25, 0.125, 0x3fb0644e40ca4587},              // 0.06403054315366861
		{1.7, -2.3, 3.9, 0xbfed09ace8edabae},                // -0.9074310826666669
		{-10.25, 4.5, 0.75, 0xbfead49e06522c43},             // -0.8384542582947535
		{81.487, 0, 64, 0xbfbcae39af025cb8},                 // -0.11203346750913823
		{123.456, 789.012, -345.678, 0x3fe3c9c1375d166b},    // 0.6183782655068176
		{1.0000003e+06, -999999.3, 0.1, 0xbf80d40fc9ffa298}, // -0.008216975547797475
		{0.3333, 0.3333, 0.3334, 0xbf2e06520c02124e},        // -0.00022907020138886392
	}
	for _, c := range cases {
		if got := s.Noise3(c.x, c.y, c.z); math.Float64bits(got) != c.want {
			t.Errorf("Noise3(%v, %v, %v) = %v (%#016x), want %v (%#016x)", c.x, c.y, c.z, got, math.Float64bits(got), math.Float64frombits(c.want), c.want)
		}
	}
}

// fractalPoints are the cylinder points the fractal pins are taken at.
func fractalPoints(t *testing.T) (Cylinder, [][2]float64) {
	m, _ := testWorld(t)
	return m, [][2]float64{{0, 0}, {1, 128}, {255.5, 64.25}, {511, 255}, {300.125, 3.5}, {77, 190}}
}

func TestFractalBits(t *testing.T) {
	m, pts := fractalPoints(t)
	s := testSource
	want := []struct{ fbm, ridged, wx, wy, comp uint64 }{
		{0x3fd1695976166e63, 0x3fdc4284edd9d261, 0x407f8c62d4bd7d75, 0x3fe9ecda3035a78c, 0xbfc6ecbe73cc0668}, // 0.27205502063745807 0.44156001308082266 504.7741286661106 0.8101626340478432 -0.17909985212468205
		{0xbf930cd2a721a456, 0x3fcc9f57e1458a18, 0x4027ca4dc50ebc11, 0x40610867c98093f0, 0x3fcc6f519ec00cdb}, // -0.01860360283936687 0.22361277103034705 11.895124586151754 136.26266932595126 0.22214718104814932
		{0xbfb5e784cabe4222, 0x3fd2b0f740fe7e61, 0x406f36c4c544d01a, 0x404c447cb083b1ce, 0xbf9a1d4a4ae220a4}, // -0.08556394529137681 0.29205113741952543 249.71151984634417 56.535055221847514 -0.02550235826541382
		{0xbfdf0a2d808b9a80, 0x3fe412900261c665, 0x400ca9156164d500, 0x406f2b44b4934960, 0xbfdec1942bb213a8}, // -0.48499620011572375 0.6272659346124781 3.5825603112122053 249.3521368862812 -0.4805651118006673
		{0xbfc256d0990d12b4, 0x3fdcec52c7d2ec72, 0x4072fbf7af923147, 0x401e698e5cd36125, 0x3fd07e5e538b1c5e}, // -0.14327437852093328 0.4519240332990143 303.74797017198983 7.603082132722453 0.2577129188397914
		{0xbfbf20c1b48f9994, 0x3fde304ca48c6aca, 0x405517418398ea8e, 0x4068e225d5394bb7, 0xbfcea404c4b667ae}, // -0.12159357698656087 0.47169796055974855 84.36337366040445 199.06711827459887 -0.23938045125106328
	}
	if len(want) != len(pts) {
		t.Fatalf("have %d pins for %d points", len(want), len(pts))
	}
	for k, p := range pts {
		q := m.Point(p[0], p[1])
		wx, wy := testWarp.Apply(s, m, p[0], p[1])
		got := []float64{testFBM.Sample(s, q), testRidged.Sample(s, q), wx, wy, composite(s, m, p[0], p[1])}
		pins := []uint64{want[k].fbm, want[k].ridged, want[k].wx, want[k].wy, want[k].comp}
		for n, name := range []string{"fbm", "ridged", "warp x", "warp y", "composite"} {
			if math.Float64bits(got[n]) != pins[n] {
				t.Errorf("%s at %v = %v (%#016x), want %v (%#016x)", name, p, got[n], math.Float64bits(got[n]), math.Float64frombits(pins[n]), pins[n])
			}
		}
	}
}

// TestNoise3Range searches for the extreme of the raw corner sum by hill
// climbing from random starts, and checks that the scaled extreme stays
// within [−1, 1] and still nearly fills it.
func TestNoise3Range(t *testing.T) {
	if testing.Short() {
		t.Skip("searches for the extreme")
	}
	r := rand.New(rand.NewPCG(3, 4))
	dirs := [][3]float64{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}}
	best := 0.0
	for key := range 20 {
		s := NewSource(uint64(key), 99)
		for range 1000 {
			x, y, z := r.Float64()*64-32, r.Float64()*64-32, r.Float64()*64-32
			v := math.Abs(s.raw3(x, y, z))
			for step := 0.05; step > 1e-6; {
				moved := false
				for _, d := range dirs {
					nx, ny, nz := x+d[0]*step, y+d[1]*step, z+d[2]*step
					if w := math.Abs(s.raw3(nx, ny, nz)); w > v {
						v, x, y, z, moved = w, nx, ny, nz, true
					}
				}
				if !moved {
					step /= 2
				}
			}
			best = max(best, v)
		}
	}
	scaled := best * simplexScale
	t.Logf("largest |raw| found %v; scaled %v", best, scaled)
	if scaled > 1 {
		t.Errorf("scaled extreme %v exceeds 1; simplexScale %v is too large", scaled, simplexScale)
	}
	if scaled < 0.95 {
		t.Errorf("scaled extreme %v below 0.95; simplexScale %v wastes range", scaled, simplexScale)
	}
}

// TestStatistics checks range, mean, and spread of the noise and fractals
// over many samples, and that nothing is NaN.
func TestStatistics(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	s := testSource
	m, _ := testWorld(t)
	type acc struct{ lo, hi, sum, sq float64 }
	var n3, fb, rg acc
	add := func(a *acc, v float64) {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("non-finite value %v", v)
		}
		a.lo, a.hi = min(a.lo, v), max(a.hi, v)
		a.sum += v
		a.sq += v * v
	}
	// One world holds only a few fBm wavelengths, so its mean is not 0; the
	// fractals are averaged over a fresh stream per sample instead.
	const count = 200_000
	for n := range count {
		add(&n3, s.Noise3(r.Float64()*1000-500, r.Float64()*1000-500, r.Float64()*1000-500))
		q := m.Point(r.Float64()*512, r.Float64()*256)
		add(&fb, testFBM.Sample(s.Stream(uint64(n)), q))
		add(&rg, testRidged.Sample(s.Stream(uint64(n)), q))
	}
	report := func(name string, a acc) (mean, sd float64) {
		mean = a.sum / count
		sd = math.Sqrt(a.sq/count - mean*mean)
		t.Logf("%-7s range [%.4f, %.4f], mean %.4f, sd %.4f", name, a.lo, a.hi, mean, sd)
		return mean, sd
	}
	if mean, sd := report("Noise3", n3); math.Abs(mean) > 0.01 || sd < 0.15 || n3.lo < -1 || n3.hi > 1 || n3.hi < 0.75 || n3.lo > -0.75 {
		t.Errorf("Noise3 statistics out of bounds")
	}
	if mean, sd := report("fBm", fb); math.Abs(mean) > 0.01 || sd < 0.1 || fb.lo < -1 || fb.hi > 1 {
		t.Errorf("fBm statistics out of bounds")
	}
	if _, sd := report("ridged", rg); sd < 0.05 || rg.lo < 0 || rg.hi > 1 {
		t.Errorf("ridged statistics out of bounds")
	}
}

func TestLatticeZerosAndDomain(t *testing.T) {
	s := testSource
	for _, p := range [][3]float64{{0, 0, 0}, {1, 2, 3}, {-5, 7, -11}} {
		// Lattice points of the skewed grid: (i,j,k) − (i+j+k)·G3.
		u := (p[0] + p[1] + p[2]) / 6
		if v := s.Noise3(p[0]-u, p[1]-u, p[2]-u); math.Abs(v) > 1e-12 {
			t.Errorf("Noise3 at lattice point %v = %v, want 0", p, v)
		}
	}
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), 0x1p51, -0x1p60} {
		if got := s.Noise3(v, 1, 2); got != 0 {
			t.Errorf("Noise3(%v, 1, 2) = %v, want 0", v, got)
		}
	}
}

func TestSeedSensitivity(t *testing.T) {
	a := compositeField(t, testSource, 0)
	for name, s := range map[string]Source{
		"world seed": New(43, "noise-test"),
		"stage":      New(42, "noise-test2"),
		"stream":     testSource.Stream(7),
	} {
		bv := compositeField(t, s, 0).Values()
		same := 0
		for k, v := range a.Values() {
			if v == bv[k] {
				same++
			}
		}
		if same > a.Len()/100 {
			t.Errorf("%s: %d of %d samples unchanged", name, same, a.Len())
		}
	}
	if New(42, "noise-test") != testSource {
		t.Error("New is not deterministic")
	}
}

func TestCylinderPoint(t *testing.T) {
	m, f := testWorld(t)
	w, h := m.Topo().W(), m.Topo().H()
	r := w / (2 * math.Pi)
	for _, y := range []float64{0, 1, 128, 255, -10, 300} {
		p := m.Point(0, y)
		if p.X != r || p.Y != 0 || p.Z != h/2-y {
			t.Errorf("Point(0, %v) = %v, want (%v, 0, %v)", y, p, r, h/2-y)
		}
		if lat := m.Topo().Latitude(y) * h / 2; math.Abs(lat-p.Z) > 1e-9 {
			t.Errorf("Point(0, %v).Z = %v, latitude·H/2 = %v", y, p.Z, lat)
		}
	}
	// Surface distance between adjacent columns is the pitch, across the
	// seam too: chord = 2R·sin(π/NX).
	chord := 2 * r * math.Sin(math.Pi/float64(f.NX()))
	for _, i := range []int{0, 100, 255} {
		a, b := m.Point(f.X(i), 0), m.Point(f.X(i+1), 0)
		if d := math.Hypot(a.X-b.X, a.Y-b.Y); math.Abs(d-chord) > 1e-9 {
			t.Errorf("chord %d→%d = %v, want %v", i, i+1, d, chord)
		}
	}
	if math.Abs(chord-f.PitchX()) > 1e-3 {
		t.Errorf("chord %v not close to pitch %v", chord, f.PitchX())
	}
}

func TestValidate(t *testing.T) {
	bad := []error{
		FBM{Octaves: 0, Lacunarity: 2, Gain: 0.5, WavelengthKm: 1}.Validate(),
		FBM{Octaves: 17, Lacunarity: 2, Gain: 0.5, WavelengthKm: 1}.Validate(),
		FBM{Octaves: 1, Lacunarity: 0, Gain: 0.5, WavelengthKm: 1}.Validate(),
		FBM{Octaves: 1, Lacunarity: 2, Gain: 17, WavelengthKm: 1}.Validate(),
		FBM{Octaves: 1, Lacunarity: 2, Gain: 0.5, WavelengthKm: math.NaN()}.Validate(),
		Ridged{Octaves: 1, Lacunarity: 2, Gain: 0.5, WavelengthKm: 1, Weight: -1}.Validate(),
		Warp{StrengthKm: math.Inf(1), FBM: DefaultFBM()}.Validate(),
		Warp{StrengthKm: 1}.Validate(),
	}
	for k, err := range bad {
		if err == nil {
			t.Errorf("case %d: invalid parameters accepted", k)
		}
	}
	for _, err := range []error{DefaultFBM().Validate(), DefaultRidged().Validate(), DefaultWarp().Validate(),
		testFBM.Validate(), testRidged.Validate(), testWarp.Validate()} {
		if err != nil {
			t.Error(err)
		}
	}
	defer func() {
		if recover() == nil {
			t.Error("Sample with invalid FBM did not panic")
		}
	}()
	FBM{}.Sample(testSource, Point3{})
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
	const pkg = "github.com/mdhender/mpg/internal/noise"
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
