// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package pipeline

import (
	"github.com/mdhender/mpg/internal/basin"
	"github.com/mdhender/mpg/internal/edges"
	"github.com/mdhender/mpg/internal/mesh"
)

// runEdges builds the edge data (bearings, compass directions, neighbors,
// coast, river, incline, passability), logs its statistics (neighbor
// counts, direction error, reverse directions that are not opposite, the
// naive nearest-point collisions, the incline histograms, coast and
// passability counts), and renders the land–land inclines, as variant
// "passability" the impassable and coast edges, and as variant "compass" a
// zoomed crop of the compass directions. The water is the land-target
// stage's (Water): the ocean, the lakes and the inland seas; the river
// classes are the river stage's (Network.Class).
func runEdges(c *Context) error {
	m, s := c.Products.Mesh, c.Products.Cells
	water := Water(m, c.Products.Target)
	d, err := edges.Build(m, s.Altitude, water, c.Products.Network.Class)
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

// Water returns each cell's water kind at the land target t: ocean for the
// ocean flood, lake or inland sea for a lake cell by its kind, and none for
// land and rim cells.
func Water(m *mesh.Mesh, t *LandTarget) []edges.Water {
	water := make([]edges.Water, len(m.Cells))
	for i, c := range m.Cells {
		switch {
		case c.Rim:
		case t.Flood.Ocean[i]:
			water[i] = edges.Ocean
		case t.Lakes.Lake[i] != basin.None:
			water[i] = edges.Lake
			if t.Lakes.Lakes[t.Lakes.Lake[i]].Kind == basin.KindInlandSea {
				water[i] = edges.InlandSea
			}
		}
	}
	return water
}
