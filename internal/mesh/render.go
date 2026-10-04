// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package mesh

import (
	"image"
	"image/color"
	"math"

	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/internal/render"
	"github.com/mdhender/mpg/internal/topo"
)

// Overlay colors.
var (
	edgeInk = color.RGBA{R: 0x1a, G: 0x1a, B: 0x1a, A: 0xff}
	siteInk = color.RGBA{R: 0xc0, G: 0x10, B: 0x10, A: 0xff}
	rimInk  = color.RGBA{R: 0x00, G: 0xc8, B: 0xff, A: 0xff}
)

// PixelsPerCell is about how many pixels across a cell the stage render
// aims for, so outlines and sites stay readable.
const PixelsPerCell = 8

// RenderScale returns the stage render's magnification over a render of f
// (one pixel per sample): the smallest whole number that gives a cell of
// m's mean size about PixelsPerCell pixels across, and at least 1.
func RenderScale(f *field.Field, m *Mesh) int {
	side := math.Sqrt(m.cyl.W() * m.cyl.H() / float64(len(m.Cells)))
	return max(1, int(math.Ceil(PixelsPerCell*min(f.PitchX(), f.PitchY())/side)))
}

// StageRender draws the mesh stage render: base, a render of f with one
// pixel per sample (such as the elevation render), magnified by
// RenderScale, with every cell outline, every site, and the rim's inner
// edges drawn over it. Outlines that cross the seam are drawn on both
// sides. It does not modify base.
func StageRender(base *image.RGBA, f *field.Field, m *Mesh) *image.RGBA {
	s := RenderScale(f, m)
	img := magnify(base, s)
	sx, sy := float64(s)/f.PitchX(), float64(s)/f.PitchY()
	px := func(p topo.Point) render.Pt { return render.Pt{X: fmath.Mul(p.X, sx), Y: fmath.Mul(p.Y, sy)} }
	w := m.cyl.W()
	wpx := w * sx

	for _, y := range []float64{m.cyl.Rim(), m.cyl.H() - m.cyl.Rim()} {
		render.Line(img, px(topo.Point{X: 0, Y: y}), px(topo.Point{X: w, Y: y}), 1, rimInk)
	}
	for _, e := range m.Edges {
		a := m.Corners[e.Corners[0]].Point
		dx, dy := m.cyl.Delta(a, m.Corners[e.Corners[1]].Point)
		pa, pb := px(a), px(topo.Point{X: a.X + dx, Y: a.Y + dy})
		render.Line(img, pa, pb, 1, edgeInk)
		if bx := a.X + dx; bx < 0 || bx >= w { // the edge crosses the seam
			shift := wpx
			if bx >= w {
				shift = -wpx
			}
			render.Line(img, render.Pt{X: pa.X + shift, Y: pa.Y}, render.Pt{X: pb.X + shift, Y: pb.Y}, 1, edgeInk)
		}
	}
	dot := max(0.75, float64(s)/2)
	for _, c := range m.Cells {
		render.Disc(img, px(c.Site), dot, siteInk)
	}
	return img
}

// magnify returns src scaled up s times by pixel replication.
func magnify(src *image.RGBA, s int) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx()*s, b.Dy()*s))
	for y := range dst.Rect.Dy() {
		for x := range dst.Rect.Dx() {
			dst.SetRGBA(x, y, src.RGBAAt(b.Min.X+x/s, b.Min.Y+y/s))
		}
	}
	return dst
}
