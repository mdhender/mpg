// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package river

import (
	"errors"
	"fmt"
	"math"

	"github.com/mdhender/mpg/internal/edges"
	"github.com/mdhender/mpg/internal/fmath"
)

// SecondsPerYear is the Julian year in seconds, which converts a volume per
// year to a discharge.
const SecondsPerYear = 31_557_600

// Discharge converts a volume in mm·km² per year (10³ m³ per year, the
// unit of package basin's water balance) to a discharge in m³/s.
func Discharge(volume float64) float64 { return volume * 1000 / SecondsPerYear }

// Params are the river selection settings (config river): an edge is a
// river when its drainage is at least ThresholdKm2, a stream from
// ThresholdKm2, a river from RiverKm2 and a major river from MajorRiverKm2.
type Params struct {
	ThresholdKm2, RiverKm2, MajorRiverKm2 float64
}

// Class returns the river class of an edge with drainage a (km²).
func (p Params) Class(a float64) edges.RiverClass {
	switch {
	case a >= p.MajorRiverKm2:
		return edges.MajorRiver
	case a >= p.RiverKm2:
		return edges.River
	case a >= p.ThresholdKm2:
		return edges.Stream
	}
	return edges.RiverNone
}

func (p Params) check() error {
	if !(p.ThresholdKm2 > 0 && p.RiverKm2 > p.ThresholdKm2 && p.MajorRiverKm2 > p.RiverKm2) || math.IsInf(p.MajorRiverKm2, 0) {
		return fmt.Errorf("river: class breaks %v, %v, %v must be finite, positive and increasing", p.ThresholdKm2, p.RiverKm2, p.MajorRiverKm2)
	}
	return nil
}

// Flow is the water the accumulation reads besides the tree's input.
type Flow struct {
	// Runoff is each cell's runoff in mm per year (the final climate
	// pass's); only land cells are read. It must be finite and not
	// negative.
	Runoff []float64
	// Overflow is each lake's overflow in mm·km² per year (basin
	// Lake.Overflow, the water balance's, which is the authority on what
	// leaves a lake); only lakes draining by an outlet are read.
	Overflow []float64
}

// Path is a river polyline from source to mouth or confluence: Edges[k]
// joins Corners[k] and Corners[k+1], downstream, and Classes[k] is its
// class.
type Path struct {
	Corners []int
	Edges   []int
	Classes []edges.RiverClass
}

// Network is the river network on a drainage tree: the water accumulated
// down it, the river edges and their classes, and the polylines.
type Network struct {
	Params Params
	// Drainage is each corner's drainage area in km² and Volume its water
	// in mm·km² per year: everything that reaches the corner, its cells'
	// runoff and the corners upstream of it, and at a lake's outlet the
	// lake's drainage (LakeDrainage) and overflow. Water leaves the corner
	// along its tree edge, or ends there at a terminal. Zero off the graph.
	Drainage, Volume []float64
	// LakeDrainage is each lake's drainage area: the drainage reaching its
	// shore corners, the lake's own cells' area, and the drainage of the
	// lakes draining straight into it. A lake with an outlet passes it on
	// there; a closed lake or one draining straight to the sea ends it.
	// LakeInflow is the volume the tree delivers to its shore (with the
	// runoff read here, for comparison with the water balance's).
	LakeDrainage, LakeInflow []float64
	// Up is each edge's upstream corner when it is a tree edge (the corner
	// whose DownEdge it is), else None. A tree edge carries its upstream
	// corner's drainage and volume.
	Up []int
	// Class is each edge's river class: RiverNone off the tree or below
	// the threshold.
	Class []edges.RiverClass
	// Paths lists the polylines, ordered by first edge id. Each river
	// edge lies on exactly one.
	Paths []Path
	// Mouth marks the corners where a polyline ends and none continues:
	// where a river reaches the sea, a lake, or a dry sink.
	Mouth []bool
}

// EdgeDrainage returns edge e's drainage in km², 0 off the tree.
func (n *Network) EdgeDrainage(e int) float64 {
	if k := n.Up[e]; k != None {
		return n.Drainage[k]
	}
	return 0
}

// EdgeDischarge returns edge e's discharge in m³/s, 0 off the tree.
func (n *Network) EdgeDischarge(e int) float64 {
	if k := n.Up[e]; k != None {
		return Discharge(n.Volume[k])
	}
	return 0
}

