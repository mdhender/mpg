// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/golden"
	"github.com/mdhender/mpg/internal/pipeline"
	"github.com/mdhender/mpg/internal/playermap"
	"github.com/mdhender/mpg/internal/render"
	"github.com/mdhender/mpg/world"
)

// TestGoldenElevation pins today's end-to-end output across architectures
// (see package golden for how to record a hash): the pipeline run through
// stage 3, elevation. Each case hashes, in order:
//
//   - the resolved config.json bytes (golden.Hasher.Bytes, length-prefixed);
//   - the raster's NX and NY (golden.Hasher.Int);
//   - every sample of the full-resolution bedrock elevation field, in
//     meters, as float64 bits, little-endian, in storage order: rows north
//     to south, columns west to east;
//   - the volcanic hotspot count (golden.Hasher.Int), then each hotspot's
//     X, Y, PeakM, ConeRadiusKm, SwellM and SwellRadiusKm as float64 bits,
//     in draw order.
//
// The field is hashed rather than the rendered image, because the color ramp
// quantizes away exactly the last-place differences a fused multiply-add
// makes. The first case is the design example (seed 42, cinematic), and its
// config bytes are checked against testdata/example.json as well. Later
// stages add world.json hashes to this template.
func TestGoldenElevation(t *testing.T) {
	example, err := os.ReadFile("../../internal/config/testdata/example.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		seed   config.Seed
		aspect string
		preset string
		want   string
	}{
		{"seed42-cinematic", 42, "cinematic", "continents", "9ce30584c0b072ec2e0832ced59713ea25814be2784615f89d890d87ad6b368f"},
		{"seed42-square", 42, "square", "continents", "795e2ca726d45bbf0738b8a46a01decb7b5fb9d300fb7fd862e9829aabd289c5"},
		{"seed7-cinematic", 7, "cinematic", "continents", "9d69ea3580c324c4dcdeb07de5dae54bb9141144a7d132d6579ab29c40d2c43d"},
		{"seed7-square", 7, "square", "continents", "098a3733d0b15447bd106a863d401c46114c9919e5febc714cfe7966c0af2c9f"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "ea024d5c49f17097be786f200ad8d3052c8ec3137513e42a786b09cd75f1eddc"},
		{"seed42-square-islands", 42, "square", "islands", "b78a06d923173c49a0c808c36c187642619af83b8459f2ca932d7cbcb2eb0463"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := config.Default()
			cfg.Seed = tc.seed
			cfg.World.Aspect = tc.aspect
			cfg.Layout.Preset = tc.preset
			if err := cfg.Resolve(); err != nil {
				t.Fatal(err)
			}
			cfgBytes, err := cfg.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			if tc.name == "seed42-cinematic" && !bytes.Equal(cfgBytes, example) {
				t.Errorf("config.json differs from testdata/example.json:\n%s", cfgBytes)
			}
			ctx, err := pipeline.NewContext(cfg, t.TempDir(), "", nil)
			if err != nil {
				t.Fatal(err)
			}
			stages := pipeline.Stages()
			last, err := pipeline.Lookup(stages, "elevation")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pipeline.Run(ctx, stages, last); err != nil {
				t.Fatal(err)
			}
			f := ctx.Products.Elevation
			h := golden.New()
			h.Bytes(cfgBytes)
			h.Int(f.NX())
			h.Int(f.NY())
			h.Float64s(f.Values()...)
			h.Int(len(ctx.Products.Hotspots))
			for _, p := range ctx.Products.Hotspots {
				h.Float64s(p.X, p.Y, p.PeakM, p.ConeRadiusKm, p.SwellM, p.SwellRadiusKm)
			}
			golden.Check(t, "elevation/"+tc.name, h.Sum(), tc.want)
		})
	}
}

