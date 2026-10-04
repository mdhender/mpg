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
		{"seed42-cinematic-pangaea", 42, "pangaea", "cinematic", "98cb9dc085f923b010818ae1c5d57a3d15f00da2210e348856655f771a930175"},
		{"seed42-cinematic-continents", 42, "continents", "cinematic", "3f14c07f422a6d8fa1ca64b1f25951165eac47da482917d0a13decf562393f14"},
		{"seed42-cinematic-archipelago", 42, "archipelago", "cinematic", "be701140703a7d81b6215cecd67a969fdd7163cf597fccd09cfc266f4c92f8a2"},
		{"seed42-cinematic-islands", 42, "islands", "cinematic", "448fe80e35712b38f4f2ecc59b7cf48a48fea06bc1281fe30b70307269a34a02"},
		{"seed7-square-continents", 7, "continents", "square", "48df21dbdc4c231a71dd4f03b7f2cc26d109e847b6f42294d044f52b23dd66a0"},
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
