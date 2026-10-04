// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package fmath

import (
	"math"
	"testing"
)

// ulps returns the number of representable float64 values between a and b,
// counting across zero (+0 and -0 are the same point).
func ulps(a, b float64) uint64 {
	oa, ob := ordered(a), ordered(b)
	if oa > ob {
		return uint64(oa - ob)
	}
	return uint64(ob - oa)
}

// ordered maps a finite float64 to an integer that is monotonic in its value.
func ordered(x float64) int64 {
	u := int64(math.Float64bits(x))
	if u < 0 {
		return math.MinInt64 - u
	}
	return u
}

// sweep calls f for count evenly spaced points in [lo, hi], computing each
// point from its index so no error accumulates.
func sweep(lo, hi float64, count int, f func(x float64)) {
	for i := range count {
		f(lo + (hi-lo)*float64(i)/float64(count-1))
	}
}

// maxULPs is the agreement required with package math. Package math is not
// itself a fixed reference: on arm64 (and amd64 built with GOAMD64=v3) its
// pure-Go functions are fused, and on amd64 Exp and Hypot are assembly. Over
// these sweeps, arm64 math differs from this package by 1 ulp in Sin, Cos,
// Atan, and Atan2 and by 2 in Hypot, and amd64 assembly Exp by 2 in the
// subnormal-adjacent range. Exact agreement with unfused Go is checked in
// unfused_amd64_test.go.
const maxULPs = 2

func TestSinCosAccuracy(t *testing.T) {
	check := func(x float64) {
		t.Helper()
		if d := ulps(Sin(x), math.Sin(x)); d > maxULPs {
			t.Fatalf("Sin(%v) = %v, math.Sin = %v (%d ulps)", x, Sin(x), math.Sin(x), d)
		}
		if d := ulps(Cos(x), math.Cos(x)); d > maxULPs {
			t.Fatalf("Cos(%v) = %v, math.Cos = %v (%d ulps)", x, Cos(x), math.Cos(x), d)
		}
	}
	sweep(-1000, 1000, 200_001, check)
	// One cycle of cylinder sampling, theta = 2*Pi*(x+0.5)/W, for W = 4096.
	for x := range 4096 {
		check(2 * math.Pi * (float64(x) + 0.5) / 4096)
	}
	// Large arguments through both reductions, up to the largest float64.
	for x := 1.0; !math.IsInf(x, 0); x *= 1.01 {
		check(x)
		check(-x)
	}
}

func TestSincosMatchesSinAndCos(t *testing.T) {
	check := func(x float64) {
		t.Helper()
		s, c := Sincos(x)
		if math.Float64bits(s) != math.Float64bits(Sin(x)) || math.Float64bits(c) != math.Float64bits(Cos(x)) {
			t.Fatalf("Sincos(%v) = %v, %v; Sin, Cos = %v, %v", x, s, c, Sin(x), Cos(x))
		}
	}
	sweep(-100, 100, 20_001, check)
	for x := 1.0; !math.IsInf(x, 0); x *= 1.07 {
		check(x)
		check(-x)
	}
	check(0)
	check(math.Copysign(0, -1))
}

func TestAtan2Accuracy(t *testing.T) {
	sweep(-50, 50, 401, func(y float64) {
		sweep(-50, 50, 401, func(x float64) {
			if d := ulps(Atan2(y, x), math.Atan2(y, x)); d > maxULPs {
				t.Fatalf("Atan2(%v, %v) = %v, math.Atan2 = %v (%d ulps)", y, x, Atan2(y, x), math.Atan2(y, x), d)
			}
		})
	})
}

func TestAtanAccuracy(t *testing.T) {
	check := func(x float64) {
		t.Helper()
		if d := ulps(Atan(x), math.Atan(x)); d > maxULPs {
			t.Fatalf("Atan(%v) = %v, math.Atan = %v (%d ulps)", x, Atan(x), math.Atan(x), d)
		}
	}
	sweep(-10, 10, 100_001, check)
	for x := 1e-300; !math.IsInf(x, 0); x *= 1.1 {
		check(x)
		check(-x)
	}
}

func TestExpAccuracy(t *testing.T) {
	sweep(-745, 709.78, 145_479, func(x float64) {
		if want := math.Exp(x); math.IsInf(want, 1) && x <= 709.78 {
			// amd64's assembly math.Exp overflows early, from about 709.44,
			// though e**709.78 is below MaxFloat64. Exp gets this right.
			if got := Exp(x); math.IsInf(got, 0) || got < 0x1p1023 {
				t.Fatalf("Exp(%v) = %v, want finite and above 2**1023", x, got)
			}
			return
		}
		if d := ulps(Exp(x), math.Exp(x)); d > maxULPs {
			t.Fatalf("Exp(%v) = %v, math.Exp = %v (%d ulps)", x, Exp(x), math.Exp(x), d)
		}
	})
}

func TestHypotAccuracy(t *testing.T) {
	sweep(-50, 50, 401, func(p float64) {
		sweep(-50, 50, 401, func(q float64) {
			if d := ulps(Hypot(p, q), math.Hypot(p, q)); d > maxULPs {
				t.Fatalf("Hypot(%v, %v) = %v, math.Hypot = %v (%d ulps)", p, q, Hypot(p, q), math.Hypot(p, q), d)
			}
		})
	})
	for _, scale := range []float64{1e-300, 1e-160, 1e160, 1e300} {
		sweep(1, 2, 1001, func(p float64) {
			p, q := p*scale, 0.75*scale
			if d := ulps(Hypot(p, q), math.Hypot(p, q)); d > maxULPs {
				t.Fatalf("Hypot(%v, %v) = %v, math.Hypot = %v (%d ulps)", p, q, Hypot(p, q), math.Hypot(p, q), d)
			}
		})
	}
}
