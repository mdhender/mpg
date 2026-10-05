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
		{"seed42-cinematic-pangaea", 42, "pangaea", "cinematic", "e053f1172186921ee7e4ff38fba13108fd08a0df7370410cef7fa1cb991bce76"},
		{"seed42-cinematic-continents", 42, "continents", "cinematic", "89df52faad2c64672d030f46430c4d5b4973ae10ce518b0227b8913913f591a9"},
		{"seed42-cinematic-archipelago", 42, "archipelago", "cinematic", "b22d6bc7e624043fc4999e94a334b9863f32b6d30b9eb478ca4d8f09c059a782"},
		{"seed42-cinematic-islands", 42, "islands", "cinematic", "6a2c0a18593c7336a871773cda679ca1e1d020c3951ee1512e4399b304fe5322"},
		{"seed7-square-continents", 7, "continents", "square", "259b5a85be0c8eb5d8d61e1ce5d2e8b4d51e5b22bba466b41d4e7f6d9c015848"},
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