// TestGoldenMesh pins the mesh stage across architectures (see package
// golden for how to record a hash): the pipeline run through stage 4, mesh.
// Each case hashes, in order, the resolved config.json bytes
// (golden.Hasher.Bytes, length-prefixed), then the mesh's canonical
// encoding (mesh.Mesh.AppendBinary: W and H; the counts; each cell's site,
// corner ids, edge ids, neighbor ids, and rim and impassable flags; each
// corner's position, boundary flag, cell ids and edge ids; each edge's
// cells and corners; all in id order).
func TestGoldenMesh(t *testing.T) {
	for _, tc := range []struct {
		name   string
		seed   config.Seed
		aspect string
		want   string
	}{
		{"seed42-cinematic", 42, "cinematic", "7df631c5c3eb0e1dc68ca95b88b28c42cc886ad3c8044b270876d6a667efed2d"},
		{"seed42-square", 42, "square", "77b4abe92f8d9d7fada50cca7978ad6dcab095893ca558c2856873f4a9a5f892"},
		{"seed7-cinematic", 7, "cinematic", "9f970fcccb7747a00d99803454bee24248e0c8a704a8c7a99a71b58105659da3"},
		{"seed7-portrait", 7, "portrait", "3af650358c8e2c12beb17e66b879a16b15c4d818e5a0c22b96aa4cb3acaeb7e6"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := config.Default()
			cfg.Seed = tc.seed
			cfg.World.Aspect = tc.aspect
			if err := cfg.Resolve(); err != nil {
				t.Fatal(err)
			}
			cfgBytes, err := cfg.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			ctx, err := pipeline.NewContext(cfg, t.TempDir(), "", nil)
			if err != nil {
				t.Fatal(err)
			}
			stages := pipeline.Stages()
			last, err := pipeline.Lookup(stages, "mesh")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pipeline.Run(ctx, stages, last); err != nil {
				t.Fatal(err)
			}
			enc, err := ctx.Products.Mesh.AppendBinary(nil)
			if err != nil {
				t.Fatal(err)
			}
			h := golden.New()
			h.Bytes(cfgBytes)
			h.Write(enc)
			golden.Check(t, "mesh/"+tc.name, h.Sum(), tc.want)
		})
	}
}

// TestGoldenCells pins the cell statistics stage across architectures (see
// package golden for how to record a hash): the pipeline run through stage
// 5, cells. Each case hashes, in order, the resolved config.json bytes
// (golden.Hasher.Bytes, length-prefixed), then the statistics' canonical
// encoding (cells.Stats.AppendBinary: the cell and sample counts; each
// cell's altitude, relief and latitude as float64 bits and its sample
// count; the empty-cell count; and every sample's cell id in storage
// order; integers and float bits little-endian).
func TestGoldenCells(t *testing.T) {
	for _, tc := range []struct {
		name   string
		seed   config.Seed
		aspect string
		want   string
	}{
		{"seed42-cinematic", 42, "cinematic", "7386b9d0b20629091c1c2368bfa23597d0e904419b338392b6f44ef85a7f5e61"},
		{"seed42-square", 42, "square", "0c345304abf1d9c10b362810915cae64819f74b31bab303165961b9c85af4e5f"},
		{"seed7-cinematic", 7, "cinematic", "d09f80d9348626118e09bc1b06d71f319fc4bbb050a421d39206830f3af986f6"},
		{"seed7-portrait", 7, "portrait", "d16068b4ff8180ecc24b683e4833f74bdf04e671663a79ad004a32518884953f"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := config.Default()
			cfg.Seed = tc.seed
			cfg.World.Aspect = tc.aspect
			if err := cfg.Resolve(); err != nil {
				t.Fatal(err)
			}
			cfgBytes, err := cfg.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			ctx, err := pipeline.NewContext(cfg, t.TempDir(), "", nil)
			if err != nil {
				t.Fatal(err)
			}
			stages := pipeline.Stages()
			last, err := pipeline.Lookup(stages, "cells")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pipeline.Run(ctx, stages, last); err != nil {
				t.Fatal(err)
			}
			enc, err := ctx.Products.Cells.AppendBinary(nil)
			if err != nil {
				t.Fatal(err)
			}
			h := golden.New()
			h.Bytes(cfgBytes)
			h.Write(enc)
			golden.Check(t, "cells/"+tc.name, h.Sum(), tc.want)
		})
	}
}

