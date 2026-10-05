// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package playermap

import (
	"errors"
	"fmt"
	"math"

	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/world"
)

// Size limits.
const (
	// MaxMapPixels bounds the full map's width and height in pixels at a
	// scale.
	MaxMapPixels = 1 << 16
	// MaxFramePixels bounds a rendered frame's pixel count (1 GiB of RGBA).
	MaxFramePixels = 1 << 28
)

// DefaultScale is the scale of the export stage render and of mpg
// render-stage without --scale or --width: one pixel per km, so a province
// (about 9 km across) is about 9 pixels across.
const DefaultScale = 1.0

// Lattice is the global pixel grid of a world's map at a scale. The full
// map is Width × Height pixels; global pixel (i, j) covers
// [i, i+1) × [j, j+1) in pixel coordinates, and the world point (x, y) km
// lies at pixel coordinates (x·SX, y·SY). Columns wrap: global column i
// and i + Width are the same pixel.
type Lattice struct {
	// Scale is the requested scale in pixels per km.
	Scale float64
	// Width and Height are the full map's size in pixels: W·Scale and
	// H·Scale rounded to whole pixels (at least 1).
	Width, Height int
	// SX and SY are the pixels per km actually used: Width/W and
	// Height/H. Snapping Width to a whole number of pixels is what makes
	// the east–west wrap a whole number of pixels, so a window across the
	// seam is pixel for pixel the crop of the full map. They differ from
	// Scale by less than half a pixel across the map.
	SX, SY float64
}

// NewLattice returns w's pixel lattice at scale pixels per km.
func NewLattice(w *world.World, scale float64) (Lattice, error) {
	wk, hk := w.Meta.WidthKm, w.Meta.HeightKm
	if !(wk > 0) || !(hk > 0) || math.IsInf(wk, 0) || math.IsInf(hk, 0) {
		return Lattice{}, fmt.Errorf("playermap: world size %v × %v km is not positive and finite", wk, hk)
	}
	if !(scale > 0) || math.IsInf(scale, 0) {
		return Lattice{}, fmt.Errorf("playermap: scale %v px/km is not positive and finite", scale)
	}
	fw, fh := math.Round(fmath.Mul(wk, scale)), math.Round(fmath.Mul(hk, scale))
	if fw > MaxMapPixels || fh > MaxMapPixels {
		return Lattice{}, fmt.Errorf("playermap: scale %v px/km gives a %.0f × %.0f pixel map, over %d pixels a side", scale, fw, fh, MaxMapPixels)
	}
	l := Lattice{Scale: scale, Width: max(1, int(fw)), Height: max(1, int(fh))}
	l.SX = float64(l.Width) / wk
	l.SY = float64(l.Height) / hk
	return l, nil
}

// Frame is a rectangle of the lattice that a render draws: the global
// pixels whose column is X, X+1, …, X+Width−1 (each taken modulo the map's
// width) and whose row is Y, …, Y+Height−1. Pixel (i, j) of the image is
// global pixel (X+i mod Lattice.Width, Y+j).
type Frame struct {
	X, Y          int
	Width, Height int
}

// Full returns the frame of the whole map.
func (l Lattice) Full() Frame { return Frame{Width: l.Width, Height: l.Height} }

// Window returns the frame of the window x, y, w, h in km: x east and y
// south of the northwest corner, w wide and h high. x is taken modulo W,
// so a window may start west of 0 or run east past W across the seam. The
// frame's origin is the global pixel holding (x, y), floor(x·SX) and
// floor(y·SY), and its size w·SX × h·SY rounded to whole pixels (at least
// 1). The window must lie within the map north to south and be at most
// the map's width.
func (l Lattice) Window(x, y, w, h float64) (Frame, error) {
	for _, v := range []float64{x, y, w, h} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return Frame{}, errors.New("playermap: window values must be finite")
		}
	}
	if !(w > 0) || !(h > 0) {
		return Frame{}, fmt.Errorf("playermap: window size %v × %v km is not positive", w, h)
	}
	gx, gy := math.Floor(fmath.Mul(x, l.SX)), math.Floor(fmath.Mul(y, l.SY))
	fw, fh := math.Round(fmath.Mul(w, l.SX)), math.Round(fmath.Mul(h, l.SY))
	if math.Abs(gx) > 1<<40 || gy < 0 || fw > float64(l.Width) || gy+max(fh, 1) > float64(l.Height) {
		return Frame{}, fmt.Errorf("playermap: window %v,%v %v×%v km does not fit a %v × %v km map (y in [0, H], width at most W)",
			x, y, w, h, float64(l.Width)/l.SX, float64(l.Height)/l.SY)
	}
	f := Frame{X: fmath.FloorMod(int(gx), l.Width), Y: int(gy), Width: max(1, int(fw)), Height: max(1, int(fh))}
	return f, l.check(f)
}

// check reports whether f is a frame of l that a render can draw.
func (l Lattice) check(f Frame) error {
	switch {
	case f.Width < 1 || f.Height < 1:
		return fmt.Errorf("playermap: frame %+v is empty", f)
	case f.Width > l.Width:
		return fmt.Errorf("playermap: frame %+v is wider than the %d-pixel map", f, l.Width)
	case f.Y < 0 || f.Y+f.Height > l.Height:
		return fmt.Errorf("playermap: frame %+v runs past the map's %d rows", f, l.Height)
	case int64(f.Width)*int64(f.Height) > MaxFramePixels:
		return fmt.Errorf("playermap: frame %+v is over %d pixels", f, MaxFramePixels)
	}
	return nil
}
