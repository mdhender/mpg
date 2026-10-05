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
		{"seed42-cinematic", 42, "cinematic", "continents", "61248c1fe77b97bd4f450a529294ce64ca5c3b74bf7cff25d7fa26a7e4eabda9"},
		{"seed42-square", 42, "square", "continents", "98562b81c506aa663dd8b91576ff59f91baefcaaedd47f06f1662ec7a8204124"},
		{"seed7-cinematic", 7, "cinematic", "continents", "0cc989126b528d7092c501f259d0d8a735d71ba53447473dd55dd38fca9ad1e7"},
		{"seed7-square", 7, "square", "continents", "283dbd7d3722f31c669b924e84ee64ba7f5f802a38d61512e23940beb706b004"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "374f6c3b6d662bc1cdc6dd543803e2db28658a0be1cf2d78138fcf3e2b052264"},
		{"seed42-square-islands", 42, "square", "islands", "89acda0dcee0be6f82b0ad14290d3b3b8288ab805c8130ef32c3ebfe725f3404"},
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
		{"seed42-cinematic", 42, "cinematic", "2ae47409c3c2fafca13a7f6afefd6ae0fea658a2ea6d52739e29ee81f3ef3a7d"},
		{"seed42-square", 42, "square", "35e7daaf45ca12b9a8d8f063cea61bbc0c7750a911abf37827dc80ef6bc14f16"},
		{"seed7-cinematic", 7, "cinematic", "ed072305e7adae034a92534cf181016b02040c7a1852f68b6e08230e08a672b7"},
		{"seed7-portrait", 7, "portrait", "f02ca5ed53ed91c0a655704fc0f76281e43270a113bdf4664a93d081c79980e9"},
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
		{"seed42-cinematic", 42, "cinematic", "a300bb70eb521481bf6c1fc82680548232abe9e1274bf8fdbf518eaaa955de50"},
		{"seed42-square", 42, "square", "0885239f2aff54b3272397083eccf8c98de69496e9352cca5e58271f9b7dacfe"},
		{"seed7-cinematic", 7, "cinematic", "2f7134089a46caa272563b28b43adb06bef785531239b108fde315eed49d2a2a"},
		{"seed7-portrait", 7, "portrait", "4365f3aa351a7573eacbc41b743cd4b6e7d4c3897216555c168799053fe549ea"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "c739543aa2869e5ca304ca2a9eeed0756cc2b65dd871c038eb38600ed1b533e3"},
		{"seed42-square", 42, "square", "continents", "816f5e23a78fd7cecf036fa37681d8303a2b1662009c7101306223ff6063baa7"},
		{"seed7-cinematic", 7, "cinematic", "continents", "89dd4074267bb62e1fba1976ba7b9946478d41835a84f2ea11026e668a2d8f61"},
		{"seed7-portrait", 7, "portrait", "continents", "ffbd7865264c14c8e559f9cd9f9396ce16085b878e9fa57fed723c591a601646"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "b3375ad80de4b8cb0d0d121e041e6a1e8ae8b905b1dee9b4c8837f6b02b290a8"},
		{"seed6-cinematic-pangaea", 6, "cinematic", "pangaea", "65cffb0f35e21d16f7ff7d93759df84bfb9524edcbffa34afedfdd38fb8f979d"}, // gallops
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
		{"seed42-cinematic", 42, "cinematic", "continents", "3c9e9cbc55b80335c448af7d76cada8475b45a15b6c2fd59d907bd17fa28a9e7"},
		{"seed7-square", 7, "square", "continents", "546172311694709b7240bdd28d4fa86d7cd76e5491ccde435c00f1dc35d6ef05"},
		{"seed7-portrait", 7, "portrait", "continents", "225e404c728b40481eadb79f7e41312e1175813cea8f7170a67f917abf5102eb"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "0bd0a51b9adca32374d31f61090e4d4dc3ce1cef5092c7de9bd9bf4959a2e22a"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "83421494345ab9b8a39f33f270b6a9a48a1a89f177533354902c4f8fc7b91e70"},
		{"seed42-square", 42, "square", "continents", "c415c1ded5f94bf3e1faac1412c2de4f7de8a869a2a98932f3bcf121820f3460"},
		{"seed7-portrait", 7, "portrait", "continents", "8f1448efab3de626e208868ba1eb5cba29b661e33a56738b548e1d9728b655d7"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "1474555b5e181ad9b86eccb7ca6e6655756c5d348ad35274fc453ea49e3c6ece"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "9ca70c4f948102b72836d4c859aa344e2c8ad2a6ab6e6bd2931643e96af9d546"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "da5af4a79ab5d98eea4ece9a5854066f51ef43d058363bbfd0d91b8c297c0067"},
		{"seed42-square", 42, "square", "continents", "6cfa0fd1c050bf6ee4050bcc0e50642b2120ece07d7611acbd54001e1c43d90d"},
		{"seed7-portrait", 7, "portrait", "continents", "d8ca54fa68d11130253b9cd58679f2e68538bf4edbb842fcd1682510f6f82b84"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "2e4d8496d77851d72e498e1f9abc4c0181a40584c53ed67273e0675a9fde8707"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "1f46065b52b6e2c84c86cda3798dfff239859cc46d3ca98c1d43000b5fc061ae"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "61b53f9781a46182c812f65c43e1a8e75f2a24632d393f8fa4c145e1257830d5"},
		{"seed7-square", 7, "square", "continents", "3927dac037901da5481b43b88e94f4d536b625349bbe2d42ad75d89daa61a4de"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "fd90db6b356ccb1a8f593f0056acc2d2825dab7ee8ef7c81791b5cfe719b2370"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "f154e909216115af8f8cfb061e12df23f5192937213e94ece577478be9450751"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "1249f2bda1e09b84d3e3e316b86d056f14c9cd4fbf98378d398b852f45840b43"},
		{"seed42-square", 42, "square", "continents", "b3539d853e8dbd8efb62e47cb0a9964b70cc09511f3076e6e7cb4e7a0f80039a"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "f888e2363af55c5574eff741ea18bdb4c60b0403b8f2e2df353124d3f8488613"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "07061fb9fe0d6d458381364de4e72ac2ed4757e34d108375ba31cb630e2b36cb"},
		{"seed8-square-pangaea", 8, "square", "pangaea", "1728c19086239a51ddc7465919a416d564858573fed9deb8714b986abc7efbc9"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "fe4c0323f124c53fbb2faeb3665aa8d8ce8efd09e68846113e9bfdae90a6ee72"},
		{"seed42-square", 42, "square", "continents", "8ce2a666960ec6295df18517aee58e94a900488d8ee62e32610dd17e59789651"},
		{"seed7-cinematic", 7, "cinematic", "continents", "939ece078e69449c4f6681580477214c94aceb293788d367226e4d1f640056cb"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "ae949361a72761490ab05986fbba7c58a2f20473804d57365a6510e5e055c582"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "f3cbdc5b021742da318c33edcae8295b554a503fcfe43c791aca7c4ca63aafca"}, // volcanic highlands
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
		{"seed42-cinematic", 42, "cinematic", "continents", "fdc55c131378be1c399dd5e1a1c3d952f620ad0a71cfc004af3967598c463d5e"},
		{"seed42-square", 42, "square", "continents", "9fbc736e8ac011979fa6cff0c1d5b6aa725f8dfd1bbe0b041b3d857e8597d033"},
		{"seed7-cinematic", 7, "cinematic", "continents", "84f06992d319b5536d834ea9583206fa95a796f1c0a2fc0e07ecf382a2cb65af"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "a57b6bd5c758df7599b80a9df99027f697527d93e61fd1961b286664ab04f3f8"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "ee64ad427bb369b021d6fb90ab8d36a418e63a5a336352938a7ba7dbc83a4dfe", "99833872989f373640030207a900e39fbef19b723f50e2468705faec70b6d061", "a5d20ea7987a5b256dde4629b565d6ed29dc3f633e01ee1f548edd525d50b52e"},
		{"seed7-square", 7, "square", "continents", "1b6e46159b03ed340e49569954bf97e74995f233938c541d141ae760a30f2824", "630589edcf069e4e2e23548e7cfdccb35899764ea745592937ff5e81069a0e02", "2f16625270efd99cc8b0198a8abbdc0381f5bec496327eb89f25eda080fcafa1"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "811a7a313b30607e03df3231be7b48c9d563f8baec150f543f4b965532f30ac1", "d214bf8ef7dd5c0605d2c7956754bf7f2b4df4040e5beb8bf4620ce7fe83598f", "6dfd4c4cfd5c67412924ef0e40c188056f4d6f80401ce43eca1b212a6a154808"},
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
