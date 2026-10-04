# mpg — Counter-proposal: a province-map generator

Status: counter-proposal to `DESIGN.md` (2026-10-04). Nothing is implemented.

## Goal

mpg generates a **playable province map** for our strategic fantasy games. The deliverable is a Voronoi mesh with these properties:

- Every cell is a **province** with the area (not the shape) of one 6-mile wilderness hex, about 81 km². That lets the game set exploring a province at one day.
- Every cell is **all land or all water**.
- Every cell carries geography (landform), biome, and altitude.
- Every edge carries direction, neighbor, river, coast, and incline.
- The mesh includes enough geometry for the game's map renderer to draw PNG segments for players.

The noise, climate, and hydrology work exists to make that mesh interesting. It is not the product.

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

## Keep, change, add, drop

Compared with `DESIGN.md`:

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
| **Change** | North and south edges become a **polar rim** of impassable deep ocean and pack ice, instead of bare bounded edges. |
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

Aspect names (square, portrait, landscape, widescreen 16:9, cinematic 2.39:1) and explicit ratios work as in `DESIGN.md`. Aspect applies to the playable area. The rim adds height only.

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
- **Falloff.** Inside the rim's edge, a falloff band (default about 12 cells) pulls elevation smoothly down to deep ocean. Land therefore never touches the rim, and coasts near the poles look natural rather than cropped.
- **Rim cells** are salt water, depth `deep`, flagged `rim` and `impassable`. Climate makes them pack ice.

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
| 3 | Elevation | raster | Bedrock elevation in meters (layout + fBm + warp + ridges + rim falloff) | Hypsometric map with hillshade |
| 4 | Mesh | mesh | Sites, Lloyd relaxation, short-edge collapse, cells, corners, edges, rim cells | Mesh over elevation; area heatmap; short edges highlighted |
| 5 | Cell statistics | raster → mesh | Per-cell elevation min, p5, median, p95, max; relief; latitude | Cell median elevation |
| 6 | Sea level and ocean | mesh | Sea level and ocean cells (flood from the rim) | Land/water cells |
| 7 | Climate | raster (+ cell mask) | Temperature, precipitation, potential evaporation, runoff, aggregated to cells | Temperature; precipitation; aridity |
| 8 | Basins and lakes | mesh | Basin hierarchy, wet/dry decision, lake and inland-sea cells, spill corners | Basins colored; lakes |
| 9 | Land-target check | mesh | Repeats stages 6 and 8 within a budget until the land count is in tolerance | Search trace |
| 10 | Rivers | mesh corners | Drainage tree on corners; river edges with drainage and discharge | Rivers on edges, width by class |
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

### Layout

Noise alone produces either one featureless block or lace. wgvc needed attractors and a rival ramp to control islands. Here that control is added to the continental bias field, before the noise:

- **Seeded attractor points** on the cylinder, using periodic distance, each with a radius and weight. They can be placed by a preset pattern or randomly with minimum spacing.
- **Repulsors** between rival attractors, so continents stay separate. The gaps they leave become straits.
- **Presets:** `pangaea`, `continents` (3–5), `archipelago`, `islands` (wgvc-style), and `custom`.

Layout is a bias, not a mask. Noise still makes the coastlines; the playability measures in stage 13 report whether the intent survived.

### Mesh

- **Sites.** Jittered grid or Poisson-disk sites over the full cylinder, including the rim, seeded from `mesh`. Site count = total area / `A`.
- **Voronoi on a cylinder.** Compute with ghost copies of the sites shifted ±W. Keep only the original sites' cells. North and south are clipped inside the rim, so clipping never affects playable cells.
- **Lloyd relaxation.** Default 2–3 passes, using wrapped centroids. Then scale so the mean cell area is exactly `A`.
- **Short-edge collapse.** Any edge shorter than `min_edge_km` is collapsed: its two corners merge into one 4-way corner. Default `min_edge_km` = 0.3 × √A, about 2.7 km.
  - The two cells that shared the collapsed edge now touch only at a point, so they are **not neighbors**.
  - Point contact never connects land to land or water to water, which matches `DESIGN.md`'s four-neighbor water rule.
  - The collapse is deterministic, shortest edge first, with ties broken by edge index.
- **Degree cap.** Every cell needs a distinct compass direction for each neighbor, so no cell may have more than 8 neighbors. After the short-edge collapse, a cell with 9 or more neighbors has its shortest edge collapsed, repeating until it has 8. Lloyd-relaxed cells have mostly 5–7 neighbors, so this should be rare; report how often it happens.
- **Mesh checks:**
  - no edge shorter than `min_edge_km`;
  - cell areas within configured bounds (default 0.5A to 1.6A);
  - neighbor counts within 3–8;
  - every corner touches 3–4 cells.

