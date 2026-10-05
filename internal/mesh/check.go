// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package mesh

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mdhender/mpg/internal/config"
)

// Limits are the bounds the mesh checks hold a mesh to.
type Limits struct {
	// AreaKm2 is the province area A.
	AreaKm2 float64
	// MinEdgeKm is the shortest edge allowed (min_edge_km).
	MinEdgeKm float64
	// AreaMin and AreaMax bound a cell's area as multiples of A.
	AreaMin, AreaMax float64
	// DegreeCap is the most neighbors a cell may have.
	DegreeCap int
}

// LimitsOf returns the limits a resolved config sets.
func LimitsOf(cfg config.Config) Limits {
	return Limits{
		AreaKm2:   cfg.Province.AreaKm2,
		MinEdgeKm: cfg.Mesh.MinEdgeKm,
		AreaMin:   cfg.Mesh.AreaMin,
		AreaMax:   cfg.Mesh.AreaMax,
		DegreeCap: cfg.Mesh.DegreeCap,
	}
}

// The mesh checks, in the order Check runs them. Each names the kind of id
// it reports in CheckResult.First.
const (
	CheckEdgeLength = "edge-length" // edges: every edge at least MinEdgeKm
	CheckArea       = "area"        // cells: area within [AreaMin·A, AreaMax·A]
	CheckNeighbors  = "neighbors"   // cells: 3 to DegreeCap neighbors, rim cells 1 to DegreeCap
	CheckCorners    = "corners"     // corners: 3–4 cells, or 2–4 on the rim boundary
	CheckRimFlags   = "rim-flags"   // cells: Rim is InRim of the site, Impassable is Rim
	CheckRimSeal    = "rim-seal"    // cells: no playable cell touches the rim boundary
)

// firstIDs is how many offending ids a CheckResult keeps.
const firstIDs = 5

// CheckResult is the outcome of one mesh check.
type CheckResult struct {
	// Name is the check's name (CheckEdgeLength and so on).
	Name string
	// Rule states what the check requires, with the limits filled in.
	Rule string
	// Kind is what First holds: "cell", "corner", or "edge" ids.
	Kind string
	// Bad counts the offenders, and First lists the lowest few of their
	// ids, ascending.
	Bad   int
	First []int
}

// Pass reports whether the check found no offender.
func (r CheckResult) Pass() bool { return r.Bad == 0 }

func (r *CheckResult) fail(id int) {
	r.Bad++
	if len(r.First) < firstIDs {
		r.First = append(r.First, id)
	}
}

// Report is the result of the mesh checks (DESIGN.md, "Mesh checks") with
// the numbers a milestone report quotes. Stats holds the general summary at
// the limits' A and MinEdgeKm.
type Report struct {
	Stats
	Limits Limits
	// Checks holds every check's result, in a fixed order.
	Checks []CheckResult
	// PlayableCells counts the cells off the rim (RimCells, in Stats,
	// counts the rest).
	PlayableCells int
	// PlayableNeighbors[k] and RimNeighbors[k] count the playable and rim
	// cells with k neighbors; the last entry collects any with more.
	PlayableNeighbors, RimNeighbors [10]int
	// BoundaryCornerCells[k] counts the corners on the rim boundary that
	// touch k cells (the last entry collects any with more); CornerCells,
	// in Stats, counts the others.
	BoundaryCornerCells [7]int
	// RimDepthMin and RimDepthMax bound how many cells deep the rim is
	// where it meets the playable cells. A rim cell's depth is 1 if it
	// touches the map's north or south edge, else one more than its
	// shallowest rim neighbor's (counting through rim cells only); a
	// playable cell beside the rim sees ice as deep as its shallowest rim
	// neighbor, and these are the least and the greatest of that over the
	// playable cells beside the rim. Both are 0 when none borders it.
	RimDepthMin, RimDepthMax int
}

