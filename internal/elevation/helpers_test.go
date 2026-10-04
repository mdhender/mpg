// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package elevation

import (
	"slices"
	"testing"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/layout"
)

var (
	presets = []string{"pangaea", "continents", "archipelago", "islands"}
	aspects = []string{"cinematic", "square", "portrait"}
)

// resolved returns the default config with the given seed, preset, and
// aspect, resolved.
func resolved(t testing.TB, seed uint64, preset, aspect string) config.Config {
	t.Helper()
	c := config.Default()
	c.Seed = config.Seed(seed)
	c.Layout.Preset = preset
	c.World.Aspect = aspect
	if err := c.Resolve(); err != nil {
		t.Fatal(err)
	}
	return c
}

// world is one built world: its config, bias, and elevation.
type world struct {
	cfg   config.Config
	bias  *field.Field
	elev  *Elevation
	f     *field.Field
	stats Stats
}

// build runs the layout and elevation for c.
func build(t testing.TB, c config.Config) world {
	t.Helper()
	l, err := layout.New(c)
	if err != nil {
		t.Fatal(err)
	}
	bias, err := l.Bias(c.Raster.SpacingKm)
	if err != nil {
		t.Fatal(err)
	}
	e, err := New(c, bias)
	if err != nil {
		t.Fatal(err)
	}
	f, st := e.Field()
	return world{cfg: c, bias: bias, elev: e, f: f, stats: st}
}

// components returns the sizes, in samples, of the 4-connected regions of f
// above level, wrapping east–west, largest first.
func components(f *field.Field, level float64) []int {
	nx, ny := f.NX(), f.NY()
	v := f.Values()
	seen := make([]bool, len(v))
	var sizes, stack []int
	for start := range v {
		if seen[start] || !(v[start] > level) {
			continue
		}
		n := 0
		seen[start] = true
		stack = append(stack[:0], start)
		for len(stack) > 0 {
			k := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			n++
			i, j := k%nx, k/nx
			for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				ii, jj := (i+d[0]+nx)%nx, j+d[1]
				if jj < 0 || jj >= ny {
					continue
				}
				if q := jj*nx + ii; !seen[q] && v[q] > level {
					seen[q] = true
					stack = append(stack, q)
				}
			}
		}
		sizes = append(sizes, n)
	}
	slices.SortFunc(sizes, func(a, b int) int { return b - a })
	return sizes
}

// intent summarizes the land of a world at a sea level: the share of the
// land in the largest region, the number of regions holding at least min of
// the land, and the share of the samples in the playable band (outside the
// rim and falloff) where land agrees with a positive bias.
func intent(w world, level, minShare float64) (largest float64, regions int, agree float64) {
	sizes := components(w.f, level)
	total := 0
	for _, s := range sizes {
		total += s
	}
	if total == 0 {
		return 0, 0, 0
	}
	for _, s := range sizes {
		if float64(s) >= minShare*float64(total) {
			regions++
		}
	}
	c := w.f.Cylinder()
	var n, same int
	w.f.Each(func(i, j int, v float64) {
		if y := w.f.Y(j); c.InRim(y) || c.InFalloff(y) {
			return
		}
		n++
		if (v > level) == (w.bias.At(i, j) > 0) {
			same++
		}
	})
	return float64(sizes[0]) / float64(total), regions, float64(same) / float64(n)
}
