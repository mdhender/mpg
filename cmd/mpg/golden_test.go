// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/golden"
	"github.com/mdhender/mpg/internal/pipeline"
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
		{"seed42-cinematic", 42, "cinematic", "continents", "026af5241170bc90a5054bf771135fa77bab89f582ae2c9bb60b925a2ff42842"},
		{"seed42-square", 42, "square", "continents", "a143b239d295ed9e9e7ac5fd29f022e5e9d6d5a0d66a2572029f0063f1702ad0"},
		{"seed7-cinematic", 7, "cinematic", "continents", "561f9e194a37766c9fce1d36811dd0dd21f63cfb2e58c62b62204f6d8bbb27fd"},
		{"seed7-square", 7, "square", "continents", "e99bc2c268e89556d43759445e5ade440f0a310c8195920c2fef029abd8705d5"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "10a2f487db9b740969a61d5070cbfa27f019fbbe73f7cd2901659956cd22b258"},
		{"seed42-square-islands", 42, "square", "islands", "315b3a25adeaa2f3a0bd71fbecac04540062ce13dd8669fd570cfc089f8a839f"},
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
		{"seed42-cinematic", 42, "cinematic", "a2090c832cfe41a172265cf9477cc7207c101dbe444514a270b97f7e43d0e90d"},
		{"seed42-square", 42, "square", "2cfa32af4a83ab8a3507e8a53527cc0ef33bac827e78175a03fc352ec59421c2"},
		{"seed7-cinematic", 7, "cinematic", "8e3d66379ead5f4986d3d7d1d1d9ea56a2a1d2f121a0d3e216d0596af878f75b"},
		{"seed7-portrait", 7, "portrait", "7c8a382c3e8cad181fa15192373eaa1cf356a693a7627910d87c0f793fbc44f3"},
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
		{"seed42-cinematic", 42, "cinematic", "5852f697e836a8b989f91ceaf8ecf7a4e6d2b44402aeeb452f52bbfb027335a3"},
		{"seed42-square", 42, "square", "8d5a420ce768fdc898b092cc0a5ba0cf2368757fcabada76d7a22d4efbfc6b1c"},
		{"seed7-cinematic", 7, "cinematic", "ce1a911a4c14ead3ba04fa92bfb2381da440f574424d33cf639300cb0d0e2737"},
		{"seed7-portrait", 7, "portrait", "6c74bee60a5f6f876bf2b41ab6d5a30682c99c7aa8d14f669a0693a1fa512b63"},
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
		{"seed42-cinematic", 42, "cinematic", "continents", "02cbe609d4a8169005e60eb8825655ea23ca320ac13ed9aac35c688c7c9ade16"},
		{"seed42-square", 42, "square", "continents", "b463d27abe851d5d5af81b10c7f355384605a2a2a1254879f090787109a32474"},
		{"seed7-cinematic", 7, "cinematic", "continents", "cc66be8f2777a4b5fd5ca62592d5c1e9c025534bf4d46cfde437e1b94fd2dacb"},
		{"seed7-portrait", 7, "portrait", "continents", "2491223c655a40920996680080197fe2570f979d5d666e1158e7d2481d554e6f"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago", "b744eda64585bd8d0e58bc10623b339799d924e85d0c8bbb98cd3f25020efd0a"},
		{"seed6-cinematic-pangaea", 6, "cinematic", "pangaea", "bf61c21f1c7fcf256be06a19e37deb5a75dfc3fff68f3231edbf0ea9d938ad0c"}, // gallops
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
