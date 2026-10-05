// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package edges

import (
	"slices"
)

// bruteResult is one order-preserving assignment and its score.
type bruteResult struct {
	sum, max int64
	seq      []uint8 // compass codes in sorted-bearing order
}

// better reports whether a beats b by the design's rule: lower total, then
// lower maximum, then the lexicographically smaller sequence.
func (a bruteResult) better(b bruteResult) bool {
	if a.sum != b.sum {
		return a.sum < b.sum
	}
	if a.max != b.max {
		return a.max < b.max
	}
	return slices.Compare(a.seq, b.seq) < 0
}

// bruteAssign enumerates every cyclic order-preserving assignment of the
// bearings (sorted, ties by index) to distinct compass points, defined
// directly: a sequence of distinct codes whose cyclic gaps, from each
// edge's code to the next edge's (the last to the first), are all at
// least 1 and sum to 8. It returns the best by the design's rule, with
// the codes mapped back to input order, and the number of assignments.
func bruteAssign(bearings []float64) ([]Direction, int) {
	k := len(bearings)
	order := sortedByBearing(bearings)
	var best bruteResult
	found, count := false, 0
	seq := make([]uint8, k)
	var rec func(i int)
	rec = func(i int) {
		if i == k {
			if k > 0 {
				gaps := 0
				for j := range k {
					g := (int(seq[(j+1)%k]) - int(seq[j]) + 8) % 8
					if g == 0 {
						g = 8 // k == 1: the one gap is the whole circle
					}
					gaps += g
				}
				if gaps != 8 {
					return
				}
			}
			count++
			r := bruteResult{seq: slices.Clone(seq)}
			for j, idx := range order {
				c := cost(bearings[idx], Direction(seq[j]))
				r.sum += c
				r.max = max(r.max, c)
			}
			if !found || r.better(best) {
				best, found = r, true
			}
			return
		}
		for d := range uint8(8) {
			if !slices.Contains(seq[:i], d) {
				seq[i] = d
				rec(i + 1)
			}
		}
	}
	rec(0)
	out := make([]Direction, k)
	for j, idx := range order {
		out[idx] = Direction(best.seq[j])
	}
	return out, count
}
