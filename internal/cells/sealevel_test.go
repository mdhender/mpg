// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package cells

import (
	"bytes"
	"math"
	"slices"
	"testing"

	"github.com/mdhender/mpg/internal/mesh"
	"github.com/mdhender/mpg/internal/render"
)

// interior returns a playable cell of m none of whose neighbors is a rim
// cell, preferring one near the middle of the cell list.
func interior(t *testing.T, m *mesh.Mesh) int {
	t.Helper()
	for k := range len(m.Cells) {
		i := (len(m.Cells)/2 + k) % len(m.Cells)
		if m.Cells[i].Rim {
			continue
		}
		if !slices.ContainsFunc(m.Cells[i].Neighbors, func(j int) bool { return m.Cells[j].Rim }) {
			return i
		}
	}
	t.Fatal("no interior cell")
	return -1
}

// filled returns altitudes v for every cell of m.
func filled(m *mesh.Mesh, v float64) []float64 {
	alt := make([]float64, len(m.Cells))
	for i := range alt {
		alt[i] = v
	}
	return alt
}

// TestClassifyDryBasin checks that a cell below sea level enclosed by land
// is a basin floor and stays land, not ocean, while water touching the rim
// is ocean, and that rim cells are never land.
func TestClassifyDryBasin(t *testing.T) {
	_, m, _ := smallWorld(t, 1, "cinematic", 600)
	alt := filled(m, 100)
	pit := interior(t, m)
	alt[pit] = -500
	// A cell beside the rim, below sea level: ocean.
	shore := -1
	for i, c := range m.Cells {
		if !c.Rim && slices.ContainsFunc(c.Neighbors, func(j int) bool { return m.Cells[j].Rim }) {
			shore = i
			break
		}
	}
	alt[shore] = -10
	f := Classify(m, alt, 0)
	if !f.Land[pit] || f.Ocean[pit] || !f.Basin[pit] {
		t.Errorf("pit %d: land %v ocean %v basin %v; want a dry basin floor that is land", pit, f.Land[pit], f.Ocean[pit], f.Basin[pit])
	}
	if !f.Ocean[shore] || f.Land[shore] || f.Basin[shore] {
		t.Errorf("shore cell %d: land %v ocean %v basin %v; want ocean", shore, f.Land[shore], f.Ocean[shore], f.Basin[shore])
	}
	playable := 0
	for i, c := range m.Cells {
		if c.Rim {
			if f.Land[i] || f.Ocean[i] || f.Basin[i] {
				t.Errorf("rim cell %d flagged", i)
			}
			continue
		}
		playable++
	}
	if f.Playable != playable || f.LandCells != playable-1 || f.OceanCells != 1 || f.BasinCells != 1 {
		t.Errorf("counts playable %d land %d ocean %d basin %d; want %d, %d, 1, 1", f.Playable, f.LandCells, f.OceanCells, f.BasinCells, playable, playable-1)
	}

	// Through Search: a target of every playable cell but the shore cell
	// is met exactly with the pit still land.
	s, err := Search(m, alt, playable-1)
	if err != nil {
		t.Fatal(err)
	}
	if s.Reason != ReasonExact || !s.Met || !s.Land[pit] || !s.Basin[pit] || !s.Ocean[shore] {
		t.Errorf("search: reason %s, met %v, level %v; pit land %v basin %v; shore ocean %v",
			s.Reason, s.Met, s.Level, s.Land[pit], s.Basin[pit], s.Ocean[shore])
	}
}

