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
		{"seed42-cinematic-pangaea", 42, "pangaea", "cinematic", "9ac9e4e7b51b073ed4cf740bad70de0273b1ef324fcdfceb102a1fe395fb9863"},
		{"seed42-cinematic-continents", 42, "continents", "cinematic", "b2f8d621a748fd878e7ac4365565e0d7343d76f9f734a3ea5a54ae59ff372421"},
		{"seed42-cinematic-archipelago", 42, "archipelago", "cinematic", "df8b87cd5b1b7ab89d047b4ba71310855cb46751b99289c6df29429845ac2a45"},
		{"seed42-cinematic-islands", 42, "islands", "cinematic", "5ffd765f397396945c4869a250c2f69b9da9bc89aba44cd067b178af37d1fa0d"},
		{"seed7-square-continents", 7, "continents", "square", "c962eaf26cc9eb7274e0c051959e39dd2a15273531bf58208fde2a86c35b4eb4"},
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