// TestGoldenSeaLevel pins the sea level stage across architectures (see
// package golden for how to record a hash): the pipeline run through stage
// 6, sea-level. Each case hashes, in order, the resolved config.json bytes
// (golden.Hasher.Bytes, length-prefixed), then the search result's
// canonical encoding (cells.SeaLevel.AppendBinary: target, tolerance and
// budget; the policy; the initial estimate and the chosen level; the
// playable, land, ocean and dry basin counts; the land area; the reason and
// whether the target was met; every probe's method, candidate index, level
// and counts; and each cell's land, ocean and basin flags; integers and
// float bits little-endian).
func TestGoldenSeaLevel(t *testing.T) {
	for _, tc := range []struct {
		name   string
		seed   config.Seed
		aspect string
		preset string
		want   string
	}{
		{"seed42-cinematic", 42, "cinematic", "continents", "5a961aeb7ad34f2657c6c29447a46615ec21a69c5d655e95dda3ae06c8de5f5b"},
		{"seed42-square", 42, "square", "continents", "4abea651a81e787f12b205f53be023c2825108b01fb65d6d48014f6d79a2a225"},
		{"seed7-cinematic", 7, "cinematic", "continents", "345f5233948c727ff961d27db1a78ee84931cc0e63d8084def6834414a3f0ce4"},
		{"seed7-portrait", 7, "portrait", "continents", "f7b80c388ae28b87ab550c9d02d3e0f8f4d9b647f8e1f719b74b4a3662a6f086"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "9ef4dbb079b6149650ade7ffc767b0b0b7c81a48b6955f3c5d0c26ac43f878f8"},
		{"seed6-cinematic-pangaea", 6, "cinematic", "pangaea", "289e6d2e4ee744cc42acb89924f8d14cd22ccff968b19f5ba0e4c486bcf3a3e9"}, // gallops
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := config.Default()
			cfg.Seed = tc.seed
			cfg.World.Aspect = tc.aspect
			cfg.Layout.Preset = tc.preset
			if err := cfg.Resolve(); err != nil {
				t.Fatal(err)
			}
			cfgBytes, err := cfg.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			ctx, err := pipeline.NewContext(cfg, t.TempDir(), "", nil)
			if err != nil {
				t.Fatal(err)
			}
			stages := pipeline.Stages()
			last, err := pipeline.Lookup(stages, "sea-level")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pipeline.Run(ctx, stages, last); err != nil {
				t.Fatal(err)
			}
			enc, err := ctx.Products.SeaLevel.AppendBinary(nil)
			if err != nil {
				t.Fatal(err)
			}
			h := golden.New()
			h.Bytes(cfgBytes)
			h.Write(enc)
			golden.Check(t, "sea-level/"+tc.name, h.Sum(), tc.want)
		})
	}
}

