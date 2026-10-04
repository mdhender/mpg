// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package fmath

import (
	"math"
	"math/big"
	"testing"
)

// TestMulAddRoundsTwice pins a case where one rounding and two disagree:
// a*a = 1 + 2^-29 + 2^-60 exactly, which rounds to 1 + 2^-29, so adding
// -(1 + 2^-29) gives 0 with two roundings and 2^-60 with one.
func TestMulAddRoundsTwice(t *testing.T) {
	a := 1 + 0x1p-30
	c := -(1 + 0x1p-29)
	pin(t, "MulAdd", MulAdd(a, a, c), 0)
	pin(t, "Mul+c", Mul(a, a)+c, 0)
	pin(t, "math.FMA", math.FMA(a, a, c), math.Float64bits(0x1p-60))
}

// TestMulAddMatchesTwoRoundings checks MulAdd against a reference computed
// in math/big: the exact product rounded to 53 bits, then added and rounded
// again. The reference cannot be written as p := a*b; p + c in Go, because the
// compiler may fuse that across the variable (arm64 does).
func TestMulAddMatchesTwoRoundings(t *testing.T) {
	ref := func(a, b, c float64) float64 {
		p := new(big.Float).SetPrec(106).Mul(big.NewFloat(a).SetPrec(106), big.NewFloat(b).SetPrec(106))
		p.SetPrec(53) // round to nearest even, as float64 does
		s := new(big.Float).SetPrec(53).Add(p, big.NewFloat(c))
		v, _ := s.Float64()
		return v
	}
	sweep(-10, 10, 201, func(a float64) {
		sweep(-10, 10, 201, func(b float64) {
			const c = 0.1
			want := ref(a, b, c)
			if got := MulAdd(a, b, c); math.Float64bits(got) != math.Float64bits(want) {
				t.Fatalf("MulAdd(%v, %v, %v) = %v, want %v", a, b, c, got, want)
			}
			if got := Mul(a, b) + c; math.Float64bits(got) != math.Float64bits(want) {
				t.Fatalf("Mul(%v, %v) + %v = %v, want %v", a, b, c, got, want)
			}
		})
	})
}
