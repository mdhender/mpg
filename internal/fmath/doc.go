// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package fmath provides the floating-point and integer arithmetic that world
// generation needs to be bit-identical on every architecture.
//
// The Go standard library does not promise identical bits across machines.
// The specification lets a compiler fuse x*y + z into one multiply-add with a
// single rounding. The arm64 backend always does, and the amd64 backend does
// when built with GOAMD64=v3 or higher, so pure-Go functions such as
// math.Atan2 and math.Sin round differently from one build to another. Other
// functions have per-architecture assembly: math.Hypot on amd64, and math.Exp
// on amd64 and arm64. The same config must rebuild the same world on any
// machine (DESIGN.md, "Determinism"), so hashed arithmetic uses this package.
//
// The rule that makes it work: every product that can reach an addition or a
// subtraction is wrapped in an explicit float64 conversion. That includes a
// product stored in a variable and added later (fusion works on the compiled
// expression graph, not on source lines) and a product converted to uint64
// (some backends lower that conversion with a subtraction). Per the language
// specification an explicit conversion rounds to float64 precision, which
// forbids fusion, and the rule survives inlining. TestNoFusedMultiplyAdd
// compiles the package for every fusing architecture and fails on any fused
// instruction. Callers write
//
//	v := fmath.MulAdd(a, b, c)    // or fmath.Mul(a, b) + c
//
// rather than a bare a*b + c. Do not substitute math.FMA, which is the opt-in
// to fusion and the opposite of what this package is for.
//
// Hypot, Atan, Atan2, Exp, Sin, Cos, and Sincos are copies of the standard
// library's pure-Go implementations (themselves from the Cephes library by
// Stephen L. Moshier) with the explicit roundings added. Their results equal
// the standard library's unfused pure-Go results bit for bit; pinned bit
// patterns in the tests are the cross-architecture contract.
//
// Operations that IEEE 754 defines as correctly rounded or exact give the same
// bits everywhere and may be taken from package math directly: math.Sqrt,
// math.Abs, math.Floor, math.Ceil, math.Trunc, math.Mod, math.Copysign,
// math.Ldexp, and math.Frexp. Plain +, -, *, and / are likewise exact-rounded;
// only their combination into a multiply-add is the hazard.
package fmath
