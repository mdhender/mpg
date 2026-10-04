// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package pipeline

import (
	"github.com/mdhender/mpg/internal/elevation"
	"github.com/mdhender/mpg/internal/mesh"
)

// runMesh builds the province mesh, leaves it in Products, logs its
// summary, and renders its outlines and sites over the elevation render.
func runMesh(c *Context) error {
	m, err := mesh.New(c.Config)
	if err != nil {
		return err
	}
	c.Products.Mesh = m
	st := m.Stats(c.Config.Mesh.MinEdgeKm)
	c.Logf("%d cells, %d corners, %d edges (%d on the rim boundary); %d cells span the seam; ghost band %.1f km",
		st.Cells, st.Corners, st.Edges, st.BoundaryEdges, st.SeamCells, m.GhostMarginKm)
	c.Logf("neighbors %d to %d; area mean %.2f km² (A = %.2f), CV %.3f, range %.2f to %.2f; shortest edge %.3g km, %d below min_edge_km %.2f",
		st.NeighborMin, st.NeighborMax, st.AreaMean, c.Config.Province.AreaKm2, st.AreaCV, st.AreaMin, st.AreaMax,
		st.EdgeMin, st.ShortEdges, c.Config.Mesh.MinEdgeKm)
	f := c.Products.Elevation
	return c.Render("", mesh.StageRender(elevation.Render(f, 0), f, m))
}
