// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package edges

import (
	"image"
	"image/color"
	"math"

	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/internal/mesh"
	"github.com/mdhender/mpg/internal/render"
	"github.com/mdhender/mpg/internal/topo"
)

// The base fills of the edge renders: pale land and water, so the edge
// colors carry the render; the rim is the mesh renders' ice.
var (
	landBase  = color.RGBA{0xe6, 0xe0, 0xcc, 0xff}
	waterBase = color.RGBA{0xc6, 0xdb, 0xec, 0xff}
)

// baseColor returns the edge renders' fill of cell i.
func baseColor(m *mesh.Mesh, water []Water, i int) color.RGBA {
	switch {
	case m.Cells[i].Rim:
		return mesh.IceColor
	case water[i] != WaterNone:
		return waterBase
	}
	return landBase
}

// GradeRamp colors an incline's magnitude in percent: light orange at 1%,
// orange at 2%, red at 5%, dark red at 10%, and dark purple from 20% to the
// cap. Most land grades are a few percent, so the ramp saturates early.
var GradeRamp = render.NewRamp(
	render.Stop{Value: 1, Color: color.RGBA{0xfd, 0xd0, 0x8a, 0xff}},
	render.Stop{Value: 2, Color: color.RGBA{0xf4, 0x8c, 0x3c, 0xff}},
	render.Stop{Value: 5, Color: color.RGBA{0xd7, 0x30, 0x1f, 0xff}},
	render.Stop{Value: 10, Color: color.RGBA{0x99, 0x00, 0x0d, 0xff}},
	render.Stop{Value: 20, Color: color.RGBA{0x5a, 0x00, 0x3a, 0xff}},
	render.Stop{Value: 100, Color: color.RGBA{0x20, 0x00, 0x30, 0xff}},
)

// InclineRender draws the edge stage render over mesh.CellRender: land
// and water in pale fills and the rim as ice, then every land–land edge
// with a grade of at least 1% in GradeRamp's color of |grade|, about a
// sample wide, the steeper drawn last so they stay on top. Grades over
// water exist in the data but are not drawn.
func InclineRender(f *field.Field, m *mesh.Mesh, d *Data, water []Water) *image.RGBA {
	img := mesh.CellRender(f, m, func(i int) color.RGBA { return baseColor(m, water, i) })
	land := func(i int) bool { return !m.Cells[i].Rim && water[i] == WaterNone }
	width := max(1.5, float64(mesh.RenderScale(f, m)))
	for k := 1; k <= len(GradeBreaks); k++ {
		mesh.DrawEdges(img, f, m, width, func(e int) (color.RGBA, bool) {
			me := m.Edges[e]
			if me.OnBoundary() || !land(me.Cells[0]) || !land(me.Cells[1]) || gradeBucket(d.Edges[e].Incline) != k {
				return color.RGBA{}, false
			}
			return GradeRamp.At(d.Edges[e].Incline.Abs().Percent()), true
		})
	}
	return img
}

// Passability render inks.
var (
	impassableInk = color.RGBA{0xd0, 0x18, 0x18, 0xff}
	coastInk      = color.RGBA{0x10, 0x2a, 0x6a, 0xff}
)

// PassabilityRender draws the edge stage's passability render over
// mesh.CellRender: land and water in pale fills and the rim as ice, every
// coast edge in navy, and every impassable edge (the rim's edges and the
// rim boundary) in red, about a sample wide.
func PassabilityRender(f *field.Field, m *mesh.Mesh, d *Data, water []Water) *image.RGBA {
	img := mesh.CellRender(f, m, func(i int) color.RGBA { return baseColor(m, water, i) })
	s := float64(mesh.RenderScale(f, m))
	mesh.DrawEdges(img, f, m, max(1, s*0.75), func(e int) (color.RGBA, bool) { return coastInk, d.Edges[e].Coast })
	mesh.DrawEdges(img, f, m, max(1.5, s), func(e int) (color.RGBA, bool) { return impassableInk, !d.Edges[e].Passable })
	return img
}

// DirectionColors are the compass render's colors, by direction code: a
// hue wheel from red (N) clockwise.
var DirectionColors = [numDirections]color.RGBA{
	{0xd6, 0x27, 0x28, 0xff}, // N
	{0xff, 0x7f, 0x0e, 0xff}, // NE
	{0xbc, 0xa0, 0x00, 0xff}, // E
	{0x2c, 0xa0, 0x2c, 0xff}, // SE
	{0x17, 0xa0, 0xb0, 0xff}, // S
	{0x1f, 0x5f, 0xd4, 0xff}, // SW
	{0x80, 0x40, 0xc0, 0xff}, // W
	{0xd0, 0x30, 0xa0, 0xff}, // NW
}

