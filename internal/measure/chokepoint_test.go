// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package measure

import (
	"reflect"
	"slices"
	"testing"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/world"
)

// fixture builds a hand-made graph from rows of a map on a square grid
// with 4-neighbors (no wrap): '.' is playable water, '#' land, 'M' a
// mountain, and 'm' a mountain whose altitude is low. The |grade| of an
// edge, in tenths of a percent, is the difference of the two cells'
// altitudes: 0 for water and land, 100 for 'M', 20 for 'm'. Cell ids run
// row by row.
func fixture(rows ...string) *graph {
	h, w := len(rows), len(rows[0])
	n := h * w
	g := &graph{nbr: make([][]int, n), grade: make([][]int, n), land: make([]bool, n), water: make([]bool, n), mountain: make([]bool, n)}
	alt := make([]int, n)
	for r, row := range rows {
		for c, ch := range row {
			i := r*w + c
			switch ch {
			case '.':
				g.water[i] = true
			case '#':
				g.land[i] = true
			case 'M':
				g.land[i], g.mountain[i], alt[i] = true, true, 100
			case 'm':
				g.land[i], g.mountain[i], alt[i] = true, true, 20
			}
		}
	}
	for r := range h {
		for c := range w {
			i := r*w + c
			for _, j := range []int{i - w, i - 1, i + 1, i + w} { // ascending
				switch {
				case j < 0 || j >= n:
					continue
				case (j == i-1 || j == i+1) && j/w != r:
					continue
				}
				g.nbr[i] = append(g.nbr[i], j)
				d := alt[i] - alt[j]
				g.grade[i] = append(g.grade[i], max(d, -d))
			}
		}
	}
	return g
}

// cell returns the id of row r, column c on a fixture w cells wide.
func cell(w, r, c int) int { return r*w + c }

func TestLandmasses(t *testing.T) {
	g := fixture(
		"#.##.###.####.#####",
		"...................",
		"#.#................",
	)
	lm := findLandmasses(g)
	if want := []int{1, 2, 3, 4, 5, 1, 1}; !slices.Equal(lm.size, want) {
		t.Fatalf("sizes %v, want %v", lm.size, want)
	}
	if want := []int{0, 2, 5, 9, 14, 38, 40}; !slices.Equal(lm.first, want) {
		t.Errorf("first cells %v, want %v", lm.first, want)
	}
	lc := config.Landmass{IsletMaxCells: 2, ContinentMinCells: 5}
	m := landmassMeasures(lm, 17, lc)
	if m.Count != 7 || m.Islets != 4 || m.Islands != 2 || m.Continents != 1 || m.LargestCells != 5 || m.LargestShare != 5.0/17 {
		t.Errorf("measures %+v", m)
	}
	if want := []int{3, 3, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0}; !slices.Equal(m.Sizes, want) || len(m.SizeBuckets) != len(m.Sizes) {
		t.Errorf("sizes %v (buckets %v), want %v", m.Sizes, m.SizeBuckets, want)
	}
	if want := (world.Landmass{Cells: 4, Class: world.ClassIsland, FirstCell: 9}); m.List[3] != want {
		t.Errorf("landmass 3 %+v, want %+v", m.List[3], want)
	}
	for size, want := range map[int]string{1: world.ClassIslet, 2: world.ClassIslet, 3: world.ClassIsland, 4: world.ClassIsland, 5: world.ClassContinent} {
		if got := landmassClass(size, lc); got != want {
			t.Errorf("class of %d = %s, want %s", size, got, want)
		}
	}
	if d := config.DefaultMeasures().Landmass; landmassClass(9, d) != world.ClassIslet || landmassClass(10, d) != world.ClassIsland ||
		landmassClass(999, d) != world.ClassIsland || landmassClass(1000, d) != world.ClassContinent {
		t.Error("default classes are not islet ≤ 9, island 10–999, continent ≥ 1000")
	}
}

