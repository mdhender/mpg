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

// The mesh renders are drawn for two scales at once: full size, where each
// cell is about PixelsPerCell pixels across and every outline shows, and a
// sweep sheet tile, shrunk by a box filter to a tenth of that or less,
// where single outlines blur into a tint. So outlines are blended, not
// opaque (they darken the terrain evenly rather than hiding it), the rim is
// a solid fill that survives any shrinking, and the short-edge marks scale
// with the cell size so they stay a pixel or two across on a sheet.

// Overlay colors.
var (
	edgeInk  = color.RGBA{R: 0x10, G: 0x10, B: 0x10, A: 0xff}
	iceFill  = color.RGBA{R: 0xee, G: 0xf5, B: 0xfa, A: 0xff}
	iceEdge  = color.RGBA{R: 0xc2, G: 0xd8, B: 0xe6, A: 0xff}
	iceFront = color.RGBA{R: 0x4f, G: 0x86, B: 0xb0, A: 0xff}
)

// edgeAlpha is the opacity of the playable cell outlines over the
// elevation, out of 255.
const edgeAlpha = 110

// PixelsPerCell is about how many pixels across a cell the stage render
// aims for, so outlines stay readable.
const PixelsPerCell = 8

// RenderScale returns the stage render's magnification over a render of f
// (one pixel per sample): the smallest whole number that gives a cell of
// m's mean size about PixelsPerCell pixels across, and at least 1.
func RenderScale(f *field.Field, m *Mesh) int {
	side := math.Sqrt(m.cyl.W() * m.cyl.H() / float64(len(m.Cells)))
	return max(1, int(math.Ceil(PixelsPerCell*min(f.PitchX(), f.PitchY())/side)))
}

// canvas is a stage render's pixel frame: sx and sy pixels per km.
type canvas struct {
	m      *Mesh
	sx, sy float64
}

func newCanvas(f *field.Field, m *Mesh) (canvas, int) {
	s := RenderScale(f, m)
	return canvas{m: m, sx: float64(s) / f.PitchX(), sy: float64(s) / f.PitchY()}, s
}

func (cv canvas) px(p topo.Point) render.Pt {
	return render.Pt{X: fmath.Mul(p.X, cv.sx), Y: fmath.Mul(p.Y, cv.sy)}
}

// cellPx is the side of a cell of mean size in pixels.
func (cv canvas) cellPx() float64 {
	m := cv.m
	return fmath.Mul(math.Sqrt(m.cyl.W()*m.cyl.H()/float64(len(m.Cells))), min(cv.sx, cv.sy))
}

// fillCell fills cell i in ink; a cell that crosses the seam is filled on
// both sides.
func (cv canvas) fillCell(img *image.RGBA, i int, ink color.RGBA) {
	m := cv.m
	w := m.cyl.W()
	poly := m.Polygon(i)
	pts := make([]render.Pt, len(poly))
	lo, hi := math.Inf(1), math.Inf(-1)
	for k, p := range poly {
		pts[k] = cv.px(p)
		lo, hi = min(lo, p.X), max(hi, p.X)
	}
	render.FillPolygon(img, pts, ink)
	var shift float64
	switch {
	case lo < 0:
		shift = fmath.Mul(w, cv.sx)
	case hi >= w:
		shift = -fmath.Mul(w, cv.sx)
	}
	if shift != 0 {
		for k := range pts {
			pts[k].X += shift
		}
		render.FillPolygon(img, pts, ink)
	}
}

// drawEdge draws edge e on img in ink, width pixels wide; an edge that
// crosses the seam is drawn on both sides.
func (cv canvas) drawEdge(img *image.RGBA, e int, width float64, ink color.RGBA) {
	m := cv.m
	w := m.cyl.W()
	edge := m.Edges[e]
	a := m.Corners[edge.Corners[0]].Point
	dx, dy := m.cyl.Delta(a, m.Corners[edge.Corners[1]].Point)
	pa, pb := cv.px(a), cv.px(topo.Point{X: a.X + dx, Y: a.Y + dy})
	render.Line(img, pa, pb, width, ink)
	if bx := a.X + dx; bx < 0 || bx >= w { // the edge crosses the seam
		shift := fmath.Mul(w, cv.sx)
		if bx >= w {
			shift = -shift
		}
		render.Line(img, render.Pt{X: pa.X + shift, Y: pa.Y}, render.Pt{X: pb.X + shift, Y: pb.Y}, width, ink)
	}
}

