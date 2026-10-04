// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package elevation

import (
	"fmt"
	"math"
	"runtime"
	"slices"
	"sync"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/internal/noise"
	"github.com/mdhender/mpg/internal/topo"
)

// Seed stream names (DESIGN.md, "Determinism"). Each keys its own noise
// source through noise.New, so each has its own lattice.
const (
	Stage       = "elevation" // continental fBm
	StageWarp   = "warp"      // domain warp
	StageRidges = "ridges"    // ridged chains and their belts
)

// Version is the elevation algorithm version. Any change to the bits of the
// field must bump it (and re-record the golden hashes).
const Version = "elevation/1"

// beltHalfWidth is half the width of the belt noise's ramp: ridges fade in
// from BeltThreshold − beltHalfWidth to BeltThreshold + beltHalfWidth.
const beltHalfWidth = 0.125

// streamBelt is the ridges source's stream for the belt noise; it is far
// from the small stream numbers the ridged octaves use.
const streamBelt uint64 = 0x62656c74 // "belt"

// streamJitter is the warp source's stream for the pull's jitter noise, far
// from the warp's own axis streams.
const streamJitter uint64 = 0x6a6974746572 // "jitter"

// Elevation fills bedrock fields for one world: the layout bias, the
// elevation inputs, and the noise sources derived from the world seed. Build
// one with New; the zero value is not valid.
type Elevation struct {
	cfg    config.Elevation
	bias   *field.Field
	cyl    topo.Cylinder
	m      noise.Cylinder
	elev   noise.Source
	warp   noise.Source
	ridges noise.Source

	contFBM   noise.FBM
	warpDef   noise.Warp
	ridged    noise.Ridged
	beltFBM   noise.FBM
	jitterFBM noise.FBM
	useWarp   bool

	landFraction float64
	// pullKm and jitterKm are the falloff's PullKm and JitterKm, scaled
	// down together when they would reach past a quarter of the playable
	// band's half height (see the package documentation).
	pullKm, jitterKm float64
}

// New returns the elevation builder for the resolved config, reading the
// layout's bias field (which must lie on the config's world).
func New(cfg config.Config, bias *field.Field) (*Elevation, error) {
	cyl, err := topo.New(cfg.World.WidthKm, cfg.World.HeightKm, cfg.Rim.Km, cfg.Rim.FalloffKm)
	if err != nil {
		return nil, fmt.Errorf("elevation: %w", err)
	}
	if bias == nil || bias.Cylinder() != cyl {
		return nil, fmt.Errorf("elevation: the bias field does not lie on the config's world")
	}
	e := cfg.Elevation
	world := uint64(cfg.Seed)
	el := &Elevation{
		cfg:    e,
		bias:   bias,
		cyl:    cyl,
		m:      noise.NewCylinder(cyl),
		elev:   noise.New(world, Stage),
		warp:   noise.New(world, StageWarp),
		ridges: noise.New(world, StageRidges),
		contFBM: noise.FBM{
			Octaves: e.Continental.Octaves, Lacunarity: e.Continental.Lacunarity,
			Gain: e.Continental.Gain, WavelengthKm: e.Continental.WavelengthKm,
		},
		warpDef: noise.Warp{StrengthKm: e.Warp.StrengthKm, FBM: noise.FBM{
			Octaves: e.Warp.Octaves, Lacunarity: e.Warp.Lacunarity,
			Gain: e.Warp.Gain, WavelengthKm: e.Warp.WavelengthKm,
		}},
		ridged: noise.Ridged{
			Octaves: e.Ridges.Octaves, Lacunarity: e.Ridges.Lacunarity, Gain: e.Ridges.Gain,
			WavelengthKm: e.Ridges.WavelengthKm, Weight: e.Ridges.Weight,
		},
		beltFBM:   noise.FBM{Octaves: 3, Lacunarity: 2, Gain: 0.5, WavelengthKm: e.Ridges.BeltWavelengthKm},
		jitterFBM: noise.FBM{Octaves: 3, Lacunarity: 2, Gain: 0.5, WavelengthKm: e.Falloff.JitterWavelengthKm},
		useWarp:   e.Warp.StrengthKm > 0,

		landFraction: cfg.World.LandFraction,
	}
	reach := e.Falloff.PullKm + e.Falloff.JitterKm
	quarter := (cyl.H() - fmath.Mul(2, cyl.Rim()+cyl.Falloff())) / 8
	scale := 1.0
	if reach > quarter {
		scale = quarter / reach
	}
	el.pullKm = fmath.Mul(e.Falloff.PullKm, scale)
	el.jitterKm = fmath.Mul(e.Falloff.JitterKm, scale)
	for _, err := range []error{el.contFBM.Validate(), el.warpDef.Validate(), el.ridged.Validate(), el.beltFBM.Validate(), el.jitterFBM.Validate()} {
		if err != nil {
			return nil, fmt.Errorf("elevation: %w", err)
		}
	}
	return el, nil
}

