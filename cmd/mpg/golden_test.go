// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/golden"
)

// TestGoldenNoisePreview pins today's end-to-end output across architectures
// (see package golden for how to record a hash). Each case hashes, in order:
//
//   - the resolved config.json bytes (golden.Hasher.Bytes, length-prefixed);
//   - the raster's NX and NY (golden.Hasher.Int);
//   - every sample of the full-resolution noise preview field, as float64
//     bits, little-endian, in storage order: rows north to south, columns west
//     to east.
//
// The field is hashed rather than the rendered image, because the color ramp
// quantizes away exactly the last-place differences a fused multiply-add
// makes. The first case is the design example (seed 42, cinematic), and its
// config bytes are checked against testdata/example.json as well. Later
// stages add world.json hashes to this template.
func TestGoldenNoisePreview(t *testing.T) {
	example, err := os.ReadFile("../../internal/config/testdata/example.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		seed   config.Seed
		aspect string
		want   string
	}{
		{"seed42-cinematic", 42, "cinematic", "a3341fc051d366e14cca7b2c62591431adf612ee07f40147cf100c6c36b548e5"},
		{"seed42-square", 42, "square", "4abfe674237b439488ba0d70f9d68a603d8198b5baf91cc053483b94dc395166"},
		{"seed7-cinematic", 7, "cinematic", "6cafb1598873aa23dac290722a6664956f5628aa751d3f9afeee3ed19b894f5f"},
		{"seed7-square", 7, "square", "fa4c0a11f9c321d7036fc3c618a1df59201ad4d2acb10c7dc11c877cb13aef5d"},
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
			if tc.name == "seed42-cinematic" && !bytes.Equal(cfgBytes, example) {
				t.Errorf("config.json differs from testdata/example.json:\n%s", cfgBytes)
			}
			f, err := noiseField(cfg)
			if err != nil {
				t.Fatal(err)
			}
			h := golden.New()
			h.Bytes(cfgBytes)
			h.Int(f.NX())
			h.Int(f.NY())
			h.Float64s(f.Values()...)
			golden.Check(t, "noise-preview/"+tc.name, h.Sum(), tc.want)
		})
	}
}
