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
//     no lake, no playa, no dry sink. Its cells route as flat ground at its
//     spill level (Result.RouteM), and their altitudes are unchanged.
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
// # Renders
//
// Render, the stage render, fills basin cells by their top-level basin's
// Palette color, darker per level of nesting, cells of shallower
// depressions in ShallowColor, other land a pale gray by altitude, the
// ocean flat blue and the rim as ice, and draws every spill edge in red with
// its spill corner marked. Variant "depth", DepthRender, fills basin cells
// by how far below their innermost basin's spill level they lie.
//
// # Determinism
//
// Every choice is ordered by altitude and cell, corner or edge id; nothing
// depends on map order or scheduling. The only arithmetic is the depth (a
// subtraction) and the corner height (a sum in cell id order and one
// division), neither of which can fuse; TestNoFusedMultiplyAdd checks the
// compiled code. Result.AppendBinary gives the canonical encoding the
// golden hashes cover.
package basin
