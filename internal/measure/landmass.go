// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package measure

import (
	"slices"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/world"
)

// landmasses is the partition of the land cells into landmasses: the sets
// connected through shared edges (cells that meet only at a point are not
// neighbors). Landmass ids rank them by lowest cell id.
type landmasses struct {
	// id is each cell's landmass, −1 for a cell that is not land.
	id []int
	// size is each landmass's cell count, and first its lowest cell id.
	size, first []int
}

// findLandmasses labels g's landmasses, visiting cells in id order.
func findLandmasses(g *graph) landmasses {
	n := g.cells()
	lm := landmasses{id: make([]int, n)}
	for i := range lm.id {
		lm.id[i] = -1
	}
	var q []int
	for i := range n {
		if !g.land[i] || lm.id[i] >= 0 {
			continue
		}
		id := len(lm.size)
		lm.id[i] = id
		q = append(q[:0], i)
		for k := 0; k < len(q); k++ {
			for _, j := range g.nbr[q[k]] {
				if g.land[j] && lm.id[j] < 0 {
					lm.id[j] = id
					q = append(q, j)
				}
			}
		}
		lm.size = append(lm.size, len(q))
		lm.first = append(lm.first, i)
	}
	return lm
}

// landmassClass returns the class of a landmass of size cells.
func landmassClass(size int, c config.Landmass) string {
	switch {
	case size <= c.IsletMaxCells:
		return world.ClassIslet
	case size >= c.ContinentMinCells:
		return world.ClassContinent
	}
	return world.ClassIsland
}

// landmassSizeFloors are the smallest sizes of world.LandmassSizeBuckets.
var landmassSizeFloors = []int{1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000}

// sizeBucket returns the bucket of world.LandmassSizeBuckets holding a size
// of s ≥ 1 cells.
func sizeBucket(s int) int {
	for b := len(landmassSizeFloors) - 1; b > 0; b-- {
		if s >= landmassSizeFloors[b] {
			return b
		}
	}
	return 0
}

// landmassMeasures measures the landmasses lm of a world with landCells
// land cells.
func landmassMeasures(lm landmasses, landCells int, c config.Landmass) world.LandmassMeasures {
	out := world.LandmassMeasures{
		Count:             len(lm.size),
		IsletMaxCells:     c.IsletMaxCells,
		ContinentMinCells: c.ContinentMinCells,
		SizeBuckets:       slices.Clone(world.LandmassSizeBuckets),
		Sizes:             make([]int, len(landmassSizeFloors)),
		List:              make([]world.Landmass, len(lm.size)),
	}
	for id, s := range lm.size {
		class := landmassClass(s, c)
		switch class {
		case world.ClassContinent:
			out.Continents++
		case world.ClassIsland:
			out.Islands++
		default:
			out.Islets++
		}
		out.LargestCells = max(out.LargestCells, s)
		out.Sizes[sizeBucket(s)]++
		out.List[id] = world.Landmass{Cells: s, Class: class, FirstCell: lm.first[id]}
	}
	out.LargestShare = ratio(float64(out.LargestCells), float64(landCells))
	return out
}
