// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package fmath

import "math"

// Signed is the set of signed integer types FloorDiv and FloorMod accept.
type Signed interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64
}

// FloorDiv returns a/b rounded toward negative infinity.
//
// Go's / truncates toward zero, so it differs from FloorDiv exactly when a is
// negative and the division is inexact: FloorDiv(-1, 4) is -1 where -1/4 is 0.
//
// b must be positive; it panics otherwise. Every divisor in the design is a
// size (a map width, a tile size), and floor and Euclidean division part
// company for a negative divisor, so a non-positive b is a caller error.
func FloorDiv[T Signed](a, b T) T {
	if b <= 0 {
		panic("fmath: FloorDiv divisor must be positive")
	}
	q := a / b
	if a%b < 0 {
		q--
	}
	return q
}

// FloorMod returns a modulo b in [0, b), the remainder that pairs with
// FloorDiv: FloorDiv(a, b)*b + FloorMod(a, b) == a. It is the east-west wrap
// of an integer column: FloorMod(-1, w) == w-1.
//
// b must be positive; it panics otherwise.
func FloorMod[T Signed](a, b T) T {
	if b <= 0 {
		panic("fmath: FloorMod divisor must be positive")
	}
	m := a % b
	if m < 0 {
		m += b
	}
	return m
}

// FloorDivFloat returns math.Floor(x/m), the number of whole periods m below
// x. The quotient is rounded once before the floor, so for an x within an ulp
// of a multiple of m it may disagree with FloorModFloat by one period; use
// FloorModFloat, not x - FloorDivFloat(x, m)*m, to wrap a coordinate.
//
// m must be positive and finite; it panics otherwise. An infinite or NaN x
// returns NaN.
func FloorDivFloat(x, m float64) float64 {
	checkPeriod(m, "FloorDivFloat")
	if math.IsInf(x, 0) || math.IsNaN(x) {
		return math.NaN()
	}
	return math.Floor(x / m)
}

// FloorModFloat returns x modulo m in [0, m), the east-west wrap of a
// coordinate: FloorModFloat(-0.25, 10) == 9.75.
//
// The remainder from math.Mod is exact; the one rounding is in adding m to a
// negative remainder. When that sum rounds up to m itself (a negative x within
// half an ulp of m below a multiple of m), the result is 0, so the half-open
// range always holds. A zero result is always +0, never -0.
//
// m must be positive and finite; it panics otherwise. An infinite or NaN x
// returns NaN.
func FloorModFloat(x, m float64) float64 {
	checkPeriod(m, "FloorModFloat")
	if math.IsInf(x, 0) || math.IsNaN(x) {
		return math.NaN()
	}
	r := math.Mod(x, m)
	if r < 0 {
		r += m
		if r >= m {
			return 0
		}
	}
	if r == 0 {
		return 0 // normalize -0
	}
	return r
}

func checkPeriod(m float64, fn string) {
	if !(m > 0) || math.IsInf(m, 1) {
		panic("fmath: " + fn + " period must be positive and finite")
	}
}
