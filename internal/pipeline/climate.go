// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package pipeline

import (
	"fmt"
	"image"
	"slices"
	"strings"

	"github.com/mdhender/mpg/internal/climate"
)

// runClimate draws the sea level stage's ocean (rim included) onto the
// raster as the cell mask, computes every cell's temperature from its
// latitude and its altitude above sea level, and the precipitation,
// potential evapotranspiration, runoff and aridity (package climate), logs
// the temperature ranges and the land's precipitation, PET, runoff and
// aridity classes, and renders the temperatures and, as variants, the mask
// ("mask"), precipitation ("precip"), moisture ("moisture"), PET ("pet"),
// runoff ("runoff") and aridity ("aridity").
func runClimate(c *Context) error {
	m, s, sl := c.Products.Mesh, c.Products.Cells, c.Products.SeaLevel
	model, err := climate.NewModel(c.Config.Climate)
	if err != nil {
		return err
	}
	r, err := climate.Compute(c.Products.Elevation, m, s, &sl.Flood, model, uint64(c.Config.Seed))
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
	var lp, lpet, lr []float64
	var classes [climate.NumAridity]int
	for i := range m.Cells {
		if !r.Ocean[i] {
			lp, lpet, lr = append(lp, r.Precipitation[i]), append(lpet, r.PET[i]), append(lr, r.Runoff[i])
			classes[climate.AridityOf(r.Aridity[i])]++
		}
	}
	if n := len(lp); n > 0 {
		for _, g := range []struct {
			name string
			v    []float64
		}{{"precipitation", lp}, {"PET", lpet}, {"runoff", lr}} {
			slices.Sort(g.v)
			c.Logf("land %s p5 %.0f, median %.0f, p95 %.0f mm/yr", g.name, g.v[n*5/100], g.v[n/2], g.v[n*95/100])
		}
		shares := make([]string, climate.NumAridity)
		for a := range climate.NumAridity {
			shares[a] = fmt.Sprintf("%s %.1f%%", climate.Aridity(a), 100*float64(classes[a])/float64(n))
		}
		c.Logf("land aridity: %s", strings.Join(shares, ", "))
	}
	f := c.Products.Elevation
	for _, v := range []struct {
		name string
		draw func() *image.RGBA
	}{
		{"", func() *image.RGBA { return climate.TemperatureRender(f, m, r) }},
		{"mask", func() *image.RGBA { return climate.MaskRender(f, r) }},
		{"precip", func() *image.RGBA { return climate.PrecipRender(f, m, r) }},
		{"moisture", func() *image.RGBA { return climate.MoistureRender(f, m, r) }},
		{"pet", func() *image.RGBA { return climate.PETRender(f, m, r) }},
		{"runoff", func() *image.RGBA { return climate.RunoffRender(f, m, r) }},
		{"aridity", func() *image.RGBA { return climate.AridityRender(f, m, r) }},
	} {
		if !c.rendering() {
			break
		}
		if err := c.Render(v.name, v.draw()); err != nil {
			return err
		}
	}
	c.Products.Climate = r
	return nil
}
