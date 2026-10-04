// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package config

import (
	"fmt"
	"math"
	"slices"

	"github.com/mdhender/mpg/internal/topo"
)

// Layout preset names.
const (
	PresetPangaea     = "pangaea"
	PresetContinents  = "continents"
	PresetArchipelago = "archipelago"
	PresetIslands     = "islands"
	PresetCustom      = "custom"
)

// Presets lists the layout preset names in documentation order.
var Presets = []string{PresetPangaea, PresetContinents, PresetArchipelago, PresetIslands, PresetCustom}

// Limits on layout inputs. They only reject absurd requests.
const (
	MaxLayoutMasses     = 256
	MaxLayoutLobes      = 64
	MaxCustomAttractors = 4096
	MaxCustomRepulsors  = 4096
)

// Layout sets the continental bias field (DESIGN.md, "Layout"): where land
// is encouraged and where straits are held open. Every preset's parameters
// are written out, so switching Preset needs no other change, and the
// unused presets' parameters do not affect the run (they are still part of
// the config hash).
type Layout struct {
	// Preset selects the pattern: pangaea, continents, archipelago,
	// islands, or custom.
	Preset string `json:"preset"`
	// PoleMargin keeps attractors away from the polar bands: an attractor's
	// center lies at least PoleMargin × its radius inside the playable band
	// (outside the rim and its falloff). The custom preset's attractors
	// need only lie outside the rim and falloff.
	PoleMargin  float64 `json:"pole_margin"`
	Pangaea     Pattern `json:"pangaea"`
	Continents  Pattern `json:"continents"`
	Archipelago Pattern `json:"archipelago"`
	Islands     Pattern `json:"islands"`
	Custom      Custom  `json:"custom"`
}

// Pattern is a seeded layout preset: a number of landmasses ("masses"),
// each a cluster of overlapping attractors ("lobes"), placed at random with
// a minimum spacing between rival masses. Sizes are relative to the land
// target, so a preset keeps its look at every world size.
type Pattern struct {
	// MassesMin and MassesMax bound the number of landmasses, drawn
	// uniformly.
	MassesMin int `json:"masses_min"`
	MassesMax int `json:"masses_max"`
	// LobesMin and LobesMax bound the attractors per landmass, drawn
	// uniformly for each.
	LobesMin int `json:"lobes_min"`
	LobesMax int `json:"lobes_max"`
	// Coverage is the attractors' total nominal area (inside their radii)
	// as a multiple of the land target's area, N × A.
	Coverage float64 `json:"coverage"`
	// SizeRatio (≥ 1) spreads the landmass sizes: each mass draws a share
	// uniformly from [1, SizeRatio], and the shares are normalized.
	SizeRatio float64 `json:"size_ratio"`
	// LobeStep is how far each new lobe lies from the lobe it grows from,
	// in lobe radii.
	LobeStep float64 `json:"lobe_step"`
	// Spacing is the minimum distance between attractors of rival
	// landmasses, as a multiple of the sum of their radii (1 means their
	// discs just touch). Placement relaxes it when it cannot be met.
	Spacing float64 `json:"spacing"`
	// WeightMin and WeightMax bound each attractor's weight, its bias at
	// the center, in (0, 1].
	WeightMin float64 `json:"weight_min"`
	WeightMax float64 `json:"weight_max"`
	// Rivals sets the shape wobble and the repulsors between rival masses.
	Rivals Rivals `json:"rivals"`
}

// Rivals sets the shape wobble and the repulsors that hold rival landmasses
// apart.
type Rivals struct {
	// Wobble, in [0, 1), scales each attractor's radius by 1 + Wobble × n,
	// with n smooth periodic noise in [−1, 1], so masses are not round.
	Wobble float64 `json:"wobble"`
	// RepulsorWeight is the bias a repulsor subtracts at its center; 0
	// places none.
	RepulsorWeight float64 `json:"repulsor_weight"`
	// RepulsorReach selects the rival pairs that get a repulsor: the closest
	// attractors of two masses closer than RepulsorReach × the sum of their
	// radii.
	RepulsorReach float64 `json:"repulsor_reach"`
	// RepulsorRadius is a repulsor's radius as a multiple of the mean of
	// the two attractors' radii.
	RepulsorRadius float64 `json:"repulsor_radius"`
}

// Custom is the custom preset: an explicit list of attractors, grouped into
// landmasses by Mass, and optional explicit repulsors. Repulsors between
// rival masses are added as for the seeded presets.
type Custom struct {
	Attractors []CustomAttractor `json:"attractors"`
	Repulsors  []CustomRepulsor  `json:"repulsors"`
	Rivals     Rivals            `json:"rivals"`
}

// CustomAttractor is one explicit attractor, in world km.
type CustomAttractor struct {
	XKm      float64 `json:"x_km"`
	YKm      float64 `json:"y_km"`
	RadiusKm float64 `json:"radius_km"`
	// Weight is the bias at the center, in (0, 1].
	Weight float64 `json:"weight"`
	// Mass groups attractors into one landmass; attractors with different
	// masses are rivals.
	Mass int `json:"mass"`
}

