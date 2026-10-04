# mpg — Noise-Based Fantasy World Generator

Go module: `github.com/mdhender/mpg`  
Design snapshot: 2026-10-04  
Status: agreed direction and proposed implementation design; no implementation implied.

## Purpose

Build a deterministic fantasy map generator that starts with a random seed, a desired number of land cells, and an aspect ratio. Produce attractive, varied geography from noise, including meaningful rivers, lakes, inland seas, glaciers, ice fields, and dry interior basins. Save a fully resolved JSON configuration for experimentation and JSON world data that a separate renderer can read in blocks.

The visual inspiration is the Panama DEM-based `github.com/maloquacious/hmz*` stages and `github.com/maloquacious/wg`. These are references for appearance, not required data sources or dependencies. This document does not assume those repositories have been inspected.

The goal is an interesting, plausible game world rather than a scientific Earth simulator or an Earth-sized dataset.

## Agreed decisions

- Implement in Go, using `math/rand/v2` when pseudorandom draws are needed.
- Derive independent seeds for named stages; avoid a global sequential random stream.
- Generate elevation with layered noise and domain warping.
- Wrap east–west; never wrap north–south. Use a cylindrical grid with bounded polar edges.
- Use latitude and altitude as proxies for temperature. Assume no axial tilt and an east–west solar path; seasonal simulation is unnecessary.
- Exclude erosion simulation and plate tectonics.
- Preserve topographic depressions. A closed basin may contain fresh or salt water, a seasonal lake, a playa, or an interior desert.
- Keep geography, climate, biomes, and rendering distinct.
- Provide separate generator and renderer executables.
- Save resolved configuration and world data as JSON.
- Support block/window rendering without regenerating the world.
- Each requested land cell denotes 100 km² of final dry land area. It does not denote a generation sample.
- Sampling resolution is configurable, with automatic selection by the generator as the default. Save the resolved sampling choice.
- Final dry land area must be within 1% of the requested area; explicitly report inability to meet that tolerance.
- The future game-data export step aggregates samples into 10 km × 10 km game cells; the game does not need sampling metadata.

## Proposed defaults

These choices complete the design without requiring additional input. They are adjustable defaults, not additional user requirements.

| Choice | Initial default | Reason |
|---|---|---|
| Generation samples | Square, rectangular grid | Simple arrays, image rendering, and neighborhood operations |
| Land fraction for sizing | 0.30 | Supplies the missing quantity needed to derive total area |
| Chunk dimensions | 64 × 64 samples | Practical unit for rectangular rendering |
| Continental noise | Seeded 3-D gradient noise, layered into fBm | Seamless cylindrical sampling and manageable implementation |
| Drainage neighbors | Eight, with diagonal distance weighting | Less directional bias than four-neighbor drainage |
| Water connectivity | Four-neighbor | Corner-touching water does not create a navigable connection |
| Equatorial/polar temperature | 30°C / −25°C | Useful initial climate range |
| Atmospheric lapse rate | 6.5°C per km | Simple altitude cooling parameter |
| Output | Manifest plus separate JSON chunk files | Real block access without parsing one enormous JSON file |

Choose and pin the exact noise implementation during the first implementation milestone. Its algorithm/version becomes part of reproducibility metadata. Do not rely on an unspecified library's evolving defaults.

## Coordinates and topology

Coordinates are `(x, y)`, with origin at the northwest corner. `x` increases east and `y` increases south. Rows are stored in row-major order. Valid stored coordinates satisfy `0 <= x < width` and `0 <= y < height`.

Normalize longitude with positive modulo. North/south out-of-range neighbors are absent, not wrapped, clamped duplicates, or automatic ocean outlets. All connectivity, distance, wind, drainage, and renderer-window operations share the same topology helper.

Sample-center latitude proxy:

```text
latitude = 1 - 2 * (y + 0.5) / height
```

This gives northern and southern polar regions without claiming an accurate spherical projection. Sample areas are constant; rows do not shrink toward the poles. Traveling across a polar edge is unsupported.

For periodic noise, sample a cylinder:

