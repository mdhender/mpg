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
		{"seed42-cinematic-pangaea", 42, "pangaea", "cinematic", "82386678339367d2c3299e07648bf3dd959221c2abb8f4587a4167ff9f9a2c96"},
		{"seed42-cinematic-continents", 42, "continents", "cinematic", "b44bb334874412a14850400c6a45e1ddcfbe453cf4f69bcdc9c5a6002dfc9952"},
		{"seed42-cinematic-archipelago", 42, "archipelago", "cinematic", "00b823f30c71e61b56a88d33431b7929a324cb4245d9003dc969ed486d505c6e"},
		{"seed42-cinematic-islands", 42, "islands", "cinematic", "8c0539ad6c02d53b1e1286d435cc6bacd693becf5bcce791e50bb76152add551"},
		{"seed7-square-continents", 7, "continents", "square", "e2dc5244d2c40ab9fb6773020e3b8815cdbb6971301170e2af402ec9e70fd618"},
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
