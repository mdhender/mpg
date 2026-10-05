// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package measure

import (
	"errors"
	"math"
	"slices"
	"strconv"

	"github.com/mdhender/mpg/internal/basin"
	"github.com/mdhender/mpg/internal/cells"
	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/edges"
	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/internal/mesh"
	"github.com/mdhender/mpg/world"
)

// SummaryFile is the name of the text summary in a world's output
// directory, beside world.MeasuresFile.
const SummaryFile = "measures.txt"

// Input is what the measures read: the resolved config and its hash, and
// the stage products they describe.
type Input struct {
	Config     config.Config
	ConfigHash string
	// Mesh is the province mesh.
	Mesh *mesh.Mesh
	// Search is the land-target search's record, Land marks the land
	// cells after lakes, LandAreaKm2 is their area, OceanCells counts the
	// ocean cells, and Lakes is the water balance at the chosen level.
	Search      *cells.Record
	Land        []bool
	LandAreaKm2 float64
	OceanCells  int
	Lakes       *basin.Lakes
	// Edges is the edge data and EdgeStats its statistics (edges.Summarize).
	Edges     *edges.Data
	EdgeStats *edges.Stats
}

// Compute measures the world described by in and runs the configured checks
// (in.Config.Measures.Checks) against the measures.
func Compute(in Input) (*world.Measures, error) {
	if in.Mesh == nil || in.Search == nil || in.Lakes == nil || in.Edges == nil || in.EdgeStats == nil {
		return nil, errors.New("measure: missing stage products")
	}
	if len(in.Land) != len(in.Mesh.Cells) {
		return nil, errors.New("measure: land mask does not match the mesh")
	}
	m := &world.Measures{
		Schema:     world.MeasuresSchemaVersion,
		Generator:  "mpg",
		Seed:       strconv.FormatUint(uint64(in.Config.Seed), 10),
		ConfigHash: in.ConfigHash,
	}
	m.Land = land(in)
	m.Mesh = meshMeasures(in)
	m.Directions = directions(in.EdgeStats)
	m.Grades = grades(in)
	m.Water = water(in)
	if err := Evaluate(m, in.Config.Measures.Checks); err != nil {
		return nil, err
	}
	return m, nil
}

// land measures the land contract.
func land(in Input) world.LandMeasures {
	r := in.Search
	n := 0
	for _, l := range in.Land {
		if l {
			n++
		}
	}
	a := in.Config.Province.AreaKm2
	return world.LandMeasures{
		TargetCells:      r.Target,
		Cells:            n,
		ToleranceCells:   r.Tolerance,
		DeviationCells:   n - r.Target,
		DeviationPercent: ratio(float64(100*(n-r.Target)), float64(r.Target)),
		Met:              r.Met,
		AreaKm2:          in.LandAreaKm2,
		AreaPerTarget:    ratio(in.LandAreaKm2, fmath.Mul(float64(r.Target), a)),
	}
}

// meshMeasures measures the mesh: cell areas, edge lengths, neighbor
// counts, and the short-edge collapse.
func meshMeasures(in Input) world.MeshMeasures {
	m, cfg := in.Mesh, in.Config
	a := cfg.Province.AreaKm2
	st := m.Stats(a, cfg.Mesh.MinEdgeKm)
	out := world.MeshMeasures{
		Cells:             len(m.Cells),
		RimCells:          st.RimCells,
		PlayableCells:     len(m.Cells) - st.RimCells,
		CellAreaMeanA:     st.AreaMean / a,
		CellAreaCV:        st.AreaCV,
		MinEdgeKm:         cfg.Mesh.MinEdgeKm,
		NeighborsPlayable: make([]int, edges.MaxDegree+1),
		NeighborsLand:     make([]int, edges.MaxDegree+1),
		DegreeCapHits:     m.DegreeCapHits,
		Collapses:         m.Collapses,
		Stretches:         m.Stretches,
		MaxShiftKm:        m.MaxShiftKm,
	}
	var landAreas []float64
	for i, c := range m.Cells {
		if c.Rim {
			continue
		}
		k := min(len(c.Neighbors), edges.MaxDegree)
		out.NeighborsPlayable[k]++
		if in.Land[i] {
			out.NeighborsLand[k]++
			landAreas = append(landAreas, m.Area(i)/a)
		}
	}
	if len(landAreas) > 0 {
		out.LandCellAreaMeanA, out.LandCellAreaCV = meanCV(landAreas)
		out.LandCellAreaMinA, out.LandCellAreaMaxA = slices.Min(landAreas), slices.Max(landAreas)
	}
	var lengths []float64
	for e, me := range m.Edges {
		if me.OnBoundary() || m.Cells[me.Cells[0]].Rim || m.Cells[me.Cells[1]].Rim {
			continue
		}
		lengths = append(lengths, m.EdgeLength(e))
	}
	if len(lengths) > 0 {
		slices.Sort(lengths)
		out.EdgeMinKm = lengths[0]
		out.EdgeP5Km = nearestRank(lengths, 5)
	}
	return out
}

