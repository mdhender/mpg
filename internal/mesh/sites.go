// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package mesh

import (
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/internal/seed"
	"github.com/mdhender/mpg/internal/topo"
)

// Stage is the seed stream the sites are drawn from.
const Stage = "mesh"

// Version is the mesh algorithm version, the version argument to the "mesh"
// seed stream. Any change to the bits of the sites or the graph built from
// them takes a new version.
const Version = "mesh/2"

// SiteCount returns the number of sites for a cylinder of the given size and
// province area A: round(W·H / A), the full cylinder, rim included, over A,
// and at least 1.
func SiteCount(w, h, area float64) int {
	return max(1, int(math.Round(w*h/area)))
}

// Sites returns cfg's sites: SiteCount sites on cyl, placed by
// cfg.Mesh.Placement from the "mesh" seed stream.
func Sites(cfg config.Config, cyl topo.Cylinder) ([]topo.Point, error) {
	if cfg.Mesh.Placement != config.PlacementJitteredGrid {
		return nil, fmt.Errorf("mesh: unknown placement %q", cfg.Mesh.Placement)
	}
	n := SiteCount(cyl.W(), cyl.H(), cfg.Province.AreaKm2)
	rng := seed.Rand(uint64(cfg.Seed), Stage, Version)
	return JitteredGrid(cyl, n, cfg.Mesh.Jitter, rng), nil
}

// JitteredGrid returns n sites on cyl in a jittered grid, drawing from rng.
//
// The grid has rows = round(√(n·H/W)) rows (at least 1, at most n) of
// height H/rows. Row r holds ⌊(r+1)·n/rows⌋ − ⌊r·n/rows⌋ sites, so the
// count is exactly n and rows differ by at most one site; a row of k sites
// divides the circumference into k boxes of width W/k. The boxes are then
// close to square, about √(W·H/n) on a side. The site of the box in column c
// of row r is
//
//	x = (c + 1/2 + jitter·(u − 1/2)) · W/k
//	y = (r + 1/2 + jitter·(v − 1/2)) · H/rows
//
// with u then v drawn from rng.Float64, box by box, rows north to south and
// columns west to east; that is also the order of the result. With jitter in
// [0, 1) every site lies strictly inside its own box, so x is in [0, W), y
// in (0, H), and no two sites coincide.
func JitteredGrid(cyl topo.Cylinder, n int, jitter float64, rng *rand.Rand) []topo.Point {
	if n < 1 {
		return nil
	}
	w, h := cyl.W(), cyl.H()
	rows := int(math.Round(math.Sqrt(float64(n) * h / w)))
	rows = min(max(rows, 1), n)
	dy := h / float64(rows)
	sites := make([]topo.Point, 0, n)
	for r := range rows {
		k := (r+1)*n/rows - r*n/rows
		dx := w / float64(k)
		for c := range k {
			// rng.Float64 inlines as a product (an exact scaling by
			// 2⁻⁵³); round it explicitly so it cannot fuse with the
			// subtraction below.
			u, v := float64(rng.Float64()), float64(rng.Float64())
			x := fmath.Mul(float64(c)+0.5+fmath.Mul(jitter, u-0.5), dx)
			y := fmath.Mul(float64(r)+0.5+fmath.Mul(jitter, v-0.5), dy)
			sites = append(sites, topo.Point{X: cyl.WrapX(x), Y: y})
		}
	}
	return sites
}
