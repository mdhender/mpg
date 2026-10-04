// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package fmath

// Mul returns a*b rounded to float64. Use it where a product feeds an addition
// or subtraction, so the compiler cannot fuse the two into one rounding:
//
//	v := fmath.Mul(a, b) + c
func Mul(a, b float64) float64 {
	return float64(a * b)
}

// MulAdd returns a*b + c with the product rounded before the addition, that is
// with two roundings on every architecture. It is the deliberate opposite of
// math.FMA, which rounds once.
func MulAdd(a, b, c float64) float64 {
	return float64(a*b) + c
}
