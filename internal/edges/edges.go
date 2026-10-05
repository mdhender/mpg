// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package edges

import (
	"cmp"
	"encoding/binary"
	"fmt"
	"math"
	"slices"

	"github.com/mdhender/mpg/internal/mesh"
)

// Water is the kind of water a cell holds, or WaterNone for land.
type Water uint8

// The water kinds. Rim cells are not described by a Water value: they are
// the polar ice sheet (mesh Cell.Rim).
const (
	WaterNone Water = iota
	Ocean
	Lake
	InlandSea
	numWaters
)

var waterNames = [numWaters]string{"", "ocean", "lake", "inland-sea"}

// String returns the water kind's name, as "inland-sea"; WaterNone is "".
func (w Water) String() string {
	if w < numWaters {
		return waterNames[w]
	}
	return fmt.Sprintf("Water(%d)", uint8(w))
}

// RiverClass is the class of the river along an edge, or RiverNone.
type RiverClass uint8

// The river classes (DESIGN.md, "Rivers on edges"). Until the river stage
// exists every edge is RiverNone.
const (
	RiverNone RiverClass = iota
	Stream
	River
	MajorRiver
	numRivers
)

var riverNames = [numRivers]string{"", "stream", "river", "major-river"}

// String returns the river class's name, as "major-river"; RiverNone is "".
func (r RiverClass) String() string {
	if r < numRivers {
		return riverNames[r]
	}
	return fmt.Sprintf("RiverClass(%d)", uint8(r))
}

// Incline is a signed grade in tenths of a percent: +150 is a climb of
// 15.0%, −150 a descent of 15.0%. Its magnitude is at most MaxIncline.
// Being an integer, it negates exactly and has no negative zero.
type Incline int16

// MaxIncline is the cap on an incline's magnitude: 100.0%.
const MaxIncline Incline = 1000

// Percent returns the grade in percent, as 15.0.
func (g Incline) Percent() float64 { return float64(g) / 10 }

// Abs returns the grade's magnitude.
func (g Incline) Abs() Incline { return max(g, -g) }

// String returns the grade with its sign and one decimal, as "+15.0%".
func (g Incline) String() string { return fmt.Sprintf("%+.1f%%", g.Percent()) }

// Grade returns the incline from a cell at altitude from (meters) to one
// at altitude to, whose sites are distKm apart: (to − from) / distance ×
// 100 percent, capped at ±100% and rounded to one decimal place, halves
// away from zero. In tenths of a percent the grade is exactly
// (to − from) / distKm (meters over kilometers), so it is one subtraction
// and one division, both correctly rounded, then a clamp and math.Round:
// the same on every machine, and Grade(b, a, d) == −Grade(a, b, d). distKm
// must be positive and the altitudes finite.
func Grade(from, to, distKm float64) Incline {
	t := (to - from) / distKm
	t = min(max(t, -float64(MaxIncline)), float64(MaxIncline))
	return Incline(math.Round(t))
}

// Edge is the game data of one undirected mesh edge, indexed by mesh edge
// id.
type Edge struct {
	// Passable is false on the rim boundary and across every edge of a rim
	// cell (mesh Mesh.Passable). The game may add rules on top.
	Passable bool
	// Coast is true when exactly one side is water and neither side is a
	// rim cell.
	Coast bool
	// Water is the kind of the water side of a coast edge, and WaterNone
	// on every other edge.
	Water Water
	// River is the class of the river along the edge. Until the river
	// stage exists it is RiverNone.
	River RiverClass
	// Incline is the grade from the mesh edge's Cells[0] to its Cells[1],
	// computed once (Grade); the half-edge of Cells[1] holds its negation.
	// It is 0 on the rim boundary.
	Incline Incline
}

// HalfEdge is one cell's side of an edge to a neighbor.
type HalfEdge struct {
	// Edge is the mesh edge id.
	Edge int
	// Neighbor is the cell across the edge.
	Neighbor int
	// Bearing is the direction from this cell's site to the neighbor's,
	// in degrees clockwise from north, in [0, 360) (topo Cylinder.Bearing,
	// across the seam the shorter way).
	Bearing float64
	// Direction is the edge's compass point in this cell (Assign). The
	// neighbor's half-edge holds its own, usually but not always the
	// opposite point.
	Direction Direction
	// Error is the angle in degrees between Bearing and Direction
	// (AngularError).
	Error float64
	// Incline is the grade from this cell to the neighbor: the edge's
	// Incline, negated when this cell is the edge's Cells[1].
	Incline Incline
}

// Data is the edge stage's product: the game data of every edge and each
// cell's half-edges.
type Data struct {
	// Edges is indexed by mesh edge id.
	Edges []Edge
	// Cells is indexed by cell id: the cell's half-edges, one per
	// neighbor, in compass order (N, NE, …, NW), so clockwise starting
	// from the edge nearest N. Edges on the rim boundary have no neighbor
	// and no half-edge.
	Cells [][]HalfEdge
}

// Toward returns cell c's half-edge in direction d, and false when c has
// no edge that way.
func (d *Data) Toward(c int, dir Direction) (HalfEdge, bool) {
	for _, h := range d.Cells[c] {
		if h.Direction == dir {
			return h, true
		}
	}
	return HalfEdge{}, false
}

