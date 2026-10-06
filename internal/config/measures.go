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
// measures"; pipeline stage 13): the landmass classes, the chokepoint and
// pass parameters, the coast distance of the usability measures, and the
// checks run against the measures.
type Measures struct {
	// Landmass sets the landmass classes.
	Landmass Landmass `json:"landmass"`
	// Chokepoints sets the straits and necks.
	Chokepoints Chokepoints `json:"chokepoints"`
	// Passes sets the mountain chains and passes.
	Passes Passes `json:"passes"`
	// Usability sets the usability measures.
	Usability Usability `json:"usability"`
	// Checks lists the checks, run in order. A file that sets the list
	// replaces the default list as a whole; [] runs none.
	Checks []Check `json:"checks"`
}

// Landmass sets the landmass classes by cell count: an islet has at most
// IsletMaxCells cells, a continent at least ContinentMinCells, and an
// island lies between. A province is a fixed area, so the thresholds are
// absolute. S34 measured 24 default worlds: stray fragments have 1 to 58
// cells, the masses a preset intends at least about 100 (islands 100 to
// 1,100; archipelago 500 to 3,200; continents 1,500 to 4,600).
type Landmass struct {
	IsletMaxCells     int `json:"islet_max_cells"`
	ContinentMinCells int `json:"continent_min_cells"`
}

// Chokepoints sets the straits and necks, in cells.
type Chokepoints struct {
	// MaxCells is k: a strait crosses at most k water cells, and a neck
	// is a cut of at most k land cells (1 to MaxChokepointCells).
	MaxCells int `json:"max_cells"`
	// DetourCells is the land distance in cell steps beyond which two
	// shores of one landmass count as its separate parts: a water
	// crossing between shores that no land path of at most DetourCells
	// steps joins is a strait.
	DetourCells int `json:"detour_cells"`
	// NeckMinRegionCells is the smallest region a neck may join: a cut is
	// a neck only when the two largest regions it leaves each have at
	// least this many cells.
	NeckMinRegionCells int `json:"neck_min_region_cells"`
}

// Passes sets the mountain chains and passes.
type Passes struct {
	// ChainMinCells is the fewest mountain cells, connected, that make a
	// chain.
	ChainMinCells int `json:"chain_min_cells"`
	// MaxCells is the most chain cells a pass's route may cross.
	MaxCells int `json:"max_cells"`
	// MaxGradePercent is the steepest |grade| allowed on any edge of a
	// pass's route, in percent.
	MaxGradePercent float64 `json:"max_grade_percent"`
	// DetourCells is the land distance in cell steps, avoiding the chain,
	// beyond which two cells beside it lie on its two sides.
	DetourCells int `json:"detour_cells"`
}

// Usability sets the usability measures.
type Usability struct {
	// CoastCells is d: usability.coast_within_cells counts the land cells
	// at most d cell steps from playable water, a cell touching water
	// being 1 step away (1 to MaxCoastCells). S35 measured 56 default
	// worlds: within 3 cells is 30 to 69% of the land (median 56%;
	// pangaea lowest, islands highest).
	CoastCells int `json:"coast_cells"`
}

// MaxCoastCells bounds measures.usability.coast_cells.
const MaxCoastCells = 1000

// MaxChokepointCells bounds measures.chokepoints.max_cells: a neck's cut
// is searched over every path of up to that many cells, which grows fast.
const MaxChokepointCells = 4

// MaxPassCells bounds measures.passes.max_cells.
const MaxPassCells = 16

// MaxDetourCells bounds the detour distances.
const MaxDetourCells = 1000

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

// DefaultMeasures returns the default landmass classes, chokepoint and
// pass parameters (S34; see Landmass, Chokepoints and Passes), the coast
// distance (S35; see Usability), and the default checks. They are all report-only
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
	return Measures{
		Landmass:    Landmass{IsletMaxCells: 9, ContinentMinCells: 1000},
		Chokepoints: Chokepoints{MaxCells: 3, DetourCells: 20, NeckMinRegionCells: 10},
		Passes:      Passes{ChainMinCells: 10, MaxCells: 3, MaxGradePercent: 3, DetourCells: 20},
		Usability:   Usability{CoastCells: 3},
		Checks: []Check{
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
	if v := m.Landmass.IsletMaxCells; v < 0 {
		bad("measures.landmass.islet_max_cells %d must not be negative", v)
	}
	if v := m.Landmass.ContinentMinCells; v <= m.Landmass.IsletMaxCells+1 {
		bad("measures.landmass.continent_min_cells %d must be greater than islet_max_cells + 1 (%d), so islands exist", v, m.Landmass.IsletMaxCells+1)
	}
	if v := m.Chokepoints.MaxCells; v < 1 || v > MaxChokepointCells {
		bad("measures.chokepoints.max_cells %d must be in [1, %d]", v, MaxChokepointCells)
	}
	if v := m.Chokepoints.DetourCells; v < 1 || v > MaxDetourCells {
		bad("measures.chokepoints.detour_cells %d must be in [1, %d]", v, MaxDetourCells)
	}
	if v := m.Chokepoints.NeckMinRegionCells; v < 1 {
		bad("measures.chokepoints.neck_min_region_cells %d must be at least 1", v)
	}
	if v := m.Passes.ChainMinCells; v < 1 {
		bad("measures.passes.chain_min_cells %d must be at least 1", v)
	}
	if v := m.Passes.MaxCells; v < 1 || v > MaxPassCells {
		bad("measures.passes.max_cells %d must be in [1, %d]", v, MaxPassCells)
	}
	if v := m.Passes.MaxGradePercent; !(v >= 0 && v <= 100) {
		bad("measures.passes.max_grade_percent %v must be in [0, 100]", v)
	}
	if v := m.Passes.DetourCells; v < 1 || v > MaxDetourCells {
		bad("measures.passes.detour_cells %d must be in [1, %d]", v, MaxDetourCells)
	}
	if v := m.Usability.CoastCells; v < 1 || v > MaxCoastCells {
		bad("measures.usability.coast_cells %d must be in [1, %d]", v, MaxCoastCells)
	}
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
