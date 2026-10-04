// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package elevation

import (
	"math"
	"math/rand/v2"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/internal/seed"
	"github.com/mdhender/mpg/internal/topo"
)

// StageVolcanic is the seed stream of the hotspot draws (DESIGN.md,
// "Determinism").
const StageVolcanic = "volcanic"

// VolcanicVersion is the hotspot draw's algorithm version, the version
// argument to seed.Derive for the volcanic stream. Any change to the count,
// the placement, or the order of the draws must bump it.
const VolcanicVersion = "volcanic/1"

// poissonChunk is the largest mean drawn in one run of Knuth's method; a
// larger mean is split into chunks, since a sum of independent Poisson
// counts is Poisson with the summed mean. e^−256 is far above the subnormal
// range, so the running product never underflows before it stops.
const poissonChunk = 256

// Hotspot is one volcanic hotspot (DESIGN.md, "Volcanic hotspots"): a cone
// on a broad, low swell, added to the bedrock before the polar falloff.
// Later stages read the list to flag the volcano cell and the volcanic
// highlands around it.
type Hotspot struct {
	// X and Y are the peak's world position in km, X in [0, W).
	X, Y float64
	// PeakM is the cone's height in meters above the ground it stands on,
	// and ConeRadiusKm the radius in km at which it meets that ground.
	PeakM        float64
	ConeRadiusKm float64
	// SwellM is the swell's height in meters, and SwellRadiusKm its radius
	// in km.
	SwellM        float64
	SwellRadiusKm float64
}

// Point returns the peak's position.
func (h Hotspot) Point() topo.Point { return topo.Point{X: h.X, Y: h.Y} }

// Reach returns the distance in km beyond which the hotspot adds nothing.
func (h Hotspot) Reach() float64 { return max(h.ConeRadiusKm, h.SwellRadiusKm) }

// Rise returns the height in meters the hotspot adds at distance d km from
// its peak: the cone PeakM·smoothstep(1 − d/ConeRadiusKm), whose summit is
// rounded and whose foot meets the ground without a kink, plus the swell
// SwellM·smoothstep(1 − (d/SwellRadiusKm)²), flat-topped like a plateau
// and easing out to nothing at its radius.
func (h Hotspot) Rise(d float64) float64 {
	var cone, swell float64
	if d < h.ConeRadiusKm {
		cone = fmath.Mul(h.PeakM, smoothstep(1-d/h.ConeRadiusKm))
	}
	if d < h.SwellRadiusKm {
		t := d / h.SwellRadiusKm
		swell = fmath.Mul(h.SwellM, smoothstep(1-fmath.Mul(t, t)))
	}
	return cone + swell
}

// Hotspots draws the volcanic hotspots for a resolved config, from the
// "volcanic" seed stream alone, so the list does not depend on the layout
// or the noise. The count is a Poisson draw with mean HotspotsPerMkm2 per
// million km² of playable area; each hotspot then draws, in order, its x
// and y (uniform over the playable band outside the rim and the falloff
// band, wrapping east–west), its peak height and its cone radius (uniform
// over their configured ranges). The swell is the configured one.
func Hotspots(cfg config.Config) []Hotspot {
	v := cfg.Volcanic
	rng := seed.Rand(uint64(cfg.Seed), StageVolcanic, VolcanicVersion)
	mean := fmath.Mul(v.HotspotsPerMkm2, cfg.World.PlayableAreaKm2/1e6)
	n := poisson(rng, mean)
	if n == 0 {
		return nil
	}
	w, h := cfg.World.WidthKm, cfg.World.HeightKm
	lo := cfg.Rim.Km + cfg.Rim.FalloffKm
	hi := h - lo
	hs := make([]Hotspot, n)
	for k := range hs {
		x := fmath.FloorModFloat(fmath.Mul(w, rng.Float64()), w)
		y := uniform(rng, lo, hi)
		hs[k] = Hotspot{
			X: x, Y: y,
			PeakM:         uniform(rng, v.ConePeakMinM, v.ConePeakMaxM),
			ConeRadiusKm:  uniform(rng, v.ConeRadiusMinKm, v.ConeRadiusMaxKm),
			SwellM:        v.SwellM,
			SwellRadiusKm: v.SwellRadiusKm,
		}
	}
	return hs
}

// uniform returns a uniform draw in [lo, hi): lo + (hi − lo)·u.
func uniform(rng *rand.Rand, lo, hi float64) float64 {
	return fmath.MulAdd(hi-lo, rng.Float64(), lo)
}

// poisson returns a Poisson draw with the given mean by Knuth's method:
// multiply uniforms until the product falls to e^−mean, counting them. A
// mean above poissonChunk is drawn as a sum of chunks, in order. Each step is
// one rounded product, so the count is the same on every machine. A mean of
// 0 (or less) draws nothing and returns 0.
func poisson(rng *rand.Rand, mean float64) int {
	n := 0
	for rem := mean; rem > 0; rem -= poissonChunk {
		limit := fmath.Exp(-min(rem, poissonChunk))
		p := rng.Float64()
		for p > limit {
			n++
			p = fmath.Mul(p, rng.Float64())
		}
	}
	return n
}

// rises returns, for each sample of f, the summed rise of the hotspots, or
// nil when there are none. Each hotspot visits only the samples within its
// reach, using the wrapped east–west distance, so a hotspot near the seam
// raises both sides of it alike. The hotspots are added in list order at
// every sample, so the sums do not depend on scheduling.
func rises(f *field.Field, hs []Hotspot) []float64 {
	if len(hs) == 0 {
		return nil
	}
	c := f.Cylinder()
	nx, ny := f.NX(), f.NY()
	px, py := f.PitchX(), f.PitchY()
	d := make([]float64, nx*ny)
	for _, h := range hs {
		r := h.Reach()
		// Sample (i, j) sits at ((i + 0.5)·px, (j + 0.5)·py).
		j0 := max(int(math.Floor((h.Y-r)/py-0.5)), 0)
		j1 := min(int(math.Ceil((h.Y+r)/py-0.5)), ny-1)
		i0 := int(math.Floor((h.X-r)/px - 0.5))
		cols := min(int(math.Ceil((h.X+r)/px-0.5))-i0+1, nx)
		for j := j0; j <= j1; j++ {
			dy := f.Y(j) - h.Y
			for k := range cols {
				i := fmath.FloorMod(i0+k, nx)
				dist := fmath.Hypot(c.DX(h.X, f.X(i)), dy)
				if dist < r {
					d[j*nx+i] += h.Rise(dist)
				}
			}
		}
	}
	return d
}
