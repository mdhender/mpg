# mpg implementation plan

## Context

`DESIGN.md` (v0.1.10-alpha) is the agreed design for mpg, a generator of playable Voronoi province maps. The repository has no code yet, only `version.go`.

This plan splits the design into steps, each sized to be one GitHub issue, and orders them by dependency. Once it's approved, we'll discuss how to turn the steps into issues.

The design's milestones group the steps. Inside a milestone, steps whose dependencies are met can run in any order but must run sequentially. Never generate code in parallel agents.

Each step lists:

- **Deps:** steps that must be finished first.
- **Builds:** what it produces.
- **Reuse:** code from earlier projects to copy or adapt.
- **Done when:** the test or render that proves it works.

Every step follows the repo rules: tests pass, the patch version is bumped, the commit references its issue, and the version is tagged.

### Code to reuse (found in sibling repos)

| Source | Use |
|---|---|
| `wgvc/internal/voronoi/` (vendored pzsz Fortune sweep, FMA-safe, no external deps) | Copy as the Voronoi core |
| `wgvc/internal/singlemesh/mesh.go` `Build` | Adapt for Lloyd relaxation |
| `wgvc/internal/fmath/fmath.go`, `wgva/internal/mathx/mathx.go` | Copy as the portable, FMA-safe math functions |
| `wgvc/random.go` `derivedRandom` | Adapt: replace SplitMix64 with SHA-256 |
| `wgva/hash.go` `Hash2`/`Hash3`, `wgva/noise.go` simplex, `wgva/field.go` fBm and warp | Adapt the FMA-safe style; extend to 3-D periodic |
| `maloquacious/wg/voronoi/voronoi.go` `Mesh{Cells,Corners,Edges}` | Model for the cell, corner and edge graph shape |
| `wgvc/rivers.go` `cornerHeap`, `betterDownstream`; `maloquacious/wg/terrain/terrain.go` | Adapt the priority flood and deterministic tie-breaks |
| `maloquacious/wg/rivers/rivers.go` | Adapt the corner-graph accumulation |
| `maloquacious/wg/render/` (`fillPolygon`, `hillshade`, `WritePNG`, `drawWideLine`) | Adapt; standard library `image` only |
| `wgvc/determinism_test.go`, `wgva/golden_test.go` | Template for golden-hash tests run on both architectures |

None of these repos handles east–west wrap, 3-D periodic noise, ridged noise, or SHA-256 seeds. Those are new work.

---

## Milestone 1: Foundation and the tuning harness

**S01. Module skeleton and CLI shell.**
- **Deps:** none.
- **Builds:** `cmd/mpg` with stub subcommands `generate`, `sweep`, `render-stage`, `validate`, plus `-version`; package directories from the design's package layout.
- **Done when:** `go build ./...` passes and `mpg -version` prints the semver.

**S02. Portable math.**
- **Deps:** none.
- **Builds:** `internal/fmath`, with FMA-safe multiply-add, floor div/mod, hypot, atan2, exp, sin and cos.
- **Reuse:** wgvc `fmath`, wgva `mathx`.
- **Done when:** unit tests pin known bit patterns.

**S03. Stage seeds.**
- **Deps:** none.
- **Builds:** `internal/seed`. SHA-256 over a domain prefix, world seed, stage name and algorithm version, with length-prefixed fields and a defined byte order. It returns two uint64 values and builds a `rand.NewPCG` source for each stage. Never a global source.
- **Reuse:** wgvc `derivedRandom`, adapted.
- **Done when:** golden vectors are pinned in tests.

**S04. Cylinder topology.**
- **Deps:** S02.
- **Builds:** `internal/topo`: wrap x, wrapped delta, distance, bearing (clockwise from north), the latitude proxy, and rim/falloff band membership.
- **Done when:** tests cover the seam, poles that don't wrap, and bearings across the seam.