// TestStraitsBetweenLandmasses: landmasses 0 and 1 are 2 water cells
// apart, a strait of width 2 at k = 3; landmasses 1 and 2 are 4 apart, no
// strait.
func TestStraitsBetweenLandmasses(t *testing.T) {
	const w = 20
	g := fixture(
		"....................",
		".####..####....####.",
		".####..####....####.",
		".####..####....####.",
		"....................",
	)
	lm := findLandmasses(g)
	got := straits(g, lm, 3, 20)
	want := []chokepoint{{a: 0, b: 1, width: 2, cells: []int{
		cell(w, 1, 5), cell(w, 1, 6), cell(w, 2, 5), cell(w, 2, 6), cell(w, 3, 5), cell(w, 3, 6)}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("straits %+v, want %+v", got, want)
	}
	// At k = 1 the 2-cell gap is too wide; at k = 4 the second gap is a
	// strait too.
	if got := straits(g, lm, 1, 20); len(got) != 0 {
		t.Errorf("k = 1: straits %+v", got)
	}
	got = straits(g, lm, 4, 20)
	if len(got) != 2 || got[1].a != 1 || got[1].b != 2 || got[1].width != 4 || !slices.Contains(got[1].cells, cell(w, 2, 13)) {
		t.Errorf("k = 4: straits %+v", got)
	}
}

// TestStraitsWithinLandmass: a C-shaped landmass whose arms are 2 water
// cells apart. Near the spine the arms are joined by a short land path;
// toward the tips the land path is longer than the detour, so the water
// there is a strait within the landmass.
func TestStraitsWithinLandmass(t *testing.T) {
	const w = 11
	g := fixture(
		"...........",
		".#########.",
		".#.........",
		".#.........",
		".#########.",
		"...........",
	)
	lm := findLandmasses(g)
	got := straits(g, lm, 3, 10)
	if len(got) != 1 {
		t.Fatalf("straits %+v, want one", got)
	}
	s := got[0]
	if s.a != 0 || s.b != 0 || s.width != 2 {
		t.Errorf("strait %+v, want width 2 within landmass 0", s)
	}
	// Tips: arms (1, 9) and (4, 9) are 2·8 + 3 = 19 land steps apart.
	for _, c := range []int{cell(w, 2, 9), cell(w, 3, 9)} {
		if !slices.Contains(s.cells, c) {
			t.Errorf("strait lacks cell %d: %v", c, s.cells)
		}
	}
	// Near the spine, (1, 2) and (4, 2) are 2 + 3 = 5 ≤ 10 steps apart.
	for _, c := range []int{cell(w, 2, 2), cell(w, 3, 2)} {
		if slices.Contains(s.cells, c) {
			t.Errorf("strait holds cell %d near the spine: %v", c, s.cells)
		}
	}
	for _, c := range s.cells {
		if !g.water[c] {
			t.Errorf("strait cell %d is not water", c)
		}
	}
	// With a detour longer than any land path, there is none.
	if got := straits(g, lm, 3, 30); len(got) != 0 {
		t.Errorf("detour 30: straits %+v", got)
	}
}

// TestNecks: four landmasses, each two 5×4 blocks joined by a corridor one
// cell long and 1, 2, 3 or 4 cells wide; and a fifth whose block is joined
// to a 2×2 block, a region smaller than the minimum. Necks are found for
// widths 1 to 3; the 4-wide corridor is no neck at k = 3 and the small
// region is rejected. On a 4-neighbor grid the block cells at either end
// of the 1-wide corridor are articulation points too, and touching cuts
// merge, so that neck is three cells long.
func TestNecks(t *testing.T) {
	const w = 11
	g := fixture(
		"...........",
		".####.####.", //  0: width 1
		".####.####.",
		".#########.",
		".####.####.",
		".####.####.",
		"...........",
		".####.####.", //  7: width 2
		".####.####.",
		".#########.",
		".#########.",
		".####.####.",
		"...........",
		".####.####.", // 13: width 3
		".#########.",
		".#########.",
		".#########.",
		".####.####.",
		"...........",
		".####.####.", // 19: width 4
		".#########.",
		".#########.",
		".#########.",
		".#########.",
		"...........",
		".####......", // 25: a small region
		".####......",
		".######....",
		".####.##...",
		".####......",
		"...........",
	)
	lm := findLandmasses(g)
	got := necks(g, lm, 3, 10)
	want := []chokepoint{
		{a: 0, b: 0, width: 1, cells: []int{cell(w, 3, 4), cell(w, 3, 5), cell(w, 3, 6)}},
		{a: 1, b: 1, width: 2, cells: []int{cell(w, 9, 5), cell(w, 10, 5)}},
		{a: 2, b: 2, width: 3, cells: []int{cell(w, 14, 5), cell(w, 15, 5), cell(w, 16, 5)}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("necks %+v,\nwant %+v", got, want)
	}
	// At k = 4 the 4-wide corridor is a neck too.
	if got := necks(g, lm, 4, 10); len(got) != 4 || got[3].width != 4 || got[3].a != 3 || len(got[3].cells) != 4 {
		t.Errorf("k = 4: necks %+v", got)
	}
	// With a minimum region of 4, the cut before the small region counts.
	got = necks(g, lm, 3, 4)
	small := slices.IndexFunc(got, func(c chokepoint) bool { return c.a == 4 })
	if small < 0 || got[small].width != 1 || !slices.Equal(got[small].cells, []int{cell(w, 27, 4)}) {
		t.Errorf("minimum region 4: necks %+v", got)
	}
}

// TestNeckCorridorMerges: a corridor one cell wide and several long is a
// run of articulation points (with the block cells at its ends) that
// merges into one neck.
func TestNeckCorridorMerges(t *testing.T) {
	const w = 16
	g := fixture(
		"................",
		".####......####.",
		".####......####.",
		".##############.",
		".####......####.",
		".####......####.",
		"................",
	)
	got := necks(g, findLandmasses(g), 3, 10)
	if len(got) != 1 || got[0].width != 1 || !slices.Equal(got[0].cells, []int{cell(w, 3, 4), cell(w, 3, 5), cell(w, 3, 6), cell(w, 3, 7), cell(w, 3, 8), cell(w, 3, 9), cell(w, 3, 10), cell(w, 3, 11)}) {
		t.Errorf("necks %+v", got)
	}
}

// TestPasses: a mountain wall splits a landmass from coast to coast; its
// cells are steep ('M', 10% from the lowland) except one low cell ('m',
// 2%), the only pass. A second, shorter wall leaves a way around of 6
// steps, within the detour of 10, so its low cell is no pass; with a
// detour of 5 it is one.
func TestPasses(t *testing.T) {
	const w = 11
	g := fixture(
		"...........",
		".####M####.",
		".####M####.",
		".####m####.",
		".####M####.",
		".####M####.",
		"...........",
		".#########.",
		".####M####.",
		".####m####.",
		".####M####.",
		".#########.",
		"...........",
	)
	lm := findLandmasses(g)
	chain, size := chains(g, 3)
	if !slices.Equal(size, []int{5, 3}) || chain[cell(w, 3, 5)] != 0 || chain[cell(w, 9, 5)] != 1 || chain[cell(w, 3, 4)] != -1 {
		t.Fatalf("chains %v", size)
	}
	got := passes(g, lm, chain, 3, 30, 10)
	want := []chokepoint{{a: 0, b: 0, width: 1, cells: []int{cell(w, 3, 5)}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("passes %+v, want %+v", got, want)
	}
	if got := passes(g, lm, chain, 3, 30, 5); len(got) != 2 || !slices.Equal(got[1].cells, []int{cell(w, 9, 5)}) || got[1].a != 1 {
		t.Errorf("detour 5: passes %+v", got)
	}
	// Grades at most 1% allow no route; at most 10% every wall cell is a
	// route, and adjacent ones group into one pass.
	if got := passes(g, lm, chain, 3, 10, 10); len(got) != 0 {
		t.Errorf("1%%: passes %+v", got)
	}
	if got := passes(g, lm, chain, 3, 100, 10); len(got) != 1 || len(got[0].cells) != 5 {
		t.Errorf("10%%: passes %+v", got)
	}
	// A minimum chain of 6 leaves no chain.
	if _, size := chains(g, 6); len(size) != 0 {
		t.Errorf("chains of 6: %v", size)
	}
}

// TestChokepointMeasures checks the counts, histograms and lists.
func TestChokepointMeasures(t *testing.T) {
	g := fixture(
		"....................",
		".####..####....####.",
		".####..####....####.",
		".####..####....####.",
		"....................",
	)
	c := config.DefaultMeasures()
	c.Landmass.IsletMaxCells = 0
	m, cm := chokepointMeasures(g, findLandmasses(g), c)
	if m.MaxCells != 3 || m.Straits != 1 || m.StraitsBetween != 1 || m.StraitsWithin != 0 || m.StraitsMajor != 1 || m.StraitCells != 6 ||
		m.Necks != 0 || m.Chains != 0 || m.Passes != 0 || !slices.Equal(m.StraitWidths, []int{0, 1, 0}) || !slices.Equal(m.NeckWidths, []int{0, 0, 0}) {
		t.Errorf("measures %+v", m)
	}
	if len(m.StraitList) != 1 || !slices.Equal(m.StraitList[0].Landmasses, []int{0, 1}) || m.StraitList[0].Width != 2 || len(m.StraitList[0].Cells) != 6 ||
		m.NeckList == nil || m.PassList == nil || len(cm.straits) != 1 {
		t.Errorf("lists %+v", m)
	}
	c.Landmass.IsletMaxCells = 12 // every landmass is an islet
	if m, _ := chokepointMeasures(g, findLandmasses(g), c); m.StraitsMajor != 0 {
		t.Errorf("major straits with islet shores: %d", m.StraitsMajor)
	}
}
