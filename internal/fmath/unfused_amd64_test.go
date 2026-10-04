// Copyright (c) 2026 Michael D Henderson. All rights reserved.

//go:build !amd64.v3

package fmath

import (
	"math"
	"testing"
)

// On amd64 below GOAMD64=v3 the compiler never fuses, and math.Sin, math.Cos,
// math.Sincos, math.Atan, and math.Atan2 are pure Go. There the standard
// library is the unfused reference this package copies, so the two must agree
// bit for bit. (math.Exp and math.Hypot are assembly on amd64 and excluded.)
func TestMatchesUnfusedStandardLibrary(t *testing.T) {
	same := func(name string, x, got, want float64) {
		t.Helper()
		if math.Float64bits(got) != math.Float64bits(want) {
			t.Fatalf("%s(%v) = %v (%#x), math = %v (%#x)", name, x, got, math.Float64bits(got), want, math.Float64bits(want))
		}
	}
	trig := func(x float64) {
		t.Helper()
		same("Sin", x, Sin(x), math.Sin(x))
		same("Cos", x, Cos(x), math.Cos(x))
		s, c := Sincos(x)
		ms, mc := math.Sincos(x)
		same("Sincos.sin", x, s, ms)
		same("Sincos.cos", x, c, mc)
		same("Atan", x, Atan(x), math.Atan(x))
	}
	sweep(-1000, 1000, 200_001, trig)
	for x := 1.0; !math.IsInf(x, 0); x *= 1.01 {
		trig(x)
		trig(-x)
	}
	sweep(-50, 50, 401, func(y float64) {
		sweep(-50, 50, 401, func(x float64) {
			same("Atan2", y, Atan2(y, x), math.Atan2(y, x))
		})
	})
}
