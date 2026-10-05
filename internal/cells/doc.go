// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package cells computes per-cell statistics, the sea level, and the ocean.
//
// # Cell statistics
//
// Pipeline stage 5 (DESIGN.md, "Cell statistics and sea level") carries the
// raster over to the mesh. Compute assigns every raster sample to a cell
// and returns Stats: per cell its altitude, relief, latitude, and sample
// count, and per sample its cell (Owner), for later stages that aggregate
// raster fields to cells.
//
//   - Assignment. A sample belongs to the cell whose site is nearest: the
//     smallest dx² + dy², with dx the wrapped east-west displacement (topo
//     DX) and dy unwrapped, each product rounded before the sum (package
//     fmath), so the choice is the same on every architecture. Equally
//     near sites go to the lower cell id. This is the Voronoi cell of the
//     site, not the collapsed polygon of package mesh: the short-edge
//     collapse moves corners by a kilometer or two, and the design defines
//     a cell's samples by its site.
//   - Altitude is the nearest-rank median of the cell's samples, relief the
//     nearest-rank 95th percentile minus the 5th (package field's
//     Percentiles, as hmz2ter computes them). Altitude is the only height
//     the mesh uses; relief is roughness and only sets landforms.
//   - Latitude is topo's latitude proxy 1 − 2·y/H at the cell's site.
//   - Every cell gets statistics, rim cells included.
//   - A cell with no sample (possible in principle, as when two sites
//     coincide, but not seen in practice: at the default 2 km raster a cell
//     holds about 20 samples, 12 to 30 in the default world) takes its
//     altitude from the field at its site by bilinear sampling, and relief
//     0. Stats.Empty counts such cells.
//
// Samples are visited in storage order and each cell's values are gathered
// in that order, so nothing depends on scheduling or map order.
//
// # Nearest-site search
//
// Locator finds the nearest site through a bucket grid over the cylinder:
// nbx × nby buckets about one mean site spacing on a side (bucket width
// bw = W/nbx, height bh = H/nby), wrapping east-west. A query visits rings
// of buckets around its own, ring r being the buckets whose column offset
// (taken the shorter way around) or row offset is r and neither is more,
// and keeps the nearest site seen, by the rule above.
//
// The search is exact, not approximate. After rings 0 to r, any bucket not
// yet visited is at least r + 1 rows away, so its sites are at least r·bh
// away in y, or at least r + 1 columns away the shorter way around, so at
// least r·bw away in x (the point and the site each lie somewhere in their
// own bucket, so r whole buckets separate them). Every unvisited site is
// therefore at least L = r·min(bw, bh) away (r·bh once the ring has wrapped
// all the way around, r·bw once it covers every row). The search stops when
// the best squared distance is below L², with L reduced by 10⁻⁶ km to cover
// a site whose bucket index rounded across a bucket boundary: every
// unvisited site is then strictly farther, so it can neither win nor tie.
// It also stops when the rings have covered every bucket. BruteNearest
// applies the same rule to every site, and the tests check the two agree on
// every raster sample of small worlds.
//
// # Renders
//
// AltitudeRender, the stage render, fills each playable cell with the
// hypsometric color of its altitude (render.Land above sea level,
// render.Water below, as the elevation render colors samples) and the rim
// cells as the ice sheet, over package mesh's CellRender (blended
// outlines, the ice front, cells across the seam drawn on both sides).
// The sea level stage has not run yet, so the stage renders against the
// 0 m datum, which the elevation stage sets at the land fraction. Variant
// "relief", ReliefRender, fills each playable cell by its relief on
// ReliefRamp.
//
// # Determinism
//
// Every non-exact result that reaches Stats is computed with package fmath
// or correctly rounded operations, and TestNoFusedMultiplyAdd checks the
// compiled code. AppendBinary gives the canonical encoding the golden
// hashes cover.
package cells
