// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package climate computes temperature, precipitation, evaporation, and runoff.
//
// Pipeline stage 7 (DESIGN.md, "Pipeline" and "Climate coupling") works on
// the raster with the cells' land and water drawn onto it. So far it builds
// that cell mask and the cell temperatures; precipitation, potential
// evaporation, runoff and aridity come next.
//
// # Cell mask
//
// The climate is computed once from the ocean-only mask: the sea level
// stage's flood (package cells, Classify). A cell is ocean to the climate
// (Result.Ocean) when it is a rim cell, which counts as deep salt water, or
// a cell the flood from the rim reached. Everything else is land, dry basin
// floors included: the basin stage has not run, and a basin floor is dry
// land until it does. Result.Mask draws this onto the raster: each raster
// sample, in storage order (rows north to south, columns west to east),
// takes its cell's flag through cells.Stats.Owner, the cell whose site is
// nearest the sample. Climate fields computed on the raster will be
// aggregated back to cells as the mean over each cell's samples, in storage
// order.
//
// # Temperature
//
// DESIGN.md keeps hmz2bio's model: latitude plus lapse-rate temperature, no
// axial tilt, no seasons. A cell's mean annual temperature in °C is
//
//	T = T₀(φ) − Γ · h / 1000
//
// where:
//
//   - φ is the cell's latitude in degrees from the equator, north and south
//     alike: 90·|lat| for topo's latitude proxy lat = 1 − 2·y/H at the
//     cell's site. The proxy's ±1, the map's north and south edges inside
//     the rim, are the poles; the equator is the middle row.
//   - T₀ is the sea-level curve, config climate.sea_level_temp_c: points
//     (lat_deg, temp_c) from 0° to 90° joined by straight lines, never
//     warmer poleward (config validation), so the equator is the warmest
//     place at sea level and the poles the coldest. The default is
//     hmz2bio's (/Users/wraith/Software/mdhender/tpty/hmz2bio/rules.go):
//     27 °C to 10°, 26.5 at 15°, 25 at 20°, 22.5 at 25°, 20 at 30°, 17 at
//     35°, 14.5 at 40°, 12 at 45°, 8 at 50°, 4 at 55°, 0 at 60°, −8 at 70°,
//     −16 at 80°, −22 at 90°. Each segment's value is clamped between its
//     end points, so rounding can never make it warmer poleward.
//   - Γ is the lapse rate, config climate.lapse_rate_c_per_km, default
//     6.5 °C per km (the standard atmosphere; hmz2bio's), in [0, 20].
//   - h is the cell's height above sea level in meters (Result.HeightM):
//     its altitude (cells.Stats.Altitude, the median of its samples, the
//     only height) minus the sea level stage's level for a land cell above
//     the sea; 0 for every other cell, ocean and rim cells and dry basin
//     floors at or below the sea alike. Water and sunken land take the
//     sea-level temperature of their latitude: water's surface is at sea
//     level, and nothing warms a basin floor below it.
//
// This follows hmz2bio's climate.go (T = SeaLevelTemp.At(lat) −
// LapseRate·elevation/1000, with sea at 0 m), with hmz2bio's raster rows
// replaced by the cylinder's latitude proxy and its hex median elevation by
// the cell altitude.
//
// The rim lies poleward of every playable cell, so with T₀ never warmer
// poleward every rim cell is at least as cold as every playable cell at sea
// level, and rim cells, being water, take exactly that: the rim is the
// coldest water on the map, and colder than any playable cell at sea level.
// Only altitude can take a playable cell below the rim's temperature, and
// it can: high land near a pole is colder than the open polar sea (seed 5,
// portrait, pangaea has land at about −25 °C against a rim no colder than
// −21.8 °C), as a high polar plateau is on Earth. The design's "climate
// extremes fall in the rim" holds at sea level, not above it.
//
// # Renders
//
// TemperatureRender, the stage render, fills every cell, rim cells
// included, by its temperature on TemperatureRamp, a fixed diverging scale
// from deep violet (−40 °C) through blue to white at freezing and on through
// yellow and orange to dark red (30 °C), so sweep tiles compare; the rim
// shows as the coldest band, not as the ice sheet. Coasts (edges between an
// ocean cell and a land cell) are drawn in dark ink over mesh.CellRender's
// outlines and ice front, so altitude cooling reads on the land. Variant
// "mask", MaskRender, draws the mask at one pixel per raster sample, ocean
// in blue and land in tan.
//
// # Determinism
//
// The stage draws no random numbers. Every product that reaches a result is
// rounded explicitly (package fmath), and TestNoFusedMultiplyAdd checks the
// compiled code; the rest are correctly rounded operations. Cells and
// samples are visited in id and storage order. Result.AppendBinary gives the
// canonical encoding the golden hashes cover.
package climate