// Field returns a new field over the world at the bias field's spacing,
// filled with the bedrock elevation in meters (see Fill).
func (e *Elevation) Field() (*field.Field, Stats) {
	f := e.bias.Clone()
	st := e.Fill(f)
	return f, st
}

// Stats reports how a field was built.
type Stats struct {
	// Shift is the datum shift (see the package documentation): the signal
	// added at a sample of bias b is Shift·(1 + b)/2.
	Shift float64
	// Share is the share of the samples outside the rim whose shifted
	// signal is positive (land at 0 m before the falloff's ceiling).
	Share float64
	// Clamped reports that the land fraction needed a shift beyond
	// ±DatumMaxShift, so Share misses it.
	Clamped bool
}

// sample is one raster sample's state between the two passes.
type sample struct {
	c float64      // continental signal, pull applied
	w float64      // datum weight (1 + b)/2 of the raw bias b
	q noise.Point3 // the warped point on the noise cylinder
}

// Fill stores the bedrock elevation in meters at every sample of f, which
// must have the bias field's grid. It runs in two passes: the continental
// signal at every sample, then the datum shift from all of them, then the
// heights. Rows are computed concurrently, but every sample depends only on
// its own coordinates and the shift, which is found by counting the samples
// in storage order, so the result does not depend on scheduling.
func (e *Elevation) Fill(f *field.Field) Stats {
	if f.Cylinder() != e.cyl || f.NX() != e.bias.NX() || f.NY() != e.bias.NY() {
		panic("elevation: field and bias have different grids")
	}
	nx, ny := f.NX(), f.NY()
	s := make([]sample, nx*ny)
	rows(ny, func(j int) {
		y := f.Y(j)
		for i := range nx {
			s[j*nx+i] = e.signal(f.X(i), y)
		}
	})

	st := e.datum(f, s)
	// A positive shift raises the cores of the continents too; dividing by
	// 1 + shift keeps their heights in the configured range without moving
	// any coast.
	norm := 1 + max(st.Shift, 0)
	v := make([]float64, nx*ny)
	rows(ny, func(j int) {
		y := f.Y(j)
		for i := range nx {
			k := j*nx + i
			h := e.height(fmath.MulAdd(st.Shift, s[k].w, s[k].c)/norm, s[k].q)
			// Volcanic hotspots (S14) add their cones and swells here,
			// before the falloff, so the falloff holds them under the
			// ceiling too.
			v[k] = e.falloff(h, y)
		}
	})
	f.SetFunc(func(i, j int, _ topo.Point) float64 { return v[j*nx+i] })
	return st
}

// rows calls fn for every row in [0, ny), spread over GOMAXPROCS goroutines.
// fn must write only its own row's results.
func rows(ny int, fn func(j int)) {
	workers := max(1, min(runtime.GOMAXPROCS(0), ny))
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for j := w; j < ny; j += workers {
				fn(j)
			}
		})
	}
	wg.Wait()
}

// datumSteps is the number of bisection steps of the datum search.
const datumSteps = 48

