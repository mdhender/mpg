// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package pipeline

import (
	"github.com/mdhender/mpg/internal/edges"
)

// runEdges builds the edge data (bearings, compass directions, neighbors,
// coast, river, incline, passability), logs its statistics (neighbor
// counts, direction error, reverse directions that are not opposite, the
// naive nearest-point collisions, the incline histograms, coast and
// passability counts), and renders the land–land inclines, as variant
// "passability" the impassable and coast edges, and as variant "compass" a
// zoomed crop of the compass directions. Until the basin and river stages
// exist the only water is the sea level stage's ocean, and there are no
// rivers.
func runEdges(c *Context) error {
	m, s, sl := c.Products.Mesh, c.Products.Cells, c.Products.SeaLevel
	water := make([]edges.Water, len(m.Cells))
	for i, ocean := range sl.Flood.Ocean {
		if ocean {
			water[i] = edges.Ocean
		}
	}
	d, err := edges.Build(m, s.Altitude, water, nil)
	if err != nil {
		return err
	}
	st := edges.Summarize(m, d, water)
	for _, line := range st.Lines() {
		c.Logf("%s", line)
	}
	f := c.Products.Elevation
	if err := c.Render("", edges.InclineRender(f, m, d, water)); err != nil {
		return err
	}
	if err := c.Render("passability", edges.PassabilityRender(f, m, d, water)); err != nil {
		return err
	}
	if err := c.Render("compass", edges.CompassRender(m, d, water)); err != nil {
		return err
	}
	c.Products.Edges = d
	c.Products.EdgeStats = &st
	return nil
}
