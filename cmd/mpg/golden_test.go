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
		{"seed42-cinematic", 42, "cinematic", "continents", "6158f78736d989acf60e44bb6a3cea3703ae5eae7ac6bc9d977fdbfe4e96ae2e"},
		{"seed42-square", 42, "square", "continents", "1c0240aa49bc2dd3bbf86018589f17d230d72b1c9a2a32b8746238373488fa99"},
		{"seed7-cinematic", 7, "cinematic", "continents", "a15be4762d829ab8fdb9748f1181bb617af6f7d11f50bc1b92369880e33c572a"},
		{"seed7-square", 7, "square", "continents", "a5d91e8311453eac6b68b1950a1a59fde9b55f5a1f060234d5a4cb261e677c8f"},
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea", "8602ce1e33a68e6a4b9e836b69cdf8bf8ac75ca4e38218fa5a30c98a2542aed8"},
		{"seed42-square-islands", 42, "square", "islands", "0cd8ef23eb09ddcccca7ce37076d164f5d283aa03cf6fe8292ce57d6493ba7a0"},
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
