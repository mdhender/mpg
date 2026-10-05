// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package classify assigns landform, depth, surface, and biome.
//
// Pipeline stage 11 (DESIGN.md, "Classification") gives every cell a
// landform, every salt-water cell a depth band, and every land cell
// holding a volcano's peak the volcano flag. The vocabulary and the rules
// are hmz2ter's (maloquacious/hmz2ter, rules.go Rules.Landform and
// Rules.Depth, terrain.go Classify), applied per cell; surface and biome
// come with the climate milestones.
//
// # Landform
//
// Every height rule uses the cell's altitude (package cells: the median of
// its samples) measured from the chosen sea level, where hmz2ter used a
// hex's median elevation above 0 m; relief is the cell's p95 − p5. A land
// cell (sea level stage Flood.Land: the land candidates and, until the
// basin stage decides them, the dry basin floors) is
//
//   - plateaus, when it is at least plateau_min_altitude_m above sea level
//     and its relief is below plateau_below_m, whatever its relief class;
//   - otherwise, by relief: flats below flats_below_m, plains below
//     plains_below_m, rolling plains below rolling_plains_below_m, hills
//     below hills_below_m, and mountains from there up.
//
// A dry basin floor below sea level is classified like any other land; its
// negative height only keeps it off the plateaus.
//
// Every other playable cell is salt water: in milestone 4 there are no
// lakes, so the ocean is the only water, and fresh water waits for the
// basin stage. Rim cells are salt water too, as the generator treats them
// (DESIGN.md, "Rim"), with depth deep; their rim and impassable flags
// override this for the game.
//
// # Volcanoes
//
// A hotspot's peak lies in the cell whose site is nearest it, by the same
// rule that assigns raster samples to cells (package cells' Locator, ties
// to the lower id). When that cell is land the hotspot is a volcano and
// the cell gets the volcano flag; a peak in the sea, or on the rim, makes
// no volcano (possible, not forced). A plateaus cell whose site is within
// volcanic_radius_km of a volcano's peak, by wrapped distance (topo
// Cylinder.Distance), becomes volcanic highlands; hotspots that are not
// volcanoes make none.
//
// # Depth
//
// A salt-water cell's sea steps are its breadth-first distance in cell
// steps from the nearest cell that is not salt water (land, and later fresh
// water), through playable salt water, over cell adjacency (mesh
// Cell.Neighbors). Two cells that meet only at the 4-way corner of a
// collapsed short edge are not neighbors, so a step never crosses a
// corner. The search does not enter rim cells: they are impassable, and a
// path along the ice is not a sea route. The band is shallow up to
// shallow_max_cells steps, open up to open_max_cells, and deep beyond;
// salt water that no land reaches (a world with no land) is deep, as in
// hmz2ter.
//
// The depth bands are play rules, so they count cell steps (DESIGN.md,
// "Units and sizing"); the defaults (3 and 8) are retuned from hmz2ter's
// 12 and 19 (see config.DefaultClassify).
//
// # Thresholds
//
// The landform thresholds are config (group classify), with defaults
// retuned from hmz2ter's DEM values for relief over a whole province of
// synthetic terrain; config.DefaultClassify gives the measurements behind
// them.
//
// # Renders
//
// LandformRender, the stage render, fills each land cell by its landform
// on a categorical palette (straw flats, green plains and rolling plains,
// tan hills, dark brown mountains, ochre plateaus, brick-red volcanic
// highlands), salt water in three blues by depth band (lighter nearer
// land), and the rim as ice, over mesh.CellRender, and marks each volcano
// cell with a black disc in a white ring at its site.
//
// # Determinism
//
// Classification compares values and counts steps. Its one computed
// quantity, the distance from a site to a peak, comes from topo, which
// rounds each product (package fmath); a cell's height above sea level is
// a single subtraction. The breadth-first search visits cells in id order
// and neighbors in their stored order. Result.AppendBinary gives the
// canonical encoding the golden hashes cover, and TestNoFusedMultiplyAdd
// checks the compiled code.
package classify
