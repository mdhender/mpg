// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package river builds the corner drainage tree and the river network on it
// (DESIGN.md, "Rivers on edges"; pipeline stage 10). Rivers run along
// Voronoi edges, corner to corner, never through a cell; the tree says, for
// every land corner, which corner its water runs to next, and Accumulate
// sends each land cell's runoff down it to select the river edges, class
// them, and split them into polylines.
//
// # Corners and terminals
//
// A corner's height is the mean altitude of the 3–4 cells meeting there
// (basin.CornerHeight, the same height the basins' spill corners use). The
// graph holds the land corners, those touching a land cell, joined by the
// land–land edges: edges with a land cell on both sides. A coast or lake
// shore edge has water beside it, so it is never in the graph, and no
// river can run along a shore. Each land corner is, in this order:
//
//   - Ocean: it touches the sea, an ocean cell or a rim cell (the rim is
//     sea, as in the basins' flood; on the worlds measured no land corner
//     touches the rim at all);
//   - Lake: it touches a lake or inland sea; a corner touching two lakes
//     (only at a 4-way corner) belongs to the one with the lower surface,
//     ties to the lower id;
//   - Sink: it is a dry sink (each playa's lowest corner);
//   - Interior otherwise.
//
// Every terminal (non-interior) corner ends water, except a lake's outlet.
//
// # The flood
//
// A priority flood from the terminals gives every land corner one
// downstream corner. The roots are the sea corners, the dry sinks, and the
// shore corners of closed lakes, each at its height. The lowest queued
// corner is taken; it chooses as its downstream corner, among its neighbors
// across land–land edges already taken, the one with the lowest level, ties
// to the one taken first; then each neighbor not yet reached is queued at
// the larger of its height and the taken corner's level. A corner's level
// is therefore the lowest level at which its water can leave: its height,
// or the spill level of the corner-level pit it lies in, which water
// climbs out of. The tree is acyclic by construction (a corner's
// downstream corner was always taken before it), and every land corner
// reaches a terminal (Build fails otherwise).
//
// Flats. Corners of one level are taken first in, first out (the queue is
// ordered by level, then by the order corners were queued), so inside a
// flat, a pit filled to its spill level or a plateau of equal heights,
// water runs breadth first from where the flat drains: each corner's path
// to the flat's edge is as short as the edge allows. Taking them by corner
// id instead (the order corners are numbered, north to south) combs the
// flow into parallel north–south runs; on the worlds measured, 1.9–4.2% of
// tree steps are flat, all in filled pits, as no two neighboring land
// corners have exactly equal heights.
//
// # Lakes
//
// A closed lake's shore corners are roots: rivers may end at any of them.
// An overflowing lake (one whose basin spills; its spill corner, the
// basins' rule, and its pass cell, the basin's spill cell) drains by one of
// these rules:
//
//   - (a) Direct. If its spill corner touches the sea, or belongs to
//     another, lower lake, the lake drains straight there and no river
//     leaves it (DirectSea, DirectLake).
//   - (b) At the spill corner. If the spill corner has a land–land edge to
//     a corner off the lake, it is the outlet (Outlet, AtSpill).
//   - (c) On the pass cell. Otherwise (about a third of overflowing lakes:
//     the spill corner, the lower end of the spill edge, usually touches
//     two lake cells and the pass cell, so it has no land–land edge, or its
//     only one runs along to another shore corner), the outlet is the first
//     of the lake's shore corners on the pass cell that the flood reaches.
//     If that corner has no way over land (it touches the sea, or lies
//     between this lake and another at a 4-way corner, where the other
//     lake's release reached it), the lake drains straight into that water
//     instead, as in (a).
//
// In every case the lake's other shore corners are held, never queued,
// until its water has somewhere to go: until its outlet is taken, which
// chooses its downstream corner from the land side, or until its spill
// corner is taken (direct). Then the shore corners are released, queued at
// the larger of their height and the outlet's level, and end the rivers
// that reach them. So an outlet's path never returns to its own lake, and
// lakes draining into lakes form no loop. The water entering a lake, by any
// shore corner, leaves only by the outlet, which carries its surplus (S31
// injects the water balance's Overflow there).
//
//   - (d) Fallback. If the flood runs out of queued corners while corners
//     are held, the held corner with the lowest (level, id) becomes the
//     outlet of every lake holding it. It is counted (Tree.Fallbacks) and
//     logged; on the worlds measured it is never needed.
//
// # Catchments
//
// Each land cell's runoff enters the tree at its lowest corner by height,
// ties to the lower id (Tree.Lowest). Its destination (Tree.Dest) is where
// that corner's path ends: the sea, a lake, or a dry sink; Tree.Final
// follows overflowing lakes on to the sea, a closed lake or a sink. Given
// the cell-level model's destinations (the water balance's steepest
// descent, basin Lakes.Sink), Build records how many land cells, and how
// much area, end elsewhere (Tree.Agreement): on the worlds measured, 1.4–
// 5.9% immediately and 0.3–8% finally, near ridges and basin rims.
//
// # Accumulation
//
// Accumulate (S31) sends each land cell's area (km²) and runoff (area ×
// the final climate pass's runoff depth, in mm·km² per year) to its lowest
// corner, and accumulates both down the tree in reverse flood order, so
// every corner sees the corners upstream of it first. A corner's Drainage
// and Volume are everything that reaches it; a tree edge carries those of
// its upstream corner (Network.Up), the water flowing through it.
//
// Lakes. The drainage reaching a lake's shore corners, the lake's own
// cells' area, and the drainage of the lakes draining straight into it
// (DirectLake) make the lake's drainage (Network.LakeDrainage). A lake with
// an outlet passes it on there, and the outlet's volume gains the water
// balance's Overflow, the authority on what leaves the lake (the tree's own
// inflow can differ, being from the final climate pass where the balance
// used the first). A closed lake, or one draining straight to the sea,
// ends its drainage. In reverse flood order every shore corner of a lake
// comes before its outlet, and a lake draining straight into another
// before that one's outlet, so each lake is complete when passed on;
// Accumulate checks it.
//
// Discharge in m³/s is the volume × 1000 / 31,557,600 (a Julian year).
//
// # River classes
//
// A tree edge is a river when its drainage is at least Params.ThresholdKm2
// (config river.threshold_km2, 500 km²); it is a stream from there, a
// river from RiverKm2 (2,000) and a major river from MajorRiverKm2
// (10,000). Drainage never falls downstream, so neither does the class.
// The classes go on the edges (edges.Build); drainage and discharge stay
// internal, for the logs, measures and debugging.
//
// # Polylines and mouths
//
// The river edges are split by main stem into polylines (Network.Paths),
// listed downstream from their source and ordered by first edge id. At a
// corner the main inflow is the river edge into it with the largest
// drainage, ties to the lower upstream corner id; at a lake's outlet the
// lake is an inflow too, with the drainage it passes on, and wins a tie. A
// polyline starts at a corner with no main river inflow (a source, or an
// outlet whose lake is its main inflow) and runs down the tree to a
// terminal, its mouth, or to a corner where it is not the main inflow, a
// confluence, where the main stem continues. Each river edge lies on
// exactly one polyline. Network.Mouth marks the corners where a polyline
// ends at a terminal: on the sea, a lake shore, or a dry sink.
//
// NetworkStats reports the class counts, the density (river edges per land
// cell, km per 1,000 km² of land, the share of land cells touching a
// river), the polylines and how they end, the mouths, the longest river,
// and the seam crossings.
//
// # Renders
//
// Render, the stage render, draws the river edges in RiverInk, a dark blue
// no water fill uses, wider by class (RiverWidths), over the land (by
// altitude), lakes (closed ones salt-green, draining ones blue), sea and
// ice, with lake outlets and dry sinks marked. TreeRender, variant "tree",
// draws every tree edge over the same base, in FlatInk where the step is
// flat or climbs out of a pit. CatchmentRender, variant "catchments",
// fills each land cell by its destination (a palette color per lake, the
// playa color for a sink, pale for the sea) and dots the cells whose
// destination differs from the cell-level model's.
//
// # Determinism
//
// Every choice is ordered by level and queue order or corner id; the queue
// order follows from processing in a fixed order, so nothing depends on
// map order or scheduling. The tree's only arithmetic is the corner height
// (a sum in cell id order and one division) and the agreement's area sums
// in cell id order. The accumulation adds in cell id order, then in
// reverse flood order, each runoff × area product rounded before it is
// added (fmath.Mul); TestNoFusedMultiplyAdd checks the compiled code.
// Tree.AppendBinary and Network.AppendBinary give the canonical encodings
// the golden hashes cover.
package river
