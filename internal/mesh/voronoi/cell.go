// Copyright 2013 Przemyslaw Szczepaniak.
// MIT License: See https://github.com/gorhill/Javascript-Voronoi/LICENSE.md

// Author: Przemyslaw Szczepaniak (przeszczep@gmail.com)
// Port of Raymond Hill's (rhill@raymondhill.net) javascript implementation
// of Steven  Forune's algorithm to compute Voronoi diagrams

package voronoi

import (
	"cmp"
	"slices"
)

// Cell of voronoi diagram
type Cell struct {
	// Site of the cell
	Site Vertex
	// Index is the site's position in the slice passed to ComputeDiagram.
	Index int
	// Array of halfedges sorted counterclockwise
	Halfedges []*Halfedge
}

func newCell(site Vertex, index int) *Cell {
	return &Cell{Site: site, Index: index}
}

func (t *Cell) prepare() int {
	halfedges := t.Halfedges
	iHalfedge := len(halfedges) - 1

	// get rid of unused halfedges
	// rhill 2011-05-27: Keep it simple, no point here in trying
	// to be fancy: dangling edges are a typically a minority.
	for ; iHalfedge >= 0; iHalfedge-- {
		edge := halfedges[iHalfedge].Edge

		if edge.Vb.Vertex == NO_VERTEX || edge.Va.Vertex == NO_VERTEX {
			halfedges[iHalfedge] = halfedges[len(halfedges)-1]
			halfedges = halfedges[:len(halfedges)-1]
		}
	}

	// Descending angle. The sort is stable, so equal angles keep the order
	// the sweep created them in, whatever the sort algorithm.
	slices.SortStableFunc(halfedges, func(a, b *Halfedge) int { return cmp.Compare(b.Angle, a.Angle) })
	t.Halfedges = halfedges
	return len(halfedges)
}
