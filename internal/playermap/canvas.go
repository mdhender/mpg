// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package playermap

import (
	"image"
	"image/color"
	"math"

	"github.com/mdhender/mpg/internal/fmath"
)

// canvas is a frame of the lattice being drawn. Primitives are rasterized
// in global pixel coordinates (the same arithmetic whatever the frame), and
// only the resulting whole pixels are mapped into the frame: global column
// g lands on image column (g − X) mod Width, when that is inside the frame.
// That integer mapping is the whole difference between a window and the
// full map, so a window is exactly the crop of the full map.
type canvas struct {
	l   Lattice
	f   Frame
	img *image.RGBA
	// mask, when not nil, receives the drawing instead of img: a layer
	// that blend later paints onto img.
	mask []bool
}

func newCanvas(l Lattice, f Frame) *canvas {
	return &canvas{l: l, f: f, img: image.NewRGBA(image.Rect(0, 0, f.Width, f.Height))}
}

// col returns the image column of global column g, or −1 when it is
// outside the frame.
func (cv *canvas) col(g int) int {
	if i := fmath.FloorMod(g-cv.f.X, cv.l.Width); i < cv.f.Width {
		return i
	}
	return -1
}

// row returns the image row of global row g, or −1 when it is outside the
// frame.
func (cv *canvas) row(g int) int {
	if j := g - cv.f.Y; j >= 0 && j < cv.f.Height {
		return j
	}
	return -1
}

// hitsCols reports whether any of the global columns g0 … g1 lies in the
// frame.
func (cv *canvas) hitsCols(g0, g1 int) bool {
	if g1 < g0 {
		return false
	}
	if g1-g0+1 >= cv.l.Width {
		return true
	}
	i := fmath.FloorMod(g0-cv.f.X, cv.l.Width)
	// the columns map to [i, i+n) and, past the wrap, [i−Width, …)
	return i < cv.f.Width || i+(g1-g0) >= cv.l.Width
}

// hitsRows reports whether any of the global rows g0 … g1 lies in the
// frame.
func (cv *canvas) hitsRows(g0, g1 int) bool {
	return g0 < cv.f.Y+cv.f.Height && g1 >= cv.f.Y
}

// set paints global pixel (gx, gy) when it is in the frame.
func (cv *canvas) set(gx, gy int, c color.RGBA) {
	j := cv.row(gy)
	if j < 0 {
		return
	}
	if i := cv.col(gx); i >= 0 {
		cv.put(i, j, c)
	}
}

func (cv *canvas) put(i, j int, c color.RGBA) {
	if cv.mask != nil {
		cv.mask[j*cv.f.Width+i] = true
		return
	}
	o := cv.img.PixOffset(i, j)
	p := cv.img.Pix[o : o+4 : o+4]
	p[0], p[1], p[2], p[3] = c.R, c.G, c.B, c.A
}

// span paints global columns [g0, g1) of global row gy.
func (cv *canvas) span(gy, g0, g1 int, c color.RGBA) {
	j := cv.row(gy)
	if j < 0 || g1 <= g0 {
		return
	}
	n := min(g1-g0, cv.l.Width)
	i := fmath.FloorMod(g0-cv.f.X, cv.l.Width)
	// [i, i+n) in image columns, and its part past the wrap at i−Width
	for _, a := range []int{i, i - cv.l.Width} {
		for x := max(a, 0); x < min(a+n, cv.f.Width); x++ {
			cv.put(x, j, c)
		}
	}
}

// pt is a point in global pixel coordinates.
type pt struct{ x, y float64 }

// line paints every global pixel whose center lies within width/2 (at
// least half a pixel) of the segment from a to b: render.Line's rule and
// arithmetic, on the lattice. The result depends on the direction a → b
// only through rounding, so callers draw each edge in one fixed direction.
func (cv *canvas) line(a, b pt, width float64, c color.RGBA) {
	r := max(width/2, 0.5)
	dx, dy := b.x-a.x, b.y-a.y
	length2 := fmath.Mul(dx, dx) + fmath.Mul(dy, dy)
	y0, y1 := int(math.Floor(min(a.y, b.y)-r)), int(math.Ceil(max(a.y, b.y)+r))
	x0, x1 := int(math.Floor(min(a.x, b.x)-r)), int(math.Ceil(max(a.x, b.x)+r))
	if !cv.hitsRows(y0, y1) || !cv.hitsCols(x0, x1) {
		return
	}
	for gy := y0; gy <= y1; gy++ {
		j := cv.row(gy)
		if j < 0 {
			continue
		}
		for gx := x0; gx <= x1; gx++ {
			i := cv.col(gx)
			if i < 0 {
				continue
			}
			// distance from the pixel center to the nearest point of the segment
			px, py := float64(gx)+0.5, float64(gy)+0.5
			t := 0.0
			if length2 > 0 {
				t = min(max((fmath.Mul(px-a.x, dx)+fmath.Mul(py-a.y, dy))/length2, 0), 1)
			}
			if fmath.Hypot(px-fmath.MulAdd(t, dx, a.x), py-fmath.MulAdd(t, dy, a.y)) <= r {
				cv.put(i, j, c)
			}
		}
	}
}

// disc paints every global pixel whose center lies within r (at least half
// a pixel) of center.
func (cv *canvas) disc(center pt, r float64, c color.RGBA) {
	cv.line(center, center, 2*r, c)
}

// startLayer directs drawing into a fresh mask; blendLayer ends it.
func (cv *canvas) startLayer() {
	cv.mask = make([]bool, cv.f.Width*cv.f.Height)
}

// blendLayer paints c at alpha (out of 255) over every pixel of the
// layer, in integer arithmetic, and returns drawing to the image. The
// layer is a set of pixels, so the result does not depend on the order
// things were drawn into it.
func (cv *canvas) blendLayer(c color.RGBA, alpha uint8) {
	mask := cv.mask
	cv.mask = nil
	a := int(alpha)
	ink := [3]int{int(c.R), int(c.G), int(c.B)}
	for k, on := range mask {
		if !on {
			continue
		}
		p := cv.img.Pix[4*k : 4*k+3 : 4*k+3]
		for n := range 3 {
			p[n] = uint8((int(p[n])*(255-a) + ink[n]*a + 127) / 255)
		}
	}
}