**S05. Config resolution.**
- **Deps:** S01.
- **Builds:** `internal/config`:
  - schema v1 types and defaults;
  - aspect names and ratio parsing;
  - sizing: `A` from `hex_flat_to_flat_mi`, playable cells from N and f, W and H, rim and falloff in km, raster spacing;
  - strict decoding that rejects unknown fields;
  - validation;
  - seed written as a decimal string;
  - resolved `config.json` written out and read back unchanged;
  - config hash.
- **Done when:** the design's example (N = 10,000, f = 0.30, cinematic) resolves to about 2,540 × 1,060 km plus rim; resolve, write, read and re-resolve gives an identical file; a misspelled field fails with a clear message.

**S06. Raster field.**
- **Deps:** S04, S05.
- **Builds:** `internal/field`: a float grid in km coordinates at the resolved spacing, wrap-aware lookup and bilinear sampling, and min/max/percentile helpers.
- **Done when:** tests sample across the seam and check that no duplicate seam column is stored.

**S07. Stage render harness.**
- **Deps:** S05, S06.
- **Builds:** `internal/render`:
  - PNG writer and color ramps;
  - hypsometric coloring and hillshade;
  - stage file naming;
  - config hash and stage name embedded as PNG text chunks.
- **Reuse:** `maloquacious/wg/render`.
- **Done when:** a test field renders, and a render-pixel hash test passes.

**S08. Pipeline runner.**
- **Deps:** S05, S07.
- **Builds:**
  - a stage registry with names fixed by the design's pipeline table;
  - `mpg generate --seed --land-cells --aspect --config --output`, which writes `config.json`;
  - `--renders DIR` and `--stop-after STAGE`.
- **Done when:** `generate` with no stages implemented writes the resolved config, and `--stop-after` is honored.

**S09. Periodic 3-D noise.**
- **Deps:** S02, S03, S06.
- **Builds:** `internal/noise`:
  - pinned 3-D gradient (simplex) noise;
  - cylindrical sampling: `theta = 2π(x + 0.5)/W`, z from latitude;
  - fBm, ridged noise, and a periodic domain warp;
  - algorithm version string.
- **Reuse:** wgva simplex, fBm and warp style, extended to 3-D.
- **Done when:** the milestone 1 proof passes: a noise field shifted across the seam renders seamlessly, checked by a test comparing values either side of the seam and by visual inspection.

**S10. Sweep contact sheet.**
- **Deps:** S08, S09.
- **Builds:** `mpg sweep --seeds 1-16 --stage … --output sheet.png`: a seeds × stages grid with labeled tiles, using `golang.org/x/image/font/basicfont`, no freetype.
- **Done when:** a 16-seed sheet of the noise stage renders.

**S11. Determinism harness and CI.**
- **Deps:** S08.
- **Builds:**
  - a golden-hash test template (environment variable to print the new hash, record it only when both architectures agree);
  - GitHub Actions running `go test ./...` on amd64 and arm64 (`ubuntu-24.04-arm`).
- **Reuse:** wgvc `determinism_test.go`.
- **Done when:** CI is green on both architectures.

## Milestone 2: Layout and elevation

**S12. Layout.**
- **Deps:** S09, S03.
- **Builds:** `internal/layout`:
  - attractors with radius and weight, using periodic distance;
  - repulsors between rival attractors;
  - presets `pangaea`, `continents`, `archipelago`, `islands`, `custom`;
  - the bias field, and its stage render with attractors marked.
- **Done when:** a sweep shows each preset's intent clearly in the bias field.

**S13. Elevation.**
- **Deps:** S12.
- **Builds:** `internal/elevation`:
  - layout bias, fBm, warp and ridges combined into meters (`relief_scale_m`);
  - rim falloff to deep ocean;
  - stage render with hypsometric color and hillshade.
- **Done when:** a sweep across seeds, presets and aspects shows intentional continents, no land in the falloff, and no seam. Golden hash recorded.

