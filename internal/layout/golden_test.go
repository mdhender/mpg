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
		{"seed42-cinematic-pangaea", 42, "pangaea", "cinematic", "517171430bcb5f58eb082474702b091b5333c6af8a5c3439843307dc0b98a26d"},
		{"seed42-cinematic-continents", 42, "continents", "cinematic", "d90eb6cfd19de3ce48c246bb3243c4475bec00c7c7c4724864953b63d0904670"},
		{"seed42-cinematic-archipelago", 42, "archipelago", "cinematic", "ba29807e86e8e56f0b8c0236cdce7dd83891b7c9fc22eec4917e0f50f8c394a3"},
		{"seed42-cinematic-islands", 42, "islands", "cinematic", "749b9495e138555f99d426771c38d586c495a58a55156e437879e29475b1a94b"},
		{"seed7-square-continents", 7, "continents", "square", "3c83a2f47a73b096c7fa15b2d3a3df56693b274459dd7eec6d38508ee5e7fd89"},
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
