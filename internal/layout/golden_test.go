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
		{"seed42-cinematic-pangaea", 42, "pangaea", "cinematic", "328527697256b9d4d2e5cd039de690f2c25ddb3201ef785701a3043f6d1cc05c"},
		{"seed42-cinematic-continents", 42, "continents", "cinematic", "91b4399ac4cf4111ee9061dc9d5089c364ecfc1ed7ad5b07485e7e767416b92a"},
		{"seed42-cinematic-archipelago", 42, "archipelago", "cinematic", "6aa6b400eff0d05fdf52a3f31226b60ad9c618f9cc588225cb5e541b15c53c23"},
		{"seed42-cinematic-islands", 42, "islands", "cinematic", "0e9c7e958819827837010ef3c2261220f81eee2097b6ecd2bc10d71b39ceadaf"},
		{"seed7-square-continents", 7, "continents", "square", "f2d881fc67f43ffb5dbd5fd65f91c1ad21836c7cf11e0e547770b3ec860ccc02"},
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
