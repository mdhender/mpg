// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package pipeline

import (
	"slices"

	"github.com/mdhender/mpg/internal/climate"
)

// runClimate draws the sea level stage's ocean (rim included) onto the
// raster as the cell mask, computes every cell's temperature from its
// latitude and its altitude above sea level (package climate), logs the
// temperature ranges, and renders the temperatures and, as variant "mask",
// the mask. Precipitation, evaporation and runoff are not computed yet.
func runClimate(c *Context) error {
	m, s, sl := c.Products.Mesh, c.Products.Cells, c.Products.SeaLevel
	model, err := climate.NewModel(c.Config.Climate)
	if err != nil {
		return err
	}
	r, err := climate.Compute(m, s, &sl.Flood, model)
	if err != nil {
		return err
	}
	var rim, sea, land []float64
	for i, cell := range m.Cells {
		switch t := r.Temperature[i]; {
		case cell.Rim:
			rim = append(rim, t)
		case r.Ocean[i]:
			sea = append(sea, t)
		default:
			land = append(land, t)
		}
	}
	ocean := 0
	for _, v := range r.Mask {
		if v {
			ocean++
		}
	}
	c.Logf("mask: %d of %d samples ocean (rim included)", ocean, len(r.Mask))
	for _, g := range []struct {
		name string
		v    []float64
	}{{"rim", rim}, {"ocean", sea}, {"land", land}} {
		if len(g.v) > 0 {
			slices.Sort(g.v)
			c.Logf("%s temperature %.1f to %.1f °C, median %.1f °C (%d cells)", g.name, g.v[0], g.v[len(g.v)-1], g.v[len(g.v)/2], len(g.v))
		}
	}
	if err := c.Render("", climate.TemperatureRender(c.Products.Elevation, m, r)); err != nil {
		return err
	}
	if err := c.Render("mask", climate.MaskRender(c.Products.Elevation, r)); err != nil {
		return err
	}
	c.Products.Climate = r
	return nil
}
