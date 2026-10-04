// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package elevation

import (
	"image"
	"image/color"
	"math"

	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/internal/render"
	"github.com/mdhender/mpg/internal/topo"
)

// Hotspot mark colors.
var (
	coneRing  = color.RGBA{R: 0xd0, G: 0x10, B: 0x10, A: 0xff}
	peakDot   = color.RGBA{R: 0x50, G: 0x00, B: 0x00, A: 0xff}
	swellRing = color.RGBA{R: 0xff, G: 0x8c, B: 0x00, A: 0xff}
)

// ZFactor is the hillshade's height exaggeration in the stage render. At 1
// the relief of a 2 km raster barely shows; this makes ranges and hills
// read at a glance. It changes only the picture.
const ZFactor = 10

// WaterShade is the hillshade strength on the sea floor: low, so the floor's
// noise does not distract from the coasts.
const WaterShade = 0.3

// Render draws the elevation stage render: f, in meters, as a hypsometric
// map split at seaLevel (render.Land above, render.Water below) with a
// northwest hillshade, at full strength on land and WaterShade on the sea
// floor. The hillshade wraps east–west, so the seam shades like any other
// column.
func Render(f *field.Field, seaLevel float64) *image.RGBA {
	opts := render.DefaultReliefOptions()
	opts.SeaLevel = seaLevel
	opts.Light.ZFactor = ZFactor
	opts.WaterShade = WaterShade
	return render.Relief(f, opts)
}

// StageRender draws the elevation stage render: Render split at seaLevel,
// with the hotspots marked by MarkHotspots.
func StageRender(f *field.Field, seaLevel float64, hs []Hotspot) *image.RGBA {
	img := Render(f, seaLevel)
	MarkHotspots(img, f, hs)
	return img
}

// MarkHotspots marks each hotspot on img, a render of f: a dashed orange
// ring at the swell's radius, a red ring at the cone's foot, and a dark dot
// at the peak. The rings leave the cone and swell themselves visible. Marks
// that cross the seam are drawn on both sides.
func MarkHotspots(img *image.RGBA, f *field.Field, hs []Hotspot) {
	nx := float64(f.NX())
	line := max(1.5, nx/600)
	dot := max(2, nx/400)
	for _, h := range hs {
		ring(img, f, h.X, h.Y, h.SwellRadiusKm, line, true, swellRing)
		ring(img, f, h.X, h.Y, h.ConeRadiusKm, line, false, coneRing)
		ctr := render.ToPixel(f, h.Point())
		for _, shift := range []float64{-nx, 0, nx} {
			render.Disc(img, render.Pt{X: ctr.X + shift, Y: ctr.Y}, dot, peakDot)
		}
	}
}

// ring draws a circle of radius r km around (x, y) km, unwrapped, three
// times: shifted one circumference west, in place, and one east, so a ring
// that crosses the seam shows on both sides. A dashed ring draws every other
// segment.
func ring(img *image.RGBA, f *field.Field, x, y, r, width float64, dashed bool, c color.RGBA) {
	const segments = 48
	nx := float64(f.NX())
	pts := make([]render.Pt, segments+1)
	for k := range pts {
		sin, cos := fmath.Sincos(2 * math.Pi * float64(k) / segments)
		pts[k] = render.ToPixel(f, topo.Point{X: fmath.MulAdd(r, cos, x), Y: fmath.MulAdd(r, sin, y)})
	}
	for _, shift := range []float64{-nx, 0, nx} {
		for k := range segments {
			if dashed && k%2 == 1 {
				continue
			}
			a, b := pts[k], pts[k+1]
			render.Line(img, render.Pt{X: a.X + shift, Y: a.Y}, render.Pt{X: b.X + shift, Y: b.Y}, width, c)
		}
	}
}
