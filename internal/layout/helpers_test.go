// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package layout

import (
	"testing"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/field"
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

// build places the layout for c and fills its bias field.
func build(t testing.TB, c config.Config) (*Layout, *field.Field) {
	t.Helper()
	l, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	f, err := l.Bias(c.Raster.SpacingKm)
	if err != nil {
		t.Fatal(err)
	}
	return l, f
}

// components returns the sizes, in samples, of the 4-connected regions of
// positive bias, wrapping east–west, largest first.
func components(f *field.Field) []int {
	nx, ny := f.NX(), f.NY()
	v := f.Values()
	seen := make([]bool, len(v))
	var sizes []int
	var stack []int
	for start := range v {
		if seen[start] || !(v[start] > 0) {
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
			for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				ii, jj := (i+d[0]+nx)%nx, j+d[1]
				if jj < 0 || jj >= ny {
					continue
				}
				q := jj*nx + ii
				if !seen[q] && v[q] > 0 {
					seen[q] = true
					stack = append(stack, q)
				}
			}
		}
		sizes = append(sizes, n)
	}
	// largest first
	for a := range sizes {
		for b := a + 1; b < len(sizes); b++ {
			if sizes[b] > sizes[a] {
				sizes[a], sizes[b] = sizes[b], sizes[a]
			}
		}
	}
	return sizes
}
