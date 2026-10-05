// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package cells

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/mdhender/mpg/internal/mesh"
)

// The land target's tolerance and the sea-level search's limits (DESIGN.md,
// "Units and sizing" and "Cell statistics and sea level"). They are design
// constants, not config: the contract is 1% of N, and the budget only
// bounds a search that normally needs a handful of probes.
const (
	// TolerancePercent is the land contract: the land cell count must be
	// within this percentage of N. A count L meets it when
	// 100·|L − N| ≤ TolerancePercent·N, in integers.
	TolerancePercent = 1
	// SearchBudget is the most probes (land counts measured) one search
	// makes. Bisection alone needs about log₂ of the playable cell count,
	// 16 in the default world and 25 at the largest config; the safeguard
	// below can double that.
	SearchBudget = 64
	// SearchPolicy names the search, as saved with its result.
	SearchPolicy = "quantile-newton-bisect"
)

// Reason says why a sea-level search ended.
type Reason string

// The termination reasons.
const (
	// ReasonExact: a level gave exactly N land cells.
	ReasonExact Reason = "exact"
	// ReasonWithinTolerance: no level gave exactly N, but the best is
	// within tolerance. The search ended because the bracket closed (no
	// untried level lies between a level with too much land and one with
	// too little) or the budget ran out.
	ReasonWithinTolerance Reason = "within-tolerance"
	// ReasonUnreachable: the bracket closed with the best outside
	// tolerance: between two adjacent candidate levels the land count
	// jumps across the whole tolerance band (or N is above the all-land
	// count or below the no-land one), so no level meets the target.
	ReasonUnreachable Reason = "unreachable"
	// ReasonBudgetExhausted: the budget ran out with the best outside
	// tolerance and the bracket still open.
	ReasonBudgetExhausted Reason = "budget-exhausted"
)

// Method says how a probe's level was chosen.
type Method uint8

// The probe methods.
const (
	// MethodEstimate is the first probe: the quantile estimate.
	MethodEstimate Method = iota
	// MethodNewton is a rank step: the level that would turn the land
	// count's excess or shortfall into water or land if every changed cell
	// were land candidate for candidate.
	MethodNewton
	// MethodBisect is the middle of the bracket.
	MethodBisect
	// MethodGallop is a rank step multiplied by 2, 4, 8, … (at most
	// maxGallop), taken instead of a bisection while the bracket is still
	// open on the side the target lies.
	MethodGallop
)

// maxGallop caps the gallop's multiplier.
const maxGallop = 1 << 20

// String returns "estimate", "newton", "bisect", or "gallop".
func (m Method) String() string {
	switch m {
	case MethodEstimate:
		return "estimate"
	case MethodNewton:
		return "newton"
	case MethodBisect:
		return "bisect"
	case MethodGallop:
		return "gallop"
	}
	return fmt.Sprintf("Method(%d)", uint8(m))
}

// Probe is one measured level in a sea-level search.
type Probe struct {
	// Method is how the level was chosen.
	Method Method
	// Index is the level's index among the candidate levels (the distinct
	// playable altitudes, ascending), or −1 for the level below them all.
	Index int
	// Level is the sea level in meters.
	Level float64
	// Land, Ocean, and Basin count the playable land cells (dry basin
	// floors included), ocean cells, and dry basin floor cells at Level.
	Land, Ocean, Basin int
}

// Flood is the land and water of the playable cells at one sea level.
type Flood struct {
	// Level is the sea level in meters. A playable cell is a water
	// candidate when its altitude is at or below Level, a land candidate
	// when above.
	Level float64
	// Ocean marks the water candidates connected to the rim through cell
	// adjacency (mesh Cell.Neighbors; cells that touch only at a corner
	// are not connected).
	Ocean []bool
	// Basin marks the water candidates not connected to the rim: basin
	// floors. Until the basin stage decides them they are dry, and land.
	Basin []bool
	// Land marks the playable cells that are not ocean: the land
	// candidates and the basin floors. Rim cells are never land.
	Land []bool
	// Playable, LandCells, OceanCells, and BasinCells count the non-rim
	// cells and the cells marked in Land, Ocean, and Basin.
	Playable, LandCells, OceanCells, BasinCells int
}

