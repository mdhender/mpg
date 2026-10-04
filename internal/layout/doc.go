// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package layout places continental attractors and repulsors and holds the
// layout presets. It builds the continental bias field, the first input to
// elevation (DESIGN.md, "Layout"): a bias, not a mask. Noise still makes
// the coastlines.
//
// # Presets
//
// The seeded presets (config.Pattern) place a number of landmasses
// ("masses"), each a cluster of overlapping attractors ("lobes"):
//
//   - pangaea: one dominant mass of many lobes;
//   - continents: 3–5 masses of a few lobes each;
//   - archipelago: many mid-size masses of one to three lobes;
//   - islands: many small masses of one or two lobes (wgvc style).
//
// custom takes an explicit attractor list, grouped into masses, and optional
// explicit repulsors (config.Custom).
//
// # Placement
//
// Placement draws from seed.Rand(world, "layout", Version) in a fixed order.
// The mass count is drawn from [masses_min, masses_max]. Each mass draws a
// share uniformly from [1, size_ratio]; the shares are normalized and the
// masses placed largest first (stable by draw order). Mass m covers a
// nominal area coverage × N × A × share, split across its lobe count n,
// drawn from [lobes_min, lobes_max], so all its lobes have one radius
//
//	r = sqrt(area / (π·(1 + (n − 1)·min(1, 0.61·lobe_step))))
//
// where 0.61 is the share of a disc that a second disc one radius away adds.
// A lobe's center must lie in the playable band outside the rim and its
// falloff, inset by pole_margin × r (r is capped so it fits). The first lobe
// is uniform over that band; each further lobe lies lobe_step × r from an
// earlier one, the most open (farthest from the lobes so far) of eight
// random candidates. A candidate mass is accepted when every lobe is at least
// spacing × (r_i + r_j) from every lobe of the masses already placed
// (wrapped distance). After 64 failed candidates the spacing is multiplied
// by 0.9 and kept for the rest of the run, at most 40 times; the achieved
// spacing and the relaxation count are reported.
//
// # Repulsors
//
// For each pair of rival masses (a < b, in mass order), the closest pair of
// their attractors (ties to the lower indices) gets a repulsor when the two
// are nearer than repulsor_reach × (r_i + r_j). It sits on the line between
// them in the middle of the gap between their discs, with radius
// repulsor_radius × (r_i + r_j)/2 and weight repulsor_weight. Explicit custom
// repulsors come first. The gaps they hold open become straits.
//
// # Bias field
//
// The bias is dimensionless, in [−1, 1]: positive is continental, negative
// oceanic, 0 the nominal coast. Far from every attractor it is −1. With
// wrapped distance d from an attractor of radius r and weight w, and the
// wobble factor s = 1 + wobble × n, where n in [−1, 1] is a 3-octave fBm of
// wavelength twice the mean attractor radius sampled on the cylinder (noise
// seeded by noise.New(world, "layout")), let t = d/(r·s) and T² = 2 + √2:
//
//	k(t) = 2·(1 − t²/T²)² − 1  for t < T, else −1
//
// so k(0) = 1, k(1) = 0, and k(T) = −1 with zero slope. Positive values are
// scaled by w. The attractor term is the maximum of k over all attractors.
// Each repulsor of radius ρ and weight v then subtracts v·(1 − (d/ρ)²)² for
// d < ρ, in repulsor order, and the result is clamped below at −1. The field
// is periodic east–west, since every distance and the noise wrap.
//
// The bias does not apply the rim falloff; elevation does. Attractor centers
// are kept out of the rim and falloff, so the positive bias stays near the
// playable band.
//
// # Determinism
//
// Every product that reaches a sum is rounded through package fmath, sines
// and cosines come from fmath, and every loop runs in index order, so the
// layout and the field have the same bits on every architecture.
package layout
