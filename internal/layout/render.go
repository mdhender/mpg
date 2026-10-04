// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package layout

import (
	"image"
	"image/color"
	"math"

	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/internal/render"
	"github.com/mdhender/mpg/internal/topo"
)

// Render colors.
var (
	attractorRing = color.RGBA{R: 0x3a, G: 0x1a, B: 0x10, A: 0xff}
	attractorDot  = color.RGBA{R: 0x10, G: 0x10, B: 0x10, A: 0xff}
	repulsorRing  = color.RGBA{R: 0xff, G: 0xd7, B: 0x00, A: 0xff}
	bandLine      = color.RGBA{R: 0x9a, G: 0xa8, B: 0xb8, A: 0xff}
)

// Render draws the layout stage render: the bias field colored by
// render.BlueRed on its fixed domain [−1, 1] (blue oceanic, white zero, red
// continental), with the inner edges of the rim and of the falloff band as
// gray lines, each attractor as a dark dot with a ring at its radius, and
// each repulsor as a yellow ring at its reach. Marks that cross the seam are
// drawn on both sides.
func Render(bias *field.Field, l *Layout) *image.RGBA {
	img := render.Colorize(bias, render.BlueRed.At)
	nx := float64(bias.NX())
	line := max(1, nx/500)
	dot := max(1.5, nx/300)
	c := bias.Cylinder()

	for _, y := range []float64{c.Rim(), c.Rim() + c.Falloff(), c.H() - c.Rim() - c.Falloff(), c.H() - c.Rim()} {
		p := render.ToPixel(bias, topo.Point{X: 0, Y: y})
		render.Line(img, render.Pt{X: 0, Y: p.Y}, render.Pt{X: nx, Y: p.Y}, max(1, line/2), bandLine)
	}
	for _, r := range l.Repulsors {
		circle(img, bias, r.X, r.Y, r.RadiusKm, line, repulsorRing)
	}
	for _, a := range l.Attractors {
		circle(img, bias, a.X, a.Y, a.RadiusKm, line, attractorRing)
	}
	for _, a := range l.Attractors {
		ctr := render.ToPixel(bias, topo.Point{X: a.X, Y: a.Y})
		for _, shift := range []float64{-nx, 0, nx} {
			render.Disc(img, render.Pt{X: ctr.X + shift, Y: ctr.Y}, dot, attractorDot)
		}
	}
	return img
}

// circle draws a ring of radius r km around (x, y) km, unwrapped, three times:
// shifted one circumference west, in place, and one east, so a ring that
// crosses the seam shows on both sides.
func circle(img *image.RGBA, f *field.Field, x, y, r, width float64, c color.RGBA) {
	const segments = 72
	nx := float64(f.NX())
	pts := make([]render.Pt, segments+1)
	for k := range pts {
		sin, cos := fmath.Sincos(2 * math.Pi * float64(k) / segments)
		pts[k] = render.ToPixel(f, topo.Point{X: fmath.MulAdd(r, cos, x), Y: fmath.MulAdd(r, sin, y)})
	}
	for _, shift := range []float64{-nx, 0, nx} {
		for k := range segments {
			a, b := pts[k], pts[k+1]
			render.Line(img, render.Pt{X: a.X + shift, Y: a.Y}, render.Pt{X: b.X + shift, Y: b.Y}, width, c)
		}
	}
}
