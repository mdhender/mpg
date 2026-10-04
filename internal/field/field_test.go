// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package field

import (
	"math"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"testing"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/topo"
)

// same reports whether a and b have identical bits.
func same(a, b float64) bool { return math.Float64bits(a) == math.Float64bits(b) }

func mustField(t *testing.T, w, h, spacing float64) *Field {
	t.Helper()
	c, err := topo.New(w, h, 2, 6)
	if err != nil {
		t.Fatalf("topo.New: %v", err)
	}
	f, err := New(c, spacing)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return f
}

// example returns a field over the design example, N = 10,000 and f = 0.30
// at cinematic aspect.
func example(t *testing.T) *Field {
	t.Helper()
	cfg := config.Default()
	cfg.Seed = 42
	cfg.World.LandCells = 10_000
	cfg.World.LandFraction = 0.30
	cfg.World.Aspect = "cinematic"
	if err := cfg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	f, err := FromConfig(cfg)
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}
	return f
}

func TestNewValidation(t *testing.T) {
	c, err := topo.New(100, 50, 2, 6)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []float64{0, -1, math.Inf(1), math.NaN(), 1e-6} {
		if _, err := New(c, s); err == nil {
			t.Errorf("New(spacing %v) returned no error", s)
		}
	}
	if _, err := New(topo.Cylinder{}, 2); err == nil {
		t.Errorf("New(zero cylinder) returned no error")
	}
	// A spacing wider than the world still gives two samples per axis.
	f, err := New(c, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if f.NX() != 2 || f.NY() != 2 || f.PitchX() != 50 || f.PitchY() != 25 {
		t.Errorf("coarse field = %d × %d, pitch %v × %v", f.NX(), f.NY(), f.PitchX(), f.PitchY())
	}
}

func TestDesignExampleDimensions(t *testing.T) {
	f := example(t)
	w, h := f.Cylinder().W(), f.Cylinder().H()
	// W = 2,536.31 km and H = 1,133.10 km at 2 km: 1268.15 and 566.55 round
	// to 1268 and 567.
	if math.Abs(w-2536.3057312095275) > 1e-9 || math.Abs(h-1133.103627742564) > 1e-9 {
		t.Fatalf("world = %v × %v km, want the design example", w, h)
	}
	if f.NX() != 1268 || f.NY() != 567 {
		t.Errorf("example field = %d × %d, want 1268 × 567", f.NX(), f.NY())
	}
	if f.Len() != 1268*567 || len(f.Values()) != f.Len() {
		t.Errorf("Len = %d, want %d", f.Len(), 1268*567)
	}
	if px, py := f.PitchX(), f.PitchY(); math.Abs(px-2) > 0.001 || math.Abs(py-2) > 0.002 {
		t.Errorf("pitch = %v × %v, want about 2 km", px, py)
	}
	// The raster covers the full height, rims included.
	if f.Y(0) <= 0 || f.Y(0) >= f.Cylinder().Rim() || f.Y(f.NY()-1) >= h || f.Y(f.NY()-1) <= h-f.Cylinder().Rim() {
		t.Errorf("first/last rows at %v, %v km do not lie in the rims of a %v km world", f.Y(0), f.Y(f.NY()-1), h)
	}
}

func TestNoDuplicateSeamColumn(t *testing.T) {
	for _, f := range []*Field{example(t), mustField(t, 100, 50, 10), mustField(t, 100, 50, 3), mustField(t, 99.7, 41.3, 2.2)} {
		w := f.Cylinder().W()
		nx := f.NX()
		if f.Len() != nx*f.NY() {
			t.Errorf("Len = %d, want %d × %d", f.Len(), nx, f.NY())
		}
		// The columns tile the circumference exactly once.
		if d := math.Abs(float64(nx)*f.PitchX() - w); d > ulp(w) {
			t.Errorf("NX·pitch = %v, W = %v: off by %v, more than one ulp", float64(nx)*f.PitchX(), w, d)
		}
		first, last := f.X(0), f.X(nx-1)
		if !(first > 0 && last < w && first != last) {
			t.Errorf("columns 0 and %d at %v and %v: not distinct positions inside [0, %v)", nx-1, first, last, w)
		}
		// The gap across the seam is one pitch, like every other gap.
		if gap := first + w - last; math.Abs(gap-f.PitchX()) > 1e-9*w {
			t.Errorf("seam gap = %v, want pitch %v", gap, f.PitchX())
		}
		// Column NX−1's east neighbor is column 0.
		f.Set(0, 3, 7)
		if f.At(nx, 3) != 7 || f.At(-nx, 3) != 7 {
			t.Errorf("At(NX, 3) = %v, want column 0's 7", f.At(nx, 3))
		}
		f.Set(-1, 3, 9)
		if f.At(nx-1, 3) != 9 {
			t.Errorf("Set(-1, 3) did not store into column NX−1")
		}
	}
}

func ulp(x float64) float64 { return math.Nextafter(x, math.Inf(1)) - x }

// columnField returns a 100 × 50 km field at 10 km whose value is 10·column +
// row, so it jumps from 90+ at column 9 back to 0+ at column 0.
func columnField(t *testing.T) *Field {
	f := mustField(t, 100, 50, 10)
	f.SetFunc(func(i, j int, _ topo.Point) float64 { return float64(10*i + j) })
	return f
}

func TestSampleAcrossSeam(t *testing.T) {
	f := columnField(t)
	// Column 9 is at x = 95 and column 0 at x = 105 ≡ 5. Between them the
	// field runs linearly from 90 (row 0) down to 0.
	cases := []struct{ x, want float64 }{
		{95, 90},
		{97.5, 67.5},
		{99.75, 47.25},
		{100, 45},
		{0, 45},
		{0.25, 42.75},
		{2.5, 22.5},
		{5, 0},
		{-0.25, 47.25},
		{-5, 90},
	}
	for _, tc := range cases {
		if got := f.Sample(tc.x, 5); got != tc.want {
			t.Errorf("Sample(%v, 5) = %v, want %v", tc.x, got, tc.want)
		}
	}
	// Continuous across the seam: just west and just east of x = 0 agree to
	// within the slope times the gap.
	west, east := f.Sample(math.Nextafter(100, 0), 5), f.Sample(0, 5)
	if math.Abs(west-east) > 1e-9 {
		t.Errorf("Sample just west of the seam %v, at the seam %v", west, east)
	}
	// Periodic bit for bit, at points where x ± W is exact.
	for _, f := range []*Field{f, example(t)} {
		w := f.Cylinder().W()
		f.SetFunc(func(i, j int, p topo.Point) float64 { return math.Sin(p.X/7) + float64(i%5) + 0.1*float64(j) })
		for _, x := range []float64{0, 0.25, 0.5, 1, 3.375, 17.5, 50.125, 99.75} {
			for _, y := range []float64{0, 3.5, 27.25, 49} {
				s := f.Sample(x, y)
				if a, b := f.Sample(x+w, y), f.Sample(x-w, y); !same(s, a) || !same(s, b) {
					t.Errorf("W=%v: Sample(%v, %v) = %v, at x+W %v, at x−W %v", w, x, y, s, a, b)
				}
			}
		}
		// Just west of W interpolates column NX−1 ↔ 0 and meets x = 0.
		y := f.Y(4)
		a, b := f.At(f.NX()-1, 4), f.At(0, 4)
		lo, hi := min(a, b), max(a, b)
		for _, x := range []float64{f.X(f.NX() - 1), math.Nextafter(w, 0), 0, f.X(0) / 2} {
			if s := f.Sample(x, y); s < lo || s > hi {
				t.Errorf("W=%v: Sample(%v) = %v, outside columns NX−1..0 [%v, %v]", w, x, s, lo, hi)
			}
		}
		if d := math.Abs(f.Sample(math.Nextafter(w, 0), y) - f.Sample(0, y)); d > 1e-9 {
			t.Errorf("W=%v: seam discontinuity %v", w, d)
		}
	}
}

func TestSampleAtGridPoints(t *testing.T) {
	for _, f := range []*Field{columnField(t), example(t), mustField(t, 99.7, 41.3, 2.2)} {
		f.SetFunc(func(i, j int, p topo.Point) float64 { return math.Cos(p.X) * float64(1+j) })
		for j := range f.NY() {
			for i := range f.NX() {
				p := f.Point(i, j)
				if got := f.Sample(p.X, p.Y); !same(got, f.At(i, j)) {
					t.Fatalf("Sample at grid point (%d, %d) = %v, stored %v", i, j, got, f.At(i, j))
				}
			}
		}
	}
}

func TestSamplePolesClamp(t *testing.T) {
	f := columnField(t)
	// Rows sit at y = 5, 15, ..., 45.
	for _, y := range []float64{-100, 0, 2.5, 5} {
		if got := f.Sample(35, y); got != 30 {
			t.Errorf("Sample(35, %v) = %v, want row 0's 30", y, got)
		}
	}
	for _, y := range []float64{45, 47.5, 50, 1e6} {
		if got := f.Sample(35, y); got != 34 {
			t.Errorf("Sample(35, %v) = %v, want row 4's 34", y, got)
		}
	}
	if got := f.Sample(35, 10); got != 30.5 {
		t.Errorf("Sample(35, 10) = %v, want 30.5", got)
	}
}

func TestSamplePinnedBits(t *testing.T) {
	f := mustField(t, 100, 50, 3) // 33 × 17, pitch 100/33 × 50/17
	f.SetFunc(func(i, j int, _ topo.Point) float64 { return float64((i*7919+j*104729)%1000) / 7 })
	cases := []struct {
		x, y float64
		bits uint64
	}{
		{12.345, 6.789, 0x403f7f41da1b2c8f},
		{99.9, 25.5, 0x4054092def07e64a},
		{0.1, 48.3, 0x404d30e560418946},
		{-73.21, 1.7, 0x4045a96c3162d4a1},
	}
	for _, tc := range cases {
		got := f.Sample(tc.x, tc.y)
		if math.Float64bits(got) != tc.bits {
			t.Errorf("Sample(%v, %v) = %v (%#016x), want %v (%#016x)", tc.x, tc.y, got, math.Float64bits(got), math.Float64frombits(tc.bits), tc.bits)
		}
	}
}

func TestSamplePanicsOnNonFinite(t *testing.T) {
	f := columnField(t)
	for _, xy := range [][2]float64{{math.NaN(), 1}, {1, math.NaN()}, {math.Inf(1), 1}, {1, math.Inf(-1)}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Sample(%v, %v) did not panic", xy[0], xy[1])
				}
			}()
			f.Sample(xy[0], xy[1])
		}()
	}
}

