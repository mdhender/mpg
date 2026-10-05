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
		{"seed42-cinematic", 42, "cinematic", "continents", "71145c99142ae64ad8b0015d6da52a906cdeabbaedcf645f98de33868b5105bc"},
		{"seed42-square", 42, "square", "continents", "fcf1e82016cbbac49f422a3aeb3abfad34ad7d5c6bb80545a4df6e4fb4e63f68"},
		{"seed7-cinematic", 7, "cinematic", "continents", "4eb901273305738098bdffbf429fc54faad0b454de2519091caf5162edc35d95"},
		{"seed7-square", 7, "square", "continents", "56b9ce3608185d7a580e17ba194298403092a707b159418827f3d543ded351f4"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "c30f7c119c84236e00333d8ed0cc7bc7effc8a7b8fe6181e98f60499f5555a28"},
		{"seed42-square-islands", 42, "square", "islands", "4dbe378462ef7be4c791495d47d616cecd7fef9dae9b4632e337c6fd84d1bde3"},
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
		{"seed42-cinematic", 42, "cinematic", "c39e80ef5fdae0c0031140717121d639272ac3914efdcf7a3545c9268c2db4d9"},
		{"seed42-square", 42, "square", "ab15a73c49342341a86538ca370aa280b35154bc4602798b150ee71d4cf23fec"},
		{"seed7-cinematic", 7, "cinematic", "ee4d367eebf6cbd0612abafb2e9f9e90d44052a60c2b0b42bb94d4ff1e5895ee"},
		{"seed7-portrait", 7, "portrait", "9bb9bb027aa63296ab5ecc8d9e105425031b32f4f86fb980501962fb188386ea"},
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
		{"seed42-cinematic", 42, "cinematic", "ab62ced978ddd807fd8cb00760b2c77635b6501e33e5d92290d480e8ba01a2bb"},
		{"seed42-square", 42, "square", "18b74b7e8309a4807721eb50380ef3bd586792bd1206695e43b61c8090ebdf8b"},
		{"seed7-cinematic", 7, "cinematic", "761b83dba4bce555aa179b52f149a4d61a8c563949397e27978058bf41032d42"},
		{"seed7-portrait", 7, "portrait", "3655a70942910ecc833ed4be7e1fe8232a74a71633cd4be50ad7883ad82c4c75"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "7d19124320c593ec73becd0ca288069e5c61ad6c9f2928d4381929592aa29e3a"},
		{"seed42-square", 42, "square", "continents", "5c18b7091c7569e00aebce178e09178ddf940af66e72c1c2160717534856dc38"},
		{"seed7-cinematic", 7, "cinematic", "continents", "8314a326d2f9a18a834a3c6c91e6bdab5ea48b5bd2e9219d6424c13f836de60f"},
		{"seed7-portrait", 7, "portrait", "continents", "529321a3471c5419ace40f4fadf6fd158df0b694fbf31fc6395f2a80864c9fc2"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "1d2254d88e4c2fa555067cd4ae08981fac6f918731fc82cb60d94467ea46bbf0"},
		{"seed6-cinematic-pangaea", 6, "cinematic", "pangaea", "8de655206407c9c0d895e6677671442f011db77d766492e95da13ce05c2022a4"}, // gallops
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
		{"seed42-cinematic", 42, "cinematic", "continents", "c2ec4bfa1ad4791f06c91c55ae23358fdaaee33926a637adf752d6b4d6af17aa"},
		{"seed7-square", 7, "square", "continents", "426a3d48a8f5d2218963279259dde3a1eec4cdc2848eed4c5f46636d6972420e"},
		{"seed7-portrait", 7, "portrait", "continents", "69c45566228de8cefe81f8d20625e28988fd8df07e1a93855b544612a0183fb4"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "5a2c455f4504c69f566d371e30432601a37b67fa13e05154deb4a5b76f330900"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "97aaee3d41301860b03f66f1c5eaf3990e53ece3b601c0df5754597c185c590c"},
		{"seed42-square", 42, "square", "continents", "b3453b6e75846c209101e5f3274040c1e8c1190e66956241e78f1bf3238b6a5c"},
		{"seed7-portrait", 7, "portrait", "continents", "657be20829fbc8ef6976285f77a0a717bc0cef2757578bc4e3c1fac8ee6951db"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "447e5775ec22b05a2ea964a37468ed483ede7d4679efab029f41f41c2fb22c08"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "6d936b9dfd744c978c4cc13cf6b97f7c661c6b39202fd69ab051982771cb2ece"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "88fb768d06357472a2fc90999cedbc77901f66a4e512bca19970c3b3ce7dc398"},
		{"seed42-square", 42, "square", "continents", "27ade8ce75ec522106fd9a60e17718e0350fc33df306530f4da6b1b075265c16"},
		{"seed7-portrait", 7, "portrait", "continents", "122210f176c795d2501e23ccaa30625f5fedf6b3fc79a84dc64797ccb29d4eef"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "b9e350c2a82b514e2b5dabfb63378155977a80e5ae4ceea875363ea92160c865"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "dc40e51992441eb8b071b4c381e406439f817082ca57a63e4382baf07c3b4f18"},
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

// TestGoldenClassify pins the classification stage across architectures
// (see package golden for how to record a hash): the pipeline run through
// stage 11, classify, passing over the deferred stages 9 and 10. Each case
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
		{"seed42-cinematic", 42, "cinematic", "continents", "a3b70ae8c2eef91bb36c21271260f257ee0df3d5366a0ff82d966e88c8555b69"},
		{"seed42-square", 42, "square", "continents", "5f68350a1a6541c19ce8d86641bfa1ce5518ce15f1406192379419b7f9500c71"},
		{"seed7-cinematic", 7, "cinematic", "continents", "1e5a69abea07a2fd4d9ee1b26a93c41a4cb61a6323197f949a22ed8db3060db0"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "cabc65c25f46e71d99fae9a88ef4d4fcc1f16fe584ce4daddc271642bd677222"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "8a2d7353f2f72c644139efcefc076fb708ebbb2da16bad6789dfc91eade2c308"}, // volcanic highlands
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
// edges, passing over the deferred stages 9 and 10. Each case hashes, in
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
		{"seed42-cinematic", 42, "cinematic", "continents", "860c8131925aa2cff83ffdb532044098691262cebaf2f613db334a77eb97255b"},
		{"seed42-square", 42, "square", "continents", "81b38f14ffb20f9923b1d915bd62bfe06b2b37a40bca8d09f8548734e76ed68a"},
		{"seed7-cinematic", 7, "cinematic", "continents", "849571a975731aed50bd5ded103a139a4bd8b359615344e36f53e75eebef127a"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "baa3301541a4d3f5fdd8bbd3557fef2bc93c57eb09b1b34779c40033725e2853"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "c23cad04354a7f76135b3a835c7595cfad7fc1e06a304e57b4f2068ce4d15563", "314f796aa17bb346737c2977cf95d1c513de5fe34bce4b88146a7f3897cba59e"},
		{"seed7-square", 7, "square", "continents", "ebeb7244c65a8c1ffcb83bc118a0572dd87ec01511eaf5374cf43bc49f193b2d", "c524babb8af76e581c8999b780e1087df0cc40184cfe43876021af41e88fb7cf"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "6701f5a02da5ff00f86b364dd7fb6f991ef54969fffec44ff4514a2cb5c4e0d6", "fd946e8045933bd9e7d309d8d881eefd331206706ae7b07a9c22f060257820bf"},
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
