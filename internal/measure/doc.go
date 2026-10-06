// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package measure computes playability measures and checks (DESIGN.md,
// "Playability measures"; pipeline stage 13).
//
// # The report
//
// Compute measures a generated world from the stage products and returns a
// world.Measures, the exported type of measures.json (package world holds
// the schema, as it does for world.json). The pipeline's measures stage
// writes it with world.Measures.Bytes, and the text summary (Lines,
// Summary) to SummaryFile, measures.txt, and to the log. Measures never
// change the world or its hashes; they describe it.
//
// The groups (S33 to S35):
//
//   - land: the land cell count against N, its deviation and tolerance,
//     whether the contract is met, and the land area against N·A;
//   - mesh: cell counts; the cell-area mean and coefficient of variation
//     over every cell (package mesh's Stats) and over the land cells, with
//     the land cells' smallest and largest area, all as multiples of A;
//     the shortest and the nearest-rank 5th-percentile edge between two
//     playable cells; neighbor-count histograms of the playable and the
//     land cells (index = neighbors, 0 to 8); the short-edge collapse's
//     collapses, stretches, degree-cap collapses and largest corner shift;
//   - directions: the compass direction error (mean, nearest-rank p95 and
//     max, in degrees, over the playable cells' half-edges), the edges
//     between playable cells whose two directions are not opposite points
//     and their share, and the naive nearest-point collisions (package
//     edges' Stats);
//   - grades: histograms of |grade| over land–land and playable edges
//     (edges.GradeBucketNames), and the land–land edges' count, steepest
//     and 95th-percentile grade in percent, and count at the 100% cap;
//   - water: ocean, lake and inland-sea cells, lakes and inland seas and
//     the largest of each, coast edges and coast edges per land cell, and
//     land–rim edges;
//   - landmasses (S34): the connected sets of land cells (land after
//     lakes, joined through shared edges; lakes and inland seas are water),
//     ids by lowest cell; their count, a size histogram, the largest and
//     its share of the land, and counts by class from config
//     measures.landmass (islet ≤ 9 cells, continent ≥ 1000, island
//     between, by default), with a list of every landmass;
//   - chokepoints (S34): straits, necks and mountain passes (below);
//   - water also gains (S35) lake and inland-sea size histograms in the
//     landmass size buckets;
//   - features (S35): every depression of the land target's basin
//     hierarchy, those shallower than basin.min_depth_m counted apart, in a
//     depth histogram (world.DepthBuckets, meters); the basins by water
//     state and the deepest; the playas and the dry basins, a dry basin
//     being a basin with no water in it or in any basin nested in it whose
//     parent holds water (count, cells, largest, deepest, and a list); the
//     glacier, ice-field, polar-desert and pack-ice cells; the hotspots,
//     the volcanoes on land, and the volcanic-highland cells. Report only:
//     each may be 0;
//   - rivers (S35): package river's NetworkStats restated: river edges by
//     class and per land cell, km per 1,000 km² of land, the land cells
//     touching a river, polylines, mouth corners and polyline ends by
//     terminal, the longest polyline and flow, seam edges, and the largest
//     drainage and discharge;
//   - usability (S35): habitable land (not biome clear, polar-desert or
//     desert, and not mountains; wetlands count) and wetlands with their
//     shares, desert and mountain cells, the coast distance (below), and
//     the land's biome and landform histograms.
//
// # Coast distance
//
// A land cell's coast distance is the fewest steps through land to
// playable water (ocean, lake or inland sea, as the coast flag counts it;
// never the rim), 1 for a cell touching water: a breadth-first search from
// every coastal cell at once. usability.coast_within_cells counts the land
// within d = measures.usability.coast_cells (default 3) of the coast, and
// coast_distance is the whole histogram (index = distance).
//
// # Chokepoints
//
// Chokepoints are measured in cells, after wgvc (seas.go assignStraits and
// assignNecks; adapted, not imported), with config measures.chokepoints
// (k = max_cells) and measures.passes:
//
//   - Straits: from each coastal land cell a breadth-first search through
//     playable water (ocean, lakes, inland seas; never the rim) records the
//     water cells within k. Two shores reaching one water cell make a
//     crossing of d_a + d_b − 1 cells, which qualifies when it is at most k
//     and the shores lie on different landmasses, or on one landmass with
//     no land path of at most detour_cells steps between them. The water
//     cells on qualifying crossings, grouped by landmass pair and water
//     adjacency, are the straits; major straits have no islet shore.
//   - Necks: cuts of at most k land cells whose removal leaves two regions
//     of the landmass of at least neck_min_region_cells each. Width 1 comes
//     from Tarjan's articulation points with subtree sizes; wider cuts are
//     paths of adjacent land cells whose end cells touch playable water,
//     tested by removal and kept only when no smaller subset is a cut.
//     Cuts that share or touch a cell merge into one neck.
//   - Passes: a mountain chain is a connected set of at least
//     chain_min_cells mountain cells. A pass is a route through at most
//     passes.max_cells of a chain's cells, every edge's |grade| at most
//     max_grade_percent, between land cells off every chain that no land
//     path of at most passes.detour_cells steps avoiding the chain joins.
//     The chain cells on such routes, grouped per chain by adjacency, are
//     the passes.
//
// Each chokepoint is listed with its width (the narrowest crossing), the
// landmasses it joins (two for a strait between landmasses, else one), and
// its cells.
//
// # Render
//
// Analyze is Compute that also returns a Map, whose Render is the
// measures stage render, the landmass and chokepoint map: landmasses by
// class (continents, islands, islets), mountain chains darkened, and the
// straits, necks and passes over them.
//
// # Names and checks
//
// A scalar measure is named by its JSON path, "group.field"
// (world.MeasureNames). config.json's measures.checks compare named
// measures with bounds; Evaluate runs them in order and records each
// result in the report. A check is report-only (mode "report": a failure
// is listed and logged) or a gate (mode "gate": a failure fails the run,
// after every output is written; mpg generate exits 3). Checks begin as
// report-only (config.DefaultMeasures); promote them to gates once tuning
// shows sensible ranges. Measures never require a feature: a count of 0
// lakes is a measurement, not a failure, unless a configured check says so.
//
// # Determinism
//
// The report holds the seed and the config hash, but no timestamps, paths,
// or generator version, so the same config gives the same bytes. Cells and
// edges are visited in id order, searches visit neighbors in ascending
// order, every sort has a complete tie-break, and no result depends on map
// order; sums run in that order with every product
// rounded (package fmath); percentiles are nearest-rank over sorted values;
// every ratio with a zero denominator is 0, so no measure is NaN or
// infinite. TestNoFusedMultiplyAdd checks the compiled code.
package measure