// TestGoldenClimate pins the climate stage across architectures (see
// package golden for how to record a hash): the pipeline run through stage
// 7, climate. Each case hashes, in order, the resolved config.json bytes
// (golden.Hasher.Bytes, length-prefixed), then the climate's canonical
// encoding (climate.Result.AppendBinary: the cell and sample counts; the
// sea level; each cell's ocean flag, height above sea level and
// temperature; every sample's mask flag in storage order; each cell's
// precipitation, moisture, lift, PET, runoff and aridity index; and every
// sample's precipitation in storage order; integers and float bits
// little-endian, flags one byte).
func TestGoldenClimate(t *testing.T) {
	for _, tc := range []struct {
		name   string
		seed   config.Seed
		aspect string
		preset string
		want   string
	}{
		{"seed42-cinematic", 42, "cinematic", "continents", "9e419b62bcc005cfe022eba8729870fe3930cac41f1715ea59723771c573e371"},
		{"seed7-square", 7, "square", "continents", "4197456cb2c2504d9af9e22b9add7b58510053913824512ab8bec3db11d64212"},
		{"seed7-portrait", 7, "portrait", "continents", "b6d20824d0a16f964f692fc84fa420e31eb5b9f0a0444752b1a5ba6092ee7137"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "8dc5b5a31237b54eaa0ff3517cd8c497ff0fb53b44336683a6ca445d8229367a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := config.Default()
			cfg.Seed = tc.seed
			cfg.World.Aspect = tc.aspect
			cfg.Layout.Preset = tc.preset
			if err := cfg.Resolve(); err != nil {
				t.Fatal(err)
			}
			cfgBytes, err := cfg.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			ctx, err := pipeline.NewContext(cfg, t.TempDir(), "", nil)
			if err != nil {
				t.Fatal(err)
			}
			stages := pipeline.Stages()
			last, err := pipeline.Lookup(stages, "climate")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pipeline.Run(ctx, stages, last); err != nil {
				t.Fatal(err)
			}
			enc, err := ctx.Products.Climate.AppendBinary(nil)
			if err != nil {
				t.Fatal(err)
			}
			h := golden.New()
			h.Bytes(cfgBytes)
			h.Write(enc)
			golden.Check(t, "climate/"+tc.name, h.Sum(), tc.want)
		})
	}
}

// TestGoldenBasins pins the basins stage across architectures (see package
// golden for how to record a hash): the pipeline run through stage 8,
// basins. Each case hashes, in order, the resolved config.json bytes
// (golden.Hasher.Bytes, length-prefixed), then the hierarchy's canonical
// encoding (basin.Result.AppendBinary: the minimum depth; every depression,
// then every basin, with its parent, children, bottom cell and altitude,
// spill level, depth, spill cell, edge and corner, size and own cells; each
// depression's basin; and each cell's depression, basin and routing
// height; integers and float bits little-endian).
func TestGoldenBasins(t *testing.T) {
	for _, tc := range []struct {
		name   string
		seed   config.Seed
		aspect string
		preset string
		want   string
	}{
		{"seed42-cinematic", 42, "cinematic", "continents", "eba5227f369ed7c5cbf0640251a95036ae9f6ff73553e4ad9e37884ce48e5742"},
		{"seed42-square", 42, "square", "continents", "849757e0eccfb84aa38a023e3f979e0b8e967d299e0001df54666a551b339485"},
		{"seed7-portrait", 7, "portrait", "continents", "e10da04a34e832018f1dd86c3052c2824bafa9d2ed9387ec51e4c4a769f2a4ac"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "c5efaa9266032c5e3122e2c9d3e8b02a165ac1238aa1ae14fbb993b91b2792d3"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "9f8b4826ba6aec9038122c663e4d4667dc0c908fc67cd7c42496b805ea3a292f"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := config.Default()
			cfg.Seed = tc.seed
			cfg.World.Aspect = tc.aspect
			cfg.Layout.Preset = tc.preset
			if err := cfg.Resolve(); err != nil {
				t.Fatal(err)
			}
			cfgBytes, err := cfg.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			ctx, err := pipeline.NewContext(cfg, t.TempDir(), "", nil)
			if err != nil {
				t.Fatal(err)
			}
			stages := pipeline.Stages()
			last, err := pipeline.Lookup(stages, "basins")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pipeline.Run(ctx, stages, last); err != nil {
				t.Fatal(err)
			}
			enc, err := ctx.Products.Basins.AppendBinary(nil)
			if err != nil {
				t.Fatal(err)
			}
			h := golden.New()
			h.Bytes(cfgBytes)
			h.Write(enc)
			golden.Check(t, "basins/"+tc.name, h.Sum(), tc.want)
		})
	}
}

