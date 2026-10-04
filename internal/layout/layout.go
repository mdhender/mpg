// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package layout

import (
	"cmp"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/internal/noise"
	"github.com/mdhender/mpg/internal/seed"
	"github.com/mdhender/mpg/internal/topo"
)

// Stage is the seed stream name of the layout stage.
const Stage = "layout"

// Version is the layout algorithm version, the version argument to
// seed.Derive for the placement stream. Any change to the placement or to the
// bias field's bits must bump it.
const Version = "layout/1"

// Placement limits: a mass is tried at tries random positions before the
// spacing is relaxed by relaxFactor, at most maxRelax times; a lobe tries
// lobeTries directions before it is clamped into the band.
const (
	tries       = 64
	relaxFactor = 0.9
	maxRelax    = 40
	lobeTries   = 8
)

// lobeOverlap is the share of a lobe's disc that each added lobe contributes
// to its mass's area per unit of lobe step: two discs of radius r whose
// centers are r apart overlap in 1.228r², so the second adds 0.61πr².
const lobeOverlap = 0.61

// Attractor is a seeded point that raises the bias around it.
type Attractor struct {
	// X and Y locate the attractor in world km.
	X, Y float64
	// RadiusKm is where the attractor's bias crosses zero (before wobble).
	RadiusKm float64
	// Weight is the bias at the center, in (0, 1].
	Weight float64
	// Mass is the landmass the attractor belongs to. Attractors of different
	// masses are rivals.
	Mass int
}

// Repulsor lowers the bias around it, holding a strait open between two
// rival masses.
type Repulsor struct {
	// X and Y locate the repulsor in world km.
	X, Y float64
	// RadiusKm is the repulsor's reach: its effect is zero beyond it.
	RadiusKm float64
	// Weight is the bias subtracted at the center.
	Weight float64
	// MassA < MassB are the rivals it separates, both −1 for an explicit
	// custom repulsor.
	MassA, MassB int
}

// Layout is the result of the layout stage: the attractors, repulsors, and
// the parameters of the bias field built from them (see Bias).
type Layout struct {
	// Preset is the preset that produced the layout.
	Preset string
	// Attractors lists the attractors, grouped by mass in mass order.
	Attractors []Attractor
	// Repulsors lists the repulsors: explicit ones first, then one per close
	// rival pair, in mass-pair order.
	Repulsors []Repulsor
	// Masses is the number of landmasses.
	Masses int
	// SpacingWanted is the preset's rival spacing, and Spacing the spacing
	// placement achieved after Relaxations relaxations (equal when none
	// were needed). Both are 0 for custom.
	SpacingWanted, Spacing float64
	Relaxations            int
	// Wobble is the radius wobble amplitude, and WobbleKm the wavelength
	// of the wobble noise.
	Wobble, WobbleKm float64

	cyl    topo.Cylinder
	source noise.Source
}

// Cylinder returns the world the layout lies on.
func (l *Layout) Cylinder() topo.Cylinder { return l.cyl }

// New places the layout for the resolved config: the preset's attractors
// and repulsors, seeded from the world seed's "layout" stream.
func New(cfg config.Config) (*Layout, error) {
	cyl, err := topo.New(cfg.World.WidthKm, cfg.World.HeightKm, cfg.Rim.Km, cfg.Rim.FalloffKm)
	if err != nil {
		return nil, fmt.Errorf("layout: %w", err)
	}
	lc := cfg.Layout
	l := &Layout{
		Preset: lc.Preset,
		cyl:    cyl,
		source: noise.New(uint64(cfg.Seed), Stage),
	}
	var rivals config.Rivals
	if p, ok := lc.Pattern(lc.Preset); ok {
		rng := seed.Rand(uint64(cfg.Seed), Stage, Version)
		land := fmath.Mul(float64(cfg.World.LandCells), cfg.Province.AreaKm2)
		l.place(p, land, lc.PoleMargin, rng)
		rivals = p.Rivals
	} else if lc.Preset == config.PresetCustom {
		l.custom(lc.Custom)
		rivals = lc.Custom.Rivals
	} else {
		return nil, fmt.Errorf("layout: unknown preset %q", lc.Preset)
	}
	l.addRivalRepulsors(rivals)
	l.Wobble = rivals.Wobble
	l.WobbleKm = l.wobbleWavelength()
	return l, nil
}