// OK reports whether every check passed.
func (r *Report) OK() bool {
	for _, c := range r.Checks {
		if !c.Pass() {
			return false
		}
	}
	return true
}

// Err returns nil if every check passed, or an error naming each failed
// check with its rule, its offender count, and the first offending ids.
func (r *Report) Err() error {
	var errs []error
	for _, c := range r.Checks {
		if !c.Pass() {
			errs = append(errs, fmt.Errorf("mesh check %s failed: %d %ss break %q (first %v)", c.Name, c.Bad, c.Kind, c.Rule, c.First))
		}
	}
	return errors.Join(errs...)
}

// Lines returns a short text report: one line per check, then the
// measured numbers.
func (r *Report) Lines() []string {
	var out []string
	for _, c := range r.Checks {
		if c.Pass() {
			out = append(out, fmt.Sprintf("check %-11s pass  %s", c.Name, c.Rule))
		} else {
			out = append(out, fmt.Sprintf("check %-11s FAIL  %s: %d %ss, first %v", c.Name, c.Rule, c.Bad, c.Kind, c.First))
		}
	}
	a := r.Limits.AreaKm2
	out = append(out,
		fmt.Sprintf("cells %d (%d playable, %d rim); area mean/A %.6f, CV %.4f, min %.3f A, max %.3f A",
			r.Cells, r.PlayableCells, r.RimCells, r.AreaMean/a, r.AreaCV, r.AreaMinA, r.AreaMaxA),
		fmt.Sprintf("shortest edge %.3f km (min_edge_km %.3f); %d collapsed, %d stretched, %d degree-cap hits; corners moved up to %.2f km",
			r.EdgeMin, r.Limits.MinEdgeKm, r.Collapses, r.Stretches, r.DegreeCapHits, r.MaxShiftKm),
		"neighbors playable "+histogram(r.PlayableNeighbors[:], 3)+"; rim "+histogram(r.RimNeighbors[:], 1),
		"corners touching cells "+histogram(r.CornerCells[:], 3)+"; on the map edge "+histogram(r.BoundaryCornerCells[:], 2),
		fmt.Sprintf("rim depth %d to %d cells where it meets the playable cells", r.RimDepthMin, r.RimDepthMax),
	)
	return out
}

// histogram formats h as "k:count" pairs, from index lo (or the lowest
// non-zero index below it) to the highest non-zero index, with the last
// index of h written "k+".
func histogram(h []int, lo int) string {
	first, last := lo, lo
	for k, n := range h {
		if n != 0 {
			first, last = min(first, k), max(last, k)
		}
	}
	parts := make([]string, 0, last-first+1)
	for k := first; k <= last; k++ {
		label := fmt.Sprint(k)
		if k == len(h)-1 {
			label += "+"
		}
		parts = append(parts, fmt.Sprintf("%s:%d", label, h[k]))
	}
	return strings.Join(parts, " ")
}

