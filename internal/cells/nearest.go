// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package cells

import (
	"fmt"
	"math"

	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/internal/topo"
)

// boundMarginKm is subtracted from the ring search's lower bound, so a site
// whose bucket index rounded across a bucket boundary is still covered by
// the bound. Bucket indices are a single correctly rounded division away
// from exact, which is off by far less than this.
const boundMarginKm = 1e-6

// Locator finds the nearest site to a point on the cylinder through a
// wrap-aware bucket grid. Build one with NewLocator; the zero value is not
// valid. See the package documentation for the definition of nearest and
// the proof that the search is exact.
type Locator struct {
	cyl      topo.Cylinder
	sites    []topo.Point
	nbx, nby int
	bw, bh   float64
	// start and items hold the sites bucket by bucket: the sites of bucket
	// b = by·nbx + bx are items[start[b]:start[b+1]], ascending.
	start, items []int
}

// NewLocator returns a locator over sites on cyl. Every site must have X in
// [0, W) and Y in [0, H], and there must be at least one.
func NewLocator(cyl topo.Cylinder, sites []topo.Point) (*Locator, error) {
	if cyl.W() == 0 {
		return nil, fmt.Errorf("cells: cylinder is not initialized")
	}
	if len(sites) == 0 {
		return nil, fmt.Errorf("cells: no sites")
	}
	for i, s := range sites {
		if !(s.X >= 0 && s.X < cyl.W() && s.Y >= 0 && s.Y <= cyl.H()) {
			return nil, fmt.Errorf("cells: site %d at (%v, %v) is outside [0, %v) × [0, %v]", i, s.X, s.Y, cyl.W(), cyl.H())
		}
	}
	// Buckets about one mean site spacing on a side, so a bucket holds
	// about one site.
	side := math.Sqrt(cyl.W() * cyl.H() / float64(len(sites)))
	nbx := max(1, int(cyl.W()/side))
	nby := max(1, int(cyl.H()/side))
	l := &Locator{
		cyl:   cyl,
		sites: sites,
		nbx:   nbx,
		nby:   nby,
		bw:    cyl.W() / float64(nbx),
		bh:    cyl.H() / float64(nby),
	}
	// Counting sort by bucket, in site order, so each bucket is ascending.
	l.start = make([]int, nbx*nby+1)
	bucket := make([]int, len(sites))
	for i, s := range sites {
		bx, by := l.bucketOf(s)
		bucket[i] = by*nbx + bx
		l.start[bucket[i]+1]++
	}
	for b := range nbx * nby {
		l.start[b+1] += l.start[b]
	}
	l.items = make([]int, len(sites))
	next := append([]int(nil), l.start[:nbx*nby]...)
	for i, b := range bucket {
		l.items[next[b]] = i
		next[b]++
	}
	return l, nil
}

// bucketOf returns the bucket column and row of a point with X in [0, W)
// and Y in [0, H].
func (l *Locator) bucketOf(p topo.Point) (bx, by int) {
	bx = min(max(int(p.X/l.bw), 0), l.nbx-1)
	by = min(max(int(p.Y/l.bh), 0), l.nby-1)
	return bx, by
}

// Dist2 returns the squared distance from p to site i that the nearest-site
// rule compares: dx² + dy² with dx the wrapped east-west displacement
// (topo DX), each product rounded before the sum.
func (l *Locator) Dist2(p topo.Point, i int) float64 {
	return dist2(l.cyl, p, l.sites[i])
}

func dist2(cyl topo.Cylinder, p, s topo.Point) float64 {
	dx, dy := cyl.Delta(p, s)
	return fmath.MulAdd(dx, dx, fmath.Mul(dy, dy))
}

// Nearest returns the index of the site nearest to p (X in [0, W), Y in
// [0, H]) by Dist2, the lowest index among equally near sites. It is the
// same site BruteNearest returns.
func (l *Locator) Nearest(p topo.Point) int {
	bx, by := l.bucketOf(p)
	best, bestD := -1, 0.0
	consider := func(b int) {
		for _, i := range l.items[l.start[b]:l.start[b+1]] {
			d := l.Dist2(p, i)
			if best < 0 || d < bestD || (d == bestD && i < best) {
				best, bestD = i, d
			}
		}
	}
	for r := 0; ; r++ {
		// Ring r: the buckets whose column offset (wrapped) and row offset
		// have a larger magnitude of exactly r. Where the ring wraps all
		// the way around, a column can be visited twice; that is harmless.
		for dj := -r; dj <= r; dj++ {
			row := by + dj
			if row < 0 || row >= l.nby {
				continue
			}
			step := 2 * r
			if dj == -r || dj == r || r == 0 {
				step = 1
			}
			for di := -r; di <= r; di += step {
				consider(row*l.nbx + fmath.FloorMod(bx+di, l.nbx))
			}
		}
		colsDone := 2*r+1 >= l.nbx
		rowsDone := r >= by && r >= l.nby-1-by
		if colsDone && rowsDone {
			return best
		}
		// Every bucket not yet visited lies r+1 or more rows away, so any
		// site in it is at least r·bh away, or (while columns remain) r+1
		// or more columns away the shorter way around, at least r·bw away.
		var bound float64
		switch {
		case colsDone:
			bound = fmath.Mul(float64(r), l.bh)
		case rowsDone:
			bound = fmath.Mul(float64(r), l.bw)
		default:
			bound = fmath.Mul(float64(r), min(l.bw, l.bh))
		}
		bound -= boundMarginKm
		if best >= 0 && bound > 0 && bestD < fmath.Mul(bound, bound) {
			return best
		}
	}
}

// BruteNearest returns the index of the site in sites nearest to p by the
// same rule as Locator.Nearest, by comparing every site. It is the
// reference the bucket grid is tested against.
func BruteNearest(cyl topo.Cylinder, sites []topo.Point, p topo.Point) int {
	best, bestD := -1, 0.0
	for i, s := range sites {
		d := dist2(cyl, p, s)
		if best < 0 || d < bestD {
			best, bestD = i, d
		}
	}
	return best
}
