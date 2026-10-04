// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package noise

import (
	"fmt"
	"math"

	"github.com/mdhender/mpg/internal/fmath"
)

// MaxOctaves bounds an octave ladder, and MaxRatio its lacunarity and gain,
// so no frequency or amplitude can overflow.
const (
	MaxOctaves = 16
	MaxRatio   = 16
)

// FBM is fractal Brownian motion: Octaves layers of Noise3, coarse to fine.
// Octave k has wavelength WavelengthKm/Lacunarityᵏ and amplitude Gainᵏ, and
// draws on its own stream of the source, s.Stream(k).
type FBM struct {
	Octaves      int     // 1..MaxOctaves
	Lacunarity   float64 // frequency ratio between octaves, in (0, MaxRatio]
	Gain         float64 // amplitude ratio between octaves, in (0, MaxRatio]
	WavelengthKm float64 // wavelength of the first octave, > 0
}

// DefaultFBM returns a continent-scale ladder: 6 octaves from 512 km down,
// lacunarity 2, gain 0.5.
func DefaultFBM() FBM {
	return FBM{Octaves: 6, Lacunarity: 2, Gain: 0.5, WavelengthKm: 512}
}

// Validate reports why p cannot be evaluated, or nil.
func (p FBM) Validate() error {
	return validateLadder("fbm", p.Octaves, p.Lacunarity, p.Gain, p.WavelengthKm)
}

// Sample returns the fBm at q in [−1, 1]: the amplitude-weighted sum of the
// octaves divided by the sum of the amplitudes. Octaves accumulate in order.
// It panics if p is not valid.
func (p FBM) Sample(s Source, q Point3) float64 {
	mustValid(p.Validate())
	var sum, norm float64
	freq, amp := 1/p.WavelengthKm, 1.0
	for k := range p.Octaves {
		v := s.Stream(uint64(k)).Noise3(fmath.Mul(q.X, freq), fmath.Mul(q.Y, freq), fmath.Mul(q.Z, freq))
		sum += fmath.Mul(amp, v)
		norm += amp
		freq = fmath.Mul(freq, p.Lacunarity)
		amp = fmath.Mul(amp, p.Gain)
	}
	return clampSigned(sum / norm)
}

// Ridged is a ridged multifractal (Musgrave): each octave's signal is
// (1 − |n|)², sharp ridges along the zero crossings of the noise, and each
// octave after the first is weighted by the previous signal times Weight
// (clamped to [0, 1]), so detail gathers on the ridges and the valleys stay
// smooth.
type Ridged struct {
	Octaves      int     // 1..MaxOctaves
	Lacunarity   float64 // frequency ratio between octaves, in (0, MaxRatio]
	Gain         float64 // amplitude ratio between octaves, in (0, MaxRatio]
	WavelengthKm float64 // wavelength of the first octave, > 0
	Weight       float64 // detail feedback from one octave to the next, >= 0
}

// DefaultRidged returns a mountain-belt ladder: 5 octaves from 256 km down,
// lacunarity 2, gain 0.5, weight 2.
func DefaultRidged() Ridged {
	return Ridged{Octaves: 5, Lacunarity: 2, Gain: 0.5, WavelengthKm: 256, Weight: 2}
}

// Validate reports why p cannot be evaluated, or nil.
func (p Ridged) Validate() error {
	if err := validateLadder("ridged", p.Octaves, p.Lacunarity, p.Gain, p.WavelengthKm); err != nil {
		return err
	}
	if !(p.Weight >= 0) || math.IsInf(p.Weight, 1) {
		return fmt.Errorf("noise: ridged weight %v must be non-negative and finite", p.Weight)
	}
	return nil
}

// Sample returns the ridged multifractal at q in [0, 1]: the weighted signals
// divided by the sum of the amplitudes. Every signal and weight is in [0, 1],
// so the bound holds. It panics if p is not valid.
func (p Ridged) Sample(s Source, q Point3) float64 {
	mustValid(p.Validate())
	var sum, norm float64
	freq, amp, weight := 1/p.WavelengthKm, 1.0, 1.0
	for k := range p.Octaves {
		n := s.Stream(uint64(k)).Noise3(fmath.Mul(q.X, freq), fmath.Mul(q.Y, freq), fmath.Mul(q.Z, freq))
		r := 1 - math.Abs(n)
		signal := fmath.Mul(fmath.Mul(r, r), weight)
		sum += fmath.Mul(amp, signal)
		norm += amp
		weight = min(max(fmath.Mul(signal, p.Weight), 0), 1)
		freq = fmath.Mul(freq, p.Lacunarity)
		amp = fmath.Mul(amp, p.Gain)
	}
	return min(max(sum/norm, 0), 1)
}

// Warp is a periodic domain warp: a world point is displaced by StrengthKm
// times a pair of FBM fields, one per axis, sampled on the cylinder.
type Warp struct {
	StrengthKm float64 // largest displacement on each axis, >= 0
	FBM        FBM     // the displacement fields
}

// Warp axis streams.
const (
	streamWarpX uint64 = 0x7761727058 // "warpX"
	streamWarpY uint64 = 0x7761727059 // "warpY"
)

// DefaultWarp returns a warp of up to 96 km driven by a 4-octave ladder from
// 384 km down.
func DefaultWarp() Warp {
	return Warp{StrengthKm: 96, FBM: FBM{Octaves: 4, Lacunarity: 2, Gain: 0.5, WavelengthKm: 384}}
}

// Validate reports why w cannot be evaluated, or nil.
func (w Warp) Validate() error {
	if !(w.StrengthKm >= 0) || math.IsInf(w.StrengthKm, 1) {
		return fmt.Errorf("noise: warp strength %v km must be non-negative and finite", w.StrengthKm)
	}
	return w.FBM.Validate()
}

// Apply returns (x, y) displaced by the warp. The displacement is computed at
// m.Point(x, y), so it is the same at x and x ± W. The warped x is wrapped
// into [0, W); the warped y is not clamped and may lie beyond a pole (see the
// package documentation). It panics if w is not valid.
func (w Warp) Apply(s Source, m Cylinder, x, y float64) (wx, wy float64) {
	mustValid(w.Validate())
	q := m.Point(x, y)
	dx := fmath.Mul(w.StrengthKm, w.FBM.Sample(s.Stream(streamWarpX), q))
	dy := fmath.Mul(w.StrengthKm, w.FBM.Sample(s.Stream(streamWarpY), q))
	return m.topo.WrapX(m.topo.WrapX(x) + dx), y + dy
}

func validateLadder(kind string, octaves int, lacunarity, gain, wavelength float64) error {
	switch {
	case octaves < 1 || octaves > MaxOctaves:
		return fmt.Errorf("noise: %s octaves %d outside [1, %d]", kind, octaves, MaxOctaves)
	case !(lacunarity > 0 && lacunarity <= MaxRatio):
		return fmt.Errorf("noise: %s lacunarity %v outside (0, %v]", kind, lacunarity, float64(MaxRatio))
	case !(gain > 0 && gain <= MaxRatio):
		return fmt.Errorf("noise: %s gain %v outside (0, %v]", kind, gain, float64(MaxRatio))
	case !(wavelength > 0) || math.IsInf(wavelength, 1):
		return fmt.Errorf("noise: %s wavelength %v km must be positive and finite", kind, wavelength)
	}
	return nil
}

func mustValid(err error) {
	if err != nil {
		panic(err.Error())
	}
}

func clampSigned(v float64) float64 { return min(max(v, -1), 1) }
