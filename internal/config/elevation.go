// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package config

// Elevation sets the bedrock heightmap (DESIGN.md, pipeline stage 3): the
// layout's bias plus continental fBm, read through a periodic domain warp,
// turned into meters, with ridged mountain chains on land and the polar
// falloff to deep ocean. See package elevation for the formula.
type Elevation struct {
	// ReliefScaleM is the land height, in meters, per unit of continental
	// signal above zero (the signal is the bias plus the fBm, about 1 in a
	// continent's interior).
	ReliefScaleM float64 `json:"relief_scale_m"`
	// OceanDepthM is the sea-floor depth, in meters, per unit of continental
	// signal below zero (about −1 in the open ocean).
	OceanDepthM float64 `json:"ocean_depth_m"`
	// DatumMaxShift bounds the datum shift δ: the continental signal at a
	// sample of bias b is raised by δ·(1 + b)/2, with δ chosen so the land
	// fraction of the samples outside the rim lies above 0 m. That puts the
	// coasts at 0 m about where the land target wants them, so the
	// sea-level search moves little, and it grows or shrinks the layout's
	// landmasses rather than raising the open ocean (b = −1). δ is clamped
	// to ±DatumMaxShift, so a layout that asks for far too much or too
	// little land is not reshaped; 0 disables the shift.
	DatumMaxShift float64 `json:"datum_max_shift"`
	// BiasFlatten, in [0, 1], flattens the layout bias b near its zero
	// (the nominal coast) before the noise is added: the bias used is
	// b·((1 − k) + k·|b|) with k = BiasFlatten, so at 1 it is b·|b|. Near
	// the coast the noise then draws the coastline (bays, peninsulas,
	// offshore islands), while the continents' cores and the open ocean keep
	// the layout's intent.
	BiasFlatten float64 `json:"bias_flatten"`
	// Continental is the fBm added to the layout bias: it makes the
	// coastlines (bays, peninsulas, offshore islands) and the rolling
	// lowland and hill relief.
	Continental Noise `json:"continental"`
	// Warp displaces every sample before the bias and the noise are read,
	// so coasts and ranges bend.
	Warp Warp `json:"warp"`
	// Ridges sets the ridged mountain chains.
	Ridges Ridges `json:"ridges"`
	// Falloff sets how the polar falloff band pulls the bedrock down to deep
	// ocean.
	Falloff Falloff `json:"falloff"`
}

// Noise is an fBm octave ladder and the amplitude it is added with.
type Noise struct {
	// WavelengthKm is the first (coarsest) octave's wavelength.
	WavelengthKm float64 `json:"wavelength_km"`
	// Octaves is the number of octaves, in [1, 16].
	Octaves int `json:"octaves"`
	// Lacunarity is the frequency ratio between octaves, in (1, 16].
	Lacunarity float64 `json:"lacunarity"`
	// Gain is the amplitude ratio between octaves, in (0, 1).
	Gain float64 `json:"gain"`
	// Amplitude scales the fBm, which lies in [−1, 1], before it is added.
	Amplitude float64 `json:"amplitude"`
}

// Warp is the periodic domain warp.
type Warp struct {
	// StrengthKm is the largest displacement on each axis; 0 disables the
	// warp.
	StrengthKm float64 `json:"strength_km"`
	// WavelengthKm, Octaves, Lacunarity and Gain are the displacement
	// fields' fBm ladder.
	WavelengthKm float64 `json:"wavelength_km"`
	Octaves      int     `json:"octaves"`
	Lacunarity   float64 `json:"lacunarity"`
	Gain         float64 `json:"gain"`
}

