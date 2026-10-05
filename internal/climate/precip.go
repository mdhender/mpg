// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package climate

import (
	"math"
	"runtime"
	"sync"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/internal/noise"
	"github.com/mdhender/mpg/internal/seed"
)

// Version is the climate algorithm version. It keys the climate stage's
// noise (seed.Derive(world, Stage, Version)); any change to the bits of
// the result, a noise.Version bump included, must bump it.
const Version = "climate/1"

// Stage is the climate stage's seed stage name (DESIGN.md, "Determinism").
const Stage = "climate"

// Noise streams of the climate source: each field has its own lattice.
const (
	streamJitter = 1
	streamPrecip = 2
)

// curve is a piecewise-linear table of latitude in degrees: values joined
// by straight lines and held flat beyond the ends.
type curve struct {
	x, y []float64
}

// at returns the table's value at x degrees, kept between the end points
// of its segment, so rounding never overshoots them.
func (c curve) at(x float64) float64 {
	n := len(c.x)
	if x <= c.x[0] {
		return c.y[0]
	}
	if x >= c.x[n-1] {
		return c.y[n-1]
	}
	k := 1
	for c.x[k] < x {
		k++
	}
	x0, x1, y0, y1 := c.x[k-1], c.x[k], c.y[k-1], c.y[k]
	t := (x - x0) / (x1 - x0)
	return min(max(y0+fmath.Mul(y1-y0, t), min(y0, y1)), max(y0, y1))
}

// precip is the precipitation, evaporation and aridity part of a Model.
type precip struct {
	windward, share                         curve
	trades, westerlies, polar               float64 // northern bearings, degrees
	tradesMax, westerMin, westerMax, polMin float64
	equatorBlend                            float64
	offsets                                 []float64 // ray offsets in degrees, in ray order
	step                                    float64   // km
	steps                                   int       // steps per ray
	window                                  int       // lift window in steps
	rainout, orographic, liftGain           float64
	noiseAmp, jitterDeg                     float64
	precipFBM, jitterFBM                    noise.FBM
	petPerC, biotempMax                     float64
}

func newPrecip(c config.Climate) precip {
	p := precip{
		trades: c.TradesFromDeg, westerlies: c.WesterliesFromDeg, polar: c.PolarFromDeg,
		tradesMax: c.TradesMaxLatDeg, westerMin: c.WesterliesMinLatDeg, westerMax: c.WesterliesMaxLatDeg, polMin: c.PolarMinLatDeg,
		equatorBlend: c.EquatorBlendDeg,
		step:         c.StepKm,
		steps:        int(math.Floor(c.ReachKm / c.StepKm)),
		window:       int(math.Round(c.LiftWindowKm / c.StepKm)),
		rainout:      c.RainoutKm, orographic: c.OrographicM, liftGain: c.LiftGainPerKm,
		noiseAmp: c.PrecipNoiseAmp, jitterDeg: c.BandJitterDeg,
		precipFBM:  noise.FBM{Octaves: c.PrecipNoiseOctaves, Lacunarity: 2, Gain: 0.5, WavelengthKm: c.PrecipNoiseWavelengthKm},
		jitterFBM:  noise.FBM{Octaves: c.BandJitterOctaves, Lacunarity: 2, Gain: 0.5, WavelengthKm: c.BandJitterWavelengthKm},
		petPerC:    c.PETMmPerC,
		biotempMax: c.BiotempMaxC,
	}
	for _, q := range c.WindwardPrecipMm {
		p.windward.x, p.windward.y = append(p.windward.x, q.LatDeg), append(p.windward.y, q.Mm)
	}
	for _, q := range c.ConvectiveShare {
		p.share.x, p.share.y = append(p.share.x, q.LatDeg), append(p.share.y, q.Share)
	}
	p.offsets = make([]float64, c.WindRays)
	if n := c.WindRays; n > 1 {
		for r := range n {
			p.offsets[r] = fmath.Mul(2*c.WindSpreadDeg, float64(r))/float64(n-1) - c.WindSpreadDeg
		}
	}
	return p
}

// fan is one wind at a sample: the bearing it blows from and its weight.
type fan struct {
	from, weight float64
}

