// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package config

import (
	"slices"

	"github.com/mdhender/mpg/world"
)

// Check modes: a failed report check is listed and logged; a failed gate
// also makes the run fail (mpg generate exits 3) after every output is
// written.
const (
	ModeReport = "report"
	ModeGate   = "gate"
)

// CheckModes lists the check modes.
var CheckModes = []string{ModeReport, ModeGate}

// CheckOps lists the comparisons a check may make: the measured value on
// the left, Check.Value on the right.
var CheckOps = []string{"<=", ">=", "<", ">", "=="}

// Measures sets the playability measures (DESIGN.md, "Playability
// measures"; pipeline stage 13): the checks run against them.
type Measures struct {
	// Checks lists the checks, run in order. A file that sets the list
	// replaces the default list as a whole; [] runs none.
	Checks []Check `json:"checks"`
}

// Check compares one measure with a bound: it passes when the measured
// value Op Value holds.
type Check struct {
	// Measure names the measure as measures.json does, "group.field" (see
	// world.MeasureNames).
	Measure string `json:"measure"`
	// Op is one of CheckOps.
	Op string `json:"op"`
	// Value is the bound; it must be finite.
	Value float64 `json:"value"`
	// Mode is ModeReport or ModeGate.
	Mode string `json:"mode"`
}

// DefaultMeasures returns the default checks. They are all report-only
// (DESIGN.md: "Checks begin as report-only"); their bounds come from S33's
// measurements of 44 worlds (seeds 1–5 at square and cinematic with every
// seeded preset, and a few portrait, widescreen and landscape worlds),
// with a margin:
//
//   - land deviation within ±1% of N (the land contract; measured −0.07%
//     to +0.02%) and land area within 2% of N·A (0.9986 to 1.0010);
//   - land cell area CV at most 0.13 (0.105 to 0.112);
//   - edge p5 at least 3 km (3.22 to 3.35 km at the default A);
//   - at most 30 degree-cap collapses (0 in every world);
//   - direction error mean at most 12° (9.96 to 10.79°), p95 at most 23°
//     (20.94 to 21.30°), max at most 45° (30.3 to 40.4°); reverse
//     directions not opposite on at most 1% of edges (0.30 to 0.50%);
//   - the steepest land–land grade at most 50% (17.4 to 27.1%), and none
//     at the 100% cap;
//   - no land–rim edges (0 in every world).
func DefaultMeasures() Measures {
	r := func(measure, op string, value float64) Check {
		return Check{Measure: measure, Op: op, Value: value, Mode: ModeReport}
	}
	return Measures{Checks: []Check{
		r("land.deviation_percent", ">=", -1),
		r("land.deviation_percent", "<=", 1),
		r("land.area_per_target", ">=", 0.98),
		r("land.area_per_target", "<=", 1.02),
		r("mesh.land_cell_area_cv", "<=", 0.13),
		r("mesh.edge_p5_km", ">=", 3),
		r("mesh.degree_cap_hits", "<=", 30),
		r("directions.error_mean_deg", "<=", 12),
		r("directions.error_p95_deg", "<=", 23),
		r("directions.error_max_deg", "<=", 45),
		r("directions.not_opposite_share", "<=", 0.01),
		r("grades.land_max_percent", "<=", 50),
		r("grades.land_cap_edges", "<=", 0),
		r("water.land_rim_edges", "<=", 0),
	}}
}

// validate appends the problems with the checks through bad: every check
// must name a measure (world.MeasureNames), use one of CheckOps and
// CheckModes, and have a finite value.
func (m *Measures) validate(bad func(format string, args ...any)) {
	names := world.MeasureNames()
	for k, c := range m.Checks {
		if !slices.Contains(names, c.Measure) {
			if s := closest(c.Measure, names); s != "" {
				bad("measures.checks[%d].measure %q is not a measure; did you mean %q?", k, c.Measure, s)
			} else {
				bad("measures.checks[%d].measure %q is not a measure", k, c.Measure)
			}
		}
		if !slices.Contains(CheckOps, c.Op) {
			bad("measures.checks[%d].op %q must be one of %q", k, c.Op, CheckOps)
		}
		if !finite(c.Value) {
			bad("measures.checks[%d].value %v must be finite", k, c.Value)
		}
		if !slices.Contains(CheckModes, c.Mode) {
			bad("measures.checks[%d].mode %q must be one of %q", k, c.Mode, CheckModes)
		}
	}
}

// derive normalizes an absent check list to an empty one.
func (m *Measures) derive() {
	if m.Checks == nil {
		m.Checks = []Check{}
	}
}

// closest returns the name in names nearest s by edit distance (ties to
// the earlier), or "" when none is close (as for unknown fields).
func closest(s string, names []string) string {
	best, bestDist := "", -1
	for _, n := range names {
		if d := editDistance(s, n); bestDist < 0 || d < bestDist {
			best, bestDist = n, d
		}
	}
	if bestDist >= 0 && bestDist <= max(2, len(s)/3) {
		return best
	}
	return ""
}
