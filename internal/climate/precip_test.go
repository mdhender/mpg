// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package climate

import (
	"math"
	"strings"
	"testing"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/fmath"
)

// TestPET checks Holdridge's PET: 58.93 mm per °C of biotemperature, the
// mean annual temperature clamped to [0, 30] °C.
func TestPET(t *testing.T) {
	m := defaultModel(t)
	for _, tc := range []struct{ t, want float64 }{
		{-40, 0}, {-0.1, 0}, {0, 0}, {1, 58.93}, {10, 589.3}, {27, 1591.11}, {30, 1767.9}, {45, 1767.9},
	} {
		if got := m.PET(tc.t); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("PET(%v) = %v, want %v", tc.t, got, tc.want)
		}
	}
}

// TestRunoff checks Budyko's curve: the value at φ = 1, the limits (no
// rain, no evaporation, PET ≪ P, PET ≫ P, a vanishing P), that runoff is
// finite and in [0, P], and that it falls as PET rises and rises with P.
func TestRunoff(t *testing.T) {
	// φ = 1: E/P = √(tanh(1)·(1 − e⁻¹)) = 0.693844...
	if got, want := Runoff(1000, 1000), 1000*(1-math.Sqrt(math.Tanh(1)*(1-math.Exp(-1)))); math.Abs(got-want) > 1e-9 {
		t.Errorf("Runoff(1000, 1000) = %v, want %v", got, want)
	}
	for _, tc := range []struct{ p, pet, want, tol float64 }{
		{0, 1000, 0, 0},          // no rain, no runoff
		{-5, 1000, 0, 0},         // never negative
		{1000, 0, 1000, 0},       // no evaporation: all runs off
		{1000, 1e-9, 1000, 1e-6}, // tiny PET: E ≈ PET
		{1000, 1, 999, 1e-3},     // PET ≪ P: E ≈ PET
		{1, 1e5, 0, 1e-9},        // PET ≫ P: everything evaporates
		{1e-300, 1000, 0, 0},     // vanishing P: cut, not 0 × ∞
		{math.SmallestNonzeroFloat64, 1, 0, 0},
		{3000, 1500, 3000 * (1 - math.Sqrt(0.5*math.Tanh(2)*(1-math.Exp(-0.5)))), 1e-9},
	} {
		got := Runoff(tc.p, tc.pet)
		if math.IsNaN(got) || math.IsInf(got, 0) || math.Abs(got-tc.want) > tc.tol {
			t.Errorf("Runoff(%v, %v) = %v, want %v", tc.p, tc.pet, got, tc.want)
		}
	}
	for _, p := range []float64{50, 300, 800, 2000, 4000} {
		prev := p
		for pet := 0.0; pet <= 3000; pet += 25 {
			r := Runoff(p, pet)
			if !(r >= 0 && r <= p) || r > prev {
				t.Fatalf("Runoff(%v, %v) = %v (previous %v): outside [0, P] or rising with PET", p, pet, r, prev)
			}
			prev = r
		}
	}
	for _, pet := range []float64{100, 800, 1600} {
		prev := 0.0
		for p := 10.0; p <= 5000; p += 10 {
			r := Runoff(p, pet)
			if r < prev {
				t.Fatalf("Runoff(%v, %v) = %v falls below %v as P rises", p, pet, r, prev)
			}
			prev = r
		}
	}
}

