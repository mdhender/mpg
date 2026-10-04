// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package mesh

import (
	"image"
	"image/color"
	"image/draw"
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
	drawOutlines(img, m, sx, sy, edgeInk)
	dot := max(0.75, float64(s)/2)
	for _, c := range m.Cells {
		render.Disc(img, px(c.Site), dot, siteInk)
	}
	return img
}

// drawOutlines draws every cell outline on img in ink, and the rim's inner
// edges, at sx and sy pixels per km. Outlines that cross the seam are drawn
// on both sides.
func drawOutlines(img *image.RGBA, m *Mesh, sx, sy float64, ink color.RGBA) {
	px := func(p topo.Point) render.Pt { return render.Pt{X: fmath.Mul(p.X, sx), Y: fmath.Mul(p.Y, sy)} }
	w := m.cyl.W()
	for _, y := range []float64{m.cyl.Rim(), m.cyl.H() - m.cyl.Rim()} {
		render.Line(img, px(topo.Point{X: 0, Y: y}), px(topo.Point{X: w, Y: y}), 1, rimInk)
	}
	for e := range m.Edges {
		drawEdge(img, m, e, sx, sy, 1, ink)
	}
}

// drawEdge draws edge e on img in ink, width pixels wide, at sx and sy
// pixels per km; an edge that crosses the seam is drawn on both sides.
func drawEdge(img *image.RGBA, m *Mesh, e int, sx, sy, width float64, ink color.RGBA) {
	px := func(p topo.Point) render.Pt { return render.Pt{X: fmath.Mul(p.X, sx), Y: fmath.Mul(p.Y, sy)} }
	w := m.cyl.W()
	edge := m.Edges[e]
	a := m.Corners[edge.Corners[0]].Point
	dx, dy := m.cyl.Delta(a, m.Corners[edge.Corners[1]].Point)
	pa, pb := px(a), px(topo.Point{X: a.X + dx, Y: a.Y + dy})
	render.Line(img, pa, pb, width, ink)
	if bx := a.X + dx; bx < 0 || bx >= w { // the edge crosses the seam
		shift := w * sx
		if bx >= w {
			shift = -shift
		}
		render.Line(img, render.Pt{X: pa.X + shift, Y: pa.Y}, render.Pt{X: pb.X + shift, Y: pb.Y}, width, ink)
	}
}

// Short-edge render inks.
var (
	shortBase    = color.RGBA{R: 0xf4, G: 0xf1, B: 0xea, A: 0xff}
	shortOutline = color.RGBA{R: 0x9a, G: 0x9a, B: 0x9a, A: 0xff}
	stretchInk   = color.RGBA{R: 0xd0, G: 0x10, B: 0x10, A: 0xff}
	fourWayInk   = color.RGBA{R: 0x10, G: 0x50, B: 0xc0, A: 0xff}
)

// ShortRender draws the mesh stage's short-edge render at StageRender's
// size: thin gray outlines on a plain ground, with what the short-edge
// collapse did marked on them: each corner off the rim boundary that
// touches four cells (a collapsed edge, or a degree-cap collapse) as a blue
// dot, and each stretched edge in red with a red disc at its midpoint, so
// the rare stretches stand out at sheet scale. It also draws the rim's
// inner edges.
func ShortRender(f *field.Field, m *Mesh) *image.RGBA {
	s := RenderScale(f, m)
	img := image.NewRGBA(image.Rect(0, 0, f.NX()*s, f.NY()*s))
	draw.Draw(img, img.Rect, image.NewUniform(shortBase), image.Point{}, draw.Src)
	sx, sy := float64(s)/f.PitchX(), float64(s)/f.PitchY()
	px := func(p topo.Point) render.Pt { return render.Pt{X: fmath.Mul(p.X, sx), Y: fmath.Mul(p.Y, sy)} }
	drawOutlines(img, m, sx, sy, shortOutline)
	dot := max(1, float64(s)/2)
	for _, k := range m.Corners {
		if !k.Boundary && len(k.Cells) == 4 {
			render.Disc(img, px(k.Point), dot, fourWayInk)
		}
	}
	for _, e := range m.Stretched {
		drawEdge(img, m, e, sx, sy, max(2, float64(s)/3), stretchInk)
		a, b := m.Corners[m.Edges[e].Corners[0]].Point, m.Corners[m.Edges[e].Corners[1]].Point
		dx, dy := m.cyl.Delta(a, b)
		render.Disc(img, px(topo.Point{X: m.cyl.WrapX(a.X + fmath.Mul(dx, 0.5)), Y: a.Y + fmath.Mul(dy, 0.5)}), 2*dot+1, stretchInk)
	}
	return img
}

// areaRamp colors a cell by its area as a multiple of A: blue below,
// white at A, red above, saturating at A/2 and 3A/2.
var areaRamp = render.NewRamp(
	render.Stop{Value: 0.5, Color: color.RGBA{R: 0x1f, G: 0x3a, B: 0x93, A: 0xff}},
	render.Stop{Value: 0.85, Color: color.RGBA{R: 0x8f, G: 0xb8, B: 0xe0, A: 0xff}},
	render.Stop{Value: 1, Color: color.RGBA{R: 0xf7, G: 0xf7, B: 0xf7, A: 0xff}},
	render.Stop{Value: 1.15, Color: color.RGBA{R: 0xf2, G: 0xa4, B: 0x82, A: 0xff}},
	render.Stop{Value: 1.5, Color: color.RGBA{R: 0x9e, G: 0x10, B: 0x1f, A: 0xff}},
)

// outlineInk is the area render's cell outline color.
var outlineInk = color.RGBA{R: 0x60, G: 0x60, B: 0x60, A: 0xff}

// AreaRender draws the mesh stage's area heatmap at StageRender's size: each
// cell filled by its area as a multiple of areaKm2 (A) on a diverging ramp,
// blue for A/2 and below, white at A, red for 3A/2 and above, with thin
// outlines and the rim's inner edges. Cells that cross the seam are drawn
// on both sides.
func AreaRender(f *field.Field, m *Mesh, areaKm2 float64) *image.RGBA {
	s := RenderScale(f, m)
	img := image.NewRGBA(image.Rect(0, 0, f.NX()*s, f.NY()*s))
	sx, sy := float64(s)/f.PitchX(), float64(s)/f.PitchY()
	w := m.cyl.W()
	wpx := w * sx
	for i := range m.Cells {
		poly := m.Polygon(i)
		pts := make([]render.Pt, len(poly))
		lo, hi := math.Inf(1), math.Inf(-1)
		for k, p := range poly {
			pts[k] = render.Pt{X: fmath.Mul(p.X, sx), Y: fmath.Mul(p.Y, sy)}
			lo, hi = min(lo, p.X), max(hi, p.X)
		}
		ink := areaRamp.At(m.Area(i) / areaKm2)
		render.FillPolygon(img, pts, ink)
		var shift float64
		switch {
		case lo < 0:
			shift = wpx
		case hi >= w:
			shift = -wpx
		}
		if shift != 0 {
			moved := make([]render.Pt, len(pts))
			for k, p := range pts {
				moved[k] = render.Pt{X: p.X + shift, Y: p.Y}
			}
			render.FillPolygon(img, moved, ink)
		}
	}
	drawOutlines(img, m, sx, sy, outlineInk)
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
