// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package measure

import (
	"reflect"
	"slices"
	"testing"

	"github.com/mdhender/mpg/internal/basin"
	"github.com/mdhender/mpg/internal/classify"
	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/mesh"
	"github.com/mdhender/mpg/world"
)

// TestCoastDistance checks the coast distance on a hand-made map: land
// touching water is 1 step away, land behind it 2, and land walled off by
// the rim ('R', neither land nor playable water) is unreached.
func TestCoastDistance(t *testing.T) {
	g := fixture(
		"......RRR",
		".####.R#R",
		".####.RRR",
		".####....",
		".........",
	)
	const w = 9
	d := coastDistance(g)
	want := map[int]int{cell(w, 2, 2): 2, cell(w, 2, 3): 2, cell(w, 1, 7): -1}
	for i := range g.cells() {
		switch k, ok := want[i]; {
		case ok:
			if d[i] != k {
				t.Errorf("cell %d: distance %d, want %d", i, d[i], k)
			}
		case g.land[i]:
			if d[i] != 1 {
				t.Errorf("land cell %d: distance %d, want 1", i, d[i])
			}
		default:
			if d[i] != 0 {
				t.Errorf("cell %d (not land): distance %d, want 0", i, d[i])
			}
		}
	}
}

// TestUsability checks the habitable land, the wetlands and the coast
// distance on the hand-made map of TestCoastDistance: a desert cell, a
// mountain cell and a glacier cell are not habitable; a wetland is.
func TestUsability(t *testing.T) {
	g := fixture(
		"......RRR",
		".####.R#R",
		".####.RRR",
		".####....",
		".........",
	)
	const w = 9
	n := g.cells()
	cl := &classify.Result{Landform: make([]classify.Landform, n), Biome: make([]classify.Biome, n), Surface: make([]classify.Surface, n)}
	for i := range n {
		if g.land[i] {
			cl.Landform[i], cl.Biome[i] = classify.Plains, classify.Grassland
		}
	}
	cl.Biome[cell(w, 1, 1)] = classify.Desert
	cl.Landform[cell(w, 1, 2)] = classify.Mountains
	cl.Biome[cell(w, 2, 2)], cl.Surface[cell(w, 2, 2)] = classify.Clear, classify.Glacier
	cl.Landform[cell(w, 2, 3)], cl.Surface[cell(w, 2, 3)] = classify.Flats, classify.Marshes
	cfg := config.Default()
	cfg.Measures.Usability.CoastCells = 1
	u := usability(Input{Config: cfg, Land: g.land, Classes: cl}, g)
	if u.HabitableCells != 10 || u.HabitableShare != 10.0/13 || u.WetlandCells != 1 || u.WetlandShare != 1.0/13 ||
		u.DesertCells != 1 || u.MountainCells != 1 {
		t.Errorf("habitable and wetlands %+v", u)
	}
	if u.CoastCells != 1 || u.CoastWithinCells != 10 || u.CoastWithinShare != 10.0/13 || !slices.Equal(u.CoastDistance, []int{0, 10, 2}) ||
		u.CoastDistanceMax != 2 || u.CoastDistanceMean != 14.0/12 || u.CoastUnreachedCells != 1 {
		t.Errorf("coast distance %+v", u)
	}
	if len(u.BiomeNames) != len(u.Biomes) || len(u.LandformNames) != len(u.Landforms) || u.BiomeNames[0] != "clear" || u.LandformNames[0] != "flats" {
		t.Errorf("histograms %q %v %q %v", u.BiomeNames, u.Biomes, u.LandformNames, u.Landforms)
	}
	for _, tc := range []struct {
		b    classify.Biome
		l    classify.Landform
		want bool
	}{
		{classify.Grassland, classify.Plains, true}, {classify.Tundra, classify.Hills, true}, {classify.Desert, classify.Flats, false},
		{classify.PolarDesert, classify.Plains, false}, {classify.Clear, classify.Plains, false}, {classify.BorealForest, classify.Mountains, false},
	} {
		if got := habitable(tc.b, tc.l); got != tc.want {
			t.Errorf("habitable(%v, %v) = %v", tc.b, tc.l, got)
		}
	}
}

func TestDepthBucket(t *testing.T) {
	for _, tc := range []struct {
		d    float64
		want int
	}{{0, 0}, {9.99, 0}, {10, 1}, {49.9, 2}, {50, 3}, {199, 4}, {200, 5}, {999, 6}, {1000, 7}, {5000, 7}} {
		if got := depthBucket(tc.d); got != tc.want {
			t.Errorf("depthBucket(%v) = %d, want %d", tc.d, got, tc.want)
		}
	}
	if len(depthFloors)+1 != len(world.DepthBuckets) {
		t.Error("depth floors do not match the bucket names")
	}
}