```text
theta = 2*pi*(x + 0.5)/width
sample = (R*cos(theta), R*sin(theta), S*latitude)
```

Choose `R` and `S` from the spatial scale so longitudinal and vertical detail have comparable cell-scale wavelengths. Domain warps and all spatial perturbations must themselves be periodic. Merely matching the first and last columns is insufficient: they are adjacent cell centers, not duplicate coordinates.

## Physical scale, samples, and game cells

Keep three concepts separate:

| Concept | Meaning |
|---|---|
| Requested land cell | A fixed area unit of 100 km² |
| Generation sample | A square of generator-selected physical size |
| Game cell | A 10 km × 10 km unit produced by a future game-data export step |

A request for 10,000 land cells means **1,000,000 km² of final dry land**, independently of sampling resolution. Changing the number of samples must never change this physical target.

Configure `sampling.mode` as `auto` by default, or `explicit` with `samples_per_game_cell_side`. The automatic policy selects an integer number of samples per 10 km side using algorithm requirements, useful detail, and a documented resource budget. Candidate values can include 1, 2, 5, and 10; no particular value is the default requirement. Sample spacing is `10000 / samples_per_game_cell_side` meters. Persist the selected value, spacing, and policy version. Replaying resolved configuration uses the saved resolution rather than choosing again.

| Samples per 10 km side | Sample spacing | Sample area | Samples equivalent to 1,000,000 km² of land |
|---|---|---|---:|
| 1 | 10 km | 100 km² | 10,000 |
| 2 | 5 km | 25 km² | 40,000 |
| 5 | 2 km | 4 km² | 250,000 |
| 10 | 1 km | 1 km² | 1,000,000 |

All generation arrays, topology, chunk dimensions, and coordinate indices refer to **samples**. Distances, slope, area, and hydrology use resolved sample spacing. World width and height in meters are sample dimensions multiplied by sample spacing. Samples have constant area at every latitude.

Altitude-sensitive climate, basin analysis, drainage, and cryosphere operate on sample terrain before any game-cell aggregation. Elevation remains in meters independently of horizontal spacing.

The future game-data render/export stage owns averaging, categorical summaries, and feature connectivity when creating 10 km × 10 km game cells. Its detailed aggregation rules are deliberately not designed here. The game receives physical game-cell dimensions and summarized geography, without needing to know the generation sample size. Image pixels are a separate rendering choice.

## Inputs and size resolution

Initial interface:

```sh
mpg generate --seed 8675309 --land-cells 1000000 --aspect cinematic --output worlds/example
mpg generate --config worlds/example/config.json --output worlds/tweaked
mpg-render --world worlds/example/map.json --layer geography --output geography.png
mpg-render --world worlds/example/map.json --layer biome --window 1200,300,512,512 --output detail.png
```

Names resolve as follows. Explicit ratios accept positive finite numbers on both sides of a colon.

| Name | Ratio |
|---|---|
| square | 1:1 |
| portrait | 3:4 |
| landscape | 4:3 |
| widescreen | 16:9 |
| cinematic | 2.39:1 |

Use **cinematic** for cinematic widescreen, avoiding an ambiguous change to the conventional 16:9 meaning of **widescreen**. Accept `16:9`, `2.39:1`, and other numeric ratios directly.

For requested land-cell area units `L`, sizing fraction `f`, aspect `a = width/height`, and sample area `A_s` in km²:

```text
target_land_area_km2 = L * 100
target_world_area_km2 ≈ target_land_area_km2 / f
target_sample_count ≈ target_world_area_km2 / A_s
height_samples ≈ sqrt(target_sample_count / a)
width_samples ≈ a * height_samples
```

Choose sampling resolution before resolving sample dimensions. Search nearby positive integer dimensions and minimize a documented combined physical-area/aspect error, with deterministic tie-breaking. Use dimensions divisible by the selected samples-per-game-cell-side so future 10 km game cells cover the whole grid without partial cells. Do not require divisibility by chunk size; edge chunks may be smaller. Check overflow and resource limits using total samples, not requested land cells.