// directions copies the compass direction statistics.
func directions(s *edges.Stats) world.DirectionMeasures {
	return world.DirectionMeasures{
		HalfEdges:        s.HalfEdges,
		ErrorMeanDeg:     s.ErrorMean,
		ErrorP95Deg:      s.ErrorP95,
		ErrorMaxDeg:      s.ErrorMax,
		Pairs:            s.Pairs,
		NotOpposite:      s.NotOpposite,
		NotOppositeShare: ratio(float64(s.NotOpposite), float64(s.Pairs)),
		NaiveCollisions:  s.NaiveCollisions,
	}
}

// grades measures the inclines: the histograms of edges.Stats, and the
// land–land edges' steepest, 95th-percentile and capped grades.
func grades(in Input) world.GradeMeasures {
	s := in.EdgeStats
	out := world.GradeMeasures{
		Buckets:  slices.Clone(edges.GradeBucketNames),
		LandLand: slices.Clone(s.GradeLand[:]),
		Playable: slices.Clone(s.GradeAll[:]),
	}
	var abs []int
	for e, me := range in.Mesh.Edges {
		if me.OnBoundary() || !in.Land[me.Cells[0]] || !in.Land[me.Cells[1]] {
			continue
		}
		g := in.Edges.Edges[e].Incline.Abs()
		abs = append(abs, int(g))
		if g >= edges.MaxIncline {
			out.LandCapEdges++
		}
	}
	out.LandEdges = len(abs)
	if len(abs) > 0 {
		slices.Sort(abs)
		out.LandMaxPercent = float64(abs[len(abs)-1]) / 10
		out.LandP95Percent = float64(nearestRank(abs, 95)) / 10
	}
	return out
}

// water measures the water bodies and the coasts.
func water(in Input) world.WaterMeasures {
	s := in.EdgeStats
	out := world.WaterMeasures{
		OceanCells:            in.OceanCells,
		CoastEdges:            s.Coast,
		CoastEdgesPerLandCell: s.CoastPerLand(),
		LandRimEdges:          s.LandRim,
	}
	for _, l := range in.Lakes.Lakes {
		n := len(l.Cells)
		if l.Kind == basin.KindInlandSea {
			out.InlandSeas++
			out.InlandSeaCells += n
			out.LargestInlandSeaCells = max(out.LargestInlandSeaCells, n)
		} else {
			out.Lakes++
			out.LakeCells += n
			out.LargestLakeCells = max(out.LargestLakeCells, n)
		}
	}
	return out
}

// nearestRank returns the p-th percentile of the sorted, non-empty xs by
// nearest rank: the element at ⌈p·n/100⌉, counting from 1.
func nearestRank[T int | float64](xs []T, p int) T {
	return xs[max((p*len(xs)+99)/100-1, 0)]
}

// meanCV returns the mean and the coefficient of variation (population
// standard deviation over the mean) of the non-empty xs, summing in order
// with every product rounded (package fmath). The CV is 0 when the mean is.
func meanCV(xs []float64) (mean, cv float64) {
	var sum float64
	for _, x := range xs {
		sum += x
	}
	mean = sum / float64(len(xs))
	var ss float64
	for _, x := range xs {
		d := x - mean
		ss += fmath.Mul(d, d)
	}
	return mean, ratio(math.Sqrt(ss/float64(len(xs))), mean)
}

// ratio returns a/b, or 0 when b is 0, so no measure is NaN or infinite.
func ratio(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}