// bands appends to fans the winds at latitude phi degrees (0 to 90) of one
// hemisphere, each weighted by w times its band weight: the trades, the
// westerlies or the polar easterlies, or two of them blended linearly in
// the gap between their bands. The southern hemisphere mirrors the
// bearings north–south.
func (p *precip) bands(fans []fan, phi, w float64, south bool) []fan {
	trades, wester, polar := p.trades, p.westerlies, p.polar
	if south {
		trades, wester, polar = mirror(trades), mirror(wester), mirror(polar)
	}
	blend := func(a, b, lo, hi float64) []fan {
		t := (phi - lo) / (hi - lo)
		return append(fans, fan{a, fmath.Mul(w, 1-t)}, fan{b, fmath.Mul(w, t)})
	}
	switch {
	case phi <= p.tradesMax:
		return append(fans, fan{trades, w})
	case phi < p.westerMin:
		return blend(trades, wester, p.tradesMax, p.westerMin)
	case phi <= p.westerMax:
		return append(fans, fan{wester, w})
	case phi < p.polMin:
		return blend(wester, polar, p.westerMax, p.polMin)
	}
	return append(fans, fan{polar, w})
}

// mirror returns the southern hemisphere's bearing for the northern bearing
// b: 180° − b, in [0, 360).
func mirror(b float64) float64 { return fmath.FloorModFloat(180-b, 360) }

// winds returns the fans at signed latitude lat degrees (north positive):
// within EquatorBlendDeg of the equator both hemispheres' winds, weighted
// linearly by the distance across the blend, else one hemisphere's.
// Northern fans come first, then southern.
func (p *precip) winds(fans []fan, lat float64) []fan {
	wn := 0.0
	switch e := p.equatorBlend; {
	case lat >= e:
		wn = 1
	case lat <= -e:
		wn = 0
	default: // |lat| < e, so e > 0
		wn = (lat + e) / (2 * e)
	}
	phi := min(math.Abs(lat), 90)
	if wn > 0 {
		fans = p.bands(fans, phi, wn, false)
	}
	if wn < 1 {
		fans = p.bands(fans, phi, 1-wn, true)
	}
	return fans
}

// grid is what a trace reads: the raster's shape and each sample's cell's
// ocean flag and height above sea level.
type grid struct {
	nx, ny       int
	w, h, px, py float64
	owner        []int
	ocean        []bool
	height       []float64 // per cell
}

// newGrid returns the grid of raster f with sample owners owner and r's
// cell ocean flags and heights.
func newGrid(f *field.Field, owner []int, r *Result) *grid {
	cyl := f.Cylinder()
	return &grid{
		nx: f.NX(), ny: f.NY(), w: cyl.W(), h: cyl.H(), px: f.PitchX(), py: f.PitchY(),
		owner: owner, ocean: r.Ocean, height: r.HeightM,
	}
}

// cellAt returns the cell of the sample containing (x, y), x wrapped onto
// the cylinder, or −1 off the map's north or south edge.
func (g *grid) cellAt(x, y float64) int {
	if !(y >= 0 && y < g.h) {
		return -1
	}
	i := min(int(fmath.FloorModFloat(x, g.w)/g.px), g.nx-1)
	j := min(int(y/g.py), g.ny-1)
	return g.owner[j*g.nx+i]
}

// trace follows one ray upwind from (x, y), a sample of a land cell with
// height self, toward the bearing from (degrees), and returns the moisture
// the air brings and the lift onto the sample (hmz2bio's Profile and
// Moisture). The trace steps p.step km at a time, at most p.steps times,
// and stops at the first point in an ocean cell (open water: the rim, the
// ocean, and in the final pass the inland seas, at their height above sea
// level) or off the map: the air leaves the sea saturated. If it finds no
// sea, the air is taken to start from the sea at 0 m one step beyond the
// last point. Moisture is
// exp(−n·step/rainout − Σrise/orographic) over the n steps of the profile,
// rise being the height gained on a step toward the sample (0 descending);
// lift is self minus the lowest point within p.window steps upwind (the
// sea's 0 m included), or 0.
func (p *precip) trace(g *grid, x, y, from, self float64) (moisture, lift float64) {
	s, c := fmath.Sincos(fmath.Mul(from, math.Pi/180))
	dx, dy := fmath.Mul(s, p.step), -fmath.Mul(c, p.step)
	prev, rise, low := self, 0.0, self
	n := 0
	for k := 1; ; k++ {
		h, sea := 0.0, true
		if k <= p.steps {
			if cell := g.cellAt(x+fmath.Mul(float64(k), dx), y+fmath.Mul(float64(k), dy)); cell >= 0 {
				// Open water's height is 0 for the ocean and the rim, and
				// an inland sea's surface above the sea level.
				h, sea = g.height[cell], g.ocean[cell]
			}
		}
		rise += max(prev-h, 0)
		n = k
		if k <= p.window {
			low = min(low, h)
		}
		if sea {
			break
		}
		prev = h
	}
	e := -(fmath.Mul(float64(n), p.step) / p.rainout) - rise/p.orographic
	return fmath.Exp(e), self - low
}

