// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package measure

import (
	"fmt"
	"strings"

	"github.com/mdhender/mpg/world"
)

// Lines returns the text summary of m: one line per group, one per check,
// and the verdict. It is the content of SummaryFile and the measures
// stage's log.
func Lines(m *world.Measures) []string {
	l, me, d, g, w, lm, cp := m.Land, m.Mesh, m.Directions, m.Grades, m.Water, m.Landmasses, m.Chokepoints
	f, r, u := m.Features, m.Rivers, m.Usability
	met := "met"
	if !l.Met {
		met = "UNMET"
	}
	out := []string{
		fmt.Sprintf("land     %d / %d cells (%+d, %+.2f%%, %s, tolerance %d); area %.0f km² (%.4f N·A)",
			l.Cells, l.TargetCells, l.DeviationCells, l.DeviationPercent, met, l.ToleranceCells, l.AreaKm2, l.AreaPerTarget),
		fmt.Sprintf("mesh     %d cells (%d playable, %d rim); area CV %.4f, land CV %.4f (%.3f to %.3f A)",
			me.Cells, me.PlayableCells, me.RimCells, me.CellAreaCV, me.LandCellAreaCV, me.LandCellAreaMinA, me.LandCellAreaMaxA),
		fmt.Sprintf("edges    min %.3f km (min_edge_km %.3f), p5 %.3f km; %d collapsed, %d stretched, %d degree-cap hits",
			me.EdgeMinKm, me.MinEdgeKm, me.EdgeP5Km, me.Collapses, me.Stretches, me.DegreeCapHits),
		"nbrs     playable " + histogram(me.NeighborsPlayable) + "; land " + histogram(me.NeighborsLand),
		fmt.Sprintf("compass  error mean %.2f°, p95 %.2f°, max %.2f° over %d half-edges; reverse not opposite %d of %d (%.2f%%)",
			d.ErrorMeanDeg, d.ErrorP95Deg, d.ErrorMaxDeg, d.HalfEdges, d.NotOpposite, d.Pairs, 100*d.NotOppositeShare),
		"grade    land-land " + shares(g.Buckets, g.LandLand) +
			fmt.Sprintf("; max %.1f%%, p95 %.1f%%, %d at the cap", g.LandMaxPercent, g.LandP95Percent, g.LandCapEdges),
		fmt.Sprintf("water    %d ocean cells; %d lakes (%d cells, largest %d), %d inland seas (%d cells, largest %d); coast %.3f edges per land cell; %d land-rim edges",
			w.OceanCells, w.Lakes, w.LakeCells, w.LargestLakeCells, w.InlandSeas, w.InlandSeaCells, w.LargestInlandSeaCells, w.CoastEdgesPerLandCell, w.LandRimEdges) +
			"; lake sizes " + named(w.SizeBuckets, w.LakeSizes) + "; inland-sea sizes " + named(w.SizeBuckets, w.InlandSeaSizes),
		fmt.Sprintf("landmass %d: %d continents, %d islands, %d islets; largest %d cells (%.1f%% of land)",
			lm.Count, lm.Continents, lm.Islands, lm.Islets, lm.LargestCells, 100*lm.LargestShare),
		fmt.Sprintf("choke    k %d: %d straits (%d between, %d within, %d major; %d cells), %d necks (%d cells); %d chains (%d cells), %d passes (%d cells) on %d chains",
			cp.MaxCells, cp.Straits, cp.StraitsBetween, cp.StraitsWithin, cp.StraitsMajor, cp.StraitCells, cp.Necks, cp.NeckCells, cp.Chains, cp.ChainCells, cp.Passes, cp.PassCells, cp.ChainsWithPass),
		fmt.Sprintf("feature  %d depressions (%d below %.0f m), %d basins (%d full, %d partial, %d dry; deepest %.0f m); depths m %s; %d dry basins (%d cells, largest %d, deepest %.0f m), %d playas; %d glacier, %d ice-field, %d polar-desert, %d pack-ice cells; %d of %d hotspots on land, %d volcanic-highland cells",
			f.Depressions, f.DepressionsBelowMin, f.MinDepthM, f.Basins, f.BasinsFull, f.BasinsPartial, f.BasinsDry, f.DeepestBasinM, named(f.DepthBuckets, f.Depths),
			f.DryBasins, f.DryBasinAreaCells, f.LargestDryBasinCells, f.DeepestDryBasinM, f.Playas, f.GlacierCells, f.IceFieldCells, f.PolarDesertCells, f.PackIceCells,
			f.Volcanoes, f.Hotspots, f.VolcanicHighlandCells),
		fmt.Sprintf("rivers   %d edges (%d stream, %d river, %d major), %.3f per land cell, %.1f km per 1000 km² of land, %.1f%% of land cells touching; %d polylines, %d mouths (ends: %d ocean, %d lake, %d sink, %d confluence); longest %d edges (%.0f km), flow %d edges (%.0f km)",
			r.RiverEdges, r.StreamEdges, r.RiverClassEdges, r.MajorRiverEdges, r.EdgesPerLandCell, r.KmPer1000Km2, 100*r.TouchShare, r.Polylines, r.Mouths,
			r.EndsOcean, r.EndsLake, r.EndsSink, r.EndsConfluence, r.LongestEdges, r.LongestKm, r.LongestFlowEdges, r.LongestFlowKm),
		fmt.Sprintf("usable   habitable %d (%.1f%%), wetland %d (%.1f%%), desert %d, mountains %d; within %d cells of the coast %d (%.1f%%); coast distance max %d, mean %.2f, %d unreached",
			u.HabitableCells, 100*u.HabitableShare, u.WetlandCells, 100*u.WetlandShare, u.DesertCells, u.MountainCells,
			u.CoastCells, u.CoastWithinCells, 100*u.CoastWithinShare, u.CoastDistanceMax, u.CoastDistanceMean, u.CoastUnreachedCells),
	}
	for _, c := range m.Checks {
		out = append(out, "check    "+CheckLine(c))
	}
	return append(out, "checks   "+Verdict(m))
}

// Summary returns Lines joined by newlines, with a trailing newline: the
// content of SummaryFile.
func Summary(m *world.Measures) []byte {
	return []byte(strings.Join(Lines(m), "\n") + "\n")
}

// histogram formats counts as "k:count" pairs, skipping zeros.
func histogram(counts []int) string {
	var parts []string
	for k, n := range counts {
		if n != 0 {
			parts = append(parts, fmt.Sprintf("%d:%d", k, n))
		}
	}
	return strings.Join(parts, " ")
}

// named formats counts against bucket names as "name:count" pairs,
// skipping zeros, or "none" when every count is 0.
func named(names []string, counts []int) string {
	var parts []string
	for k, n := range counts {
		if n != 0 && k < len(names) {
			parts = append(parts, fmt.Sprintf("%s:%d", names[k], n))
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, " ")
}

// shares formats counts against bucket names as percentages of their sum.
func shares(names []string, counts []int) string {
	total := 0
	for _, n := range counts {
		total += n
	}
	parts := make([]string, len(counts))
	for k, n := range counts {
		parts[k] = fmt.Sprintf("%s %.1f%%", names[k], 100*ratio(float64(n), float64(total)))
	}
	return strings.Join(parts, ", ")
}
