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
// # Sea level and ocean
//
// Pipeline stage 6 (DESIGN.md, "Cell statistics and sea level") chooses the
// sea level on the cells' altitudes. Classify gives the land and water at
// one level:
//
//   - A playable (non-rim) cell is a water candidate when its altitude is at
//     or below the level, a land candidate when strictly above it.
//   - The ocean is a flood from every rim cell (rim cells count as deep salt
//     water) over cell adjacency, mesh Cell.Neighbors, through the water
//     candidates. Cells that touch only at a point, as the two cells across
//     a collapsed short edge do at its 4-way corner, are not neighbors, so
//     water that meets only at a corner is not connected.
//   - Water candidates the flood does not reach are basin floors. The basin
//     stage (8) decides them; until it exists each is a dry basin and counts
//     as land. So land is every playable cell that is not ocean; rim cells
//     are never land.
//
// Search chooses the level for the land target N (world.land_cells):
//
//   - Candidate levels are the distinct playable altitudes, ascending,
//     a₀ < a₁ < … < a_{k−1}, plus one below them all. The level is always
//     one of them: a cell altitude aⱼ, so the cells at or below aⱼ are the
//     water candidates, or for "all land" the largest float64 below a₀
//     (math.Nextafter). Either way it is finite and exact, with no
//     arithmetic.
//   - The first probe is the quantile estimate: the lowest candidate that
//     leaves at most N playable cells above it (N + expected lake cells,
//     and there are no lakes yet). It is high on land by the dry basins.
//   - Then a bracketed search over the candidate indexes, keeping the open
//     bracket between the highest level measured with too much land and the
//     lowest with too little (initially all of them). The next probe is a
//     rank step: the candidate whose count of cells at or below it differs
//     from the last probe's by the last probe's excess (or shortfall) of
//     land, as if every cell that changed were a land candidate. When the
//     rank step falls outside the bracket, or the last rank step failed to
//     halve the deviation (a slow approach or a jump), the next probe is
//     instead the middle of the bracket ("bisect"), or, while no level has
//     been measured on the side where the target lies, so that the bracket
//     is still open there, a rank step multiplied by 2, 4, 8, … ("gallop",
//     kept inside the bracket). The search ends at a probe
//     with exactly N, when the bracket closes (no untried candidate lies
//     between too much and too little land), or after SearchBudget probes.
//   - The result is the best probe measured: the least |land − N|, ties to
//     the lower level (more land). Nothing assumes the land count falls as
//     the level rises: a basin floor joining the ocean drops the count by
//     its whole size, and once the basin stage runs inside the search
//     basins can flip either way. A non-monotonic count only slows the
//     bracket; the best measured result is still kept.
//   - The land contract is met when 100·|land − N| ≤ TolerancePercent·N
//     (1% of N). The reason saved is "exact", "within-tolerance" (met, not
//     exact), "unreachable" (the bracket closed outside tolerance: the count
//     jumps across the band between adjacent levels), or "budget-exhausted".
//     An unmet target is reported, not an error.
//
// SeaLevel saves the policy, budget, initial estimate, chosen level, the
// land, ocean and dry basin cells and counts, the measured land area (the
// land cells' areas summed in id order), every probe, and the reason.
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
// ReliefRamp. SeaLevelRender, the sea level stage render, colors land by
// its height above the chosen level, ocean by its depth below it, dry basin
// floors in BasinColor, and the rim as ice.
//
// # Determinism
//
// Every non-exact result that reaches Stats is computed with package fmath
// or correctly rounded operations, and TestNoFusedMultiplyAdd checks the
// compiled code. The sea-level search compares altitudes and counts cells
// only; its one sum, the land area, adds package mesh's areas in cell id
// order. Stats.AppendBinary and SeaLevel.AppendBinary give the canonical
// encodings the golden hashes cover.
package cells
