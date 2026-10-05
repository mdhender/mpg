// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package pipeline

import (
	"fmt"
	"strings"

	"github.com/mdhender/mpg/internal/classify"
	"github.com/mdhender/mpg/internal/topo"
)

// runClassify assigns every cell its landform and every salt-water cell its
// depth band, flags the volcano cells, logs the land landform and depth
// histograms and the volcano count, and renders the landforms. Until the
// basin stage exists the land and water are the sea level stage's.
func runClassify(c *Context) error {
	m, s, sl := c.Products.Mesh, c.Products.Cells, c.Products.SeaLevel
	peaks := make([]topo.Point, len(c.Products.Hotspots))
	for k, h := range c.Products.Hotspots {
		peaks[k] = h.Point()
	}
	res, err := classify.Classify(m, s.Altitude, s.Relief, &sl.Flood, peaks, classify.RulesOf(c.Config))
	if err != nil {
		return err
	}
	c.Logf("land: %s", strings.Join(res.LandShares(false), ", "))
	var depth []string
	for _, d := range classify.Depths {
		n := 0
		for i, v := range res.Depth {
			if v == d && !m.Cells[i].Rim {
				n++
			}
		}
		depth = append(depth, fmt.Sprintf("%s %d", d, n))
	}
	c.Logf("salt water: %s", strings.Join(depth, ", "))
	c.Logf("%d volcanic hotspots, %d volcanoes on land", len(peaks), res.Volcanoes())
	if err := c.Render("", classify.LandformRender(c.Products.Elevation, m, res)); err != nil {
		return err
	}
	c.Products.Classes = res
	return nil
}