// TestGoldenLakes pins the basins stage's water balance across
// architectures (see package golden for how to record a hash): the pipeline
// run through stage 8, basins. Each case hashes, in order, the resolved
// config.json bytes (golden.Hasher.Bytes, length-prefixed), then the water
// balance's canonical encoding (basin.Lakes.AppendBinary: the settings;
// each cell's downstream cell, sink basin and lake; the stray count; each
// basin's state, level, filled cells, inflow, overflow, overflow target and
// via cell, remainder and lake; each lake's basin, cells, surface, kind,
// full and salt flags, volumes and outlet; each playa's basin, cell and
// sink corner; and the totals; integers and float bits little-endian).
func TestGoldenLakes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		seed   config.Seed
		aspect string
		preset string
		want   string
	}{
		{"seed42-cinematic", 42, "cinematic", "continents", "85b9da748a322d795eff00415edd2cb3776d34e756d1884906d6f653c5df678e"},
		{"seed42-square", 42, "square", "continents", "9e0f364945a93ab1578bcfc83127925258e6951d15f9a43fc0e9e0a0f75a7365"},
		{"seed7-portrait", 7, "portrait", "continents", "48f4b4ffd5e2a82038368fdf455541432324f4c7a7e71e30a4acaec23602d16c"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "925379fee221bb8f352af5b98d0e37a16a26b874b45a4dd9f4a1ca703c1f91ae"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "13061872070b56109aff4758ada5f5f786d14cabb18523564e6c235aa223dddb"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := config.Default()
			cfg.Seed = tc.seed
			cfg.World.Aspect = tc.aspect
			cfg.Layout.Preset = tc.preset
			if err := cfg.Resolve(); err != nil {
				t.Fatal(err)
			}
			cfgBytes, err := cfg.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			ctx, err := pipeline.NewContext(cfg, t.TempDir(), "", nil)
			if err != nil {
				t.Fatal(err)
			}
			stages := pipeline.Stages()
			last, err := pipeline.Lookup(stages, "basins")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pipeline.Run(ctx, stages, last); err != nil {
				t.Fatal(err)
			}
			enc, err := ctx.Products.Lakes.AppendBinary(nil)
			if err != nil {
				t.Fatal(err)
			}
			h := golden.New()
			h.Bytes(cfgBytes)
			h.Write(enc)
			golden.Check(t, "lakes/"+tc.name, h.Sum(), tc.want)
		})
	}
}

// TestGoldenLandTarget pins the land-target stage across architectures
// (see package golden for how to record a hash): the pipeline run through
// stage 9, land-target. Each case hashes, in order, the resolved config.json
// bytes (golden.Hasher.Bytes, length-prefixed), the elevation pre-pass's
// lake cells (golden.Hasher.Int) and datum land share (golden.Hasher.Float64s),
// then the land target's canonical encoding (pipeline.LandTarget.AppendBinary:
// the search's target, tolerance, budget, expected lake cells, policy,
// initial estimate, reason, met flag, best probe and every probe; the land,
// ocean, lake and dry basin counts and the land area; each cell's land,
// ocean and lake flags; the water balance at the chosen level; the climate
// pass count; and the final climate pass, inland seas recharging the air).
func TestGoldenLandTarget(t *testing.T) {
	for _, tc := range []struct {
		name   string
		seed   config.Seed
		aspect string
		preset string
		want   string
	}{
		{"seed42-cinematic", 42, "cinematic", "continents", "fc69c3ea43e6b9b9ea0125aade8e60c50ea91a0a41826c30f39459d5f41aac6b"},
		{"seed7-square", 7, "square", "continents", "9939454c430061e847b1c67c4b2c6c7ea811f6844062a409ea5ce96f2f3442dd"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "b4a069774d9a584a01d0e54d7c31ec63ca443749f3115af0cca45fefde197187"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "e9fa4047d6403bd5f151725849a984e7a0fdb2dcde08c998d1639243350d7798"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := config.Default()
			cfg.Seed = tc.seed
			cfg.World.Aspect = tc.aspect
			cfg.Layout.Preset = tc.preset
			if err := cfg.Resolve(); err != nil {
				t.Fatal(err)
			}
			cfgBytes, err := cfg.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			ctx, err := pipeline.NewContext(cfg, t.TempDir(), "", nil)
			if err != nil {
				t.Fatal(err)
			}
			stages := pipeline.Stages()
			last, err := pipeline.Lookup(stages, "land-target")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pipeline.Run(ctx, stages, last); err != nil {
				t.Fatal(err)
			}
			enc, err := ctx.Products.Target.AppendBinary(nil)
			if err != nil {
				t.Fatal(err)
			}
			h := golden.New()
			h.Bytes(cfgBytes)
			h.Int(ctx.Products.PrePass.LakeCells)
			h.Float64s(ctx.Products.PrePass.LandShare)
			h.Write(enc)
			golden.Check(t, "land-target/"+tc.name, h.Sum(), tc.want)
		})
	}
}

