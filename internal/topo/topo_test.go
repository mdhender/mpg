// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package topo

import (
	"math"
	"os"
	"os/exec"
	"regexp"
	"testing"
)

// mustNew returns the cylinder for the given sizes or fails the test.
func mustNew(t *testing.T, w, h, rim, falloff float64) Cylinder {
	t.Helper()
	c, err := New(w, h, rim, falloff)
	if err != nil {
		t.Fatalf("New(%v, %v, %v, %v): %v", w, h, rim, falloff, err)
	}
	return c
}

// same reports whether a and b have identical bits, so +0 and −0 differ.
func same(a, b float64) bool {
	return math.Float64bits(a) == math.Float64bits(b)
}

func TestNewValidation(t *testing.T) {
	inf, nan := math.Inf(1), math.NaN()
	bad := []struct {
		name               string
		w, h, rim, falloff float64
	}{
		{"zero width", 0, 100, 2, 6},
		{"negative width", -1, 100, 2, 6},
		{"infinite width", inf, 100, 2, 6},
		{"NaN width", nan, 100, 2, 6},
		{"zero height", 100, 0, 2, 6},
		{"negative height", 100, -5, 2, 6},
		{"infinite height", 100, inf, 2, 6},
		{"NaN height", 100, nan, 2, 6},
		{"zero rim", 100, 100, 0, 6},
		{"negative rim", 100, 100, -2, 6},
		{"infinite rim", 100, 100, inf, 6},
		{"NaN rim", 100, 100, nan, 6},
		{"negative falloff", 100, 100, 2, -1},
		{"infinite falloff", 100, 100, 2, inf},
		{"NaN falloff", 100, 100, 2, nan},
		{"bands exactly fill height", 100, 16, 2, 6},
		{"bands overflow height", 100, 10, 2, 6},
		{"rim alone fills height", 100, 4, 2, 0},
	}
	for _, tc := range bad {
		if _, err := New(tc.w, tc.h, tc.rim, tc.falloff); err == nil {
			t.Errorf("%s: New(%v, %v, %v, %v) returned no error", tc.name, tc.w, tc.h, tc.rim, tc.falloff)
		}
	}
	c := mustNew(t, 100, 16.5, 2, 6)
	if c.W() != 100 || c.H() != 16.5 || c.Rim() != 2 || c.Falloff() != 6 {
		t.Errorf("accessors = %v, %v, %v, %v", c.W(), c.H(), c.Rim(), c.Falloff())
	}
	mustNew(t, 100, 50, 2, 0) // zero falloff is allowed
}

func TestWrapXSeam(t *testing.T) {
	c := mustNew(t, 100, 50, 2, 6)
	cases := []struct{ x, want float64 }{
		{0, 0},
		{math.Copysign(0, -1), 0},
		{0.25, 0.25},
		{99.75, 99.75},
		{100, 0},
		{100.25, 0.25},
		{-0.25, 99.75},
		{-1, 99},
		{-100, 0},
		{-100.5, 99.5},
		{250, 50},
		{-250, 50},
		{1e6 + 3, 3},
		{-1e6 - 3, 97},
	}
	for _, tc := range cases {
		if got := c.WrapX(tc.x); !same(got, tc.want) {
			t.Errorf("WrapX(%v) = %v, want %v", tc.x, got, tc.want)
		}
	}
	// A negative x within half an ulp of a period still lands in [0, W).
	if got := c.WrapX(-1e-20); got < 0 || got >= c.W() {
		t.Errorf("WrapX(-1e-20) = %v, outside [0, %v)", got, c.W())
	}
	if got := c.WrapX(math.Inf(1)); !math.IsNaN(got) {
		t.Errorf("WrapX(+Inf) = %v, want NaN", got)
	}
}

