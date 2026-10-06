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
			w.OceanCells, w.Lakes, w.LakeCells, w.LargestLakeCells, w.InlandSeas, w.InlandSeaCells, w.LargestInlandSeaCells, w.CoastEdgesPerLandCell, w.LandRimEdges),
		fmt.Sprintf("landmass %d: %d continents, %d islands, %d islets; largest %d cells (%.1f%% of land)",
			lm.Count, lm.Continents, lm.Islands, lm.Islets, lm.LargestCells, 100*lm.LargestShare),
		fmt.Sprintf("choke    k %d: %d straits (%d between, %d within, %d major; %d cells), %d necks (%d cells); %d chains (%d cells), %d passes (%d cells) on %d chains",
			cp.MaxCells, cp.Straits, cp.StraitsBetween, cp.StraitsWithin, cp.StraitsMajor, cp.StraitCells, cp.Necks, cp.NeckCells, cp.Chains, cp.ChainCells, cp.Passes, cp.PassCells, cp.ChainsWithPass),
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
