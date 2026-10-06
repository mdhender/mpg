# mpg — Province-map generator

Go module: `github.com/mdhender/mpg`  
Design snapshot: 2026-10-04  
Status: implemented through S37 (milestones 1–9); `world.json` schema 1 is frozen.

## Goal

mpg generates a **playable province map** for our strategic fantasy games. The deliverable is a Voronoi mesh with these properties:

- Every cell is a **province** with the area (not the shape) of one 6-mile wilderness hex, about 81 km². That lets the game set exploring a province at one day.
- Every cell is **all land or all water**.
- Every cell carries geography (landform), biome, and altitude.
- Every edge carries direction, neighbor, river, coast, and incline.
- The mesh includes enough geometry for the game's map renderer to draw PNG segments for players.

The noise, climate, and hydrology work exists to make that mesh interesting. It is not the product.

**Possible, not forced.** Dry basins, inland lakes and seas, ice fields, glaciers, and volcanoes come out of the terrain and climate when conditions allow. No setting guarantees any of them, and a world without one is valid. The measures report how many appeared; they never require one.

This is the same shape as the Panama pipeline (`maloquacious/hmz2*`):

| Panama (hmz2*) | mpg |
|---|---|
| Real DEM (`dem2hm`) | Synthetic heightmap built from noise |
| Flat-top hex grid overlay | Relaxed Voronoi overlay on a cylinder |
| `hmz2ter` landforms, `hmz2bio` climate, `hmz2riv` edge rivers | The same steps, done per cell, with rivers on Voronoi edges |
| `hmz2map` emits the game's JSON | `world.json` |

Two unit rules carry over from `hmz2ter`:

- **Rules about the ground use physical units.** Relief and elevation are in meters; drainage and lake areas are in km².
- **Rules about play use cell steps.** Rim width, depth bands, strait and neck widths, and coast distance are counted in cells.

## Changes from the first design

This design replaces the first noise-based design (`DESIGN.md` at commit `9f391c2`; see git history). Compared with it:

| | Item |
|---|---|
| **Keep** | Go; `math/rand/v2`; per-stage seeds from SHA-256 → `rand.NewPCG`; stable tie-breaking by index |
| **Keep** | East–west wrap; cylindrical periodic noise; periodic domain warp |
| **Keep** | Layered noise with domain warp and ridged mountains; no erosion and no plate tectonics |
| **Keep** | Latitude plus lapse-rate temperature; stylized precipitation with winds and rain shadows; no tilt or seasons |
| **Keep** | Depressions preserved: a closed basin can hold a lake, a salt lake, or a dry playa or desert. Lakes are outcomes, not compulsory filling. |
| **Keep** | Drainage that is acyclic by construction, with explicit terminal sinks |
| **Keep** | Resolved `config.json` holds inputs only; outcomes go to the output files; replaying a config never reads current defaults |
| **Keep** | Bounded, deterministic sea-level search that targets land, with an explicit report when the target is unmet |
| **Keep** | Geography, climate, biome, and rendering kept separate |
| **Change** | "Land cell" means one Voronoi land cell (one province). The land target becomes a **count of land cells**, not an area measured in raster samples. |
| **Change** | The game unit becomes the **Voronoi province**, replacing the 10 km square game cell. The "future export" step becomes the core of the generator. |
| **Change** | North and south edges become a **polar rim**: a sheet of impassable ice, instead of bare bounded edges. |
| **Change** | Ocean, lakes, basins, and rivers are decided **on the mesh**, not on the raster. The raster stays as the synthetic DEM and the climate grid. |
| **Change** | Water balance becomes a discrete per-cell fill, replacing storage curves on raster samples. |
| **Change** | The schema is exported Go types (as in `hmz2map`); JSON Schema files are optional. |
| **Add** | A **layout** stage that controls continent count, size, and separation |
| **Add** | Mesh construction: cylinder Voronoi, Lloyd relaxation, short-edge collapse, rim cells |
| **Add** | Per-edge game data and per-cell geometry for the game renderer |
| **Add** | **Playability measures** in every run, with checks on them |
| **Add** | **Stage renders from milestone 1**, plus a multi-seed contact sheet |
| **Drop** | Samples-per-game-cell divisibility; chunked raster JSON as the game contract; the raster window renderer as a product (it remains a dev view) |
| **Drop** | Ocean seeded from the global minimum elevation (the rim replaces it); raster 8-neighbor river extraction |

## Units and sizing

- **Province area `A`** is the area of one wilderness hex, so exploring a province takes one game day. Configure the hex instead of the area: `province.hex_flat_to_flat_mi` (default 6). Then `A = (√3/2)·d²` = 31.18 mi² ≈ **80.75 km²**. Save both `d` and the derived `A`. Only area matches the hex; cells are not hex-shaped or hex-sized in any one direction.
- **Requested land cells `N`.**
- **Land fraction `f`.** Default 0.30, measured over playable (non-rim) cells.

Derived sizes:

```text
playable_cells ≈ N / f
playable_area  = playable_cells × A            (km²)
height_km      = sqrt(playable_area / aspect) + 2 × rim_km
width_km       = aspect × (height_km − 2 × rim_km)
```

Aspect names (square, portrait, landscape, widescreen 16:9, cinematic 2.39:1) and explicit ratios (positive finite numbers on both sides of a colon, such as `16:9` or `2.39:1`) are accepted. `widescreen` means 16:9; `cinematic` means 2.39:1. Aspect applies to the playable area. The rim adds height only.

Example: N = 10,000 and f = 0.30 give about 33,300 playable cells and 2.69M km². At cinematic aspect that is about 2,540 × 1,060 km before the rim.

**Raster spacing** is a generator concern. Default: about 2 km, which gives about 20 samples per province. That is enough for per-cell statistics such as median, percentiles, and relief. The resolved spacing is saved. Raster dimensions do not need to be divisible by anything.

**Land contract:**
- The final number of land cells must be within **1% of N**. With cells this uniform, the count stands in for area: land area ≈ N × A. Report the measured land area too.
- Lake and inland-sea cells are water.
- Glacier-covered land is land.
- Rim cells are never counted.

## Coordinates, topology, and the rim

- **World coordinates** are kilometers. The origin is the northwest corner; `x` runs east in `[0, W)` and wraps, and `y` runs south in `[0, H)` and does not wrap.
- **One shared topology package** handles wrapped deltas, distances, and bearings for both the raster and the mesh.
- **Latitude proxy** is computed over the full height, so the rim is the true pole:

  ```text
  latitude = 1 − 2·y/H
  ```

### Rim

- **Band.** The top and bottom bands are rim. Default: 4 cells deep, after `hmz2map -border`'s 4-hex band.
- **Falloff.** Inside the rim's edge, the falloff band pulls elevation smoothly down to deep ocean. Land therefore never touches the rim, and coasts near the poles look natural rather than cropped.
- **Rim cells** are flagged `rim` and `impassable`. The flag overrides everything else about the cell: the game treats it as impassable, and the renderer draws it as a sheet of impassable ice whatever its geography and biome say.
  - Inside the generator, rim cells count as deep salt water, so the ocean beside them is connected to them and climate sees open water at the poles.
- **Rim width** is how many cells deep the ice band reaches in from the north and south edges. Default: 4 cells.
- **Falloff width** is how many cells of open ocean, at least, separate the ice from any land. Default: about 12 cells. It keeps coasts from running straight into the ice.

The rim solves several problems:

