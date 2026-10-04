// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package elevation synthesizes the bedrock heightmap (DESIGN.md, pipeline
// stage 3): the layout's continental bias, fBm, a periodic domain warp, and
// ridged mountain chains combined into meters, then the polar falloff to deep
// ocean. Volcanic hotspots (S14) will add their cones and swells just before
// the falloff. This is the one height (DESIGN.md, "Cell statistics and sea
// level"): later stages take each cell's median of it.
//
// Sea level is not decided here; stage 6 searches for it on cell counts. The
// field is built so that 0 m is a good first guess: about the land fraction
// of the raster outside the rim lies above 0 m (see "Datum"), and 0 m is the
// layout's nominal coast, moved by the noise.
//
// # Signal
//
// Each sample (x, y) is first displaced by the domain warp (noise.Warp, keyed
// by the "warp" stream), to (wx, wy); everything below is read at the warped
// point, whose noise point is q = noise.Cylinder.Point(wx, wy). With b the
// layout bias at (wx, wy) (bilinear, field.Sample), the dimensionless
// continental signal is
//
//	b' = b·((1 − k) + k·|b|)                k = bias_flatten
//	c  = b' + amplitude·fbm(q) − pull(wy, q)
//
// where fbm is the continental ladder (keyed by the "elevation" stream). The
// flattening keeps the bias's sign but makes it shallow near its zero, so the
// noise draws the coasts there (bays, peninsulas, offshore islands) while the
// bias keeps the cores of the continents and the open ocean.
//
// The pull lowers the signal toward the poles. With u the distance from the
// rim's inner edge (topo.Cylinder.RimDistance) at wy, moved by up to
// jitter_km along a coarse fBm (on the warp source), F the falloff band's
// depth, and P = pull_km, it is 0 for u ≥ F + P, rises as a smoothstep to
// pull at u = F, and stays there. Land so thins out toward the poles with
// coasts as ragged as any other, before the ceiling below applies. In a
// small world, where pull_km + jitter_km is more than a quarter of the half
// height of the playable band outside the falloff, both are scaled down
// together to that quarter, so the pull cannot swallow the band.
//
// # Datum
//
// The signal is then shifted and rescaled,
//
//	c ← (c + δ·(1 + b)/2) / (1 + max(δ, 0))
//
// where δ in [−datum_max_shift, datum_max_shift] is found by bisection (48
// steps) so that the land fraction of the samples outside the rim have
// c > 0. The weight (1 + b)/2 is 0 in the open ocean (b = −1), so the shift
// grows or shrinks the layout's landmasses instead of raising the sea floor,
// and the division keeps a raised continent's heights in range without
// moving any coast. A layout whose land is out of reach within the bound
// keeps the clamped shift, and Stats reports it. The search counts samples
// in a fixed order, so the shift does not depend on scheduling.
//
// # Meters
//
// Below zero the signal becomes the sea floor, a shallow shelf, a continental
// slope, and an abyssal plain:
//
//	h = −ocean_depth_m · smoothstep(−c)
//
// Above zero it is land, the lowland and hill relief plus the ridges:
//
//	h = relief_scale_m·c + height_m · r² · smoothstep(c / land_ramp) · belt
//	r    = max(0, (ridged(q) − threshold) / (1 − threshold))
//	belt = smoothstep((fbm_belt(q) − belt_threshold)/0.25 + 0.5)
//
// where ridged is the ridged multifractal (noise.Ridged) and fbm_belt a
// 3-octave fBm of wavelength belt_wavelength_km, both keyed by the "ridges"
// stream. The ridges fade in from the coast, so they never raise sea floor
// into land, and run only along the belts, so the land between keeps its
// plains. Every term is non-negative on land and non-positive at sea, so h
// has the sign of c: the coast at 0 m is the coast of the signal.
//
// smoothstep(t) = 3t² − 2t³ for t clamped to [0, 1].
//
// # Falloff
//
// With C = falloff.ceiling_m (< 0), D = falloff.depth_m (≤ C), T =
// falloff.taper_km and u the true (unwarped) distance from the rim's edge:
//
//   - for F ≤ u < F + T, a soft ceiling: h ← h − s·(h − C) where h > C, with
//     s a smoothstep from 0 at u = F + T to 1 at u = F;
//   - for u < F (the falloff band and the rim), h ← min(h', L(u)), where h'
//     is h under the full ceiling (min(h, C)) and L runs from C at u = F to D
//     at the rim's edge as a smoothstep, and is D in the rim.
//
// Both steps are continuous in u, so there is no step at the band's edge,
// and together they guarantee the falloff band and the rim never rise above
// C, and the rim never above D. C defaults to −1000 m, far below any sea
// level the search will choose (the datum puts it near 0 m), so no land
// survives in the falloff or the rim, whatever the noise does. Rim cells are
// deep salt water in the generator (DESIGN.md, "Rim").
//
// # Seeds
//
// The noise sources are noise.New(world, "elevation"), noise.New(world,
// "warp") and noise.New(world, "ridges") (DESIGN.md's stage names), each
// keyed with noise.Version. Version, "elevation/1", names this formula; any
// change to the field's bits must bump it and re-record the golden hashes.
//
// # Determinism
//
// Every product that reaches a sum is rounded through package fmath, and the
// noise is pinned (package noise). Rows are computed concurrently, but each
// sample depends only on its coordinates and the shift, so the field has the
// same bits on every machine and at every GOMAXPROCS.
//
// # Render
//
// Render draws the hypsometric map (render.Land, render.Water) split at a sea
// level, with a northwest hillshade exaggerated by ZFactor; the stage render
// splits at 0 m.
package elevation
