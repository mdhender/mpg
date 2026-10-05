// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package pipeline

import (
	"slices"

	"github.com/mdhender/mpg/internal/cells"
)

// runCells assigns every elevation sample to its nearest cell site and
// computes each cell's altitude (median), relief (p95 − p5) and latitude,
// logs a summary, and renders the cell altitudes against the 0 m datum
// (the sea level stage sets the real one) and, as variant "relief", the
// cell reliefs.
func runCells(c *Context) error {
	f, m := c.Products.Elevation, c.Products.Mesh
	s, err := cells.Compute(f, m)
	if err != nil {
		return err
	}
	lo, hi := slices.Min(s.Samples), slices.Max(s.Samples)
	c.Logf("%d samples over %d cells: %d to %d per cell, mean %.1f; %d cells with no sample",
		len(s.Owner), s.Len(), lo, hi, float64(len(s.Owner))/float64(s.Len()), s.Empty)
	var alt, rel []float64
	for i, cell := range m.Cells {
		if !cell.Rim {
			alt = append(alt, s.Altitude[i])
			rel = append(rel, s.Relief[i])
		}
	}
	if len(alt) > 0 {
		slices.Sort(alt)
		slices.Sort(rel)
		pick := func(v []float64, p float64) float64 { return v[min(len(v)-1, int(p*float64(len(v))))] }
		c.Logf("playable altitude %.0f to %.0f m, median %.0f m; relief median %.0f m, p95 %.0f m, max %.0f m",
			alt[0], alt[len(alt)-1], pick(alt, 0.5), pick(rel, 0.5), pick(rel, 0.95), rel[len(rel)-1])
	}
	if err := c.Render("", cells.AltitudeRender(f, m, s, 0)); err != nil {
		return err
	}
	if err := c.Render("relief", cells.ReliefRender(f, m, s)); err != nil {
		return err
	}
	c.Products.Cells = s
	return nil
}