// CustomRepulsor is one explicit repulsor, in world km.
type CustomRepulsor struct {
	XKm      float64 `json:"x_km"`
	YKm      float64 `json:"y_km"`
	RadiusKm float64 `json:"radius_km"`
	// Weight is the bias subtracted at the center, > 0.
	Weight float64 `json:"weight"`
}

// Pattern returns the parameters of the named seeded preset, and false for
// custom or an unknown name.
func (l *Layout) Pattern(name string) (Pattern, bool) {
	switch name {
	case PresetPangaea:
		return l.Pangaea, true
	case PresetContinents:
		return l.Continents, true
	case PresetArchipelago:
		return l.Archipelago, true
	case PresetIslands:
		return l.Islands, true
	}
	return Pattern{}, false
}

// DefaultLayout returns the default layout inputs.
func DefaultLayout() Layout {
	return Layout{
		Preset:     PresetContinents,
		PoleMargin: 1,
		Pangaea: Pattern{
			MassesMin: 1, MassesMax: 1,
			LobesMin: 12, LobesMax: 16,
			Coverage: 1.15, SizeRatio: 1, LobeStep: 1.1, Spacing: 1,
			WeightMin: 0.7, WeightMax: 1,
			Rivals: Rivals{Wobble: 0.3, RepulsorWeight: 0.8, RepulsorReach: 1.6, RepulsorRadius: 0.7},
		},
		Continents: Pattern{
			MassesMin: 3, MassesMax: 5,
			LobesMin: 2, LobesMax: 4,
			Coverage: 1.05, SizeRatio: 2, LobeStep: 0.9, Spacing: 1.25,
			WeightMin: 0.7, WeightMax: 1,
			Rivals: Rivals{Wobble: 0.3, RepulsorWeight: 0.8, RepulsorReach: 1.6, RepulsorRadius: 0.7},
		},
		Archipelago: Pattern{
			MassesMin: 9, MassesMax: 14,
			LobesMin: 1, LobesMax: 3,
			Coverage: 1.05, SizeRatio: 2, LobeStep: 0.9, Spacing: 1.2,
			WeightMin: 0.6, WeightMax: 1,
			Rivals: Rivals{Wobble: 0.3, RepulsorWeight: 0.8, RepulsorReach: 1.6, RepulsorRadius: 0.7},
		},
		Islands: Pattern{
			MassesMin: 24, MassesMax: 36,
			LobesMin: 1, LobesMax: 2,
			Coverage: 1.05, SizeRatio: 3, LobeStep: 0.8, Spacing: 1.15,
			WeightMin: 0.5, WeightMax: 1,
			Rivals: Rivals{Wobble: 0.3, RepulsorWeight: 0.8, RepulsorReach: 1.6, RepulsorRadius: 0.7},
		},
		Custom: Custom{
			Attractors: []CustomAttractor{},
			Repulsors:  []CustomRepulsor{},
			Rivals:     Rivals{Wobble: 0.3, RepulsorWeight: 0.8, RepulsorReach: 1.6, RepulsorRadius: 0.7},
		},
	}
}

// validate appends the problems with the layout inputs to errs through bad.
func (l *Layout) validate(bad func(format string, args ...any)) {
	if !slices.Contains(Presets, l.Preset) {
		bad("layout.preset %q must be one of %v", l.Preset, Presets)
	}
	if v := l.PoleMargin; !nonNegative(v) || v > 4 {
		bad("layout.pole_margin %v must be in [0, 4]", v)
	}
	for _, p := range []struct {
		name string
		p    *Pattern
	}{
		{PresetPangaea, &l.Pangaea},
		{PresetContinents, &l.Continents},
		{PresetArchipelago, &l.Archipelago},
		{PresetIslands, &l.Islands},
	} {
		p.p.validate("layout."+p.name, bad)
	}
	l.Custom.Rivals.validate("layout.custom.rivals", bad)
	if n := len(l.Custom.Attractors); n > MaxCustomAttractors {
		bad("layout.custom.attractors has %d entries, more than %d", n, MaxCustomAttractors)
	}
	if n := len(l.Custom.Repulsors); n > MaxCustomRepulsors {
		bad("layout.custom.repulsors has %d entries, more than %d", n, MaxCustomRepulsors)
	}
	if l.Preset == PresetCustom && len(l.Custom.Attractors) == 0 {
		bad("layout.preset is custom but layout.custom.attractors is empty")
	}
	for k, a := range l.Custom.Attractors {
		name := fmt.Sprintf("layout.custom.attractors[%d]", k)
		if !finite(a.XKm) || !finite(a.YKm) {
			bad("%s position (%v, %v) km must be finite", name, a.XKm, a.YKm)
		}
		if !positive(a.RadiusKm) {
			bad("%s.radius_km %v must be positive and finite", name, a.RadiusKm)
		}
		if !(a.Weight > 0 && a.Weight <= 1) {
			bad("%s.weight %v must be in (0, 1]", name, a.Weight)
		}
		if a.Mass < 0 {
			bad("%s.mass %d must not be negative", name, a.Mass)
		}
	}
	for k, r := range l.Custom.Repulsors {
		name := fmt.Sprintf("layout.custom.repulsors[%d]", k)
		if !finite(r.XKm) || !finite(r.YKm) {
			bad("%s position (%v, %v) km must be finite", name, r.XKm, r.YKm)
		}
		if !positive(r.RadiusKm) {
			bad("%s.radius_km %v must be positive and finite", name, r.RadiusKm)
		}
		if !(r.Weight > 0 && r.Weight <= 2) {
			bad("%s.weight %v must be in (0, 2]", name, r.Weight)
		}
	}
}

