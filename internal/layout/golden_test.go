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
		{"seed42-cinematic-pangaea", 42, "pangaea", "cinematic", "ed0410d9b1a0992326a59d538c0c70e13570b046ff8ee4918d7588128acdebd1"},
		{"seed42-cinematic-continents", 42, "continents", "cinematic", "20e54eaab08139dbcf2d0ec11656b2994911fc315ddc455765bdde1823a028eb"},
		{"seed42-cinematic-archipelago", 42, "archipelago", "cinematic", "e0e93eb76c2e6922e3f97f87fdd7c3cb1fe6b874fb61007934a9ddf6b70856d7"},
		{"seed42-cinematic-islands", 42, "islands", "cinematic", "168bfa126c8709655ffa47eb988313c215a0cc37a8709777fb496c1e06a510d6"},
		{"seed7-square-continents", 7, "continents", "square", "e9ba39dd7007f2c3a40db4be28206f00b08d64c89437d20caa16c209eca5f74e"},
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
