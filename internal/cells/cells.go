// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package cells

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/mesh"
	"github.com/mdhender/mpg/internal/topo"
)

// The nearest-rank percentiles of a cell's samples (see package field).
const (
	// LowPercentile and HighPercentile bound relief: relief is the
	// HighPercentile value minus the LowPercentile value.
	LowPercentile  = 5
	HighPercentile = 95
	// MedianPercentile gives altitude.
	MedianPercentile = 50
)

// Stats holds the per-cell statistics of a raster over a set of sites: one
// entry per cell (site), in cell id order. See the package documentation.
type Stats struct {
	// Altitude is the cell's altitude in meters: the nearest-rank median of
	// its samples. It is the only height the mesh stages use.
	Altitude []float64
	// Relief is the cell's roughness in meters: the nearest-rank 95th
	// percentile of its samples minus the 5th. It is not a height.
	Relief []float64
	// Latitude is the latitude proxy 1 − 2·y/H at the cell's site.
	Latitude []float64
	// Samples counts the raster samples assigned to the cell.
	Samples []int
	// Owner holds, for every raster sample in the field's storage order
	// (rows north to south, columns west to east), the id of the cell it
	// was assigned to: the cell whose site is nearest.
	Owner []int
	// Empty counts the cells that received no sample. Each takes its
	// altitude from the field at its site (bilinear) and has relief 0.
	Empty int
}

// Compute returns the statistics of the raster f over the cells of m. f and
// m must lie on the same cylinder.
func Compute(f *field.Field, m *mesh.Mesh) (*Stats, error) {
	fc, mc := f.Cylinder(), m.Cylinder()
	if fc.W() != mc.W() || fc.H() != mc.H() {
		return nil, fmt.Errorf("cells: field is %v × %v km, mesh %v × %v km", fc.W(), fc.H(), mc.W(), mc.H())
	}
	sites := make([]topo.Point, len(m.Cells))
	for i, c := range m.Cells {
		sites[i] = c.Site
	}
	return FromSites(f, sites)
}

// FromSites returns the statistics of the raster f over the Voronoi cells
// of sites, which lie on f's cylinder (X in [0, W), Y in [0, H]). Cell i is
// the cell of site i.
func FromSites(f *field.Field, sites []topo.Point) (*Stats, error) {
	cyl := f.Cylinder()
	loc, err := NewLocator(cyl, sites)
	if err != nil {
		return nil, err
	}
	n := len(sites)
	s := &Stats{
		Altitude: make([]float64, n),
		Relief:   make([]float64, n),
		Latitude: make([]float64, n),
		Samples:  make([]int, n),
		Owner:    make([]int, f.Len()),
	}

	// Assign every sample, in storage order.
	k := 0
	for j := range f.NY() {
		y := f.Y(j)
		for i := range f.NX() {
			c := loc.Nearest(topo.Point{X: f.X(i), Y: y})
			s.Owner[k] = c
			s.Samples[c]++
			k++
		}
	}

	// Group the sample values by cell, each cell's in storage order.
	start := make([]int, n+1)
	for c, cnt := range s.Samples {
		start[c+1] = start[c] + cnt
	}
	next := append([]int(nil), start[:n]...)
	values := f.Values()
	grouped := make([]float64, len(values))
	for k, c := range s.Owner {
		grouped[next[c]] = values[k]
		next[c]++
	}

	for c, site := range sites {
		s.Latitude[c] = cyl.Latitude(site.Y)
		vs := grouped[start[c]:start[c+1]]
		if len(vs) == 0 {
			s.Empty++
			s.Altitude[c] = f.Sample(site.X, site.Y)
			continue
		}
		p, err := field.Percentiles(vs, LowPercentile, MedianPercentile, HighPercentile)
		if err != nil {
			return nil, fmt.Errorf("cells: cell %d: %w", c, err)
		}
		s.Altitude[c] = p[1]
		s.Relief[c] = p[2] - p[0]
	}
	for c := range n {
		if v := s.Relief[c]; math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("cells: cell %d relief %v is not finite", c, v)
		}
	}
	return s, nil
}

// Len returns the number of cells.
func (s *Stats) Len() int { return len(s.Altitude) }

// AppendBinary appends the statistics' canonical encoding to b; it is the
// input to the golden hashes. Integers are little-endian uint64; floats are
// their IEEE 754 bits (math.Float64bits) as little-endian uint64. In order:
//
//	number of cells, number of samples           integers
//	per cell, in id order:
//	    Altitude, Relief, Latitude               floats
//	    Samples                                  integer
//	Empty                                        integer
//	Owner, in sample storage order               integers
//
// The error is always nil; the signature is encoding.BinaryAppender's.
func (s *Stats) AppendBinary(b []byte) ([]byte, error) {
	f := func(v float64) { b = binary.LittleEndian.AppendUint64(b, math.Float64bits(v)) }
	i := func(v int) { b = binary.LittleEndian.AppendUint64(b, uint64(int64(v))) }
	i(s.Len())
	i(len(s.Owner))
	for c := range s.Len() {
		f(s.Altitude[c])
		f(s.Relief[c])
		f(s.Latitude[c])
		i(s.Samples[c])
	}
	i(s.Empty)
	for _, c := range s.Owner {
		i(c)
	}
	return b, nil
}
