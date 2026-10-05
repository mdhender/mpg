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
		{"seed42-cinematic-pangaea", 42, "pangaea", "cinematic", "33ae079a2c4228d09c38eac14a419b4f582f7b9e4e759fcb8a0ea74d510477e0"},
		{"seed42-cinematic-continents", 42, "continents", "cinematic", "0afd46fdbe9f6e149d1f4981aa0df66691c674d8cc86acea844851de7263a5a8"},
		{"seed42-cinematic-archipelago", 42, "archipelago", "cinematic", "166c36b77ccb7acec7cc63e3779cd13a56ae45648d5f1c663c33646e072eee5e"},
		{"seed42-cinematic-islands", 42, "islands", "cinematic", "461a3a1a79ea987591db6ca22cc1f4142be340c0cd77e4a41de8db0437a1b29f"},
		{"seed7-square-continents", 7, "continents", "square", "e0477172c37ff68d56526f0798aaf865332d5b47bd15125350ddf7be5deda366"},
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