func TestDXSeam(t *testing.T) {
	c := mustNew(t, 100, 50, 2, 6)
	cases := []struct{ a, b, want float64 }{
		{10, 10, 0},
		{10, 20, 10},
		{20, 10, -10},
		{99, 1, 2},  // east across the seam
		{1, 99, -2}, // west across the seam
		{99.75, 0.25, 0.5},
		{0.25, 99.75, -0.5},
		{0, 100, 0}, // the same meridian
		{-1, 1, 2},  // unwrapped inputs
		{1, 201, 0},
		{10, 59.5, 49.5},
		{10, 60.5, -49.5},
		{10, 60, -50}, // the tie goes west
		{60, 10, -50}, // in both directions
		{0, 50, -50},  //
		{75, 25, -50}, //
		{99, 49, -50}, //
		{-50, 0, -50}, //
		{0, -50, -50}, //
		{0, 49.9, 49.9},
	}
	for _, tc := range cases {
		got := c.DX(tc.a, tc.b)
		if !same(got, tc.want) {
			t.Errorf("DX(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
		if got < -c.W()/2 || got >= c.W()/2 {
			t.Errorf("DX(%v, %v) = %v, outside [-W/2, W/2)", tc.a, tc.b, got)
		}
	}
	// Away from the tie, DX is antisymmetric.
	for a := range 100 {
		for b := range 100 {
			x, y := float64(a)+0.3, float64(b)+0.7
			if d := c.DX(x, y); d != -c.W()/2 && c.DX(y, x) != -d {
				t.Fatalf("DX(%v, %v) = %v but DX(%v, %v) = %v", x, y, d, y, x, c.DX(y, x))
			}
		}
	}
}

func TestSeamPointsAreClose(t *testing.T) {
	c := mustNew(t, 100, 50, 2, 6)
	a, b := Point{99.5, 20}, Point{0.5, 20}
	if d := c.Distance(a, b); d != 1 {
		t.Errorf("Distance across seam = %v, want 1", d)
	}
	if dx, dy := c.Delta(a, b); dx != 1 || dy != 0 {
		t.Errorf("Delta across seam = (%v, %v), want (1, 0)", dx, dy)
	}
	if dx, dy := c.Delta(b, a); dx != -1 || dy != 0 {
		t.Errorf("Delta back across seam = (%v, %v), want (-1, 0)", dx, dy)
	}
	// Points straddling the seam diagonally: a 3-4-5 triangle.
	if d := c.Distance(Point{98.5, 10}, Point{1.5, 14}); d != 5 {
		t.Errorf("diagonal Distance across seam = %v, want 5", d)
	}
}

func TestPolesDoNotWrap(t *testing.T) {
	c := mustNew(t, 100, 50, 2, 6)
	north, south := Point{30, 0.5}, Point{30, 49.5}
	if dx, dy := c.Delta(north, south); dx != 0 || dy != 49 {
		t.Errorf("Delta(north, south) = (%v, %v), want (0, 49)", dx, dy)
	}
	if dx, dy := c.Delta(south, north); dx != 0 || dy != -49 {
		t.Errorf("Delta(south, north) = (%v, %v), want (0, -49)", dx, dy)
	}
	if d := c.Distance(north, south); d != 49 {
		t.Errorf("Distance(north, south) = %v, want 49 (no wrap through the pole)", d)
	}
	if b := c.Bearing(north, south); b != 180 {
		t.Errorf("Bearing(north, south) = %v, want 180", b)
	}
	if b := c.Bearing(south, north); b != 0 {
		t.Errorf("Bearing(south, north) = %v, want 0", b)
	}
	// dy is never wrapped, even beyond the map.
	if _, dy := c.Delta(Point{0, -10}, Point{0, 90}); dy != 100 {
		t.Errorf("dy beyond the map = %v, want 100", dy)
	}
}

func TestBearingCompassPoints(t *testing.T) {
	c := mustNew(t, 100, 50, 2, 6)
	cases := []struct {
		dx, dy, want float64
	}{
		{0, -1, 0},
		{1, -1, 45},
		{1, 0, 90},
		{1, 1, 135},
		{0, 1, 180},
		{-1, 1, 225},
		{-1, 0, 270},
		{-1, -1, 315},
	}
	// Each compass point from mid-map, and from a site 0.25 km west of the
	// seam so every eastward target lies across it, and from 0.25 km east of
	// the seam so every westward one does.
	for _, x := range []float64{50, 99.75, 0.25} {
		for _, tc := range cases {
			for _, scale := range []float64{0.5, 3, 7.25} {
				a := Point{x, 25}
				b := Point{c.WrapX(x + tc.dx*scale), 25 + tc.dy*scale}
				if got := c.Bearing(a, b); !same(got, tc.want) {
					t.Errorf("Bearing(%v, %v) = %v, want %v", a, b, got, tc.want)
				}
			}
		}
	}
}

func TestBearingAcrossSeam(t *testing.T) {
	c := mustNew(t, 100, 50, 2, 6)
	cases := []struct {
		a, b Point
		want float64
	}{
		{Point{99, 10}, Point{1, 10}, 90},   // due east across the seam
		{Point{1, 10}, Point{99, 10}, 270},  // due west across the seam
		{Point{99, 10}, Point{1, 8}, 45},    // northeast across the seam
		{Point{99, 10}, Point{1, 12}, 135},  // southeast across the seam
		{Point{1, 10}, Point{99, 12}, 225},  // southwest across the seam
		{Point{1, 10}, Point{99, 8}, 315},   // northwest across the seam
		{Point{0, 10}, Point{0, 10}, 0},     // identical points
		{Point{0, 10}, Point{100, 10}, 0},   // the same point, unwrapped
		{Point{10, 10}, Point{60, 10}, 270}, // half way round: the tie goes west
		{Point{60, 10}, Point{10, 10}, 270},
	}
	for _, tc := range cases {
		if got := c.Bearing(tc.a, tc.b); !same(got, tc.want) {
			t.Errorf("Bearing(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
	// Half way round with a north or south component: the tie still goes west.
	tall := mustNew(t, 100, 200, 2, 6)
	if got := tall.Bearing(Point{10, 100}, Point{60, 50}); got != 315 {
		t.Errorf("tie to the north = %v, want 315", got)
	}
	if got := tall.Bearing(Point{60, 100}, Point{10, 150}); got != 225 {
		t.Errorf("tie to the south = %v, want 225", got)
	}
}

func TestBearingRangeAndReverse(t *testing.T) {
	c := mustNew(t, 100, 50, 2, 6)
	for i := range 41 {
		for j := range 41 {
			a := Point{99, 25}
			b := Point{c.WrapX(99 + float64(i-20)*0.37), 25 + float64(j-20)*0.41}
			got := c.Bearing(a, b)
			if got < 0 || got >= 360 || math.Signbit(got) || math.IsNaN(got) {
				t.Fatalf("Bearing(%v, %v) = %v, outside [0, 360)", a, b, got)
			}
			if i == 20 && j == 20 {
				continue
			}
			back := c.Bearing(b, a)
			diff := math.Abs(math.Mod(back-got+360, 360) - 180)
			if diff > 1e-9 {
				t.Fatalf("Bearing(%v, %v) = %v but reverse = %v", a, b, got, back)
			}
		}
	}
	// A tiny westward step just west of north must not round to 360.
	if got := c.Bearing(Point{50, 25}, Point{50 - 1e-300, 24}); got < 0 || got >= 360 {
		t.Errorf("Bearing just west of north = %v, outside [0, 360)", got)
	}
}

// TestBits pins results of non-exact computations. They are part of the
// cross-architecture contract (see package fmath): a change to any of them
// changes generated worlds and must be deliberate.
func TestBits(t *testing.T) {
	c := mustNew(t, 100, 50, 2, 6)
	cases := []struct {
		a, b          Point
		bearing, dist uint64
	}{
		{Point{50, 25}, Point{53, 21}, 0x40426f58ce59e23c, 0x4014000000000000},       // 36.86989764584402, 5
		{Point{50, 25}, Point{47.5, 32}, 0x4068f4ec206e55aa, 0x401dbb6d5ce3a42f},     // 199.65382405805332, 7.433034373659253
		{Point{99.5, 20}, Point{0.25, 19}, 0x40426f58ce59e23c, 0x3ff4000000000000},   // 36.86989764584402, 1.25
		{Point{0.3, 3.7}, Point{98.2, 40.1}, 0x4066e9a8e233ef77, 0x40423af28920763e}, // 183.301865674435, 36.46052660069516
		{Point{50, 25}, Point{51, 24}, 0x4046800000000000, 0x3ff6a09e667f3bcd},       // 45, 1.4142135623730951
	}
	for _, tc := range cases {
		if got := c.Bearing(tc.a, tc.b); math.Float64bits(got) != tc.bearing {
			t.Errorf("Bearing(%v, %v) = %v (%#016x), want %v (%#016x)", tc.a, tc.b, got, math.Float64bits(got), math.Float64frombits(tc.bearing), tc.bearing)
		}
		if got := c.Distance(tc.a, tc.b); math.Float64bits(got) != tc.dist {
			t.Errorf("Distance(%v, %v) = %v (%#016x), want %v (%#016x)", tc.a, tc.b, got, math.Float64bits(got), math.Float64frombits(tc.dist), tc.dist)
		}
	}
}

func TestLatitude(t *testing.T) {
	c := mustNew(t, 100, 50, 2, 6)
	cases := []struct{ y, want float64 }{
		{0, 1},
		{12.5, 0.5},
		{25, 0},
		{37.5, -0.5},
		{50, -1},
	}
	for _, tc := range cases {
		if got := c.Latitude(tc.y); got != tc.want {
			t.Errorf("Latitude(%v) = %v, want %v", tc.y, got, tc.want)
		}
	}
}

func TestBands(t *testing.T) {
	// H = 50, rim 2, falloff 6: north rim [0, 2), north falloff [2, 8),
	// playable [8, 42], south falloff (42, 48], south rim (48, 50].
	c := mustNew(t, 100, 50, 2, 6)
	cases := []struct {
		y             float64
		rim, falloff  bool
		pole, fromRim float64
	}{
		{-1, true, false, -1, -3},
		{0, true, false, 0, -2},
		{1.75, true, false, 1.75, -0.25},
		{2, false, true, 2, 0},
		{5, false, true, 5, 3},
		{7.75, false, true, 7.75, 5.75},
		{8, false, false, 8, 6},
		{25, false, false, 25, 23},
		{42, false, false, 8, 6},
		{42.25, false, true, 7.75, 5.75},
		{48, false, true, 2, 0},
		{48.25, true, false, 1.75, -0.25},
		{50, true, false, 0, -2},
		{51, true, false, -1, -3},
	}
	for _, tc := range cases {
		if got := c.InRim(tc.y); got != tc.rim {
			t.Errorf("InRim(%v) = %v, want %v", tc.y, got, tc.rim)
		}
		if got := c.InFalloff(tc.y); got != tc.falloff {
			t.Errorf("InFalloff(%v) = %v, want %v", tc.y, got, tc.falloff)
		}
		if got := c.PoleDistance(tc.y); got != tc.pole {
			t.Errorf("PoleDistance(%v) = %v, want %v", tc.y, got, tc.pole)
		}
		if got := c.RimDistance(tc.y); got != tc.fromRim {
			t.Errorf("RimDistance(%v) = %v, want %v", tc.y, got, tc.fromRim)
		}
	}
	if nan := math.NaN(); c.InRim(nan) || c.InFalloff(nan) {
		t.Errorf("NaN y is in a band")
	}
	// With no falloff, nothing is in the falloff band.
	c0 := mustNew(t, 100, 50, 2, 0)
	for _, y := range []float64{1, 2, 3, 47, 48, 49} {
		if c0.InFalloff(y) {
			t.Errorf("InFalloff(%v) with zero falloff = true", y)
		}
	}
}

// TestNoFusedMultiplyAdd compiles this package for the architectures whose Go
// backends contract a*b + c and checks the code holds no fused instruction.
// Package fmath's test of the same name shows the pattern would see one.
func TestNoFusedMultiplyAdd(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles for several architectures")
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool not found")
	}
	fusedOp := regexp.MustCompile(`\tV?FN?M(ADD|SUB)`)
	const pkg = "github.com/mdhender/mpg/internal/topo"
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