// Ridges sets the ridged-multifractal mountain chains.
type Ridges struct {
	// HeightM is the height a full ridge adds, in meters.
	HeightM float64 `json:"height_m"`
	// WavelengthKm, Octaves, Lacunarity and Gain are the ridged ladder;
	// Weight is its detail feedback from one octave to the next.
	WavelengthKm float64 `json:"wavelength_km"`
	Octaves      int     `json:"octaves"`
	Lacunarity   float64 `json:"lacunarity"`
	Gain         float64 `json:"gain"`
	Weight       float64 `json:"weight"`
	// Threshold, in [0, 1), is the ridged value below which nothing is
	// added; above it the ridge rises as the square of the excess, to
	// HeightM at 1. It keeps the low ground between chains free of
	// mountains.
	Threshold float64 `json:"threshold"`
	// LandRamp is the continental signal over which ridges fade in from the
	// coast (signal 0, no ridges) to full height, so ridges stay on land.
	LandRamp float64 `json:"land_ramp"`
	// BeltWavelengthKm is the wavelength of the smooth belt noise (an fBm
	// in [−1, 1]) that decides where mountain belts run. Ridges reach full
	// height where it is above BeltThreshold + 0.125 and vanish below
	// BeltThreshold − 0.125, so a higher threshold gives fewer, narrower
	// belts; −1.2 or less puts ridges everywhere on land.
	BeltWavelengthKm float64 `json:"belt_wavelength_km"`
	BeltThreshold    float64 `json:"belt_threshold"`
}

// Falloff sets how the falloff band pulls the bedrock down to deep ocean.
type Falloff struct {
	// CeilingM is the highest elevation allowed anywhere in the falloff band
	// and the rim, in meters; it must be negative and lie well below any
	// plausible sea level, so no land survives there.
	CeilingM float64 `json:"ceiling_m"`
	// DepthM is the elevation the falloff deepens to at the rim's edge, in
	// meters, and the most the rim may reach; at most CeilingM.
	DepthM float64 `json:"depth_m"`
	// Pull is how far the falloff lowers the continental signal at the band's
	// inner edge and beyond; it eases in over PullKm km inside that edge,
	// read through the domain warp, so land thins out toward the poles with
	// natural coasts before the ceiling applies.
	Pull   float64 `json:"pull"`
	PullKm float64 `json:"pull_km"`
	// JitterKm moves the pull's start nearer to or farther from the pole
	// by up to this many km, along a 3-octave fBm whose first wavelength is
	// JitterWavelengthKm, so polar coasts do not follow a parallel.
	JitterKm           float64 `json:"jitter_km"`
	JitterWavelengthKm float64 `json:"jitter_wavelength_km"`
	// TaperKm is the strip inside the falloff band's inner edge over which
	// the ceiling eases in, so the bedrock meets it without a step.
	TaperKm float64 `json:"taper_km"`
}

// DefaultElevation returns the default elevation inputs.
func DefaultElevation() Elevation {
	return Elevation{
		ReliefScaleM:  500,
		OceanDepthM:   4000,
		DatumMaxShift: 2,
		BiasFlatten:   0.6,
		Continental: Noise{
			WavelengthKm: 256, Octaves: 6, Lacunarity: 2, Gain: 0.6, Amplitude: 1.3,
		},
		Warp: Warp{
			StrengthKm: 40, WavelengthKm: 400, Octaves: 2, Lacunarity: 2, Gain: 0.5,
		},
		Ridges: Ridges{
			HeightM: 3500, WavelengthKm: 300, Octaves: 5, Lacunarity: 2, Gain: 0.5, Weight: 2,
			Threshold: 0.35, LandRamp: 0.35, BeltWavelengthKm: 700, BeltThreshold: 0,
		},
		Falloff: Falloff{CeilingM: -1000, DepthM: -4000, Pull: 1.5, PullKm: 100, JitterKm: 100, JitterWavelengthKm: 300, TaperKm: 30},
	}
}