Explicit width and height in an edited configuration are sample counts and take precedence over automatic sizing. Validate their physical area, aspect, divisibility, and ability to contain the requested land area. Store requested and actual aspect ratios.

## Final land-area contract

`--land-cells` expresses **100 km² per requested unit**. It does not request a number of above-sea-level samples, continental cells, or final game cells classified as land.

Count as land:
- Dry ground, including dry closed basins below ocean sea level.
- Ground covered by glaciers, snowfields, or ice fields.

Exclude from land:
- Ocean, lakes, and inland seas.
- Other explicitly modeled standing-water surfaces; seasonal water uses the model's documented representative annual state.
- Explicitly modeled river water area, if present. Rivers represented only as line overlays do not subtract area.
- Ice over ocean or standing water; freezing water does not turn it into land.

Calculate final area after basin water balance and inland-water classification. In v1 each sample contributes its full physical area according to its authoritative land/water surface class; report this discretization explicitly. A future fractional-coverage model may refine area estimates without changing the area-request meaning.

**Acceptance tolerance: absolute relative error <= 1%.** Report requested area, achieved area, signed area error, and relative error. If resolution or geographic connectivity prevents meeting tolerance, report an unmet target explicitly rather than claiming success. Automatically choose resolution fine enough to make the tolerance feasible for the requested size, subject to resource limits; never silently change the requested area to fit resources.

Sea-level quantile selection is an initial estimate only. It cannot guarantee final dry area because ocean connectivity and inland water matter. Evaluate candidate sea levels by recomputing affected ocean, basin, climate/water-balance, and drainage stages with unchanged stage seeds. Use a bounded, deterministic search, retaining the best measured candidate. Do not assume final dry area varies monotonically: basin connections can cause jumps and water balance can change inland water.

Save the search policy, iteration budget, tolerance, selected sea level, achieved area, and termination reason. Input policy belongs in resolved configuration; calculated outcomes belong in the manifest. Do not move isolated water/land samples merely to force the target.

Require a nonempty primary ocean in v1. Choose a deterministic ocean seed from the global minimum elevation, ties by sample index. Disconnected low depressions are inland basins and may be dry. Generation and area checks retain this distinction.

## Generation pipeline

| Stage | Produces | Key constraint |
|---|---|---|
| Resolve configuration | Target area, sampling resolution, sample dimensions, units, algorithms, defaults | Save defaults and automatic resolution decisions |
| Elevation | Bedrock elevation in meters | Periodic east–west; no erosion or tectonics |
| Ocean selection | Candidate sea level, ocean mask, coast | Initial estimate; final dry area checked after inland water |
| Basin analysis | Basin hierarchy, spill levels, catchments | Preserve original elevations |
| Baseline climate | Temperature and precipitation | Latitude, altitude, winds, and water influence |
| Water balance | Basin water levels and dry/wet states | Lakes are outcomes, not compulsory sink filling |
| Drainage | Flow graph, discharge, river reaches | No drainage cycles; explicit closed sinks |
| Cryosphere | Snowfields, ice fields, glacier masks | Temperature and snowfall constraints |
| Biomes | Vegetation/ecological classification | Derived from climate and surface conditions |
| Final area validation | Dry land area and target error | Bounded candidate search; acceptance within 1% |
| Export | Resolved config, manifest, chunks, summaries | Image rendering never changes generation |

Climate and hydrology interact. For v1 use a deterministic bounded coupling procedure: first climate uses ocean proximity, then basin water balance determines inland water, then climate may apply one inland-water adjustment and recompute balance once. Save the pass count. This is a finite approximation, not a claim of converged atmospheric equilibrium.

### Elevation

Combine low-frequency continental noise, periodic domain warp, mid-frequency relief, optional ridged mountain noise, and small detail. Keep amplitude and wavelength parameters explicit. Convert normalized height to a documented meter scale before climate and hydrology use it. Do not interpret arbitrary noise values as physical altitude.

Mountain chains are noise-derived features; no plate model is required. Store original elevation and any hydrology working surface separately. Hydrology must not silently alter terrain to erase depressions.

### Basin analysis and water balance

