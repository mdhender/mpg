// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package classify

import (
	"image"
	"image/color"

	"github.com/mdhender/mpg/internal/edges"
	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/mesh"
)

// The biome and surface palette: hmz2bio's preview colors
// (tpty/hmz2bio/preview.go) for its biomes and wetlands, and mpg's for
// polar desert, the two kinds of land ice, and pack ice. The player map
// uses the same colors.
var (
	biomeColors = [numBiomes]color.RGBA{
		BiomeNone:           {0xff, 0x00, 0xff, 0xff}, // never drawn on land: an unset cell shows magenta
		Clear:               {0xff, 0xff, 0xff, 0xff}, // under ice: drawn by its surface
		PolarDesert:         {0xd9, 0xd4, 0xc7, 0xff},
		Tundra:              {0x9f, 0xb3, 0xa6, 0xff},
		Alpine:              {0xbd, 0xb6, 0xad, 0xff},
		Desert:              {0xf2, 0xe2, 0xa8, 0xff},
		Scrubland:           {0xc9, 0xa8, 0x6a, 0xff},
		Steppe:              {0xd6, 0xcf, 0x98, 0xff},
		Grassland:           {0xa9, 0xcf, 0x6e, 0xff},
		Savanna:             {0xd9, 0xb4, 0x4a, 0xff},
		BorealForest:        {0x3f, 0x6a, 0x5a, 0xff},
		TemperateForest:     {0x4f, 0x8f, 0x3a, 0xff},
		TemperateRainforest: {0x2f, 0x6f, 0x4f, 0xff},
		TropicalDryForest:   {0x8f, 0xaa, 0x3c, 0xff},
		TropicalRainforest:  {0x1f, 0x5f, 0x1f, 0xff},
		CloudForest:         {0x5a, 0x8f, 0x9a, 0xff},
	}
	surfaceColors = [numSurfaces]color.RGBA{
		SurfaceNone: {0xff, 0x00, 0xff, 0xff}, // never drawn
		Glacier:     {0xff, 0xff, 0xff, 0xff},
		IceField:    {0xd4, 0xe3, 0xee, 0xff},
		PackIce:     {0xc4, 0xdc, 0xec, 0xff},
		Marshes:     {0x7a, 0xd0, 0xc0, 0xff},
		Swamps:      {0x3c, 0x8c, 0x7c, 0xff},
		Bogs:        {0x8a, 0x7a, 0x9a, 0xff},
		Mangroves:   {0x00, 0xa0, 0x80, 0xff},
		SaltFlats:   {0xff, 0xf0, 0xf5, 0xff},
	}
	// landformShade darkens a biome's color by landform, in percent, so
	// relief still reads on the player map.
	landformShade = [numLandforms]int{
		RollingPlains: 94, Hills: 84, Mountains: 68, Plateaus: 90, VolcanicHighlands: 90,
	}
	// The surface render's other inks: bare land, polar desert (bare cold
	// land, the cold desert without ice), and open water.
	bareLandInk    = color.RGBA{0xe0, 0xdc, 0xcf, 0xff}
	polarDesertInk = color.RGBA{0xa0, 0x52, 0x2d, 0xff}
	openWaterInk   = color.RGBA{0xb0, 0xc4, 0xd8, 0xff}
	// The wetness render's inks, one per signal and one for several.
	wetRiverInk   = color.RGBA{0x1f, 0x4f, 0xd0, 0xff}
	wetShoreInk   = color.RGBA{0x20, 0xb0, 0xc0, 0xff}
	wetSurplusInk = color.RGBA{0x30, 0xa0, 0x40, 0xff}
	wetManyInk    = color.RGBA{0x7b, 0x3f, 0xa0, 0xff}
	dryFlatsInk   = color.RGBA{0xe6, 0xe0, 0xd4, 0xff}
	dryLandInk    = color.RGBA{0xc8, 0xc2, 0xb4, 0xff}
	riverInk      = color.RGBA{0x2a, 0x6f, 0xc0, 0xff}
)

// BiomeColor returns the fill of biome b, magenta for one outside the
// codebook.
func BiomeColor(b Biome) color.RGBA {
	if b < numBiomes {
		return biomeColors[b]
	}
	return biomeColors[BiomeNone]
}