// halfBand returns half the height of the playable band outside the rim and
// falloff.
func (l *Layout) halfBand() float64 {
	return (l.cyl.H() - fmath.Mul(2, l.cyl.Rim()+l.cyl.Falloff())) / 2
}

// band returns the y range a lobe of radius r may be centered in: the
// playable band outside the rim and falloff, shrunk by margin·r at each
// side, but never to less than its middle tenth.
func (l *Layout) band(r, margin float64) (lo, hi float64) {
	edge := l.cyl.Rim() + l.cyl.Falloff()
	inset := min(fmath.Mul(margin, r), fmath.Mul(0.95, l.halfBand()))
	return edge + inset, l.cyl.H() - edge - inset
}

// uniform returns a value drawn uniformly from [lo, hi).
func uniform(rng *rand.Rand, lo, hi float64) float64 {
	return fmath.MulAdd(hi-lo, rng.Float64(), lo)
}

// place fills the attractors of a seeded preset. land is the land target's
// area in km².
func (l *Layout) place(p config.Pattern, land, margin float64, rng *rand.Rand) {
	n := p.MassesMin + rng.IntN(p.MassesMax-p.MassesMin+1)
	type mass struct {
		index int
		share float64
	}
	masses := make([]mass, n)
	var total float64
	for k := range masses {
		s := uniform(rng, 1, p.SizeRatio)
		masses[k] = mass{k, s}
		total += s
	}
	// Largest first, so the big masses claim room before the small ones.
	slices.SortStableFunc(masses, func(a, b mass) int { return cmp.Compare(b.share, a.share) })

	area := fmath.Mul(p.Coverage, land)
	spacing := p.Spacing
	l.Masses = n
	l.SpacingWanted = p.Spacing
	for id, m := range masses {
		lobes := p.LobesMin + rng.IntN(p.LobesMax-p.LobesMin+1)
		grow := 1 + fmath.Mul(float64(lobes-1), min(1, fmath.Mul(lobeOverlap, p.LobeStep)))
		r := math.Sqrt(fmath.Mul(area, m.share/total) / fmath.Mul(math.Pi, grow))
		// A lobe must fit in the band with its margin.
		if margin > 0 {
			r = min(r, fmath.Mul(0.95, l.halfBand())/margin)
		}
		var cand []Attractor
		for relax := 0; ; relax++ {
			placed := false
			for range tries {
				cand = l.mass(cand[:0], p, id, lobes, r, margin, rng)
				if l.clear(cand, spacing) {
					placed = true
					break
				}
			}
			if placed || relax == maxRelax {
				break
			}
			spacing = fmath.Mul(spacing, relaxFactor)
			l.Relaxations++
		}
		l.Attractors = append(l.Attractors, cand...)
	}
	l.Spacing = spacing
}

// mass appends to dst one candidate landmass of the given lobe count and
// radius: a first lobe at a random point of the band, and each further lobe
// LobeStep radii from an earlier lobe: the most open of lobeTries random
// candidates.
func (l *Layout) mass(dst []Attractor, p config.Pattern, id, lobes int, r, margin float64, rng *rand.Rand) []Attractor {
	lo, hi := l.band(r, margin)
	weight := func() float64 { return uniform(rng, p.WeightMin, p.WeightMax) }
	dst = append(dst, Attractor{
		X:        uniform(rng, 0, l.cyl.W()),
		Y:        uniform(rng, lo, hi),
		RadiusKm: r,
		Weight:   weight(),
		Mass:     id,
	})
	step := fmath.Mul(p.LobeStep, r)
	for range lobes - 1 {
		// Grow toward open ground: of lobeTries candidates, each a random
		// direction from a random earlier lobe, keep the one farthest from
		// the lobes so far. Candidates outside the band lose to any inside.
		var bx, by, bestScore float64
		bestIn := false
		for try := range lobeTries {
			parent := dst[rng.IntN(len(dst))]
			sin, cos := fmath.Sincos(fmath.Mul(2*math.Pi, rng.Float64()))
			x := l.cyl.WrapX(fmath.MulAdd(step, cos, parent.X))
			y := fmath.MulAdd(step, sin, parent.Y)
			in := y >= lo && y <= hi
			score := math.Inf(1)
			for _, a := range dst {
				score = min(score, l.cyl.Distance(topo.Point{X: x, Y: y}, topo.Point{X: a.X, Y: a.Y}))
			}
			if try == 0 || (in && !bestIn) || (in == bestIn && score > bestScore) {
				bx, by, bestScore, bestIn = x, y, score, in
			}
		}
		dst = append(dst, Attractor{
			X:        bx,
			Y:        min(max(by, lo), hi),
			RadiusKm: r,
			Weight:   weight(),
			Mass:     id,
		})
	}
	return dst
}