// Compass crop size, in mean cell sides, and scale.
const (
	compassCellsX = 16
	compassCellsY = 11
	compassPxCell = 80
)

// CompassRender draws a zoomed crop for checking the compass assignment:
// a window compassCellsX × compassCellsY mean cell sides at compassPxCell
// pixels per side, centered on the land cell nearest the middle of the map
// (the middle itself when there is no land), clamped north–south to the
// map and wrapped east–west. Cells are filled pale (land, water, ice) and
// outlined; from each site a tick runs a quarter of the way toward
// each neighbor's site in the direction's color (DirectionColors), and the
// direction's name is written just beyond it.
func CompassRender(m *mesh.Mesh, d *Data, water []Water) *image.RGBA {
	cyl := m.Cylinder()
	w, h := cyl.W(), cyl.H()
	side := math.Sqrt(w * h / float64(len(m.Cells)))
	mid := topo.Point{X: w / 2, Y: h / 2}
	center, bestD := mid, math.Inf(1)
	for i, c := range m.Cells {
		if !c.Rim && water[i] == WaterNone {
			if dd := cyl.Distance(mid, c.Site); dd < bestD {
				center, bestD = c.Site, dd
			}
		}
	}
	ww := min(fmath.Mul(compassCellsX, side), w)
	wh := min(fmath.Mul(compassCellsY, side), h)
	y0 := min(max(center.Y-fmath.Mul(wh, 0.5), 0), h-wh)
	scale := compassPxCell / side
	img := image.NewRGBA(image.Rect(0, 0, int(math.Ceil(fmath.Mul(ww, scale))), int(math.Ceil(fmath.Mul(wh, scale)))))
	// local returns the pixel of the point at offset off from the site
	// of a cell whose site is at window x sx.
	local := func(sx float64, site, off topo.Point) render.Pt {
		return render.Pt{X: fmath.Mul(sx+off.X, scale), Y: fmath.Mul(site.Y+off.Y-y0, scale)}
	}
	margin := fmath.Mul(2, side)
	var shown []int
	sxOf := make([]float64, len(m.Cells)) // each shown cell's site x in the window
	for i, c := range m.Cells {
		sx := fmath.Mul(ww, 0.5) + cyl.DX(center.X, c.Site.X)
		if sx < -margin || sx > ww+margin || c.Site.Y < y0-margin || c.Site.Y > y0+wh+margin {
			continue
		}
		shown = append(shown, i)
		sxOf[i] = sx
	}
	outline := color.RGBA{0x50, 0x50, 0x50, 0xff}
	for _, i := range shown {
		offs := m.Offsets(i)
		pts := make([]render.Pt, len(offs))
		for k, o := range offs {
			pts[k] = local(sxOf[i], m.Cells[i].Site, o)
		}
		render.FillPolygon(img, pts, baseColor(m, water, i))
	}
	for _, i := range shown {
		offs := m.Offsets(i)
		for k := range offs {
			a := local(sxOf[i], m.Cells[i].Site, offs[k])
			b := local(sxOf[i], m.Cells[i].Site, offs[(k+1)%len(offs)])
			render.Line(img, a, b, 1, outline)
		}
	}
	ink := color.RGBA{0x10, 0x10, 0x10, 0xff}
	for _, i := range shown {
		site := m.Cells[i].Site
		for _, hf := range d.Cells[i] {
			dx, dy := cyl.Delta(site, m.Cells[hf.Neighbor].Site)
			a := local(sxOf[i], site, topo.Point{})
			b := local(sxOf[i], site, topo.Point{X: fmath.Mul(dx, 0.25), Y: fmath.Mul(dy, 0.25)})
			render.Line(img, a, b, 3, DirectionColors[hf.Direction])
		}
		render.Disc(img, local(sxOf[i], site, topo.Point{}), 3, ink)
	}
	for _, i := range shown {
		site := m.Cells[i].Site
		for _, hf := range d.Cells[i] {
			dx, dy := cyl.Delta(site, m.Cells[hf.Neighbor].Site)
			t := local(sxOf[i], site, topo.Point{X: fmath.Mul(dx, 0.37), Y: fmath.Mul(dy, 0.37)})
			name := hf.Direction.String()
			lw := len(name) * render.LabelAdvance
			min := image.Pt(int(math.Round(t.X))-lw/2, int(math.Round(t.Y))-render.LabelHeight/2)
			render.Label(img, image.Rectangle{Min: min, Max: min.Add(image.Pt(lw, render.LabelHeight))}, name, ink)
		}
	}
	return img
}
