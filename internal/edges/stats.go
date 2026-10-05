// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package edges

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mdhender/mpg/internal/mesh"
)

// GradeBreaks are the upper bounds, in tenths of a percent, of the incline
// histogram's buckets of |grade|: [0, 1%), [1, 2%), [2, 5%), [5, 10%),
// [10, 20%), [20, 50%), [50, 100%), and a last bucket for grades at the
// 100% cap.
var GradeBreaks = []Incline{10, 20, 50, 100, 200, 500, MaxIncline}

// GradeBucketNames names the incline histogram's buckets.
var GradeBucketNames = []string{"0-1", "1-2", "2-5", "5-10", "10-20", "20-50", "50-100", "cap"}

// gradeBucket returns the histogram bucket of |g|.
func gradeBucket(g Incline) int {
	a := g.Abs()
	for k, hi := range GradeBreaks {
		if a < hi {
			return k
		}
	}
	return len(GradeBreaks)
}

// Stats summarizes the edge data of a mesh for the logs and the
// playability measures.
type Stats struct {
	// Playable and LandCells count the non-rim cells and the land cells.
	Playable, LandCells int
	// Neighbors[k] counts the playable cells with k neighbors.
	Neighbors [MaxDegree + 1]int

	// HalfEdges counts the half-edges of playable cells, and ErrorMean,
	// ErrorP95 (nearest rank), and ErrorMax summarize their angular errors
	// in degrees.
	HalfEdges                     int
	ErrorMean, ErrorP95, ErrorMax float64
	// Pairs counts the edges between two playable cells, and NotOpposite
	// those whose two directions are not opposite compass points.
	Pairs, NotOpposite int
	// NaiveCollisions counts the playable cells in which labelling each
	// edge with its nearest compass point (Nearest) would repeat a point.
	NaiveCollisions int

	// GradeAll and GradeLand are incline histograms (buckets of |grade|,
	// see GradeBreaks) over the edges between two playable cells and over
	// the edges between two land cells. MaxLandGrade is the steepest
	// land–land grade's magnitude.
	GradeAll, GradeLand [8]int
	MaxLandGrade        Incline

	// Coast counts the coast edges; LandRim the edges between a land cell
	// and a rim cell, which the rim falloff should prevent (they are not
	// coasts).
	Coast, LandRim int
	// Passable, Impassable, and Boundary count the edges: Impassable
	// includes the Boundary edges on the rim boundary.
	Passable, Impassable, Boundary int
}

// Summarize computes the statistics of d, the edge data of m with the
// cells' water kinds water (as given to Build).
func Summarize(m *mesh.Mesh, d *Data, water []Water) Stats {
	var s Stats
	land := func(i int) bool { return !m.Cells[i].Rim && water[i] == WaterNone }
	var errs []float64
	var sum float64
	for i, c := range m.Cells {
		if c.Rim {
			continue
		}
		s.Playable++
		if land(i) {
			s.LandCells++
		}
		hs := d.Cells[i]
		s.Neighbors[min(len(hs), MaxDegree)]++
		var seen [numDirections]bool
		collide := false
		for _, h := range hs {
			errs = append(errs, h.Error)
			sum += h.Error
			p := Nearest(h.Bearing)
			collide = collide || seen[p]
			seen[p] = true
		}
		if collide {
			s.NaiveCollisions++
		}
	}
	s.HalfEdges = len(errs)
	if len(errs) > 0 {
		slices.Sort(errs)
		s.ErrorMean = sum / float64(len(errs))
		s.ErrorP95 = errs[(95*len(errs)+99)/100-1]
		s.ErrorMax = errs[len(errs)-1]
	}
	for e, me := range m.Edges {
		ed := d.Edges[e]
		if ed.Passable {
			s.Passable++
		} else {
			s.Impassable++
		}
		if ed.Coast {
			s.Coast++
		}
		if me.OnBoundary() {
			s.Boundary++
			continue
		}
		a, b := me.Cells[0], me.Cells[1]
		ra, rb := m.Cells[a].Rim, m.Cells[b].Rim
		if (ra && land(b)) || (rb && land(a)) {
			s.LandRim++
		}
		if ra || rb {
			continue
		}
		s.Pairs++
		da, _ := direction(d, a, e)
		db, _ := direction(d, b, e)
		if db != da.Opposite() {
			s.NotOpposite++
		}
		s.GradeAll[gradeBucket(ed.Incline)]++
		if land(a) && land(b) {
			s.GradeLand[gradeBucket(ed.Incline)]++
			s.MaxLandGrade = max(s.MaxLandGrade, ed.Incline.Abs())
		}
	}
	return s
}

// direction returns cell c's direction along edge e.
func direction(d *Data, c, e int) (Direction, bool) {
	for _, h := range d.Cells[c] {
		if h.Edge == e {
			return h.Direction, true
		}
	}
	return 0, false
}

// pct returns 100·a/b, or 0 when b is 0.
func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return 100 * float64(a) / float64(b)
}

// histogram formats counts against bucket names as "0-1 12 (3.4%), …".
func histogram(names []string, counts []int) string {
	total := 0
	for _, c := range counts {
		total += c
	}
	parts := make([]string, len(counts))
	for k, c := range counts {
		parts[k] = fmt.Sprintf("%s %d (%.1f%%)", names[k], c, pct(c, total))
	}
	return strings.Join(parts, ", ")
}

// Lines returns the statistics as log lines.
func (s Stats) Lines() []string {
	var nb []string
	for k, c := range s.Neighbors {
		if c > 0 {
			nb = append(nb, fmt.Sprintf("%d:%d", k, c))
		}
	}
	return []string{
		fmt.Sprintf("%d playable cells (%d land); neighbors %s", s.Playable, s.LandCells, strings.Join(nb, " ")),
		fmt.Sprintf("direction error over %d half-edges: mean %.2f°, p95 %.2f°, max %.2f°; reverse not opposite on %d of %d edges (%.1f%%)",
			s.HalfEdges, s.ErrorMean, s.ErrorP95, s.ErrorMax, s.NotOpposite, s.Pairs, pct(s.NotOpposite, s.Pairs)),
		fmt.Sprintf("nearest-point labels would repeat in %d of %d playable cells (%.1f%%)",
			s.NaiveCollisions, s.Playable, pct(s.NaiveCollisions, s.Playable)),
		fmt.Sprintf("|grade| %% land-land: %s; max %s", histogram(GradeBucketNames, s.GradeLand[:]), s.MaxLandGrade),
		fmt.Sprintf("|grade| %% all playable: %s", histogram(GradeBucketNames, s.GradeAll[:])),
		fmt.Sprintf("%d coast edges (%.2f per land cell); %d land-rim edges; %d passable, %d impassable (%d on the rim boundary)",
			s.Coast, s.CoastPerLand(), s.LandRim, s.Passable, s.Impassable, s.Boundary),
	}
}

// CoastPerLand returns the coast edges per land cell.
func (s Stats) CoastPerLand() float64 {
	if s.LandCells == 0 {
		return 0
	}
	return float64(s.Coast) / float64(s.LandCells)
}
