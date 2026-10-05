// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package climate

import (
	"github.com/mdhender/mpg/internal/cells"
	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/noise"
	"github.com/mdhender/mpg/internal/seed"
)

// ComputeWorkers is Compute with an explicit worker count.
var ComputeWorkers = compute

// WindsAt returns the winds at signed latitude lat degrees (north
// positive), before any jitter: the bearings they blow from and their
// weights, northern fans first.
func (m Model) WindsAt(lat float64) (from, weight []float64) {
	for _, f := range m.p.winds(nil, lat) {
		from, weight = append(from, f.from), append(weight, f.weight)
	}
	return from, weight
}

// WindwardAt returns the windward-coast precipitation table at phi degrees.
func (m Model) WindwardAt(phi float64) float64 { return m.p.windward.at(phi) }

// SampleAt returns the precipitation, moisture and lift Compute gives a
// point (x, y) of r's world whose cell is cell.
func SampleAt(f *field.Field, s *cells.Stats, r *Result, model Model, world uint64, x, y float64, cell int) (p, moisture, lift float64) {
	g := newGrid(f, s.Owner, r)
	src := noise.NewSource(seed.Derive(world, Stage, Version))
	v, _ := model.p.at(g, src, noise.NewCylinder(f.Cylinder()), x, y, r.Ocean[cell], r.HeightM[cell], nil)
	return v.p, v.moisture, v.lift
}
