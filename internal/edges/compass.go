// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package edges

import (
	"cmp"
	"fmt"
	"math"
	"slices"
)

// Direction is one of the 8 compass points, numbered clockwise from north:
// N is 0, NE 1, …, NW 7. Its angle is 45° times its code.
type Direction uint8

// The compass points, clockwise from north.
const (
	N Direction = iota
	NE
	E
	SE
	S
	SW
	W
	NW
	numDirections
)

// MaxDegree is the most neighbors a cell may have: one per compass point.
const MaxDegree = int(numDirections)

// Directions lists the compass points in code order, clockwise from north.
var Directions = []Direction{N, NE, E, SE, S, SW, W, NW}

var directionNames = [numDirections]string{"N", "NE", "E", "SE", "S", "SW", "W", "NW"}

// String returns the compass point's name, as "NE".
func (d Direction) String() string {
	if d < numDirections {
		return directionNames[d]
	}
	return fmt.Sprintf("Direction(%d)", uint8(d))
}

// Degrees returns the compass point's angle in degrees clockwise from
// north: 45 times its code, exactly (an integer product, so it cannot fuse
// into a later subtraction).
func (d Direction) Degrees() float64 { return float64(45 * int(d)) }

// Opposite returns the compass point 180° away.
func (d Direction) Opposite() Direction { return (d + 4) % numDirections }

// Nearest returns the compass point nearest bearing (degrees in [0, 360)),
// ties to the lower code. It is the naive per-edge label, which collides
// within a cell; Assign is the per-cell assignment the edges use.
func Nearest(bearing float64) Direction {
	best := N
	for _, d := range Directions[1:] {
		if AngularError(bearing, d) < AngularError(bearing, best) {
			best = d
		}
	}
	return best
}

// AngularError returns the angle in degrees, in [0, 180], between bearing
// (degrees in [0, 360)) and the compass point d: the smaller of the two
// ways around. It is one subtraction and one comparison, so it is the same
// on every machine.
func AngularError(bearing float64, d Direction) float64 {
	a := math.Abs(bearing - d.Degrees())
	return min(a, 360-a)
}

// costBits is the fixed-point scale of the assignment's costs: an angular
// error is compared in units of 2⁻³² degree (about 2.3·10⁻¹⁰°).
const costBits = 32

// cost returns AngularError(bearing, d) in units of 2⁻³² degree, rounded to
// the nearest unit. Scaling by a power of two is exact, so the only
// rounding is the final one, and integer sums of costs are exact.
func cost(bearing float64, d Direction) int64 {
	return int64(math.Round(math.Ldexp(AngularError(bearing, d), costBits)))
}

// Assign returns the compass point of each of a cell's edges, given the
// bearings of its neighbors (degrees in [0, 360), at most MaxDegree, in
// any order): out[k] is the direction of the edge with bearing
// bearings[k]. See the package documentation for the rule; in brief, the
// edges sorted by bearing (ties by input index) take distinct compass
// points in the same cyclic clockwise order, with the smallest total
// angular error, then the smallest largest error, then the
// lexicographically smallest sequence of compass codes in sorted-bearing
// order.
func Assign(bearings []float64) ([]Direction, error) {
	k := len(bearings)
	if k > MaxDegree {
		return nil, fmt.Errorf("edges: %d edges, more than the %d compass points", k, MaxDegree)
	}
	for i, b := range bearings {
		if !(b >= 0 && b < 360) {
			return nil, fmt.Errorf("edges: bearing %d is %v, want [0, 360)", i, b)
		}
	}
	order := sortedByBearing(bearings)
	var c costs
	for i, j := range order {
		for _, d := range Directions {
			c[i][d] = cost(bearings[j], d)
		}
	}
	p := assign(&c, k)
	out := make([]Direction, k)
	for i, j := range order {
		out[j] = Direction(p.seq[i])
	}
	return out, nil
}

// sortedByBearing returns the indexes of bearings sorted by bearing, ties
// by index.
func sortedByBearing(bearings []float64) []int {
	order := make([]int, len(bearings))
	for i := range order {
		order[i] = i
	}
	slices.SortFunc(order, func(a, b int) int {
		return cmp.Or(cmp.Compare(bearings[a], bearings[b]), cmp.Compare(a, b))
	})
	return order
}

