// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package pipeline

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mdhender/mpg/internal/measure"
	"github.com/mdhender/mpg/world"
)

// runMeasures is stage 13. It computes the playability measures (package
// measure) from the land target, the mesh and the edge data, runs the
// configured checks, writes measures.json and the text summary
// (measure.SummaryFile), and logs the summary. A failed check, gate or
// not, is not a stage error: the run goes on to export, and the caller
// reads Products.Measures (mpg generate exits 3 when a gate failed). The
// stage has no render yet; the landmass and chokepoint map comes with S34.
func runMeasures(c *Context) error {
	p := &c.Products
	m, err := measure.Compute(measure.Input{
		Config:      c.Config,
		ConfigHash:  c.ConfigHash,
		Mesh:        p.Mesh,
		Search:      p.Target.Search,
		Land:        p.Target.Land,
		LandAreaKm2: p.Target.LandAreaKm2,
		OceanCells:  p.Target.OceanCells,
		Lakes:       p.Target.Lakes,
		Edges:       p.Edges,
		EdgeStats:   p.EdgeStats,
	})
	if err != nil {
		return err
	}
	b, err := m.Bytes()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(c.OutputDir, world.MeasuresFile), b, 0o644); err != nil {
		return fmt.Errorf("pipeline: %w", err)
	}
	if err := os.WriteFile(filepath.Join(c.OutputDir, measure.SummaryFile), measure.Summary(m), 0o644); err != nil {
		return fmt.Errorf("pipeline: %w", err)
	}
	for _, line := range measure.Lines(m) {
		c.Logf("%s", line)
	}
	p.Measures = m
	return nil
}
