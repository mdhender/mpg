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
		{"seed42-cinematic", 42, "cinematic", "continents", "d2280614e5fd1fd1084a5d75aa627439181f549c56bab809e9575f41c2ab0298"},
		{"seed42-square", 42, "square", "continents", "1715d7b2ba0b1345744e76099dd25c2c0614958bc23f3c5b5f5598feb7f0764f"},
		{"seed7-cinematic", 7, "cinematic", "continents", "c97af5c7e7a649ee364d3a1c222675834798e9180063e2292a6e1b0b91dd9e97"},
		{"seed7-square", 7, "square", "continents", "ab5f960e9d70b71c27a83b01d2a416fa310a30651677d90e42b992237e1d7271"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "013c946e5fde41d23d0dbe7281e39d3591d07f3d5fbe60d0ac42839a1d94f9eb"},
		{"seed42-square-islands", 42, "square", "islands", "7a48c90a3f0daeba283a34352e7b9ddc3a776baa6e8bf2fa19b59739a2ded9b7"},
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
		{"seed42-cinematic", 42, "cinematic", "fe22a888b65b9934de6b214a2ebc87476ec2b1cf2ce2689e1d71e7ee495172af"},
		{"seed42-square", 42, "square", "02397f2bee3ee7cacb9316850c1fe2a3d5d0dae44d635ec79ae316424e332059"},
		{"seed7-cinematic", 7, "cinematic", "259305858d94b89ca826a63e706b136356d435c96c0302fa8c89c9de6a9a7dd2"},
		{"seed7-portrait", 7, "portrait", "c830edf5a65e75bb4f8b20b17edd9c3812877574342b8b88913cb4662388ab9c"},
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
		{"seed42-cinematic", 42, "cinematic", "3f80cef319cd8bc07faac93fa42470535bfbe98e4126a49da4b051b33def94c6"},
		{"seed42-square", 42, "square", "5e0738a206fda2b3653beb88b767780877368636d85fa8ed6904022d35893ccb"},
		{"seed7-cinematic", 7, "cinematic", "5d31e4de86067ea5c377b08dec4409c6a372398bd0f2276a321421d4320d2616"},
		{"seed7-portrait", 7, "portrait", "3927e748c8cf310e1a41a3416586f241813b60ff6198f4670e5709f9f27fdbc9"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "051f6d6980f7336e002992886abf79d4787c12880c486480a2056215c311d922"},
		{"seed42-square", 42, "square", "continents", "64ef8a452ca038e50fb32639ac196db91cf85208fa0847d70ed75fc15fa6881e"},
		{"seed7-cinematic", 7, "cinematic", "continents", "7a7d796725bd08c3272db90ad4e7d555bb4bf2523e7568c875e961f4cb674c94"},
		{"seed7-portrait", 7, "portrait", "continents", "cfa9b9022280a9e43ef7804c8417f51bc9b5b567355a7a8dee74c3b7f8720c0a"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "4fa72840524a39636004facc0e127b731f4e6e3e6820c3f89ff2b55ec46eaf01"},
		{"seed6-cinematic-pangaea", 6, "cinematic", "pangaea", "31c84615e26f71e48b4691f75061e01ad1920fb1a2a1280d0cbabea81dc2f8f0"}, // gallops
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
		{"seed42-cinematic", 42, "cinematic", "continents", "c1c4444d9b0841731bb0ddf5fc44a30fb057838b4f06c0cc39fc186ad0e614ba"},
		{"seed7-square", 7, "square", "continents", "409571964b6e7610e463fc7974fe4bb21b31ce877a2f1289f61f006f336459b2"},
		{"seed7-portrait", 7, "portrait", "continents", "ece659fc8e5ca9cf6427ede7e38d1331748eadafd63c5382d16721ad42420abf"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "72a24015021eb5f7051ad3d745ba9dbafb2e1dc96e3538737f07f40545937c95"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "3cb1aa9f2ed39ebb4eb69ea15c403eaa8d88cac20a158805a04ba0138f074072"},
		{"seed42-square", 42, "square", "continents", "d2c241be420dfe7e2158a1df2062732d6ba39b4ecf75d632cd8d4b0fc7cc9956"},
		{"seed7-portrait", 7, "portrait", "continents", "2f224a1b7d7bf53d38c3a716bc1a3b20cbf691041dc0266f30022c8cc74e5af1"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "b79567df377a1ddce01d795912665d9bc4c9ea129a522cdfc525cc230949b5c4"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "59c708110f136544075ab459066c995c7492cb89de6ddf2a3ca05f4626e80ae2"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "8158b93eb6f7bd2ebe7d86e1dd6b9170fe4e9d57d33d7ef005192628f451b796"},
		{"seed42-square", 42, "square", "continents", "ae5e0cf22e4cc0d485d29dd3cdbc7a8855d489e23a62b574a93d2da38e502f41"},
		{"seed7-portrait", 7, "portrait", "continents", "d0c0e5de75fc508610ff76ce24e78f4ac56dcf3ab878ab25428a0cb91f6c5d2d"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "17674587dd1beb158cc1feeab1ddca537c1d69dc6f77f54b832fa7f8393b46e1"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "b999d050cdbe928eb479167afcf54b043065d462b02c7d2460421b4878ef2e50"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "38ca6e3a14b9b4343913e276cf5971b3ac02d679f02d8f05ac1cc98b748919a2"},
		{"seed7-square", 7, "square", "continents", "6a8d3879a28b16ffe1e215df7d6ec95370893c988cfc92397005b1a6fc74586c"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "114f3455525b854d5131964223523e24acd6d0da06148348a5426b2bae591823"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "4db3a3335240cf59ee86ad3d9810e0cedeefa5a240b6f4c8e54bc6e5821ada8a"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "ac5dceaf1de98c572925f252b5fba1c0c7f980708c1659862848c121222db25b"},
		{"seed42-square", 42, "square", "continents", "c957b4c8311a8babe289fcc52b321f8526da4d9d8d14d14b8e77dc5f5af2efdb"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "e03e014ab56a54e5c8d6aadc3368037aa46f16ff8fe59514a496c9ab50ad9ec5"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "392155bfae4229e10ac2131da204268199ac274a1e6dd7a87c96bf028b84acc9"},
		{"seed8-square-pangaea", 8, "square", "pangaea", "fa81590f936c2e139251056459aae9ce7c7825475a79ebeafc12ffe6d64fe191"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "8e73d328445364c3147040ba5727a9401750b6f668640aa3118475ff735ef626"},
		{"seed42-square", 42, "square", "continents", "d932c4cca56bbb0d65ede145cefa14f3a27917bedcc3d28dfb5c186edab96953"},
		{"seed7-cinematic", 7, "cinematic", "continents", "6c0fe26dc4037d0fed484c1fdef3869d7b796d2a9a3ccf7ca6b5884dfcd621e1"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "884686a3de894e5454943f1b25f2a7547ea09de5979b9f36d1e0679aebc3b5e5"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "e4dfe0b4262322e422dc48660fc8fc92b2ac1b00594ddeebf224b28392e55949"}, // volcanic highlands
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
		{"seed42-cinematic", 42, "cinematic", "continents", "c50d8d9dddaa3d7d41b7203629c0bdb7d93fa77ee925fc7e51ff70d0ede86620"},
		{"seed42-square", 42, "square", "continents", "005f884ce541482fa54667d2f849f20ded6f1a86b15e323481babb6e30ee9244"},
		{"seed7-cinematic", 7, "cinematic", "continents", "d122a56fa7800a7365bc0b6b34fb6f797d5e4704b19b743198529b8659c97d91"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "40e2fae1b6cea9252fa722cda81db2a232cea4b5f82e954496b74909d067d552"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "173248cf70a8d8e18b787b124ff2c19b16188e4d1be2fffdae4bfbb1dd530b16", "99833872989f373640030207a900e39fbef19b723f50e2468705faec70b6d061", "7f845c350623759462d7952aa10d35b51a8c3678cff0c09c530f7331e1f3656d"},
		{"seed7-square", 7, "square", "continents", "6b17893aeabd7d14743ea839897e9763f574589adc5a93a040de42c23954c5f2", "630589edcf069e4e2e23548e7cfdccb35899764ea745592937ff5e81069a0e02", "4543c748722a19f2072e2dfdd080589b8b1cd5bb32f49ca5714bac530df9a3c4"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "98488ed39da3f1f497c8160fc9b7e72bbb7978ff90e45de8064fca83863b0771", "d214bf8ef7dd5c0605d2c7956754bf7f2b4df4040e5beb8bf4620ce7fe83598f", "0eb8a2141079604183848e03c43e10b82d718ca9dbcb534c8792895cd13621b7"},
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
