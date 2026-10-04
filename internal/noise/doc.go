// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package noise provides pinned periodic noise, domain warps, and ridges.
//
// Everything here is written out in this package rather than taken from a
// dependency, and every product that reaches a sum is rounded through
// package fmath, so the same seed and coordinates give the same bits on every
// architecture (DESIGN.md, "Determinism"). Changing any result is an algorithm
// change and must bump Version, which feeds seed.Derive for every noise stage.
//
// # Noise
//
// Source is 3-D simplex gradient noise (Perlin 2001; Gustavson, "Simplex
// noise demystified", 2005) keyed by two uint64 values, normally the stage
// seed from New or seed.Derive. Gradients are chosen by hashing the lattice
// point with the key (SplitMix64, after wgva's Hash3), so the noise depends
// only on the key and the coordinates: there is no permutation table and no
// shared random source. Noise3 returns a value in [−1, 1]. Stream derives an
// independent key, so each octave and each warp axis has its own lattice.
//
// # Cylinder sampling
//
// The world is a cylinder W km around and H km tall. Cylinder maps a world
// point (x, y) in km onto a circular cylinder in 3-D whose surface distances
// are km:
//
//	theta = 2π·x/W,   R = W/(2π)
//	px = R·cos(theta),  py = R·sin(theta),  pz = H/2 − y = latitude·H/2
//
// where latitude = 1 − 2y/H (topo.Cylinder.Latitude). Noise sampled at
// (px, py, pz) is truly periodic in x with period W: x and x + W give the same
// point, and x is wrapped into [0, W) first so they give the same bits.
//
// The issue's form theta = 2π(x + 0.5)/W is the same mapping in raster units,
// with x a column index and W the column count: a raster sample sits at
// field.X(i) = (i + 0.5)·PitchX km, and PitchX = W_km/NX, so
// 2π·X(i)/W_km = 2π(i + 0.5)/NX.
//
// Feature sizes are wavelengths in km: a wavelength λ samples the noise at
// (px, py, pz)/λ, so one lattice cell spans λ km on the surface in every
// direction.
//
// # Fractals and warp
//
// FBM sums octaves of Noise3, normalized to [−1, 1]. Ridged is a ridged
// multifractal (Musgrave) in [0, 1], high along the zero crossings of the
// noise. Warp displaces a world point by a pair of FBM fields sampled on the
// same cylinder, so the displacement at x and at x + W is identical; the
// displaced x is wrapped back into [0, W). The displaced y is not clamped:
// a point pushed past a pole extrapolates the latitude (pz keeps growing),
// which is still a valid noise point. The rim and its falloff are ocean, so
// the values found there do not matter.
//
// Parameter structs carry defaults (DefaultFBM, DefaultRidged, DefaultWarp);
// they are not yet part of config.json.
package noise