// datum returns the datum shift: the δ in [−DatumMaxShift, DatumMaxShift]
// that leaves the land fraction of the samples outside the rim with
// c + δ·w > 0, found by bisection (the count only grows with δ, since w ≥ 0).
func (e *Elevation) datum(f *field.Field, s []sample) Stats {
	nx := f.NX()
	var pts []sample
	for j := range f.NY() {
		if !e.cyl.InRim(f.Y(j)) {
			pts = append(pts, s[j*nx:(j+1)*nx]...)
		}
	}
	share := func(d float64) float64 {
		n := 0
		for _, p := range pts {
			if fmath.MulAdd(d, p.w, p.c) > 0 {
				n++
			}
		}
		return float64(n) / float64(max(len(pts), 1))
	}
	want, lim := e.landFraction, e.cfg.DatumMaxShift
	if lim == 0 {
		return Stats{Share: share(0)}
	}
	if sh := share(-lim); sh >= want {
		return Stats{Shift: -lim, Share: sh, Clamped: sh > want}
	}
	if sh := share(lim); sh < want {
		return Stats{Shift: lim, Share: sh, Clamped: true}
	}
	// share(lo) < want <= share(hi)
	lo, hi := -lim, lim
	for range datumSteps {
		mid := (lo + hi) / 2
		if share(mid) < want {
			lo = mid
		} else {
			hi = mid
		}
	}
	return Stats{Shift: hi, Share: share(hi)}
}

// quantile sorts v and returns its nearest-rank value at p: the element at
// rank round(p·n), counting from 1, clamped to the slice.
func quantile(v []float64, p float64) float64 {
	slices.Sort(v)
	k := int(fmath.MulAdd(float64(len(v)), min(max(p, 0), 1), 0.5))
	return v[min(max(k-1, 0), len(v)-1)]
}

// signal returns the continental signal at the world point (x, y) km, before
// the datum shift, and the warped point the noise is read at: the layout bias
// and the continental fBm, both read at the warped point, less the polar
// pull.
func (e *Elevation) signal(x, y float64) sample {
	wx, wy := x, y
	if e.useWarp {
		wx, wy = e.warpDef.Apply(e.warp, e.m, x, y)
	}
	q := e.m.Point(wx, wy)
	raw := e.bias.Sample(wx, wy)
	b := raw
	// Flatten the bias near the nominal coast, b·((1 − k) + k·|b|), so the
	// noise draws the coasts there while the bias keeps the continents'
	// cores and the open ocean.
	k := e.cfg.BiasFlatten
	b = fmath.Mul(b, fmath.MulAdd(k, math.Abs(b), 1-k))
	c := b + fmath.Mul(e.cfg.Continental.Amplitude, e.contFBM.Sample(e.elev, q))
	return sample{c: c - e.pull(wy, q), w: fmath.Mul(1+raw, 0.5), q: q}
}

// height turns the continental signal c at the noise point q into meters,
// before the polar falloff: the sea floor below zero, and above it the
// lowland relief plus the ridges.
func (e *Elevation) height(c float64, q noise.Point3) float64 {
	if !(c > 0) {
		// Sea floor: a shallow shelf at the coast, a continental slope, and
		// an abyssal plain at −OceanDepthM once the signal reaches −1.
		return -fmath.Mul(e.cfg.OceanDepthM, smoothstep(-c))
	}
	h := fmath.Mul(e.cfg.ReliefScaleM, c)
	rc := e.cfg.Ridges
	if rc.HeightM == 0 {
		return h
	}
	land := smoothstep(c / rc.LandRamp)
	belt := smoothstep(fmath.MulAdd(e.beltFBM.Sample(e.ridges.Stream(streamBelt), q)-rc.BeltThreshold, 1/(2*beltHalfWidth), 0.5))
	if !(land > 0 && belt > 0) {
		return h
	}
	r := (e.ridged.Sample(e.ridges, q) - rc.Threshold) / (1 - rc.Threshold)
	if !(r > 0) {
		return h
	}
	// Squaring sharpens the crests and widens the foothills.
	return h + fmath.Mul(rc.HeightM, fmath.Mul(fmath.Mul(r, r), fmath.Mul(land, belt)))
}