// costs holds, for the edges in sorted-bearing order, each edge's cost
// (fixed-point angular error) for each compass point.
type costs [MaxDegree][numDirections]int64

// path is a partial or complete assignment: seq[i] is the compass code of
// the i'th edge in sorted-bearing order.
type path struct {
	ok    bool
	sum   int64 // total cost
	count int   // number of assignments with this sum, saturated at 2
	seq   [MaxDegree]uint8
}

// assign returns the chosen assignment of k edges with costs c: the least
// total cost; among those, the least largest cost; among those, the
// lexicographically least sequence. The dynamic program (dp) is exact for
// the total and the sequence, and counts the assignments that reach the
// least total. When there is more than one, the least largest cost is
// found by a bisection over the cost values, each step a dynamic program
// restricted to costs at or below a limit: the smallest limit that still
// reaches the least total is the least largest cost, and the restricted
// program's choice is then the lexicographically least among the
// assignments with both.
func assign(c *costs, k int) path {
	if k == 0 {
		return path{ok: true, count: 1}
	}
	best := dp(c, k, math.MaxInt64)
	if best.count == 1 {
		return best
	}
	vals := make([]int64, 0, k*MaxDegree)
	for i := range k {
		vals = append(vals, c[i][:]...)
	}
	slices.Sort(vals)
	vals = slices.Compact(vals)
	lo, hi := 0, len(vals)-1 // the largest value is no restriction
	for lo < hi {
		mid := (lo + hi) / 2
		if p := dp(c, k, vals[mid]); p.ok && p.sum == best.sum {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return dp(c, k, vals[lo])
}

// dp is the dynamic program over the cyclic order-preserving assignments
// of k edges whose costs are all at most limit. An assignment is lifted to
// integers: the first edge (sorted by bearing) takes D₀ = d₀ in [0, 8), and
// the others strictly increasing D₁ < … < D_{k−1} ≤ D₀ + 7, with compass
// code D mod 8. For each start D₀ the program runs over the edges in order
// with state D_i, keeping per state the least partial sum, the number of
// partial assignments reaching it (saturated at 2), and the
// lexicographically least code sequence among them. The sums are exact
// integers and the sequences are compared from the first edge, so a state's
// choice is the restriction of every best completion: the program is exact
// for (sum, sequence). It returns ok false when no assignment meets the
// limit.
func dp(c *costs, k int, limit int64) path {
	var res path
	for d0 := range MaxDegree {
		if c[0][d0] > limit {
			continue
		}
		// cur[off] is the best partial assignment with D_i = d0 + off.
		var cur [MaxDegree]path
		cur[0] = path{ok: true, sum: c[0][d0], count: 1}
		cur[0].seq[0] = uint8(d0)
		for i := 1; i < k; i++ {
			var next [MaxDegree]path
			for off := i; off <= MaxDegree-k+i; off++ {
				d := (d0 + off) % MaxDegree
				cd := c[i][d]
				if cd > limit {
					continue
				}
				for prev := i - 1; prev < off; prev++ {
					if !cur[prev].ok {
						continue
					}
					cand := cur[prev]
					cand.sum += cd
					cand.seq[i] = uint8(d)
					merge(&next[off], cand, i+1)
				}
			}
			cur = next
		}
		for off := k - 1; off < MaxDegree; off++ {
			if cur[off].ok {
				merge(&res, cur[off], k)
			}
		}
	}
	return res
}

// merge folds cand, an assignment of the first n edges, into best: the
// lesser sum wins, then the lexicographically lesser sequence, and the
// counts of equal sums add (saturated at 2).
func merge(best *path, cand path, n int) {
	switch {
	case !best.ok || cand.sum < best.sum:
		*best = cand
	case cand.sum == best.sum:
		count := min(2, best.count+cand.count)
		if slices.Compare(cand.seq[:n], best.seq[:n]) < 0 {
			*best = cand
		}
		best.count = count
	}
}