// TestClassifyCornerContact checks that water touching the ocean only at a
// 4-way corner left by a collapsed short edge is not ocean: the two cells
// across the collapsed edge are not neighbors.
func TestClassifyCornerContact(t *testing.T) {
	_, m, _ := smallWorld(t, 2, "square", 600)
	if m.Collapses == 0 {
		t.Fatal("the test mesh has no collapsed edge")
	}
	tried := 0
	for _, corner := range m.Corners {
		if len(corner.Cells) != 4 || slices.ContainsFunc(corner.Cells, func(i int) bool { return m.Cells[i].Rim }) {
			continue
		}
		// The pair of corner cells that are not neighbors.
		p, q := -1, -1
		for a, i := range corner.Cells {
			for _, j := range corner.Cells[a+1:] {
				if !slices.Contains(m.Cells[i].Neighbors, j) {
					p, q = i, j
				}
			}
		}
		if p < 0 {
			continue
		}
		// Everything water, except a ring of land around q; q itself water.
		alt := filled(m, -100)
		for _, j := range m.Cells[q].Neighbors {
			alt[j] = 100
		}
		f := Classify(m, alt, 0)
		if !f.Ocean[p] {
			continue // p is cut off by q's ring too; try another corner
		}
		tried++
		if f.Ocean[q] || !f.Basin[q] || !f.Land[q] {
			t.Errorf("corner cells %d and %d: %d is ocean and %d has ocean %v, basin %v, land %v; want a basin floor, not ocean",
				p, q, p, q, f.Ocean[q], f.Basin[q], f.Land[q])
		}
		if tried == 5 {
			break
		}
	}
	if tried == 0 {
		t.Fatal("no 4-way corner to test")
	}
}

// TestSearchUnreachable checks targets no level meets: above the playable
// count, and between the two sides of a jump. The search reports the
// unmet target, says why, and keeps the best level it measured.
func TestSearchUnreachable(t *testing.T) {
	_, m, _ := smallWorld(t, 3, "cinematic", 600)
	alt := filled(m, 50)
	s, err := Search(m, alt, 10*len(m.Cells))
	if err != nil {
		t.Fatal(err)
	}
	if s.Met || s.Reason != ReasonUnreachable || s.LandCells != s.Playable || s.Level >= 50 {
		t.Errorf("target above playable: met %v reason %s land %d of %d level %v; want unreachable, all land",
			s.Met, s.Reason, s.LandCells, s.Playable, s.Level)
	}

	// One altitude for every playable cell: the count jumps from all land
	// to none, so half is unreachable; the deviations tie and the lower
	// level (all land) wins.
	half := s.Playable / 2
	s, err = Search(m, alt, s.Playable-half)
	if err != nil {
		t.Fatal(err)
	}
	if s.Met || s.Reason != ReasonUnreachable || s.LandCells != s.Playable || s.Deviation() != half {
		t.Errorf("jump: met %v reason %s land %d (deviation %d); want unreachable, all land", s.Met, s.Reason, s.LandCells, s.Deviation())
	}
	if len(s.Trace) == 0 || len(s.Trace) > SearchBudget {
		t.Errorf("%d probes", len(s.Trace))
	}
}

