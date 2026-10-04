// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package topo

import (
	"fmt"
	"math"

	"github.com/mdhender/mpg/internal/fmath"
)

// Point is a location in world kilometers: X east from the west edge, Y south
// from the north edge.
type Point struct {
	X, Y float64
}

// Cylinder is the world's shape: W km around (x wraps), H km pole to pole
// (y does not), with a polar rim and falloff band of the given widths at both
// the north and south edges. Build one with New; the zero value is not valid.
type Cylinder struct {
	w, h, rim, falloff float64
}

// New returns the cylinder W km around and H km tall, whose rim is rim km deep
// and whose falloff band is falloff km deep inside the rim, at both poles.
//
// W and H must be positive and finite, rim positive and finite, and falloff
// non-negative and finite. Both rims and both falloff bands must leave a
// playable band between them: 2·(rim + falloff) < H.
func New(w, h, rim, falloff float64) (Cylinder, error) {
	switch {
	case !(w > 0) || math.IsInf(w, 1):
		return Cylinder{}, fmt.Errorf("topo: width %v km must be positive and finite", w)
	case !(h > 0) || math.IsInf(h, 1):
		return Cylinder{}, fmt.Errorf("topo: height %v km must be positive and finite", h)
	case !(rim > 0) || math.IsInf(rim, 1):
		return Cylinder{}, fmt.Errorf("topo: rim %v km must be positive and finite", rim)
	case !(falloff >= 0) || math.IsInf(falloff, 1):
		return Cylinder{}, fmt.Errorf("topo: falloff %v km must be non-negative and finite", falloff)
	}
	if band := rim + falloff; !(band < h/2) {
		return Cylinder{}, fmt.Errorf("topo: rim %v km plus falloff %v km at both poles leaves no playable band in height %v km", rim, falloff, h)
	}
	return Cylinder{w: w, h: h, rim: rim, falloff: falloff}, nil
}

// W returns the east-west circumference in km.
func (c Cylinder) W() float64 { return c.w }

// H returns the pole-to-pole height in km.
func (c Cylinder) H() float64 { return c.h }

// Rim returns the depth of the polar rim in km.
func (c Cylinder) Rim() float64 { return c.rim }

// Falloff returns the depth of the falloff band, inside the rim, in km.
func (c Cylinder) Falloff() float64 { return c.falloff }

// WrapX returns x wrapped into [0, W). A zero result is +0. A non-finite x
// returns NaN.
func (c Cylinder) WrapX(x float64) float64 {
	return fmath.FloorModFloat(x, c.w)
}

// DX returns the shortest east-west displacement from x a to x b, in
// [−W/2, W/2): positive is east. Inputs need not be wrapped; for inputs in
// [0, W) the result is b − a, shifted by exactly W when that is outside the
// range, so DX(b, a) == −DX(a, b) except at the tie. At the tie, b
// exactly half way around, the result is −W/2: the shorter way is taken to be
// west. A zero result is +0.
func (c Cylinder) DX(a, b float64) float64 {
	d := b - a
	if d < -c.w || d >= c.w {
		d = fmath.FloorModFloat(d, c.w) // unwrapped inputs
	}
	// Both shifts are exact (Sterbenz), so for wrapped inputs DX(b, a) is
	// exactly −DX(a, b) away from the tie.
	switch {
	case d >= c.w/2:
		d -= c.w
	case d < -c.w/2:
		d += c.w
	}
	if d == 0 {
		return 0 // normalize −0
	}
	return d
}

// Delta returns the displacement from a to b: dx wrapped as by DX, dy = b.Y −
// a.Y unwrapped (positive is south).
func (c Cylinder) Delta(a, b Point) (dx, dy float64) {
	return c.DX(a.X, b.X), b.Y - a.Y
}

// Distance returns the straight-line distance in km from a to b across the
// shorter way around the cylinder.
func (c Cylinder) Distance(a, b Point) float64 {
	dx, dy := c.Delta(a, b)
	return fmath.Hypot(dx, dy)
}

// Bearing returns the direction from a to b in degrees clockwise from north,
// in [0, 360), using the wrapped delta: north (−y) is 0, east 90, south 180,
// west 270. The bearing of a point to itself (a zero delta) is 0. Exactly
// half way around, DX's tie rule makes a due-east or due-west target 270.
func (c Cylinder) Bearing(a, b Point) float64 {
	dx, dy := c.Delta(a, b)
	if dx == 0 && dy == 0 {
		return 0
	}
	// Atan2(east, north) measures clockwise from north, in [−180°, 180°].
	deg := fmath.Mul(fmath.Atan2(dx, -dy), 180/math.Pi)
	if deg < 0 {
		deg += 360
	}
	if deg >= 360 || deg == 0 {
		return 0 // a tiny negative angle can round up to 360; also normalizes −0
	}
	return deg
}

// Latitude returns the latitude proxy 1 − 2·y/H: +1 at the north edge (y = 0),
// 0 at the equator (y = H/2), and −1 at the south edge (y = H). It is not
// clamped, so a y outside [0, H] gives a value outside [−1, 1].
func (c Cylinder) Latitude(y float64) float64 {
	return 1 - 2*y/c.h
}

// PoleDistance returns the distance in km from y to the nearer polar edge:
// min(y, H − y). It is negative for a y outside [0, H].
func (c Cylinder) PoleDistance(y float64) float64 {
	return min(y, c.h-y)
}

// RimDistance returns how far y lies inside the rim's inner edge, in km:
// PoleDistance(y) − Rim. It is negative in the rim, 0 on its inner edge, and
// runs from 0 to Falloff across the falloff band, which makes
// RimDistance(y)/Falloff the ramp parameter for the falloff.
func (c Cylinder) RimDistance(y float64) float64 {
	return c.PoleDistance(y) - c.rim
}

// InRim reports whether y lies in the north or south rim: PoleDistance(y) <
// Rim. The north rim is [0, Rim) and the south rim (H − Rim, H], so the
// boundary y values belong to the band inside, symmetrically at both poles.
// Any y outside [0, H] is rim; NaN is not.
func (c Cylinder) InRim(y float64) bool {
	return c.PoleDistance(y) < c.rim
}

// InFalloff reports whether y lies in the falloff band, outside the rim: Rim ≤
// PoleDistance(y) < Rim + Falloff. The north band is [Rim, Rim + Falloff) and
// the south band (H − Rim − Falloff, H − Rim]. It is always false when
// Falloff is 0.
func (c Cylinder) InFalloff(y float64) bool {
	d := c.PoleDistance(y)
	return d >= c.rim && d < c.rim+c.falloff
}