### Cell statistics and sea level

- **Sample assignment.** Each raster sample belongs to the cell whose site is nearest, using wrapped distance and a bucket grid. Statistics follow `hmz2ter`: elevation min, p5, median, p95, and max, using nearest-rank percentiles; relief = p95 − p5.
- **Land test.** A cell's **altitude** is its median elevation. A non-rim cell is a land candidate if its altitude is above sea level.
- **Ocean.** Water candidates connected to the rim are ocean. Unconnected candidates below sea level are basin floors, which stage 8 decides.
- **Sea-level search.**
  - The initial estimate is the quantile that leaves `N` + (expected lake cells) land candidates.
  - After that, run a bounded deterministic search over candidate cell altitudes, keeping the best measured result.
  - Basins can jump, so the land count is not assumed to change monotonically.
  - Save the policy, budget, chosen level, achieved count, and termination reason.

### Basins and lakes

- **Priority flood.** A priority flood over the cell graph, seeded from ocean cells and using cell p5 elevation, finds depressions, spill cells, spill edges, and the nested hierarchy. Original elevations are never modified.
- **Water balance** is a discrete fill, because every cell is one whole area unit:
  1. Sort each basin's cells by elevation.
  2. Fill them in that order.
  3. Stop when evaporation from the lake area plus seepage balances catchment runoff, or when the spill level is reached.
- **Surplus at the spill** overflows through **one spill corner**, which became a wgvc rule for good reason, and continues downstream.
- **No stable level** means the basin stays dry land with surface `playa` and is a **dry sink** for rivers.
- **Classification:**
  - lake: 1 to `inland_sea_min_cells` − 1 cells;
  - inland sea: `inland_sea_min_cells` or more;
  - each is marked salt if endorheic and evaporation-dominated (a salinity proxy, not chemistry).
- A lake is one connected set of cells with one surface level.

### Rivers on edges

Rivers run along Voronoi edges, corner to corner, and never through a cell's interior. This follows mapgen2, wgvc, and `hmz2riv`'s snapped edges.

- **Corner elevation** is the raster elevation sampled at the corner, with a light 3×3 median, as `hmz2ele` samples corners.
- **Corner graph.** The graph has land corners joined by **land–land edges**. A corner that touches any water cell is **terminal**: a mouth on the ocean or a lake. Dry-sink corners from stage 8 are also terminal.
- **Drainage tree.** A priority flood over the corner graph, seeded from terminal corners, gives every land corner one downstream corner. The result is acyclic by construction. Flats are broken by stable index.
- **Lake outlets.** An overflowing lake's spill corner starts its own downstream path, carrying the lake's surplus.
- **Accumulation.**
  - Each land cell sends its runoff (area × runoff depth) to its lowest corner on the graph.
  - Accumulate down the tree.
  - Each edge on the tree carries the drainage (km²) and discharge (m³/s) of its downstream corner.
- **River selection.**
  - An edge is a river when its drainage is at least `river_threshold_km2`.
  - Classes (stream, river, major river) come from drainage or discharge breaks.
  - Each cell is about 81 km², so the threshold has to span several cells. Start around 500 km² and tune.
  - For comparison: on Panama, `hmz2riv`'s 50 km² gave about one river edge for every two 10 km hexes, chosen deliberately to slow north–south travel.

This guarantees by construction:

- Rivers are only ever on edges.
- Rivers never run along a coast or lake shore, because those edges are excluded from the graph.
- Every river ends at a mouth, a lake, or a dry sink.
- Rivers never cross the rim.
- Rivers cross the east–west seam naturally.

### Classification

Use the hm* codebooks where they fit, so the engine and converter tools stay familiar.

- **Landform (land), from relief and altitude:**
  - `flats`, `plains`, `rolling-plains`, `hills`, `mountains`, `plateaus`, using `hmz2ter`'s relief thresholds as starting values. Retune them, because relief within an 81 km² cell of synthetic terrain will not match DEM relief.
  - `volcanic-highlands` stays in the codebook, but nothing produces it until a volcanism stage exists.
