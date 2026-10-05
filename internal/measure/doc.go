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
// The groups so far (S33):
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
//     land–rim edges.
//
// Landmasses and chokepoints (S34) and features, rivers and usability
// (S35) join as further groups.
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
// edges are visited in id order; sums run in that order with every product
// rounded (package fmath); percentiles are nearest-rank over sorted values;
// every ratio with a zero denominator is 0, so no measure is NaN or
// infinite. TestNoFusedMultiplyAdd checks the compiled code.
package measure
