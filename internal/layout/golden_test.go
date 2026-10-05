// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package layout

import (
	"testing"

	"github.com/mdhender/mpg/internal/golden"
)

// TestGoldenLayout pins the layout stage across architectures (see package
// golden for how to record a hash). Each case hashes, in order: the resolved
// config.json bytes; the attractor count and each attractor's X, Y,
// RadiusKm, Weight (float64 bits) and Mass; the repulsor count and each
// repulsor's X, Y, RadiusKm, Weight, MassA, and MassB; the raster's NX and
// NY; and every bias sample in storage order.
func TestGoldenLayout(t *testing.T) {
	for _, tc := range []struct {
		name   string
		seed   uint64
		preset string
		aspect string
		want   string
	}{
		{"seed42-cinematic-pangaea", 42, "pangaea", "cinematic", "f38fa43a14203f2217663894773736c00954205fe69070d1b9995371590cce93"},
		{"seed42-cinematic-continents", 42, "continents", "cinematic", "0fe4279c5d4c483243995cf8a17d4bab13e926e65c6e3ed6030e44b7049fecb8"},
		{"seed42-cinematic-archipelago", 42, "archipelago", "cinematic", "ac54a23b898bd59184d0625ddcfe28186bc1ab6b084df7b0609fa27b15c07b3f"},
		{"seed42-cinematic-islands", 42, "islands", "cinematic", "d0849a969cb0d04d2b96a3b17fc74e721e7fa382034e855b35c2a4d744dfdfb4"},
		{"seed7-square-continents", 7, "continents", "square", "e309812f296441767611ea6505ed89450117e8dd5206f8217aaa980fbde22440"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := resolved(t, tc.seed, tc.preset, tc.aspect)
			b, err := c.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			l, f := build(t, c)
			h := golden.New()
			h.Bytes(b)
			h.Int(len(l.Attractors))
			for _, a := range l.Attractors {
				h.Float64s(a.X, a.Y, a.RadiusKm, a.Weight)
				h.Int(a.Mass)
			}
			h.Int(len(l.Repulsors))
			for _, r := range l.Repulsors {
				h.Float64s(r.X, r.Y, r.RadiusKm, r.Weight)
				h.Int(r.MassA)
				h.Int(r.MassB)
			}
			h.Int(f.NX())
			h.Int(f.NY())
			h.Float64s(f.Values()...)
			golden.Check(t, "layout/"+tc.name, h.Sum(), tc.want)
		})
	}
}
