// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package pipeline

import (
	"strconv"
	"strings"

	"github.com/mdhender/mpg/internal/elevation"
	"github.com/mdhender/mpg/internal/mesh"
)

// runMesh builds the province mesh, runs the mesh checks, logs its summary
// and the check report, and renders it: the rim as ice and the playable
// cells' outlines over the elevation render, as variant "area" its
// cell-area heatmap, and as variant "short" the short-edge collapse's 4-way
// corners and stretched edges. A mesh that fails a check is still rendered,
// so it can be inspected, but the stage then fails and leaves no product: a
// bad mesh never reaches the later stages. The mesh depends only on the
// config, so the stage reuses the one the elevation stage's pre-pass built.
func runMesh(c *Context) error {
	m := c.preMesh // the elevation stage's pre-pass built it from the same config
	if m == nil {
		var err error
		if m, err = mesh.New(c.Config); err != nil {
			return err
		}
	}
	a := c.Config.Province.AreaKm2
	rep := m.Check(mesh.LimitsOf(c.Config))
	c.Logf("%d cells, %d corners, %d edges (%d on the rim boundary); %d cells span the seam; ghost band %.1f km",
		rep.Cells, rep.Corners, rep.Edges, rep.BoundaryEdges, rep.SeamCells, m.GhostMarginKm)
	c.Logf("%d Lloyd passes; area CV by pass %s, now %.4f; area mean %.4f km² (A = %.4f, deviation %+.2e)",
		len(m.LloydCV), formatCVs(m.LloydCV), rep.AreaCV, rep.AreaMean, a, rep.AreaDev)
	for _, line := range rep.Lines() {
		c.Logf("%s", line)
	}
	f := c.Products.Elevation
	base := elevation.Render(f, 0)
	if err := c.Render("", mesh.StageRender(base, f, m)); err != nil {
		return err
	}
	if err := c.Render("area", mesh.AreaRender(f, m, a)); err != nil {
		return err
	}
	if err := c.Render("short", mesh.ShortRender(base, f, m)); err != nil {
		return err
	}
	if err := rep.Err(); err != nil {
		return err
	}
	c.Products.Mesh = m
	return nil
}

// formatCVs formats the per-pass CVs as a bracketed list, "[]" for none.
func formatCVs(cvs []float64) string {
	parts := make([]string, len(cvs))
	for k, v := range cvs {
		parts[k] = strconv.FormatFloat(v, 'f', 4, 64)
	}
	return "[" + strings.Join(parts, " ") + "]"
}