// TestAridity checks the aridity index (P/PET, capped at 10, and 10 with
// no evaporation) and the UNEP classes at their breaks.
func TestAridity(t *testing.T) {
	for _, tc := range []struct{ p, pet, want float64 }{
		{300, 600, 0.5}, {10, 1000, 0.01}, {1000, 0, AridityCap}, {0, 0, AridityCap}, {50000, 10, AridityCap}, {0, 500, 0},
	} {
		if got := AridityIndex(tc.p, tc.pet); got != tc.want {
			t.Errorf("AridityIndex(%v, %v) = %v, want %v", tc.p, tc.pet, got, tc.want)
		}
	}
	for _, tc := range []struct {
		ai   float64
		want Aridity
	}{
		{0, HyperArid}, {0.0499, HyperArid}, {0.05, Arid}, {0.1999, Arid}, {0.2, SemiArid}, {0.4999, SemiArid},
		{0.5, DrySubhumid}, {0.6499, DrySubhumid}, {0.65, Humid}, {1, Humid}, {AridityCap, Humid},
	} {
		if got := AridityOf(tc.ai); got != tc.want {
			t.Errorf("AridityOf(%v) = %v, want %v", tc.ai, got, tc.want)
		}
	}
	names := []string{"hyper-arid", "arid", "semi-arid", "dry-subhumid", "humid"}
	for a := range NumAridity {
		if got := Aridity(a).String(); got != names[a] {
			t.Errorf("Aridity(%d) = %q, want %q", a, got, names[a])
		}
	}
}

// TestWinds checks the wind bands: the trades from 60° (south of the
// equator 120°), the westerlies from 255° (285°), the polar easterlies from
// 60° (120°), blended linearly between their bands and across ±5° of the
// equator, with weights summing to 1 everywhere.
func TestWinds(t *testing.T) {
	m := defaultModel(t)
	type w struct{ from, weight float64 }
	for _, tc := range []struct {
		lat  float64
		want []w
	}{
		{10, []w{{60, 1}}},
		{-10, []w{{120, 1}}},
		{27, []w{{60, 1}}},
		{30, []w{{60, 0.5}, {255, 0.5}}},
		{-30, []w{{120, 0.5}, {285, 0.5}}},
		{45, []w{{255, 1}}},
		{-45, []w{{285, 1}}},
		{60, []w{{255, 0.5}, {60, 0.5}}},
		{80, []w{{60, 1}}},
		{-80, []w{{120, 1}}},
		{0, []w{{60, 0.5}, {120, 0.5}}},
		{2.5, []w{{60, 0.75}, {120, 0.25}}},
		{-2.5, []w{{60, 0.25}, {120, 0.75}}},
		{5, []w{{60, 1}}},
	} {
		from, weight := m.WindsAt(tc.lat)
		if len(from) != len(tc.want) {
			t.Errorf("lat %v: winds %v %v, want %v", tc.lat, from, weight, tc.want)
			continue
		}
		for k, x := range tc.want {
			if from[k] != x.from || math.Abs(weight[k]-x.weight) > 1e-12 {
				t.Errorf("lat %v: wind %d from %v weight %v, want %v", tc.lat, k, from[k], weight[k], x)
			}
		}
	}
	for lat := -90.0; lat <= 90; lat += 0.125 {
		_, weight := m.WindsAt(lat)
		sum := 0.0
		for _, v := range weight {
			if !(v > 0 && v <= 1) {
				t.Fatalf("lat %v: weight %v", lat, v)
			}
			sum += v
		}
		if math.Abs(sum-1) > 1e-12 || len(weight) > 2 {
			t.Fatalf("lat %v: %d winds, weights sum to %v", lat, len(weight), sum)
		}
	}
	c := config.DefaultClimate()
	c.EquatorBlendDeg = 0
	m0, err := NewModel(c)
	if err != nil {
		t.Fatal(err)
	}
	if from, _ := m0.WindsAt(0); len(from) != 1 || from[0] != 60 {
		t.Errorf("no blend: winds at the equator %v, want the northern trades", from)
	}
	if from, _ := m0.WindsAt(-1e-9); len(from) != 1 || from[0] != 120 {
		t.Errorf("no blend: winds just south of the equator %v, want the southern trades", from)
	}
}

