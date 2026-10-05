// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package export converts the generator's stage products into the world.json
// types of package world (pipeline stage 14, DESIGN.md "One game data
// file").
//
// The world package is the schema the game imports, so it must not import
// mpg's internal packages; the conversion lives here instead (DESIGN.md's
// package layout has no export package; this one sits between the stage
// packages and world). Build reads the mesh, the cell statistics, the sea
// level, the classification, and the edge data, and returns a *world.World;
// the pipeline's export stage validates it (world.Validate) and writes its
// canonical bytes (World.Bytes).
//
// # Rules
//
//   - Geometry. Corners and sites are rounded to world.PrecisionKm (1 mm),
//     x kept in [0, W) (a value that rounds up to W wraps to 0), y kept
//     exactly when it is 0 or H. Polygon offsets are the rounded
//     displacement from the rounded site to each rounded corner (topo DX the
//     shorter way, dy unwrapped), the bounding box the rounded extremes of
//     site + offset, the centroid package mesh's rounded, and an edge's
//     length the rounded wrapped distance between its rounded corners.
//   - Cells. Landform and depth are classify's names; the water kind is
//     ocean for the sea level stage's ocean (milestone 4 has no other
//     water); rim cells have none. Flags: rim, impassable (mesh), volcano
//     (classify), and coast when any side is a coast edge. Exits are the
//     edge stage's half-edges in its compass order, bearing and error
//     rounded to 0.01°.
//   - Corner height is the mean altitude of the corner's cells (DESIGN.md,
//     "Rivers on edges": "the mean altitude of the 3–4 cells that meet at
//     the corner"), summed in ascending cell id order and divided by the
//     count. A corner is terminal when it touches a land cell and a water or
//     rim cell (a shore corner, where a river would end); dry-sink corners
//     join them in milestone 6, and the mouth flag comes with rivers.
//   - Edge seeds. Each edge's noise seed is a uint32 drawn from the seed
//     stream "edge-noise" (DESIGN.md, "Determinism") at algorithm version
//     EdgeNoiseVersion: seed.Rand(world seed, "edge-noise", "1").Uint32(),
//     one draw per edge in edge id order. A uint32 survives any JSON reader
//     exactly, which a uint64 would not.
//   - Coastlines. Each coast edge is directed with its land cell on the
//     right (its own clockwise order). The next edge leaves the end corner
//     hugging that land: the land cell's next side clockwise; where that
//     side leads to more land, that cell's next side, and so on around the
//     corner until a side has water across it. So land that meets other
//     land only at a 4-way corner keeps its own coastline, as cell
//     adjacency keeps such land apart. A side against the rim or the map
//     edge ends a chain (open); such chains start at the edge with no
//     predecessor. Every other chain is closed and starts at its lowest
//     edge id. Chains are ordered by first edge id. Chains cross the seam
//     like any other edge: they follow corner ids, not coordinates.
//   - Outcomes are the sea level stage's, and the deferred stages the run
//     passed over.
//
// # Determinism
//
// Rounding is a correctly rounded product, math.Round, and a correctly
// rounded quotient; nothing is a product feeding a sum, and
// TestNoFusedMultiplyAdd checks the compiled code. Cells, corners, and edges
// are visited in id order; coastline starts are sorted.
package export
