// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package config resolves, defaults, validates, and migrates the generator
// configuration written to config.json.
//
// # The file
//
// config.json is the complete resolved input to one run. It holds every
// input, with every default written out, and the sizes derived from them
// (province area, playable cells, world width and height, rim and falloff in
// km). It never holds outcomes such as sea level or achieved counts; those go
// in world.json. It holds no timestamps and no paths.
//
// The seed is a uint64 written as a decimal string, so that JSON readers that
// parse numbers as doubles cannot lose bits.
//
// # Life cycle
//
// Default returns the inputs' defaults with the derived fields zero. Callers
// (and, later, CLI flags) set inputs on the struct, then call Resolve, which
// validates the inputs and fills the derived fields. Encode writes the
// canonical bytes; Hash is the SHA-256 of those bytes.
//
// Decode reads a file strictly: the schema version must be present and
// supported, unknown and duplicate fields are errors (naming the field and
// suggesting the closest known one), and nothing may follow the object.
// Fields the file leaves out keep their defaults, so a hand-written partial
// config is accepted. Decode resolves what it reads.
//
// # Derived fields
//
// A derived field that is zero is computed. A derived field that is present
// must equal, bit for bit, what the inputs give; otherwise Resolve fails,
// because the file was edited inconsistently. Delete the derived field (or
// call ClearDerived after changing inputs in code) to have it recomputed. A
// resolved file is therefore a fixed point: decoding and re-encoding it gives
// the same bytes.
//
// # Sizing
//
// The province area is A = (√3/2)·d², where d is province.hex_flat_to_flat_mi
// converted to km with the international mile (1.609344 km exactly). The
// playable cell count is round(N / f), the playable area that count times A,
// and the playable band's height and width follow from the aspect ratio:
//
//	playable_height = sqrt(playable_area / aspect)
//	width_km        = aspect × playable_height
//	height_km       = playable_height + 2 × rim_km
//
// width_km is computed from playable_height directly rather than from
// height_km − 2 × rim_km, which is the same quantity without the extra
// rounding. The falloff band lies inside the playable height.
//
// The rim and falloff widths are configured in cells, because they are play
// rules (DESIGN.md: "rules about play use cell steps"). They are converted to
// km with the side of a square of area A, √A ≈ 8.99 km per cell:
// rim_km = rim.cells × √A and falloff_km = rim.falloff_cells × √A.
//
// Every product that could feed an addition is rounded explicitly (package
// fmath), so the derived values have the same bits on every architecture.
//
// # Layout
//
// The layout group selects a preset (pangaea, continents, archipelago,
// islands, or custom) and writes out the parameters of every preset, so
// switching layout.preset needs no other edit. The unused presets'
// parameters do not affect the run, but they are part of the file and so of
// the config hash. Seeded presets size their landmasses relative to the land
// target (N × A), so they keep their look at any world size; custom
// attractors and repulsors are explicit, in world km, and are checked against
// the resolved world (attractors must lie outside the rim and its falloff).
// Their lists are written as [] when empty. See package layout for how the
// parameters are used.
//
// # Elevation
//
// The elevation group sets the bedrock heightmap: the land height and sea
// depth per unit of continental signal (relief_scale_m, ocean_depth_m), the
// datum shift's bound and the bias flattening, the continental fBm, the
// domain warp, the ridged mountain chains and their belts, and the polar
// falloff (its signal pull, jitter, ceiling, depth, and taper). Lengths are
// km and heights meters (DESIGN.md: "rules about the ground use physical
// units"). See package elevation for the formula.
//
// # Mesh
//
// The mesh group sets the province mesh: the site placement (a jittered
// grid, with its jitter as a fraction of the grid box), the Lloyd passes,
// the short-edge collapse threshold, the cell-area bounds of the mesh checks
// (as multiples of A), and the degree cap. The collapse threshold is
// configured as a fraction of √A, like the rim widths, and converted to km:
// mesh.min_edge_km = mesh.min_edge_fraction × √A (derived), about 2.7 km at
// the default 0.3. See package mesh.
//
// # Climate
//
// The climate group sets the temperature model (DESIGN.md: "Latitude plus
// lapse-rate temperature"): the mean annual sea-level temperature by
// latitude, as points (lat_deg, temp_c) joined by straight lines, with 0°
// at the equator and 90° at the map's north and south edges (the latitude
// proxy's ±1), and the lapse rate in °C per km of altitude above sea level.
// The curve must run from 0° to 90° with latitudes increasing and
// temperatures not increasing, and the pole must be colder than the
// equator, so the rim is always the coldest water. DefaultClimate is
// hmz2bio's model. See package climate.
//
// # Classify
//
// The classify group sets the landform and depth rules (DESIGN.md,
// "Classification"): the relief breaks between flats, plains, rolling
// plains, hills and mountains, which must not decrease; the plateau rule's
// height above sea level and relief limit; the volcanic-highlands radius in
// km; and the depth bands in cell steps. The names follow hmz2ter's rules,
// and DefaultClassify records how the defaults were retuned from hmz2ter's.
// See package classify.
//
// # Versioning
//
// The file carries "schema": 1. A file without a schema, or with another
// version, is rejected: an older schema needs an explicit migration and must
// never silently adopt this version's defaults.
package config
