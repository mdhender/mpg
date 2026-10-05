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
		{"seed42-cinematic", 42, "cinematic", "continents", "96208167f0a1955fc7e8ede9693710f7afa47eaacdd04338c9ab78fe90255d51"},
		{"seed42-square", 42, "square", "continents", "1a1907fe6181b7e5aa4c46cf695241417b6520fae8c35359b2da7eabf7cbebe0"},
		{"seed7-cinematic", 7, "cinematic", "continents", "362f0d49b4a093fbb01e6715f7cb5d65fedc393b6fbacc9bd7a67e6f5d2ef880"},
		{"seed7-square", 7, "square", "continents", "5ece01691e6f9e11ea06c2b3597c82024e8bfe00b77e93dd2dcdc24ac73e79ed"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "d54757195a6a0c131180378c992b1ce3c574fe6ff65cae3395ecf9822a0f0c94"},
		{"seed42-square-islands", 42, "square", "islands", "136d75abdf3bd5e3d4b24882594f99cd5acdef60abc363221bc29f2860bab726"},
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
		{"seed42-cinematic", 42, "cinematic", "ebe452abdc67724608ead793404b3041a25e5ad7884f32047ced03551860c49e"},
		{"seed42-square", 42, "square", "144faf19696d3e9d83c46a65302bc498276325ca95bb68bcab4721188567f249"},
		{"seed7-cinematic", 7, "cinematic", "144bc474b888b35d246d46d993c4c8f8a64e9a66e79d91bf3b1c14a7f8c0be73"},
		{"seed7-portrait", 7, "portrait", "69bb426c3cf75d1b1ff5094cd5b03711063093443236acfe8a3e97a5631d8ee0"},
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
		{"seed42-cinematic", 42, "cinematic", "069a719f17a60f950a1650f7cc36b062d37665b428b07fbc1c7faa9cc2c54ec9"},
		{"seed42-square", 42, "square", "b19e0e7db8bc087991aa77ff8e78eda261d0ee7ff06c177aa218a753fc6061aa"},
		{"seed7-cinematic", 7, "cinematic", "ee23554847bed6af3a06fef9fc38dc676e929037cb5a5c294a3f76c652fa8276"},
		{"seed7-portrait", 7, "portrait", "3bb1f422ff9025317613494f1b77475cdbf626896c1f78727a3435d1d1f2e267"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "ad7e47aaf66d2bf1efa8341795ade7485c92c25289715ba4c9a0b1eff9cfee2c"},
		{"seed42-square", 42, "square", "continents", "da778ede98ab50e1bc2b576675e9e40131f89cf52597897b3d2e487247ecd909"},
		{"seed7-cinematic", 7, "cinematic", "continents", "df0b8a9779fa13d8cc02f0ad84ddd4d8d27e7e6d668fbb9c89d90667e1cb4a6a"},
		{"seed7-portrait", 7, "portrait", "continents", "e463307337ebe8cca6715b34a95c609aa1b1e5ad09fb7105bad9a288820bc10f"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "9a94c4f6c37a80cc8fa232e7fb808a3391b05782fe62ebe1612faec9b4cb48f6"},
		{"seed6-cinematic-pangaea", 6, "cinematic", "pangaea", "adc590c59825cc7d936009d53f175bc1f146a9f6f12ec8d0e65944fd4ad1c7aa"}, // gallops
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

// TestGoldenClassify pins the classification stage across architectures
// (see package golden for how to record a hash): the pipeline run through
// stage 11, classify, passing over the deferred stages 7 to 10. Each case
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
		{"seed42-cinematic", 42, "cinematic", "continents", "34528846e185aadc8120aadb33b441daffab10fd06bd37747ffe8aa134d9f9cb"},
		{"seed42-square", 42, "square", "continents", "fc2fbfe4ffe2686d42bec65c6a5a90221fd6fd48d5b8bbabf2cea3326ad0d83f"},
		{"seed7-cinematic", 7, "cinematic", "continents", "5f6c30cdf45b4981f2c2c683584f97257c287abc5de33a8863e887c00d960c9f"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "4a9feefc7431c319fa637c5bd4547321e0551f8c74cf850e87cd88ce5cb61de6"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "57610185db5503c2ae5ab92c78d4237fc82b5b0b329d77bbf73d29f60ed09429"}, // volcanic highlands
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
// edges, passing over the deferred stages 7 to 10. Each case hashes, in
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
		{"seed42-cinematic", 42, "cinematic", "continents", "d8033f8a9405a75aaf1f8a943cd91fa3a417e172d2f941a0ea23d3c23a9c1bff"},
		{"seed42-square", 42, "square", "continents", "43cf768bf819de5f29949630178d133e5fb143172a6bb453304ada505b9ed848"},
		{"seed7-cinematic", 7, "cinematic", "continents", "a361f7edb73d77d4236c215867f5c49933af15a651959ead8e6e4aa388cf28cc"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "27e9c0b4b430d26a742a6c25abace17de7afc470a8117aa483dc8d070e3f5bd3"},
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
// covered too.
func TestGoldenWorld(t *testing.T) {
	for _, tc := range []struct {
		name   string
		seed   config.Seed
		aspect string
		preset string
		want   string
	}{
		{"seed42-cinematic", 42, "cinematic", "continents", "267b576435f785d5f023613798ee06be65b02eaf8f6f6ee15f20d42df38a91f5"},
		{"seed7-square", 7, "square", "continents", "e03a9a7e42c8e6788715befe4654a16f7a2d11594db4ddd0c0efd5703d3a6d24"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "7ba422fd7cd15a036e14e57895373c3ffdf7d3e16ce474e01a713c4a89b84da0"},
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
		})
	}
}
