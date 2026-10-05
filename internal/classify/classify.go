// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package classify

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/mdhender/mpg/internal/cells"
	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/mesh"
	"github.com/mdhender/mpg/internal/topo"
)

// Landform is a cell's physical geography, in hmz2ter's vocabulary.
type Landform uint8

// The landforms. The zero value is unset, which Classify never leaves.
const (
	LandformNone Landform = iota
	SaltWater
	FreshWater
	Flats
	Plains
	RollingPlains
	Hills
	Mountains
	Plateaus
	VolcanicHighlands
	numLandforms
)

// Landforms lists the landforms Classify assigns, in code order.
var Landforms = []Landform{SaltWater, FreshWater, Flats, Plains, RollingPlains, Hills, Mountains, Plateaus, VolcanicHighlands}

// LandLandforms lists the landforms of land cells, in code order.
var LandLandforms = []Landform{Flats, Plains, RollingPlains, Hills, Mountains, Plateaus, VolcanicHighlands}

var landformNames = [numLandforms]string{
	"", "salt-water", "fresh-water", "flats", "plains", "rolling-plains",
	"hills", "mountains", "plateaus", "volcanic-highlands",
}

// String returns the landform's hm* name, as "rolling-plains".
func (l Landform) String() string {
	if l < numLandforms {
		return landformNames[l]
	}
	return fmt.Sprintf("Landform(%d)", uint8(l))
}

// IsLand reports whether the landform is land shaped by relief.
func (l Landform) IsLand() bool { return l >= Flats && l < numLandforms }

// Depth is a salt-water cell's depth band, from its distance to land.
type Depth uint8

// The depth bands. DepthNone is the depth of every cell that is not salt
// water.
const (
	DepthNone Depth = iota
	Shallow
	Open
	Deep
	numDepths
)

// Depths lists the salt-water depth bands, in code order.
var Depths = []Depth{Shallow, Open, Deep}

var depthNames = [numDepths]string{"", "shallow", "open", "deep"}

// String returns the depth band's hm* name, as "shallow".
func (d Depth) String() string {
	if d < numDepths {
		return depthNames[d]
	}
	return fmt.Sprintf("Depth(%d)", uint8(d))
}

// Rules holds the landform and depth thresholds (config group classify).
// Heights and relief are in meters, the volcanic radius in km, and the
// depth bands in cell steps.
type Rules struct {
	FlatsBelowM, PlainsBelowM, RollingPlainsBelowM, HillsBelowM float64
	PlateauMinAltitudeM, PlateauBelowM                          float64
	VolcanicRadiusKm                                            float64
	ShallowMaxCells, OpenMaxCells                               int
}

// RulesOf returns the rules of a config.
func RulesOf(cfg config.Config) Rules {
	c := cfg.Classify
	return Rules{
		FlatsBelowM:         c.FlatsBelowM,
		PlainsBelowM:        c.PlainsBelowM,
		RollingPlainsBelowM: c.RollingPlainsBelowM,
		HillsBelowM:         c.HillsBelowM,
		PlateauMinAltitudeM: c.PlateauMinAltitudeM,
		PlateauBelowM:       c.PlateauBelowM,
		VolcanicRadiusKm:    c.VolcanicRadiusKm,
		ShallowMaxCells:     c.ShallowMaxCells,
		OpenMaxCells:        c.OpenMaxCells,
	}
}

// Landform returns the landform of a land cell with relief meters of
// relief and height meters above sea level, as hmz2ter's Rules.Landform:
// plateaus when the cell is at least PlateauMinAltitudeM high with relief
// below PlateauBelowM, otherwise the relief class. It never returns
// VolcanicHighlands, which depends on where the volcanoes are.
func (r Rules) Landform(relief, height float64) Landform {
	if height >= r.PlateauMinAltitudeM && relief < r.PlateauBelowM {
		return Plateaus
	}
	switch {
	case relief < r.FlatsBelowM:
		return Flats
	case relief < r.PlainsBelowM:
		return Plains
	case relief < r.RollingPlainsBelowM:
		return RollingPlains
	case relief < r.HillsBelowM:
		return Hills
	}
	return Mountains
}

