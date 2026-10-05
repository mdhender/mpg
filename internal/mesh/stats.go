// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package mesh

import (
	"math"
)

// Stats summarizes a mesh for logs and tuning. It is report-only; Check
// runs the mesh checks (DESIGN.md, "Mesh checks") and carries a Stats.
type Stats struct {
	Cells, Corners, Edges int
	// BoundaryEdges counts the edges on the rim boundary.
	BoundaryEdges int
	// SeamCells counts the cells whose polygon spans x = 0.
	SeamCells int
	// NeighborMin and NeighborMax bound the neighbor counts, and
	// Neighbors[k] counts the cells with k neighbors (the last entry
	// collects any with more). RimCells counts the rim cells (Cell.Rim),
	// and PlayableNeighborMin and PlayableNeighborMax bound the neighbor
	// counts of the others.
	NeighborMin, NeighborMax                 int
	Neighbors                                [12]int
	RimCells                                 int
	PlayableNeighborMin, PlayableNeighborMax int
	// CornerCells[k] counts the corners off the rim boundary that touch k
	// cells (the last entry collects any with more), and BoundaryCorners
	// the corners on it.
	CornerCells     [7]int
	BoundaryCorners int
	// AreaMean is the mean cell area in km², and AreaCV the coefficient of
	// variation of the cell areas; AreaMin and AreaMax bound them.
	AreaMean, AreaCV, AreaMin, AreaMax float64
	// AreaDev is AreaMean's relative deviation from the province area A
	// passed to Stats, AreaMean/A − 1, and AreaMinA and AreaMaxA are
	// AreaMin and AreaMax as multiples of A. The cells tile the fixed
	// W × H cylinder, so AreaMean is W·H/n, and with n = round(W·H/A) the
	// deviation is at most about 1/(2n).
	AreaDev, AreaMinA, AreaMaxA float64
	// EdgeMin is the shortest edge in km, and ShortEdges counts the edges
	// shorter than the threshold passed to Stats.
	EdgeMin    float64
	ShortEdges int
	// Collapses, Stretches, DegreeCapHits, MaxShiftKm, and
	// StretchedEdges report the short-edge collapse (see Mesh).
	Collapses, Stretches, DegreeCapHits, StretchedEdges int
	MaxShiftKm                                          float64
}

// Stats returns m's summary for province area areaKm2 (A), counting edges
// shorter than shortKm.
func (m *Mesh) Stats(areaKm2, shortKm float64) Stats {
	s := Stats{
		Cells: len(m.Cells), Corners: len(m.Corners), Edges: len(m.Edges),
		NeighborMin: math.MaxInt, PlayableNeighborMin: math.MaxInt,
		AreaMin: math.Inf(1), AreaMax: math.Inf(-1), EdgeMin: math.Inf(1),
		Collapses: m.Collapses, Stretches: m.Stretches, DegreeCapHits: m.DegreeCapHits,
		StretchedEdges: len(m.Stretched), MaxShiftKm: m.MaxShiftKm,
	}
	for _, k := range m.Corners {
		if k.Boundary {
			s.BoundaryCorners++
		} else {
			s.CornerCells[min(len(k.Cells), len(s.CornerCells)-1)]++
		}
	}
	areas := make([]float64, len(m.Cells))
	for i, c := range m.Cells {
		n := len(c.Neighbors)
		s.NeighborMin, s.NeighborMax = min(s.NeighborMin, n), max(s.NeighborMax, n)
		s.Neighbors[min(n, len(s.Neighbors)-1)]++
		if m.InRim(i) {
			s.RimCells++
		} else {
			s.PlayableNeighborMin, s.PlayableNeighborMax = min(s.PlayableNeighborMin, n), max(s.PlayableNeighborMax, n)
		}
		a := m.Area(i)
		areas[i] = a
		s.AreaMin, s.AreaMax = min(s.AreaMin, a), max(s.AreaMax, a)
		for _, p := range m.Polygon(i) {
			if p.X < 0 || p.X >= m.cyl.W() {
				s.SeamCells++
				break
			}
		}
	}
	if len(areas) > 0 {
		s.AreaMean, s.AreaCV = meanCV(areas)
		s.AreaDev = s.AreaMean/areaKm2 - 1
		s.AreaMinA, s.AreaMaxA = s.AreaMin/areaKm2, s.AreaMax/areaKm2
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
