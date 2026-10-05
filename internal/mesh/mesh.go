// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package mesh

import (
	"fmt"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/topo"
)

// Boundary stands for the rim boundary in Edge.Cells: the far side of an
// edge that lies on the north (y = 0) or south (y = H) edge of the map.
const Boundary = -1

// Cell is one province: the part of the cylinder closer to its site than to
// any other, clipped to 0 ≤ y ≤ H.
type Cell struct {
	// Site is the cell's site, with X in [0, W) and Y in (0, H).
	Site topo.Point
	// Corners lists the cell's corner ids clockwise on the map (north up),
	// starting at the lowest id.
	Corners []int
	// Edges lists the cell's edge ids: Edges[k] joins Corners[k] and
	// Corners[k+1] (cyclically).
	Edges []int
	// Neighbors lists the ids of the cells that share an edge with this
	// one, ascending, without duplicates. The rim boundary is not a
	// neighbor.
	Neighbors []int
	// Rim is true for a rim cell: one whose site lies in the north or south
	// rim band (topo Cylinder.InRim of Site.Y). Rim cells are the polar ice
	// sheet: they hold every edge on the rim boundary, and no other cell
	// touches the map's north or south edge (Check verifies it).
	Rim bool
	// Impassable is true for a cell the game may not enter. Only rim cells
	// are impassable, so it equals Rim.
	Impassable bool
}

// Corner is a point where cell polygons meet.
type Corner struct {
	// Point is the corner's position, with X in [0, W) and Y in [0, H].
	Point topo.Point
	// Cells lists the ids of the cells whose polygons pass through the
	// corner, ascending.
	Cells []int
	// Edges lists the ids of the edges that end at the corner, ascending.
	Edges []int
	// Boundary is true when the corner lies on the north (y = 0) or south
	// (y = H) edge of the map.
	Boundary bool
}

// Edge is a side of a cell polygon, stored once. It separates two cells, or
// a rim cell from the rim boundary.
type Edge struct {
	// Cells holds the cells on either side with Cells[0] < Cells[1], or
	// Cells[1] == Boundary for an edge on y = 0 or y = H.
	Cells [2]int
	// Corners holds the ends of the edge in the clockwise order of
	// Cells[0]'s polygon, so Cells[0] lies to the right of the walk from
	// Corners[0] to Corners[1] (north up).
	Corners [2]int
}

// OnBoundary reports whether the edge lies on the rim boundary.
func (e *Edge) OnBoundary() bool { return e.Cells[1] == Boundary }

// Other returns the cell across the edge from cell c: the other entry of
// Cells, which is Boundary for c's side of a boundary edge. It returns
// Boundary as well when c is not on the edge.
func (e *Edge) Other(c int) int {
	switch c {
	case e.Cells[0]:
		return e.Cells[1]
	case e.Cells[1]:
		return e.Cells[0]
	}
	return Boundary
}

// Mesh is the province graph on a cylinder: cells, corners, and edges, with
// stable ids. Cell i is the cell of site i; corners are numbered north to
// south, then west to east; edges by their corner ids. See the package
// documentation.
type Mesh struct {
	cyl topo.Cylinder

	Cells   []Cell
	Corners []Corner
	Edges   []Edge

	// GhostMarginKm is the width of the ghost band the sweep needed (see
	// the package documentation). It is a diagnostic, not part of the
	// graph.
	GhostMarginKm float64

	// LloydCV holds the coefficient of variation of the cell areas before
	// each Lloyd pass New ran: LloydCV[0] is the unrelaxed sites'. The
	// mesh's own CV is in Stats. It is a diagnostic, not part of the graph.
	LloydCV []float64

	// Collapses, Stretches, and DegreeCapHits count what Collapse did:
	// short edges collapsed into 4-way corners (the degree cap's
	// included), short edges stretched to the minimum instead, and
	// collapses made by the degree cap. MaxShiftKm is the farthest any
	// Voronoi corner moved, and Stretched lists the stretched edges,
	// ascending. They are diagnostics, not part of the graph.
	Collapses, Stretches, DegreeCapHits int
	MaxShiftKm                          float64
	Stretched                           []int
}

// Cylinder returns the cylinder the mesh lies on.
func (m *Mesh) Cylinder() topo.Cylinder { return m.cyl }

// New builds cfg's mesh: the sites from the "mesh" seed stream, relaxed by
// cfg.Mesh.LloydPasses Lloyd passes, the cylinder Voronoi graph over them,
// and the short-edge collapse and degree cap (Collapse) at
// cfg.Mesh.MinEdgeKm and cfg.Mesh.DegreeCap. cfg must be resolved.
func New(cfg config.Config) (*Mesh, error) {
	cyl, err := topo.New(cfg.World.WidthKm, cfg.World.HeightKm, cfg.Rim.Km, cfg.Rim.FalloffKm)
	if err != nil {
		return nil, fmt.Errorf("mesh: %w", err)
	}
	sites, err := Sites(cfg, cyl)
	if err != nil {
		return nil, err
	}
	relaxed, cv, err := Lloyd(cyl, sites, cfg.Mesh.LloydPasses)
	if err != nil {
		return nil, err
	}
	m, err := Build(cyl, relaxed)
	if err != nil {
		return nil, err
	}
	m.LloydCV = cv
	return Collapse(m, cfg.Mesh.MinEdgeKm, cfg.Mesh.DegreeCap)
}
