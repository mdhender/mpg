// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package render

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/fmath"
)

// Colorize returns an image of f with each sample colored by fn, one pixel
// per sample (see the package documentation for the layout).
func Colorize(f *field.Field, fn func(v float64) color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, f.NX(), f.NY()))
	f.Each(func(i, j int, v float64) {
		img.SetRGBA(i, j, fn(v))
	})
	return img
}

// Hypsometric returns f, in meters, tinted by height relative to seaLevel:
// samples above sea level are land, colored by Land at their height above
// it, and the rest are water, colored by Water at their depth below it. The
// land test matches the mesh's: land is strictly above sea level.
func Hypsometric(f *field.Field, seaLevel float64) *image.RGBA {
	return Colorize(f, func(v float64) color.RGBA {
		if v > seaLevel {
			return Land.At(v - seaLevel)
		}
		return Water.At(seaLevel - v)
	})
}

// Sequential returns f colored by r, with the field's minimum mapped to the
// start of r's domain and its maximum to the end. A constant field takes the
// start color.
func Sequential(f *field.Field, r Ramp) *image.RGBA {
	lo, hi := f.MinMax()
	span := hi - lo
	return Colorize(f, func(v float64) color.RGBA {
		if !(span > 0) {
			return r.Frac(0)
		}
		return r.Frac((v - lo) / span)
	})
}

// Diverging returns f colored by r around center: center maps to the middle
// of r's domain, and the sample farthest from center maps to the matching
// end, so equal distances above and below get equally strong colors. A field
// equal to center everywhere takes the middle color.
func Diverging(f *field.Field, r Ramp, center float64) *image.RGBA {
	lo, hi := f.MinMax()
	m := max(math.Abs(hi-center), math.Abs(lo-center))
	return Colorize(f, func(v float64) color.RGBA {
		if !(m > 0) {
			return r.Frac(0.5)
		}
		return r.Frac(fmath.MulAdd((v-center)/m, 0.5, 0.5))
	})
}

// Light sets the sun for Hillshade.
type Light struct {
	// Azimuth is the compass direction toward the sun in degrees, clockwise
	// from north: 315 lights the map from the northwest.
	Azimuth float64
	// Altitude is the sun's angle above the horizon in degrees, (0, 90].
	Altitude float64
	// ZFactor exaggerates heights before shading. Heights are in meters and
	// sample spacing is converted from km to meters, so 1 shades the true
	// terrain; larger values deepen the relief.
	ZFactor float64
}

// DefaultLight is the conventional cartographic sun: northwest, 45° high,
// true relief.
var DefaultLight = Light{Azimuth: 315, Altitude: 45, ZFactor: 1}

// Shade is a hillshade grid: the illumination, 0 to 1, of each raster
// sample. Build one with Hillshade.
type Shade struct {
	nx, ny int
	flat   float64
	v      []float64
}

// NX returns the number of columns.
func (s *Shade) NX() int { return s.nx }

// NY returns the number of rows.
func (s *Shade) NY() int { return s.ny }

// At returns the illumination of sample (i, j). The column wraps; a row
// outside [0, NY) panics.
func (s *Shade) At(i, j int) float64 {
	if j < 0 || j >= s.ny {
		panic(fmt.Sprintf("render: shade row %d outside [0, %d)", j, s.ny))
	}
	return s.v[j*s.nx+fmath.FloorMod(i, s.nx)]
}

// Flat returns the illumination of level ground, sin(altitude). ApplyShade
// leaves pixels at this value unchanged.
func (s *Shade) Flat() float64 { return s.flat }

