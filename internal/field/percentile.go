// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package field

import (
	"errors"
	"fmt"
	"math"
	"slices"
)

// Percentile returns the nearest-rank p-th percentile of values: the value of
// rank max(1, ⌈p·n/100⌉) among the n values sorted ascending. values is not
// modified.
//
// It returns an error if values is empty or holds a NaN or infinity, or if p
// is outside [0, 100] or NaN.
func Percentile(values []float64, p float64) (float64, error) {
	r, err := Percentiles(values, p)
	if err != nil {
		return 0, err
	}
	return r[0], nil
}

// Percentiles returns the nearest-rank percentile of values for each p in ps,
// in order, sorting one copy of values. The rules and errors are those of
// Percentile.
func Percentiles(values []float64, ps ...float64) ([]float64, error) {
	if len(values) == 0 {
		return nil, errors.New("field: percentile of no values")
	}
	for _, p := range ps {
		if !(p >= 0 && p <= 100) {
			return nil, fmt.Errorf("field: percentile %v outside [0, 100]", p)
		}
	}
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("field: percentile of non-finite value %v", v)
		}
	}
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	out := make([]float64, len(ps))
	for k, p := range ps {
		out[k] = sorted[rank(p, len(sorted))-1]
	}
	return out, nil
}

// rank returns the 1-based nearest rank max(1, ⌈p·n/100⌉) for p in [0, 100].
// The product is taken before the division, so an integer p and a count
// below 2⁵³/100 give an exact quotient.
func rank(p float64, n int) int {
	r := int(math.Ceil(p * float64(n) / 100))
	return min(max(r, 1), n)
}