Distinguish topographic basin, drainage catchment, and actual water body. Use deterministic priority-flood/depression analysis to identify spill points and nested basins. A filled working surface may help establish routing and hierarchy; it is not the rendered terrain or a declaration that every depression is wet.

For each basin, build an elevation/area/storage curve. Aggregate runoff from contributing catchments. Annual water balance uses consistent units:

```text
storage change = catchment runoff + precipitation on water
                 - evaporation from water - seepage - overflow
```

Rainfall over the catchment contributes through runoff; do not also add it wholesale as direct water-surface precipitation. Use area in square meters, annual depths in meters, and resulting volumes in cubic meters per year.

Solve a simplified equilibrium water level against the basin's area curve, bounded by its spill level. Positive surplus at the spill level becomes downstream overflow. If no stable wet level exists, classify the basin as dry or seasonal under explicit rules. Handle nested basins from upstream to downstream, merging pools when their levels reach connecting saddles. Record unresolved or model-limited states rather than hiding numerical failures.

A low basin is not automatically desert: a humid dry basin may still support vegetation. Desert biome follows aridity, while playa describes the exposed basin floor. Likewise, a closed lake is not automatically salty; use a documented salinity proxy based on endorheic status and evaporation/inflow, not a full salt-chemistry simulation.

Proposed water classifications: lake, inland sea, seasonal lake, salt lake, salt inland sea. “Inland sea” is a configurable size-based game classification, independent of salinity. Thresholds must use cell area or explicitly documented cell counts.

### Climate

Baseline temperature interpolates from equator to poles using a smooth latitude curve, then subtracts lapse-rate cooling above sea level. Below-sea-level dry terrain does not receive unlimited artificial warming; define and bound that adjustment explicitly.

Precipitation is a stylized annual model: latitude bands, smooth periodic variability, ocean moisture supply, simple prevailing winds, and orographic lifting/rain shadows. This is sufficient to create wet coasts and dry interiors without simulating global atmosphere dynamics. Distance to water alone is not a rain-shadow model.

Advection across wrapped rows uses a fixed iteration count and deterministic update order. Avoid unbounded circular scans. Store precipitation, potential evaporation, runoff, temperature, and an aridity index with explicit units.

### Rivers

Route runoff using the basin hierarchy and a deterministic acyclic flow graph. Resolve flats and equal-height neighbors with stable ordering; never introduce cycles through tie-breaking. Rivers terminate in ocean, an inland water body, or an explicit dry closed sink; polar map edges are not sinks by default.

Accumulate discharge in cubic meters per second using the annual runoff approximation. Select river channels from configurable discharge thresholds. Keep river overlays distinct from land/water surface class. Store reach connectivity and destination identifiers so the renderer can draw continuous rivers across chunk and longitude boundaries.

### Snow, glaciers, and ice fields

Permanent ice requires cold conditions and sufficient snow accumulation; temperature alone can otherwise turn cold deserts into implausible glaciers. Use an annual accumulation/melt proxy. Distinguish land ice from sea ice and standing-water ice.

V1 glaciers are a simplified geography classification, not a dynamical ice-flow simulation. If downhill glacier tongues are added, constrain extension by slope, cold conditions, accumulation, and a finite distance. Do not label all perennial snow as a flowing glacier. Ice fields and glacier-covered terrain remain separate overlays on bedrock and hydrology.

### Biomes and game terrain

Assign biomes from temperature, precipitation/aridity, and relevant surface conditions, using a versioned lookup table. Initial candidates: polar desert, tundra, boreal forest, temperate forest, temperate rainforest, grassland, steppe, desert, savanna, tropical seasonal forest, tropical rainforest, and alpine vegetation.

Wetlands require a hydrological wetness indicator, not just high rainfall. Keep ecological biome separate from landform and surface overlays. Game movement costs and production yields are future consumers, not requirements of the generator.

## Determinism and reproducibility

Derive seeds with a specified stable digest such as SHA-256 over a domain prefix, world seed, stage name, and algorithm version. Use defined byte order and length-prefixed fields. Feed two derived uint64 values into `rand.NewPCG`; never use Go's process-randomized map hashing as a seed function.