// Build computes the edge data of m from the cells' altitudes (package
// cells' Stats.Altitude, the one height), each cell's water kind
// (WaterNone for land; ignored for rim cells), and each edge's river class
// (nil for none). It fails on a cell with more than MaxDegree neighbors, a
// cell that shares more than one edge with a neighbor, or two sites at the
// same place. See the package documentation for the rules.
func Build(m *mesh.Mesh, alt []float64, water []Water, river []RiverClass) (*Data, error) {
	n := len(m.Cells)
	if len(alt) != n || len(water) != n {
		return nil, fmt.Errorf("edges: %d altitudes and %d water kinds for %d cells", len(alt), len(water), n)
	}
	if river != nil && len(river) != len(m.Edges) {
		return nil, fmt.Errorf("edges: %d river classes for %d edges", len(river), len(m.Edges))
	}
	for i, a := range alt {
		if math.IsNaN(a) || math.IsInf(a, 0) {
			return nil, fmt.Errorf("edges: cell %d altitude %v", i, a)
		}
		if water[i] >= numWaters {
			return nil, fmt.Errorf("edges: cell %d water kind %d", i, water[i])
		}
	}
	cyl := m.Cylinder()
	d := &Data{Edges: make([]Edge, len(m.Edges)), Cells: make([][]HalfEdge, n)}
	for e, me := range m.Edges {
		if river != nil {
			if river[e] >= numRivers {
				return nil, fmt.Errorf("edges: edge %d river class %d", e, river[e])
			}
			d.Edges[e].River = river[e]
		}
		if me.OnBoundary() {
			continue
		}
		a, b := me.Cells[0], me.Cells[1]
		dist := cyl.Distance(m.Cells[a].Site, m.Cells[b].Site)
		if !(dist > 0) {
			return nil, fmt.Errorf("edges: edge %d joins cells %d and %d whose sites are %v km apart", e, a, b, dist)
		}
		ed := &d.Edges[e]
		ed.Passable = m.Passable(e)
		ed.Incline = Grade(alt[a], alt[b], dist)
		if !m.Cells[a].Rim && !m.Cells[b].Rim {
			wa, wb := water[a], water[b]
			switch {
			case wa == WaterNone && wb != WaterNone:
				ed.Coast, ed.Water = true, wb
			case wa != WaterNone && wb == WaterNone:
				ed.Coast, ed.Water = true, wa
			}
		}
	}
	for i, c := range m.Cells {
		var hs []HalfEdge
		for _, e := range c.Edges {
			me := &m.Edges[e]
			if me.OnBoundary() {
				continue
			}
			j := me.Other(i)
			g := d.Edges[e].Incline
			if i == me.Cells[1] {
				g = -g
			}
			hs = append(hs, HalfEdge{Edge: e, Neighbor: j, Bearing: cyl.Bearing(c.Site, m.Cells[j].Site), Incline: g})
		}
		if len(hs) != len(c.Neighbors) {
			return nil, fmt.Errorf("edges: cell %d has %d edges to %d neighbors", i, len(hs), len(c.Neighbors))
		}
		if len(hs) > MaxDegree {
			return nil, fmt.Errorf("edges: cell %d has %d neighbors, more than %d", i, len(hs), MaxDegree)
		}
		bearings := make([]float64, len(hs))
		for k, h := range hs {
			bearings[k] = h.Bearing
		}
		dirs, err := Assign(bearings)
		if err != nil {
			return nil, fmt.Errorf("cell %d: %w", i, err)
		}
		for k := range hs {
			hs[k].Direction = dirs[k]
			hs[k].Error = AngularError(hs[k].Bearing, dirs[k])
		}
		slices.SortFunc(hs, func(x, y HalfEdge) int { return cmp.Compare(x.Direction, y.Direction) })
		d.Cells[i] = hs
	}
	return d, nil
}

// AppendBinary appends the edge data's canonical encoding to b; it is the
// input to the golden hashes. Integers are little-endian uint64 (two's
// complement for the incline); floats are their IEEE 754 bits as
// little-endian uint64. In order:
//
//	number of edges                         integer
//	per edge, in id order:
//	    Passable, Coast                     one byte each, 0 or 1
//	    Water, River                        one byte each
//	    Incline                             integer
//	number of cells                         integer
//	per cell, in id order:
//	    number of half-edges                integer
//	    per half-edge, in list order:
//	        Edge, Neighbor                  integers
//	        Bearing                         float
//	        Direction                       one byte
//	        Error                           float
//	        Incline                         integer
//
// The error is always nil; the signature is encoding.BinaryAppender's.
func (d *Data) AppendBinary(b []byte) ([]byte, error) {
	i := func(v int) { b = binary.LittleEndian.AppendUint64(b, uint64(int64(v))) }
	f := func(v float64) { b = binary.LittleEndian.AppendUint64(b, math.Float64bits(v)) }
	flag := func(v bool) {
		if v {
			b = append(b, 1)
		} else {
			b = append(b, 0)
		}
	}
	i(len(d.Edges))
	for _, e := range d.Edges {
		flag(e.Passable)
		flag(e.Coast)
		b = append(b, byte(e.Water), byte(e.River))
		i(int(e.Incline))
	}
	i(len(d.Cells))
	for _, hs := range d.Cells {
		i(len(hs))
		for _, h := range hs {
			i(h.Edge)
			i(h.Neighbor)
			f(h.Bearing)
			b = append(b, byte(h.Direction))
			f(h.Error)
			i(int(h.Incline))
		}
	}
	return b, nil
}