// TestSearchPolicy checks the search on synthetic land counts: it keeps
// the best measured probe when the count is not monotonic, breaks ties to
// the lower level, reports a met target that is not exact as
// within-tolerance, gallops while the bracket is open, and stops at the
// budget.
func TestSearchPolicy(t *testing.T) {
	const k = 1000
	var c candidates
	for j := range k {
		c.levels = append(c.levels, float64(j))
		c.ranks = append(c.ranks, j+1)
	}
	// Land falls by 3 cells per level: 3000 at j = −1. Target 1501 is
	// between 1500 (j = 499) and 1503 (j = 498): no exact level; the best
	// is j = 499, 1 cell short.
	steps := func(j int) Probe { return Probe{Land: 3 * (k - 1 - j)} }
	trace, best, reason := search(c, 1501, 20, SearchBudget, steps)
	if reason != ReasonWithinTolerance || trace[best].Index != 499 {
		t.Errorf("steps: reason %s best index %d; want within-tolerance at 499 (trace %+v)", reason, trace[best].Index, trace)
	}
	// Same, tolerance 0: unreachable, best kept.
	trace, best, reason = search(c, 1501, 0, SearchBudget, steps)
	if reason != ReasonUnreachable || trace[best].Index != 499 {
		t.Errorf("steps, no tolerance: reason %s best index %d; want unreachable at 499", reason, trace[best].Index)
	}
	// A tie: target 1500 + 1.5 is impossible in integers, so use 2 per
	// level and target 1001: 1002 at j = 498 and 1000 at j = 499 tie; the
	// lower level wins.
	twos := func(j int) Probe { return Probe{Land: 2 * (k - 1 - j)} }
	trace, best, _ = search(c, 1001, 0, SearchBudget, twos)
	if trace[best].Index != 498 || trace[best].Land != 1002 {
		t.Errorf("tie: best %+v, want index 498 with 1002", trace[best])
	}
	// Non-monotonic: a sawtooth. Whatever the search visits, the best
	// probe it returns is the best it measured.
	saw := func(j int) Probe { return Probe{Land: 2*(k-1-j) + 37*(j%5)} }
	trace, best, reason = search(c, 1200, 5, SearchBudget, saw)
	for _, p := range trace {
		if d, bd := absInt(p.Land-1200), absInt(trace[best].Land-1200); d < bd || (d == bd && p.Index < trace[best].Index) {
			t.Errorf("sawtooth: probe %+v beats the returned best %+v", p, trace[best])
		}
	}
	if reason == "" || len(trace) > SearchBudget {
		t.Errorf("sawtooth: reason %q, %d probes", reason, len(trace))
	}
	// A slow count (a third of a cell per level): rank steps fall short,
	// so the search gallops on the open side instead of bisecting far off.
	slow := func(j int) Probe { return Probe{Land: 1000 - (j+1)/3} }
	trace, best, reason = search(c, 900, 0, SearchBudget, slow)
	galloped := slices.ContainsFunc(trace, func(p Probe) bool { return p.Method == MethodGallop })
	if reason != ReasonExact || trace[best].Land != 900 || !galloped || len(trace) > 12 {
		t.Errorf("slow: reason %s, best %+v, galloped %v, %d probes: %+v", reason, trace[best], galloped, len(trace), trace)
	}
	// Budget: two probes cannot find it.
	trace, _, reason = search(c, 1501, 0, 2, steps)
	if reason != ReasonBudgetExhausted || len(trace) != 2 {
		t.Errorf("budget 2: reason %s, %d probes", reason, len(trace))
	}
	// Every probe stays strictly inside the bracket, so no level repeats.
	trace, _, _ = search(c, 777, 0, SearchBudget, saw)
	seen := map[int]bool{}
	for _, p := range trace {
		if seen[p.Index] {
			t.Errorf("sawtooth: level %d probed twice", p.Index)
		}
		seen[p.Index] = true
	}
}

func absInt(v int) int { return max(v, -v) }

