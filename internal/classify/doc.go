// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package classify assigns landform, depth, surface, and biome.
//
// Pipeline stage 11 (DESIGN.md, "Classification") gives every cell a
// landform, every salt-water cell a depth band, and every land cell
// holding a volcano's peak the volcano flag. The vocabulary and the rules
// are hmz2ter's (maloquacious/hmz2ter, rules.go Rules.Landform and
// Rules.Depth, terrain.go Classify), applied per cell. Result.Cover then
// gives every land cell a biome and the cells their surfaces, by hmz2bio's
// table (see "Biomes and surfaces").
//
// # Landform
//
// Every height rule uses the cell's altitude (package cells: the median of
// its samples) measured from the chosen sea level, where hmz2ter used a
// hex's median elevation above 0 m; relief is the cell's p95 − p5. A land
// cell (the land-target stage's: its flood's land, the land candidates and
// the basin floors, less the lake cells of its water balance) is
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
// A lake cell is water by its lake's size (DESIGN.md, "Classification"):
// fresh water for a lake (fewer than basin.inland_sea_min_cells cells),
// salt water for an inland sea; salinity is world.json's salt flag, not the
// landform. Classify without lakes (nil) leaves every basin floor land.
// Every other playable cell is salt water: the ocean. Rim cells are salt
// water too, as the generator treats them (DESIGN.md, "Rim"), with depth
// deep; their rim and impassable flags override this for the game.
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
// steps from the nearest cell that is not salt water (land and lakes),
// through playable salt water (the ocean and the inland seas), over cell adjacency (mesh
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
// # Biomes and surfaces
//
// Cover reads the land-target stage's land, lakes and playas, its final
// climate pass (temperature, precipitation, aridity index and lift per
// cell), the sea-level temperature at each cell's latitude, and the river
// stage's edge classes. Its table is Go constants (DefaultBiomeRules), not
// config; BiomeTableVersion names it ("hmz2bio-0.3.0+mpg.1") and world.json
// records it.
//
// Seasons. The climate has none (DESIGN.md, "Climate"). hmz2bio's rules
// read the warmest and coldest months, so the table derives them as
// hmz2bio does (tpty/hmz2bio, climate.go Climate): the annual range is
// 1 + 0.33·|latitude in degrees| °C, split evenly about the mean annual
// temperature T. They exist only inside the table.
//
// Biome (BiomeRules.Biome; the first row that applies wins), with P the
// annual precipitation and Köppen's aridity limit 20·T + 280 mm:
//
//   - warmest month below 0 °C: permanent ice (biome clear) when the
//     snowfall is at least 300 mm, else polar desert, so cold dry land
//     stays bare. Snowfall is P times the share of the year below
//     freezing, a straight ramp from the coldest month to the warmest:
//     clamp((range/2 − T)/range, 0, 1);
//   - warmest month below 10 °C: tundra where the sea-level warmest month
//     at the latitude is below 10 °C too, alpine where only height makes
//     it cold;
//   - then hmz2bio's rows unchanged (rules.go Rules.Biome; README
//     "Biomes"): desert below half the aridity limit, scrubland (T ≥ 18
//     °C) or steppe below it, cloud forest and tropical montane forest,
//     tropical rainforest, tropical dry forest and savanna (T ≥ 18 °C),
//     boreal forest (T < 4 °C), temperate rainforest (P ≥ 2,000 mm),
//     grassland (below 1.5 × the aridity limit), and temperate forest.
//
// A playa is never permanent ice. Biome is clear exactly when the surface
// is glacier or ice field.
//
// Land ice. Each connected body of permanent ice (cell adjacency, found
// breadth first in id order) is split: a mountain cell is glacier; a
// non-mountain cell is ice field when the body has at least
// IceFieldMinCells (5) non-mountain cells, or no mountain cell at all; and
// glacier otherwise, a tongue reaching down from the mountains.
//
// Wetlands. A land cell that is not permanent ice has wetness signals
// (Result.Wetness): WetRiver, a flats cell beside a major-river edge;
// WetShore, a flats or plains cell touching a lake or inland sea; and
// WetSurplus, a flats cell whose aridity index is at least 2. A cell with
// any signal is a wetland, its kind in hmz2bio's order (classify.go
// Surface): salt flats when dry (P below the aridity limit), mangroves on
// an ocean coast whose coldest month is at least 15 °C, bogs below
// 10 °C, swamps in a forest biome, marshes otherwise. Every playa is salt
// flats, whatever its landform.
//
// Pack ice. Playable water (ocean, inland seas, lakes; lakes and inland
// seas at their surface's temperature) whose warmest month is below 0 °C
// is pack ice. Rim cells get no biome and no surface: they are the ice
// sheet.
//
// Measured on 12 default-size worlds (S32): desert 0–27% of land (Köppen;
// UNEP's arid classes are nearly absent, since the convective share floors
// P), polar desert 0–177 cells, glaciers 0–109 cells (almost all on
// mountains), ice fields 0–98, wetlands 1.7–8.8% of land, pack ice 13–17%
// of playable water. Temperate rainforest needs 2,000 mm away from the
// tropics and does not occur; possible, not forced.
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
// cell with a black disc in a white ring at its site. Its variants:
// "biome" (BiomeRender: land by biome in hmz2bio's preview colors), "surface"
// (SurfaceRender: glaciers, ice fields, pack ice and wetlands in their
// colors, polar desert in rust, other land pale) and "wetness"
// (WetnessRender: each land cell by its wetness signals, with the major
// rivers). The player map uses the same biome and surface colors.
//
// # Determinism
//
// Classification compares values and counts steps; the biome table's
// sums of products (the annual range, the aridity limit) round each
// product first (fmath.MulAdd). Its one computed
// quantity, the distance from a site to a peak, comes from topo, which
// rounds each product (package fmath); a cell's height above sea level is
// a single subtraction. The breadth-first search visits cells in id order
// and neighbors in their stored order. Result.AppendBinary gives the
// canonical encoding the golden hashes cover, and TestNoFusedMultiplyAdd
// checks the compiled code.
package classify
