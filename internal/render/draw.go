// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package render

import (
	"image"
	"image/color"
	"math"

	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/internal/topo"
)

// Pt is a point in pixel coordinates: x east, y south, with pixel (x, y)
// covering [x, x+1) × [y, y+1).
type Pt struct {
	X, Y float64
}

// ToPixel returns the pixel coordinates of world point p on a render of f:
// p.X/PitchX and p.Y/PitchY, so sample (i, j) lands on its pixel's center
// (i + 0.5, j + 0.5). It does not wrap p.X.
func ToPixel(f *field.Field, p topo.Point) Pt {
	return Pt{X: p.X / f.PitchX(), Y: p.Y / f.PitchY()}
}

// The drawing primitives below work in plain pixel coordinates and clip to
// the image; they do not wrap across the seam. A shape that crosses it is
// drawn twice by the caller, shifted by the image width.

// FillPolygon paints c on every pixel whose center lies inside the convex
// polygon: pixel (x, y) is inside when its center row crosses the polygon at
// left ≤ x + 0.5 < right. Adapted from maloquacious/wg/render.
func FillPolygon(img *image.RGBA, polygon []Pt, c color.RGBA) {
	if len(polygon) < 3 {
		return
	}
	b := img.Bounds()
	minY, maxY := math.Inf(1), math.Inf(-1)
	for _, q := range polygon {
		minY, maxY = min(minY, q.Y), max(maxY, q.Y)
	}
	y0 := max(int(math.Floor(minY)), b.Min.Y)
	y1 := min(int(math.Ceil(maxY)), b.Max.Y)
	for y := y0; y < y1; y++ {
		yc := float64(y) + 0.5
		left, right := math.Inf(1), math.Inf(-1)
		for n, p := range polygon {
			q := polygon[(n+1)%len(polygon)]
			if (p.Y <= yc) != (q.Y <= yc) {
				x := p.X + fmath.Mul(yc-p.Y, q.X-p.X)/(q.Y-p.Y)
				left, right = min(left, x), max(right, x)
			}
		}
		if left > right {
			continue
		}
		x0 := max(int(math.Ceil(left-0.5)), b.Min.X)
		x1 := min(int(math.Ceil(right-0.5)), b.Max.X)
		for x := x0; x < x1; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

// Line paints c on every pixel whose center lies within width/2 (at least
// half a pixel) of the segment from a to b. Adapted from
// maloquacious/wg/render's drawWideLine.
func Line(img *image.RGBA, a, b Pt, width float64, c color.RGBA) {
	r := max(width/2, 0.5)
	dx, dy := b.X-a.X, b.Y-a.Y
	length2 := fmath.Mul(dx, dx) + fmath.Mul(dy, dy)
	bounds := img.Bounds()
	y0 := max(int(math.Floor(min(a.Y, b.Y)-r)), bounds.Min.Y)
	y1 := min(int(math.Ceil(max(a.Y, b.Y)+r)), bounds.Max.Y-1)
	x0 := max(int(math.Floor(min(a.X, b.X)-r)), bounds.Min.X)
	x1 := min(int(math.Ceil(max(a.X, b.X)+r)), bounds.Max.X-1)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			// distance from the pixel center to the nearest point of the segment
			px, py := float64(x)+0.5, float64(y)+0.5
			t := 0.0
			if length2 > 0 {
				t = min(max((fmath.Mul(px-a.X, dx)+fmath.Mul(py-a.Y, dy))/length2, 0), 1)
			}
			if fmath.Hypot(px-fmath.MulAdd(t, dx, a.X), py-fmath.MulAdd(t, dy, a.Y)) <= r {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

// Disc paints c on every pixel whose center lies within radius r (at least
// half a pixel) of center, for marks such as volcanic hotspots.
func Disc(img *image.RGBA, center Pt, r float64, c color.RGBA) {
	Line(img, center, center, 2*r, c)
}