// sample is one point's precipitation and its parts.
type sample struct {
	p, moisture, lift float64
}

// at returns the climate at (x, y), a point whose cell is ocean (water) or
// land of height self above sea level, using fans as scratch space.
func (p *precip) at(g *grid, src noise.Source, m noise.Cylinder, x, y float64, water bool, self float64, fans []fan) (sample, []fan) {
	q := m.Point(x, y)
	lat := fmath.Mul(90, m.Topo().Latitude(y))
	if p.jitterDeg > 0 {
		lat += fmath.Mul(p.jitterDeg, p.jitterFBM.Sample(src.Stream(streamJitter), q))
	}
	lat = min(max(lat, -90), 90)
	phi := math.Abs(lat)
	base := p.windward.at(phi)
	if p.noiseAmp > 0 {
		base = fmath.Mul(base, 1+fmath.Mul(p.noiseAmp, p.precipFBM.Sample(src.Stream(streamPrecip), q)))
	}
	if water {
		return sample{p: base, moisture: 1}, fans
	}
	fans = p.winds(fans[:0], lat)
	var moisture, lift float64
	nr := float64(len(p.offsets))
	for _, f := range fans {
		var ms, ls float64
		for _, o := range p.offsets {
			mo, li := p.trace(g, x, y, f.from+o, self)
			ms += mo
			ls += li
		}
		moisture += fmath.Mul(f.weight, ms/nr)
		lift += fmath.Mul(f.weight, ls/nr)
	}
	share := p.share.at(phi)
	gain := 1 + fmath.Mul(p.liftGain, lift)/1000
	return sample{p: fmath.Mul(base, share+fmath.Mul(fmath.Mul(1-share, moisture), gain)), moisture: moisture, lift: lift}, fans
}

// rowsDo calls fn for every row in [0, ny) on workers goroutines (at least
// 1). fn must write only its own row's results, so the output does not
// depend on the worker count.
func rowsDo(ny, workers int, fn func(j int, fans []fan) []fan) {
	workers = max(1, min(workers, ny))
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			fans := make([]fan, 0, 8)
			for j := w; j < ny; j += workers {
				fans = fn(j, fans)
			}
		})
	}
	wg.Wait()
}

// defaultWorkers is the worker count Compute uses.
func defaultWorkers() int { return runtime.GOMAXPROCS(0) }

// PET returns the potential evapotranspiration in mm per year at mean
// annual temperature t °C: Holdridge's PETMmPerC times the biotemperature,
// t clamped to [0, BiotempMaxC] (no seasons, so the biotemperature is the
// clamped mean).
func (m Model) PET(t float64) float64 {
	return fmath.Mul(m.p.petPerC, min(max(t, 0), m.p.biotempMax))
}

// Runoff returns the annual runoff in mm from precipitation p and potential
// evapotranspiration pet, both in mm and not negative, by Budyko's (1974)
// curve: with the dryness index φ = pet/p, evaporation is
// E = p·√(φ·tanh(1/φ)·(1 − e^(−φ))), the geometric mean of Schreiber's and
// Ol'dekop's curves, and runoff is p − E, never below 0. With no
// precipitation there is no runoff; with no evaporation all of it runs off.
func Runoff(p, pet float64) float64 {
	if !(p > 0) {
		return 0
	}
	if !(pet > 0) {
		return p
	}
	phi := pet / p
	if !(phi <= maxDryness) {
		return 0 // E/P is 1 to within 1e-6: everything evaporates
	}
	a := fmath.Exp(-2 * (p / pet)) // tanh(1/φ) = (1 − e^(−2/φ)) / (1 + e^(−2/φ))
	th := (1 - a) / (1 + a)
	ex := 1 - fmath.Exp(-phi)
	e := fmath.Mul(p, math.Sqrt(fmath.Mul(fmath.Mul(phi, th), ex)))
	return max(p-e, 0)
}

