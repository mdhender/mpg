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
		{"seed42-cinematic-pangaea", 42, "pangaea", "cinematic", "56d5ef103777b0295211e9bd1a6a0ac2a88e328e753a2ee654509cf37320a236"},
		{"seed42-cinematic-continents", 42, "continents", "cinematic", "28b806af398c885379f93ac02135fc8dd9101f8f7ca443adfa01adb96d8659b2"},
		{"seed42-cinematic-archipelago", 42, "archipelago", "cinematic", "ce831e2a2a69071d4c6f476f633d9b467715049ac61d95b701ea3cd87054c5fc"},
		{"seed42-cinematic-islands", 42, "islands", "cinematic", "884676fa4a2565e1c4f81b83b2ac6de827d0b45c5983d65b2d7ce3e2a405c3b7"},
		{"seed7-square-continents", 7, "continents", "square", "c29c12a2b8cf47ebeeedaf706daf172c6d2e697916b6c7fe99459e6a32d9e931"},
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
