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
		{"seed42-cinematic", 42, "cinematic", "continents", "127d8681dc1d5649298e40f0da7d110a477d2754aa57074ece85bc5bcd173fd7"},
		{"seed42-square", 42, "square", "continents", "468a25b6648f75b71df01c8d5eb63c27546847fdbc98ed5d11f22d27f2e919a6"},
		{"seed7-cinematic", 7, "cinematic", "continents", "0544f2eeb230bed1a8af1613a746197b24e54723a6abedadc6c42388032e51c8"},
		{"seed7-square", 7, "square", "continents", "141cbc960be998f0b463cf56f131aaac41b003beb3ae50434fd94adbb1d717bb"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "618377c69271228e223352296f1a0a5f8d7392d539b6fd494830bbaec8201cbc"},
		{"seed42-square-islands", 42, "square", "islands", "aab07cfe1d4e621276d7af11f4d3570c829a065df6033e40c7d1b12a0da8daea"},
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
		{"seed42-cinematic", 42, "cinematic", "99a62be06e9d97bdebd2dd27a86434ac87a65e94aad4bed7189348c2b8d51b43"},
		{"seed42-square", 42, "square", "71a8bfd01a60dffd6bade13a357d2e49c22c631be398865db5f7698e04ce7bc2"},
		{"seed7-cinematic", 7, "cinematic", "9701d0e958bd68606ab256c349fbe2796166ba9871ccdf788c617d22c3a4b4ea"},
		{"seed7-portrait", 7, "portrait", "846f5c1c282fc3ce4fb083bacc03e6c7031b216a19a453db5786343b068f5448"},
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
		{"seed42-cinematic", 42, "cinematic", "fad40866961731c4e30bf463accba6b0166c7b6d37a33f1e9be90533f2b0ddfd"},
		{"seed42-square", 42, "square", "287f6ac4962ed49580c3787a817742a1c09cd361545a006fc4405bc35d814344"},
		{"seed7-cinematic", 7, "cinematic", "3b3835cddf4e8777f6d92e5fe6dd90a5dbd936ff4fbb008b2d823d1346cbe528"},
		{"seed7-portrait", 7, "portrait", "8f9205c51f95eb0baa9f04004ead52b080e68ac5c8615da8cc902d5a051c51b7"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "5b82b285242a0fae784d7f0e1b88581463336eebdb6597e3ce974ed928acea42"},
		{"seed42-square", 42, "square", "continents", "656143809f425fece36b465d7b33b04ee139c1c53ce146b49407ae02b049d035"},
		{"seed7-cinematic", 7, "cinematic", "continents", "15b5f4f4b932470580357245c02a789a0b34f635e427defd4c50d8f97739bf25"},
		{"seed7-portrait", 7, "portrait", "continents", "09c5f9d1920cd93e2392c463b2b01a5b4480f5bc8f1810fc2e60e9031dcc86e4"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "8469452fff1a1ed258ce884151474175a56968ac28a3a7625c8e2906a28d53d2"},
		{"seed6-cinematic-pangaea", 6, "cinematic", "pangaea", "57d28171df46275c7b3795ab2860a02700595a4dab2b9b321c23ac3e1cc043ed"}, // gallops
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
		{"seed42-cinematic", 42, "cinematic", "continents", "38b437528c9fc9699a52446aadbd076dea3542a2a0f47d9f41d2e595b6bba486"},
		{"seed7-square", 7, "square", "continents", "4f0c812d618745e8ca6bdf1db39a9cfdaeacad689f3d11229bfab05f91e92f14"},
		{"seed7-portrait", 7, "portrait", "continents", "339bea80ffce4522a81193f379b02237d74db64fa316c77a925e90f7ab013c89"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "e67b7494bbf48cf3cce70e01b0434d5713928bb2de3a7c7479c6109ab43ca2c9"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "c2a53a6b80402628a2a1159ee213ce2ab866e4f23bb5d5ce2f238bc354486048"},
		{"seed42-square", 42, "square", "continents", "ecaeab8b45c28376a1560f78bf63c3b3d3e8dbfaa060b326ef46faf6102156b1"},
		{"seed7-portrait", 7, "portrait", "continents", "752d82036eb2aaad94e2f6d6f82808b71c7caed015c1796b10786137ce7d66ae"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "ff7ff1cdabc344e2fefc6f13a19de100e19b14929cdf81ae9dd172d9b1d485e9"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "c94c09e78ae3f776e733c4eb46630bcf9ec9f5d2e58077e352785e7507d39e3f"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "b91603f4a774dce2f9d48db88c892a17bfd437b183c41447e2f3ca6102d8da1e"},
		{"seed42-square", 42, "square", "continents", "625f05d944bf8cea861b5beed9f95685bb4d5e3eaccc15d62cf4b977c83e0229"},
		{"seed7-portrait", 7, "portrait", "continents", "0ef0ac77120a9a6aa8b93774c0c6a182fc9c33d5b4772f29fd16e7df09b7b3c4"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "099f4676ff0fb16c82de44ab0c0f73a149ab72a0c0a8950d696fdba27753f2ab"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "5de8acd62b4bef74601ba934caf5fe2e9b602d1b75666370e23b616568f8b970"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "ed410ef46d4bd19b7c922356fdc4151337382af78d96ba736004d368b2e54e1a"},
		{"seed7-square", 7, "square", "continents", "bd5048e5d574505f1e871d3ed8c25877b0cef93ad891d30d9fb745422ec45a2e"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "54368fc247497efbcc24b2a23ca400b8cd8ca3b8fba20b082029a26aa4fbb200"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "7c421a9e809acfb950a4aa8b47ba0c613974eefc8fc36d96d276206b23951172"},
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
// little-endian). The cases include a pangaea and worlds whose rivers cross
// the seam.
func TestGoldenRivers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		seed   config.Seed
		aspect string
		preset string
		want   string
	}{
		{"seed42-cinematic", 42, "cinematic", "continents", "13b97a413c84deefc8f6c400e3254b3cfb10f5871fc73974647368e3baef1548"},
		{"seed42-square", 42, "square", "continents", "9dd734ecd3db5d26a3d3e4992cd91dacfc2b941f07a9fd1b1c52e53b68ba2890"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "45766b743dfdd3bc24053eaa388875fc408d6e90852348008fdf57eb484dcf23"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "4297cecb5e3e5f2f4dc752feb05569b218c58d9c9673d689bb32768ee37ab721"},
		{"seed8-square-pangaea", 8, "square", "pangaea", "3ad2fe024b60f38ccdfcdd5e9e20ba4f2ffa68d4b758721197a39463f20b5788"},
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
// hotspot's cell and whether it is on land; integers little-endian).
func TestGoldenClassify(t *testing.T) {
	for _, tc := range []struct {
		name   string
		seed   config.Seed
		aspect string
		preset string
		want   string
	}{
		{"seed42-cinematic", 42, "cinematic", "continents", "38231032cb9442d3c5840e8ee646f46273e52dd9a0bc235fb6d4dd69fee626a7"},
		{"seed42-square", 42, "square", "continents", "31759ce0e6a3ab7b9c38146149e956c47f6a63cec8d853b6bf6a34f6abfcdcd3"},
		{"seed7-cinematic", 7, "cinematic", "continents", "74c192ece238efc10b9fe24e86becc85f622e48a2358ad706418197418a54a0c"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "d41406ff53740294ab9082d23b7cad654f2b0ec42d96ec2eff2f82ad5eab42f6"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "7eaeac0cfcb2bb61f364d804316708cf86fb81e9652343a840c1968c49a96467"}, // volcanic highlands
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
// edges, passing over the deferred stage 10. Each case hashes, in
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
		{"seed42-cinematic", 42, "cinematic", "continents", "b4e89567523fff0accb08046857cff440aed853893cd460a29a250498424bbc9"},
		{"seed42-square", 42, "square", "continents", "1d82aaff28ae0bbd7d6bc687f5334fc4e71a14e1437c90103068bb0b8e4edb3d"},
		{"seed7-cinematic", 7, "cinematic", "continents", "956b3f94583e92d14199626a614e6933a3d2827a5a27e3e23551a533cbb660f6"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "89956d88e809f8acf12df739acb30ca93a4ed1ebeb63541d696fcb653342ce57"},
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
// render.PixelHash of the full map ("player/" hashes).
func TestGoldenWorld(t *testing.T) {
	for _, tc := range []struct {
		name   string
		seed   config.Seed
		aspect string
		preset string
		want   string
		player string
	}{
		{"seed42-cinematic", 42, "cinematic", "continents", "0cd5f1a3dd9a151ddc7c76754336c136a95029d263af393296e8db4cce35cb32", "7e8f6b5d9ef57330ac04cfdb18304aedc012ae293629f8ec8861078679a718be"},
		{"seed7-square", 7, "square", "continents", "1bdb2de8bb243d7274bdd566352dd41930cf443e3edce0ff2f30fcca96b8bb8a", "65c083be4d98e9e98b277fdc9d9c1b7cedd96d560fe3a5279a92fd911092dec4"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "b65a7c7363e4740c462db915f473dec88ef303bc9d8457ce9c5dfa8a58e712e2", "9370382c19329da2a706d9474f0104cd24220db42e9b2d1b7662a8f28df0190d"},
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