// maxDryness is the dryness index PET/P beyond which Runoff is 0. There
// tanh(1/φ) ≈ 1/φ and e^(−φ) ≈ 0, so E/P = 1 to far better than the
// precision of the inputs; the cut keeps a vanishing p from making
// 0 × ∞ in the curve.
const maxDryness = 1e6

// AridityCap is the largest aridity index stored: humid enough for every
// class, and the value where there is no evaporation.
const AridityCap = 10

// AridityIndex returns the aridity index P/PET (UNEP), capped at AridityCap,
// and AridityCap when pet is 0.
func AridityIndex(p, pet float64) float64 {
	if !(pet > 0) {
		return AridityCap
	}
	return min(p/pet, AridityCap)
}

// Aridity is a UNEP aridity class.
type Aridity uint8

// The UNEP aridity classes, driest first, by the aridity index P/PET.
const (
	HyperArid   Aridity = iota // below 0.05
	Arid                       // 0.05 to below 0.2
	SemiArid                   // 0.2 to below 0.5
	DrySubhumid                // 0.5 to below 0.65
	Humid                      // 0.65 and above
	NumAridity  = 5
)

// AridityBreaks are the aridity index values that start each class after
// HyperArid.
var AridityBreaks = [NumAridity - 1]float64{0.05, 0.2, 0.5, 0.65}

// AridityOf returns the class of aridity index ai.
func AridityOf(ai float64) Aridity {
	a := HyperArid
	for _, b := range AridityBreaks {
		if ai >= b {
			a++
		}
	}
	return a
}

// String returns the class's kebab-case name.
func (a Aridity) String() string {
	switch a {
	case HyperArid:
		return "hyper-arid"
	case Arid:
		return "arid"
	case SemiArid:
		return "semi-arid"
	case DrySubhumid:
		return "dry-subhumid"
	case Humid:
		return "humid"
	}
	return "unknown"
}

// precipitate fills r's precipitation, moisture, lift, PET, runoff and
// aridity: every raster sample's precipitation on workers goroutines, one
// row each, then the per-cell means in storage order, then each cell's
// PET, runoff and aridity from its mean precipitation and its temperature.
// A cell with no samples takes the climate at its site.
func (r *Result) precipitate(f *field.Field, g *grid, siteX, siteY []float64, counts []int, model Model, world uint64, workers int) {
	p := &model.p
	src := noise.NewSource(seed.Derive(world, Stage, Version))
	cyl := noise.NewCylinder(f.Cylinder())
	nx, ny := g.nx, g.ny
	n := len(r.Ocean)
	out := make([]sample, nx*ny)
	rowsDo(ny, workers, func(j int, fans []fan) []fan {
		y := f.Y(j)
		for i := range nx {
			k := j*nx + i
			c := g.owner[k]
			out[k], fans = p.at(g, src, cyl, f.X(i), y, g.ocean[c], g.height[c], fans)
		}
		return fans
	})
	r.RasterPrecipitation = make([]float64, nx*ny)
	r.Precipitation = make([]float64, n)
	r.Moisture = make([]float64, n)
	r.LiftM = make([]float64, n)
	for k, s := range out {
		c := g.owner[k]
		r.RasterPrecipitation[k] = s.p
		r.Precipitation[c] += s.p
		r.Moisture[c] += s.moisture
		r.LiftM[c] += s.lift
	}
	var fans []fan
	for c := range n {
		if counts[c] > 0 {
			d := float64(counts[c])
			r.Precipitation[c] /= d
			r.Moisture[c] /= d
			r.LiftM[c] /= d
			continue
		}
		var s sample
		s, fans = p.at(g, src, cyl, siteX[c], siteY[c], g.ocean[c], g.height[c], fans)
		r.Precipitation[c], r.Moisture[c], r.LiftM[c] = s.p, s.moisture, s.lift
	}
	r.PET = make([]float64, n)
	r.Runoff = make([]float64, n)
	r.Aridity = make([]float64, n)
	for c := range n {
		r.PET[c] = model.PET(r.Temperature[c])
		r.Runoff[c] = Runoff(r.Precipitation[c], r.PET[c])
		r.Aridity[c] = AridityIndex(r.Precipitation[c], r.PET[c])
	}
}