// TestCurve checks the latitude tables: the points, linear in between,
// flat beyond the ends.
func TestCurve(t *testing.T) {
	m := defaultModel(t)
	for _, p := range config.DefaultClimate().WindwardPrecipMm {
		if got := m.WindwardAt(p.LatDeg); got != p.Mm {
			t.Errorf("windward at %v° = %v, want %v", p.LatDeg, got, p.Mm)
		}
	}
	if got := m.WindwardAt(2.5); got != 3200 {
		t.Errorf("windward at 2.5° = %v, want 3200", got)
	}
	if got := m.p.share.at(50); got != 0.2 {
		t.Errorf("share beyond the last point %v, want 0.2", got)
	}
	if got := m.p.share.at(5); math.Abs(got-0.475) > 1e-15 {
		t.Errorf("share at 5° = %v, want 0.475", got)
	}
}

// TestTrace checks one ray of the upwind trace on a synthetic grid 200 km
// around and 20 km tall at 1 km per sample, one cell per sample: ocean in
// columns 0 to 19, land at 0 m elsewhere, and a 1,500 m ridge in columns
// 60 to 69. It checks the step count to the sea, the rise over the ridge
// (the rain shadow), the lift window, the reach with no sea (the air
// starts from the sea one step beyond), the east–west wrap, and the polar
// edges.
func TestTrace(t *testing.T) {
	const nx, ny = 200, 20
	g := &grid{nx: nx, ny: ny, w: nx, h: ny, px: 1, py: 1, owner: make([]int, nx*ny), ocean: make([]bool, nx*ny), height: make([]float64, nx*ny)}
	for k := range g.owner {
		g.owner[k] = k
		switch i := k % nx; {
		case i < 20:
			g.ocean[k] = true
		case i >= 60 && i < 70:
			g.height[k] = 1500
		}
	}
	p := &precip{step: 1, steps: 1000, window: 30, rainout: 500, orographic: 1200}
	moist := func(n, rise float64) float64 { return fmath.Exp(-(n / 500) - rise/1200) }
	for _, tc := range []struct {
		name             string
		x, y, from, self float64
		steps            int
		moisture, lift   float64
	}{
		{"windward", 50.5, 10.5, 270, 0, 1000, moist(31, 0), 0},
		{"lee", 100.5, 10.5, 270, 0, 1000, moist(81, 1500), 0},
		{"ridge", 65.5, 10.5, 270, 1500, 1000, moist(46, 1500), 1500},
		{"no sea in reach", 100.5, 10.5, 270, 0, 10, moist(11, 0), 0},
		{"across the seam", 195.5, 10.5, 90, 0, 1000, moist(5, 0), 0},
		{"off the north edge", 100.5, 5.5, 0, 0, 1000, moist(6, 0), 0},
		{"off the south edge", 100.5, 15.5, 180, 0, 1000, moist(5, 0), 0},
	} {
		p.steps = tc.steps
		m, l := p.trace(g, tc.x, tc.y, tc.from, tc.self)
		if math.Abs(m-tc.moisture) > 1e-15 || l != tc.lift {
			t.Errorf("%s: moisture %v lift %v, want %v and %v", tc.name, m, l, tc.moisture, tc.lift)
		}
	}
}

// TestNewModelPrecipErrors checks that NewModel rejects precipitation
// inputs the trace cannot use.
func TestNewModelPrecipErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*config.Climate)
		want string
	}{
		{"windward zero", func(c *config.Climate) { c.WindwardPrecipMm[3].Mm = 0 }, "windward_precip_mm[3].mm"},
		{"noise amp 1", func(c *config.Climate) { c.PrecipNoiseAmp = 1 }, "precip_noise_amp"},
		{"step", func(c *config.Climate) { c.StepKm = 0 }, "step_km"},
		{"rays", func(c *config.Climate) { c.WindRays = 0 }, "wind_rays"},
	} {
		c := config.DefaultClimate()
		tc.edit(&c)
		if _, err := NewModel(c); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: NewModel = %v, want an error containing %q", tc.name, err, tc.want)
		}
	}
}
