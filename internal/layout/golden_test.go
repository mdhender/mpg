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
		{"seed42-cinematic-pangaea", 42, "pangaea", "cinematic", "d0328d691ed10e9a4cd16ce2624614b43c407f938b291bafc3c1d04ed1dd0ee0"},
		{"seed42-cinematic-continents", 42, "continents", "cinematic", "8e83f4036517bd28203c0a831d880964a31a93f53ddafb7ab4cc4d5959dc2110"},
		{"seed42-cinematic-archipelago", 42, "archipelago", "cinematic", "edded750704e9db401a53d26f89524afa171820ca7b71057e15132d119c29e2e"},
		{"seed42-cinematic-islands", 42, "islands", "cinematic", "329de905806ef5692a0c9b4c0d812bcd25cf9f00907fd61f3cfa3bee828a57ff"},
		{"seed7-square-continents", 7, "continents", "square", "824ddc822beb83580a841e60b3e9bd532745875bb7b5b2213a69618f16566fbd"},
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
