// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package mesh

import (
	"encoding/binary"
	"math"

	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/internal/topo"
)

// Offsets returns cell c's polygon as displacements in km from its site,
// clockwise from Corners[0]: dx wrapped the shorter way around (topo DX),
// dy unwrapped. A cell that spans the seam comes out whole.
func (m *Mesh) Offsets(c int) []topo.Point {
	cell := &m.Cells[c]
	out := make([]topo.Point, len(cell.Corners))
	for k, id := range cell.Corners {
		dx, dy := m.cyl.Delta(cell.Site, m.Corners[id].Point)
		out[k] = topo.Point{X: dx, Y: dy}
	}
	return out
}

// Polygon returns cell c's polygon in world km, unwrapped about its site:
// the site plus each of Offsets. X may fall outside [0, W) for a cell that
// spans the seam, so the polygon can be drawn or measured without special
// cases.
func (m *Mesh) Polygon(c int) []topo.Point {
	s := m.Cells[c].Site
	out := m.Offsets(c)
	for k := range out {
		out[k].X += s.X
		out[k].Y += s.Y
	}
	return out
}

// Area returns cell c's area in km², by the shoelace formula on Offsets.
func (m *Mesh) Area(c int) float64 {
	a, _ := areaCentroid(m.Offsets(c))
	return a
}

// Centroid returns cell c's centroid (center of mass) with X wrapped into
// [0, W).
func (m *Mesh) Centroid(c int) topo.Point {
	_, g := areaCentroid(m.Offsets(c))
	s := m.Cells[c].Site
	return topo.Point{X: m.cyl.WrapX(s.X + g.X), Y: s.Y + g.Y}
}

// areaCentroid returns the area of the clockwise polygon q (positive) and
// its centroid, in q's coordinates.
func areaCentroid(q []topo.Point) (area float64, centroid topo.Point) {
	var sum, cx, cy float64
	for k, a := range q {
		b := q[(k+1)%len(q)]
		cross := fmath.Mul(a.X, b.Y) - fmath.Mul(b.X, a.Y)
		sum += cross
		cx += fmath.Mul(a.X+b.X, cross)
		cy += fmath.Mul(a.Y+b.Y, cross)
	}
	if sum == 0 {
		return 0, topo.Point{}
	}
	return sum / 2, topo.Point{X: cx / (3 * sum), Y: cy / (3 * sum)}
}

// EdgeLength returns edge e's length in km, across the seam if it spans it.
func (m *Mesh) EdgeLength(e int) float64 {
	c := m.Edges[e].Corners
	return m.cyl.Distance(m.Corners[c[0]].Point, m.Corners[c[1]].Point)
}

// AppendBinary appends the mesh's canonical encoding to b. It is the input
// to the mesh's golden hashes, and it holds the graph only (not
// GhostMarginKm). Integers are little-endian uint64 (ids two's complement,
// so Boundary is all ones); floats are their IEEE 754 bits
// (math.Float64bits) as little-endian uint64. In order:
//
//	W, H                                              floats
//	len(Cells), len(Corners), len(Edges)              integers
//	per cell, in id order:
//	    Site.X, Site.Y                                floats
//	    len(Corners), then each corner id             integers
//	    each edge id (as many as corners)             integers
//	    len(Neighbors), then each neighbor id         integers
//	per corner, in id order:
//	    Point.X, Point.Y                              floats
//	    Boundary                                      integer, 0 or 1
//	    len(Cells), then each cell id                 integers
//	    len(Edges), then each edge id                 integers
//	per edge, in id order:
//	    Cells[0], Cells[1], Corners[0], Corners[1]    integers
//
// The error is always nil; the signature is encoding.BinaryAppender's.
func (m *Mesh) AppendBinary(b []byte) ([]byte, error) {
	f := func(v float64) { b = binary.LittleEndian.AppendUint64(b, math.Float64bits(v)) }
	i := func(v int) { b = binary.LittleEndian.AppendUint64(b, uint64(int64(v))) }
	ints := func(vs []int) {
		i(len(vs))
		for _, v := range vs {
			i(v)
		}
	}
	f(m.cyl.W())
	f(m.cyl.H())
	i(len(m.Cells))
	i(len(m.Corners))
	i(len(m.Edges))
	for _, c := range m.Cells {
		f(c.Site.X)
		f(c.Site.Y)
		ints(c.Corners)
		for _, e := range c.Edges {
			i(e)
		}
		ints(c.Neighbors)
	}
	for _, c := range m.Corners {
		f(c.Point.X)
		f(c.Point.Y)
		if c.Boundary {
			i(1)
		} else {
			i(0)
		}
		ints(c.Cells)
		ints(c.Edges)
	}
	for _, e := range m.Edges {
		i(e.Cells[0])
		i(e.Cells[1])
		i(e.Corners[0])
		i(e.Corners[1])
	}
	return b, nil
}