// Accumulate builds the river network on tree t, built from in. See the
// package documentation.
func Accumulate(in Input, t *Tree, f Flow, p Params) (*Network, error) {
	if err := p.check(); err != nil {
		return nil, err
	}
	m := in.Mesh
	switch {
	case len(f.Runoff) != len(m.Cells):
		return nil, fmt.Errorf("river: %d runoff values for %d cells", len(f.Runoff), len(m.Cells))
	case len(f.Overflow) != len(in.Lakes):
		return nil, fmt.Errorf("river: %d overflows for %d lakes", len(f.Overflow), len(in.Lakes))
	case len(t.Land) != len(m.Corners) || len(t.Lakes) != len(in.Lakes):
		return nil, errors.New("river: the tree was not built from this input")
	}
	nc, nl := len(m.Corners), len(in.Lakes)
	n := &Network{
		Params:       p,
		Drainage:     make([]float64, nc),
		Volume:       make([]float64, nc),
		LakeDrainage: make([]float64, nl),
		LakeInflow:   make([]float64, nl),
		Up:           make([]int, len(m.Edges)),
		Class:        make([]edges.RiverClass, len(m.Edges)),
		Mouth:        make([]bool, nc),
	}
	// Each land cell's area and runoff enter at its lowest corner, and
	// each lake cell's area into its lake, in cell id order.
	lakeArea := make([]float64, nl)
	for c := range m.Cells {
		switch {
		case in.Land[c]:
			r := f.Runoff[c]
			if !(r >= 0) || math.IsInf(r, 0) {
				return nil, fmt.Errorf("river: cell %d runoff %v", c, r)
			}
			a := m.Area(c)
			k := t.Lowest(m, c)
			n.Drainage[k] += a
			n.Volume[k] += fmath.Mul(r, a)
		case in.Lake[c] != None:
			lakeArea[in.Lake[c]] += m.Area(c)
		}
	}
	outlets := make([][]int, nc) // lakes by outlet corner
	direct := make([][]int, nl)  // lakes draining straight into each lake
	for l, o := range t.Lakes {
		switch o.Drain {
		case Outlet:
			if v := f.Overflow[l]; !(v >= 0) || math.IsInf(v, 0) {
				return nil, fmt.Errorf("river: lake %d overflow %v", l, v)
			}
			outlets[o.Corner] = append(outlets[o.Corner], l)
		case DirectLake:
			direct[o.Into] = append(direct[o.Into], l)
		}
	}
	// lake returns lake l's drainage: its shore inflow, its area, and the
	// lakes draining straight into it (which drain no further, so the
	// recursion follows the acyclic drains).
	shore := make([]float64, nl) // drainage reaching each lake's shore
	var lake func(l int) float64
	lake = func(l int) float64 {
		a := shore[l] + lakeArea[l]
		for _, u := range direct[l] {
			a += lake(u)
		}
		return a
	}
	passed := make([]float64, nl) // what each outlet passed on, to check
	// Accumulate in reverse flood order: every corner after the corners
	// upstream of it, and an overflowing lake's outlet after its shore
	// (Tree.Order).
	for i := len(t.Order) - 1; i >= 0; i-- {
		k := t.Order[i]
		for _, l := range outlets[k] {
			passed[l] = lake(l)
			n.Drainage[k] += passed[l]
			n.Volume[k] += f.Overflow[l]
		}
		d := t.Down[k]
		if d == None {
			if t.Terminal[k] == Lake {
				shore[t.Water[k]] += n.Drainage[k]
				n.LakeInflow[t.Water[k]] += n.Volume[k]
			}
			continue
		}
		n.Drainage[d] += n.Drainage[k]
		n.Volume[d] += n.Volume[k]
	}
	for l := range nl {
		n.LakeDrainage[l] = lake(l)
		if t.Lakes[l].Drain == Outlet && passed[l] != n.LakeDrainage[l] {
			return nil, fmt.Errorf("river: lake %d received drainage after its outlet passed it on", l)
		}
	}
	for e := range n.Up {
		n.Up[e] = None
	}
	for k, e := range t.DownEdge {
		if e != None {
			n.Up[e] = k
			n.Class[e] = p.Class(n.Drainage[k])
		}
	}
	if err := n.paths(t, outlets); err != nil {
		return nil, err
	}
	return n, nil
}
