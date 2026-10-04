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
		{"seed42-cinematic-pangaea", 42, "pangaea", "cinematic", "7730c7231fb6d1754f4ede20d056a36a88b7c2c918c3c248cb2687941232e64e"},
		{"seed42-cinematic-continents", 42, "continents", "cinematic", "049569224de790e50425e94829f13d1464bd91fd6d1586b7fe9ac304a80a2851"},
		{"seed42-cinematic-archipelago", 42, "archipelago", "cinematic", "be7c0c0a5dc3a5b9078b99a7020523cf284d6805a3b098c5bed0c75de673c255"},
		{"seed42-cinematic-islands", 42, "islands", "cinematic", "ca4aef557a90b2e3e706b5a17645c45395f4559b8c0c502817879fbb923d48f0"},
		{"seed7-square-continents", 7, "continents", "square", "89b70ea0772cc0bab6bd015f8f2f079a45b3f719a6cc616aa8cee926f83af698"},
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
