// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package river

import (
	"cmp"
	"encoding/binary"
	"fmt"
	"math"
	"slices"

	"github.com/mdhender/mpg/internal/edges"
)

// lakeMain marks, in paths, a lake outlet whose main inflow is its lake.
const lakeMain = -2

// paths splits the river edges into polylines by main stem. At each corner
// the main inflow is the river edge into it with the largest drainage, ties
// to the lower upstream corner id; at a lake's outlet the lake counts as an
// inflow with the drainage it passes on, and wins a tie. A polyline starts
// at a corner with no main river inflow (a source, or an outlet whose lake
// is its main inflow) and runs down the tree until it reaches a terminal
// (a mouth) or a corner where it is not the main inflow (a confluence,
// where the main stem continues).
func (n *Network) paths(t *Tree, outlets [][]int) error {
	main := make([]int, len(t.Land))
	for k := range main {
		main[k] = None
	}
	for k, d := range t.Down {
		if d == None || n.Class[t.DownEdge[k]] == edges.RiverNone {
			continue
		}
		if j := main[d]; j == None || n.Drainage[k] > n.Drainage[j] {
			main[d] = k
		}
	}
	for k, ls := range outlets {
		var a float64
		for _, l := range ls {
			a += n.LakeDrainage[l]
		}
		if len(ls) > 0 && (main[k] == None || a >= n.Drainage[main[k]]) {
			main[k] = lakeMain
		}
	}
	for k, d := range t.Down {
		if d == None || n.Class[t.DownEdge[k]] == edges.RiverNone || main[k] >= 0 {
			continue
		}
		p := Path{Corners: []int{k}}
		for x := k; ; {
			e := t.DownEdge[x]
			cl := n.Class[e]
			if cl == edges.RiverNone {
				return fmt.Errorf("river: edge %d below the threshold downstream of a river", e)
			}
			y := t.Down[x]
			p.Corners = append(p.Corners, y)
			p.Edges = append(p.Edges, e)
			p.Classes = append(p.Classes, cl)
			if t.Down[y] == None {
				n.Mouth[y] = true
				break
			}
			if main[y] != x {
				break
			}
			x = y
		}
		n.Paths = append(n.Paths, p)
	}
	slices.SortFunc(n.Paths, func(a, b Path) int { return cmp.Compare(a.Edges[0], b.Edges[0]) })
	return nil
}

// NetworkStats summarizes a river network, for the logs and the measures.
type NetworkStats struct {
	// Edges counts the tree edges by river class (Edges[RiverNone] those
	// below the threshold); RiverEdges the river edges, RiverKm their
	// length, and Seam those crossing the east–west seam.
	Edges      [4]int
	RiverEdges int
	RiverKm    float64
	Seam       int
	// LandCells counts the land cells, LandAreaKm2 their area, and
	// TouchCells those with a river edge on their border.
	LandCells, TouchCells int
	LandAreaKm2           float64
	// Paths counts the polylines, Mouths the mouth corners, and
	// Confluences the corners where a polyline ends on another. Ends
	// counts the polylines by where they end: Ends[Ocean], Ends[Lake] and
	// Ends[Sink] at a mouth on the sea, a lake or a dry sink, and
	// Ends[Interior] at a confluence. OutletSources counts the polylines
	// starting at a lake's outlet.
	Paths, Mouths, Confluences, OutletSources int
	Ends                                      [4]int
	// Longest is the polyline with the most edges (ties to the longer,
	// then the first), LongestEdges and LongestKm its size; FlowEdges and
	// FlowKm are the longest path from a polyline's source down the tree
	// to where its water ends, by edges (ties to the longer).
	Longest, LongestEdges int
	LongestKm             float64
	FlowEdges             int
	FlowKm                float64
	// MaxDrainageKm2 and MaxDischargeM3s are the largest on any edge.
	MaxDrainageKm2, MaxDischargeM3s float64
}

// EdgesPerLandCell is the river edges per land cell.
func (s *NetworkStats) EdgesPerLandCell() float64 {
	return ratio(float64(s.RiverEdges), float64(s.LandCells))
}

// KmPer1000Km2 is the river length in km per 1,000 km² of land.
func (s *NetworkStats) KmPer1000Km2() float64 {
	return ratio(s.RiverKm*1000, s.LandAreaKm2)
}

// TouchShare is the share of land cells with a river on their border.
func (s *NetworkStats) TouchShare() float64 {
	return ratio(float64(s.TouchCells), float64(s.LandCells))
}