Stage names are stable strings such as `elevation`, `warp`, and `precipitation`. Coordinate noise must use a stable seed and coordinates, not random draws whose order depends on chunk scheduling.

- Fix queue, sort, traversal, and equal-value tie-breaking by stable cell index.
- Sort map keys where serialized ordering matters.
- Parallelize independent per-cell work; perform reductions in a defined order.
- Pin dependencies and store algorithm versions.
- Exclude timestamps and absolute paths from canonical content hashes.
- Do not serialize NaN or infinity.

Promise repeatability for a pinned generator/toolchain and configuration. Cross-architecture bit-identical floating-point output needs dedicated verification; do not promise it prematurely. Stage-specific seeds isolate random draws, but a change to upstream geography will legitimately change downstream hydrology and biomes.

## Configuration contract

`config.json` is the complete resolved input document. It stores requested parameters and explicit defaults, but calculated results such as final sea level belong in `map.json`. A user may override sea level explicitly by selecting a different ocean mode; that mode no longer promises the land target.

Illustrative shape; the resolved sampling choice shown is an example, not a mandatory default. Width and height count samples. Numeric tuning values and resource/search budgets are starting points, not validated recommendations:

```json
{
  "schema_version": 1,
  "algorithm_version": "mpg-v1",
  "seed": "8675309",
  "size": {
    "land_cells": 10000,
    "land_area_per_requested_cell_km2": 100,
    "target_land_area_km2": 1000000,
    "aspect": "cinematic",
    "aspect_ratio": 2.39,
    "land_fraction": 0.30,
    "dimensions_mode": "automatic",
    "width": 2820,
    "height": 1180,
    "game_cell_size_m": 10000
  },
  "sampling": {
    "mode": "auto",
    "policy_version": "resolution-v1",
    "samples_per_game_cell_side": 5,
    "sample_size_m": 2000,
    "memory_budget_mib": 2048
  },
  "topology": {"wrap_east_west": true, "wrap_north_south": false},
  "elevation": {
    "noise_algorithm": "gradient3-v1",
    "continental_wavelength_samples": 800,
    "detail_wavelength_samples": 80,
    "octaves": 6,
    "persistence": 0.5,
    "lacunarity": 2.0,
    "warp_amplitude_samples": 100,
    "relief_scale_m": 6000
  },
  "ocean": {"mode": "target_dry_land_area", "sea_level_override_m": null},
  "area_search": {
    "policy": "bounded-candidates-v1",
    "relative_tolerance": 0.01,
    "max_evaluations": 24
  },
  "climate": {
    "equator_temperature_c": 30,
    "pole_temperature_c": -25,
    "lapse_rate_c_per_km": 6.5,
    "model": "latitude-wind-v1",
    "coupling_passes": 2
  },
  "hydrology": {
    "model": "basin-balance-v1",
    "inland_sea_min_area_km2": 500,
    "river_min_discharge_m3_s": 1,
    "seepage_mm_year": 10
  },
  "cryosphere": {"model": "snow-balance-v1"},
  "biomes": {"classification": "temperature-aridity-v1"},
  "output": {"chunk_size": 64, "format": "json-chunks-v1"}
}
```

Use a decimal string for a uint64 seed so other JSON consumers do not lose precision. Validate positive sample dimensions/spacing and consistency with the 10 km game-cell size, `0 < land_fraction < 1`, finite numeric parameters, allowed enum values, and compatible versions. Unknown fields should fail with a useful message to catch misspelled tweaks. Resolved configuration must eventually include every coefficient used by each model; the example abbreviates model-specific tuning fields.

Maintain machine-readable JSON Schema files alongside Go validation. Schema checks structural constraints; Go checks cross-field constraints, resource budgets, and graph/data invariants. Loading old schema versions requires an explicit migration, never silently adopting newer defaults.

## World data and block access

Recommended output directory:

```text
worlds/example/
  config.json
  map.json
  basins.json
  rivers.json
  chunks/
    000000-000000.json
    000001-000000.json
    ...
```

