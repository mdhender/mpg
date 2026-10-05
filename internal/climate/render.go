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
	img := mesh.CellRender(f, m, func(i int) color.RGBA { return TemperatureRamp.At(r.Temperature[i]) })
	width := max(1.5, float64(mesh.RenderScale(f, m))/2)
	mesh.DrawEdges(img, f, m, width, func(e int) (color.RGBA, bool) {
		a, b := m.Edges[e].Cells[0], m.Edges[e].Cells[1]
		return coastInk, b != mesh.Boundary && r.Ocean[a] != r.Ocean[b]
	})
	return img
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