// edgeMidpoint returns edge e's midpoint, x wrapped.
func (m *Mesh) edgeMidpoint(e int) topo.Point {
	a, b := m.Corners[m.Edges[e].Corners[0]].Point, m.Corners[m.Edges[e].Corners[1]].Point
	dx, dy := m.cyl.Delta(a, b)
	return topo.Point{X: m.cyl.WrapX(a.X + fmath.Mul(dx, 0.5)), Y: a.Y + fmath.Mul(dy, 0.5)}
}

// disc paints a disc of radius r pixels at p, and again across the seam
// where it overhangs an edge of img.
func (cv canvas) disc(img *image.RGBA, p topo.Point, r float64, ink color.RGBA) {
	c := cv.px(p)
	render.Disc(img, c, r, ink)
	wpx := fmath.Mul(cv.m.cyl.W(), cv.sx)
	if c.X-r < 0 {
		render.Disc(img, render.Pt{X: c.X + wpx, Y: c.Y}, r, ink)
	}
	if c.X+r >= wpx {
		render.Disc(img, render.Pt{X: c.X - wpx, Y: c.Y}, r, ink)
	}
}

// drawIce fills every rim cell as the ice sheet, outlines the rim cells
// faintly, and draws the ice front (the edges between rim and playable
// cells) frontWidth pixels wide.
func (cv canvas) drawIce(img *image.RGBA, frontWidth float64) {
	m := cv.m
	for i, c := range m.Cells {
		if c.Rim {
			cv.fillCell(img, i, iceFill)
		}
	}
	for e, edge := range m.Edges {
		if a, b := edge.Cells[0], edge.Cells[1]; m.Cells[a].Rim && (b == Boundary || m.Cells[b].Rim) {
			cv.drawEdge(img, e, 1, iceEdge)
		}
	}
	for e, edge := range m.Edges {
		if b := edge.Cells[1]; b != Boundary && m.Cells[edge.Cells[0]].Rim != m.Cells[b].Rim {
			cv.drawEdge(img, e, frontWidth, iceFront)
		}
	}
}

// drawPlayableOutlines blends the outlines of the playable cells (every
// edge with a playable cell on either side) onto img in ink at alpha (out
// of 255).
func (cv canvas) drawPlayableOutlines(img *image.RGBA, ink color.RGBA, alpha uint8) {
	m := cv.m
	layer := image.NewRGBA(img.Rect)
	for e, edge := range m.Edges {
		if b := edge.Cells[1]; !m.Cells[edge.Cells[0]].Rim || (b != Boundary && !m.Cells[b].Rim) {
			cv.drawEdge(layer, e, 1, ink)
		}
	}
	blend(img, layer, alpha)
}

// blend paints each pixel of layer that is not transparent onto img at
// alpha (out of 255), in integer arithmetic.
func blend(img, layer *image.RGBA, alpha uint8) {
	a := int(alpha)
	for o := 0; o < len(layer.Pix); o += 4 {
		if layer.Pix[o+3] == 0 {
			continue
		}
		for c := range 3 {
			img.Pix[o+c] = uint8((int(img.Pix[o+c])*(255-a) + int(layer.Pix[o+c])*a + 127) / 255)
		}
	}
}

// StageRender draws the mesh stage render: base, a render of f with one
// pixel per sample (such as the elevation render), magnified by
// RenderScale, with the rim cells drawn as the ice sheet (a pale fill with
// faint outlines, and the ice front, where rim meets playable cells, in
// steel blue) and the playable cells' outlines blended over the terrain.
// Sites are not drawn. It does not modify base.
func StageRender(base *image.RGBA, f *field.Field, m *Mesh) *image.RGBA {
	cv, s := newCanvas(f, m)
	img := magnify(base, s)
	cv.drawPlayableOutlines(img, edgeInk, edgeAlpha)
	cv.drawIce(img, max(1.5, float64(s)))
	return img
}

// Short-edge render inks.
var (
	stretchInk  = color.RGBA{R: 0xd0, G: 0x10, B: 0x10, A: 0xff}
	stretchEdge = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	fourWayInk  = color.RGBA{R: 0x10, G: 0x50, B: 0xc0, A: 0xff}
)

