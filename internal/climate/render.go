// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package climate

import (
	"image"
	"image/color"

	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/mesh"
	"github.com/mdhender/mpg/internal/render"
)

// TemperatureRamp colors a temperature in °C on a fixed scale, so sweep
// tiles compare: deep violet at −40 °C and below, through blue (−25 °C)
// and pale blue (−10 °C) to near white at freezing, then pale yellow
// (10 °C), orange (20 °C), and dark red at 30 °C and above. Freezing is
// the white center, so cold and warm read apart at a glance.
var TemperatureRamp = render.NewRamp(
	render.Stop{Value: -40, Color: color.RGBA{0x3b, 0x0f, 0x70, 0xff}},
	render.Stop{Value: -25, Color: color.RGBA{0x2c, 0x4f, 0xa3, 0xff}},
	render.Stop{Value: -10, Color: color.RGBA{0x74, 0xad, 0xd1, 0xff}},
	render.Stop{Value: 0, Color: color.RGBA{0xf2, 0xf6, 0xf9, 0xff}},
	render.Stop{Value: 10, Color: color.RGBA{0xfe, 0xe0, 0x8b, 0xff}},
	render.Stop{Value: 20, Color: color.RGBA{0xf4, 0x6d, 0x43, 0xff}},
	render.Stop{Value: 30, Color: color.RGBA{0xa5, 0x00, 0x26, 0xff}},
)

// coastInk draws the coasts over the temperature render.
var coastInk = color.RGBA{0x20, 0x20, 0x20, 0xff}

// TemperatureRender draws the climate stage render over mesh.CellRender:
// every cell, rim cells included, filled by its temperature on
// TemperatureRamp (the rim is open water at the poles, so it shows as the
// coldest band rather than as the ice sheet), with blended outlines, the
// ice front, and the coasts (edges between an Ocean cell and a land cell)
// in dark ink, so altitude cooling reads on the land.
func TemperatureRender(f *field.Field, m *mesh.Mesh, r *Result) *image.RGBA {
	return cellRender(f, m, r, func(i int) color.RGBA { return TemperatureRamp.At(r.Temperature[i]) })
}

// The mask render's colors: ocean (rim included) blue, land tan.
var (
	maskOcean = color.RGBA{0x4a, 0x8c, 0xc4, 0xff}
	maskLand  = color.RGBA{0xd8, 0xc8, 0x96, 0xff}
)

// MaskRender draws the climate stage's mask render: the cell mask on the
// raster, one pixel per sample of f, ocean (rim included) in blue and land
// (dry basin floors included) in tan.
func MaskRender(f *field.Field, r *Result) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, f.NX(), f.NY()))
	for k, ocean := range r.Mask {
		c := maskLand
		if ocean {
			c = maskOcean
		}
		img.SetRGBA(k%f.NX(), k/f.NX(), c)
	}
	return img
}

// PrecipRamp colors annual precipitation in mm on a fixed scale, so sweep
// tiles compare: dark brown at 0 mm, tan (250 mm), cream (500 mm), pale
// teal (1,000 mm), teal (2,000 mm) and dark teal at 3,500 mm and above.
var PrecipRamp = render.NewRamp(
	render.Stop{Value: 0, Color: color.RGBA{0x8c, 0x51, 0x0a, 0xff}},
	render.Stop{Value: 250, Color: color.RGBA{0xd8, 0xb3, 0x65, 0xff}},
	render.Stop{Value: 500, Color: color.RGBA{0xf6, 0xe8, 0xc3, 0xff}},
	render.Stop{Value: 1000, Color: color.RGBA{0xc7, 0xea, 0xe5, 0xff}},
	render.Stop{Value: 2000, Color: color.RGBA{0x5a, 0xb4, 0xac, 0xff}},
	render.Stop{Value: 3500, Color: color.RGBA{0x01, 0x66, 0x5e, 0xff}},
)

// MoistureRamp colors the share of the sea air's moisture the winds bring,
// 0 to 1, on PrecipRamp's colors: dark brown when dry, dark teal at 1.
var MoistureRamp = render.NewRamp(
	render.Stop{Value: 0, Color: color.RGBA{0x8c, 0x51, 0x0a, 0xff}},
	render.Stop{Value: 0.3, Color: color.RGBA{0xd8, 0xb3, 0x65, 0xff}},
	render.Stop{Value: 0.5, Color: color.RGBA{0xf6, 0xe8, 0xc3, 0xff}},
	render.Stop{Value: 0.7, Color: color.RGBA{0xc7, 0xea, 0xe5, 0xff}},
	render.Stop{Value: 0.85, Color: color.RGBA{0x5a, 0xb4, 0xac, 0xff}},
	render.Stop{Value: 1, Color: color.RGBA{0x01, 0x66, 0x5e, 0xff}},
)

