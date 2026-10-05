// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package world defines the exported Go types for world.json; these types are
// the schema.
//
// world.json is mpg's one game data file (DESIGN.md, "One game data file").
// The game engine and the player-map renderer both read it: for every cell
// it holds the game data (geography, altitude, flags including rim) and the
// geometry to draw it. The package imports nothing from mpg, so the engine
// and converter tools can import it alone. It reads (Decode), writes
// (World.Bytes), and checks (Validate) the file; package internal/export
// fills it from the generator's stage products.
//
// The layout follows hmz2map's (mdhender/tpty/hmz2map, map.go: exported
// types as the schema, a schema version field, coded values as kebab-case
// strings with their lists in the package, a Validate that returns the
// problems found) with the cell, corner, and edge graph of
// maloquacious/wg/voronoi (voronoi.go Mesh{Cells, Corners, Edges}).
//
// # The file
//
// The top-level object holds, in order: the schema version (0); Meta, the
// map's size and units; Codebooks, every coded field's values; Outcomes,
// what the run found; then Cells, Corners, Edges, Coastlines, and Rivers.
// Every list of things with ids is indexed by id, and each element repeats
// its id. Names are snake_case, as in config.json, and quantities carry
// their unit as a suffix (altitude_m, length_km).
//
//   - Coordinates are km from the northwest corner: x runs east in [0, W)
//     and wraps; y runs south in [0, H] and does not. A Point is written as
//     [x, y].
//   - A Cell has its landform, depth band (salt water only), water kind
//     (playable water only), altitude (the median of its raster samples:
//     the one height), and flags; its site and centroid; its polygon as
//     clockwise corner ids (Corners, from the lowest id), the edge along
//     each side (Sides), and the same polygon unwrapped about the site in
//     km (Polygon), so a cell across the seam draws without special cases;
//     the bounding box of the unwrapped polygon in world km (x may leave
//     [0, W)); and its exits: one half-edge per neighbor in compass order
//     (N first, so clockwise), with direction, neighbor, edge id, signed
//     incline, bearing, and direction error.
//   - A Corner has its position, height (the mean altitude of the 3–4 cells
//     that meet there, as DESIGN.md's rivers define it), flags (boundary,
//     terminal, and later mouth), and its cells and edges.
//   - An Edge has its two cells (Cells[1] = Boundary, −1, on the map's
//     north or south edge), its corners in Cells[0]'s clockwise order,
//     length, noise seed, passability, coast flag and water kind, river
//     class, and the incline from Cells[0] to Cells[1].
//   - A Coastline is a chain of coast edges walked with land on the right:
//     clockwise around islands, counterclockwise around enclosed water.
//   - Rivers are river polylines; none until milestone 7.
//   - Outcomes hold the sea level, the land, ocean and dry basin counts, the
//     target and tolerance, whether it was met and why the search ended, its
//     policy, budget, and trace, and the deferred pipeline stages. Outcomes
//     never go in config.json.
//
// # Units and precision
//
// Lengths are km, areas km², heights m, angles degrees clockwise from north,
// and inclines tenths of a percent (incline_permille: +150 climbs 15.0%).
// Positions, offsets, lengths, and bounding boxes are rounded to
// PrecisionKm (1 mm); offsets are taken between the rounded site and
// corner, so site + offset reaches the corner modulo W to within
// floating-point error. A y of exactly 0 or H is kept exactly. Bearings and
// direction errors are rounded to 0.01°. Altitudes, corner heights, the sea
// level, and the land area keep every bit (the shortest decimal that reads
// back exactly), because the land test (altitude above sea level) and the
// incline are exact rules on them.
//
// # Omitted until later milestones
//
// Version 0 is the first playable export (milestone 4). Surface and biome
// come with milestone 8, lakes and inland seas (fresh-water landform, lake
// and inland-sea water kinds) with milestone 6, and rivers (edge classes,
// river polylines, corner mouths) with milestone 7; the codebooks already
// list the values. The schema is frozen as version 1 in milestone 9; until
// then it may change without migration.
//
// # Determinism
//
// World.Bytes writes compact JSON in the field order of the Go types, with
// floats formatted as encoding/json formats them, so the same world always
// gives the same bytes. The file holds no timestamps, no paths, and not the
// generator's version (which would change the bytes at every commit); it
// identifies its input by the config hash. Validate's arithmetic rounds
// every product before it is added, and TestNoFusedMultiplyAdd checks the
// compiled code.
package world
