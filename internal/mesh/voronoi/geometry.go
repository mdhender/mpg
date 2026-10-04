// MIT License: See https://github.com/pzsz/voronoi/LICENSE.md

// Author: Przemyslaw Szczepaniak (przeszczep@gmail.com)
// Port of Raymond Hill's (rhill@raymondhill.net) javascript implementation
// of Steven Forune's algorithm to compute Voronoi diagrams

package voronoi

import (
	"math"

	"github.com/mdhender/mpg/internal/fmath"
)

// Vertex on 2D plane
type Vertex struct {
	X float64
	Y float64
}

// Vertex representing lack of vertex (or bad vertex)
var NO_VERTEX = Vertex{math.Inf(1), math.Inf(1)}

// EdgeVertex is an end point of an Edge. (Upstream also linked each vertex
// to the edges that share it, through a map keyed by the vertex; this copy
// does not, so no result passes through map iteration.)
type EdgeVertex struct {
	Vertex
}

// Edge structure
type Edge struct {
	// Cell on the left
	LeftCell *Cell
	// Cell on the right
	RightCell *Cell
	// Start Vertex
	Va EdgeVertex
	// End Vertex
	Vb EdgeVertex
}

func (e *Edge) GetOtherCell(cell *Cell) *Cell {
	if cell == e.LeftCell {
		return e.RightCell
	} else if cell == e.RightCell {
		return e.LeftCell
	}
	return nil
}

func (e *Edge) GetOtherEdgeVertex(v Vertex) EdgeVertex {
	if v == e.Va.Vertex {
		return e.Vb
	} else if v == e.Vb.Vertex {
		return e.Va
	}
	return EdgeVertex{NO_VERTEX}
}

func newEdge(LeftCell, RightCell *Cell) *Edge {
	return &Edge{
		LeftCell:  LeftCell,
		RightCell: RightCell,
		Va:        EdgeVertex{NO_VERTEX},
		Vb:        EdgeVertex{NO_VERTEX},
	}
}

// Halfedge (directed edge)
type Halfedge struct {
	Cell  *Cell
	Edge  *Edge
	Angle float64
}

func newHalfedge(edge *Edge, LeftCell, RightCell *Cell) *Halfedge {
	ret := &Halfedge{
		Cell: LeftCell,
		Edge: edge,
	}

	// 'angle' is a value to be used for properly sorting the
	// halfsegments counterclockwise. By convention, we will
	// use the angle of the line defined by the 'site to the left'
	// to the 'site to the right'.
	// However, border edges have no 'site to the right': thus we
	// use the angle of line perpendicular to the halfsegment (the
	// edge should have both end points defined in such case.)
	if RightCell != nil {
		ret.Angle = fmath.Atan2(RightCell.Site.Y-LeftCell.Site.Y, RightCell.Site.X-LeftCell.Site.X)
	} else {
		va := edge.Va
		vb := edge.Vb
		// rhill 2011-05-31: used to call GetStartpoint()/GetEndpoint(),
		// but for performance purpose, these are expanded in place here.
		if edge.LeftCell == LeftCell {
			ret.Angle = fmath.Atan2(vb.X-va.X, va.Y-vb.Y)
		} else {
			ret.Angle = fmath.Atan2(va.X-vb.X, vb.Y-va.Y)
		}
	}
	return ret
}

func (h *Halfedge) GetStartpoint() Vertex {
	if h.Edge.LeftCell == h.Cell {
		return h.Edge.Va.Vertex
	}
	return h.Edge.Vb.Vertex

}

func (h *Halfedge) GetEndpoint() Vertex {
	if h.Edge.LeftCell == h.Cell {
		return h.Edge.Vb.Vertex
	}
	return h.Edge.Va.Vertex
}