// ShortRender draws the mesh stage's short-edge render at StageRender's
// size, over base (as for StageRender) faded to a pale gray: the rim as
// ice, faint playable outlines, each corner off the rim boundary that
// touches four cells (a collapsed edge) as a small blue dot, and each
// stretched edge in white on a red disc about a cell across, so the rare
// stretches stand out at sheet scale. It does not modify base.
func ShortRender(base *image.RGBA, f *field.Field, m *Mesh) *image.RGBA {
	cv, s := newCanvas(f, m)
	img := magnify(fade(base), s)
	cv.drawPlayableOutlines(img, edgeInk, 60)
	cv.drawIce(img, max(1.5, float64(s)))
	dot := max(1, float64(s)/2)
	for _, k := range m.Corners {
		if !k.Boundary && len(k.Cells) == 4 {
			cv.disc(img, k.Point, dot, fourWayInk)
		}
	}
	r := max(3, cv.cellPx())
	for _, e := range m.Stretched {
		cv.disc(img, m.edgeMidpoint(e), r, stretchInk)
	}
	for _, e := range m.Stretched {
		cv.drawEdge(img, e, max(1, float64(s)/2), stretchEdge)
	}
	return img
}

// fade returns img in gray, lightened two thirds of the way to white.
func fade(img *image.RGBA) *image.RGBA {
	out := image.NewRGBA(img.Rect)
	for o := 0; o < len(img.Pix); o += 4 {
		r, g, b := int(img.Pix[o]), int(img.Pix[o+1]), int(img.Pix[o+2])
		y := (299*r + 587*g + 114*b + 500) / 1000
		v := uint8((y + 2*255 + 1) / 3)
		out.Pix[o], out.Pix[o+1], out.Pix[o+2], out.Pix[o+3] = v, v, v, 0xff
	}
	return out
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

// AreaRender draws the mesh stage's area heatmap at StageRender's size: each
// cell, rim cells included, filled by its area as a multiple of areaKm2
// (A) on a diverging ramp, blue for A/2 and below, white at A, red for 3A/2
// and above, with blended outlines and the ice front (see CellRender).
func AreaRender(f *field.Field, m *Mesh, areaKm2 float64) *image.RGBA {
	return CellRender(f, m, func(i int) color.RGBA { return areaRamp.At(m.Area(i) / areaKm2) })
}

// CellRender draws a cell map at StageRender's size: every cell, rim cells
// included, filled with fill(cell id), then every edge blended over the
// fills in a dark ink at about a third of full opacity, then the ice
// front (the edges between rim and playable cells) in steel blue. Cells
// that cross the seam are drawn on both sides. Later stages use it for
// their per-cell renders.
func CellRender(f *field.Field, m *Mesh, fill func(cell int) color.RGBA) *image.RGBA {
	cv, s := newCanvas(f, m)
	img := image.NewRGBA(image.Rect(0, 0, f.NX()*s, f.NY()*s))
	for i := range m.Cells {
		cv.fillCell(img, i, fill(i))
	}
	layer := image.NewRGBA(img.Rect)
	for e := range m.Edges {
		cv.drawEdge(layer, e, 1, edgeInk)
	}
	blend(img, layer, 90)
	for e, edge := range m.Edges {
		if b := edge.Cells[1]; b != Boundary && m.Cells[edge.Cells[0]].Rim != m.Cells[b].Rim {
			cv.drawEdge(img, e, max(1.5, float64(s)), iceFront)
		}
	}
	return img
}

// Mark paints a disc on img, a render at StageRender's size over f (as
// CellRender returns), centered at p: radius cells mean cell sides across,
// but at least minPx pixels, in ink, drawn again across the seam where it
// overhangs an edge. Later stages use it to mark points such as volcanoes.
func Mark(img *image.RGBA, f *field.Field, m *Mesh, p topo.Point, radius, minPx float64, ink color.RGBA) {
	cv, _ := newCanvas(f, m)
	cv.disc(img, p, max(minPx, fmath.Mul(radius, cv.cellPx())), ink)
}

// IceColor is the fill of the polar ice sheet in the mesh renders, for
// later stages that draw rim cells as ice with CellRender.
var IceColor = iceFill

// IceFrontColor is the ink of the ice front (the edges between rim and
// playable cells) in the mesh renders, for the player map.
var IceFrontColor = iceFront

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

// DrawEdges draws, on img (a render at StageRender's size over f, as
// CellRender returns), every edge e of m for which ink returns ok, in the
// color it returns, width pixels wide, in edge id order; an edge that
// crosses the seam is drawn on both sides. Later stages use it for their
// per-edge renders.
func DrawEdges(img *image.RGBA, f *field.Field, m *Mesh, width float64, ink func(e int) (c color.RGBA, ok bool)) {
	cv, _ := newCanvas(f, m)
	for e := range m.Edges {
		if c, ok := ink(e); ok {
			cv.drawEdge(img, e, width, c)
		}
	}
}