// TestGoldenRivers pins the river stage across architectures (see package
// golden for how to record a hash): the pipeline run through stage 10,
// rivers. Each case hashes, in order, the resolved config.json bytes
// (golden.Hasher.Bytes, length-prefixed), then the drainage tree's
// canonical encoding (river.Tree.AppendBinary: the corner count; each
// corner's land flag and, for a land corner, its terminal kind, lake,
// level, downstream corner and edge; the flood order; each lake's drain,
// outlet corner, target lake and flags; the fallback count; and the
// agreement with the water balance's catchments; integers and float bits
// little-endian), then the river network's (river.Network.AppendBinary:
// the class breaks; each corner's drainage, volume and mouth flag; each
// lake's drainage and inflow; each edge's upstream corner and river class;
// and every polyline's corners, edges and classes). The cases include a
// pangaea and worlds whose rivers cross the seam.
func TestGoldenRivers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		seed   config.Seed
		aspect string
		preset string
		want   string
	}{
		{"seed42-cinematic", 42, "cinematic", "continents", "28a9f8270aceb4624cb45ed94a87163ae17174dbb88bbbaf961cdcb735cdb10f"},
		{"seed42-square", 42, "square", "continents", "7f038eff3752411c97107ef1afaaa5d676576bfa9535513c0b3965c252aeecd1"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "37a5a7da73620e7d776f3afcd407123a3ee7339b087293bf0930d6a98272edf8"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "fb6676e83a12ae0590ec6ffa08aabbd1c313150aa0850f43839ea9f507a04cff"},
		{"seed8-square-pangaea", 8, "square", "pangaea", "4263d756c65f2ff498fc31684198ce006e4e55d26b5b2520ebf4454d9e164ecb"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := config.Default()
			cfg.Seed = tc.seed
			cfg.World.Aspect = tc.aspect
			cfg.Layout.Preset = tc.preset
			if err := cfg.Resolve(); err != nil {
				t.Fatal(err)
			}
			cfgBytes, err := cfg.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			ctx, err := pipeline.NewContext(cfg, t.TempDir(), "", nil)
			if err != nil {
				t.Fatal(err)
			}
			stages := pipeline.Stages()
			last, err := pipeline.Lookup(stages, "rivers")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pipeline.Run(ctx, stages, last); err != nil {
				t.Fatal(err)
			}
			enc, err := ctx.Products.Rivers.AppendBinary(nil)
			if err != nil {
				t.Fatal(err)
			}
			enc, err = ctx.Products.Network.AppendBinary(enc)
			if err != nil {
				t.Fatal(err)
			}
			h := golden.New()
			h.Bytes(cfgBytes)
			h.Write(enc)
			golden.Check(t, "rivers/"+tc.name, h.Sum(), tc.want)
		})
	}
}

