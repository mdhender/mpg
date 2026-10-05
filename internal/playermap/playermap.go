// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package playermap

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/world"
)

// selectMargin is how many pixels a cell's bounding box is widened by when
// selecting the cells a frame needs; the polygon's pixel coordinates and
// its rounded bounding box differ by far less.
const selectMargin = 2

// Render draws frame f of the map of w on lattice l (see the package
// documentation for what it draws, and in what order). It reads nothing but
// w. A frame's image is exactly the matching crop of the full map's
// (Crop).
func Render(w *world.World, l Lattice, f Frame) (*image.RGBA, error) {
	if err := l.check(f); err != nil {
		return nil, err
	}
	if err := checkWorld(w); err != nil {
		return nil, err
	}
	cv := newCanvas(l, f)
	g := newGeometry(w, l)
	cellPx := fmath.Mul(math.Sqrt(w.Meta.ProvinceAreaKm2), min(l.SX, l.SY))

	// 1. Every cell the frame needs, in id order: rim cells as ice, the
	// rest by landform and depth. The fills tile the map exactly, so the
	// ice is one sheet with no seams between its cells.
	for i := range w.Cells {
		c := &w.Cells[i]
		if g.cellHits(cv, c) {
			g.fillCell(cv, c, CellColor(c))
		}
	}

	// 2. Faint borders between land cells.
	cv.startLayer()
	for e := range w.Edges {
		if a, b := w.Edges[e].Cells[0], w.Edges[e].Cells[1]; b != world.Boundary && g.land[a] && g.land[b] {
			g.edge(cv, e, 1, BorderColor)
		}
	}
	cv.blendLayer(BorderColor, BorderAlpha)

	// 3. The ice front, where the rim meets playable cells.
	for e := range w.Edges {
		if a, b := w.Edges[e].Cells[0], w.Edges[e].Cells[1]; b != world.Boundary && g.rim[a] != g.rim[b] {
			g.edge(cv, e, max(1.5, cellPx/8), IceFrontColor)
		}
	}

	// 4. Rivers by class, then 5. the coastlines.
	for _, r := range w.Rivers {
		for k, e := range r.Edges {
			g.edge(cv, e, max(1, fmath.Mul(riverWidth(r.Classes[k]), cellPx)), RiverColor)
		}
	}
	for _, cl := range w.Coastlines {
		for _, e := range cl.Edges {
			g.edge(cv, e, max(1.5, cellPx/8), CoastColor)
		}
	}

	// 6. Volcanoes.
	for i := range w.Cells {
		if c := &w.Cells[i]; c.HasFlag(world.FlagVolcano) {
			p := pt{fmath.Mul(c.Site.X, l.SX), fmath.Mul(c.Site.Y, l.SY)}
			cv.disc(p, max(4, fmath.Mul(0.45, cellPx)), VolcanoRing)
			cv.disc(p, max(2.5, fmath.Mul(0.3, cellPx)), VolcanoInk)
		}
	}
	return cv.img, nil
}

// RenderFull draws the whole map of w at scale pixels per km, and returns
// it with its lattice.
func RenderFull(w *world.World, scale float64) (*image.RGBA, Lattice, error) {
	l, err := NewLattice(w, scale)
	if err != nil {
		return nil, Lattice{}, err
	}
	img, err := Render(w, l, l.Full())
	return img, l, err
}

// Crop returns frame f cut from full, a render of l.Full(): columns are
// taken modulo the map's width, so a frame across the seam is the east end
// of full followed by its west end. Render(w, l, f) equals it pixel for
// pixel.
func Crop(full *image.RGBA, l Lattice, f Frame) (*image.RGBA, error) {
	if err := l.check(f); err != nil {
		return nil, err
	}
	if b := full.Bounds(); b.Dx() != l.Width || b.Dy() != l.Height {
		return nil, fmt.Errorf("playermap: full render is %v, want %d × %d", b.Size(), l.Width, l.Height)
	}
	out := image.NewRGBA(image.Rect(0, 0, f.Width, f.Height))
	b := full.Bounds()
	for j := range f.Height {
		for i := range f.Width {
			x := fmath.FloorMod(f.X+i, l.Width)
			out.SetRGBA(i, j, full.RGBAAt(b.Min.X+x, b.Min.Y+f.Y+j))
		}
	}
	return out, nil
}

// geometry holds a world's corners in global pixel coordinates and its
// per-cell rim and land flags.
type geometry struct {
	w      *world.World
	l      Lattice
	px, py []float64 // corner positions in pixels, px in [0, l.Width]
	wpx    float64   // l.Width
	rim    []bool
	land   []bool
}

func newGeometry(w *world.World, l Lattice) *geometry {
	g := &geometry{
		w: w, l: l,
		px:   make([]float64, len(w.Corners)),
		py:   make([]float64, len(w.Corners)),
		wpx:  float64(l.Width),
		rim:  make([]bool, len(w.Cells)),
		land: make([]bool, len(w.Cells)),
	}
	for k := range w.Corners {
		p := w.Corners[k].Point
		g.px[k], g.py[k] = fmath.Mul(p.X, l.SX), fmath.Mul(p.Y, l.SY)
	}
	for i := range w.Cells {
		c := &w.Cells[i]
		g.rim[i] = c.HasFlag(world.FlagRim)
		g.land[i] = !g.rim[i] && c.Landform.IsLand()
	}
	return g
}

// dx returns the east–west pixel offset from corner a to corner b, the
// shorter way around the map. Every caller passes a < b, so an edge's
// offset is computed one way only.
func (g *geometry) dx(a, b int) float64 {
	d := g.px[b] - g.px[a]
	if d > g.wpx/2 {
		d -= g.wpx
	} else if d < -g.wpx/2 {
		d += g.wpx
	}
	return d
}

