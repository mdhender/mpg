# mpg usage

`mpg` generates a playable Voronoi province map. This page is the command
reference; DESIGN.md explains the model, and the Go types in package
`github.com/mdhender/mpg/world` are the file schema.

## Build

```sh
go build ./cmd/mpg
mpg -version          # or: mpg version
mpg -h                # commands; mpg <command> -h for a command's flags
```

Flags take one or two dashes (`-seed` = `--seed`).

## Outputs

`mpg generate --output DIR` writes:

| File | What it is |
|---|---|
| `config.json` | The complete resolved input: every default written out, the seed as a decimal string. Schema 1. |
| `world.json` | The game data file: metadata, codebooks, outcomes, cells, corners, edges, coastlines, rivers. Schema 1, compact JSON (about 55 MB, 13 MB gzipped, at the default 10,000 land cells). |
| `measures.json` | The playability report and the configured checks. Schema 1. |
| `measures.txt` | A one-screen summary of the measures. |

`--renders DIR` adds one PNG per stage render, named
`NN-stage.png` or `NN-stage-variant.png` (for example `07-climate-precip.png`).
Each PNG records the config hash and stage in its text chunks. Renders never
change the data.

The same config and the same generator version always give the same bytes.

### Schema versions

`world.json` and `measures.json` are frozen at schema 1. Any change to their
layout makes a new version. Readers reject schema 0 (the pre-release layout,
which has no migration: generate the world again) and any schema newer than
they know. `config.json` stays at schema 1. A new input joins it only when its
default reproduces the old behavior. See `world/doc.go` ("Versions and
migration") and `internal/config/doc.go` ("Versioning").

## Exit codes

| Code | `generate` | `sweep` | `render-stage` | `validate` |
|---|---|---|---|---|
| 0 | success, report-only check failures included | success | success | the world is valid |
| 1 | config or stage error | config, stage or run error | the world can't be read, is invalid, or doesn't fit the window | invalid or unreadable world, or it doesn't match `--config` |
| 2 | usage error | usage error | usage error | usage error |
| 3 | a gate check failed; every output is still written | — | — | — |

## Stages

| # | Name | Render variants |
|---|---|---|
| 1 | `config` | — |
| 2 | `layout` | — |
| 3 | `elevation` | — |
| 4 | `mesh` | `area`, `short` |
| 5 | `cells` | `relief` |
| 6 | `sea-level` | — |
| 7 | `climate` | `mask`, `precip`, `moisture`, `pet`, `runoff`, `aridity` |
| 8 | `basins` | `depth`, `lakes` |
| 9 | `land-target` | `lakes`, `precip`, `aridity` |
| 10 | `rivers` | `tree`, `catchments` |
| 11 | `classify` | `biome`, `surface`, `wetness` |
| 12 | `edges` | `compass`, `passability` |
| 13 | `measures` | — |
| 14 | `export` | — (the player-style map) |

Stages are named by name or number wherever a stage is asked for.

## generate

```text
mpg generate [--config FILE] [--seed N] [--land-cells N] [--aspect A]
             [--preset P] [--renders DIR] [--stop-after STAGE] --output DIR
```

| Flag | Meaning |
|---|---|
| `--output DIR` | Required. Created if missing; files are overwritten. |
| `--config FILE` | Start from this config, read strictly: unknown fields fail, with the closest name suggested. Default: the built-in defaults. A partial file is fine; the missing fields keep their defaults. |
| `--seed N` | World seed, a decimal uint64. |
| `--land-cells N` | Requested land cells, N. The land count lands within 1% of N, or the miss is reported. |
| `--aspect A` | Playable aspect: `square`, `portrait`, `landscape`, `widescreen` (16:9), `cinematic` (2.39:1), or `W:H`. |
| `--preset P` | Layout preset: `pangaea`, `continents`, `archipelago`, `islands`, or `custom`. |
| `--renders DIR` | Write the stage renders. |
| `--stop-after STAGE` | Stop after this stage. Later outputs aren't written. |

`--seed`, `--land-cells`, `--aspect` and `--preset` override the config. The
resolved result is what `config.json` holds. stdout gets the config hash,
seed, size, stages run and the check verdict. Logs and failed checks go to
stderr.

```sh
mpg generate --seed 42 --output worlds/42
mpg generate --config worlds/42/config.json --output /tmp/replay   # same bytes
mpg generate --seed 7 --aspect square --preset pangaea --renders r --stop-after elevation --output /tmp/t
```

## sweep

```text
mpg sweep --seeds SEEDS --stage STAGES [--aspect A,...] [--preset P,...]
          [--config FILE] [--land-cells N] [--tile PX]
          [--rank SPEC [--rank-scope group|all] [--table FILE]] --output FILE
```

Builds a contact sheet PNG. There is one row per aspect × preset × seed (aspects
outermost, seeds innermost) and one column per requested stage. Each tile is
that stage's render with captions.

| Flag | Meaning |
|---|---|
| `--seeds SEEDS` | Required. Values and ranges: `1-16`, `1,5,9-12`. |
| `--stage STAGES` | Required. Comma-separated stage names or numbers, each optionally `stage:variant` (see Stages), as in `elevation,climate:precip,classify:biome,measures`. |
| `--aspect`, `--preset` | Comma-separated lists, one block of rows each. They override the config. |
| `--config`, `--land-cells` | As for `generate`. |
| `--tile PX` | Tile width (default 320). The height follows the world's shape. |
| `--output FILE` | Required. The sheet PNG. |
| `--rank SPEC` | Rank the rows by measures (see below). This runs every row through the measures stage. |
| `--rank-scope` | `group` (default): rank within each aspect × preset block. `all`: pool every row. |
| `--table FILE` | Write the Markdown rank table here. Default: the output path with `.rank.md` in place of its extension. |

Row labels show the seed and the land count against N (`UNMET` past 1%). They
end in `GATE` when a gate check failed; a failed gate never stops the sweep.
Each stage's tile caption carries its key numbers. A `measures` column has
five lines:

- landmasses as `LM n (c/i/.)`, straits `all/major`, necks and passes;
- check counts;
- `hab`, `wet`, `c<d>`, `riv`, `mo`, `L` (habitable and wetland shares,
  coast share within d cells, river edges per land cell, mouths, longest
  river);
- `dep`, `bas`, `dry`, `pl`, `ice`, `v` (depressions, basins, dry basins,
  playas, ice cells, volcanoes on land/hotspots).

**Rank SPEC.** Either `default` or comma-separated items
`measure[:max|:min|:~X][*weight]`.

- A measure is a `measures.json` scalar path, such as `land.cells` or
  `usability.habitable_share`.
- `max` (the default) ranks higher values better, `min` lower values better,
  and `~X` values closer to X better.
- The weight defaults to 1.
- `default` means
  `usability.habitable_share*2,rivers.touch_share,chokepoints.straits_major,chokepoints.necks,usability.coast_within_share`.

A row's score is 100 · Σ w·p / Σ w, where p is its mid-rank percentile on each
key. Rows with a failed gate rank last, and ties keep run order. The sheet
reorders rows by rank within each block.

```sh
mpg sweep --seeds 1-8 --aspect square,cinematic --stage elevation,classify:biome,measures --output sheet.png
mpg sweep --seeds 1-16 --preset continents,islands --stage export --rank default --output ranked.png
```

## render-stage

```text
mpg render-stage [--window x,y,w,h] [--scale px/km | --width px] --output FILE PATH
```

Draws the player-style map from `world.json` alone. PATH is the file or the
directory holding it.

| Flag | Meaning |
|---|---|
| `--output FILE` | Required. The PNG. |
| `--window x,y,w,h` | Draw only this window, in km from the northwest corner. x is taken modulo W, so a window may cross the east–west seam. Default: the whole map. |
| `--scale PX` | Pixels per km (default 1). |
| `--width PX` | Full-map width in pixels; sets the scale instead of `--scale`. |

```sh
mpg render-stage --width 4000 --output map.png worlds/42
mpg render-stage --window 2400,300,400,250 --scale 4 --output seam.png worlds/42
```

## validate

```text
mpg validate [--config FILE] PATH
```

Reads `world.json` strictly and checks its structural invariants (ids and
references, codebooks, polygons, exits and compass order, incline symmetry,
corners, coastlines, rivers, and outcome counts). PATH is the file or its
directory. With `--config FILE` it also checks that the world was generated
from that config: config hash, seed and sizes. A schema 0 or newer-schema
file is rejected with a message naming the version.

```sh
mpg validate worlds/42
mpg validate --config worlds/42/config.json worlds/42
```
