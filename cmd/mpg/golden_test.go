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
		{"seed42-cinematic", 42, "cinematic", "continents", "24a2f5ed8923ec6aa0e7f4d5627dafca58f769f2e4b777fdd2bd6c1f8ae3dfeb"},
		{"seed42-square", 42, "square", "continents", "98d44ab407921d22f5f5d70ef7881589a6ac55f3a0a9e9e29d5e3964b52beb3d"},
		{"seed7-cinematic", 7, "cinematic", "continents", "4007701b2efc73ae4c5b9122a476f63d38a7f3cb36f6821fda5e8b3d1bd074d5"},
		{"seed7-square", 7, "square", "continents", "6d5f2c0c7b281bc6027c4c2c31ee699697e1923be779c5415c5411f4a8c28df6"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "a1458f25ed112f533e61d677c0d9d62651c76b83bd1d24790017a052a7969a59"},
		{"seed42-square-islands", 42, "square", "islands", "b84828e166847e74bcb7afc4dbbe7fcae4a83bc8fd4bfe7752c6642184164467"},
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
		{"seed42-cinematic", 42, "cinematic", "287c56dd940685d631ca66fc19acf5fa250f5e6417d6af4a5bf61fbe922ec8e0"},
		{"seed42-square", 42, "square", "c9e9cb345624f2cabf037c4d7e513a1d97cb807f1a9dd741082132ad7b21a4a7"},
		{"seed7-cinematic", 7, "cinematic", "7644657e5ad21a5fafd3e0f7d7202bad3e3c434e77f72d9d7b27deda2696a3b8"},
		{"seed7-portrait", 7, "portrait", "df980c9dc60b80c745aad21e56eec001e0a34db1814f079fe8111513b173fbe8"},
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
		{"seed42-cinematic", 42, "cinematic", "5b37147542b4f7f2546d8d1c58884a5ceffac08b24190f8951af8a3cd011500a"},
		{"seed42-square", 42, "square", "c9ea915affb2269eb59d847a152fe791d4f333779447bd0bcc9c825e2ce59fc4"},
		{"seed7-cinematic", 7, "cinematic", "bf271308cf59402ae346e7d656608a361dff2f200419d24fec4def5c113b39c2"},
		{"seed7-portrait", 7, "portrait", "5a750b4073d8e1d8ff35eb82e449332e965ceb4f6f5b74acb6345b0f57e5e984"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "99a656ff6eda2ea2a78d64fe04409f647ddc32fc00aca29034d8cc4537ffe0ab"},
		{"seed42-square", 42, "square", "continents", "03cfdc46b485fab16c2239dadaa518a5d75914f3ee8c56515ca864a6e0909ba3"},
		{"seed7-cinematic", 7, "cinematic", "continents", "aae5b796973bcaac477b2aae80d27faeba9d2c72ae0e03ae4175fd9d6067d894"},
		{"seed7-portrait", 7, "portrait", "continents", "9d1a0d3f0328693ff72ae9cf3dd3a61d0982be58f8b5556cc35f0bff197234f0"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "d3f88258437e464002073777ea99c00ad45efb85e13a288673005a204cc835d3"},
		{"seed6-cinematic-pangaea", 6, "cinematic", "pangaea", "f0f8b5774fe8855a2eac6a1cb7fe7bfe975253be5f61291d56dc35ebeb820741"}, // gallops
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
// temperature; and every sample's mask flag in storage order; integers and
// float bits little-endian, flags one byte).
func TestGoldenClimate(t *testing.T) {
	for _, tc := range []struct {
		name   string
		seed   config.Seed
		aspect string
		preset string
		want   string
	}{
		{"seed42-cinematic", 42, "cinematic", "continents", "f8c0f50165bd8d52f9ed4fc93be688fb51d272a8ba18cbb3014115b2c7c0de33"},
		{"seed7-square", 7, "square", "continents", "e0212ae4063a8c05cd33883ee4250babf7fc6dac14d0652fd5009b2ced418479"},
		{"seed7-portrait", 7, "portrait", "continents", "74d808ebba4bcd1798abc7d705f2eaf5dfdf46635637089db461fab98295198b"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "11b816f036a30d93800e8a1dc3b5db990832ab47e7d152a35b934cb0a1c66b02"},
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

// TestGoldenClassify pins the classification stage across architectures
// (see package golden for how to record a hash): the pipeline run through
// stage 11, classify, passing over the deferred stages 8 to 10. Each case
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
		{"seed42-cinematic", 42, "cinematic", "continents", "84b4ef0afb85f3fd5b3722fe1c2c2ab133fc7c3379881214b0f113c0178e1cf6"},
		{"seed42-square", 42, "square", "continents", "c3efe3092490065c5c6bcf791138396759b07b336bf9e9c8b9a0fde8a01ba0a6"},
		{"seed7-cinematic", 7, "cinematic", "continents", "773c95a1ddd6b13a0ff22f31c6262aadd42f76b0363bd23953989fb105fe91fe"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "4674e9d15fcc909c8d74d1ff660977e0159b164752e8a143feab26c38e7abb6f"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "b00592b834f3c2dcce3fd91916373b64e4e20b0d2ba2c4e55d60e32f06045c92"}, // volcanic highlands
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
// edges, passing over the deferred stages 8 to 10. Each case hashes, in
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
		{"seed42-cinematic", 42, "cinematic", "continents", "b1c59a9129a2a97155ec3e283f0304f9bbf1e469fc34ab850e13399c93e1aaaa"},
		{"seed42-square", 42, "square", "continents", "63b6210d00dc37c2d5d8310f76280ebd8c39a8e1acaba16253fead6a54a4b01a"},
		{"seed7-cinematic", 7, "cinematic", "continents", "4290b5459fdaf2b9ece97213eec70b5f2114c5dd3de0375b34d5902274bee90f"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "b511b5d0ba8d9773765c3ede990347de6f82fca86f694920352292e407fd7eaf"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "8477f13b6ef568ae46bc8c9634d6cd7fa60b66bd207882e84373d7646ba1e3d7", "314f796aa17bb346737c2977cf95d1c513de5fe34bce4b88146a7f3897cba59e"},
		{"seed7-square", 7, "square", "continents", "93e135d66a1717a415f3c48dd2ec994e8578ec40ac59c98f4f121ac6b7b5e905", "c524babb8af76e581c8999b780e1087df0cc40184cfe43876021af41e88fb7cf"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "e242573132eef3dbd2cceb22cd7daf86fef2ee8b17fd4ec7855383ec71b78018", "fd946e8045933bd9e7d309d8d881eefd331206706ae7b07a9c22f060257820bf"},
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
