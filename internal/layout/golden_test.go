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
		{"seed42-cinematic-pangaea", 42, "pangaea", "cinematic", "1e54a304be84243bae130024d29ca1d5dcd81c75508e69c0f42c98b6c18de374"},
		{"seed42-cinematic-continents", 42, "continents", "cinematic", "e2140af433d42a3e583d551a357dd4a3b716b6b7eb3921ff86052de275ffced2"},
		{"seed42-cinematic-archipelago", 42, "archipelago", "cinematic", "4dddf2f3f6fff97958168a0f70a9ae581161b7406195969effbb29b402242a1d"},
		{"seed42-cinematic-islands", 42, "islands", "cinematic", "45d363dc160e7254d3c73150530796f00beef7de48ec9eae88802176923bbb1a"},
		{"seed7-square-continents", 7, "continents", "square", "86cb0159e30968f3acb54fe50211558a1536595e14ca910c37c5a2d5e218ac0d"},
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
