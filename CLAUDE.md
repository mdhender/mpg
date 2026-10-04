# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

@AGENTS.md

## Project state

`mpg` (`github.com/mdhender/mpg`, Go 1.26) is a deterministic, noise-based fantasy world generator. The repository is
pre-implementation: only the root package (`version.go`) exists. `DESIGN.md` is the authoritative design — read the
relevant section before implementing any stage, and follow its "Implementation order" (foundation → geography preview →
world format → basins → climate → water balance/rivers → cryosphere/biomes → usability). Its "Package layout" section
gives the planned `cmd/` and `internal/` structure; keep APIs under `internal/`.

## Commands

```sh
go build ./...
go vet ./...
go test ./...
go test ./internal/grid -run TestWrap   # single package / single test
```

Planned CLIs (per `DESIGN.md`): `cmd/mpg` (`mpg generate ...`) and `cmd/mpg-render` (reads saved worlds only).

## Versioning

`version.go` holds a `semver.Version` (`github.com/maloquacious/semver`); `Build` is filled from VCS info via
`semver.Commit()`. Bump `Patch` there on every commit, per AGENTS.md.

## Design contracts that span stages

These are easy to violate and hard to spot from any single package:

- **Topology:** grid is `(x, y)` from the northwest corner, row-major, sample index `y*width+x`. East–west wraps
  (positive modulo); north–south never wraps — out-of-range neighbors are absent, not clamped or treated as ocean.
  All stages and the renderer must use one shared topology helper. No duplicate seam column is ever stored.
- **Periodic noise:** sample noise on a cylinder (`theta = 2π(x+0.5)/width`, z from latitude); domain warps must also be
  periodic. Matching first/last columns is not sufficient.
- **Three distinct units:** a requested land cell is 100 km² of *final dry land*; a generation sample has
  resolver-chosen size (`samples_per_game_cell_side` ∈ e.g. 1/2/5/10); a game cell is 10 km × 10 km and belongs to a
  future export step. All arrays, chunks, and coordinates are in samples; physics uses resolved sample spacing in meters.
- **Land-area target:** final dry area (after inland-water classification; land ice counts, ice over water does not)
  must be within 1% of `land_cells × 100 km²`, found by a bounded deterministic sea-level search. Report an unmet target
  explicitly — never fudge samples or silently change the request.
- **Determinism:** derive per-stage seeds via SHA-256 over domain prefix, world seed, stage name, and algorithm version,
  feeding two uint64s into `rand.NewPCG`. Never use map iteration order, worker scheduling, or a global RNG. Break ties
  by stable cell index; reduce in a defined order; no NaN/Inf in output; no timestamps or absolute paths in hashed content.
- **Hydrology preserves terrain:** keep original elevation separate from any filled working surface. Depressions are
  basins whose wet/dry/seasonal state comes from water balance, not compulsory filling. Drainage graphs must be acyclic.
- **Config vs. manifest:** `config.json` is the complete resolved input (seed as decimal string, unknown fields
  rejected, every default written out); calculated outcomes such as sea level go in `map.json`. Replaying a config must
  never consult current defaults. Output is a manifest plus per-chunk JSON files (default 64×64 samples).
- **Separation of concerns:** geography, climate, biomes, and rendering stay distinct; rendering never changes
  generation data or config hashes.