**S14. Volcanic hotspots.**
- **Deps:** S13.
- **Builds:**
  - Poisson count (`volcanic_hotspots_per_mkm2`) and uniform placement outside the falloff;
  - cone plus swell added to elevation;
  - the hotspot list carried forward for later steps;
  - hotspots marked on the elevation render.
- **Done when:** counts vary across seeds, including zero, and cones and swells are visible.

## Milestone 3: Mesh

**S15. Cylinder Voronoi.**
- **Deps:** S03, S04, S02.
- **Builds:** `internal/mesh`:
  - jittered-grid sites over the full cylinder, including the rim;
  - ghost sites at ±W;
  - the vendored Fortune sweep;
  - only the original sites' cells kept, clipped north and south inside the rim;
  - a canonical cells/corners/edges graph with wrap-aware geometry and stable ids.
- **Reuse:** wgvc `internal/voronoi`; graph shape from `maloquacious/wg/voronoi`.
- **Done when:** seam fixture tests pass: a cell spanning x = 0 has consistent neighbors, and every edge has exactly two cells or the rim boundary.

**S16. Lloyd relaxation and area scaling.**
- **Deps:** S15.
- **Builds:** Lloyd passes using wrapped centroids, then scaling so the mean cell area is exactly `A`.
- **Reuse:** wgvc `singlemesh.Build`.
- **Done when:** the cell-area coefficient of variation is reported and falls with more passes.

**S17. Short-edge collapse and degree cap.**
- **Deps:** S16.
- **Builds:**
  - deterministic collapse of edges below `min_edge_km`, shortest first;
  - cells that then touch only at a point are no longer neighbors;
  - the degree cap at 8.
- **Done when:** the collapse fixture passes, and on real meshes no edge is shorter than the minimum and every cell has 3–8 neighbors.

**S18. Rim cells, mesh checks, mesh render.**
- **Deps:** S17, S07.
- **Builds:**
  - rim cell flags (`rim`, `impassable`);
  - the mesh checks: edge length, area bounds, neighbor count, corners touching 3–4 cells;
  - mesh renders: overlay on elevation, area heatmap, short edges highlighted.
- **Done when:** checks pass across a seed sweep, and the rim fixture passes.

## Milestone 4: First playable export

**S19. Cell statistics.**
- **Deps:** S18, S13.
- **Builds:** `internal/cells`:
  - each raster sample assigned to its nearest site through a wrap-aware bucket grid;
  - median altitude, relief (p95 − p5) and latitude per cell;
  - cell altitude render.
- **Done when:** a hand-built field gives the known medians, and seam cells collect samples from both sides of the seam.

**S20. Sea level and ocean.**
- **Deps:** S19.
- **Builds:**
  - land candidates by altitude, with rim cells as deep water;
  - ocean found by flooding from the rim;
  - a bounded deterministic sea-level search on land-cell count, with no basins yet;
  - search trace and termination reason saved;
  - land/water render.
- **Done when:** the land count is within 1% of N across a sweep, or the unmet target is reported explicitly; a dry basin below sea level stays land.

**S21. Landform and depth.**
- **Deps:** S20, S14.
- **Builds:** `internal/classify`:
  - `flats` to `mountains` and `plateaus` from relief and altitude, starting from `hmz2ter`'s thresholds;
  - `volcano` flag and `volcanic-highlands`;
  - depth bands in cell steps;
  - landform render.
- **Done when:** the landform histogram looks plausible across a sweep, after a first retune of the thresholds.

