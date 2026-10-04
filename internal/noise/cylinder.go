// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package noise

import (
	"math"

	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/internal/topo"
)

// Point3 is a point in the 3-D space the noise is sampled in, in km.
type Point3 struct {
	X, Y, Z float64
}

// Cylinder maps world points onto a 3-D cylinder whose surface distances are
// km, so noise sampled there is periodic east–west. Build one with
// NewCylinder; the zero value is not valid.
type Cylinder struct {
	topo  topo.Cylinder
	r     float64 // W/(2π)
	halfH float64 // H/2
}

// NewCylinder returns the mapper for the world c.
func NewCylinder(c topo.Cylinder) Cylinder {
	return Cylinder{topo: c, r: c.W() / (2 * math.Pi), halfH: c.H() / 2}
}

// Topo returns the world the mapper covers.
func (m Cylinder) Topo() topo.Cylinder { return m.topo }

// Point maps the world point (x, y) km to the cylinder:
//
//	theta = 2π·x/W,  (R·cos theta, R·sin theta, H/2 − y),  R = W/(2π)
//
// x is wrapped into [0, W) first, so x and x ± W give identical bits whenever
// the wrap is exact. H/2 − y is latitude·H/2, the latitude proxy scaled to km
// (computed directly, without rounding through the latitude); y is not
// clamped, so a y beyond a pole extrapolates.
func (m Cylinder) Point(x, y float64) Point3 {
	x = m.topo.WrapX(x)
	theta := fmath.Mul(x/m.topo.W(), 2*math.Pi)
	sin, cos := fmath.Sincos(theta)
	return Point3{X: fmath.Mul(m.r, cos), Y: fmath.Mul(m.r, sin), Z: m.halfH - y}
}

// Fill stores fn(x, y) at every sample of f, where (x, y) is the sample's
// location in km, in storage order (rows north to south, columns west to
// east). It panics if fn returns NaN or an infinity.
func Fill(f *field.Field, fn func(x, y float64) float64) {
	f.SetFunc(func(_, _ int, p topo.Point) float64 { return fn(p.X, p.Y) })
}
