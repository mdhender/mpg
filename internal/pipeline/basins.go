// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package pipeline

import (
	"fmt"
	"image"
	"strings"

	"github.com/mdhender/mpg/internal/basin"
	"github.com/mdhender/mpg/internal/cells"
	"github.com/mdhender/mpg/internal/mesh"
)

// BasinSeed returns the seed cells of the basin flood at sea level sl: the
// rim cells and the ocean.
func BasinSeed(m *mesh.Mesh, sl *cells.Flood) []bool {
	seed := make([]bool, len(m.Cells))
	for i, c := range m.Cells {
		seed[i] = c.Rim || sl.Ocean[i]
	}
	return seed
}

// DepthBins are the upper bounds, in meters, of the basins stage's depth
// histogram bins; the last bin is open.
var DepthBins = []float64{10, 25, 50, 100, 200, 500}

// DepthHistogram counts the depressions of r in DepthBins.
func DepthHistogram(r *basin.Result) []int {
	h := make([]int, len(DepthBins)+1)
	for _, d := range r.Depressions {
		k := 0
		for k < len(DepthBins) && d.DepthM >= DepthBins[k] {
			k++
		}
		h[k]++
	}
	return h
}

// runBasins finds the basin hierarchy (package basin) on the sea level
// stage's ocean at basin.min_depth_m, logs the depressions found, the
// basins kept, their depth histogram, sizes and nesting, and renders the
// basins and, as variant "depth", their depth below the spill level.
func runBasins(c *Context) error {
	m, s, sl := c.Products.Mesh, c.Products.Cells, c.Products.SeaLevel
	seed := BasinSeed(m, &sl.Flood)
	r, err := basin.Find(m, s.Altitude, seed, c.Config.Basin.MinDepthM)
	if err != nil {
		return err
	}
	h := DepthHistogram(r)
	bins := make([]string, len(h))
	lo := 0.0
	for k, n := range h {
		if k < len(DepthBins) {
			bins[k] = fmt.Sprintf("%g–%g m %d", lo, DepthBins[k], n)
			lo = DepthBins[k]
		} else {
			bins[k] = fmt.Sprintf("%g+ m %d", lo, n)
		}
	}
	c.Logf("%d depressions; depth %s", len(r.Depressions), strings.Join(bins, ", "))
	in, below, nest, largest := 0, 0, 0, 0
	for _, b := range r.Of {
		if b != basin.None {
			in++
		}
	}
	top := 0
	for b, bs := range r.Basins {
		if bs.BottomM <= sl.Level {
			below++
		}
		if bs.Parent == basin.None {
			top++
			largest = max(largest, bs.Size)
		}
		nest = max(nest, r.Nesting(b))
	}
	c.Logf("%d basins at least %g m deep (%d top-level, nesting to depth %d, %d with the bottom at or below sea level); %d cells in basins, largest %d",
		len(r.Basins), r.MinDepthM, top, nest, below, in, largest)
	f := c.Products.Elevation
	for _, v := range []struct {
		name string
		draw func() *image.RGBA
	}{
		{"", func() *image.RGBA { return basin.Render(f, m, s.Altitude, seed, sl.Level, r) }},
		{"depth", func() *image.RGBA { return basin.DepthRender(f, m, s.Altitude, seed, sl.Level, r) }},
	} {
		if !c.rendering() {
			break
		}
		if err := c.Render(v.name, v.draw()); err != nil {
			return err
		}
	}
	c.Products.Basins = r
	return nil
}