- **The edge.** No land, river, or basin ever meets the map edge, and players never reach a wall of land.
- **Ocean definition.** Ocean is the water cells connected, through cell adjacency, to the rim. Any other water is inland: a lake, inland sea, or salt lake. This replaces the global-minimum ocean seed.
- **Mesh edges.** No ragged polygons at the top and bottom. The rim absorbs Voronoi clipping.
- **Climate extremes** fall in the rim, so playable land ranges from polar fringe to equator.

## Pipeline

| # | Stage | Domain | Produces | Stage render |
|---|---|---|---|---|
| 1 | Resolve config | — | Sizes, spacing, seeds, every default | — |
| 2 | Layout | raster | Continental bias field from attractors and repulsors | Bias field with attractor marks |
| 3 | Elevation | raster | Bedrock elevation in meters (layout + fBm + warp + ridges + volcanic hotspots + rim falloff) | Hypsometric map with hillshade; hotspots marked |
| 4 | Mesh | mesh | Sites, Lloyd relaxation, short-edge collapse, cells, corners, edges, rim cells | Mesh over elevation; area heatmap; short edges highlighted |
| 5 | Cell statistics | raster → mesh | Per-cell altitude (median), relief (p95 − p5), latitude | Cell altitude |
| 6 | Sea level and ocean | mesh | Sea level and ocean cells (flood from the rim) | Land/water cells |
| 7 | Climate | raster (+ cell mask) | Precipitation, potential evaporation, and runoff aggregated to cells; cell temperature from latitude and altitude | Temperature; precipitation; aridity |
| 8 | Basins and lakes | mesh | Basin hierarchy, wet/dry decision, lake and inland-sea cells, spill corners | Basins colored; lakes |
| 9 | Land-target check | mesh | Repeats stages 6 and 8 within a budget until the land count is in tolerance | Search trace |
| 10 | Rivers | mesh corners | Drainage tree on corners; river edges with a class | Rivers on edges, width by class |
| 11 | Classification | mesh | Landform, depth band, surface, biome, flags | Landforms; biomes |
| 12 | Edges | mesh | Bearing, compass direction, neighbor, coast, river, incline, passability | Inclines; passability |
| 13 | Measures | mesh | Playability report | Landmass and chokepoint map |
| 14 | Export | — | `world.json`, `measures.json` | Player-style map |

**Stages 2, 3, and 7** are field-like, so they stay on the raster.

**Stages 6 to 13** are topological and run on the cell graph. At 30k to 200k cells each pass costs milliseconds, which is why the land-target search can afford to re-run basins.

**Climate coupling** is bounded:
1. Compute climate once with the ocean-only mask from the first sea-level estimate.
2. The land-target search re-runs stages 6 and 8 against that fixed climate.
3. Compute climate once more with the final lakes, for biomes.
4. Save the pass count.

How this is built (S29):
- **Lake allowance in the datum.** Lakes take 3–17% of N, and on pangaea replacing them at the coast lowered the sea by 200–500 m, lifting whole continents into plateau. So stage 3 first runs a **pre-pass** of stages 3–8 once, privately (no allowance, renders, logs or outputs; the mesh is reused), and counts its lake cells L. The final elevation puts the 0 m datum at a land share of f·(N + L)/N. Exactly one pre-pass; generation takes about 1.3–1.6× as long.
  - `elevation.datum_max_shift` defaults to 8 (validation limit 16), since pangaea needs shifts of about 2.6–4.7. The larger shift squeezes lowland relief, so pangaea interiors are flatter (flats up to ~50% of land; the landform test bound is 60%). Accepted.
  - The datum does not count samples in the polar falloff band as land when the falloff ceiling is below 0 m, since the falloff drowns them.
