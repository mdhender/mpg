// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package measure

import (
	"slices"

	"github.com/mdhender/mpg/internal/basin"
	"github.com/mdhender/mpg/internal/classify"
	"github.com/mdhender/mpg/internal/edges"
	"github.com/mdhender/mpg/internal/river"
	"github.com/mdhender/mpg/world"
)

// depthFloors are the smallest depths, in meters, of world.DepthBuckets
// after the first.
var depthFloors = []float64{10, 25, 50, 100, 200, 500, 1000}

// depthBucket returns the bucket of world.DepthBuckets holding a depth of
// d meters.
func depthBucket(d float64) int {
	for b := len(depthFloors) - 1; b >= 0; b-- {
		if d >= depthFloors[b] {
			return b + 1
		}
	}
	return 0
}

// features measures the depressions and basins, the dry basins and playas,
// permanent ice, and volcanoes.
func features(in Input) world.FeatureMeasures {
	r, lk, cl := in.Basins, in.Lakes, in.Classes
	out := world.FeatureMeasures{
		MinDepthM:    r.MinDepthM,
		Depressions:  len(r.Depressions),
		Basins:       len(r.Basins),
		DepthBuckets: slices.Clone(world.DepthBuckets),
		Depths:       make([]int, len(world.DepthBuckets)),
		Playas:       len(lk.Playas),
		DryBasinList: dryBasins(r, lk.Water),
	}
	for _, d := range r.Depressions {
		out.Depths[depthBucket(d.DepthM)]++
		if d.DepthM < r.MinDepthM {
			out.DepressionsBelowMin++
		}
	}
	for b, d := range r.Basins {
		out.DeepestBasinM = max(out.DeepestBasinM, d.DepthM)
		switch lk.Water[b].State {
		case basin.Full:
			out.BasinsFull++
		case basin.Partial:
			out.BasinsPartial++
		default:
			out.BasinsDry++
		}
	}
	out.DryBasins = len(out.DryBasinList)
	for _, d := range out.DryBasinList {
		out.DryBasinAreaCells += d.Cells
		out.LargestDryBasinCells = max(out.LargestDryBasinCells, d.Cells)
		out.DeepestDryBasinM = max(out.DeepestDryBasinM, d.DepthM)
	}
	biomes, _ := cl.BiomeHistogram()
	out.GlacierCells = cl.SurfaceCount(classify.Glacier)
	out.IceFieldCells = cl.SurfaceCount(classify.IceField)
	out.PolarDesertCells = biomes[classify.PolarDesert-classify.Clear]
	out.PackIceCells = cl.SurfaceCount(classify.PackIce)
	out.Hotspots = len(cl.VolcanoCell)
	out.Volcanoes = cl.Volcanoes()
	out.VolcanicHighlandCells = cl.Count(in.Mesh, classify.VolcanicHighlands, true)
	return out
}

// dryBasins lists, by basin id, the dry basins of r with water states w:
// the basins with no water in them or in any basin nested in them, whose
// parent, if any, holds water. Basins are numbered children first, so one
// pass in id order sees every child before its parent.
func dryBasins(r *basin.Result, w []basin.Water) []world.DryBasin {
	wet := make([]bool, len(r.Basins))
	for b, x := range r.Basins {
		wet[b] = w[b].State != basin.Dry
		for _, c := range x.Children {
			wet[b] = wet[b] || wet[c]
		}
	}
	out := []world.DryBasin{}
	for b, x := range r.Basins {
		if wet[b] || x.Parent != basin.None && !wet[x.Parent] {
			continue
		}
		out = append(out, world.DryBasin{Basin: b, Cells: x.Size, DepthM: x.DepthM, Bottom: x.Bottom})
	}
	return out
}

// riverMeasures restates the river network's statistics.
func riverMeasures(s *river.NetworkStats) world.RiverMeasures {
	return world.RiverMeasures{
		RiverEdges:       s.RiverEdges,
		StreamEdges:      s.Edges[edges.Stream],
		RiverClassEdges:  s.Edges[edges.River],
		MajorRiverEdges:  s.Edges[edges.MajorRiver],
		EdgesPerLandCell: s.EdgesPerLandCell(),
		Km:               s.RiverKm,
		KmPer1000Km2:     s.KmPer1000Km2(),
		TouchCells:       s.TouchCells,
		TouchShare:       s.TouchShare(),
		Polylines:        s.Paths,
		Mouths:           s.Mouths,
		EndsOcean:        s.Ends[river.Ocean],
		EndsLake:         s.Ends[river.Lake],
		EndsSink:         s.Ends[river.Sink],
		EndsConfluence:   s.Ends[river.Interior],
		Confluences:      s.Confluences,
		OutletSources:    s.OutletSources,
		LongestEdges:     s.LongestEdges,
		LongestKm:        s.LongestKm,
		LongestFlowEdges: s.FlowEdges,
		LongestFlowKm:    s.FlowKm,
		SeamEdges:        s.Seam,
		MaxDrainageKm2:   s.MaxDrainageKm2,
		MaxDischargeM3s:  s.MaxDischargeM3s,
	}
}