// TestGoldenClassify pins the classification stage across architectures
// (see package golden for how to record a hash): the pipeline run through
// stage 11, classify. Each case
// hashes, in order, the resolved config.json bytes (golden.Hasher.Bytes,
// length-prefixed), then the classification's canonical encoding
// (classify.Result.AppendBinary: the cell count; each cell's landform and
// depth codes, sea steps and volcano flag; the hotspot count; each
// hotspot's cell and whether it is on land; the covered cell count; each
// cell's biome and surface codes; integers little-endian).
func TestGoldenClassify(t *testing.T) {
	for _, tc := range []struct {
		name   string
		seed   config.Seed
		aspect string
		preset string
		want   string
	}{
		{"seed42-cinematic", 42, "cinematic", "continents", "fead5c6b7b4bede51b917892b85b64d8516d8068bc8a82f2ca11dc7a95960014"},
		{"seed42-square", 42, "square", "continents", "cf952781ea6b5c012faf06055abb3c5e6c646798f44a8766e9d86b9bd140b59a"},
		{"seed7-cinematic", 7, "cinematic", "continents", "a47fc767eaf73d132886ba6ca149cf1aea609938def8f71104a208129f48bb3e"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "bd082273b0eb269a5c95844c7619ffd5f153469322f64c4320f44e63c8ea36c8"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "a9177f116737627f1c47b4e08df47f7fd307e93bf9c1348869d644676060f27d"}, // volcanic highlands
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := config.Default()
			cfg.Seed = tc.seed
			cfg.World.Aspect = tc.aspect
			cfg.Layout.Preset = tc.preset
			if err := cfg.Resolve(); err != nil {
				t.Fatal(err)
			}
			cfgBytes, err := cfg.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			ctx, err := pipeline.NewContext(cfg, t.TempDir(), "", nil)
			if err != nil {
				t.Fatal(err)
			}
			stages := pipeline.Stages()
			last, err := pipeline.Lookup(stages, "classify")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pipeline.Run(ctx, stages, last); err != nil {
				t.Fatal(err)
			}
			enc, err := ctx.Products.Classes.AppendBinary(nil)
			if err != nil {
				t.Fatal(err)
			}
			h := golden.New()
			h.Bytes(cfgBytes)
			h.Write(enc)
			golden.Check(t, "classify/"+tc.name, h.Sum(), tc.want)
		})
	}
}

// TestGoldenEdges pins the edge stage across architectures (see package
// golden for how to record a hash): the pipeline run through stage 12,
// edges, with the river stage's classes on the edges. Each case hashes, in
// order, the resolved config.json bytes (golden.Hasher.Bytes,
// length-prefixed), then the edge data's canonical encoding
// (edges.Data.AppendBinary: per edge its passable and coast flags, water
// kind, river class and incline; per cell its half-edges in compass order,
// each with edge id, neighbor, bearing bits, direction, error bits and
// incline; integers little-endian).
func TestGoldenEdges(t *testing.T) {
	for _, tc := range []struct {
		name   string
		seed   config.Seed
		aspect string
		preset string
		want   string
	}{
		{"seed42-cinematic", 42, "cinematic", "continents", "427a33092d80e91d3f716c0d20d150c6e27334f6359211a9b39afe0071fbb2b4"},
		{"seed42-square", 42, "square", "continents", "f5eba9a0dd4c8278cc3f80eb3a22d8e8fbdbdce67b340ac0cf4bac1d3a15a60d"},
		{"seed7-cinematic", 7, "cinematic", "continents", "303cdcf1b1e708d13345375e2912a1a61c26af60405729952c84a8dec8dacf8f"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "37bac3359c7c6329514c61b853f21b147096285b773bd5917325179dbc89faba"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := config.Default()
			cfg.Seed = tc.seed
			cfg.World.Aspect = tc.aspect
			cfg.Layout.Preset = tc.preset
			if err := cfg.Resolve(); err != nil {
				t.Fatal(err)
			}
			cfgBytes, err := cfg.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			ctx, err := pipeline.NewContext(cfg, t.TempDir(), "", nil)
			if err != nil {
				t.Fatal(err)
			}
			stages := pipeline.Stages()
			last, err := pipeline.Lookup(stages, "edges")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pipeline.Run(ctx, stages, last); err != nil {
				t.Fatal(err)
			}
			enc, err := ctx.Products.Edges.AppendBinary(nil)
			if err != nil {
				t.Fatal(err)
			}
			h := golden.New()
			h.Bytes(cfgBytes)
			h.Write(enc)
			golden.Check(t, "edges/"+tc.name, h.Sum(), tc.want)
		})
	}
}