// TestSearchSmallWorlds checks the search on the small test worlds'
// synthetic altitudes: the target met, the flags consistent with the
// level, determinism, and finite outputs.
func TestSearchSmallWorlds(t *testing.T) {
	for _, w := range []struct {
		seed   uint64
		aspect string
	}{{1, "cinematic"}, {2, "square"}, {3, "portrait"}} {
		_, m, f := smallWorld(t, w.seed, w.aspect, 600)
		st, err := Compute(f, m)
		if err != nil {
			t.Fatal(err)
		}
		s, err := Search(m, st.Altitude, 600)
		if err != nil {
			t.Fatal(err)
		}
		if !s.Met || s.Policy != SearchPolicy || s.Budget != SearchBudget || s.Tolerance != 6 {
			t.Errorf("seed %d: met %v land %d reason %s", w.seed, s.Met, s.LandCells, s.Reason)
		}
		if math.IsNaN(s.Level) || math.IsInf(s.Level, 0) || !(s.LandAreaKm2 > 0) {
			t.Errorf("seed %d: level %v land area %v", w.seed, s.Level, s.LandAreaKm2)
		}
		if !slices.ContainsFunc(s.Trace, func(p Probe) bool { return p.Level == s.Level && p.Land == s.LandCells }) {
			t.Errorf("seed %d: level %v is not a probe's", w.seed, s.Level)
		}
		want := Classify(m, st.Altitude, s.Level)
		if !slices.Equal(want.Land, s.Land) || !slices.Equal(want.Ocean, s.Ocean) || !slices.Equal(want.Basin, s.Basin) {
			t.Errorf("seed %d: flags differ from Classify at the level", w.seed)
		}
		for i, c := range m.Cells {
			if c.Rim {
				continue
			}
			if s.Land[i] == s.Ocean[i] || (s.Basin[i] && !s.Land[i]) || (st.Altitude[i] > s.Level) == (s.Ocean[i] || s.Basin[i]) {
				t.Errorf("seed %d cell %d: altitude %v land %v ocean %v basin %v at level %v",
					w.seed, i, st.Altitude[i], s.Land[i], s.Ocean[i], s.Basin[i], s.Level)
				break
			}
		}
		again, err := Search(m, slices.Clone(st.Altitude), 600)
		if err != nil {
			t.Fatal(err)
		}
		a, _ := s.AppendBinary(nil)
		b, _ := again.AppendBinary(nil)
		if !bytes.Equal(a, b) {
			t.Errorf("seed %d: two searches differ", w.seed)
		}
	}
}

// TestSearchErrors checks the rejected inputs.
func TestSearchErrors(t *testing.T) {
	_, m, _ := smallWorld(t, 1, "cinematic", 600)
	alt := filled(m, 1)
	if _, err := Search(m, alt[1:], 10); err == nil {
		t.Error("short altitudes accepted")
	}
	if _, err := Search(m, alt, 0); err == nil {
		t.Error("target 0 accepted")
	}
	alt[5] = math.NaN()
	if _, err := Search(m, alt, 10); err == nil {
		t.Error("NaN altitude accepted")
	}
}

// TestSeaLevelRender checks the sea level render's colors at cell sites:
// rim ice, a dry basin floor in BasinColor, land and ocean hypsometric
// against the level.
func TestSeaLevelRender(t *testing.T) {
	_, m, f := smallWorld(t, 1, "cinematic", 600)
	st, err := Compute(f, m)
	if err != nil {
		t.Fatal(err)
	}
	pit := interior(t, m)
	for _, j := range m.Cells[pit].Neighbors {
		st.Altitude[j] = max(st.Altitude[j], 3000)
	}
	st.Altitude[pit] = -3000
	s, err := Search(m, st.Altitude, 600)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Basin[pit] {
		t.Fatal("the pit is not a basin floor")
	}
	img := SeaLevelRender(f, m, st, s)
	if render.PixelHash(img) != render.PixelHash(SeaLevelRender(f, m, st, s)) {
		t.Error("two renders differ")
	}
	scale := mesh.RenderScale(f, m)
	sx, sy := float64(scale)/f.PitchX(), float64(scale)/f.PitchY()
	at := func(i int) [3]uint8 {
		p := m.Cells[i].Site
		c := img.RGBAAt(int(p.X*sx), int(p.Y*sy))
		return [3]uint8{c.R, c.G, c.B}
	}
	rgb := func(c interface{ RGBA() (r, g, b, a uint32) }) [3]uint8 {
		r, g, b, _ := c.RGBA()
		return [3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)}
	}
	if got := at(pit); got != rgb(BasinColor) {
		t.Errorf("pit pixel %v, want basin %v", got, rgb(BasinColor))
	}
	land, ocean := false, false
	for i, c := range m.Cells {
		if c.Rim || s.Basin[i] {
			continue
		}
		if at(i) == rgb(AltitudeColor(st.Altitude[i], s.Level)) {
			land = land || s.Land[i]
			ocean = ocean || s.Ocean[i]
		}
	}
	if !land || !ocean {
		t.Errorf("land seen in its color %v, ocean %v", land, ocean)
	}
}
