// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package mesh

import (
	"math"

	"github.com/mdhender/mpg/internal/fmath"
)

// Stats summarizes a mesh for logs and tuning. The mesh checks proper
// (DESIGN.md, "Mesh checks") come later; these are report-only.
type Stats struct {
	Cells, Corners, Edges int
	// BoundaryEdges counts the edges on the rim boundary.
	BoundaryEdges int
	// SeamCells counts the cells whose polygon spans x = 0.
	SeamCells int
	// NeighborMin and NeighborMax bound the neighbor counts, and
	// Neighbors[k] counts the cells with k neighbors (the last entry
	// collects any with more).
	NeighborMin, NeighborMax int
	Neighbors                [12]int
	// AreaMean is the mean cell area in km², and AreaCV the coefficient of
	// variation of the cell areas; AreaMin and AreaMax bound them.
	AreaMean, AreaCV, AreaMin, AreaMax float64
	// EdgeMin is the shortest edge in km, and ShortEdges counts the edges
	// shorter than the threshold passed to Stats.
	EdgeMin    float64
	ShortEdges int
}

// Stats returns m's summary, counting edges shorter than shortKm.
func (m *Mesh) Stats(shortKm float64) Stats {
	s := Stats{
		Cells: len(m.Cells), Corners: len(m.Corners), Edges: len(m.Edges),
		NeighborMin: math.MaxInt, AreaMin: math.Inf(1), AreaMax: math.Inf(-1), EdgeMin: math.Inf(1),
	}
	areas := make([]float64, len(m.Cells))
	var sum float64
	for i, c := range m.Cells {
		n := len(c.Neighbors)
		s.NeighborMin, s.NeighborMax = min(s.NeighborMin, n), max(s.NeighborMax, n)
		s.Neighbors[min(n, len(s.Neighbors)-1)]++
		a := m.Area(i)
		areas[i] = a
		sum += a
		s.AreaMin, s.AreaMax = min(s.AreaMin, a), max(s.AreaMax, a)
		for _, p := range m.Polygon(i) {
			if p.X < 0 || p.X >= m.cyl.W() {
				s.SeamCells++
				break
			}
		}
	}
	if len(areas) > 0 {
		s.AreaMean = sum / float64(len(areas))
		var ss float64
		for _, a := range areas {
			d := a - s.AreaMean
			ss += fmath.Mul(d, d)
		}
		s.AreaCV = math.Sqrt(ss/float64(len(areas))) / s.AreaMean
	}
	for e := range m.Edges {
		if m.Edges[e].OnBoundary() {
			s.BoundaryEdges++
		}
		l := m.EdgeLength(e)
		s.EdgeMin = min(s.EdgeMin, l)
		if l < shortKm {
			s.ShortEdges++
		}
	}
	return s
}
