// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package river

import (
	"encoding/binary"
	"math"

	"github.com/mdhender/mpg/internal/mesh"
)

// End returns the corner where water from corner k stops: the end of its
// path down the tree.
func (t *Tree) End(k int) int {
	for t.Down[k] != None {
		k = t.Down[k]
	}
	return k
}

// destOf returns what terminal corner k ends water in.
func (t *Tree) destOf(k int) Dest {
	switch t.Terminal[k] {
	case Ocean:
		return Dest{Ocean, None}
	case Lake:
		return Dest{Lake, t.Water[k]}
	}
	return Dest{t.Terminal[k], k}
}

// Dest returns where water from corner k ends: the sea, a lake, or a dry
// sink.
func (t *Tree) Dest(k int) Dest { return t.destOf(t.End(k)) }

// Final returns where water from corner k finally rests: Dest, followed
// through every overflowing lake it reaches to the sea, a closed lake, or a
// dry sink.
func (t *Tree) Final(k int) Dest {
	d := t.Dest(k)
	for n := 0; d.Kind == Lake && n <= len(t.Lakes); n++ {
		out := &t.Lakes[d.ID]
		switch out.Drain {
		case Closed:
			return d
		case DirectSea:
			return Dest{Ocean, None}
		case DirectLake:
			d = Dest{Lake, out.Into}
		case Outlet:
			d = t.Dest(out.Corner)
		}
	}
	return d
}

// Lowest returns cell c's lowest corner by height, ties to the lower id:
// the corner its runoff enters the tree at.
func (t *Tree) Lowest(m *mesh.Mesh, c int) int {
	low := None
	for _, k := range m.Cells[c].Corners {
		if low == None || t.Height[k] < t.Height[low] || t.Height[k] == t.Height[low] && k < low {
			low = k
		}
	}
	return low
}

// agreement compares the tree's catchments with the input's, cell by cell
// in id order.
func (t *Tree) agreement(in Input) Agreement {
	var a Agreement
	m := in.Mesh
	for c, land := range in.Land {
		if !land {
			continue
		}
		k := t.Lowest(m, c)
		area := m.Area(c)
		a.Cells++
		a.AreaKm2 += area
		if t.Dest(k) != in.Catchment[c] {
			a.Differ++
			a.DifferKm2 += area
		}
		if t.Final(k) != in.FinalCatchment[c] {
			a.FinalDiffer++
			a.FinalDifferKm2 += area
		}
	}
	return a
}

// Stats counts what a tree holds, for the logs and measures.
type Stats struct {
	// LandCorners counts the corners in the graph, and Terminals them by
	// kind (Interior: the corners that are not terminal).
	LandCorners int
	Terminals   [4]int
	// Edges counts the tree's edges; Seam those crossing the east–west
	// seam; Flat those between corners at one flood level; Climb those
	// rising from a corner to a higher one (out of a pit).
	Edges, Seam, Flat, Climb int
	// Overflowing counts the lakes that overflow, Drains them by drain,
	// AtSpill the outlets at the spill corner, and Fallbacks those chosen
	// by the fallback.
	Overflowing, AtSpill, Fallbacks int
	Drains                          [4]int
}

// Stats returns the tree's statistics on mesh m.
func (t *Tree) Stats(m *mesh.Mesh) Stats {
	var s Stats
	half := m.Cylinder().W() / 2
	for k, land := range t.Land {
		if !land {
			continue
		}
		s.LandCorners++
		s.Terminals[t.Terminal[k]]++
		d := t.Down[k]
		if d == None {
			continue
		}
		s.Edges++
		if math.Abs(m.Corners[k].Point.X-m.Corners[d].Point.X) > half {
			s.Seam++
		}
		if t.Level[k] == t.Level[d] {
			s.Flat++
		}
		if t.Height[d] > t.Height[k] {
			s.Climb++
		}
	}
	for _, l := range t.Lakes {
		if l.Drain == Closed {
			continue
		}
		s.Overflowing++
		s.Drains[l.Drain]++
		if l.AtSpill {
			s.AtSpill++
		}
		if l.Fallback {
			s.Fallbacks++
		}
	}
	return s
}

// AppendBinary appends the tree's canonical encoding to b, for the golden
// hashes: integers as int64 and floats as their IEEE 754 bits, all
// little-endian, in this order:
//
//	number of corners                          integer
//	per corner: land flag                      integer (0 or 1)
//	  for a land corner: Terminal, Water,      integers
//	  Level                                    float
//	  Down, DownEdge                           integers
//	number of corners in Order, then each      integers
//	number of lakes                            integer
//	per lake: Drain, Corner, Into, AtSpill,    integers
//	  Fallback
//	Fallbacks                                  integer
//	with an agreement: Cells, Differ,          integers
//	  FinalDiffer
//	  AreaKm2, DifferKm2, FinalDifferKm2       floats
//
// Height is left out: it is the cells' altitudes, hashed upstream. The
// error is always nil; the signature is encoding.BinaryAppender's.
func (t *Tree) AppendBinary(b []byte) ([]byte, error) {
	fl := func(v float64) { b = binary.LittleEndian.AppendUint64(b, math.Float64bits(v)) }
	in := func(v int) { b = binary.LittleEndian.AppendUint64(b, uint64(int64(v))) }
	bl := func(v bool) {
		if v {
			in(1)
		} else {
			in(0)
		}
	}
	in(len(t.Land))
	for k, land := range t.Land {
		bl(land)
		if !land {
			continue
		}
		in(int(t.Terminal[k]))
		in(t.Water[k])
		fl(t.Level[k])
		in(t.Down[k])
		in(t.DownEdge[k])
	}
	in(len(t.Order))
	for _, k := range t.Order {
		in(k)
	}
	in(len(t.Lakes))
	for _, l := range t.Lakes {
		in(int(l.Drain))
		in(l.Corner)
		in(l.Into)
		bl(l.AtSpill)
		bl(l.Fallback)
	}
	in(t.Fallbacks)
	if a := t.Agreement; a != nil {
		in(a.Cells)
		in(a.Differ)
		in(a.FinalDiffer)
		fl(a.AreaKm2)
		fl(a.DifferKm2)
		fl(a.FinalDifferKm2)
	}
	return b, nil
}
