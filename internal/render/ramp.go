// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package render

import (
	"fmt"
	"image/color"
	"math"

	"github.com/mdhender/mpg/internal/fmath"
)

// Stop is one color stop of a Ramp: the color at Value.
type Stop struct {
	Value float64
	Color color.RGBA
}

// Ramp is a piecewise-linear color map over a value domain. Values below the
// first stop take its color and values above the last stop take the last
// color. Build one with NewRamp; the zero value is not valid.
type Ramp struct {
	stops []Stop
}

// NewRamp returns a ramp through stops, which must be finite and strictly
// increasing in value. It panics on an empty, unordered, or non-finite list,
// since ramps are fixed by code.
func NewRamp(stops ...Stop) Ramp {
	if len(stops) == 0 {
		panic("render: ramp has no stops")
	}
	for k, s := range stops {
		if math.IsNaN(s.Value) || math.IsInf(s.Value, 0) {
			panic(fmt.Sprintf("render: ramp stop %d value %v is not finite", k, s.Value))
		}
		if k > 0 && !(s.Value > stops[k-1].Value) {
			panic(fmt.Sprintf("render: ramp stop %d value %v does not increase", k, s.Value))
		}
	}
	return Ramp{stops: append([]Stop(nil), stops...)}
}

// Domain returns the values of the first and last stops.
func (r Ramp) Domain() (lo, hi float64) {
	return r.stops[0].Value, r.stops[len(r.stops)-1].Value
}

// Stops returns a copy of the ramp's stops.
func (r Ramp) Stops() []Stop { return append([]Stop(nil), r.stops...) }

// At returns the color at v, interpolating each channel linearly between the
// two stops around v and rounding to the nearest integer. NaN takes the
// first stop's color.
func (r Ramp) At(v float64) color.RGBA {
	s := r.stops
	if !(v > s[0].Value) {
		return s[0].Color
	}
	last := len(s) - 1
	if v >= s[last].Value {
		return s[last].Color
	}
	k := 1
	for s[k].Value <= v {
		k++
	}
	a, b := s[k-1], s[k]
	t := (v - a.Value) / (b.Value - a.Value)
	return color.RGBA{
		R: lerpByte(a.Color.R, b.Color.R, t),
		G: lerpByte(a.Color.G, b.Color.G, t),
		B: lerpByte(a.Color.B, b.Color.B, t),
		A: lerpByte(a.Color.A, b.Color.A, t),
	}
}

// Frac returns the color at fraction t of the ramp's domain: t = 0 is the
// first stop and t = 1 the last.
func (r Ramp) Frac(t float64) color.RGBA {
	lo, hi := r.Domain()
	return r.At(fmath.MulAdd(t, hi-lo, lo))
}

// lerpByte returns a + t·(b − a) rounded to the nearest byte.
func lerpByte(a, b uint8, t float64) uint8 {
	return toByte(fmath.MulAdd(t, float64(b)-float64(a), float64(a)))
}

// toByte rounds v half up and clamps it to [0, 255]. NaN gives 0.
func toByte(v float64) uint8 {
	v = math.Floor(v + 0.5)
	if !(v > 0) {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(int(v))
}

func rgb(hex uint32) color.RGBA {
	return color.RGBA{R: uint8(hex >> 16), G: uint8(hex >> 8), B: uint8(hex), A: 255}
}

// Predefined ramps.
var (
	// Land colors land by height above sea level in meters: lowland green
	// through tan and brown to bare rock and snow. The land part of the
	// hypsometric tint; adapted from the maloquacious/wg topo tint.
	Land = NewRamp(
		Stop{0, rgb(0x5a9a52)},
		Stop{200, rgb(0x8cb462)},
		Stop{600, rgb(0xcdc882)},
		Stop{1200, rgb(0xc8a069)},
		Stop{2000, rgb(0xa07350)},
		Stop{3000, rgb(0x8a7468)},
		Stop{4500, rgb(0xf0f0f0)},
	)

	// Water colors water by depth below sea level in meters, from pale
	// shallows to deep navy. The water part of the hypsometric tint.
	Water = NewRamp(
		Stop{0, rgb(0xa5d2e6)},
		Stop{200, rgb(0x78afd7)},
		Stop{1000, rgb(0x4682be)},
		Stop{3000, rgb(0x235096)},
		Stop{6000, rgb(0x0f285f)},
	)

	// Gray runs from black at 0 to white at 1.
	Gray = NewRamp(
		Stop{0, rgb(0x000000)},
		Stop{1, rgb(0xffffff)},
	)

	// Viridis is a perceptually uniform sequential ramp on [0, 1], sampled at
	// five points from matplotlib's viridis (CC0). Use it for bias fields,
	// noise, temperature, and other one-sided quantities.
	Viridis = NewRamp(
		Stop{0.00, rgb(0x440154)},
		Stop{0.25, rgb(0x3b528b)},
		Stop{0.50, rgb(0x21918c)},
		Stop{0.75, rgb(0x5ec962)},
		Stop{1.00, rgb(0xfde725)},
	)

	// BlueRed is a diverging ramp on [−1, 1]: blue below the center, near
	// white at 0, red above (ColorBrewer RdBu end points).
	BlueRed = NewRamp(
		Stop{-1, rgb(0x2166ac)},
		Stop{0, rgb(0xf7f7f7)},
		Stop{1, rgb(0xb2182b)},
	)
)
