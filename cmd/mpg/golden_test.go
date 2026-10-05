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
		{"seed42-cinematic", 42, "cinematic", "continents", "45653f9aef45eeecf5f897786f05a012904b899a37238d094430144bc351b452"},
		{"seed42-square", 42, "square", "continents", "fc1ce09bd3d4f3192ab8ddf75cbc82426e4bc2aa525b9c5b9f03097a923aca8e"},
		{"seed7-cinematic", 7, "cinematic", "continents", "0a3d8436af190baee38cb602ab584ee170782405e8ca5d987de977a473cbbda8"},
		{"seed7-square", 7, "square", "continents", "dd40654419d68ba20d0a8352bfcdf60821b9bea7a3ade6db687bdaac37e93376"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "0018c52aa2bb67674abab1ef09864bfd4ea4ba43da075027a7e4bd47a5ad38e7"},
		{"seed42-square-islands", 42, "square", "islands", "64b67f185cf5c26e667a1903dc39cbcba84a145a270db9153c277103eb40f34e"},
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
		{"seed42-cinematic", 42, "cinematic", "6fe65a9afa032d5bf7fe0531c51d6ec6315837fa411162d14964382123bfae3a"},
		{"seed42-square", 42, "square", "062057208aae389c2f160b6bc9130910d0c03213b6333d5f9a664bdce8729358"},
		{"seed7-cinematic", 7, "cinematic", "50cbe3190c5f3dca7527a69988499a647fcfa5d399784edbfcbb64425471ed51"},
		{"seed7-portrait", 7, "portrait", "64d97c21f87a0d199f8c0738dc4df127d37c1e446ea98041ae73288bfb8fde2a"},
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
		{"seed42-cinematic", 42, "cinematic", "1e2b229d5556a20c40774434b7820254e51a592503afbd7f49343455a641b7e2"},
		{"seed42-square", 42, "square", "ba7e239f2add0b66ad98617fb82d5e4587bfb50eaab75ad7a2417b47da35e4fb"},
		{"seed7-cinematic", 7, "cinematic", "a82abfe4f0b2c9496920cf34f1fc52ddabd8400de73b283d2261d8c0bf6d646b"},
		{"seed7-portrait", 7, "portrait", "f82b6a1179140436f10e44015ac438697eeb851fd761aac8af01faee56e14124"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "6440abfdee00e687aeafe8866df23c1256fd508d97ba6dc6cf708a5fff0a5be9"},
		{"seed42-square", 42, "square", "continents", "2103315d4e0bc6b3e8f43b5fffb2e6a0474a29421fb1e2818f15447b7e24d9e5"},
		{"seed7-cinematic", 7, "cinematic", "continents", "a93d52fdd0df90a4f12f81c98d470b663fefeb3246bd097654f1a0522d7e6287"},
		{"seed7-portrait", 7, "portrait", "continents", "d45e7468592056824ebfad5d0bf44fef30850dcfb37ca9af270cab011a66fe6f"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "f4a2b7ff29f3711786b8e2e51df6bbc279b75feda708f583c9365321ae5dddf5"},
		{"seed6-cinematic-pangaea", 6, "cinematic", "pangaea", "15736b40ac57c1a0bbee3f8ff2df70d85d6e6ecb815273c50dcaf29e96e14a45"}, // gallops
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
		{"seed42-cinematic", 42, "cinematic", "continents", "de309ca1e78be11818579db9fc32d84939874cdca4a0d005bda76c7bfc4b2a93"},
		{"seed7-square", 7, "square", "continents", "8017bc80bad484387c30a1e7fa02a987a76cb3ccd07bb521904d16977280d625"},
		{"seed7-portrait", 7, "portrait", "continents", "060e7fe2b58dd0f1b7367957f81dd4f10d7d88d882f91d0834146f80d551427c"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "4c505581ca1b0259e20b0946c2d5f694d81922d665928e2def827fb172c7b2c5"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "db7c8317560ae4097630a7f31c2d5c2db286f882685808c31d3881de4cc3bce7"},
		{"seed42-square", 42, "square", "continents", "4773781523c2f1dcc4e917611a26ea49da641b063faeeb7229123582a5fa0589"},
		{"seed7-portrait", 7, "portrait", "continents", "a67a38c0519dd02725ccc0838415146df12b404d1ed93f450e4012b1ac4f406d"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "205e394b9e19f0d9933010edbeff40c28f80815bcb103317b7628f974be1cff6"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "7070db227e3cfbb6be8b3c09c68e9cdb44b9ce8299517a09b0692a6a85deb759"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "3aacf159a6a9550e5888f904fef9ee2edcac70b8505d35ba1fb5e9d13471c9a6"},
		{"seed42-square", 42, "square", "continents", "75a3b9207e9f06adffbdfd66bb8200831fbf9331366a53728376093376718c43"},
		{"seed7-portrait", 7, "portrait", "continents", "16b39edf0a2eeb53e6dfd11cf154455f3dfde529e55938821b75e3ad019c103e"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "272850523828de123d64c81559fa0e71e8281440fc8f67ae3b167035bfb37e22"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "3944ee507444b0e20c07741caa69a2174ffa2405ae7ba2e4e92333e3f0e5a3ac"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "09d6265a4d006caedde4625d6d16294a4d877a8686893d15172baaabedd808dc"},
		{"seed7-square", 7, "square", "continents", "390a0ddcaea8f7c05d2e5aec30ced67220007e374916adc9fec8b9709a1ce3ac"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "3bfb47893880e0bd33bf0bc96b7ec0ad5c9588e46f89dba5ca5f959547fd6283"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "0de39810813cc3db936b4aa7754c6b9b5a75616020a9958a618f1b458177a044"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "3eaf5e1a70dd1d16cac4820fbc9d5dca9ac26ff255ea8f2143e5d50ae113aa41"},
		{"seed42-square", 42, "square", "continents", "3beee006b06b4cabe6c99555a0957b9569058469972ebadd98ea49604977b488"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "9e9635de649787cabf1b3d9044069739316189a145dab561c92c5c31add44ff7"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "1ac7fd9a770cb26cd82c589af2599b02aa9690ac010329db27fdfe9765b7a4c5"},
		{"seed8-square-pangaea", 8, "square", "pangaea", "c135bb05181bb5560faaa67cc205314f7cab2558c334fb87e110f82c69e1fcbb"},
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
// hotspot's cell and whether it is on land; integers little-endian).
func TestGoldenClassify(t *testing.T) {
	for _, tc := range []struct {
		name   string
		seed   config.Seed
		aspect string
		preset string
		want   string
	}{
		{"seed42-cinematic", 42, "cinematic", "continents", "80276f218bec253b7f40b73d976ed97d0e59de3733709bf12562a8f6c832bfcb"},
		{"seed42-square", 42, "square", "continents", "84cf0118306277e3c657a2ea33aff98152ae75565f7b1164291fbefda88fe115"},
		{"seed7-cinematic", 7, "cinematic", "continents", "1cb362acbfa03a7976807fc0dd4d10a267586519d4bbb8116bae75f82ef0d6db"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "925457d3bfbe22709b64ecd602f41be41dfc29cba261a25d7a23cc79004c58cf"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "af120c355c680123649431c550fb6a74b163bc29345f025c8ef7dce2a30be768"}, // volcanic highlands
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
		{"seed42-cinematic", 42, "cinematic", "continents", "b0a14353715ebdb6a11379526c15f5694080f5866b68b496823f0e0d14a2a5e9"},
		{"seed42-square", 42, "square", "continents", "4408667f6fc90bf90acfcfa537169240bb033c30231f5d3de6198b0e123d0817"},
		{"seed7-cinematic", 7, "cinematic", "continents", "3354a7971d401f058829e043f1235fd19d864ac286180a1b21622dd665806afe"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "db6c5a845702fd97fdc13f863b700d7553a0d95b01d95fefd89b55c82d9d89b7"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "15ba683dcc9779869e17781c9ac7846046739682afb526988aaaa2e0409e25b3", "cd40a0536f3e1fc29af3888961540507d55728c6275684272aafc676da5c969d"},
		{"seed7-square", 7, "square", "continents", "69424baf69ed6b6219f532b3f2f777d3f065856d4407ec715d6b927bca814ede", "30035d5767758319b52d975c4d61f5e70d1cf9d3eeb3ee5537458ef3ec90a0bb"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "489986766b7d2977e462ca092bfd619436d8a6ad569a4841828ab84b05c1a645", "d11223042e030405d13f28520c50d28e301afe71ae4d56c14fe340072bb08991"},
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