// Hillshade returns the illumination of f, a height field in meters, under
// light l.
//
// The slope at each sample comes from Horn's method: a 3 × 3 window weighted
// 1-2-1, divided by 8 pitches, with the pitches converted from km to meters
// and the heights multiplied by l.ZFactor. The window wraps east–west, so the
// seam shades like any other column, and clamps north–south by repeating the
// first and last rows. The illumination is the cosine between the surface
// normal and the direction to the sun, clamped at 0 for slopes facing away.
// It panics on an altitude outside (0, 90] or a non-finite or non-positive
// z-factor.
func Hillshade(f *field.Field, l Light) *Shade {
	if !(l.Altitude > 0 && l.Altitude <= 90) {
		panic(fmt.Sprintf("render: hillshade altitude %v° outside (0, 90]", l.Altitude))
	}
	if !(l.ZFactor > 0) || math.IsInf(l.ZFactor, 1) {
		panic(fmt.Sprintf("render: hillshade z-factor %v must be positive and finite", l.ZFactor))
	}
	const deg = math.Pi / 180
	sinAz, cosAz := fmath.Sincos(fmath.Mul(l.Azimuth, deg))
	sinAlt, cosAlt := fmath.Sincos(fmath.Mul(l.Altitude, deg))
	// the unit vector toward the sun: east, north, up
	east, north, up := fmath.Mul(sinAz, cosAlt), fmath.Mul(cosAz, cosAlt), sinAlt

	nx, ny := f.NX(), f.NY()
	sx := 8 * (f.PitchX() * 1000) / l.ZFactor
	sy := 8 * (f.PitchY() * 1000) / l.ZFactor
	s := &Shade{nx: nx, ny: ny, flat: sinAlt, v: make([]float64, nx*ny)}
	for j := range ny {
		jn, js := max(j-1, 0), min(j+1, ny-1)
		for i := range nx {
			nw, n, ne := f.At(i-1, jn), f.At(i, jn), f.At(i+1, jn)
			w, e := f.At(i-1, j), f.At(i+1, j)
			sw, so, se := f.At(i-1, js), f.At(i, js), f.At(i+1, js)
			// dz/dx toward the east, dz/dy toward the south
			gx := ((ne + e + e + se) - (nw + w + w + sw)) / sx
			gy := ((sw + so + so + se) - (nw + n + n + ne)) / sy
			// the surface normal is (−dz/dx_east, −dz/dy_north, 1) = (−gx, gy, 1)
			dot := fmath.Mul(-gx, east) + fmath.Mul(gy, north) + up
			length := math.Sqrt(fmath.Mul(gx, gx) + fmath.Mul(gy, gy) + 1)
			s.v[j*nx+i] = max(dot/length, 0)
		}
	}
	return s
}

// ApplyShade darkens and lightens img by s: each channel of pixel (i, j) is
// multiplied by 1 + strength·(shade − flat) and rounded, so level ground is
// unchanged, slopes facing the sun brighten, and slopes facing away darken.
// Strength 0 changes nothing. The pixel is skipped when mask is non-nil and
// mask(i, j) is false. img must be at least as large as the shade grid.
func ApplyShade(img *image.RGBA, s *Shade, strength float64, mask func(i, j int) bool) {
	b := img.Bounds()
	if b.Dx() < s.nx || b.Dy() < s.ny {
		panic(fmt.Sprintf("render: image %v smaller than shade %d × %d", b.Size(), s.nx, s.ny))
	}
	for j := range s.ny {
		for i := range s.nx {
			if mask != nil && !mask(i, j) {
				continue
			}
			k := fmath.MulAdd(strength, s.v[j*s.nx+i]-s.flat, 1)
			x, y := b.Min.X+i, b.Min.Y+j
			c := img.RGBAAt(x, y)
			img.SetRGBA(x, y, color.RGBA{
				R: toByte(fmath.Mul(float64(c.R), k)),
				G: toByte(fmath.Mul(float64(c.G), k)),
				B: toByte(fmath.Mul(float64(c.B), k)),
				A: c.A,
			})
		}
	}
}

// ReliefOptions configures Relief.
type ReliefOptions struct {
	// SeaLevel splits land from water, in the field's meters.
	SeaLevel float64
	// Light is the sun for the hillshade.
	Light Light
	// LandShade and WaterShade are the ApplyShade strengths on land and
	// water.
	LandShade, WaterShade float64
}

// DefaultReliefOptions returns sea level 0, DefaultLight, full shading on land
// and half shading on the sea floor.
func DefaultReliefOptions() ReliefOptions {
	return ReliefOptions{Light: DefaultLight, LandShade: 1, WaterShade: 0.5}
}

// Relief returns f, in meters, as a hypsometric map with hillshade: the
// Hypsometric tint, shaded by Hillshade at opts.LandShade on land and
// opts.WaterShade under water.
func Relief(f *field.Field, opts ReliefOptions) *image.RGBA {
	img := Hypsometric(f, opts.SeaLevel)
	s := Hillshade(f, opts.Light)
	land := func(i, j int) bool { return f.At(i, j) > opts.SeaLevel }
	ApplyShade(img, s, opts.LandShade, land)
	ApplyShade(img, s, opts.WaterShade, func(i, j int) bool { return !land(i, j) })
	return img
}