// validate appends the problems with the elevation inputs through bad.
func (e *Elevation) validate(bad func(format string, args ...any)) {
	if v := e.ReliefScaleM; !positive(v) || v > 20_000 {
		bad("elevation.relief_scale_m %v must be in (0, 20000]", v)
	}
	if v := e.OceanDepthM; !positive(v) || v > 20_000 {
		bad("elevation.ocean_depth_m %v must be in (0, 20000]", v)
	}
	if v := e.DatumMaxShift; !nonNegative(v) || v > 2 {
		bad("elevation.datum_max_shift %v must be in [0, 2]", v)
	}
	if v := e.BiasFlatten; !(v >= 0 && v <= 1) {
		bad("elevation.bias_flatten %v must be in [0, 1]", v)
	}
	e.Continental.validate("elevation.continental", bad)
	if v := e.Continental.Amplitude; !nonNegative(v) || v > 4 {
		bad("elevation.continental.amplitude %v must be in [0, 4]", v)
	}
	w := e.Warp
	if v := w.StrengthKm; !nonNegative(v) || v > 10_000 {
		bad("elevation.warp.strength_km %v must be in [0, 10000]", v)
	}
	ladder("elevation.warp", w.WavelengthKm, w.Octaves, w.Lacunarity, w.Gain, bad)
	r := e.Ridges
	if v := r.HeightM; !nonNegative(v) || v > 20_000 {
		bad("elevation.ridges.height_m %v must be in [0, 20000]", v)
	}
	ladder("elevation.ridges", r.WavelengthKm, r.Octaves, r.Lacunarity, r.Gain, bad)
	if v := r.Weight; !nonNegative(v) || v > 16 {
		bad("elevation.ridges.weight %v must be in [0, 16]", v)
	}
	if v := r.Threshold; !(v >= 0 && v < 1) {
		bad("elevation.ridges.threshold %v must be in [0, 1)", v)
	}
	if v := r.LandRamp; !positive(v) || v > 4 {
		bad("elevation.ridges.land_ramp %v must be in (0, 4]", v)
	}
	if v := r.BeltWavelengthKm; !positive(v) || v > 1e6 {
		bad("elevation.ridges.belt_wavelength_km %v must be in (0, 1e6]", v)
	}
	if v := r.BeltThreshold; !(v >= -2 && v <= 2) {
		bad("elevation.ridges.belt_threshold %v must be in [−2, 2]", v)
	}
	f := e.Falloff
	if v := f.CeilingM; !(v < 0 && v >= -20_000) {
		bad("elevation.falloff.ceiling_m %v must be in [−20000, 0)", v)
	}
	if v := f.DepthM; !(v <= f.CeilingM && v >= -20_000) {
		bad("elevation.falloff.depth_m %v must be in [−20000, ceiling_m %v]", v, f.CeilingM)
	}
	if v := f.Pull; !nonNegative(v) || v > 16 {
		bad("elevation.falloff.pull %v must be in [0, 16]", v)
	}
	if v := f.PullKm; !positive(v) || v > 10_000 {
		bad("elevation.falloff.pull_km %v must be in (0, 10000]", v)
	}
	if v := f.JitterKm; !nonNegative(v) || v > 10_000 {
		bad("elevation.falloff.jitter_km %v must be in [0, 10000]", v)
	}
	if v := f.JitterWavelengthKm; !positive(v) || v > 1e6 {
		bad("elevation.falloff.jitter_wavelength_km %v must be in (0, 1e6]", v)
	}
	if v := f.TaperKm; !nonNegative(v) || v > 10_000 {
		bad("elevation.falloff.taper_km %v must be in [0, 10000]", v)
	}
}

// validate reports the problems with a noise ladder.
func (n *Noise) validate(name string, bad func(format string, args ...any)) {
	ladder(name, n.WavelengthKm, n.Octaves, n.Lacunarity, n.Gain, bad)
}

// ladder reports the problems with an octave ladder.
func ladder(name string, wavelength float64, octaves int, lacunarity, gain float64, bad func(format string, args ...any)) {
	if !positive(wavelength) || wavelength > 1e6 {
		bad("%s.wavelength_km %v must be in (0, 1e6]", name, wavelength)
	}
	if octaves < 1 || octaves > 16 {
		bad("%s.octaves %d must be in [1, 16]", name, octaves)
	}
	if !(lacunarity > 1 && lacunarity <= 16) {
		bad("%s.lacunarity %v must be in (1, 16]", name, lacunarity)
	}
	if !(gain > 0 && gain < 1) {
		bad("%s.gain %v must be in (0, 1)", name, gain)
	}
}