- **Stage 6** aims for N + L land cells before lakes, so the first climate pass sees roughly the final ocean.
- **Stage 9** runs the sea-level search (first probe: the quantile for N + stage 8's lake cells), re-running the sea-level flood, basin hierarchy and water balance at each probe against the fixed first climate. Land is counted after lakes: lake and inland-sea cells are water; playas and dry basin floors are land. Measured on 48 worlds: all met (45 exact), 2–7 probes, final sea level within 25 m of 0 except pangaea outliers (−81 m where lakes grew past the pre-pass estimate).
- **Final climate pass:** inland seas recharge the air like the ocean (open-water precipitation; a trace stops at them and leaves at their height); lakes are traced like land. Lake and inland-sea cells take temperature from their altitude above the sea level, like land. The pass count (normally 2) is saved.
- **world.json** (added in schema 0; frozen in schema 1): cell flags `salt` (lakes and inland seas only; the ocean and rim are salt by definition) and `playa`; corner flag `sink` (also `terminal`; a sink touches a playa, and each playa's lowest corner is a sink); outcomes `lake_cells`, `inland_sea_cells`, `lakes`, `inland_seas`, `salt_lakes`, `salt_inland_seas`, `playas`, `expected_lake_cells`, `prepass_lake_cells`, `datum_land_share`, `climate_passes`, and a `lake` count per probe. Landform stays by size: lakes `fresh-water`, inland seas `salt-water`. The validator checks inland water never touches the ocean, neighboring inland-water cells share kind and salt, and outcome counts match the cells.
- **Overflow:** a full basin whose target is full and spills no lower sends its overflow to the lowest neighbor of the spill flat that drains to the sea or a strictly lower basin (two basins closing at one flat had looped).

### Layout

Noise alone produces either one featureless block or lace. wgvc needed attractors and a rival ramp to control islands. Here that control is added to the continental bias field, before the noise:

- **Seeded attractor points** on the cylinder, using periodic distance, each with a radius and weight. They can be placed by a preset pattern or randomly with minimum spacing.
- **Repulsors** between rival attractors, so continents stay separate. The gaps they leave become straits.
- **Presets:** `pangaea`, `continents` (3–5), `archipelago`, `islands` (wgvc-style), and `custom`.

Layout is a bias, not a mask. Noise still makes the coastlines; the playability measures in stage 13 report whether the intent survived.

### Volcanic hotspots

Volcanoes are possible, not forced: rare, seeded, and sometimes absent.

- **Count.** Drawn from the `volcanic` seed with a mean of `volcanic_hotspots_per_mkm2` per million km² of playable area. Default 2, which gives about 5 for the 10,000-cell example. Any count, including zero, is valid.
- **Placement.** Uniform over the playable area outside the rim falloff, in ocean or on land. A hotspot in the sea makes a volcanic island only if its cone reaches sea level.
- **Shape.** Each hotspot adds a cone (default peak 1,500–3,000 m, radius 15–30 km) on a broad, low swell (default +300 m over about 100 km). The swell is what makes the surrounding plateau.
- **Result.** The land cell containing a cone's peak gets the `volcano` flag. Nearby plateaus become `volcanic-highlands`.

### Mesh

- **Sites.** Jittered grid or Poisson-disk sites over the full cylinder, including the rim, seeded from `mesh`. Site count = total area / `A`.
- **Voronoi on a cylinder.** Compute with ghost copies of the sites shifted ±W. Keep only the original sites' cells. North and south are clipped inside the rim, so clipping never affects playable cells.
- **Lloyd relaxation.** Default 2 passes, using wrapped centroids. The cylinder's size is fixed by the raster, so the mesh is not rescaled: with site count n = round(total area / `A`), the mean cell area W·H/n is within 1/(2n) of `A`. Report the mean area, its deviation from `A`, and the area coefficient of variation.
- **Short-edge collapse.** Edges shorter than `min_edge_km` (default 0.3 × √A, about 2.7 km) are taken shortest first, with ties broken by edge index; whenever a corner moves, the lengths of its edges are recomputed. A short edge is **collapsed** when that is safe: the merged corner would touch at most 4 cells, and each of the two cells beside the edge keeps at least 4 sides and 3 neighbors. Collapsing merges the edge's two corners into one 4-way corner at the edge's midpoint.
  - The two cells that shared the collapsed edge now touch only at a point, so they are **not neighbors**.
  - Point contact never connects land to land or water to water, so water that touches only at a corner is not connected.
  - Otherwise the edge is **stretched**: its ends move apart along the perpendicular bisector of its two cells' sites until it is `min_edge_km` long, and its cells stay neighbors. This happens where two short edges meet, which would otherwise make a 5-way corner, and where a collapse would leave a triangle well under A/2. It affects one or two in a hundred short edges.
  - The process is deterministic and bounded; corners move at most about half of `min_edge_km`, a little more where moves chain. Report collapses, stretches, and the largest corner shift.
- **Degree cap.** Every cell needs a distinct compass direction for each neighbor, so no cell may have more than 8 neighbors. After the short-edge collapse, a cell with 9 or more neighbors has its shortest collapsible edge collapsed, repeating until it has 8. Lloyd-relaxed cells have mostly 5–7 neighbors, so this should be rare; report how often it happens.
- **Mesh checks:**
  - no edge shorter than `min_edge_km`;
  - cell areas within configured bounds (default 0.5A to 1.6A);
  - neighbor counts within 3–8 for playable cells; rim cells 1–8, since a rim cell against the map edge can have only one or two neighbors straight from the Voronoi diagram;
  - every corner touches 3–4 cells, or 2–4 on the north and south map edge.

### Cell statistics and sea level

- **Sample assignment.** Each raster sample belongs to the cell whose site is nearest, using wrapped distance and a bucket grid. Statistics follow `hmz2ter`'s nearest-rank percentiles: the median gives altitude, and p95 − p5 gives relief.
- **Altitude is the only height.** A cell's **altitude** is its median elevation. Every height rule on the mesh uses it: the land test, sea level, basins and spill levels, lake surfaces, corner heights, incline, landforms, and cell temperature (latitude curve minus lapse-rate cooling at the cell's altitude). Relief (p95 − p5) is a roughness measure, not a height; it only sets landforms.
- **Land test.** A non-rim cell is a land candidate if its altitude is above sea level.
- **Ocean.** Water candidates connected to the rim are ocean. Unconnected candidates below sea level are basin floors, which stage 8 decides.
- **Sea-level search.**
  - The initial estimate is the quantile that leaves `N` + (expected lake cells) land candidates.
  - After that, run a bounded deterministic search over candidate cell altitudes, keeping the best measured result.
  - Basins can jump, so the land count is not assumed to change monotonically.
  - Save the policy, budget, chosen level, achieved count, and termination reason.

### Climate

Stage 7 computes climate on the raster with the cell mask drawn onto it: each raster sample takes its cell's class through the sample-to-cell assignment of stage 5. To the climate, open salt water is every rim cell and every cell the ocean flood reached; dry basin floors are land. Raster fields are averaged into each cell over its samples.

**Temperature.** A cell's mean annual temperature is T = T₀(φ) − Γ·h/1000 °C.
- φ is the cell's latitude in degrees, 90·|1 − 2y/H| at its site, so the map's north and south edges (inside the rim) are the poles.
- T₀ is the sea-level curve `climate.sea_level_temp_c`: points (lat_deg, temp_c) from 0° to 90° joined by straight lines, never warmer poleward. The default is hmz2bio's: 27 °C to 10°, 26.5 at 15°, 25 at 20°, 22.5 at 25°, 20 at 30°, 17 at 35°, 14.5 at 40°, 12 at 45°, 8 at 50°, 4 at 55°, 0 at 60°, −8 at 70°, −16 at 80°, −22 at 90°.
- Γ is `climate.lapse_rate_c_per_km`, default 6.5.
- h is the cell's altitude above the sea level in meters for land above the sea, and 0 for ocean, rim, and basin floors at or below the sea, which take the sea-level temperature of their latitude.
- No tilt, no seasons.
- **Rim cells are coldest at sea level:** as open polar water they are colder than every playable cell at sea level. High land near a pole can be colder than the rim (seed 5 portrait pangaea has a cell 2,684 m up at about 69.5° at −25 °C against a rim of −21.8 to −20.2 °C), and that is accepted.

**Precipitation.** Per raster sample, after hmz2bio (`tpty/hmz2bio` climate.go, rules.go, README "Precipitation"): P = W(φ′)·v·(s + (1 − s)·m·(1 + g·lift/1000)) mm/yr.
- φ′ is the signed latitude, 90·(1 − 2y/H), shifted by up to `band_jitter_deg` (default 4°; 3 octaves of cylinder noise from 2,000 km) so the bands waver with longitude. Temperature is not jittered.
- W is `windward_precip_mm`, hmz2bio's windward-coast table: 3,100 mm at 0°, 3,300 at 5°, 2,700 at 10°, 2,000 at 15°, 1,400 at 20°, 1,000 at 25°, 850 at 30°, 1,000 at 35°, 1,250 at 40°, 1,400 at 45–50°, 1,250 at 55°, 1,000 at 60°, 500 at 70°, 250 at 80°, 150 at 90°. s is `convective_share`: 0.5 at 0°, 0.45 at 10°, 0.3 at 20°, 0.2 from 30°.
- v = 1 + `precip_noise_amp`·noise is the periodic variability (default ±0.15, 4 octaves from 800 km; the amplitude must stay below 1). g = `lift_gain_per_km`, 1.5.
- Ocean samples get W·v.

**Winds.** Bearings are given for the north and mirrored (180° − b) in the south.
- Trades from 60° up to 27°; westerlies from 255° over 33–57°; polar easterlies from 60° beyond 63°.
- Neighbouring bands blend linearly, and the hemispheres blend over ±5° of the equator (`equator_blend_deg`). Blending weighs each wind's moisture and lift, never bearings.

**Moisture and rain shadows: a backward trace.** This is the "advection with a fixed iteration count".
- Each wind is traced along 5 rays spread ±20° about its bearing, stepping upwind 5 km at a time for at most 1,000 km (200 steps). Each step reads the cell under the point; x wraps.
- The trace stops at ocean (rim included) or off the map, where the air is saturated. With no sea in reach, the air starts at sea one step beyond the last point.
- Moisture is m = exp(−n·step/`rainout_km` − Σrise/`orographic_m`), defaults 500 km and 1,200 m (hmz2bio's 800 km and 1,500 m gave weaker rain shadows). Lift is the sample's height above the lowest point within 30 km upwind.
- Heights are cell altitudes above sea level (one height); the raster elevation is not read.
- Each sample is independent: no sweep order, nothing special at the seam, and results are identical for any worker count.
- Cell values are the means of their samples.

**Evaporation, runoff, aridity** (per cell).
- PET (Holdridge) = 58.93 mm per °C of biotemperature, which is T clamped to [0, 30] °C.
- Runoff follows Budyko (1974): with φ = PET/P, E = P·√(φ·tanh(1/φ)·(1 − e^(−φ))) and R = P − E ≥ 0. With PET = 0 everything runs off.
- Aridity index AI = P/PET, capped at 10, and 10 when PET = 0. UNEP classes: hyper-arid < 0.05, arid < 0.2, semi-arid < 0.5, dry sub-humid < 0.65, humid otherwise.
- Biomes may also use hmz2bio's Köppen dry limit, 20·T + 280 mm.
- Default world, land: P p5/p50/p95 ≈ 330/1,040/2,490 mm; PET p50 ≈ 1,270 mm; runoff p50 ≈ 290 mm. Arid land is nearly absent (the convective share floors P); desert tuning is left to biomes (M8).
- Windward coasts get ≈ 0.93–0.98 of W at their latitude; lee sides and interiors ≈ 0.33–0.49.
- Renders: temperature, mask, precip, moisture, pet, runoff, aridity.

**Final pass with lakes (S29).** Inland seas recharge the air like the ocean; lakes do not (see "Climate coupling").

**Seasons (resolved, S32).** The climate stays seasonless. Only the biome table uses hmz2bio's synthetic annual range, 1 + 0.33·|lat°| °C split about the mean, for its warmest- and coldest-month rules and the snowfall ramp (see "Surface and biome"). The range is never stored.

### Basins and lakes

- **Priority flood.** A priority flood over the cell graph, seeded from ocean cells and using cell altitude, finds depressions, spill cells, spill edges, and the nested hierarchy. Original elevations are never modified.
  - The water rises everywhere at once (a merge tree, Barnes et al. 2020): the sea (rim and ocean) is flooded from the start, and every other cell is taken by altitude, ties to the lower cell id, joining the flooded components it touches. A flood from the sea alone fills each depression in one piece and cannot see nesting; the tests check the two agree on every cell's outermost spill level.
  - Cells of equal altitude are taken as connected flats that act as one cell, so plateaus never split by id and every depth is positive. A flat touching two or more depressions closes them and starts their parent.
  - A depression's **spill cell** is the pass: the cell outside it at its spill level (a cell of its parent, or free-draining land). Its **spill edge** and **spill corner** are, among the edges between the pass flat and the depression, the one ending at the lowest corner by corner height (mean altitude of the corner's cells), ties to the lower corner id, then edge id.
  - Measured at 50 m on 37 worlds (N = 10,000): 233–489 depressions become 36–190 basins (2–19% of land cells); 70–80% of depressions are under 50 m deep. Pangaea nests deepest (up to 25 levels).
- **Water balance** is a discrete fill, because every cell is one whole area unit:
  1. Sort each basin's cells by altitude.
  2. Fill them in that order.
  3. Stop when evaporation from the lake area plus seepage balances catchment runoff, or when the spill level is reached.
  - **Catchment:** every cell drains by steepest descent on its routing height (altitude, with each depression shallower than the minimum raised to the spill of its outermost shallow ancestor; sea lowest; ties by id; flats resolved from their exits). A basin's catchment is the cells whose descent ends in it, typically 6–11× its own area. Rivers (S30) may differ near ridges; the balance is self-consistent either way.
  - **Costs:** a lake cell evaporates its PET and receives its own precipitation instead of its runoff, and loses `basin.seepage_mm` (default 50 mm/yr). Turning cell c into lake costs d_c = area_c·(PET_c − AET_c + seepage) ≥ 0, with AET = P − R (Budyko), so the fill is a prefix of the fill order.
  - **Fill:** from the basin's bottom, a priority flood over its cells by (altitude, id); a cell is filled only if the remaining budget is at least its d_c. The surface is the highest filled altitude, or the spill level when full.
  - **Order:** top-level basin trees by descending spill level (ties by id); within a tree, children before parents. A full child's surplus goes to its non-full sibling with the lowest surface (ties by id), or, once all children are full, the parent continues the flood as one surface over them. A full top-level basin overflows through its spill corner and runs down to the sea or to a lower basin, which receives it as inflow (passing on through basins already full).
  - **Conservation** is checked in real cell area × mm/yr: runoff + lake precipitation = evaporation + seepage + overflow to the sea + unplaced water (playa inflow and partial-fill remainders), to a relative 10⁻⁹ (measured ≈ 3·10⁻¹⁵).
- **Surplus at the spill** overflows through **one spill corner**, which became a wgvc rule for good reason, and continues downstream.
- **No stable level** means the basin stays dry land (flag `playa`, surface `salt-flats` from S32) and is a **dry sink** for rivers.
  - That is a leaf basin whose inflow is less than d of its bottom cell. Only the bottom cell is `playa`; its lowest corner by corner height (ties by id) is the dry sink. A partly filled basin is a lake plus ordinary land; a dry parent whose children hold lakes is plain land.
- **Classification:**
  - lake: 1 to `inland_sea_min_cells` − 1 cells (one-cell lakes are fine);
  - inland sea: `inland_sea_min_cells` or more (**default 20**);
  - each is marked salt if endorheic and evaporation-dominated (a salinity proxy, not chemistry): no overflow, and evaporation / (evaporation + seepage) ≥ `basin.salt_evap_share` (default 0.5). Frozen lakes (PET 0) are fresh.
  - Measured on 8 default-size worlds: 15–47 lakes, 4–13 inland seas, 2–17 salt, 1–12 playas; lake cells 2.7–12.8% of N, which the land-target search (S29) must absorb.
  - world.json (from S29): a `salt` cell flag (landform lake or inland sea by size), a `playa` cell flag, a `sink` corner flag, and lake, inland-sea, playa and salt counts in the outcomes.
- A lake is one connected set of cells with one surface level.
- **Minimum depth.** A depression counts as a basin only if its spill level is at least `basin_min_depth_m` above its lowest cell's altitude. **Default: 50 m.**
  - Shallower depressions are treated as flat ground at their spill level for routing only: no lake, no playa, no dry sink, and altitudes are not changed. Nested sub-basins shallower than the minimum below their own spill merge into their parent.
  - Why 50 m: closed ground 50 m deep across at least one ~9 km province is real topography. Shallower dips are mostly noise left over after taking each cell's median, the same size as `hmz2ter`'s plains relief band (20–60 m), and keeping them pockmarks the map with one-cell lakes and pits.
  - Tune it with the basin depth histogram in the measures. Lower it for more lakes and playas, raise it for fewer.

### Rivers on edges

Rivers run along Voronoi edges, corner to corner, and never through a cell's interior. This follows mapgen2, wgvc, and `hmz2riv`'s snapped edges.

- **Corner height** is the mean altitude of the 3–4 cells that meet at the corner, so rivers follow the low ground between low cells using the same single height.
- **Corner graph.** The graph has the land corners (those touching a land cell) joined by **land–land edges**.
  - A corner that touches the sea (an ocean or rim cell; the rim counts as sea) is a terminal **mouth on the ocean**. No land corner touches the rim on any world measured; the rule only makes sure of it.
  - A corner that touches a lake or inland sea is a terminal **lake shore**. A corner touching two lakes (only at a 4-way corner, 0–7 per world) belongs to the one with the lower surface, ties to the lower id.
  - Dry-sink corners from stage 8 are also terminal.
- **Drainage tree.** A priority flood over the corner graph gives every land corner one downstream corner.
  - The flood starts from the ocean corners, the dry sinks, and the shore corners of closed lakes.
  - Corners are taken by level (corner height, filled to the spill of any corner-level pit). Among corners of equal level, the first queued is taken first (FIFO), so water crosses a flat breadth first from where the flat drains.
  - Each corner flows to its already-taken neighbor with the lowest level, ties to the one taken first.
  - The result is acyclic by construction.
  - About 2–4% of tree steps lie on flats, all in corner-level pits filled to their spill level; no two neighboring land corners have exactly equal heights. Taking flats by corner id instead (the first design) combed the flow into parallel north–south runs, with flat paths up to 1.2× the shortest route on average; FIFO brings that to about 1.0 (S30, measured on 10 worlds).
- **Lake outlets.** A river may enter a lake at any shore corner. An overflowing lake leaves by exactly one way:
  - **(a) Direct.** If its spill corner touches the sea, or belongs to another, lower lake, it drains straight there and no river leaves it.
  - **(b) Spill corner.** If the spill corner has a land–land edge to a corner that does not touch the lake, the spill corner is the outlet (about 70% of overflowing lakes).
  - **(c) Pass cell.** Otherwise the outlet is the first of the lake's shore corners on the pass cell that the flood reaches. The spill corner is the lower end of the spill edge, so it often touches two lake cells and has no land–land edge (31% of overflowing lakes on 10 worlds; a strict spill-corner rule deadlocked on 2 of them). If that outlet itself has no way over land (a 4-way corner between two lakes, or the sea), the lake drains directly, as in (a).
  - Until the outlet is taken, the lake's other shore corners are held out of the flood. The outlet's downstream path therefore never re-enters its lake, and lakes draining into lakes cannot loop. The held corners are then released at the outlet's level as inflow terminals.
  - **(d) Fallback.** If the flood stalls with corners held, the lowest held corner, by level then id, becomes the outlet; the count is logged. It was never used in 128 test worlds.
  - The outlet's downstream path carries the lake's surplus: the water balance's `Overflow` (S28), which is the authority, even though the tree's own inflow to a lake can differ by up to about 2×.
- **Catchments.** Each land cell's runoff enters the tree at its lowest corner. The river stage reports how many land cells, and how much area, end somewhere other than the water balance's cell-level catchment, both immediately and after following lake overflow. Over 128 worlds: 0.4–8% immediately (mean 3.2%) and 0–9% finally (mean 1.6%), near ridges and basin rims. The water balance stays self-consistent either way.
- **Accumulation.**
  - Each land cell sends its area and its runoff (area × the final climate pass's runoff depth) to its lowest corner on the graph.
  - Both accumulate down the tree in reverse flood order. A corner's drainage (km²) and water are everything reaching it.
  - Each tree edge carries the drainage and discharge of its **upstream** corner: the water flowing through it. (Reading it as the downstream corner's would make every one-edge stub that joins a big river a river itself.) Discharge in m³/s is the volume (mm·km²/yr) × 1000 / 31,557,600 (a Julian year).
  - **Lakes pass drainage through.** A lake's drainage is the drainage reaching its shore corners, plus its own cells' area, plus the drainage of lakes draining straight into it.
    - An overflowing lake passes it on at its outlet. The outlet's discharge is the water balance's `Overflow` (the authority), plus whatever the tree brings to the outlet corner itself. The tree's own inflow, from the final climate pass, can differ by up to about 2×; discharge is internal, so the step is harmless.
    - A closed lake, or one draining straight to the sea, ends its drainage.
    - On 12 worlds, passing through instead of stopping adds 3–7% river edges; the largest drainage per world is 9.4k–150k km².
- **River selection.**
  - An edge is a river when its drainage is at least `river.threshold_km2` (500 km², about 6 cells).
  - Classes come from drainage breaks: `stream` from the threshold, `river` from `river.river_km2` (2,000 km², about 25 cells), `major-river` from `river.major_river_km2` (10,000 km², about 124 cells). The breaks must increase strictly, so the lowest break is the threshold.
  - Measured on 12 default worlds: stream 60–82%, river 18–33%, major-river 0–11% of river edges. Islands have almost no major rivers; possible, not forced.
  - Drainage never falls downstream, so neither does a river's class.
  - Drainage and discharge are internal: they set the class and appear in the logs, measures and debug dumps, not in `world.json`.
  - For comparison: on Panama, `hmz2riv`'s 50 km² gave about one river edge for every two 10 km hexes, chosen deliberately to slow north–south travel. Here 500 km² gives 0.15–0.29 river edges per land cell; about 100–150 km² would match Panama. Tune against play.
- **Polylines and mouths.**
  - The river edges are split by main stem into polylines, listed source to mouth and ordered by first edge id.
  - At a corner the main inflow is the river edge into it with the largest drainage, ties to the lower upstream corner id. At a lake's outlet the lake also counts as an inflow, with the drainage it passes on, and wins a tie.
  - A polyline starts at a corner with no main river inflow: a source, or an outlet whose lake is its main inflow.
  - It ends at a terminal (its mouth: the sea, a lake shore, or a dry sink), or at a confluence: a corner inside another polyline where the main stem continues.
  - Every river edge lies on exactly one polyline.
  - The `mouth` corner flag goes exactly where a polyline ends and none continues. A lake outlet is a terminal corner (it touches the lake) but not a mouth.
  - On 12 worlds: 302–418 polylines, 200–263 mouths, 12–42 rivers per world leaving lake outlets, longest river 18–75 edges (123–572 km).
- **Density** is reported by the river stage (its log, and `river.NetworkStats` for the measures; `measures.json` from S35), not in `world.json`:
  - river edges per land cell (0.15–0.29);
  - river km per 1,000 km² of land (12–25);
  - share of land cells touching a river (18–32%);
  - mouths, polyline endings, the longest river, and seam crossings (0–21 per world).
- **Validator rules** (`mpg validate`):
  - a river edge is land–land, and no corner of it touches the rim;
  - polylines follow their edges corner to corner, with no corner twice, classes never falling, ordered by first edge id;
  - every river-class edge is on exactly one polyline;
  - `mouth` sits exactly on corners where a polyline ends and none continues, and a mouth is terminal.

This guarantees by construction:

- Rivers are only ever on edges.
- Rivers never run along a coast or lake shore, because those edges are excluded from the graph.
- Every river ends at a mouth, a lake, or a dry sink.
- Rivers never cross the rim.
- Rivers cross the east–west seam naturally.

### Classification

Use the hm* codebooks where they fit, so the engine and converter tools stay familiar.

- **Landform (land), from relief and altitude:**
  - `flats`, `plains`, `rolling-plains`, `hills`, `mountains`, `plateaus`, using `hmz2ter`'s relief thresholds as starting values. Retune them, because relief within an 81 km² cell of synthetic terrain will not match DEM relief. First retune (S21): relief breaks 35 / 65 / 150 / 450 m (flats / plains / rolling-plains / hills, mountains above), plateaus at altitude ≥ 500 m above sea level with relief < 150 m. Synthetic land keeps about 30 m of relief even when flat, so hmz2ter's 20 m gave almost no flats, and 350 m spread mountains down the flanks.
  - `volcanic-highlands`: a `plateaus` cell within `volcanic_radius_km` of a volcano (`hmz2ter`'s rule, default 25 km). At 25 km this falls inside the cone and rarely fires; that is accepted as a possible, not forced outcome.
- **Landform (water):** `salt-water` (ocean, inland sea) and `fresh-water` (lake). Salinity is a flag.
- **Depth (salt water):** `shallow`, `open`, `deep`, from distance in cell steps to the nearest non-salt-water cell, as in `hmz2ter`. The bands are play rules, so they are retuned in cells: shallow ≤ 3 steps, open ≤ 8, deep beyond (S21; `hmz2ter`'s 12 / 19 made over half the sea shallow). Rim cells are deep salt water and are not stepped through.
- **Surface and biome** (S32). The table `hmz2bio-0.3.0+mpg.1` is Go constants in `internal/classify`, recorded in world.json's `biome_table` outcome; it is not in the config.
  - **Seasons for the table only:** warmest/coldest month = T ± (1 + 0.33·|lat°|)/2. Snowfall = P·clamp((range/2 − T)/range, 0, 1).
  - **Biome** (playable land only; first row wins; Köppen aridity limit L = 20·T + 280 mm):
    1. warmest < 0 °C and snowfall ≥ 300 mm → permanent ice, biome `clear`;
    2. warmest < 0 °C → `polar-desert`, bare (cold dry land stays bare);
    3. warmest < 10 °C → `tundra` if the sea-level warmest month at that latitude is below 10 °C too, else `alpine`;
    4. P < ½L → `desert`; P < L → `scrubland` if T ≥ 18 °C, else `steppe`;
    5. T < 18 °C on hills, mountains, plateaus or volcanic highlands, sea-level coldest month ≥ 15 °C, lift ≥ 100 m, P ≥ 1,000 mm → `cloud-forest`; otherwise T < 18 °C with sea-level coldest ≥ 18 °C (tropical montane) → `temperate-forest`;
    6. T ≥ 18 °C → `tropical-rainforest` (P ≥ 2,000), `tropical-dry-forest` (P ≥ 1,200), else `savanna`;
    7. T < 4 °C → `boreal-forest`; P ≥ 2,000 → `temperate-rainforest`; P < 1.5·L → `grassland`; else `temperate-forest`.
  - Rows 3–7 are hmz2bio's rules unchanged; rows 1–2 split its glacial ice by snowfall and add `polar-desert`. Deserts come from the Köppen limit. UNEP's arid classes are nearly absent because the convective share floors P, and the climate is not retuned.
  - **Glacier and ice field** are surfaces on permanent-ice land, and the biome is `clear` exactly under them. In each connected ice body, mountain cells are `glacier`. Non-mountain cells are `ice-field` when the body has ≥ 5 non-mountain cells or no mountain at all, and otherwise `glacier` (tongues reaching down). Neither is placed on purpose.
  - **Pack ice** is a surface on playable water (ocean, inland seas, and lakes at their surface temperature) whose warmest month is below 0 °C. It is never on the rim, which stays the ice sheet.
  - **Wetlands** are surfaces on land that is not ice, from a wetness signal: flats beside a `major-river` edge, flats or plains touching a lake or inland sea, or flats with aridity ≥ 2. The kind follows hmz2bio's order: `salt-flats` if P < L, `mangroves` on an ocean coast with coldest month ≥ 15 °C, `bogs` below 10 °C, `swamps` in a forest biome, else `marshes`. Every playa (the flag stays) has surface `salt-flats`.
  - **world.json:**
    - cell `biome` (playable land only) and `surface` (omitempty; never on the rim);
    - codebooks `biomes` and `surfaces`;
    - outcomes `biome_table`, `glacier_cells`, `ice_field_cells`, `pack_ice_cells`, `wetland_cells`;
    - the validator checks the pairings and the counts.
  - **Player map:** fills a cell by its surface, else by its biome darkened by landform (hills 84%, mountains 68%, plateaus 90%, rolling plains 94%). The rim stays the ice sheet.
  - **Renders:** classify `biome`, `surface`, `wetness`.
  - **Measured on 12 default worlds:**
    - desert 0–27% of land, mostly cold (BWk);
    - polar desert 0–177 cells;
    - glaciers 0–109 cells, almost all on mountains;
    - ice fields 0–98 cells;
    - wetlands 1.7–8.8% of land, with 12.7% on seed 7 cinematic pangaea (mostly bogs on cold flats);
    - pack ice 13–17% of playable water;
    - temperate rainforest does not occur (mid-latitude P tops out near 2,000 mm), which is accepted as possible, not forced.

### Edges

The game wants per-edge data. Store each undirected edge once, and give each cell an ordered list of half-edges.

- **Bearing.** Degrees clockwise from north, from this cell's site to the neighbor's site, using the wrapped delta.
- **Direction.** The game's direction for the edge: one of the 8 compass points `N`, `NE`, `E`, `SE`, `S`, `SW`, `W`, `NW`, ordered clockwise.
  - **Unique per cell.** No two edges of a cell share a direction, so an order like "move NE" is never ambiguous. A cell with fewer than 8 neighbors leaves some directions unused, and moving in an unused direction is not possible.
  - **Order-preserving.** Sort the cell's edges by bearing. Directions follow the same clockwise order, so the edges and the rose never cross.
  - **Closest fit.** Among order-preserving assignments, choose the one with the smallest total angular error between each bearing and its compass point's angle (multiples of 45°). With at most 8 edges this is a small dynamic program per cell. Break ties by the lower maximum error, then by the assignment that starts at the earliest compass point. (S22 reads this as the lexicographically least sequence of compass points in bearing order; errors are compared exactly, with no epsilon.) On the relaxed mesh a naive nearest-point label repeats in about 1% of cells; the mean error is about 10°, p95 about 21°, max about 46°.
  - **Not symmetric.** If A's edge to B is `NE`, B's edge to A is usually, but not always, `SW`. Both directions are stored, one on each half-edge.
  - **Error recorded.** Store each edge's angular error from its bearing. wgvc found that nearest-point labels on a Voronoi mesh collide often (193 of 300 provinces had a duplicate), which is why the assignment is solved per cell instead.
- Cells list their half-edges in clockwise order, starting from the one nearest `N`.
- **Neighbor.** The neighbor cell id and the shared edge id.
- **Coast.** Set when exactly one side is water. The water kind is ocean, lake, or inland sea.
- **River.** The river class (`stream`, `river`, `major-river`), or none. That is all the game gets. The game turns the class into a travel-time modifier; rules such as major rivers being impassable except across nearly level edges are game rules, not generator rules. A river edge is always land–land, so it is a border between provinces.
- **Incline.** A signed grade in percent between the two cells' altitudes: (neighbor altitude − this altitude) / site distance × 100. Positive climbs, negative descends. The magnitude is capped at 100% and rounded to one decimal place.
  - There is one gradient per edge, so A → B is exactly the negative of B → A (+15% and −15%). Compute it once per undirected edge and negate it for the reverse half-edge, so rounding can never break the symmetry.
  - The game decides what grades mean for movement; the generator does not classify them.
  - Centers are about 9 km apart, so even a 2,000 m difference is a grade of about 22%. Most edges will be in single digits, and 100% will be very rare. Watch the grade histogram in the measures. Stored as whole tenths of a percent (S22), so the reverse is an exact integer negation.
- **Passable.** False across the rim. The game may add rules on top. Short edges are already gone, so every remaining edge is a real border.

### One game data file

`world.json` is the only game data file. The engine and the player-map renderer both read it; there is no separate export for rendering. For every cell it holds the game data (geography, biome, altitude, flags including `rim`) and the geometry to draw it.

The geometry in `world.json`:

- **Corners:** id, `(x, y)` in km, corner height, terminal or mouth flags.
- **Cells:** site, centroid, a polygon as a clockwise list of corner ids, an unwrapped polygon in km relative to the site (so seam cells draw without special cases), and a bounding box.
- **Edges:** corner ids at each end, length in km, and a per-edge seed, so the renderer can draw deterministic noisy edges for coasts and rivers if it wants to.
- **River polylines:** corner chains from source to mouth, with the class of each segment, ready for drawing by width.
- **Coastline polylines:** chains of coast edges, closed for islands and lakes.
- **World metadata:** W, H, rim, wrap flag, province area, units, codebooks.

v1 (S37) is frozen. It is compact JSON with `schema: 1`: inclines are in permille (tenths of a percent), positions are rounded to 1e-6 km, and heights keep full precision.
- **Contents:** v1 is v0 as milestone 8 left it, minus the always-empty `outcomes.deferred` and two edge fields that were never written.
- **Size:** the default 10,000-land-cell world is about 55 MB (13 MB gzipped). Trimming derivable fields waits for the game's feedback and would be v2.
- **Changes:** any change to the layout makes a new version. Unknown fields are rejected, so an added field does too.
- **Migration:** a reader migrates an older frozen version only through explicit, tested steps that never invent data. v0 was pre-release, so it is rejected with a request to regenerate the world; a newer version asks for a newer reader.
- **Identity:** the schema is the layout, not the world. The same config gives the same bytes only from the same generator version.
- `mpg validate` checks the file's structural invariants. The rules are in `world/doc.go` ("Versions and migration").

Rendering a player's map segment means selecting cells whose bounding boxes intersect the window (taken modulo W) and drawing each polygon by its geography and biome, then rivers and coasts. Rim cells are drawn as impassable ice instead. No raster is needed.

## Playability measures

Write `measures.json` and a short text summary on every run. Configured checks fail loudly. Sweeps rank seeds by these measures.

- **Land:** land cell count against N; land area; cell area mean and coefficient of variation; edge length minimum and p5; neighbor-count histogram; grade histogram; cells that needed the degree cap; direction error (mean, p95, max) and how often a reverse direction is not the opposite point.
- **Landmasses:** count, size histogram, largest share of land, and count by class (continent, island, islet) using cell-count thresholds.
- **Water:** ocean, inland sea, and lake counts and sizes; coast edges per land cell.
- **Features:** dry basins (count, sizes, depths); basin depth histogram, including the depressions below the minimum; ice-field and glacier cells; volcanoes, and how many are on land. Report only.
- **Chokepoints:**
  - **straits**: water crossings of at most `k` cells between landmasses or between parts of one landmass;
  - **necks**: land isthmuses of at most `k` cells;
  - **passes**: low-incline routes through mountain chains.
  These come from wgvc, measured in cells.
- **Rivers:** river edges per land cell, mouths, longest river in edges, share of land cells touching a river.
- **Usability:** habitable share of land (not glacier, desert, mountain, or polar desert); biome and landform histograms; land within `d` cells of the coast.

Checks begin as report-only. Promote them to gates once tuning shows sensible ranges.

How it is built (S33):
- **Stage 13** measures every run and writes `measures.json` and `measures.txt` (a one-screen summary, also logged). `measures.json` has its own schema, versioned apart from `world.json` (schema 1 from S37, frozen under the same rules; its scalar names are part of it); its exported Go types are in `world/measures.go`. It is two-space indented with full float precision and records the seed and config hash, with no timestamps, paths or generator version, so it is byte-stable and golden-hashed.
- **Names:** scalar measures are named by their JSON paths (`land.deviation_percent`, `directions.error_p95_deg`). S33 covers land, mesh, directions, grades and water (counts, largest lake and inland sea, coast edges per land cell); later groups join the same way.
- **Checks:** `config.json` has `measures.checks`, a list of `{measure, op, value, mode}` with op `<=`, `>=`, `<`, `>` or `==` and mode `report` or `gate`. An unknown measure name fails when the config resolves. Config and measures JSON are written without HTML escaping, so ops stay readable.
- **Defaults:** 14 report-only checks, with bounds from 44 measured worlds: land within ±1% of N and 2% of N·A, land area CV ≤ 0.13, edge p5 ≥ 3 km, ≤ 30 degree-cap collapses, direction error mean ≤ 12°, p95 ≤ 23° and max ≤ 45°, reverse not opposite ≤ 1%, steepest land grade ≤ 50% with none at the cap, and no land–rim edges.
- **Failures:** a failed report check is listed and logged. A failed gate still writes every output, `world.json` included, and then `mpg generate` exits 3 (1 is a config or stage error, 2 a usage error). Sweeps never stop on a gate: the row label ends in GATE, and a `measures` column shows the check counts.

How it is built (S34):
- **Landmasses** are connected sets of land cells (land after lakes, joined through shared edges; lakes and inland seas are water), with ids by lowest cell. Classes come from `measures.landmass`: islet ≤ 9 cells, continent ≥ 1000, island between. On 24 measured worlds, stray fragments have 1–58 cells and the masses a preset intends have ≥ 100.
- **Chokepoints** are counted in cells with one shared k = `measures.chokepoints.max_cells` (default 3), adapted from wgvc's `seas.go`:
  - A **strait** is the water on crossings of at most k playable water cells (ocean, lakes, inland seas; never the rim) joining two landmasses, or two shores of one landmass that no land path of at most `detour_cells` (20) steps joins (wgvc had only the first kind). Crossings group by landmass pair and water adjacency, one strait each. A major strait has no islet shore.
  - A **neck** is a cut of at most k land cells leaving two regions of at least `neck_min_region_cells` (10) each. Width 1 comes from articulation points; wider cuts are paths whose end cells touch water, kept only when no smaller subset is a cut. Touching cuts merge into one neck.
  - A **mountain chain** is at least `measures.passes.chain_min_cells` (10) connected mountain cells. A **pass** (new; wgvc has none) is a route through at most 3 chain cells, with every edge |grade| ≤ 3%, between land off every chain that no land path of at most 20 steps avoiding the chain joins. Route cells group per chain by adjacency. Our mountains are compact blobs with gentle grades (median 4%), so grade alone barely separates passes.
- **measures.json** gains `landmasses` (counts by class, size histogram, largest share, a per-landmass list) and `chokepoints` (counts, width histograms, and strait, neck and pass lists with width, landmass ids and cells). world.json is unchanged, and there are no new default checks.
- **Render:** stage 13 draws the landmass and chokepoint map, with landmasses by class, chains darkened, and straits (red; green within one landmass), necks (magenta) and passes (orange) marked. The sweep measures tile is captioned `LM n (c/i/.) str all/major neck n pass n`, plus the check counts.
- **Measured** on 24 default worlds: 0–14 major straits, 0–13 necks and 5–46 passes. The stage costs 47–112 ms per world.

How it is built (S35):
- **Features** (`features`): every depression of the land target's basin hierarchy goes into a depth histogram (0-10, 10-25, 25-50, 50-100, 100-200, 200-500, 500-1000, 1000+ m), and those shallower than `basin.min_depth_m` are also counted on their own. Basins are reported by water state, with the deepest.
  - A **dry basin** is a basin with no water in it or in any basin nested in it, whose parent holds water: the largest waterless closed ground. Each holds at least one playa.
  - Dry basins are reported with count, cells, largest, deepest, and a list. The cells measure is `dry_basin_area_cells`, which is not world.json's `outcomes.dry_basin_cells` (land at or below sea level).
  - Also reported: glacier, ice-field, polar-desert and pack-ice cells; hotspots, volcanoes on land, and volcanic-highland cells.
- **Rivers** (`rivers`) restate the river stage's `NetworkStats`:
  - edges by class, and per land cell;
  - km per 1,000 km², and the share of land touching a river;
  - polylines, mouth corners, and polyline ends by terminal;
  - the longest polyline and flow path, seam edges, and the largest drainage and discharge.
- **Usability** (`usability`): habitable land is not under permanent ice, not polar desert or desert, and not mountains. Wetlands are habitable and are reported separately.
  - A land cell's coast distance is its steps through land to playable water (ocean, lake or inland sea; 1 when touching it).
  - `measures.usability.coast_cells` (d, default 3) sets the share reported within d. The full distance histogram and the biome and landform histograms are listed too.
- **Water** gains lake and inland-sea size histograms.
- **Measured** on 56 default worlds (seeds 1–7, square and cinematic, four presets):
  - habitable land: 76–95%;
  - wetlands: 0.6–12.7%, above 7% only on cinematic pangaea;
  - rivers: 0.14–0.32 river edges per land cell, touching 17–35% of land; 149–263 mouths; longest river 17–75 edges;
  - within 3 cells of the coast: 30–69% (pangaea lowest, islands highest);
  - dry basins: 0–11.
- **Outputs:** there is no new render and no new default check. The sweep measures tile gains two caption lines, `hab … wet … c<d> … riv … mo … L…` and `dep … bas … dry … pl … ice … v on-land/hotspots`.

These are **measurements, not placements**. Starting positions, settlements, resources, and balance are game rules, as in wgvc.

## Tuning: early and often

The tuning tool was wgvb's most valuable artifact, and it came too late. Here it is milestone 1.

- `mpg generate --renders DIR` writes one PNG per stage (the right-hand column of the pipeline table), using the same names for every seed.
- `mpg generate --stop-after STAGE` lets tuning iterate on early stages without paying for later ones.
- `mpg sweep --seeds 1-16 --stage elevation,cells,biomes --output sheet.png` builds a contact sheet: seeds × stages, each tile labeled with its key measures. Use it to compare presets and parameter changes side by side.
- Each render records the config hash and stage, so an image always traces back to its inputs.
- Renders never change data or hashes.
- `mpg sweep --rank SPEC` ranks the rows by measures (S36). SPEC is `default` or a comma-separated list of `measure[:max|:min|:~X][*weight]`: higher, lower, or closest to X is better, and the weight defaults to 1.
  - `default` is `usability.habitable_share*2, rivers.touch_share, chokepoints.straits_major, chokepoints.necks, usability.coast_within_share`.
  - A ranking belongs to a sweep, not a world, so it is a flag and never part of `config.json`. Ranking runs every world through the measures stage.
- **Score:** a row's score is 100 · Σ w·p / Σ w, where p is its mid-rank percentile on each key among the rows it is ranked against. Ties count half, and a lone row scores 50.
  - By default, rows are ranked within their aspect × preset block (`--rank-scope group`), because most measures follow the preset; pooling ranks islands above pangaea. `--rank-scope all` pools every row.
  - Rows with a failed gate rank last, and ties keep run order.
- **Outputs:** the sheet reorders rows by rank within each block. Each label starts `#n seed s` and ends with `score …`, and the PNG records `mpg:rank` and the ranked `mpg:rows`.
  - A Markdown table goes to `<output>.rank.md` (or `--table PATH`), with rank, seed, aspect, preset, score, pass, land met, and each key's raw value.
- **Cost:** 20 default worlds take about 69 s; the ranking itself adds nothing measurable.

## Outputs

```text
worlds/example/
  config.json      resolved input (every default written out; seed as a decimal string)
  world.json       the game contract: metadata, codebooks, cells, edges, corners, rivers, coasts, outcomes
  measures.json    playability and validation report
  renders/         stage PNGs (optional)
  fields/          raster dumps for debugging (optional, not a contract)
```

- One `world.json` is fine at this scale: tens of thousands of cells, a few hundred thousand edges.
- Chunking and compression wait for real measurements.
- Outcomes such as sea level, achieved count, search trace, and pass counts go in `world.json`, never in `config.json`.
- Unknown config fields fail with a useful message, to catch misspelled tweaks. Loading an older schema version needs an explicit migration and never silently adopts newer defaults. `config.json` stays at schema 1, frozen from S37: a new input joins it only when its default reproduces the old behaviour, and any other change is schema 2 with a migration that writes the old values out (`internal/config/doc.go`, "Versioning"). Command usage is in `docs/usage.md`.
- The Go types in a small exported package are the schema, as `hmz2map` does, so the engine and converters can import them.

## Determinism

Identical config and pinned generator produce identical `world.json` and checksums.

- stage seeds from SHA-256 over a domain prefix, the world seed, the stage name, and the algorithm version, with a defined byte order and length-prefixed fields; never Go's process-randomized map hashing;
- two derived uint64 values feed a `rand.NewPCG` source owned by each stage; no global source;
- coordinate noise depends only on the seed and coordinates, never on the order work is scheduled;
- parallel work only on independent per-cell or per-sample computations, with reductions in a defined order;
- stable ordering and tie-breaks everywhere;
- no map iteration order in results;
- no NaN or Inf;
- no timestamps or paths in hashed content.

Stage names include `layout`, `elevation`, `warp`, `ridges`, `volcanic`, `mesh`, `climate`, and `edge-noise`.

Cross-machine replay is a stated goal: one gamemaster's config should rebuild the same world on another machine. wgvc hit FMA differences on arm64, so the same safeguards apply here:

- avoid fused-multiply-add contraction in hashed arithmetic, by explicit rounding through `float64(...)` conversions;
- pin noise and transcendental functions;
- run golden-hash tests on both architectures.

## Package layout

```text
cmd/mpg/              generate, sweep, render-stage, validate
internal/config/      resolution, defaults, validation, migration
internal/seed/        stage seed derivation
internal/topo/        cylinder math: wrap, distance, bearing, latitude, rim
internal/field/       raster type, sampling, stage renders
internal/noise/       pinned periodic noise, warp, ridges
internal/layout/      attractors, repulsors, presets
internal/elevation/   heightmap synthesis, volcanic hotspots, rim falloff
internal/mesh/        cylinder Voronoi, Lloyd, short-edge collapse, corner/edge graph
internal/cells/       per-cell statistics, sea level, ocean
internal/climate/     temperature, precipitation, evaporation, runoff
internal/basin/       cell-graph basins, water balance, lakes
internal/river/       corner drainage tree, river edges
internal/classify/    landform, depth, surface, biome
internal/measure/     playability measures and checks
internal/edges/       half-edges, compass directions, coast, incline, passability
internal/export/      products → world types; world.json writer
internal/playermap/   player-style map and windows drawn from world.json alone
internal/render/      stage renders, contact sheets
world/                exported Go types for world.json (the schema)
```

## Milestones

Each milestone ends with renders inspected across several seeds and aspects.

1. **Foundation and the tuning harness:**
   - config resolution, seeds, cylinder topology, and the raster field;
   - stage PNG output and `sweep` contact sheets;
   - proof: a periodic noise field renders seamlessly when shifted across the seam.
2. **Layout and elevation:** presets, attractors and repulsors, volcanic hotspots, and rim falloff. Tune until continents look intentional across seeds.
3. **Mesh:** cylinder Voronoi, Lloyd, short-edge collapse, rim cells, and the mesh checks and render.
4. **First playable export:**
   - cell statistics, sea-level search on cell counts, landforms, depth, and edges with bearing, compass direction, coast, and incline;
   - `world.json` v0 (frozen as v1 in milestone 9) and a player-style render;
   - hand it to the game early.
5. **Climate:** temperature, precipitation with rain shadows, evaporation, runoff, and per-cell aggregation.
6. **Basins and lakes:** cell-graph hierarchy, discrete water balance, lakes and inland seas, dry sinks; the land-target search with basins included.
7. **Rivers:** corner drainage tree, river edges and polylines, and river measures. Tune the threshold against play.
8. **Biomes and surfaces:** biome table, glacier and ice, wetlands.
9. **Playability measures and gates:** chokepoints, landmass classes, habitability; seed ranking; schema freeze for `world.json` v1.

Fixtures (each a test):
- seam-crossing cells: `mesh` TestSeamFixture, TestLloydSeamFixture; `cells` TestHandBuiltSeam;
- seam-crossing rivers: `river` TestSeam, TestSeamRiver;
- rim behavior: `mesh` TestRimFixture;
- short-edge collapse: `mesh` TestCollapseFixture, TestStretchFixture, TestDegreeCapFixture;
- a nested basin with overflow: `basin` TestNestedBasin, TestNestedOverflow;
- a dry basin below sea level: `cells` TestClassifyDryBasin; `basin` TestDryBasin;
- a one-cell lake: `basin` TestOneCellLake;
- a river that must not run along a shore: `river` TestShore, TestShoreRivers.

## Exclusions

No erosion, plate tectonics, axial tilt, seasons, or north–south wrap.

No placement of settlements, resources, starting positions, or borders. Those belong to the game. mpg measures potential; it does not place things.

## Open questions

None at present.