func TestValuesStayFinite(t *testing.T) {
	f := columnField(t)
	mustPanic := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s did not panic", name)
			}
		}()
		fn()
	}
	mustPanic("Set NaN", func() { f.Set(0, 0, math.NaN()) })
	mustPanic("Set Inf", func() { f.Set(0, 0, math.Inf(1)) })
	mustPanic("Fill NaN", func() { f.Fill(math.NaN()) })
	mustPanic("SetFunc Inf", func() { f.SetFunc(func(int, int, topo.Point) float64 { return math.Inf(-1) }) })
	mustPanic("At row -1", func() { f.At(0, -1) })
	mustPanic("At row NY", func() { f.At(0, f.NY()) })
	mustPanic("Set row NY", func() { f.Set(0, f.NY(), 1) })
}

func TestStorageOrderAndHelpers(t *testing.T) {
	f := columnField(t)
	var got []float64
	f.Each(func(i, j int, v float64) {
		if v != float64(10*i+j) {
			t.Fatalf("Each(%d, %d) = %v", i, j, v)
		}
		got = append(got, v)
	})
	if !slices.Equal(got, f.Values()) {
		t.Errorf("Each order differs from Values")
	}
	if got[1] != 10 || got[f.NX()] != 1 {
		t.Errorf("storage is not row-major: %v", got[:f.NX()+1])
	}
	if lo, hi := f.MinMax(); lo != 0 || hi != 94 {
		t.Errorf("MinMax = %v, %v, want 0, 94", lo, hi)
	}
	if p, err := f.Percentile(50); err != nil || p != 44 {
		t.Errorf("Percentile(50) = %v, %v, want 44", p, err)
	}
	g := f.Clone()
	g.Fill(-3)
	if f.At(0, 0) != 0 || g.At(5, 2) != -3 || g.Max() != -3 {
		t.Errorf("Clone shares storage")
	}
	v := f.Values()
	v[0] = 99
	if f.At(0, 0) != 0 {
		t.Errorf("Values shares storage")
	}
	if p := f.Point(-1, 2); p.X != 95 || p.Y != 25 {
		t.Errorf("Point(-1, 2) = %v, want {95 25}", p)
	}
}

