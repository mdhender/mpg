// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package pipeline

import (
	"strconv"
	"strings"

	"github.com/mdhender/mpg/internal/elevation"
	"github.com/mdhender/mpg/internal/mesh"
)

// runMesh builds the province mesh, leaves it in Products, logs its
// summary, and renders its outlines and sites over the elevation render
// and, as variant "area", its cell-area heatmap.
func runMesh(c *Context) error {
	m, err := mesh.New(c.Config)
	if err != nil {
		return err
	}
	c.Products.Mesh = m
	a := c.Config.Province.AreaKm2
	st := m.Stats(a, c.Config.Mesh.MinEdgeKm)
	c.Logf("%d cells, %d corners, %d edges (%d on the rim boundary); %d cells span the seam; ghost band %.1f km",
		st.Cells, st.Corners, st.Edges, st.BoundaryEdges, st.SeamCells, m.GhostMarginKm)
	c.Logf("%d Lloyd passes; area CV by pass %s, now %.4f", len(m.LloydCV), formatCVs(m.LloydCV), st.AreaCV)
	c.Logf("area mean %.4f km² (A = %.4f, deviation %+.2e), range %.3f A to %.3f A",
		st.AreaMean, a, st.AreaDev, st.AreaMinA, st.AreaMaxA)
	c.Logf("neighbors %d to %d; shortest edge %.3g km, %d below min_edge_km %.2f",
		st.NeighborMin, st.NeighborMax, st.EdgeMin, st.ShortEdges, c.Config.Mesh.MinEdgeKm)
	f := c.Products.Elevation
	if err := c.Render("", mesh.StageRender(elevation.Render(f, 0), f, m)); err != nil {
		return err
	}
	return c.Render("area", mesh.AreaRender(f, m, a))
}

// formatCVs formats the per-pass CVs as a bracketed list, "[]" for none.
func formatCVs(cvs []float64) string {
	parts := make([]string, len(cvs))
	for k, v := range cvs {
		parts[k] = strconv.FormatFloat(v, 'f', 4, 64)
	}
	return "[" + strings.Join(parts, " ") + "]"
}
