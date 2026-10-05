// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package pipeline

import (
	"fmt"
	"strings"

	"github.com/mdhender/mpg/internal/cells"
	"github.com/mdhender/mpg/internal/classify"
	"github.com/mdhender/mpg/internal/climate"
	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/river"
	"github.com/mdhender/mpg/internal/topo"
)

// runClassify assigns every cell its landform and every salt-water cell its
// depth band, flags the volcano cells, then gives every land cell its biome
// and the cells their surfaces (classify.Result.Cover, by
// classify.DefaultBiomeRules), logs the land landform, depth, biome and
// surface counts and the volcano count, and renders the landforms and, as
// variants, the biomes ("biome"), the surfaces ("surface": ice, polar
// desert, wetlands, pack ice) and the wetness signals ("wetness"). The land
// and water are the land-target stage's: its flood and lakes at the level
// it chose, and its final climate pass; the river classes are the river
// stage's.
func runClassify(c *Context) error {
	m, s, t := c.Products.Mesh, c.Products.Cells, c.Products.Target
	peaks := make([]topo.Point, len(c.Products.Hotspots))
	for k, h := range c.Products.Hotspots {
		peaks[k] = h.Point()
	}
	res, err := classify.Classify(m, s.Altitude, s.Relief, t.Flood, t.Lakes, peaks, classify.RulesOf(c.Config))
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
	c.Logf("salt water: %s; %d fresh-water (lake) cells", strings.Join(depth, ", "), res.Count(m, classify.FreshWater, true))
	c.Logf("%d volcanic hotspots, %d volcanoes on land", len(peaks), res.Volcanoes())

	br := classify.DefaultBiomeRules()
	in, err := CoverInput(c.Config, s, t, c.Products.Network)
	if err != nil {
		return err
	}
	if err := res.Cover(m, in, br); err != nil {
		return err
	}
	c.Logf("biome table %s: %s", classify.BiomeTableVersion, strings.Join(res.BiomeShares(false), ", "))
	var surf []string
	for _, k := range classify.Surfaces {
		surf = append(surf, fmt.Sprintf("%s %d", k, res.SurfaceCount(k)))
	}
	c.Logf("surfaces: %s; %d wetland cells", strings.Join(surf, ", "), res.WetlandCount())

	f := c.Products.Elevation
	if err := c.Render("", classify.LandformRender(f, m, res)); err != nil {
		return err
	}
	if c.rendering() {
		if err := c.Render("biome", classify.BiomeRender(f, m, res)); err != nil {
			return err
		}
		if err := c.Render("surface", classify.SurfaceRender(f, m, res)); err != nil {
			return err
		}
		if err := c.Render("wetness", classify.WetnessRender(f, m, res, in.RiverClass, br.WetRiverClass)); err != nil {
			return err
		}
	}
	c.Products.Classes = res
	return nil
}

// CoverInput returns the input of the classification stage's cover
// (classify.Result.Cover): the land target's land, lakes and final climate
// pass, each cell's latitude in degrees and the sea-level temperature
// there (cfg's climate model), and the river network's edge classes.
func CoverInput(cfg config.Config, s *cells.Stats, t *LandTarget, net *river.Network) (classify.CoverInput, error) {
	model, err := climate.NewModel(cfg.Climate)
	if err != nil {
		return classify.CoverInput{}, err
	}
	cl := t.Climate
	n := len(s.Latitude)
	lat, sea := make([]float64, n), make([]float64, n)
	for i, l := range s.Latitude {
		lat[i] = climate.Degrees(l)
		sea[i] = model.SeaLevel(l)
	}
	return classify.CoverInput{
		Land: t.Land, Lakes: t.Lakes,
		LatitudeDeg: lat, SeaLevelC: sea, TemperatureC: cl.Temperature, PrecipitationMM: cl.Precipitation,
		Aridity: cl.Aridity, LiftM: cl.LiftM,
		RiverClass: net.Class,
	}, nil
}