- **Landform (water):** `salt-water` (ocean, inland sea) and `fresh-water` (lake). Salinity is a flag.
- **Depth (salt water):** `shallow`, `open`, `deep`, from distance in cell steps to the nearest non-salt-water cell, as in `hmz2ter`. The bands are play rules, so they are retuned in cells.
- **Surface and biome:**
  - From temperature, precipitation and aridity, and wetness. The versioned lookup table follows `DESIGN.md`'s candidates and `hmz2bio`'s vocabulary.
  - Glacier and ice field are **surfaces** on land cells. Permanent ice needs cold plus enough snowfall.
  - Pack ice is a surface on water.
  - Wetlands need a hydrological wetness signal: river edges, a lake shore, or low relief with surplus.

### Edges

The game wants per-edge data. Store each undirected edge once, and give each cell an ordered list of half-edges.

- **Bearing.** Degrees clockwise from north, from this cell's site to the neighbor's site, using the wrapped delta.
- **Direction.** The game's direction for the edge: one of the 8 compass points `N`, `NE`, `E`, `SE`, `S`, `SW`, `W`, `NW`, ordered clockwise.
  - **Unique per cell.** No two edges of a cell share a direction, so an order like "move NE" is never ambiguous. A cell with fewer than 8 neighbors leaves some directions unused, and moving in an unused direction is not possible.
  - **Order-preserving.** Sort the cell's edges by bearing. Directions follow the same clockwise order, so the edges and the rose never cross.
  - **Closest fit.** Among order-preserving assignments, choose the one with the smallest total angular error between each bearing and its compass point's angle (multiples of 45°). With at most 8 edges this is a small dynamic program per cell. Break ties by the lower maximum error, then by the assignment that starts at the earliest compass point.
  - **Not symmetric.** If A's edge to B is `NE`, B's edge to A is usually, but not always, `SW`. Both directions are stored, one on each half-edge.
  - **Error recorded.** Store each edge's angular error from its bearing. wgvc found that nearest-point labels on a Voronoi mesh collide often (193 of 300 provinces had a duplicate), which is why the assignment is solved per cell instead.
- Cells list their half-edges in clockwise order, starting from the one nearest `N`.
- **Neighbor.** The neighbor cell id and the shared edge id.
- **Coast.** Set when exactly one side is water. The water kind is ocean, lake, or inland sea.
- **River.** Set when the edge is a river edge. Record its class, drainage, discharge, and flow direction (`from_corner` → `to_corner`). A river edge is always land–land, so it is a border between provinces.
- **Incline.** (neighbor altitude − this altitude) / site distance, in m/km, signed from this cell. Classify it (level, gentle, steep, cliff), with tunable thresholds.
- **Passable.** False across the rim. The game may add rules on top. Short edges are already gone, so every remaining edge is a real border.

### Geometry for the game renderer

`world.json` includes:

- **Corners:** id, `(x, y)` in km, elevation, terminal or mouth flags.
- **Cells:** site, centroid, a polygon as a clockwise list of corner ids, an unwrapped polygon in km relative to the site (so seam cells draw without special cases), and a bounding box.
- **Edges:** corner ids at each end, length in km, and a per-edge seed, so the renderer can draw deterministic noisy edges for coasts and rivers if it wants to.
- **River polylines:** corner chains from source to mouth, with class and discharge, ready for drawing by width.
- **Coastline polylines:** chains of coast edges, closed for islands and lakes.
- **World metadata:** W, H, rim, wrap flag, province area, units, codebooks.

Rendering a player's map segment means selecting cells whose bounding boxes intersect the window (taken modulo W) and drawing their polygons, rivers, and coasts. No raster is needed.

## Playability measures

Write `measures.json` and a short text summary on every run. Configured checks fail loudly. Sweeps rank seeds by these measures.

- **Land:** land cell count against N; land area; cell area mean and coefficient of variation; edge length minimum and p5; neighbor-count histogram; cells that needed the degree cap; direction error (mean, p95, max) and how often a reverse direction is not the opposite point.
- **Landmasses:** count, size histogram, largest share of land, and count by class (continent, island, islet) using cell-count thresholds.
- **Water:** ocean, inland sea, and lake counts and sizes; coast edges per land cell.
- **Chokepoints:**
  - **straits**: water crossings of at most `k` cells between landmasses or between parts of one landmass;
  - **necks**: land isthmuses of at most `k` cells;
  - **passes**: low-incline routes through mountain chains.
  These come from wgvc, measured in cells.
- **Rivers:** river edges per land cell, mouths, longest river in edges, share of land cells touching a river.
- **Usability:** habitable share of land (not glacier, desert, mountain, or polar desert); biome and landform histograms; land within `d` cells of the coast.