// Classify returns the land and water of m's cells with altitudes alt at
// sea level level. The ocean is a flood over cell adjacency from every rim
// cell (rim cells count as deep salt water) through the playable water
// candidates.
func Classify(m *mesh.Mesh, alt []float64, level float64) *Flood {
	n := len(m.Cells)
	f := &Flood{
		Level: level,
		Ocean: make([]bool, n),
		Basin: make([]bool, n),
		Land:  make([]bool, n),
	}
	seen := make([]bool, n)
	queue := make([]int, 0, n)
	for i, c := range m.Cells {
		if c.Rim {
			seen[i] = true
			queue = append(queue, i)
		}
	}
	for k := 0; k < len(queue); k++ {
		for _, j := range m.Cells[queue[k]].Neighbors {
			if !seen[j] && !m.Cells[j].Rim && alt[j] <= level {
				seen[j] = true
				f.Ocean[j] = true
				queue = append(queue, j)
			}
		}
	}
	for i, c := range m.Cells {
		if c.Rim {
			continue
		}
		f.Playable++
		switch {
		case f.Ocean[i]:
			f.OceanCells++
		default:
			f.Land[i] = true
			f.LandCells++
			if alt[i] <= level {
				f.Basin[i] = true
				f.BasinCells++
			}
		}
	}
	return f
}

// SeaLevel is the result of the sea-level search: the chosen level, the
// land and water there, and how the search went.
type SeaLevel struct {
	// Flood is the land and water at the chosen level, Level.
	Flood
	// Target is N, the requested land cells, and Tolerance the largest
	// |land − N| that meets the land contract: ⌊TolerancePercent·N/100⌋.
	Target, Tolerance int
	// Policy and Budget are SearchPolicy and SearchBudget.
	Policy string
	Budget int
	// Initial is the first probe's level: the quantile estimate.
	Initial float64
	// LandAreaKm2 is the summed area of the land cells, in cell id order.
	LandAreaKm2 float64
	// Trace lists the probes in the order made.
	Trace []Probe
	// Reason is why the search ended.
	Reason Reason
	// Met reports whether LandCells is within Tolerance of Target.
	Met bool
}

// Deviation returns LandCells − Target.
func (s *SeaLevel) Deviation() int { return s.LandCells - s.Target }

// DeviationPercent returns the deviation as a percentage of Target.
func (s *SeaLevel) DeviationPercent() float64 {
	return 100 * float64(s.Deviation()) / float64(s.Target)
}

// candidates holds the candidate levels of a search: the distinct playable
// altitudes, ascending, and for each the number of playable cells at or
// below it.
type candidates struct {
	levels []float64
	ranks  []int
}

// newCandidates returns the candidate levels of the playable altitudes.
func newCandidates(alt []float64) candidates {
	v := slices.Clone(alt)
	slices.Sort(v)
	var c candidates
	for k, a := range v {
		if k+1 < len(v) && v[k+1] == a {
			continue
		}
		c.levels = append(c.levels, a)
		c.ranks = append(c.ranks, k+1)
	}
	return c
}

// level returns candidate j's level: levels[j], or for j = −1 the largest
// float64 below levels[0], which leaves every playable cell a land
// candidate.
func (c candidates) level(j int) float64 {
	if j < 0 {
		return math.Nextafter(c.levels[0], math.Inf(-1))
	}
	return c.levels[j]
}

// rank returns the number of playable cells at or below candidate j.
func (c candidates) rank(j int) int {
	if j < 0 {
		return 0
	}
	return c.ranks[j]
}

// atRank returns the lowest candidate with at least r playable cells at or
// below it: −1 for r ≤ 0, the highest candidate for r beyond them all.
func (c candidates) atRank(r int) int {
	if r <= 0 {
		return -1
	}
	j, _ := slices.BinarySearch(c.ranks, r)
	return min(j, len(c.ranks)-1)
}

// search runs the sea-level search over cand for target land cells within
// tol, measuring each probe with eval, and returns the probes, the index of
// the best one in them, and the reason it ended. See the package
// documentation for the policy. eval may be any land count, monotonic or
// not; only the bracket's speed assumes the count falls as the level rises.
func search(cand candidates, target, tol, budget int, eval func(j int) Probe) (trace []Probe, best int, reason Reason) {
	k := len(cand.levels)
	lo, hi := -2, k // the open bracket: candidates lo+1 … hi−1 are untried
	j := cand.atRank(cand.rank(k-1) - target)
	method := MethodEstimate
	var bisectNext bool
	gallop := 1
	best = -1
	bestDev := 0
	closed := false
	prevDev := 0
	for len(trace) < budget {
		p := eval(j)
		p.Method, p.Index = method, j
		trace = append(trace, p)
		dev := p.Land - target
		if dev < 0 {
			dev = -dev
		}
		if best < 0 || dev < bestDev || (dev == bestDev && j < trace[best].Index) {
			best, bestDev = len(trace)-1, dev
		}
		if dev == 0 {
			return trace, best, ReasonExact
		}
		if p.Land > target {
			lo = j
		} else {
			hi = j
		}
		if hi-lo <= 1 {
			closed = true
			break
		}
		// A rank step that did not halve the deviation is converging
		// slowly (or the count jumped), so the next probe is not one.
		bisectNext = method == MethodNewton && 2*dev > prevDev
		prevDev = dev
		// A rank step: make the excess land water (or the shortfall land),
		// counting each candidate cell once.
		excess := p.Land - target
		newton := cand.atRank(cand.rank(j) + excess)
		// No level measured yet on the side the target lies: the bracket
		// is open there, and its middle is far off.
		open := (excess > 0 && hi == k) || (excess < 0 && lo == -2)
		switch {
		case !bisectNext && lo < newton && newton < hi:
			j, method = newton, MethodNewton
		case open:
			gallop = min(2*gallop, maxGallop)
			j, method = cand.atRank(cand.rank(j)+gallop*excess), MethodGallop
			j = min(max(j, lo+1), hi-1)
		default:
			j, method = lo+(hi-lo)/2, MethodBisect
		}
	}
	switch {
	case bestDev <= tol:
		reason = ReasonWithinTolerance
	case closed:
		reason = ReasonUnreachable
	default:
		reason = ReasonBudgetExhausted
	}
	return trace, best, reason
}