// validate reports the problems with a seeded preset's parameters.
func (p *Pattern) validate(name string, bad func(format string, args ...any)) {
	if p.MassesMin < 1 || p.MassesMax < p.MassesMin || p.MassesMax > MaxLayoutMasses {
		bad("%s.masses_min %d and masses_max %d must satisfy 1 ≤ min ≤ max ≤ %d", name, p.MassesMin, p.MassesMax, MaxLayoutMasses)
	}
	if p.LobesMin < 1 || p.LobesMax < p.LobesMin || p.LobesMax > MaxLayoutLobes {
		bad("%s.lobes_min %d and lobes_max %d must satisfy 1 ≤ min ≤ max ≤ %d", name, p.LobesMin, p.LobesMax, MaxLayoutLobes)
	}
	if v := p.Coverage; !positive(v) || v > 4 {
		bad("%s.coverage %v must be in (0, 4]", name, v)
	}
	if v := p.SizeRatio; !(v >= 1 && v <= 100) {
		bad("%s.size_ratio %v must be in [1, 100]", name, v)
	}
	if v := p.LobeStep; !nonNegative(v) || v > 4 {
		bad("%s.lobe_step %v must be in [0, 4]", name, v)
	}
	if v := p.Spacing; !nonNegative(v) || v > 4 {
		bad("%s.spacing %v must be in [0, 4]", name, v)
	}
	if !(p.WeightMin > 0 && p.WeightMin <= p.WeightMax && p.WeightMax <= 1) {
		bad("%s.weight_min %v and weight_max %v must satisfy 0 < min ≤ max ≤ 1", name, p.WeightMin, p.WeightMax)
	}
	p.Rivals.validate(name+".rivals", bad)
}

// validate reports the problems with a Rivals group.
func (r *Rivals) validate(name string, bad func(format string, args ...any)) {
	if v := r.Wobble; !(v >= 0 && v < 1) {
		bad("%s.wobble %v must be in [0, 1)", name, v)
	}
	if v := r.RepulsorWeight; !(v >= 0 && v <= 2) {
		bad("%s.repulsor_weight %v must be in [0, 2]", name, v)
	}
	if v := r.RepulsorReach; !nonNegative(v) || v > 8 {
		bad("%s.repulsor_reach %v must be in [0, 8]", name, v)
	}
	if v := r.RepulsorRadius; !positive(v) || v > 8 {
		bad("%s.repulsor_radius %v must be in (0, 8]", name, v)
	}
}

// derive normalizes absent custom lists to empty ones and checks the custom
// positions against the resolved world: every attractor must lie in the
// playable band outside the rim and its falloff, and every repulsor outside
// the rim.
func (l *Layout) derive(cyl topo.Cylinder) error {
	if l.Custom.Attractors == nil {
		l.Custom.Attractors = []CustomAttractor{}
	}
	if l.Custom.Repulsors == nil {
		l.Custom.Repulsors = []CustomRepulsor{}
	}
	for k, a := range l.Custom.Attractors {
		if !(a.XKm >= 0 && a.XKm < cyl.W()) {
			return fmt.Errorf("config: layout.custom.attractors[%d].x_km %v must be in [0, %v)", k, a.XKm, cyl.W())
		}
		if !(a.YKm >= 0 && a.YKm <= cyl.H()) || cyl.InRim(a.YKm) || cyl.InFalloff(a.YKm) {
			return fmt.Errorf("config: layout.custom.attractors[%d].y_km %v must lie in the playable band outside the rim and falloff, [%v, %v] km", k, a.YKm, cyl.Rim()+cyl.Falloff(), cyl.H()-cyl.Rim()-cyl.Falloff())
		}
	}
	for k, r := range l.Custom.Repulsors {
		if !(r.XKm >= 0 && r.XKm < cyl.W()) {
			return fmt.Errorf("config: layout.custom.repulsors[%d].x_km %v must be in [0, %v)", k, r.XKm, cyl.W())
		}
		if !(r.YKm >= 0 && r.YKm <= cyl.H()) || cyl.InRim(r.YKm) {
			return fmt.Errorf("config: layout.custom.repulsors[%d].y_km %v must lie outside the rim, in [%v, %v] km", k, r.YKm, cyl.Rim(), cyl.H()-cyl.Rim())
		}
	}
	return nil
}

// finite reports whether v is finite.
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
