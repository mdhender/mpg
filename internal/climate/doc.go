// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package climate computes temperature, precipitation, evaporation, and runoff.
//
// Pipeline stage 7 (DESIGN.md, "Pipeline" and "Climate coupling") works on
// the raster with the cells' land and water drawn onto it. It builds that
// cell mask, the cell temperatures, and the precipitation, potential
// evapotranspiration, runoff and aridity, averaged into each cell. Basins
// (stage 8) and rivers (stage 10) will consume the runoff and evaporation,
// and the biomes (stage 11) the temperature, precipitation and aridity.
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
// # Precipitation
//
// DESIGN.md keeps a stylized precipitation with prevailing winds and rain
// shadows. The model is hmz2bio's
// (/Users/wraith/Software/mdhender/tpty/hmz2bio: climate.go, Profile,
// Moisture, Fan and Climate; rules.go, DefaultRules, Wind and WindOffsets;
// README.md, "Precipitation" and "Moisture and lift"), computed for every
// raster sample instead of every hex, on the cylinder, pole to pole. A
// sample's annual precipitation in mm is
//
//	P = W(φ′) · v · (s(φ′) + (1 − s(φ′)) · m · (1 + g · lift / 1000))
//
// where:
//
//   - φ′ is the sample's latitude in degrees, signed north positive,
//     90·(1 − 2y/H), shifted by the band jitter J·n_j (config
//     climate.band_jitter_deg, default 4°, times fractal noise n_j in
//     [−1, 1] of band_jitter_octaves octaves from band_jitter_wavelength_km,
//     default 3 from 2,000 km) and clamped to [−90°, 90°]; W and s read
//     |φ′|. The jitter makes the latitude bands waver with longitude;
//     temperature does not use it. Pole to pole is only about 1,130 km on
//     the default world, so a degree is about 6 km and the jitter a few
//     cells.
//   - W is the windward-coast table climate.windward_precip_mm, hmz2bio's:
//     3,100 mm at 0°, 3,300 at 5°, 2,700 at 10°, 2,000 at 15°, 1,400 at
//     20°, 1,000 at 25°, 850 at 30° (the subtropical dry belt), 1,000 at
//     35°, 1,250 at 40°, 1,400 at 45° and 50°, 1,250 at 55°, 1,000 at 60°,
//     500 at 70°, 250 at 80°, 150 at 90°.
//   - s is the convective share climate.convective_share, the part that
//     falls whatever the wind does (hmz2bio: 0.5 at 0°, 0.45 at 10°, 0.3 at
//     20°, 0.2 from 30°). Tables are straight lines between their points,
//     held flat beyond the last.
//   - v = 1 + a·n_p is the periodic variability: fractal noise n_p in
//     [−1, 1] (precip_noise_octaves, default 4, from
//     precip_noise_wavelength_km, default 800 km) at amplitude a
//     (precip_noise_amp, default 0.15, below 1 so P stays positive).
//   - m is the moisture the prevailing winds bring and lift the height they
//     climb onto the sample (below); g is climate.lift_gain_per_km, 1.5.
//
// A sample of an ocean cell (the mask: rim and flooded ocean) has
// P = W(φ′)·v, moisture 1 and lift 0.
//
// # Winds
//
// The winds blow from fixed bearings by latitude band, given for the
// northern hemisphere and mirrored north–south (b → 180° − b) in the south:
// the trades from 60° up to 27°, the westerlies from 255° from 33° to 57°,
// and the polar easterlies from 60° beyond 63° (trades_from_deg,
// westerlies_from_deg, polar_from_deg, trades_max_lat_deg,
// westerlies_min_lat_deg, westerlies_max_lat_deg, polar_min_lat_deg). In
// the gaps the two neighboring winds are blended linearly, and across
// ±equator_blend_deg (5°) of the equator the two hemispheres' winds are
// blended linearly too, so neither the band edges nor the equator make a
// seam. The trades and westerlies are hmz2bio's; the polar band and the
// equator blend are mpg's, since hmz2bio's map spanned 7–27°N. Blending
// weighs each wind's moisture and lift, never its bearing. At most two
// winds act on a sample with the defaults, each weighted; the weights sum
// to 1.
//
// # Moisture: the upwind trace
//
// Each wind is traced along a fan of wind_rays rays (5) spread evenly over
// ±wind_spread_deg (20°) about its bearing (hmz2bio's fan: one ray made
// straight stripes downwind of every peak). A ray starts at the sample and
// steps upwind step_km (5 km) at a time, at most ⌊reach_km/step_km⌋ steps
// (1,000 km: 200 steps). Each step finds the raster sample containing the
// point, x wrapped onto the cylinder, and reads that sample's cell. The
// trace stops at the first point in an ocean cell, or off the map's north
// or south edge (which the rim makes unreachable in practice): there the
// air leaves the sea saturated. If it reaches no sea, the air is taken to
// start from the sea at 0 m one step beyond its last point, as in hmz2bio.
// Lakes do not yet recharge the air (the climate's first pass has none).
//
// Every height the trace reads is a cell's height above sea level
// (Result.HeightM: the cell altitude above the sea, 0 for water and for
// basin floors below it), so the orographic rules follow DESIGN.md's
// "Altitude is the only height", as hmz2bio reads its hexes' medians; the
// raster elevation is never read.
//
// Along the profile (the sea, the points upwind farthest first, then the
// sample), the air keeps exp(−step/rainout − rise/orographic) of its
// moisture on each of its n steps, rise being the height gained toward the
// sample (0 descending). So
//
//	m_ray = exp(−n·step/rainout − Σ rise / orographic)
//
// with rainout_km 500 and orographic_m 1,200 (hmz2bio's 800 and 1,500 were
// tuned in S26: with them lee sides kept half the windward rain and almost
// no land was dry). Air that crossed a range arrives dry: the rain shadow.
// lift_ray is the sample's height minus the lowest point within
// round(lift_window_km/step_km) steps upwind (30 km: 6 steps), the sea's
// 0 m included, or 0. A wind's moisture and lift are the means over its
// rays, summed in ray order; the sample's m and lift are the weighted sums
// over its winds, northern winds first, each band in latitude order.
//
// Each sample is a pure function of its position, the mask, the heights
// and the seed: there is no sweep and no traversal order, so the seam
// needs no special handling, and rows are computed on GOMAXPROCS
// goroutines, each writing only its own row. Per sample that is at most
// 2 winds × 5 rays × 200 steps; the default world (1268 × 567 samples)
// takes about 0.1 s on 8 cores, 0.5 s on one.
//
// # Aggregation
//
// A cell's precipitation, moisture and lift (Result.Precipitation,
// Moisture, LiftM) are the means of its samples' values, summed in sample
// storage order. A cell with no samples takes the values at its site.
// Result.RasterPrecipitation keeps every sample's precipitation.
//
// # Evaporation, runoff and aridity
//
// These are per cell, from the cell's temperature and mean precipitation.
//
//   - Potential evapotranspiration follows Holdridge: PET = 58.93 mm per °C
//     (pet_mm_per_c) of biotemperature, which without seasons is the mean
//     annual temperature clamped to [0, 30] °C (biotemp_max_c): 0 at or
//     below freezing, 589 mm at 10 °C, 1,591 mm at 27 °C. Thornthwaite on
//     a constant temperature gives nearly the same (584 and 1,695 mm).
//     hmz2bio has no evaporation.
//   - Runoff follows Budyko's (1974) curve: with the dryness index
//     φ = PET/P, actual evaporation is E = P·√(φ·tanh(1/φ)·(1 − e^(−φ))),
//     the geometric mean of Schreiber's and Ol'dekop's curves, and runoff
//     is R = P − E, never below 0. With PET = 0 everything runs off; beyond
//     φ = 10⁶ nothing does. Unlike max(P − PET, 0), it leaves semi-arid
//     land some runoff for the basins.
//   - The aridity index is P/PET (UNEP), capped at 10 (AridityCap), and 10
//     where PET is 0 (frozen land is not dry). Its classes (AridityOf) are
//     hyper-arid below 0.05, arid below 0.2, semi-arid below 0.5, dry
//     sub-humid below 0.65, and humid. The biomes will also have hmz2bio's
//     Köppen dryness limit, 20·T + 280 mm, from T and P.
//
// On the default world the land's precipitation is about 330 / 1,040 /
// 2,490 mm at p5 / p50 / p95, its PET 490 / 1,270 / 1,550 mm and its
// runoff p50 about 290 mm. Windward coasts get about 0.98 of the windward
// table at their latitude, lee sides about 0.43 and interiors about 0.49
// (TestWetWindwardDryLee).
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
// in blue and land in tan. The other variants are per cell, on fixed
// scales, with the coasts inked: "precip" (PrecipRender, every cell, so the
// latitude bands show over the sea, brown at 0 mm to dark teal at
// 3,500 mm), "moisture" (MoistureRender, the land's moisture, which shows
// the windward coasts and rain shadows directly), "pet" (PETRender, every
// cell, white at 0 to dark red at 1,800 mm), "runoff" (RunoffRender, the
// land's, 0 to 1,750 mm) and "aridity" (AridityRender, the land's UNEP
// class). Water is flat in the land-only renders.
//
// # Determinism
//
// The stage draws no random numbers. Its noise is keyed by
// seed.Derive(world, "climate", Version), with each field on its own
// stream, and depends only on the coordinates. Every product that reaches
// a result is rounded explicitly (package fmath), transcendentals are
// fmath's (Exp, Sincos), and TestNoFusedMultiplyAdd checks the compiled
// code; the rest are correctly rounded operations (division, sqrt).
// Samples are independent and each row's results are written by one
// goroutine, so the result does not depend on the worker count
// (TestWorkers); every reduction (ray, wind, sample-to-cell sums) runs
// sequentially in a fixed order. Cells and samples are visited in id and
// storage order. No result is NaN or infinite: P > 0 by validation, and
// the PET = 0 and vanishing-P cases are explicit. Result.AppendBinary gives
// the canonical encoding the golden hashes cover, raster precipitation
// included.
package climate
