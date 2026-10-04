// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package layout

import (
	"fmt"
	"math"

	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/internal/noise"
	"github.com/mdhender/mpg/internal/topo"
)

// Bias range: the ocean floor far from every attractor, and the largest
// value (an attractor of weight 1 at its center).
const (
	Ocean       = -1.0
	Continental = 1.0
)

// supportSq is T², where T is the attractor kernel's support in radii:
// (1 − T⁻²)² = 1/2, so the kernel is 0 at one radius.
const supportSq = 2 + math.Sqrt2

// wobbleFBM returns the wobble noise ladder for a wavelength.
func wobbleFBM(wavelengthKm float64) noise.FBM {
	return noise.FBM{Octaves: 3, Lacunarity: 2, Gain: 0.5, WavelengthKm: wavelengthKm}
}

// Bias returns a new field over the layout's cylinder at the given raster
// spacing, filled with the bias field (see Fill).
func (l *Layout) Bias(spacingKm float64) (*field.Field, error) {
	f, err := field.New(l.cyl, spacingKm)
	if err != nil {
		return nil, fmt.Errorf("layout: %w", err)
	}
	l.Fill(f)
	return f, nil
}

// Fill stores the bias field in f, which must lie on the layout's cylinder.
// See the package documentation for the formula.
func (l *Layout) Fill(f *field.Field) {
	if f.Cylinder() != l.cyl {
		panic("layout: field and layout lie on different cylinders")
	}
	nx, ny := f.NX(), f.NY()
	v := make([]float64, nx*ny)
	for k := range v {
		v[k] = Ocean
	}

	// scale[k] multiplies every attractor radius at sample k.
	scale := make([]float64, nx*ny)
	if l.Wobble > 0 {
		m := noise.NewCylinder(l.cyl)
		fbm := wobbleFBM(l.WobbleKm)
		src := l.source.Stream(0)
		for j := range ny {
			for i := range nx {
				n := fbm.Sample(src, m.Point(f.X(i), f.Y(j)))
				scale[j*nx+i] = fmath.MulAdd(l.Wobble, n, 1)
			}
		}
	} else {
		for k := range scale {
			scale[k] = 1
		}
	}

	// Attractors: the largest kernel value wins.
	reach := math.Sqrt(supportSq) * (1 + l.Wobble)
	for _, a := range l.Attractors {
		l.each(f, a.X, a.Y, fmath.Mul(reach, a.RadiusKm), func(k int, d float64) {
			q := d / fmath.Mul(a.RadiusKm, scale[k])
			u := 1 - fmath.Mul(q, q)/supportSq
			if u <= 0 {
				return
			}
			b := fmath.Mul(2, fmath.Mul(u, u)) - 1
			if b > 0 {
				b = fmath.Mul(a.Weight, b)
			}
			v[k] = max(v[k], b)
		})
	}

	// Repulsors subtract, in order.
	for _, r := range l.Repulsors {
		l.each(f, r.X, r.Y, r.RadiusKm, func(k int, d float64) {
			q := d / r.RadiusKm
			u := 1 - fmath.Mul(q, q)
			if u <= 0 {
				return
			}
			v[k] -= fmath.Mul(r.Weight, fmath.Mul(u, u))
		})
	}

	f.SetFunc(func(i, j int, _ topo.Point) float64 { return max(v[j*nx+i], Ocean) })
}

// each calls fn(k, d) for every sample k of f within reach km of (x, y),
// where d is the wrapped distance, in storage order. Samples just outside
// reach may be visited too.
func (l *Layout) each(f *field.Field, x, y, reach float64, fn func(k int, d float64)) {
	nx, ny := f.NX(), f.NY()
	px, py := f.PitchX(), f.PitchY()
	j0 := max(0, int(math.Floor((y-reach)/py-0.5))-1)
	j1 := min(ny-1, int(math.Ceil((y+reach)/py-0.5))+1)
	i0 := int(math.Floor((x-reach)/px-0.5)) - 1
	i1 := int(math.Ceil((x+reach)/px-0.5)) + 1
	if i1-i0+1 >= nx {
		i0, i1 = 0, nx-1
	}
	c := topo.Point{X: x, Y: y}
	for j := j0; j <= j1; j++ {
		for ii := i0; ii <= i1; ii++ {
			i := fmath.FloorMod(ii, nx)
			k := j*nx + i
			fn(k, l.cyl.Distance(c, f.Point(i, j)))
		}
	}
}

// PositiveShare returns the share of the samples of bias in the playable band
// (outside the rim and falloff) whose bias is above zero: roughly the share
// of the band the layout asks to be land.
func PositiveShare(bias *field.Field) float64 {
	c := bias.Cylinder()
	var n, pos int
	bias.Each(func(i, j int, v float64) {
		y := bias.Y(j)
		if c.InRim(y) || c.InFalloff(y) {
			return
		}
		n++
		if v > 0 {
			pos++
		}
	})
	if n == 0 {
		return 0
	}
	return float64(pos) / float64(n)
}
