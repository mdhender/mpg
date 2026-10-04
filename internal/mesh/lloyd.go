// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package mesh

import (
	"fmt"
	"math"
	"slices"

	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/internal/topo"
)

// Lloyd returns sites after passes Lloyd relaxation passes on cyl. Each
// pass builds the cylinder Voronoi diagram of the current sites and moves
// every site, rim sites included, to its cell's wrapped centroid: the
// centroid of the polygon unwrapped about the site, with x wrapped back
// into [0, W). Site i stays the site of cell i.
//
// cv[k] is the coefficient of variation of the cell areas of the diagram
// that pass k started from, so cv[0] is that of the input sites; the CV of
// the result is that of Build's mesh (Stats). With passes = 0 the result is
// a copy of sites and cv is empty.
//
// The sites must be valid input to Build. Lloyd fails, rather than return
// bad sites, on a cell with no positive finite area, on a centroid that is
// not finite or leaves (0, H), and on two relaxed sites that coincide.
func Lloyd(cyl topo.Cylinder, sites []topo.Point, passes int) (relaxed []topo.Point, cv []float64, err error) {
	if passes < 0 {
		return nil, nil, fmt.Errorf("mesh: %d Lloyd passes", passes)
	}
	h := cyl.H()
	relaxed = slices.Clone(sites)
	cv = make([]float64, 0, passes)
	areas := make([]float64, len(sites))
	var q []topo.Point
	for pass := range passes {
		rings, _, err := cellRings(cyl, relaxed)
		if err != nil {
			return nil, nil, fmt.Errorf("%w (Lloyd pass %d)", err, pass+1)
		}
		next := make([]topo.Point, len(relaxed))
		for i, ring := range rings {
			s := relaxed[i]
			q = q[:0]
			for _, sg := range ring { // the sweep's ring is unwrapped about s
				q = append(q, topo.Point{X: sg.P.X - s.X, Y: sg.P.Y - s.Y})
			}
			a, g := areaCentroid(q)
			if !(a > 0) || !finite(a) || !finite(g.X) || !finite(g.Y) {
				return nil, nil, fmt.Errorf("mesh: Lloyd pass %d: cell %d has area %v and centroid offset (%v, %v)", pass+1, i, a, g.X, g.Y)
			}
			p := topo.Point{X: cyl.WrapX(s.X + g.X), Y: s.Y + g.Y}
			if !(p.Y > 0 && p.Y < h) {
				return nil, nil, fmt.Errorf("mesh: Lloyd pass %d: cell %d's centroid y %v is outside (0, %v)", pass+1, i, p.Y, h)
			}
			areas[i] = a
			next[i] = p
		}
		_, c := meanCV(areas)
		cv = append(cv, c)
		relaxed = next
	}
	if err := checkSites(cyl, relaxed); err != nil {
		return nil, nil, fmt.Errorf("%w (after %d Lloyd passes)", err, passes)
	}
	return relaxed, cv, nil
}

// meanCV returns the mean of vs and their coefficient of variation (the
// population standard deviation over the mean), summing in index order. It
// returns zeros for no values.
func meanCV(vs []float64) (mean, cv float64) {
	if len(vs) == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range vs {
		sum += v
	}
	mean = sum / float64(len(vs))
	var ss float64
	for _, v := range vs {
		d := v - mean
		ss += fmath.Mul(d, d)
	}
	return mean, math.Sqrt(ss/float64(len(vs))) / mean
}