// pull returns the polar pull on the continental signal at the warped point
// (wx, wy), whose noise point is q: 0 until PullKm inside the falloff band's
// inner edge, rising smoothly to Pull at that edge and beyond. The distance
// from the rim is moved by up to JitterKm by a coarse fBm (on the warp
// source), and the pull is subtracted from a noisy signal, so the coasts it
// makes near the poles are as ragged as any other.
func (e *Elevation) pull(wy float64, q noise.Point3) float64 {
	if e.cfg.Falloff.Pull == 0 {
		return 0
	}
	band := e.cyl.Falloff()
	pullKm, jitterKm := e.pullKm, e.jitterKm
	u := e.cyl.RimDistance(wy)
	if u >= band+pullKm+jitterKm {
		return 0
	}
	if jitterKm > 0 {
		u += fmath.Mul(jitterKm, e.jitterFBM.Sample(e.warp.Stream(streamJitter), q))
	}
	if u >= band+pullKm {
		return 0
	}
	if u <= band {
		return e.cfg.Falloff.Pull
	}
	return fmath.Mul(e.cfg.Falloff.Pull, smoothstep((band+pullKm-u)/pullKm))
}

// falloff applies the polar falloff to h at row y: a soft ceiling that eases
// in over the taper strip and holds the falloff band and rim at or below
// CeilingM, deepening to DepthM at the rim's edge.
func (e *Elevation) falloff(h, y float64) float64 {
	fc := e.cfg.Falloff
	band := e.cyl.Falloff()
	u := e.cyl.RimDistance(y)
	if u >= band+fc.TaperKm {
		return h
	}
	// s: 0 at the taper's inner edge, 1 at the falloff band's edge and beyond.
	s := 1.0
	if u > band {
		s = smoothstep((band + fc.TaperKm - u) / fc.TaperKm)
	}
	if h > fc.CeilingM {
		h -= fmath.Mul(s, h-fc.CeilingM)
	}
	if u >= band {
		return h
	}
	// In the falloff band and the rim: the limit runs from CeilingM at the
	// band's inner edge to DepthM at the rim's edge.
	r := 1.0
	if u > 0 {
		r = smoothstep(1 - u/band)
	}
	return min(h, fmath.MulAdd(r, fc.DepthM-fc.CeilingM, fc.CeilingM))
}

// smoothstep returns 3t² − 2t³ for t clamped to [0, 1].
func smoothstep(t float64) float64 {
	t = min(max(t, 0), 1)
	return fmath.Mul(fmath.Mul(t, t), 3-fmath.Mul(2, t))
}

// SeaLevelForShare returns the level that leaves about share of the samples
// outside the rim strictly above it: the elevation of the sample ranked
// round((1 − share)·n) from the bottom (nearest rank), clamped to the
// samples. It is a raster estimate of where stage 6's sea-level search on
// cell counts will land, for renders and tests; it is not that search.
func SeaLevelForShare(f *field.Field, share float64) float64 {
	c := f.Cylinder()
	var v []float64
	f.Each(func(_, j int, x float64) {
		if !c.InRim(f.Y(j)) {
			v = append(v, x)
		}
	})
	return quantile(v, 1-share)
}

// LandShare returns the share of the samples outside the rim strictly above
// seaLevel.
func LandShare(f *field.Field, seaLevel float64) float64 {
	c := f.Cylinder()
	var n, land int
	f.Each(func(_, j int, x float64) {
		if c.InRim(f.Y(j)) {
			return
		}
		n++
		if x > seaLevel {
			land++
		}
	})
	return float64(land) / float64(max(n, 1))
}

// BandMax returns the highest elevation in the falloff band and the rim.
func BandMax(f *field.Field) float64 {
	c := f.Cylinder()
	hi := f.Min()
	f.Each(func(_, j int, x float64) {
		if y := f.Y(j); c.InRim(y) || c.InFalloff(y) {
			hi = max(hi, x)
		}
	})
	return hi
}