`map.json` is the world manifest. One monolithic JSON object containing all chunks would still require scanning/parsing the whole file to reach an arbitrary chunk; separate chunk files provide actual block access while keeping JSON as requested. A single-file interchange archive can be added later.

Manifest contract:

| Field | Meaning |
|---|---|
| schema_version, algorithm_version | Reader and generation compatibility |
| config_sha256 | Hash of canonical resolved input |
| width, height, chunk_size | Resolved grid shape |
| sample_size_m, samples_per_game_cell_side, game_cell_size_m, topology | Resolved generation scale, 10000 m game-cell scale, and topology |
| sea_level_m | Calculated ocean surface |
| counts | Sample counts for dry surface, ocean, inland water, and ice overlays |
| land_area | Requested/achieved dry area in km², signed error, relative error, tolerance status |
| area_search | Evaluations, termination reason, and selected candidate |
| units | Units of every numeric layer |
| codebooks | Stable integer-to-label mappings for categorical layers |
| chunks | Chunk origin, actual dimensions, relative file path, content checksum |
| basins_file, rivers_file | Paths to global graph tables |

Each chunk stores its sample origin `(x, y)`, actual width/height, and parallel row-major arrays. Array index `i` resolves to `(x + i % width, y + i / width)`. Every required array has exactly `width*height` elements. Edge chunks are smaller; no padding or duplicate seam column is stored.

Numeric layers: bedrock elevation, temperature, precipitation, potential evaporation, runoff, river discharge, and slope. Categorical/reference layers: ocean mask or surface class, biome, ice class, basin ID, water-body ID, and downstream cell index. Use explicit null/sentinel conventions in the schema. A globally unique sample index is `y*world_width+x`.

Separate graphs contain basin spill/outlet references, balance summaries, water-surface levels and classifications, plus river reaches. Reference IDs are assigned in stable geographic order, never discovery order from nondeterministic workers.

Do not quantize authoritative data merely to improve file size in v1. Optional gzip JSON can follow after measurements. Generation remains a global process because sea-level selection, watersheds, and rivers depend on distant cells; chunked export is not a promise of independent local generation.

## Renderer

`mpg-render` reads saved data only. Layers: elevation, geography, temperature, precipitation, climate, biome, hydrology, and cryosphere. Define `climate` as a documented composite; retain individual numeric layers for inspection.

Support full-world images and rectangular windows. A window may cross the east–west seam and stitch the appropriate chunks. Reject windows outside north/south bounds. Read only intersecting chunk files plus relevant global metadata/graphs. Use a small halo for slope shading and feature continuity.

Keep palettes, legends, hillshade, and line widths in renderer options. Save numeric legends with units. The geography view combines landforms, water, and ice; biome and climate views retain river/coast overlays as optional context. PNG is the initial output format. Rendering settings do not change map data or configuration hashes.

## Package layout

```text
cmd/mpg/                 Generator CLI and config validation commands
cmd/mpg-render/          Renderer CLI
internal/config/         Input resolution, defaults, migrations, validation
internal/seed/           Stable stage/coordinate seed derivation
internal/grid/           Coordinates, neighborhoods, wrapped distances
internal/noise/          Pinned periodic noise and domain warping
internal/elevation/      Terrain synthesis
internal/ocean/          Ocean connectivity and sea-level selection
internal/basin/          Depression hierarchy and storage curves
internal/climate/        Temperature, precipitation, evaporation, runoff
internal/hydrology/      Water balance, drainage graph, river extraction
internal/cryosphere/     Snow/ice classifications
internal/biome/          Versioned ecological classification
internal/world/          Shared layer types and graph records
internal/storage/        Manifest/chunk JSON readers and writers
internal/render/         Palettes, windows, shading, overlays
schemas/                 config, manifest, chunk, basin, river schemas
features/                Human-readable behavior specifications
```

The future game-data export may require its own executable; image rendering and game-data aggregation are distinct consumers. Its aggregation design remains deferred.

Keep APIs internal until another project needs stable imports. Centralize units and topology rather than reproducing them in every stage.

## Invariants and acceptance checks

