// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package basin finds cell-graph basins, balances water, and forms lakes.
//
// # Basin hierarchy
//
// Pipeline stage 8 (DESIGN.md, "Basins and lakes") starts with the
// depressions of the cell graph: connected sets of cells that water cannot
// leave without rising above every one of them. Find builds them, nested,
// from the cells' altitudes (cells.Stats.Altitude, the one height) and the
// seed cells, the sea: every rim cell and every ocean cell of the sea level
// stage's flood (cells.Flood). Altitudes are read, never modified.
//
// # The flood
//
// The flood is a priority flood in which the water rises everywhere at
// once, from the sea up. The sea is flooded from the start, whatever its
// cells' altitudes. Every other cell is taken from a priority order by
// altitude, ties to the lower cell id (the order of wgvc's cornerHeap,
// held here as one sorted slice since every cell is queued at the start),
// and joins, through a union-find over cells, the flooded components it
// touches (mesh Cell.Neighbors: cells that meet only at a collapsed 4-way
// corner are not neighbors). A classic priority flood from the sea alone
// (Barnes et al. 2014) gives each cell its spill level but fills every
// depression to it in one piece, so it cannot see the depressions nested
// inside; flooding everywhere at once is the merge tree of the terrain
// (Barnes et al. 2020, "depression hierarchy"), and the tests check that
// its outermost spill levels agree with a classic priority flood.
//
// Cells of equal altitude are taken together, a group at a time, and a
// group falls into flats: its cells joined through each other. A flat
// behaves as one cell, so a plateau is never split by cell ids, and every
// component already flooded is strictly below it. For each flat, in the
// order of its lowest cell id, with K the flooded components it touches:
//
//   - K empty: the flat is a local minimum, the floor of a new depression.
//     Its bottom cell is the flat's lowest id.
//   - K holds the sea: every depression in K closes at the flat and spills
//     to the sea; it is top-level. The flat joins the sea: it drains
//     freely.
//   - K is one depression: the flat raises it; its cells join it.
//   - K is two or more depressions: each closes at the flat and spills into
//     a new depression, their parent, which holds them all, starts with the
//     flat as its own cells, and takes its bottom from the lowest child
//     (altitude, then cell id). Deeper water first fills a child, overflows
//     at the pass, and fills the parent as one.
//
// A depression that closes at a flat at altitude a has:
//
//   - spill level a, and depth a − (its bottom's altitude), always
//     positive, since its cells were all flooded before the flat;
//   - spill edge, corner and cell: of the edges between a cell of the flat
//     and a cell of the depression (its nested depressions included), the
//     one ending at the lowest corner, by corner height (CornerHeight: the
//     mean altitude of the 3–4 cells meeting there, as the rivers use),
//     ties to the lower corner id and then the lower edge id. The spill
//     corner is that corner: overflow leaves through it alone (S28, S30).
//     The spill cell is the edge's cell in the flat: the pass, outside the
//     depression at its spill level, and a cell of the parent (for a
//     nested depression) or a freely draining land cell (for a top-level
//     one). Spill corners touching the rim are no special case; the rim
//     is sea.
//
// A parent always spills strictly higher than its children, and a child has
// a lower id than its parent (depressions are numbered as they form). The
// flood ends with every non-sea cell taken; a depression that never closes
// (cells with no path to the sea) is an error.
//
// # Minimum depth
//
// A depression is a basin only when its depth is at least
// basin.min_depth_m (default 50 m). Depth never falls going outward (a
// parent spills higher and its bottom is no higher), so the dropped
// depressions are whole subtrees:
//
//   - a shallow depression nested in a basin merges into it: its cells
//     become the basin's own cells (Basin.Cells), and its deep siblings
//     stay the basin's children;
//   - a shallow top-level depression, with everything in it, is no basin:
//     no lake, no playa, no dry sink.
//
// Either way a shallow depression routes as flat ground at the spill level
// of its outermost shallow depression (Result.RouteM): the top-level one for
// cells in no basin, the one merged into a basin for a basin's cells. So
// water crosses a shallow dip instead of stopping in it, and altitudes are
// unchanged.
//
// Result keeps both: Depressions, the full hierarchy (for the depth
// histogram the minimum is tuned by), and Basins, the hierarchy after the
// minimum, renumbered in the same order, with BasinOf mapping one to the
// other. Each cell's innermost depression and basin are Result.Depression
// and Result.Of.
//
// The flood sorts the cells once and visits each edge a bounded number of
// times, so it costs about as much as the sea level stage's flood: the
// land-target search (stage 9) can run it at every probe.
//
// # Water balance
//
// Balance decides, for each basin, how much of it holds water (DESIGN.md,
// "Basins and lakes"). It reads the hierarchy, the cell areas, and the
// climate stage's precipitation P, potential evapotranspiration PET and
// runoff R (mm per year); volumes are mm·km² per year (10³ m³ per year),
// with each cell's real area, as the rivers will count discharge. Every
// cell is a whole unit: a cell is lake or land, never part of each.
//
// Catchment. Each cell drains by steepest descent on the routing height
// (RouteM): to its lowest lower neighbor, the sea counting lowest, ties to
// the lower id (Lakes.Down). A flat (connected cells of one routing height)
// drains breadth first toward its exits, the cells with a lower neighbor
// outside it; a flat with no exit is a pit, drained to its lowest cell id.
// With every shallow depression raised to its spill, the pits are the
// floors of the basins, and a cell's runoff reaches the basin of the pit
// its descent ends in (Lakes.Sink), or the sea. The catchment of a basin
// is many times its own cells (a median of 6 to 11 times on the worlds
// measured in S28): the slopes above the spill level drain into it too. A
// pit in no basin cannot occur; Lakes.Stray would count its cells, whose
// runoff then counts as reaching the sea. The rivers (S30) will build
// their own drainage on the corners; Down is the cells' version of it.
//
// Cost. A lake cell takes its precipitation, stops running off, and loses
// its PET (open-water evaporation) and the seepage S (basin.seepage_mm,
// default 50 mm). Against the runoff it gave as land, turning cell c to
// lake costs
//
//	d_c = area_c · (PET_c − (P_c − R_c) + S) = area_c · (PET_c − AET_c + S)
//
// where AET = P − R is the land's actual evaporation (Budyko), never above
// PET. So every cell costs at least its seepage, a lake's surplus only falls
// as it grows, and the lake is the largest set of cells, filled in order,
// whose costs the water reaching it covers.
//
// Fill. A basin fills from its bottom cell outward, lowest first: a
// priority queue of the cells next to the water, inside the basin, by
// altitude, ties to the lower id, so the water level (the highest
// altitude filled) only rises, a pocket from a merged shallow depression
// fills when the water reaches it, and the lake stays one connected set
// with one surface. A cell fills only when the basin's budget covers its
// cost (budget ≥ d_c). The fill stops at the first cell it cannot pay for,
// leaving the budget as the remainder (less than that cell's cost), or
// when no cell is left: the basin is full, at its spill level, and its
// whole remaining budget overflows through its one spill corner.
//
// Order. Each basin's budget starts as the runoff of its catchment.
// Top-level basins are taken by descending spill level (ties to the lower
// id), each with its nested basins, children before parents (their id
// order). Water reaching a basin whose children are not all full goes to
// the child with the lowest water level (a basin without water of its own
// counting at its bottom), ties to the lower id. When every child is full
// the parent fills as one surface: from the shore of its children's lakes,
// over its own cells, by the same rule. So the overflow of a full child
// runs to its parent, which passes it to a sibling not yet full, or, once
// all are full, fills above them. A top-level basin's overflow leaves
// through its spill corner onto the flat at the spill level around its
// spill cell and runs down from the lowest cell next to that flat outside
// the basin, the sea first (Water.OverflowVia): to the sea, or into the
// basin that cell drains to (Water.OverflowTo), whose spill is lower, so
// it is taken later. Overflow reaching a full basin passes on through it.
//
// Outcomes. A basin is full, partial (water below its spill level), or dry
// (no water of its own). A lake is the water of a basin with water whose
// parent has none, with every basin in it (they are all full): one
// connected set of cells, numbered by that basin's id, whose surface is
// the highest altitude filled, or the spill level when full. Two lakes
// never touch: sibling lakes meet only at their pass, which is not lake
// until their parent fills. A lake of basin.inland_sea_min_cells cells
// (default 20) or more is an inland sea. It is salt when it is closed (no
// overflow) and evaporation-dominated: its evaporation is positive and at
// least basin.salt_evap_share (default 0.5) of its evaporation plus
// seepage, a salinity proxy, not chemistry. A dry basin with no children
// has no stable level: it is a playa, dry land with surface playa on its
// bottom cell and a dry sink for the rivers at that cell's lowest corner
// by corner height, ties to the lower id; its inflow is its remainder. A
// partial basin's cells above its water are ordinary land. Lakes and
// playas below sea level are allowed: a basin floor the sea level stage
// left below the sea is a lake when it is wet and land (a playa at the
// bottom) when it is dry. Possible, not forced: worlds without playas or
// salt lakes are fine.
//
// Conservation. The water entering the basins (the runoff of the land
// cells that drain to them, and the precipitation on the lakes) equals
// the lakes' evaporation and seepage, the overflow reaching the sea, and
// every basin's remainder; Lakes.Residual is the relative difference, and
// each lake's own balance (its runoff, precipitation and received
// overflow against its evaporation, seepage, overflow and remainder) is
// Lake.Residual. Both are within ConservationTolerance (10⁻⁹): they differ
// from 0 only by rounding.
//
// Balance is linear in the cells and basins, so the land-target search
// (S29) can run it at every probe against a fixed climate. The later
// stages do not read its lakes yet; S29 brings them into classification,
// the edges and world.json.
//
// # Renders
//
// Render, the stage render, fills basin cells by their top-level basin's
// Palette color, darker per level of nesting, cells of shallower
// depressions in ShallowColor, other land a pale gray by altitude, the
// ocean flat blue and the rim as ice, and draws every spill edge in red with
// its spill corner marked. Variant "depth", DepthRender, fills basin cells
// by how far below their innermost basin's spill level they lie. Variant
// "lakes", LakesRender, fills lake cells by kind and salinity (LakeColor:
// fresh and salt lakes, fresh and salt inland seas), playa cells in
// PlayaColor and other basin cells in DryBasinColor, marks each dry sink
// corner, draws the spill edge and outlet corner of every overflowing lake,
// and dots the path by which a top-level basin's overflow runs into the
// basin downstream.
//
// # Determinism
//
// Every choice is ordered by altitude and cell, corner or edge id; nothing
// depends on map order or scheduling. Find's only arithmetic is the depth
// (a subtraction) and the corner height (a sum in cell id order and one
// division), neither of which can fuse. Balance rounds every product
// (package fmath) before it is added, adds the inflows and the lakes'
// volumes in cell id order, and moves water in the fixed order above;
// TestNoFusedMultiplyAdd checks the compiled code. Result.AppendBinary and
// Lakes.AppendBinary give the canonical encodings the golden hashes cover.
package basin