**S22. Edge data.**
- **Deps:** S20, S17.
- **Builds:**
  - half-edges in clockwise order;
  - bearing;
  - per-cell compass assignment by dynamic program (unique, order-preserving, smallest total error, with the design's tie-breaks);
  - coast flag and water kind;
  - signed incline computed once per edge and negated for the reverse half-edge;
  - passable flag;
  - incline and passability renders.
- **Done when:**
  - the compass tests pass: hand-made 3- to 8-neighbor cells, and directions never repeated within a cell;
  - incline A→B = −(B→A) for every edge;
  - direction error statistics are reported.

**S23. `world` types and `world.json` v0.**
- **Deps:** S21, S22.
- **Builds:** the exported `world/` package. `world.json` holds:
  - metadata, units and codebooks;
  - cells with geography, altitude, flags, site, centroid, polygon corner ids, unwrapped polygon and bounding box;
  - edges with corner ids, length and an edge-noise seed;
  - corners;
  - coastline polylines;
  - outcomes: sea level, achieved count, search trace.
- **Done when:** the file passes a read-back validator (`mpg validate`), and its golden hash is recorded on both architectures.

**S24. Player-style render from `world.json`.**
- **Deps:** S23, S07.
- **Builds:** `mpg render-stage`, which draws a full map or a window from `world.json` alone:
  - polygons colored by geography;
  - rim drawn as an ice sheet;
  - coastlines;
  - windows that cross the seam.
- **Done when:** a seam-crossing window matches the matching crop of the full render. **Hand `world.json` v0 to the game.**

## Milestone 5: Climate

**S25. Cell mask and temperature.**
- **Deps:** S20.
- **Builds:** `internal/climate`:
  - the cell land/water mask drawn onto the raster;
  - cell temperature from the latitude curve minus lapse-rate cooling at the cell's altitude;
  - temperature render.
- **Done when:** the pole-to-equator gradient and altitude cooling are visible, and rim cells are coldest.

**S26. Precipitation, evaporation, runoff.**
- **Deps:** S25, S09.
- **Builds:**
  - latitude bands and periodic variability;
  - prevailing-wind moisture advection with a fixed iteration count and a deterministic order across the seam;
  - orographic rain and rain shadows;
  - potential evaporation, runoff and aridity, averaged into each cell;
  - renders of each.
- **Done when:** a sweep shows wet windward coasts and dry interiors and lee sides.

## Milestone 6: Basins and lakes

**S27. Basin hierarchy.**
- **Deps:** S20.
- **Builds:** `internal/basin`:
  - a priority flood over the cell graph by altitude, from the ocean;
  - depressions, spill cell, edge and corner, and nesting;
  - `basin_min_depth_m` = 50, with shallower sub-basins merged into their parents;
  - basins render.
- **Reuse:** wgvc `cornerHeap` pattern.
- **Done when:** the nested-basin and below-minimum fixtures pass, and altitudes are never modified.

**S28. Discrete water balance.**
- **Deps:** S27, S26.
- **Builds:**
  - cells filled lowest first until runoff balances evaporation and seepage, or the water reaches the spill;
  - overflow through one spill corner;
  - basins with no stable level left dry, as `playa` and a dry sink;
  - lake, inland sea (20 cells or more) and salt classification;
  - lakes render.
- **Done when:** the overflow, dry-basin and one-cell-lake fixtures pass, and water is conserved within tolerance.

**S29. Land-target search with basins, and climate coupling.**
- **Deps:** S28.
- **Builds:**
  - the search re-runs stages 6 and 8 against a fixed climate;
  - final climate pass with lakes;
  - pass count saved;
  - search trace render.
- **Done when:** the 1% target is met, or reported as unmet, across a sweep with lakes present.

## Milestone 7: Rivers

**S30. Corner drainage tree.**
- **Deps:** S29.
- **Builds:** `internal/river`:
  - corner height as the mean altitude of adjacent cells;
  - a land-corner graph of land–land edges only;
  - terminal corners: water-adjacent, dry sinks;
  - lake outlets at the spill corner;
  - a priority-flood tree with stable tie-breaks.
- **Reuse:** wgvc `assignRivers` heap logic; `maloquacious/wg/rivers`.
- **Done when:**
  - the tree is acyclic;
  - every land corner reaches a terminal corner;
  - the fixture where a river must not run along a shore passes;
  - the seam-crossing river fixture passes.

**S31. Accumulation, river classes, export.**
- **Deps:** S30, S23.
- **Builds:**
  - runoff sent to each cell's lowest corner and accumulated down the tree;
  - `river_threshold_km2` = 500 and class breaks;
  - river class on edges and river polylines in `world.json`;
  - drainage kept internal;
  - river render.
- **Done when:** river class is only ever on land–land edges, and density is reported. **Hand the updated `world.json` to the game.**

## Milestone 8: Biomes and surfaces

**S32. Biomes and surfaces.**
- **Deps:** S26, S28, S31.
- **Builds:**
  - versioned biome table in `hmz2bio` vocabulary;
  - glacier and ice field from cold plus snowfall;
  - pack ice;
  - wetlands from river, shore or low-relief surplus;
  - playa;
  - biome render;
  - biome added to `world.json`.
- **Done when:** a sweep shows cold deserts without glaciers, alpine ice, and humid basins that aren't desert. Player render shows biomes.

## Milestone 9: Playability measures and gates

**S33. Measures framework and basic measures.**
- **Deps:** S18, S20.
- **Builds:** `internal/measure`:
  - `measures.json` and a text summary;
  - checks that are report-only or gates, from config;
  - land, mesh, direction-error and grade measures.
- **Done when:** every `generate` writes the measures, and a failing gate exits nonzero.

**S34. Landmasses and chokepoints.**
- **Deps:** S33, S22.
- **Builds:**
  - landmass count, sizes and classes;
  - straits and necks of at most `k` cells;
  - passes;
  - landmass and chokepoint render.
- **Done when:** hand-built fixtures with known straits and necks are detected.

**S35. Feature, river and usability measures.**
- **Deps:** S33, S31, S32.
- **Builds:**
  - dry basins, basin depth histogram, ice and glacier cells, volcanoes;
  - river density, mouths, longest river;
  - habitable share and distance from the coast.
- **Done when:** the measures appear in sweep tile labels.

**S36. Seed ranking in sweep.**
- **Deps:** S34, S35.
- **Builds:** `sweep --rank` orders seeds by configured measures.
- **Done when:** the ranked sheet and a table are written.

**S37. `world.json` v1 schema freeze.**
- **Deps:** S36.
- **Builds:**
  - schema version set to 1, with migration rules documented;
  - all design fixtures checked present;
  - golden hashes refreshed on both architectures;
  - usage docs for `generate`, `sweep`, `render-stage`, `validate`.
- **Done when:** CI is green, and the game reads v1.

---

## Dependency summary

```text
S01 ─ S05 ─┬ S06 ─ S07 ─ S08 ─┬ S10
S02 ─ S04 ─┘        │          └ S11
S03 ───────── S09 ──┴ S10
S09 ─ S12 ─ S13 ─ S14
S02,S03,S04 ─ S15 ─ S16 ─ S17 ─ S18
S18,S13 ─ S19 ─ S20 ─┬ S21(+S14) ─┐
                     ├ S22(+S17) ─┴ S23 ─ S24
                     ├ S25 ─ S26
                     └ S27 ─ S28(+S26) ─ S29 ─ S30 ─ S31(+S23) ─ S32
S18,S20 ─ S33 ─ S34(+S22), S35(+S31,S32) ─ S36 ─ S37
```

**Execution order:** steps run one at a time, never in parallel. The step numbers S01 → S37 are a valid order: every step's dependencies come before it. Each step's predecessor is the step numbered just before it.

## Verification

- **Each step:** its "done when" test passes in `go test ./...`. Steps with a stage add a render to `--renders`, which is checked by sweep inspection.
- **Each milestone:** a `mpg sweep` over at least 8 seeds and 2 aspects is inspected and attached to the milestone's final issue.
- **Determinism:** golden hashes are recorded from S13 on, and checked on amd64 and arm64 in CI from S11.
- **Game checks:** milestone 4 (S24) and milestone 7 (S31) hand `world.json` to the game.
