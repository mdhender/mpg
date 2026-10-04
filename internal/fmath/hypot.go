// Copyright (c) 2026 Michael D Henderson. All rights reserved.
//
// The algorithm in this file is copied from the Go standard library's math
// package (Copyright 2009 The Go Authors, BSD-style license; see
// https://go.dev/LICENSE), with explicit roundings added.

package fmath

import "math"

// Hypot returns Sqrt(p*p + q*q), scaled to avoid needless overflow and
// underflow.
//
// Domain: all float64. Accuracy: within 1 ulp of the correctly rounded result
// (it is not itself correctly rounded). The sign of p and q is ignored.
//
// Special cases, as math.Hypot:
//
//	Hypot(±Inf, q) = +Inf
//	Hypot(p, ±Inf) = +Inf
//	Hypot(NaN, q) = NaN   (unless q is ±Inf)
//	Hypot(p, NaN) = NaN   (unless p is ±Inf)
//	Hypot(±0, ±0) = +0
func Hypot(p, q float64) float64 {
	p, q = math.Abs(p), math.Abs(q)
	switch {
	case math.IsInf(p, 1) || math.IsInf(q, 1):
		return math.Inf(1)
	case math.IsNaN(p) || math.IsNaN(q):
		return math.NaN()
	}
	if p < q {
		p, q = q, p
	}
	if p == 0 {
		return 0
	}
	q = q / p
	return p * math.Sqrt(1+float64(q*q))
}