// habitable reports whether a land cell of biome b and landform l is
// habitable: not under permanent ice (biome clear), not polar desert or
// desert, and not mountains.
func habitable(b classify.Biome, l classify.Landform) bool {
	switch {
	case b == classify.Clear, b == classify.PolarDesert, b == classify.Desert:
		return false
	}
	return l != classify.Mountains
}

// usability measures the habitable land, the wetlands, the coast distance,
// and the land's biomes and landforms.
func usability(in Input, g *graph) world.UsabilityMeasures {
	cl := in.Classes
	d := in.Config.Measures.Usability.CoastCells
	out := world.UsabilityMeasures{
		WetlandCells: cl.WetlandCount(),
		CoastCells:   d,
	}
	land := 0
	for i, l := range in.Land {
		if !l {
			continue
		}
		land++
		if habitable(cl.Biome[i], cl.Landform[i]) {
			out.HabitableCells++
		}
	}
	out.HabitableShare = ratio(float64(out.HabitableCells), float64(land))
	out.WetlandShare = ratio(float64(out.WetlandCells), float64(land))
	biomes, _ := cl.BiomeHistogram()
	out.DesertCells = biomes[classify.Desert-classify.Clear]
	out.Biomes = biomes
	for _, b := range classify.Biomes {
		out.BiomeNames = append(out.BiomeNames, b.String())
	}
	out.Landforms, _ = cl.LandHistogram()
	for _, l := range classify.LandLandforms {
		out.LandformNames = append(out.LandformNames, l.String())
	}
	out.MountainCells = out.Landforms[classify.Mountains-classify.Flats]

	out.CoastDistance = []int{0}
	reached, sum := 0, 0
	for i, k := range coastDistance(g) {
		switch {
		case !g.land[i]:
			continue
		case k < 0:
			out.CoastUnreachedCells++
			continue
		}
		for len(out.CoastDistance) <= k {
			out.CoastDistance = append(out.CoastDistance, 0)
		}
		out.CoastDistance[k]++
		out.CoastDistanceMax = max(out.CoastDistanceMax, k)
		if k <= d {
			out.CoastWithinCells++
		}
		reached++
		sum += k
	}
	out.CoastWithinShare = ratio(float64(out.CoastWithinCells), float64(land))
	out.CoastDistanceMean = ratio(float64(sum), float64(reached))
	return out
}

// coastDistance returns each cell's coast distance in g: for a land cell,
// the fewest steps through land to playable water (1 for a land cell
// touching water), or −1 when no land path reaches water; 0 for the other
// cells. It is a breadth-first search from every coastal land cell at
// once, in id order.
func coastDistance(g *graph) []int {
	n := g.cells()
	dist := make([]int, n)
	var q []int
	for i := range n {
		if !g.land[i] {
			continue
		}
		dist[i] = -1
		if g.coastal(i) {
			dist[i] = 1
			q = append(q, i)
		}
	}
	for k := 0; k < len(q); k++ {
		x := q[k]
		for _, j := range g.nbr[x] {
			if g.land[j] && dist[j] < 0 {
				dist[j] = dist[x] + 1
				q = append(q, j)
			}
		}
	}
	return dist
}

// waterSizes fills the lake and inland-sea size histograms of w from the
// water balance's lakes.
func waterSizes(w *world.WaterMeasures, lakes []basin.Lake) {
	w.SizeBuckets = slices.Clone(world.LandmassSizeBuckets)
	w.LakeSizes = make([]int, len(landmassSizeFloors))
	w.InlandSeaSizes = make([]int, len(landmassSizeFloors))
	for _, l := range lakes {
		if n := len(l.Cells); l.Kind == basin.KindInlandSea {
			w.InlandSeaSizes[sizeBucket(n)]++
		} else {
			w.LakeSizes[sizeBucket(n)]++
		}
	}
}