// Depth returns the depth band of salt water steps cell steps from land,
// as hmz2ter's Rules.Depth.
func (r Rules) Depth(steps int) Depth {
	switch {
	case steps <= r.ShallowMaxCells:
		return Shallow
	case steps <= r.OpenMaxCells:
		return Open
	}
	return Deep
}

// Result is the classification of every cell of a mesh, indexed by cell id.
type Result struct {
	// Landform is each cell's landform. Rim cells are SaltWater.
	Landform []Landform
	// Depth is each salt-water cell's depth band, DepthNone for the rest.
	// Rim cells are Deep.
	Depth []Depth
	// SeaSteps is, for a playable salt-water cell, the number of cell steps
	// through playable salt water from the nearest land cell, or −1 when no
	// land reaches it. It is 0 for land and −1 for rim cells.
	SeaSteps []int
	// Volcano marks the land cells holding a volcano's peak.
	Volcano []bool
	// VolcanoCell is, for each hotspot peak in the order given, the cell
	// that holds it (the cell of the nearest site), and VolcanoLand whether
	// that cell is land, so the hotspot is a volcano.
	VolcanoCell []int
	VolcanoLand []bool
}

// Count returns the number of cells with landform l, rim cells excluded
// when playable is true. m must be the mesh classified.
func (r *Result) Count(m *mesh.Mesh, l Landform, playable bool) int {
	n := 0
	for i, v := range r.Landform {
		if v == l && !(playable && m.Cells[i].Rim) {
			n++
		}
	}
	return n
}

// LandHistogram returns the number of cells of each land landform, in the
// order of LandLandforms, and their total, the land cells. Rim cells are
// never land.
func (r *Result) LandHistogram() (counts []int, total int) {
	counts = make([]int, len(LandLandforms))
	for _, l := range r.Landform {
		if l.IsLand() {
			counts[l-Flats]++
			total++
		}
	}
	return counts, total
}

// landAbbrev abbreviates the land landforms for compact summaries.
var landAbbrev = [...]string{"fl", "pl", "rp", "hi", "mt", "pt", "vh"}

// LandShares returns, for each land landform in the order of
// LandLandforms, its count and share of the land cells, as
// "hills 1234 (12.3%)", or with compact true its abbreviation and rounded
// percentage, as "hi12".
func (r *Result) LandShares(compact bool) []string {
	counts, total := r.LandHistogram()
	out := make([]string, len(counts))
	for k, l := range LandLandforms {
		pct := 0.0
		if total > 0 {
			pct = 100 * float64(counts[k]) / float64(total)
		}
		if compact {
			out[k] = fmt.Sprintf("%s%.0f", landAbbrev[k], pct)
		} else {
			out[k] = fmt.Sprintf("%s %d (%.1f%%)", l, counts[k], pct)
		}
	}
	return out
}

// Volcanoes returns the number of hotspots whose peak is on land.
func (r *Result) Volcanoes() int {
	n := 0
	for _, v := range r.VolcanoLand {
		if v {
			n++
		}
	}
	return n
}

