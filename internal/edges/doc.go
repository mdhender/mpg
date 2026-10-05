// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package edges builds the per-edge game data (DESIGN.md, "Edges";
// pipeline stage 12): each cell's half-edges with bearing, compass
// direction, neighbor, and incline, and each undirected edge's coast,
// river, incline, and passability.
//
// DESIGN.md's package layout has no edges package; the stage is its own
// package, like classification (stage 11), because it is a separate set of
// rules over the mesh and the cell statistics, and the exported world
// types will copy from it.
//
// # Edges and half-edges
//
// Data.Edges is indexed by mesh edge id, so each undirected edge is stored
// once. Data.Cells lists each cell's half-edges, one per neighbor. Edges on
// the rim boundary (mesh Edge.Cells[1] == mesh.Boundary, on y = 0 or y = H,
// held only by rim cells) have no neighbor, so they have no half-edge and
// no direction; their Edge entry is impassable, not a coast, with incline
// 0. The two cells across a collapsed short edge touch only at a point and
// are not neighbors, so they have no half-edge to each other (the half-edges
// come from the cell's edges, not from its corners). Build checks that a
// cell has exactly one half-edge per neighbor and at most MaxDegree (8), the
// mesh's degree cap.
//
// # Bearing
//
// A half-edge's bearing is topo Cylinder.Bearing from the cell's site to the
// neighbor's site: degrees clockwise from north in [0, 360), using the
// wrapped east–west delta, with north toward −y (y runs south). Each side's
// bearing is computed from its own site, so the two are not forced to
// differ by exactly 180°.
//
// # Direction
//
// Each half-edge gets one of the 8 compass points N, NE, E, SE, S, SW, W,
// NW (codes 0 to 7, at 45° times the code), solved per cell by Assign:
//
//   - Sort the cell's edges by bearing, ties by position (the cell's edge
//     order), giving e₀ … e_{k−1}.
//   - An assignment gives each edge a distinct compass point. It is
//     order-preserving when the points follow the edges' cyclic clockwise
//     order: going clockwise from e_j's point, the next point used is
//     e_{j+1}'s (and after e_{k−1}'s, e₀'s), so the edges and the rose never
//     cross. Equivalently the clockwise gaps (code of e_{j+1} − code of e_j,
//     mod 8, and e₀'s after e_{k−1}'s) are all at least 1 and sum to 8. The
//     smallest bearing need not take the smallest code: bearings 340°, 350°,
//     355° become NW, N, NE. There are C(8, k)·k such assignments.
//   - Each edge's error is the angle between its bearing and its point, the
//     smaller way around (AngularError).
//   - Closest fit: the least total error; then the least largest error;
//     then "the assignment that starts at the earliest compass point",
//     made precise as the lexicographically least sequence of compass codes
//     listed in sorted-bearing order (e₀'s code first, so the first test is
//     the code given to the edge with the smallest bearing, and later edges
//     break any remaining tie). That order is total, so the choice is
//     unique.
//   - Errors are compared exactly as integers: each error is scaled by 2³²
//     (exact) and rounded to the nearest integer, so errors are compared
//     in units of 2⁻³² degree (about 2.3·10⁻¹⁰°) and sums of up to 8 of them
//     are exact. Errors equal at that resolution tie; no epsilon is used.
//
// The solver is a dynamic program over the edges in sorted order. For each
// start point d₀ of e₀ it lifts the codes to D₀ = d₀ < D₁ < … < D_{k−1} ≤
// d₀ + 7 (code = D mod 8) and keeps, per edge and lifted code, the least
// partial total, the number of partial assignments reaching it (saturated
// at 2), and the lexicographically least partial sequence. That is exact
// for (total, sequence). Only when more than one assignment reaches the
// least total does the largest error matter; then a bisection over the
// cost values, each step the same program restricted to errors at or
// below a limit, finds the least limit that still reaches the least total
// (the least largest error), and the restricted program's sequence is the
// answer. The tests check it against an enumeration of every
// order-preserving assignment.
//
// Directions are per cell and not symmetric: A's edge to B is usually the
// opposite point of B's edge to A, but not always. Each half-edge stores
// its own, and its error.
//
// The nearest-point label (Nearest) is not used: wgvc (exits.go
// compassFor; docs/explanations/bearing-not-compass.md) labeled each exit
// with its nearest point and found duplicates in 193 of 300 provinces.
// Stats.NaiveCollisions measures the same thing here: about 1% of the
// playable cells. That is far fewer than wgvc's, whose cells had 4 to 10
// sides; here the cells are Lloyd-relaxed, short edges are collapsed, and
// most cells have 5 or 6 neighbors. It is not zero, so the per-cell solve is still needed. wgvc numbered exits clockwise from the
// smallest outward bearing (exits.go assignExits); the clockwise order
// here comes from the compass points instead.
//
// # Clockwise order
//
// A cell's half-edges are listed by compass code, N first. Because the
// assignment preserves the cyclic order, that list is clockwise, and it
// starts from the edge nearest N in compass terms: the edge whose point is
// N, or if N is unused the first used point clockwise from N. The game's
// "move NE" is a lookup in this list (Data.Toward).
//
// # Coast and water
//
// An edge is a coast when exactly one side is water and neither side is a
// rim cell; Edge.Water is the water side's kind (ocean, lake, or inland
// sea; the pipeline passes the land-target stage's: its ocean, and its
// lakes by kind, while dry basin floors and playas are land). A lake never
// touches the ocean, so no edge joins two kinds of water. The rim is the impassable ice sheet, which the
// generator counts as deep salt water: an ocean–rim edge is not a coast,
// and a land–rim edge, which the rim falloff should prevent, is not a coast
// either; Stats.LandRim counts those (0 in every world seen).
//
// # River
//
// Edge.River is the river class, RiverNone until the river stage exists.
//
// # Incline
//
// The signed grade from Cells[0] to Cells[1] of each mesh edge, computed
// once (Grade): (altitude of Cells[1] − altitude of Cells[0]) / site
// distance × 100%, with the cells' altitudes (package cells' median, the
// one height) and topo's wrapped site distance. It is stored as an integer
// number of tenths of a percent (Incline), which is exactly (Δ m) / (km),
// clamped to ±1000 (100%) and rounded halves away from zero, so the
// rounding is symmetric; Cells[1]'s half-edge holds the exact negation,
// and an integer has no negative zero. Every edge between two cells gets
// an incline, over water and against the rim included: altitude is the
// one height, and the game decides what grades mean.
//
// # Passability
//
// Edge.Passable is mesh Mesh.Passable: false on the rim boundary and on
// every edge of a rim cell. The game may add rules on top.
//
// # Statistics
//
// Summarize gives the numbers the stage logs and the playability measures
// will report: the playable cells' neighbor-count histogram; the
// direction error (mean, nearest-rank p95, and max over the half-edges of
// playable cells); how often an edge between two playable cells has
// directions that are not opposite points; the naive nearest-point
// collisions; incline histograms of |grade| (0–1, 1–2, 2–5, 5–10, 10–20,
// 20–50, 50–100%, and at the cap) over land–land edges and over all edges
// between playable cells; coast edges and coast edges per land cell; and
// the passable, impassable, and rim-boundary edge counts.
//
// # Renders
//
// InclineRender, the stage render, draws the land–land edges colored by
// |grade| (GradeRamp, from 1%) over pale land and water fills and the ice.
// Variant "passability", PassabilityRender, draws the impassable edges in
// red and the coast edges in navy. Variant "compass", CompassRender, is a
// zoomed crop around the land cell nearest the middle of the map, with a
// tick and label per half-edge colored by direction, for checking the
// assignment by eye.
//
// # Determinism
//
// The edge data depends only on the mesh, the altitudes, and the water
// kinds. Bearings and distances come from topo (package fmath); the
// assignment compares exact integers; the incline is one subtraction, one
// division, a clamp, and math.Round. Edges and cells are visited in id
// order, and sorting has complete tie-breaks. Data.AppendBinary gives the
// canonical encoding the golden hashes cover, and TestNoFusedMultiplyAdd
// checks the compiled code.
package edges
