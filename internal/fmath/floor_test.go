// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package fmath

import (
	"math"
	"testing"
)

func TestFloorDivModTable(t *testing.T) {
	cases := []struct{ a, b, div, mod int }{
		{0, 4, 0, 0},
		{3, 4, 0, 3},
		{4, 4, 1, 0},
		{7, 4, 1, 3},
		{-1, 4, -1, 3},
		{-3, 4, -1, 1},
		{-4, 4, -1, 0},
		{-5, 4, -2, 3},
		{-8, 4, -2, 0},
		{-33, 32, -2, 31},
		{-1, 1000, -1, 999}, // one column west of x=0 wraps to the last column
		{1000, 1000, 1, 0},  // one column past the last wraps to 0
		{1001, 1000, 1, 1},
		{-2000, 1000, -2, 0},
		{math.MinInt, 1, math.MinInt, 0},
		{math.MinInt, math.MaxInt, -2, math.MaxInt - 1},
		{math.MaxInt, math.MaxInt, 1, 0},
	}
	for _, c := range cases {
		if got := FloorDiv(c.a, c.b); got != c.div {
			t.Errorf("FloorDiv(%d, %d) = %d, want %d", c.a, c.b, got, c.div)
		}
		if got := FloorMod(c.a, c.b); got != c.mod {
			t.Errorf("FloorMod(%d, %d) = %d, want %d", c.a, c.b, got, c.mod)
		}
	}
}

// TestFloorDivModAroundZero is exhaustive over the range where Go's truncating
// division and the floor disagree: every negative dividend with an inexact
// quotient.
func TestFloorDivModAroundZero(t *testing.T) {
	for b := int64(1); b <= 64; b++ {
		for a := int64(-4096); a <= 4096; a++ {
			// Derived independently rather than from the implementation.
			want := int64(math.Floor(float64(a) / float64(b)))
			if got := FloorDiv(a, b); got != want {
				t.Fatalf("FloorDiv(%d, %d) = %d, want %d", a, b, got, want)
			}
			m := FloorMod(a, b)
			if m < 0 || m >= b {
				t.Fatalf("FloorMod(%d, %d) = %d, outside [0, %d)", a, b, m, b)
			}
			if got := want*b + m; got != a {
				t.Fatalf("FloorDiv(%d, %d)*%d + FloorMod = %d, want %d", a, b, b, got, a)
			}
		}
	}
}

// TestFloorDivModNarrowTypes checks the generic instantiation on a type whose
// whole range can be enumerated.
func TestFloorDivModNarrowTypes(t *testing.T) {
	for b := int8(1); b > 0; b++ { // stops when b wraps past MaxInt8
		for a := int16(math.MinInt8); a <= math.MaxInt8; a++ {
			q, m := FloorDiv(int8(a), b), FloorMod(int8(a), b)
			if m < 0 || m >= b || int16(q)*int16(b)+int16(m) != a {
				t.Fatalf("int8 FloorDiv/FloorMod(%d, %d) = %d, %d", a, b, q, m)
			}
		}
	}
}

// TestFloorDivDiffersFromGo pins the reason these helpers exist.
func TestFloorDivDiffersFromGo(t *testing.T) {
	a, b := -33, 32
	if a/b == FloorDiv(a, b) {
		t.Error("Go's / already floors; the helper's reason for existing has changed")
	}
	if a%b == FloorMod(a, b) {
		t.Error("Go's % already takes the sign of the divisor")
	}
}

func TestFloorModFloatTable(t *testing.T) {
	cases := []struct {
		x, m float64
		mod  uint64
		div  float64
	}{
		{0, 10, 0x0000000000000000, 0},
		{negZero, 10, 0x0000000000000000, 0}, // -0 normalizes to +0
		{2.5, 10, 0x4004000000000000, 0},     // 2.5
		{10, 10, 0x0000000000000000, 1},
		{10.5, 10, 0x3fe0000000000000, 1},   // 0.5
		{-0.25, 10, 0x4023800000000000, -1}, // 9.75
		{-10, 10, 0x0000000000000000, -1},
		{-10.5, 10, 0x4023000000000000, -2}, // 9.5
		{-1, 360, 0x4076700000000000, -1},   // 359
		{725, 360, 0x4014000000000000, 2},   // 5
		{-725, 360, 0x4076300000000000, -3}, // 355
		{0.1, 0.03, 0x3f847ae147ae1480, 3},  // exact remainder of the stored operands; checked against C fmod
		{-0.1, 0.03, 0x3f947ae147ae1478, -4},
		{-1e-20, 1, 0x0000000000000000, -1}, // 1 - 1e-20 rounds to 1; wraps to 0
		{1e300, 7, 0x3ff0000000000000, 1.4285714285714286e299},
		{-1e300, 7, 0x4018000000000000, -1.4285714285714286e299},
	}
	for _, c := range cases {
		got := FloorModFloat(c.x, c.m)
		pin(t, "FloorModFloat("+fmtF(c.x)+", "+fmtF(c.m)+")", got, c.mod)
		if !(got >= 0 && got < c.m) {
			t.Errorf("FloorModFloat(%v, %v) = %v, outside [0, %v)", c.x, c.m, got, c.m)
		}
		if got := FloorDivFloat(c.x, c.m); got != c.div {
			t.Errorf("FloorDivFloat(%v, %v) = %v, want %v", c.x, c.m, got, c.div)
		}
	}
}

func TestFloorModFloatRange(t *testing.T) {
	const w = 4096.0
	sweep(-3*w, 3*w, 100_003, func(x float64) {
		if m := FloorModFloat(x, w); !(m >= 0 && m < w) || math.Signbit(m) {
			t.Fatalf("FloorModFloat(%v, %v) = %v, outside [0, %v)", x, w, m, w)
		}
	})
	for x := 1e-300; x < 1e300; x *= 3 {
		for _, v := range []float64{x, -x} {
			if m := FloorModFloat(v, w); !(m >= 0 && m < w) {
				t.Fatalf("FloorModFloat(%v, %v) = %v, outside [0, %v)", v, w, m, w)
			}
		}
	}
}

func TestFloorFloatNonFinite(t *testing.T) {
	for _, x := range []float64{math.NaN(), inf, -inf} {
		if got := FloorModFloat(x, 10); !math.IsNaN(got) {
			t.Errorf("FloorModFloat(%v, 10) = %v, want NaN", x, got)
		}
		if got := FloorDivFloat(x, 10); !math.IsNaN(got) {
			t.Errorf("FloorDivFloat(%v, 10) = %v, want NaN", x, got)
		}
	}
}

func TestFloorRejectsBadDivisor(t *testing.T) {
	for _, b := range []int{0, -1, math.MinInt} {
		mustPanic(t, "FloorDiv", func() { FloorDiv(7, b) })
		mustPanic(t, "FloorMod", func() { FloorMod(7, b) })
	}
	for _, m := range []float64{0, negZero, -1, inf, -inf, math.NaN()} {
		mustPanic(t, "FloorDivFloat", func() { FloorDivFloat(7, m) })
		mustPanic(t, "FloorModFloat", func() { FloorModFloat(7, m) })
	}
}

func mustPanic(t *testing.T, what string, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s did not panic", what)
		}
	}()
	f()
}