// Classify classifies the cells of m with altitudes alt and reliefs relief
// (package cells' Stats) against the land and water in f (the sea level
// stage's flood at f.Level) and the volcanic hotspot peaks, by rules r.
// See the package documentation for the rules.
func Classify(m *mesh.Mesh, alt, relief []float64, f *cells.Flood, peaks []topo.Point, r Rules) (*Result, error) {
	n := len(m.Cells)
	if len(alt) != n || len(relief) != n || len(f.Land) != n || len(f.Ocean) != n {
		return nil, fmt.Errorf("classify: %d altitudes, %d reliefs, %d land and %d ocean flags for %d cells",
			len(alt), len(relief), len(f.Land), len(f.Ocean), n)
	}
	for i := range n {
		if a, b := alt[i], relief[i]; !finite(a) || !finite(b) || b < 0 {
			return nil, fmt.Errorf("classify: cell %d altitude %v, relief %v", i, a, b)
		}
	}
	res := &Result{
		Landform:    make([]Landform, n),
		Depth:       make([]Depth, n),
		SeaSteps:    make([]int, n),
		Volcano:     make([]bool, n),
		VolcanoCell: make([]int, len(peaks)),
		VolcanoLand: make([]bool, len(peaks)),
	}

	// Landforms from relief and the height above sea level.
	for i, c := range m.Cells {
		switch {
		case c.Rim:
			res.Landform[i] = SaltWater
		case f.Land[i]:
			res.Landform[i] = r.Landform(relief[i], alt[i]-f.Level)
		default:
			res.Landform[i] = SaltWater
		}
	}

	// Volcanoes: the cell of the site nearest each peak, if it is land.
	if len(peaks) > 0 {
		sites := make([]topo.Point, n)
		for i, c := range m.Cells {
			sites[i] = c.Site
		}
		loc, err := cells.NewLocator(m.Cylinder(), sites)
		if err != nil {
			return nil, err
		}
		for k, p := range peaks {
			i := loc.Nearest(p)
			res.VolcanoCell[k] = i
			if !m.Cells[i].Rim && res.Landform[i].IsLand() {
				res.VolcanoLand[k] = true
				res.Volcano[i] = true
			}
		}
	}

	// Plateaus near a volcano are volcanic highlands.
	cyl := m.Cylinder()
	for i, c := range m.Cells {
		if res.Landform[i] != Plateaus {
			continue
		}
		for k, p := range peaks {
			if res.VolcanoLand[k] && cyl.Distance(c.Site, p) <= r.VolcanicRadiusKm {
				res.Landform[i] = VolcanicHighlands
				break
			}
		}
	}

	// Breadth-first search from every land cell through playable salt
	// water, over cell adjacency.
	salt := func(i int) bool { return !m.Cells[i].Rim && res.Landform[i] == SaltWater }
	queue := make([]int, 0, n)
	for i, c := range m.Cells {
		switch {
		case c.Rim, salt(i):
			res.SeaSteps[i] = -1
		default:
			queue = append(queue, i)
		}
	}
	for k := 0; k < len(queue); k++ {
		i := queue[k]
		for _, j := range m.Cells[i].Neighbors {
			if salt(j) && res.SeaSteps[j] < 0 {
				res.SeaSteps[j] = res.SeaSteps[i] + 1
				queue = append(queue, j)
			}
		}
	}
	for i, c := range m.Cells {
		switch {
		case c.Rim:
			res.Depth[i] = Deep
		case salt(i):
			res.Depth[i] = Deep
			if res.SeaSteps[i] >= 0 {
				res.Depth[i] = r.Depth(res.SeaSteps[i])
			}
		}
	}
	return res, nil
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// AppendBinary appends the classification's canonical encoding to b; it is
// the input to the golden hashes. Integers are little-endian uint64. In
// order:
//
//	number of cells                     integer
//	per cell, in id order:
//	    Landform, Depth                 one byte each
//	    SeaSteps                        integer
//	    Volcano                         one byte, 0 or 1
//	number of hotspots                  integer
//	per hotspot, in order:
//	    VolcanoCell                     integer
//	    VolcanoLand                     one byte, 0 or 1
//
// The error is always nil; the signature is encoding.BinaryAppender's.
func (r *Result) AppendBinary(b []byte) ([]byte, error) {
	i := func(v int) { b = binary.LittleEndian.AppendUint64(b, uint64(int64(v))) }
	flag := func(v bool) {
		if v {
			b = append(b, 1)
		} else {
			b = append(b, 0)
		}
	}
	i(len(r.Landform))
	for c := range r.Landform {
		b = append(b, byte(r.Landform[c]), byte(r.Depth[c]))
		i(r.SeaSteps[c])
		flag(r.Volcano[c])
	}
	i(len(r.VolcanoCell))
	for k := range r.VolcanoCell {
		i(r.VolcanoCell[k])
		flag(r.VolcanoLand[k])
	}
	return b, nil
}