// clear reports whether every candidate lobe is at least spacing times the
// sum of the radii from every attractor already placed (all of which belong
// to rival masses).
func (l *Layout) clear(cand []Attractor, spacing float64) bool {
	for _, c := range cand {
		for _, a := range l.Attractors {
			d := l.cyl.Distance(topo.Point{X: c.X, Y: c.Y}, topo.Point{X: a.X, Y: a.Y})
			if d < fmath.Mul(spacing, c.RadiusKm+a.RadiusKm) {
				return false
			}
		}
	}
	return true
}

// custom copies the custom preset's explicit attractors and repulsors. Masses
// are renumbered densely in order of their configured ids, so Mass is an
// index in [0, Masses).
func (l *Layout) custom(c config.Custom) {
	var ids []int
	for _, a := range c.Attractors {
		ids = append(ids, a.Mass)
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	l.Masses = len(ids)
	for _, a := range c.Attractors {
		m, _ := slices.BinarySearch(ids, a.Mass)
		l.Attractors = append(l.Attractors, Attractor{X: a.XKm, Y: a.YKm, RadiusKm: a.RadiusKm, Weight: a.Weight, Mass: m})
	}
	// Group by mass, keeping the configured order within each.
	slices.SortStableFunc(l.Attractors, func(a, b Attractor) int { return cmp.Compare(a.Mass, b.Mass) })
	for _, r := range c.Repulsors {
		l.Repulsors = append(l.Repulsors, Repulsor{X: r.XKm, Y: r.YKm, RadiusKm: r.RadiusKm, Weight: r.Weight, MassA: -1, MassB: -1})
	}
}

// addRivalRepulsors places one repulsor between each pair of rival masses
// whose closest attractors are nearer than RepulsorReach times the sum of
// their radii. It sits on the line between those two attractors, in the
// middle of the gap between their discs.
func (l *Layout) addRivalRepulsors(rv config.Rivals) {
	if rv.RepulsorWeight == 0 || rv.RepulsorReach == 0 {
		return
	}
	// start[m] is the index of mass m's first attractor.
	start := make([]int, l.Masses+1)
	for _, a := range l.Attractors {
		start[a.Mass+1]++
	}
	for m := range l.Masses {
		start[m+1] += start[m]
	}
	for ma := range l.Masses {
		for mb := ma + 1; mb < l.Masses; mb++ {
			bi, bj, best := -1, -1, math.Inf(1)
			for i := start[ma]; i < start[ma+1]; i++ {
				for j := start[mb]; j < start[mb+1]; j++ {
					a, b := l.Attractors[i], l.Attractors[j]
					d := l.cyl.Distance(topo.Point{X: a.X, Y: a.Y}, topo.Point{X: b.X, Y: b.Y})
					if d < best {
						bi, bj, best = i, j, d
					}
				}
			}
			if bi < 0 {
				continue
			}
			a, b := l.Attractors[bi], l.Attractors[bj]
			sum := a.RadiusKm + b.RadiusKm
			if !(best < fmath.Mul(rv.RepulsorReach, sum)) {
				continue
			}
			x, y := a.X, a.Y
			if best > 0 {
				dx, dy := l.cyl.Delta(topo.Point{X: a.X, Y: a.Y}, topo.Point{X: b.X, Y: b.Y})
				along := (a.RadiusKm + fmath.Mul(best-sum, 0.5)) / best
				x = l.cyl.WrapX(fmath.MulAdd(dx, along, a.X))
				y = fmath.MulAdd(dy, along, a.Y)
			}
			l.Repulsors = append(l.Repulsors, Repulsor{
				X:        x,
				Y:        y,
				RadiusKm: fmath.Mul(rv.RepulsorRadius, sum/2),
				Weight:   rv.RepulsorWeight,
				MassA:    ma,
				MassB:    mb,
			})
		}
	}
}

// wobbleWavelength returns the wobble noise wavelength: twice the mean
// attractor radius.
func (l *Layout) wobbleWavelength() float64 {
	var sum float64
	for _, a := range l.Attractors {
		sum += a.RadiusKm
	}
	if len(l.Attractors) == 0 {
		return 1
	}
	return 2 * sum / float64(len(l.Attractors))
}
