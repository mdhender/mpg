// Copyright (c) 2026 Michael D Henderson. All rights reserved.
//
// The algorithms in this file are copied from the Go standard library's math
// package (Copyright 2009 The Go Authors, BSD-style license; see
// https://go.dev/LICENSE) and, through it, from the Cephes math library by
// Stephen L. Moshier, with explicit roundings added.

package fmath

import "math"

// Atan2 returns the arc tangent of y/x in [-Pi, Pi], using the signs of the
// two to determine the quadrant. It is the bearing function: Atan2(dy, dx).
//
// Domain: all float64. Accuracy: within 2 ulp of math.Atan2 (Cephes reports a
// peak relative error near 1e-16 for the underlying arctangent).
//
// Special cases, as math.Atan2, in order:
//
//	Atan2(y, NaN) = NaN
//	Atan2(NaN, x) = NaN
//	Atan2(+0, x>=0) = +0
//	Atan2(-0, x>=0) = -0
//	Atan2(+0, x<=-0) = +Pi
//	Atan2(-0, x<=-0) = -Pi
//	Atan2(y>0, 0) = +Pi/2
//	Atan2(y<0, 0) = -Pi/2
//	Atan2(+Inf, +Inf) = +Pi/4
//	Atan2(-Inf, +Inf) = -Pi/4
//	Atan2(+Inf, -Inf) = 3Pi/4
//	Atan2(-Inf, -Inf) = -3Pi/4
//	Atan2(y, +Inf) = 0 with the sign of y
//	Atan2(y>0, -Inf) = +Pi
//	Atan2(y<0, -Inf) = -Pi
//	Atan2(+Inf, x) = +Pi/2
//	Atan2(-Inf, x) = -Pi/2
func Atan2(y, x float64) float64 {
	switch {
	case math.IsNaN(y) || math.IsNaN(x):
		return math.NaN()
	case y == 0:
		if x >= 0 && !math.Signbit(x) {
			return math.Copysign(0, y)
		}
		return math.Copysign(math.Pi, y)
	case x == 0:
		return math.Copysign(math.Pi/2, y)
	case math.IsInf(x, 0):
		if math.IsInf(x, 1) {
			if math.IsInf(y, 0) {
				return math.Copysign(math.Pi/4, y)
			}
			return math.Copysign(0, y)
		}
		if math.IsInf(y, 0) {
			return math.Copysign(3*math.Pi/4, y)
		}
		return math.Copysign(math.Pi, y)
	case math.IsInf(y, 0):
		return math.Copysign(math.Pi/2, y)
	}

	q := Atan(y / x)
	if x < 0 {
		if q <= 0 {
			return q + math.Pi
		}
		return q - math.Pi
	}
	return q
}

// Atan returns the arctangent of x in [-Pi/2, Pi/2].
//
// Domain: all float64. Accuracy: within 2 ulp of math.Atan.
//
// Special cases, as math.Atan:
//
//	Atan(±0) = ±0
//	Atan(±Inf) = ±Pi/2
//	Atan(NaN) = NaN
func Atan(x float64) float64 {
	if x == 0 {
		return x
	}
	if x > 0 {
		return satan(x)
	}
	return -satan(-x)
}

// satan reduces its positive argument to the range [0, 0.66] and calls xatan.
func satan(x float64) float64 {
	const (
		morebits = 6.123233995736765886130e-17 // pi/2 = PIO2 + morebits
		tan3pio8 = 2.41421356237309504880      // tan(3*pi/8)
	)
	if x <= 0.66 {
		return xatan(x)
	}
	if x > tan3pio8 {
		return math.Pi/2 - xatan(1/x) + morebits
	}
	return math.Pi/4 + xatan((x-1)/(x+1)) + 0.5*morebits
}

// xatan evaluates a series valid in the range [0, 0.66].
func xatan(x float64) float64 {
	const (
		p0 = -8.750608600031904122785e-01
		p1 = -1.615753718733365076637e+01
		p2 = -7.500855792314704667340e+01
		p3 = -1.228866684490136173410e+02
		p4 = -6.485021904942025371773e+01
		q0 = +2.485846490142306297962e+01
		q1 = +1.650270098316988542046e+02
		q2 = +4.328810604912902668951e+02
		q3 = +4.853903996359136964868e+02
		q4 = +1.945506571482613964425e+02
	)
	z := float64(x * x) // feeds z + q0 below
	num := float64(p0*z) + p1
	num = float64(num*z) + p2
	num = float64(num*z) + p3
	num = float64(num*z) + p4
	den := z + q0
	den = float64(den*z) + q1
	den = float64(den*z) + q2
	den = float64(den*z) + q3
	den = float64(den*z) + q4
	z = z * num / den
	return float64(x*z) + x
}