// Search finds the sea level of m's cells with altitudes alt that leaves
// target land cells (N), and returns it with the land and water there. No
// basins yet: a basin floor stays dry land. It fails when m has no playable
// cell, alt does not match m, an altitude is not finite, or target is not
// positive; an unmet target is not an error but a result with Met false.
func Search(m *mesh.Mesh, alt []float64, target int) (*SeaLevel, error) {
	if len(alt) != len(m.Cells) {
		return nil, fmt.Errorf("cells: %d altitudes for %d cells", len(alt), len(m.Cells))
	}
	if target < 1 {
		return nil, fmt.Errorf("cells: land target %d is not positive", target)
	}
	var playable []float64
	for i, c := range m.Cells {
		if v := alt[i]; math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("cells: cell %d altitude %v is not finite", i, v)
		}
		if !c.Rim {
			playable = append(playable, alt[i])
		}
	}
	if len(playable) == 0 {
		return nil, errors.New("cells: no playable cells")
	}
	cand := newCandidates(playable)
	eval := func(j int) Probe {
		f := Classify(m, alt, cand.level(j))
		return Probe{Level: f.Level, Land: f.LandCells, Ocean: f.OceanCells, Basin: f.BasinCells}
	}
	tol := TolerancePercent * target / 100
	trace, best, reason := search(cand, target, tol, SearchBudget, eval)
	s := &SeaLevel{
		Flood:   *Classify(m, alt, trace[best].Level),
		Target:  target,
		Policy:  SearchPolicy,
		Budget:  SearchBudget,
		Initial: trace[0].Level,
		Trace:   trace,
		Reason:  reason,
	}
	s.Tolerance = tol
	d := s.Deviation()
	s.Met = d <= tol && -d <= tol
	for i, land := range s.Land {
		if land {
			s.LandAreaKm2 += m.Area(i)
		}
	}
	return s, nil
}

// AppendBinary appends the search result's canonical encoding to b; it is
// the input to the golden hashes. Integers are little-endian uint64; floats
// their IEEE 754 bits as little-endian uint64; strings their byte length as
// an integer, then the bytes; booleans an integer 0 or 1. In order:
//
//	Target, Tolerance, Budget                    integers
//	Policy                                       string
//	Initial, Level                               floats
//	Playable, LandCells, OceanCells, BasinCells  integers
//	LandAreaKm2                                  float
//	Reason                                       string
//	Met                                          boolean
//	number of probes                             integer
//	per probe, in order:
//	    Method, Index                            integers
//	    Level                                    float
//	    Land, Ocean, Basin                       integers
//	number of cells                              integer
//	per cell, in id order, one byte:             1 land | 2 ocean | 4 basin
//
// The error is always nil; the signature is encoding.BinaryAppender's.
func (s *SeaLevel) AppendBinary(b []byte) ([]byte, error) {
	f := func(v float64) { b = binary.LittleEndian.AppendUint64(b, math.Float64bits(v)) }
	i := func(v int) { b = binary.LittleEndian.AppendUint64(b, uint64(int64(v))) }
	str := func(v string) { i(len(v)); b = append(b, v...) }
	i(s.Target)
	i(s.Tolerance)
	i(s.Budget)
	str(s.Policy)
	f(s.Initial)
	f(s.Level)
	i(s.Playable)
	i(s.LandCells)
	i(s.OceanCells)
	i(s.BasinCells)
	f(s.LandAreaKm2)
	str(string(s.Reason))
	met := 0
	if s.Met {
		met = 1
	}
	i(met)
	i(len(s.Trace))
	for _, p := range s.Trace {
		i(int(p.Method))
		i(p.Index)
		f(p.Level)
		i(p.Land)
		i(p.Ocean)
		i(p.Basin)
	}
	i(len(s.Land))
	for c := range s.Land {
		var v byte
		if s.Land[c] {
			v |= 1
		}
		if s.Ocean[c] {
			v |= 2
		}
		if s.Basin[c] {
			v |= 4
		}
		b = append(b, v)
	}
	return b, nil
}