// SurfaceColor returns the fill of surface s, magenta for none or one
// outside the codebook.
func SurfaceColor(s Surface) color.RGBA {
	if s < numSurfaces {
		return surfaceColors[s]
	}
	return surfaceColors[SurfaceNone]
}

// Shade returns c darkened for landform l: rolling plains to 94%, hills
// 84%, mountains 68%, plateaus and volcanic highlands 90%, the rest
// unchanged. It uses integer arithmetic only.
func Shade(c color.RGBA, l Landform) color.RGBA {
	k := 100
	if l < numLandforms && landformShade[l] != 0 {
		k = landformShade[l]
	}
	f := func(v uint8) uint8 { return uint8(int(v) * k / 100) }
	return color.RGBA{R: f(c.R), G: f(c.G), B: f(c.B), A: c.A}
}

// waterFill is the classification render's fill of a water cell, the rim
// as ice.
func waterFill(m *mesh.Mesh, r *Result, i int) color.RGBA {
	if m.Cells[i].Rim {
		return mesh.IceColor
	}
	return LandformColor(r.Landform[i], r.Depth[i])
}

// BiomeRender draws the classification stage's "biome" render over
// mesh.CellRender: each land cell by its biome (permanent ice white),
// water by depth band as LandformRender draws it, and the rim as ice.
// Surfaces are not drawn; see SurfaceRender.
func BiomeRender(f *field.Field, m *mesh.Mesh, r *Result) *image.RGBA {
	return mesh.CellRender(f, m, func(i int) color.RGBA {
		if r.Biome[i] == BiomeNone {
			return waterFill(m, r, i)
		}
		return BiomeColor(r.Biome[i])
	})
}

// SurfaceRender draws the "surface" render: glaciers, ice fields, pack ice
// and wetlands in their colors, polar desert (cold land too dry for ice) in
// rust, other bare land pale, open water in a flat pale blue, and the rim
// as ice, so ice and cold deserts stand out.
func SurfaceRender(f *field.Field, m *mesh.Mesh, r *Result) *image.RGBA {
	return mesh.CellRender(f, m, func(i int) color.RGBA {
		switch {
		case m.Cells[i].Rim:
			return mesh.IceColor
		case r.Surface[i] != SurfaceNone:
			return SurfaceColor(r.Surface[i])
		case r.Biome[i] == PolarDesert:
			return polarDesertInk
		case r.Biome[i] != BiomeNone:
			return bareLandInk
		}
		return openWaterInk
	})
}

// WetnessRender draws the "wetness" render: each land cell by its wetness
// signals (river in blue, lake shore in cyan, surplus in green, two or more
// in purple), permanent ice white, other flats pale and other land grey,
// water pale blue (pack ice in its color), the rim as ice, and the edges
// whose river class is at least minClass in river blue.
func WetnessRender(f *field.Field, m *mesh.Mesh, r *Result, class []edges.RiverClass, minClass edges.RiverClass) *image.RGBA {
	img := mesh.CellRender(f, m, func(i int) color.RGBA {
		switch {
		case m.Cells[i].Rim:
			return mesh.IceColor
		case r.Biome[i] == BiomeNone:
			if r.Surface[i] == PackIce {
				return SurfaceColor(PackIce)
			}
			return openWaterInk
		case r.Surface[i].IsIce():
			return SurfaceColor(Glacier)
		}
		switch w := r.Wetness[i]; w {
		case 0:
			if r.Landform[i] == Flats {
				return dryFlatsInk
			}
			return dryLandInk
		case WetRiver:
			return wetRiverInk
		case WetShore:
			return wetShoreInk
		case WetSurplus:
			return wetSurplusInk
		}
		return wetManyInk
	})
	if len(class) == len(m.Edges) {
		s := float64(mesh.RenderScale(f, m))
		mesh.DrawEdges(img, f, m, max(1, s), func(e int) (color.RGBA, bool) {
			return riverInk, class[e] >= minClass && class[e] != edges.RiverNone
		})
	}
	return img
}