Checks begin as report-only. Promote them to gates once tuning shows sensible ranges.

These are **measurements, not placements**. Starting positions, settlements, resources, and balance are game rules, as in wgvc.

## Tuning: early and often

The tuning tool was wgvb's most valuable artifact, and it came too late. Here it is milestone 1.

- `mpg generate --renders DIR` writes one PNG per stage (the right-hand column of the pipeline table), using the same names for every seed.
- `mpg generate --stop-after STAGE` lets tuning iterate on early stages without paying for later ones.
- `mpg sweep --seeds 1-16 --stage elevation,cells,biomes --output sheet.png` builds a contact sheet: seeds × stages, each tile labeled with its key measures. Use it to compare presets and parameter changes side by side.
- Each render records the config hash and stage, so an image always traces back to its inputs.
- Renders never change data or hashes.

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
- The Go types in a small exported package are the schema, as `hmz2map` does, so the engine and converters can import them.

## Determinism

The determinism rules from `DESIGN.md` carry over unchanged:

- stage seeds from SHA-256 over domain, world seed, stage name, and algorithm version;
- `rand.NewPCG` sources owned by each stage;
- stable ordering and tie-breaks everywhere;
- no map iteration order in results;
- no NaN or Inf;
- no timestamps or paths in hashed content.

Stage names include `layout`, `elevation`, `warp`, `ridges`, `mesh`, `climate`, and `edge-noise`.

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
internal/elevation/   heightmap synthesis and rim falloff
internal/mesh/        cylinder Voronoi, Lloyd, short-edge collapse, corner/edge graph
internal/cells/       per-cell statistics, sea level, ocean
internal/climate/     temperature, precipitation, evaporation, runoff
internal/basin/       cell-graph basins, water balance, lakes
internal/river/       corner drainage tree, river edges
internal/classify/    landform, depth, surface, biome
internal/measure/     playability measures and checks
internal/render/      stage renders, contact sheets
world/                exported Go types for world.json (the schema)
```

## Milestones

Each milestone ends with renders inspected across several seeds and aspects.

1. **Foundation and the tuning harness:**
   - config resolution, seeds, cylinder topology, and the raster field;
   - stage PNG output and `sweep` contact sheets;
   - proof: a periodic noise field renders seamlessly when shifted across the seam.
2. **Layout and elevation:** presets, attractors and repulsors, and rim falloff. Tune until continents look intentional across seeds.
3. **Mesh:** cylinder Voronoi, Lloyd, short-edge collapse, rim cells, and the mesh checks and render.
4. **First playable export:**
   - cell statistics, sea-level search on cell counts, landforms, depth, and edges with bearing, compass direction, coast, and incline;
   - `world.json` v0 and a player-style render;
   - hand it to the game early.
5. **Climate:** temperature, precipitation with rain shadows, evaporation, runoff, and per-cell aggregation.
6. **Basins and lakes:** cell-graph hierarchy, discrete water balance, lakes and inland seas, dry sinks; the land-target search with basins included.
7. **Rivers:** corner drainage tree, river edges and polylines, and river measures. Tune the threshold against play.
8. **Biomes and surfaces:** biome table, glacier and ice, wetlands.
9. **Playability measures and gates:** chokepoints, landmass classes, habitability; seed ranking; schema freeze for `world.json` v1.

Fixtures: seam-crossing cells and rivers; rim behavior; short-edge collapse; a nested basin with overflow; a dry basin below sea level; a one-cell lake; a river that must not run along a shore.

## Exclusions

No erosion, plate tectonics, axial tilt, seasons, or north–south wrap.

No placement of settlements, resources, starting positions, or borders. Those belong to the game. mpg measures potential; it does not place things.

## Open questions

1. **Incline.** Are m/km plus a class enough, or does the engine want degrees or movement-cost bands?
2. **River threshold and classes.** How dense should the network be? Does the engine want discharge, or only a class?
3. **Lakes.** Allow one-cell lakes? What `inland_sea_min_cells` should apply? May a river enter a lake and leave it from a different corner? Proposed: yes, through the spill corner only.
4. **Renderer geometry.** Is cell and edge geometry with an optional noisy-edge seed enough, or does the player map also want a hillshade raster per segment?
5. **Volcanism.** Should `volcanic-highlands` get a hotspot stage, or stay unused for now?
6. **Rim depth and look.** Is 4 cells of impassable rim plus about 12 cells of falloff right? Should the rim render as open polar sea, pack ice, or both?