// PETRamp colors potential evapotranspiration in mm per year: white at 0
// (frozen), through pale yellow and orange to dark red at 1,800 mm.
var PETRamp = render.NewRamp(
	render.Stop{Value: 0, Color: color.RGBA{0xf7, 0xf7, 0xf7, 0xff}},
	render.Stop{Value: 600, Color: color.RGBA{0xfe, 0xe0, 0x8b, 0xff}},
	render.Stop{Value: 1200, Color: color.RGBA{0xf4, 0x6d, 0x43, 0xff}},
	render.Stop{Value: 1800, Color: color.RGBA{0xa5, 0x00, 0x26, 0xff}},
)

// RunoffRamp colors annual runoff in mm: PrecipRamp's colors over half its
// range (dark brown at 0, dark teal at 1,750 mm and above).
var RunoffRamp = render.NewRamp(
	render.Stop{Value: 0, Color: color.RGBA{0x8c, 0x51, 0x0a, 0xff}},
	render.Stop{Value: 125, Color: color.RGBA{0xd8, 0xb3, 0x65, 0xff}},
	render.Stop{Value: 250, Color: color.RGBA{0xf6, 0xe8, 0xc3, 0xff}},
	render.Stop{Value: 500, Color: color.RGBA{0xc7, 0xea, 0xe5, 0xff}},
	render.Stop{Value: 1000, Color: color.RGBA{0x5a, 0xb4, 0xac, 0xff}},
	render.Stop{Value: 1750, Color: color.RGBA{0x01, 0x66, 0x5e, 0xff}},
)

// AridityColors colors the UNEP aridity classes, by Aridity: hyper-arid
// dark red, arid orange-red, semi-arid orange, dry sub-humid cream, humid
// green.
var AridityColors = [NumAridity]color.RGBA{
	{0xa5, 0x00, 0x26, 0xff},
	{0xe0, 0x6d, 0x43, 0xff},
	{0xfd, 0xc0, 0x6e, 0xff},
	{0xfe, 0xf0, 0xb0, 0xff},
	{0x56, 0xa8, 0x5d, 0xff},
}

// waterFill is the flat fill of open water in the land-only climate
// renders.
var waterFill = color.RGBA{0x2c, 0x3e, 0x5c, 0xff}

// cellRender draws a per-cell climate render: every cell filled by fill,
// with mesh.CellRender's outlines and ice front, and the coasts in dark ink.
func cellRender(f *field.Field, m *mesh.Mesh, r *Result, fill func(i int) color.RGBA) *image.RGBA {
	img := mesh.CellRender(f, m, fill)
	width := max(1.5, float64(mesh.RenderScale(f, m))/2)
	mesh.DrawEdges(img, f, m, width, func(e int) (color.RGBA, bool) {
		a, b := m.Edges[e].Cells[0], m.Edges[e].Cells[1]
		return coastInk, b != mesh.Boundary && r.Ocean[a] != r.Ocean[b]
	})
	return img
}

// landRender is cellRender with open water (rim included) in waterFill.
func landRender(f *field.Field, m *mesh.Mesh, r *Result, fill func(i int) color.RGBA) *image.RGBA {
	return cellRender(f, m, r, func(i int) color.RGBA {
		if r.Ocean[i] {
			return waterFill
		}
		return fill(i)
	})
}

// PrecipRender draws render variant "precip": every cell, rim and ocean
// included (the latitude bands show over the sea), by its precipitation on
// PrecipRamp, with the coasts inked.
func PrecipRender(f *field.Field, m *mesh.Mesh, r *Result) *image.RGBA {
	return cellRender(f, m, r, func(i int) color.RGBA { return PrecipRamp.At(r.Precipitation[i]) })
}

// MoistureRender draws render variant "moisture": land cells by the share
// of the sea air's moisture the winds bring them on MoistureRamp, which
// shows the windward coasts, the rain shadows and the dry interiors
// directly; water flat.
func MoistureRender(f *field.Field, m *mesh.Mesh, r *Result) *image.RGBA {
	return landRender(f, m, r, func(i int) color.RGBA { return MoistureRamp.At(r.Moisture[i]) })
}

// PETRender draws render variant "pet": every cell by its potential
// evapotranspiration on PETRamp.
func PETRender(f *field.Field, m *mesh.Mesh, r *Result) *image.RGBA {
	return cellRender(f, m, r, func(i int) color.RGBA { return PETRamp.At(r.PET[i]) })
}

// RunoffRender draws render variant "runoff": land cells by their runoff on
// RunoffRamp; water flat.
func RunoffRender(f *field.Field, m *mesh.Mesh, r *Result) *image.RGBA {
	return landRender(f, m, r, func(i int) color.RGBA { return RunoffRamp.At(r.Runoff[i]) })
}

// AridityRender draws render variant "aridity": land cells by their UNEP
// aridity class in AridityColors; water flat.
func AridityRender(f *field.Field, m *mesh.Mesh, r *Result) *image.RGBA {
	return landRender(f, m, r, func(i int) color.RGBA { return AridityColors[AridityOf(r.Aridity[i])] })
}
