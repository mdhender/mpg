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
// stage render is the landmass and chokepoint map (measure Map.Render):
// landmasses by class, mountain chains, straits, necks and passes.
func runMeasures(c *Context) error {
	p := &c.Products
	m, mp, err := measure.Analyze(measure.Input{
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
		Landform:    p.Classes.Landform,
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
	if err := c.Render("", mp.Render(p.Elevation, p.Mesh)); err != nil {
		return err
	}
	p.Measures = m
	return nil
}