func TestPercentile(t *testing.T) {
	cases := []struct {
		name   string
		values []float64
		p      float64
		want   float64
	}{
		{"single p0", []float64{4}, 0, 4},
		{"single p50", []float64{4}, 50, 4},
		{"single p100", []float64{4}, 100, 4},
		{"odd p0", []float64{5, 1, 3}, 0, 1},
		{"odd median", []float64{5, 1, 3}, 50, 3},
		{"odd p100", []float64{5, 1, 3}, 100, 5},
		{"even median is lower middle", []float64{4, 1, 3, 2}, 50, 2},
		{"even p51", []float64{4, 1, 3, 2}, 51, 3},
		{"even p100", []float64{4, 1, 3, 2}, 100, 4},
		{"even p0", []float64{4, 1, 3, 2}, 0, 1},
		{"duplicates", []float64{2, 7, 2, 2, 9}, 50, 2},
		{"duplicates p80", []float64{2, 7, 2, 2, 9}, 80, 7},
		{"negatives", []float64{-1, -5, 0}, 50, -1},
		{"twenty p5", seq(20), 5, 1},
		{"twenty p95", seq(20), 95, 19},
		{"twenty p50", seq(20), 50, 10},
		{"twenty p96", seq(20), 96, 20},
	}
	for _, tc := range cases {
		in := slices.Clone(tc.values)
		got, err := Percentile(in, tc.p)
		if err != nil || got != tc.want {
			t.Errorf("%s: Percentile(%v, %v) = %v, %v, want %v", tc.name, tc.values, tc.p, got, err, tc.want)
		}
		if !slices.Equal(in, tc.values) {
			t.Errorf("%s: input reordered to %v", tc.name, in)
		}
	}
	got, err := Percentiles([]float64{9, 3, 5, 1, 7}, 5, 50, 95)
	if err != nil || !slices.Equal(got, []float64{1, 5, 9}) {
		t.Errorf("Percentiles = %v, %v, want [1 5 9]", got, err)
	}
	bad := []struct {
		values []float64
		p      float64
	}{
		{nil, 50},
		{[]float64{}, 50},
		{[]float64{1}, -0.1},
		{[]float64{1}, 100.1},
		{[]float64{1}, math.NaN()},
		{[]float64{1, math.NaN()}, 50},
		{[]float64{1, math.Inf(1)}, 50},
	}
	for _, tc := range bad {
		if _, err := Percentile(tc.values, tc.p); err == nil {
			t.Errorf("Percentile(%v, %v) returned no error", tc.values, tc.p)
		}
	}
}

// seq returns 1..n.
func seq(n int) []float64 {
	s := make([]float64, n)
	for k := range s {
		s[k] = float64(n - k) // descending, to exercise the sort
	}
	return s
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
	const pkg = "github.com/mdhender/mpg/internal/field"
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
