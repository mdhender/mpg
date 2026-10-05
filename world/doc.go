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
//     (playable water only), biome (playable land only), surface (glacier,
//     ice field or a wetland on land, pack ice on playable water; none on
//     the rim), altitude (the median of its raster samples: the one
//     height), and flags; its site and centroid; its polygon as
//     clockwise corner ids (Corners, from the lowest id), the edge along
//     each side (Sides), and the same polygon unwrapped about the site in
//     km (Polygon), so a cell across the seam draws without special cases;
//     the bounding box of the unwrapped polygon in world km (x may leave
//     [0, W)); and its exits: one half-edge per neighbor in compass order
//     (N first, so clockwise), with direction, neighbor, edge id, signed
//     incline, bearing, and direction error. Lakes and inland seas are
//     water cells: a lake has the fresh-water landform and water kind lake,
//     an inland sea (basin.inland_sea_min_cells cells or more) salt-water
//     and inland-sea, by size alone; the salt flag marks a salt lake or
//     inland sea (closed and evaporation-dominated), and only those (the
//     ocean and rim are salt by definition). A playa's cell, the bottom of
//     a dry basin, is land with the playa flag.
//   - A Corner has its position, height (the mean altitude of the 3–4 cells
//     that meet there, as DESIGN.md's rivers define it), flags (boundary,
//     terminal, mouth where a river ends, and sink for a playa's dry sink), and its
//     cells and edges.
//   - An Edge has its two cells (Cells[1] = Boundary, −1, on the map's
//     north or south edge), its corners in Cells[0]'s clockwise order,
//     length, noise seed, passability, coast flag and water kind, river
//     class, and the incline from Cells[0] to Cells[1].
//   - A Coastline is a chain of coast edges walked with land on the right:
//     clockwise around islands, counterclockwise around enclosed water.
//   - Rivers are river polylines from source to mouth or confluence, split
//     by main stem, each edge with its class.
//   - Outcomes hold the sea level, the land (after lakes), ocean, dry
//     basin, lake and inland-sea cell counts, the lake, inland-sea, salt
//     and playa counts, the biome table and the glacier, ice-field,
//     pack-ice and wetland cell counts, the target and tolerance, whether
//     it was met and why the land-target search ended, its policy, budget,
//     expected lake cells and trace (each probe with its lake cells), the
//     climate passes, the elevation pre-pass's lake cells and datum land
//     share, and the deferred pipeline stages. Outcomes never go in
//     config.json.
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
// Version 0 is the first playable export (milestone 4); lakes, inland
// seas, the salt and playa cell flags, the sink corner flag and the lake
// outcomes came with milestone 6 (S29), and rivers (edge classes, river
// polylines, corner mouths) with milestone 7 (S31), and biomes and
// surfaces (the cell fields, the biomes and surfaces codebooks, and the
// biome table and surface counts in the outcomes) with milestone 8 (S32).
// The schema is frozen as version 1 in milestone 9; until then it may
// change without migration.
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
