// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package pipeline

import (
	"strconv"
	"strings"

	"github.com/mdhender/mpg/internal/elevation"
	"github.com/mdhender/mpg/internal/mesh"
)

// runMesh builds the province mesh, leaves it in Products, logs its
// summary, and renders its outlines and sites over the elevation render,
// as variant "area" its cell-area heatmap, and as variant "short" the
// short-edge collapse's 4-way corners and stretched edges.
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
	c.Logf("short-edge collapse at %.2f km: %d collapsed, %d stretched, %d degree-cap hits; corners moved up to %.2f km",
		c.Config.Mesh.MinEdgeKm, st.Collapses, st.Stretches, st.DegreeCapHits, st.MaxShiftKm)
	c.Logf("corners touching 3/4/5+ cells %d/%d/%d (%d on the rim boundary); shortest edge %.3f km, %d below min_edge_km",
		st.CornerCells[3], st.CornerCells[4], sum(st.CornerCells[5:]), st.BoundaryCorners, st.EdgeMin, st.ShortEdges)
	c.Logf("neighbors %d to %d (%d to %d off the rim, %d rim cells); by count 3–9+ %v",
		st.NeighborMin, st.NeighborMax, st.PlayableNeighborMin, st.PlayableNeighborMax, st.RimCells, st.Neighbors[3:10])
	f := c.Products.Elevation
	if err := c.Render("", mesh.StageRender(elevation.Render(f, 0), f, m)); err != nil {
		return err
	}
	if err := c.Render("area", mesh.AreaRender(f, m, a)); err != nil {
		return err
	}
	return c.Render("short", mesh.ShortRender(f, m))
}

// sum returns the sum of vs.
func sum(vs []int) int {
	var t int
	for _, v := range vs {
		t += v
	}
	return t
}

// formatCVs formats the per-pass CVs as a bracketed list, "[]" for none.
func formatCVs(cvs []float64) string {
	parts := make([]string, len(cvs))
	for k, v := range cvs {
		parts[k] = strconv.FormatFloat(v, 'f', 4, 64)
	}
	return "[" + strings.Join(parts, " ") + "]"
}