// cellHits reports whether cell c's bounding box, widened by
// selectMargin pixels, reaches into the frame (DESIGN.md: "selecting
// cells whose bounding boxes intersect the window (taken modulo W)").
func (g *geometry) cellHits(cv *canvas, c *world.Cell) bool {
	l := g.l
	x0 := int(math.Floor(fmath.Mul(c.BBox.Min.X, l.SX))) - selectMargin
	x1 := int(math.Ceil(fmath.Mul(c.BBox.Max.X, l.SX))) + selectMargin
	y0 := int(math.Floor(fmath.Mul(c.BBox.Min.Y, l.SY))) - selectMargin
	y1 := int(math.Ceil(fmath.Mul(c.BBox.Max.Y, l.SY))) + selectMargin
	return cv.hitsRows(y0, y1) && cv.hitsCols(x0, x1)
}

// fillCell paints every pixel whose center lies inside cell c.
//
// The polygon is scanned row by row. Where a side crosses a row's center
// line, the crossing is computed from the side's lower-numbered corner
// (its position in pixels plus the side's wrapped offset), so the two
// cells that share a side compute the same crossing, and it is turned into
// a whole column before the cell's own shift across the seam (a whole
// number of map widths) is added. A pixel is inside when left ≤ its
// center < right, so neighboring cells meet without a gap or an overlap,
// and no arithmetic depends on the frame.
func (g *geometry) fillCell(cv *canvas, c *world.Cell, ink color.RGBA) {
	n := len(c.Corners)
	shift := make([]int, n) // map widths from each corner's position to the cell's polygon
	minY, maxY := math.Inf(1), math.Inf(-1)
	for k, id := range c.Corners {
		ux := c.Site.X + c.Polygon[k].X
		shift[k] = int(math.Round((ux - g.w.Corners[id].Point.X) / g.w.Meta.WidthKm))
		minY, maxY = min(minY, g.py[id]), max(maxY, g.py[id])
	}
	y0 := max(int(math.Floor(minY)), cv.f.Y)
	y1 := min(int(math.Ceil(maxY)), cv.f.Y+cv.f.Height)
	width := cv.l.Width
	for gy := y0; gy < y1; gy++ {
		yc := float64(gy) + 0.5
		left, right := math.MaxInt, math.MinInt
		for k := range n {
			a, b := c.Corners[k], c.Corners[(k+1)%n]
			if (g.py[a] <= yc) == (g.py[b] <= yc) {
				continue
			}
			lo, hi, s := a, b, shift[k]
			if b < a {
				lo, hi, s = b, a, shift[(k+1)%n]
			}
			x := g.px[lo] + fmath.Mul(yc-g.py[lo], g.dx(lo, hi))/(g.py[hi]-g.py[lo])
			col := int(math.Ceil(x-0.5)) + s*width
			left, right = min(left, col), max(right, col)
		}
		if left < right {
			cv.span(gy, left, right, ink)
		}
	}
}

// edge draws edge e from its lower-numbered corner to the other, width
// pixels wide, in ink (or into the layer).
func (g *geometry) edge(cv *canvas, e int, width float64, ink color.RGBA) {
	cs := g.w.Edges[e].Corners
	a, b := min(cs[0], cs[1]), max(cs[0], cs[1])
	pa := pt{g.px[a], g.py[a]}
	cv.line(pa, pt{pa.x + g.dx(a, b), g.py[b]}, width, ink)
}

// checkWorld checks the references Render follows, so a damaged world
// gives an error rather than a panic. world.Validate checks much more.
func checkWorld(w *world.World) error {
	nc, nk, ne := len(w.Cells), len(w.Corners), len(w.Edges)
	if w.Meta.WidthKm <= 0 || w.Meta.HeightKm <= 0 || !(w.Meta.ProvinceAreaKm2 > 0) {
		return fmt.Errorf("playermap: world meta has a non-positive size or province area")
	}
	for i := range w.Cells {
		c := &w.Cells[i]
		if len(c.Corners) < 3 || len(c.Polygon) != len(c.Corners) {
			return fmt.Errorf("playermap: cell %d has %d corners and %d polygon points", i, len(c.Corners), len(c.Polygon))
		}
		for _, k := range c.Corners {
			if k < 0 || k >= nk {
				return fmt.Errorf("playermap: cell %d has corner %d of %d", i, k, nk)
			}
		}
	}
	edgeOK := func(e int) error {
		if e < 0 || e >= ne {
			return fmt.Errorf("playermap: edge %d of %d", e, ne)
		}
		return nil
	}
	for e := range w.Edges {
		ed := &w.Edges[e]
		for _, k := range ed.Corners {
			if k < 0 || k >= nk {
				return fmt.Errorf("playermap: edge %d has corner %d of %d", e, k, nk)
			}
		}
		if a, b := ed.Cells[0], ed.Cells[1]; a < 0 || a >= nc || b < world.Boundary || b >= nc {
			return fmt.Errorf("playermap: edge %d has cells %v of %d", e, ed.Cells, nc)
		}
	}
	for _, r := range w.Rivers {
		if len(r.Classes) != len(r.Edges) {
			return fmt.Errorf("playermap: river has %d edges and %d classes", len(r.Edges), len(r.Classes))
		}
		for _, e := range r.Edges {
			if err := edgeOK(e); err != nil {
				return err
			}
		}
	}
	for _, cl := range w.Coastlines {
		for _, e := range cl.Edges {
			if err := edgeOK(e); err != nil {
				return err
			}
		}
	}
	return nil
}
