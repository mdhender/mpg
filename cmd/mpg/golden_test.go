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
// corner ids, edge ids and neighbor ids; each corner's position, boundary
// flag, cell ids and edge ids; each edge's cells and corners; all in id
// order).
func TestGoldenMesh(t *testing.T) {
	for _, tc := range []struct {
		name   string
		seed   config.Seed
		aspect string
		want   string
	}{
		{"seed42-cinematic", 42, "cinematic", "4c5ac4ed16e2fd23e94056341503fe9050f508c2ca0ed32ff9f3e8d0835bb3db"},
		{"seed42-square", 42, "square", "547af7d85178e4cd6c90556771acbad40c9112ca4ea905eca7193a839aae7234"},
		{"seed7-cinematic", 7, "cinematic", "fc50094d8f92436f9b4a522c4243b90a3541cc0d4da807ba46ca1825078d00f0"},
		{"seed7-portrait", 7, "portrait", "96c46f83f16d72fe7609dca2bffb4f6fb657a9b45e4c18bee559a79064129622"},
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