// TestFeatures checks the depth histogram, the depressions below the
// minimum, the basin states and the dry basins on a hand-made hierarchy:
//
//   - basins 0 (dry) and 1 (full) nest in basin 2 (dry, but holding 1's
//     lake): basin 0 is a dry basin, 2 is not;
//   - basin 3 (dry) nests in basin 4 (dry): 4 is the dry basin, 3 not;
//   - basin 5 holds a partial lake.
func TestFeatures(t *testing.T) {
	none := basin.None
	r := &basin.Result{
		MinDepthM: 50,
		Depressions: []basin.Basin{
			{DepthM: 5}, {DepthM: 30}, {DepthM: 50}, {DepthM: 60}, {DepthM: 70}, {DepthM: 120}, {DepthM: 300}, {DepthM: 1200},
		},
		Basins: []basin.Basin{
			{Parent: 2, Size: 3, DepthM: 50, Bottom: 10},
			{Parent: 2, Size: 4, DepthM: 60, Bottom: 11},
			{Parent: none, Children: []int{0, 1}, Size: 9, DepthM: 70, Bottom: 10},
			{Parent: 4, Size: 2, DepthM: 120, Bottom: 20},
			{Parent: none, Children: []int{3}, Size: 6, DepthM: 1200, Bottom: 20},
			{Parent: none, Size: 5, DepthM: 300, Bottom: 30},
		},
	}
	lk := &basin.Lakes{
		Water:  []basin.Water{{State: basin.Dry}, {State: basin.Full}, {State: basin.Dry}, {State: basin.Dry}, {State: basin.Dry}, {State: basin.Partial}},
		Playas: []basin.Playa{{Basin: 0, Cell: 10}, {Basin: 3, Cell: 20}},
	}
	wantDry := []world.DryBasin{{Basin: 0, Cells: 3, DepthM: 50, Bottom: 10}, {Basin: 4, Cells: 6, DepthM: 1200, Bottom: 20}}
	if got := dryBasins(r, lk.Water); !slices.Equal(got, wantDry) {
		t.Errorf("dryBasins = %+v, want %+v", got, wantDry)
	}

	const n = 4
	m := &mesh.Mesh{Cells: make([]mesh.Cell, n)}
	m.Cells[3].Rim = true
	cl := &classify.Result{
		Landform:    []classify.Landform{classify.Mountains, classify.VolcanicHighlands, classify.Plains, classify.SaltWater},
		Biome:       []classify.Biome{classify.Clear, classify.PolarDesert, classify.Clear, classify.BiomeNone},
		Surface:     []classify.Surface{classify.Glacier, classify.SurfaceNone, classify.IceField, classify.PackIce},
		VolcanoCell: []int{1, 3, 2},
		VolcanoLand: []bool{true, false, true},
	}
	f := features(Input{Mesh: m, Basins: r, Lakes: lk, Classes: cl})
	want := world.FeatureMeasures{
		MinDepthM: 50, Depressions: 8, DepressionsBelowMin: 2, Basins: 6, BasinsFull: 1, BasinsPartial: 1, BasinsDry: 4, DeepestBasinM: 1200,
		Playas: 2, DryBasins: 2, DryBasinAreaCells: 9, LargestDryBasinCells: 6, DeepestDryBasinM: 1200,
		GlacierCells: 1, IceFieldCells: 1, PolarDesertCells: 1, PackIceCells: 1, Hotspots: 3, Volcanoes: 2, VolcanicHighlandCells: 1,
	}
	if !slices.Equal(f.Depths, []int{1, 0, 1, 3, 1, 1, 0, 1}) || !slices.Equal(f.DepthBuckets, world.DepthBuckets) || !slices.Equal(f.DryBasinList, wantDry) {
		t.Errorf("depths %v %q, dry basins %+v", f.Depths, f.DepthBuckets, f.DryBasinList)
	}
	f.Depths, f.DepthBuckets, f.DryBasinList = nil, nil, nil
	if !reflect.DeepEqual(f, want) {
		t.Errorf("features %+v\nwant %+v", f, want)
	}
	// No dry basin is an empty list, not null.
	if got := dryBasins(&basin.Result{}, nil); got == nil || len(got) != 0 {
		t.Errorf("dryBasins of none = %#v", got)
	}
}

func TestWaterSizes(t *testing.T) {
	var w world.WaterMeasures
	waterSizes(&w, []basin.Lake{
		{Cells: make([]int, 1)}, {Cells: make([]int, 4)}, {Cells: make([]int, 19)},
		{Cells: make([]int, 20), Kind: basin.KindInlandSea}, {Cells: make([]int, 801), Kind: basin.KindInlandSea},
	})
	if !slices.Equal(w.SizeBuckets, world.LandmassSizeBuckets) || !slices.Equal(w.LakeSizes, []int{1, 1, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0}) ||
		!slices.Equal(w.InlandSeaSizes, []int{0, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0}) {
		t.Errorf("water sizes %+v", w)
	}
}