// Check runs the mesh checks against l and returns the report:
//
//   - edge-length: no edge shorter than l.MinEdgeKm;
//   - area: every cell's area within [l.AreaMin·A, l.AreaMax·A];
//   - neighbors: every playable cell has 3 to l.DegreeCap neighbors, and
//     every rim cell 1 to l.DegreeCap (a rim cell against the map edge can
//     have only one or two, straight from the Voronoi diagram);
//   - corners: every corner touches 3 or 4 cells, or 2 to 4 on the rim
//     boundary;
//   - rim-flags: each cell's Rim is topo InRim of its site's y, and
//     Impassable equals Rim;
//   - rim-seal: no playable cell has a corner on the rim boundary, so every
//     boundary edge belongs to a rim cell and the rim separates every
//     playable cell from the map's north and south edges.
//
// It also measures the rim's depth in cells (Report.RimDepthMin).
func (m *Mesh) Check(l Limits) Report {
	r := Report{Stats: m.Stats(l.AreaKm2, l.MinEdgeKm), Limits: l}
	edge := CheckResult{Name: CheckEdgeLength, Kind: "edge", Rule: fmt.Sprintf("every edge at least %.3f km", l.MinEdgeKm)}
	area := CheckResult{Name: CheckArea, Kind: "cell", Rule: fmt.Sprintf("every cell area within %g A to %g A", l.AreaMin, l.AreaMax)}
	nbs := CheckResult{Name: CheckNeighbors, Kind: "cell", Rule: fmt.Sprintf("playable cells 3 to %d neighbors, rim cells 1 to %d", l.DegreeCap, l.DegreeCap)}
	corners := CheckResult{Name: CheckCorners, Kind: "corner", Rule: "corners touch 3 to 4 cells, 2 to 4 on the map edge"}
	flags := CheckResult{Name: CheckRimFlags, Kind: "cell", Rule: "rim cells are those with sites in the rim band, and only they are impassable"}
	seal := CheckResult{Name: CheckRimSeal, Kind: "cell", Rule: "no playable cell touches the map's north or south edge"}

	for e := range m.Edges {
		if !(m.EdgeLength(e) >= l.MinEdgeKm) {
			edge.fail(e)
		}
	}
	lo, hi := l.AreaMin*l.AreaKm2, l.AreaMax*l.AreaKm2
	for i, c := range m.Cells {
		if a := m.Area(i); !(a >= lo && a <= hi) {
			area.fail(i)
		}
		n := len(c.Neighbors)
		if c.Rim {
			r.RimNeighbors[min(n, len(r.RimNeighbors)-1)]++
			if n < 1 || n > l.DegreeCap {
				nbs.fail(i)
			}
		} else {
			r.PlayableCells++
			r.PlayableNeighbors[min(n, len(r.PlayableNeighbors)-1)]++
			if n < 3 || n > l.DegreeCap {
				nbs.fail(i)
			}
			for _, k := range c.Corners {
				if m.Corners[k].Boundary {
					seal.fail(i)
					break
				}
			}
		}
		if c.Rim != m.cyl.InRim(c.Site.Y) || c.Impassable != c.Rim {
			flags.fail(i)
		}
	}
	for k, c := range m.Corners {
		n := len(c.Cells)
		lo := 3
		if c.Boundary {
			lo = 2
			r.BoundaryCornerCells[min(n, len(r.BoundaryCornerCells)-1)]++
		}
		if n < lo || n > 4 {
			corners.fail(k)
		}
	}
	r.RimDepthMin, r.RimDepthMax = m.rimDepth()
	r.Checks = []CheckResult{edge, area, nbs, corners, flags, seal}
	return r
}

// rimDepth returns the least and greatest rim depth, in cells, beside a
// playable cell (see Report.RimDepthMin): a breadth-first search through
// rim cells only, from the rim cells with a corner on the rim boundary
// (depth 1).
func (m *Mesh) rimDepth() (lo, hi int) {
	depth := make([]int, len(m.Cells))
	var queue []int
	for i, c := range m.Cells {
		if !c.Rim {
			continue
		}
		for _, k := range c.Corners {
			if m.Corners[k].Boundary {
				depth[i] = 1
				queue = append(queue, i)
				break
			}
		}
	}
	for len(queue) > 0 {
		i := queue[0]
		queue = queue[1:]
		for _, j := range m.Cells[i].Neighbors {
			if m.Cells[j].Rim && depth[j] == 0 {
				depth[j] = depth[i] + 1
				queue = append(queue, j)
			}
		}
	}
	for _, c := range m.Cells {
		if c.Rim {
			continue
		}
		near := 0 // the shallowest rim cell beside this one
		for _, j := range c.Neighbors {
			if d := depth[j]; m.Cells[j].Rim && d > 0 && (near == 0 || d < near) {
				near = d
			}
		}
		if near > 0 {
			if lo == 0 || near < lo {
				lo = near
			}
			hi = max(hi, near)
		}
	}
	return lo, hi
}