// TestGoldenWorld pins the game data file across architectures (see
// package golden for how to record a hash): the whole pipeline, through
// stage 14, export, passing over the deferred stages. Each case hashes the
// world.json bytes exactly as written (golden.Sum: SHA-256 of the file).
// The file carries the config hash in its metadata, so the config is
// covered too. Each case also pins the player-style map drawn from the
// decoded file alone (package playermap) at playermap.DefaultScale: the
// render.PixelHash of the full map ("player/" hashes), and the
// measures.json bytes exactly as written ("measures/" hashes; the file
// carries the config hash too).
func TestGoldenWorld(t *testing.T) {
	for _, tc := range []struct {
		name     string
		seed     config.Seed
		aspect   string
		preset   string
		want     string
		player   string
		measures string
	}{
		{"seed42-cinematic", 42, "cinematic", "continents", "3c00c78e2f4db97f0fa5b26790911517f10aa6f00ca5fd5ea6848ec41ca6c975", "99833872989f373640030207a900e39fbef19b723f50e2468705faec70b6d061", "4d6c5c79833075874438eaecc81dc538bef6c5dd8b6031b183164b612c93b43b"},
		{"seed7-square", 7, "square", "continents", "8c26f2cee4781ab13ef3a10134f4912cd300004b40fce68d346343d200882102", "630589edcf069e4e2e23548e7cfdccb35899764ea745592937ff5e81069a0e02", "afc5c1231dc2975e9e666b49d09910568678c5bd2dcc8cd9a08abeca019d01b4"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "b97271b95c1a6cf7451809602a14c27b45bcdc8b40695130f8e5999384c2789e", "d214bf8ef7dd5c0605d2c7956754bf7f2b4df4040e5beb8bf4620ce7fe83598f", "8d110eb7cbcd57b75fd53b219b6537cffdd1648f2b6a8199e79ba8a803da66da"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := config.Default()
			cfg.Seed = tc.seed
			cfg.World.Aspect = tc.aspect
			cfg.Layout.Preset = tc.preset
			out := t.TempDir()
			ctx, err := pipeline.NewContext(cfg, out, "", nil)
			if err != nil {
				t.Fatal(err)
			}
			res, err := pipeline.Run(ctx, pipeline.Stages(), -1)
			if err != nil {
				t.Fatal(err)
			}
			if res.NotImplemented != nil {
				t.Fatalf("run stopped at %s", res.NotImplemented)
			}
			b, err := os.ReadFile(filepath.Join(out, world.File))
			if err != nil {
				t.Fatal(err)
			}
			golden.Check(t, "world/"+tc.name, golden.Sum(b), tc.want)
			mb, err := os.ReadFile(filepath.Join(out, world.MeasuresFile))
			if err != nil {
				t.Fatal(err)
			}
			golden.Check(t, "measures/"+tc.name, golden.Sum(mb), tc.measures)
			w, err := world.Decode(bytes.NewReader(b))
			if err != nil {
				t.Fatal(err)
			}
			img, _, err := playermap.RenderFull(w, playermap.DefaultScale)
			if err != nil {
				t.Fatal(err)
			}
			golden.Check(t, "player/"+tc.name, render.PixelHash(img), tc.player)
		})
	}
}
