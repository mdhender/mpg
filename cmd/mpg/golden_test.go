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
		{"seed42-cinematic", 42, "cinematic", "89e17a7b5c2b72c3858cd41465ce32106d86db31954096ed80573f4155484dc5"},
		{"seed42-square", 42, "square", "6174ad77d2aa1c1b8badfefe5066d82e6a37437817c0bb1357ac71c3b745cf06"},
		{"seed7-cinematic", 7, "cinematic", "826646aa58738c848d2f2c6b54dd5aba52b47afe5bd5089886b8d9877f77f88d"},
		{"seed7-square", 7, "square", "b22191587541709a34dafebe124e29e12f61180174f8766ba6b427881c35db9e"},
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
