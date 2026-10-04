// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package field provides the raster: a grid of float64 samples laid over the
// world cylinder, with wrap-aware lookup, bilinear sampling, and the
// min/max/percentile helpers the cell statistics use. Stage renders are drawn
// by package render.
//
// # Grid
//
// A Field built for a cylinder W × H km at spacing s has
//
//	NX = max(2, round(W/s)) columns, pitch PX = W/NX,
//	NY = max(2, round(H/s)) rows,    pitch PY = H/NY,
//
// so the actual pitches are close to s (exactly s when it divides W or H) and
// the world size need not be a multiple of s. The raster covers the full height,
// rims included.
//
// Samples sit at cell centers: sample (i, j) is at
//
//	X(i) = (i + 0.5)·PX,  Y(j) = (j + 0.5)·PY,
//
// each a single rounded product. The NX columns tile the circumference
// exactly once: column NX−1 sits half a pitch west of the seam and column 0
// half a pitch east of it, so there is no duplicate seam column, and column
// NX−1's east neighbor is column 0. NX·PX equals W to within one ulp of W;
// the seam itself is always W, taken from the cylinder.
//
// Values are stored row-major, north to south and west to east within a row:
// index j·NX + i.
//
// # Lookup
//
// At and Set take integer indices. The column wraps (fmath.FloorMod), so
// At(-1, j) is At(NX-1, j). The row does not wrap: a row outside [0, NY)
// panics.
//
// # Sampling
//
// Sample(x, y) interpolates bilinearly between the four samples around a
// point. x is wrapped into [0, W) by the cylinder. y is clamped to
// [Y(0), Y(NY−1)], so between a pole and the first or last row the field is
// constant along y. The column and row are the largest index whose sample
// coordinate is ≤ the wrapped (or clamped) coordinate; a point west of X(0)
// uses column −1, which is column NX−1 one circumference west, so sampling
// interpolates across the seam between columns NX−1 and 0. The weights are
//
//	tx = (x − X(i0))/PX,  ty = (y − Y(j0))/PY,  each clamped to at most 1,
//
// interpolated first along x on both rows and then along y, each step as
// a + t·(b − a) with the product rounded before the sum. A zero weight
// returns a exactly, so sampling at a grid point returns the stored value.
//
// # Values
//
// Every stored value is finite: New fills with 0, and Set, Fill, and SetFunc
// panic on NaN or ±Inf. Values should stay far enough inside the float64
// range that differences between neighbors are finite.
//
// # Percentiles
//
// Percentile uses the nearest-rank method, as hmz2ter does: for n values
// sorted ascending, the p-th percentile (0 ≤ p ≤ 100) is the value of rank
// max(1, ⌈p·n/100⌉). p = 0 is the minimum, p = 100 the maximum, and the
// median of an even count is the lower middle value. The result is always
// one of the inputs. Inputs are copied, never reordered.
//
// Every non-exact result is computed with package fmath or plain correctly
// rounded operations, so it has the same bits on every architecture.
package field