func ratio(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

// Stats returns the network's statistics; in and t are what it was built
// from.
func (n *Network) Stats(in Input, t *Tree) NetworkStats {
	var s NetworkStats
	m := in.Mesh
	half := m.Cylinder().W() / 2
	touch := make([]bool, len(m.Cells))
	for e, k := range n.Up {
		if k == None {
			continue
		}
		cl := n.Class[e]
		s.Edges[cl]++
		s.MaxDrainageKm2 = max(s.MaxDrainageKm2, n.Drainage[k])
		s.MaxDischargeM3s = max(s.MaxDischargeM3s, Discharge(n.Volume[k]))
		if cl == edges.RiverNone {
			continue
		}
		s.RiverEdges++
		s.RiverKm += m.EdgeLength(e)
		c := m.Edges[e].Corners
		if math.Abs(m.Corners[c[0]].Point.X-m.Corners[c[1]].Point.X) > half {
			s.Seam++
		}
		for _, x := range m.Edges[e].Cells {
			touch[x] = true // a tree edge is land–land
		}
	}
	for c, land := range in.Land {
		if land {
			s.LandCells++
			s.LandAreaKm2 += m.Area(c)
			if touch[c] {
				s.TouchCells++
			}
		}
	}
	isOutlet := make([]bool, len(t.Land))
	for _, l := range t.Lakes {
		if l.Drain == Outlet {
			isOutlet[l.Corner] = true
		}
	}
	confluence := make([]bool, len(t.Land))
	s.Paths = len(n.Paths)
	s.Longest = None
	for i, p := range n.Paths {
		last := p.Corners[len(p.Corners)-1]
		if n.Mouth[last] {
			s.Ends[t.Terminal[last]]++
		} else {
			s.Ends[Interior]++
			confluence[last] = true
		}
		if isOutlet[p.Corners[0]] {
			s.OutletSources++
		}
		var km float64
		for _, e := range p.Edges {
			km += m.EdgeLength(e)
		}
		if ne := len(p.Edges); ne > s.LongestEdges || ne == s.LongestEdges && km > s.LongestKm {
			s.Longest, s.LongestEdges, s.LongestKm = i, ne, km
		}
		ne, fk := 0, 0.0
		for x := p.Corners[0]; t.Down[x] != None; x = t.Down[x] {
			ne++
			fk += m.EdgeLength(t.DownEdge[x])
		}
		if ne > s.FlowEdges || ne == s.FlowEdges && fk > s.FlowKm {
			s.FlowEdges, s.FlowKm = ne, fk
		}
	}
	for k := range n.Mouth {
		if n.Mouth[k] {
			s.Mouths++
		}
		if confluence[k] {
			s.Confluences++
		}
	}
	return s
}

// AppendBinary appends the network's canonical encoding to b, for the golden
// hashes: integers as int64 and floats as their IEEE 754 bits, all
// little-endian, in this order:
//
//	ThresholdKm2, RiverKm2, MajorRiverKm2      floats
//	number of corners                          integer
//	per corner: Drainage, Volume               floats
//	  Mouth                                    integer (0 or 1)
//	number of lakes                            integer
//	per lake: LakeDrainage, LakeInflow         floats
//	number of edges                            integer
//	per edge: Up, Class                        integers
//	number of paths                            integer
//	per path: number of edges, then each       integers
//	  corner, edge and class                   integers
//
// The error is always nil; the signature is encoding.BinaryAppender's.
func (n *Network) AppendBinary(b []byte) ([]byte, error) {
	fl := func(v float64) { b = binary.LittleEndian.AppendUint64(b, math.Float64bits(v)) }
	in := func(v int) { b = binary.LittleEndian.AppendUint64(b, uint64(int64(v))) }
	fl(n.Params.ThresholdKm2)
	fl(n.Params.RiverKm2)
	fl(n.Params.MajorRiverKm2)
	in(len(n.Drainage))
	for k := range n.Drainage {
		fl(n.Drainage[k])
		fl(n.Volume[k])
		if n.Mouth[k] {
			in(1)
		} else {
			in(0)
		}
	}
	in(len(n.LakeDrainage))
	for l := range n.LakeDrainage {
		fl(n.LakeDrainage[l])
		fl(n.LakeInflow[l])
	}
	in(len(n.Up))
	for e, k := range n.Up {
		in(k)
		in(int(n.Class[e]))
	}
	in(len(n.Paths))
	for _, p := range n.Paths {
		in(len(p.Edges))
		for _, k := range p.Corners {
			in(k)
		}
		for _, e := range p.Edges {
			in(e)
		}
		for _, c := range p.Classes {
			in(int(c))
		}
	}
	return b, nil
}