- Identical pinned inputs produce identical canonical data and checksums.
- Every spatial stage honors east–west wrap and bounded north/south edges.
- No extra seam column exists; longitude-shifted render windows stitch correctly.
- Requested land area equals requested land-cell units multiplied by 100 km² at every sampling resolution.
- Final dry land area is measured after inland-water classification and is within 1%, or output explicitly reports an unmet target.
- Resolution changes preserve target area; replay uses the saved resolution.
- Terrain ice counts as land; ice over water remains water.
- Surface counts sum to total cells; ice and river overlays do not double-count surfaces.
- Every basin has a valid hierarchy/outlet or is an explicit closed terminal basin.
- Drainage is acyclic; accumulated water is conserved within documented numerical tolerance.
- Standing water has one surface level per connected pool and does not exceed an unprocessed spill level.
- A dry basin can exist below ocean level without being classified as ocean.
- Cold arid terrain need not have glaciers; humid basins need not be deserts.
- Chunk coverage has no overlaps/gaps, layer lengths match dimensions, and references resolve.
- Full-image crops and equivalent window renders agree, including seam-crossing windows.
- Saved configuration regenerates without consulting current defaults.

Use small hand-built fixtures for seam connectivity, flat drainage, nested basins, overflow, dry basins, and polar boundaries. Use fixed seeds for reproducibility and representative visual inspection. Golden images help detect rendering regressions but do not establish hydrological correctness.

Optional human-readable behavior examples can live in paths such as `features/hydrology/preserve_dry_closed_basins.feature` and `features/rendering/render_across_longitude_seam.feature`. The design does not require Cucumber or a particular agent/test workflow.

## Implementation order

1. **Foundation:** module, CLIs, config/schema validation, area-based sizing, automatic/explicit sampling, units, topology, stable seed derivation. Finish when resolved inputs round-trip and boundary behavior is verified.
2. **Geography preview:** periodic elevation, ocean selection, measured preliminary physical land area, initial elevation/geography rendering. Finish when seam behavior and attractive continental shapes are inspected across several seeds/aspects.
3. **Persistent world format:** manifest, chunks, checksums, window reader and renderer. Finish when a saved world renders full or selected windows without regeneration.
4. **Basin structure:** depression hierarchy, storage curves, routing flats, nested/outlet fixtures. Finish when closed basins survive analysis without terrain replacement.
5. **Climate:** temperature, rainfall/rain shadows, evaporation and runoff. Finish when poles, altitude cooling, wet coasts, and dry interiors are inspectable as separate layers.
6. **Water balance and rivers:** wet/dry/seasonal basin outcomes, overflow and river graph, bounded climate coupling. Finish when water conservation and contrasting dry/wet basin fixtures pass, and final-area candidate search achieves the tolerance or explicitly reports failure.
7. **Cryosphere and biomes:** snow/ice proxy and classification tables. Finish when cold deserts, alpine ice, polar regions, and biome transitions are plausible.
8. **Usability and scale:** legends, presets, validation messages, memory profiling, deterministic parallelism where useful, regression suite and usage documentation.

Do not begin with every climate knob or optimized storage codec. Establish appealing periodic terrain and coherent basins first, while preserving schema/version contracts.

## Explicit exclusions and future extensions

No erosion, plate tectonics, axial tilt, seasonal weather simulation, spherical pole traversal, north–south wrap, or mandatory Earth-scale maps. No game movement costs, economic yields, settlements, or political borders in v1.

Possible later work: alternative noise recipes, richer precipitation, glacier tongues, lake salinity refinement, compressed chunks, alternate render styles, and data adapters for other game projects. These remain independent of the agreed exclusions.

## Transition handoff

This document is sufficient to start implementation in `github.com/mdhender/mpg`. No repository has been created or modified by producing this snapshot. First validate the proposed defaults with small prototypes; tune them visibly, record resolved values, and preserve the agreed choices above. The agreed land contract is final dry area: requested land-cell units × 100 km², accepted within 1%. Sampling is a generator concern; aggregation into 10 km game cells belongs to a future export design.
