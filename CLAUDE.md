# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

@AGENTS.md

## Project state

`mpg` (`github.com/mdhender/mpg`, Go 1.26) generates a playable Voronoi **province map** for our strategic fantasy
games. Noise, climate, and hydrology on a raster feed a mesh; the mesh is the product. The repository is
pre-implementation: only the root package (`version.go`) exists. `DESIGN.md` is the authoritative design — read the
relevant section before implementing any stage, and follow its "Milestones" (tuning harness first, then layout and
elevation, mesh, a first playable export, climate, basins, rivers, biomes, measures). Its "Package layout" gives the
planned `cmd/`, `internal/`, and exported `world/` packages.

## Commands

```sh
go build ./...
go vet ./...
go test ./...
go test ./internal/topo -run TestWrap   # single package / single test
```

Planned CLI (per `DESIGN.md`): `cmd/mpg` with `generate`, `sweep`, `render-stage`, `validate`.

## Versioning

`version.go` holds a `semver.Version` (`github.com/maloquacious/semver`); `Build` is filled from VCS info via
`semver.Commit()`. Bump `Patch` there on every commit, per AGENTS.md.

## Design contracts that span stages

These are easy to violate and hard to spot from any single package:

- **Province = cell.** Each Voronoi cell has the *area* of one 6-mile wilderness hex (≈80.75 km², derived from
  `hex_flat_to_flat_mi`), so exploring one takes a game day. Every cell is wholly land or wholly water. The land target
  is a count of land cells within 1% of N, found by a bounded deterministic sea-level search; report an unmet target.
- **Topology:** km coordinates from the northwest corner; east–west wraps, north–south never does. One shared
  topology package for raster and mesh. Noise and domain warps are sampled on a cylinder, so they are truly periodic.
- **Polar rim:** top and bottom bands of `rim` cells, an impassable ice sheet, behind an ocean falloff, so land never
  touches the edge. Ocean is water connected to the rim.
- **One height.** Cell altitude (median of its raster samples) drives every height rule: land test, basins, lakes,
  corner heights, incline, landforms, temperature. Relief (p95 − p5) is roughness only.
- **Edges carry the game data:** a unique clockwise 8-point compass direction per cell (≤ 8 neighbors; short edges
  are collapsed), signed grade incline where A→B = −(B→A), coast, and river class. Rivers run only on land–land
  edges along a corner drainage tree; never through cells, never along shores.
- **Possible, not forced:** dry basins, lakes, inland seas, ice, glaciers, and volcanoes emerge or don't; measures
  report them and never require them.
- **Determinism:** per-stage seeds via SHA-256 → `rand.NewPCG`; stable tie-breaks by index; no map-order or
  scheduling dependence; no NaN/Inf; no timestamps or paths in hashed content; guard against FMA differences.
- **Files:** `config.json` is the complete resolved input (unknown fields rejected); `world.json` is the one game data
  file (game data plus renderer geometry, outcomes); `measures.json` is the playability report. Exported Go types are
  the schema. Rendering never changes data or hashes.
- **Tuning early and often:** every stage has a render; keep `--renders`, `--stop-after`, and `sweep` working.
