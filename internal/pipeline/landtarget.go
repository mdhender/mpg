// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package pipeline

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"math"
	"strings"

	"github.com/mdhender/mpg/internal/basin"
	"github.com/mdhender/mpg/internal/cells"
	"github.com/mdhender/mpg/internal/climate"
	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/mesh"
)

// LandTarget is the land-target stage's product: the sea-level search with
// the basins inside it, the land and water at the level it chose, lakes
// included, and the climate's final pass over them.
type LandTarget struct {
	// Search is the search's record. Each probe's Land counts after the
	// lakes, Lake the lake and inland-sea cells, and Basin the basin
	// floors (water candidates the ocean flood does not reach) left dry.
	Search *cells.Record
	// Flood is the land and water at the chosen level before the lakes:
	// its Land includes the lake cells.
	Flood *cells.Flood
	// Basins and Lakes are the basin hierarchy and its water balance at
	// the chosen level, against the first climate pass.
	Basins *basin.Result
	Lakes  *basin.Lakes
	// Land marks the land after lakes: the playable cells that are neither
	// ocean nor lake. Rim cells are never land.
	Land []bool
	// LandCells, OceanCells and LakeCells count the land, ocean, and lake
	// and inland-sea cells; DryBasinCells the land at or below the level
	// (basin floors left dry). LandAreaKm2 is the land cells' area summed
	// in id order.
	LandCells, OceanCells, LakeCells, DryBasinCells int
	LandAreaKm2                                     float64
	// Climate is the final climate pass, with the lakes: inland seas
	// recharge the air as the ocean does, lakes do not (climate
	// LakeSurface). ClimatePasses counts the climate passes of the run: 2,
	// the first (stage 7) and this one.
	Climate       *climate.Result
	ClimatePasses int
}

// Met reports whether the land contract is met.
func (t *LandTarget) Met() bool { return t.Search.Met }

// Lake returns the lake of cell i, or nil when it is not a lake cell.
func (t *LandTarget) Lake(i int) *basin.Lake {
	if k := t.Lakes.Lake[i]; k != basin.None {
		return &t.Lakes.Lakes[k]
	}
	return nil
}

// InlandSeas returns, per cell, whether it is an inland-sea cell.
func (t *LandTarget) InlandSeas() []bool {
	sea := make([]bool, len(t.Land))
	for _, k := range t.Lakes.Lakes {
		if k.Kind == basin.KindInlandSea {
			for _, c := range k.Cells {
				sea[c] = true
			}
		}
	}
	return sea
}

// lakesAt measures one sea level for the land-target search: the ocean
// flood, the basin hierarchy on it at the configured minimum depth, and its
// water balance against the fixed climate cl.
func lakesAt(m *mesh.Mesh, alt, area []float64, cl basin.Climate, cfg *config.Config, level float64) (*cells.Flood, *basin.Result, *basin.Lakes, error) {
	fl := cells.Classify(m, alt, level)
	seed := BasinSeed(m, fl)
	r, err := basin.Find(m, alt, seed, cfg.Basin.MinDepthM)
	if err != nil {
		return nil, nil, nil, err
	}
	lakes, err := basin.Balance(m, alt, seed, area, r, cl, BasinParams(cfg))
	if err != nil {
		return nil, nil, nil, err
	}
	return fl, r, lakes, nil
}

// probeOf returns the search probe of a flood and its lakes: land after
// lakes, ocean, the basin floors left dry, and the lake cells.
func probeOf(fl *cells.Flood, lakes *basin.Lakes) cells.Probe {
	p := cells.Probe{Level: fl.Level, Ocean: fl.OceanCells, Lake: lakes.Cells()}
	p.Land = fl.LandCells - p.Lake
	for i, b := range fl.Basin {
		if b && lakes.Lake[i] == basin.None {
			p.Basin++
		}
	}
	return p
}

// SearchLand runs the land-target search (DESIGN.md, "Climate coupling"):
// the sea-level search of package cells, measuring at every probe the ocean
// flood (stage 6), the basin hierarchy and its water balance (stage 8)
// against the fixed climate cl, the first pass, and counting land after
// the lakes. The first probe is the quantile estimate for N + expected
// land candidates, expected being the lake cells the caller expects (the
// basins stage's at the first sea level). It returns everything but the
// final climate pass. An unmet target is a result, not an error.
func SearchLand(m *mesh.Mesh, alt []float64, cl *climate.Result, cfg *config.Config, expected int) (*LandTarget, error) {
	if cl == nil {
		return nil, errors.New("pipeline: land target needs the climate")
	}
	area := CellAreas(m)
	bc := basin.Climate{Precipitation: cl.Precipitation, PET: cl.PET, Runoff: cl.Runoff}
	var failed error
	rec, err := cells.SearchLevels(m, alt, cfg.World.LandCells, expected, func(level float64) cells.Probe {
		if failed != nil {
			return cells.Probe{Level: level}
		}
		fl, _, lakes, err := lakesAt(m, alt, area, bc, cfg, level)
		if err != nil {
			failed = err
			return cells.Probe{Level: level}
		}
		return probeOf(fl, lakes)
	})
	if err != nil {
		return nil, err
	}
	if failed != nil {
		return nil, failed
	}
	fl, r, lakes, err := lakesAt(m, alt, area, bc, cfg, rec.Result().Level)
	if err != nil {
		return nil, err
	}
	t := &LandTarget{Search: rec, Flood: fl, Basins: r, Lakes: lakes, Land: make([]bool, len(m.Cells))}
	p := probeOf(fl, lakes)
	if b := rec.Result(); p.Land != b.Land || p.Ocean != b.Ocean || p.Basin != b.Basin || p.Lake != b.Lake {
		return nil, fmt.Errorf("pipeline: land target remeasured %+v, the search measured %+v", p, b)
	}
	for i := range m.Cells {
		if fl.Land[i] && lakes.Lake[i] == basin.None {
			t.Land[i] = true
			t.LandAreaKm2 += area[i]
		}
	}
	t.LandCells, t.OceanCells, t.LakeCells, t.DryBasinCells = p.Land, p.Ocean, p.Lake, p.Basin
	return t, nil
}

// AppendBinary appends the land target's canonical encoding to b; it is the
// input to the golden hashes. Integers are little-endian uint64; floats
// their IEEE 754 bits as little-endian uint64; strings their byte length,
// then the bytes; booleans an integer 0 or 1. In order:
//
//	Target, Tolerance, Budget, Expected           integers
//	Policy                                        string
//	Initial                                       float
//	Reason                                        string
//	Met                                           boolean
//	Best, number of probes                        integers
//	per probe: Method, Index                      integers
//	    Level                                     float
//	    Land, Ocean, Basin, Lake                  integers
//	LandCells, OceanCells, LakeCells,
//	DryBasinCells                                 integers
//	LandAreaKm2                                   float
//	number of cells                               integer
//	per cell, in id order, one byte:              1 land | 2 ocean | 4 lake
//	Lakes                                         basin.Lakes.AppendBinary
//	ClimatePasses                                 integer
//	Climate                                       climate.Result.AppendBinary
//
// The hierarchy is not repeated: the lakes encode each cell's lake, sink
// and descent. The error is always nil; the signature is
// encoding.BinaryAppender's.
func (t *LandTarget) AppendBinary(b []byte) ([]byte, error) {
	f := func(v float64) { b = binary.LittleEndian.AppendUint64(b, math.Float64bits(v)) }
	i := func(v int) { b = binary.LittleEndian.AppendUint64(b, uint64(int64(v))) }
	str := func(v string) { i(len(v)); b = append(b, v...) }
	r := t.Search
	i(r.Target)
	i(r.Tolerance)
	i(r.Budget)
	i(r.Expected)
	str(r.Policy)
	f(r.Initial)
	str(string(r.Reason))
	i(btoi(r.Met))
	i(r.Best)
	i(len(r.Trace))
	for _, p := range r.Trace {
		i(int(p.Method))
		i(p.Index)
		f(p.Level)
		i(p.Land)
		i(p.Ocean)
		i(p.Basin)
		i(p.Lake)
	}
	i(t.LandCells)
	i(t.OceanCells)
	i(t.LakeCells)
	i(t.DryBasinCells)
	f(t.LandAreaKm2)
	i(len(t.Land))
	for c := range t.Land {
		var v byte
		if t.Land[c] {
			v |= 1
		}
		if t.Flood.Ocean[c] {
			v |= 2
		}
		if t.Lakes.Lake[c] != basin.None {
			v |= 4
		}
		b = append(b, v)
	}
	b, _ = t.Lakes.AppendBinary(b)
	i(t.ClimatePasses)
	if t.Climate != nil {
		b, _ = t.Climate.AppendBinary(b)
	}
	return b, nil
}

func btoi(v bool) int {
	if v {
		return 1
	}
	return 0
}

// traceLine returns the search's probes as "method level → land (lakes)".
func traceLine(r *cells.Record) string {
	probes := make([]string, len(r.Trace))
	for k, p := range r.Trace {
		probes[k] = fmt.Sprintf("%s %.1f m → %d (%d lake)", p.Method, p.Level, p.Land, p.Lake)
	}
	return strings.Join(probes, "; ")
}

// ClimatePasses is the number of climate passes a run through the
// land-target stage makes: stage 7's, with the ocean of the first sea
// level, and the final one with the lakes.
const ClimatePasses = 2

// traceWidth is the width in pixels of the land-target stage's trace
// render; its height follows the raster's shape, so sweep tiles keep their
// proportions.
const traceWidth = 960

// runLandTarget is stage 9 (DESIGN.md, "Climate coupling"). It runs the
// sea-level search with the basins inside it (SearchLand) against the
// first climate pass, expecting the basins stage's lake cells at the first
// sea level, then the final climate pass with the lakes, in which inland
// seas recharge the air and lakes do not (climate.ComputeFinal). It logs
// the target, the result, the probes, the lakes and the passes, and renders
// the search trace (cells.TraceRender), and as variants the final lakes
// ("lakes", basin.LakesRender) and the final climate's precipitation
// ("precip") and aridity ("aridity"). An unmet target is reported, not an
// error.
func runLandTarget(c *Context) error {
	p := &c.Products
	m, s := p.Mesh, p.Cells
	expected := 0
	if p.Lakes != nil {
		expected = p.Lakes.Cells()
	}
	t, err := SearchLand(m, s.Altitude, p.Climate, &c.Config, expected)
	if err != nil {
		return err
	}
	model, err := climate.NewModel(c.Config.Climate)
	if err != nil {
		return err
	}
	t.Climate, err = climate.ComputeFinal(p.Elevation, m, s, t.Flood, t.InlandSeas(), model, uint64(c.Config.Seed))
	if err != nil {
		return err
	}
	t.ClimatePasses = ClimatePasses
	r := t.Search
	c.Logf("target %d land cells ± %d after lakes; expecting %d lake cells; policy %s, budget %d",
		r.Target, r.Tolerance, r.Expected, r.Policy, r.Budget)
	c.Logf("level %.3f m (estimate %.3f m): %d land (%+d, %+.2f%%), %d ocean, %d lake and inland-sea, %d dry basin cells; land area %.0f km²",
		r.Result().Level, r.Initial, t.LandCells, r.Deviation(), r.DeviationPercent(), t.OceanCells, t.LakeCells, t.DryBasinCells, t.LandAreaKm2)
	c.Logf("%d probes, %s: %s", len(r.Trace), r.Reason, traceLine(r))
	nl, ns, salt, full := t.Lakes.Counts()
	c.Logf("%d lakes, %d inland seas (%d salt), %d playas, %d of %d basins full; residual %.1e; %d climate passes",
		nl, ns, salt, len(t.Lakes.Playas), full, len(t.Basins.Basins), t.Lakes.Residual, t.ClimatePasses)
	if !r.Met {
		c.Logf("UNMET land target: %d land cells, %d outside %d ± %d (%s)",
			t.LandCells, abs(r.Deviation())-r.Tolerance, r.Target, r.Tolerance, r.Reason)
	}
	f := p.Elevation
	seed := BasinSeed(m, t.Flood)
	for _, v := range []struct {
		name string
		draw func() *image.RGBA
	}{
		{"", func() *image.RGBA {
			return cells.TraceRender(r, traceWidth, max(1, int(math.Round(float64(traceWidth*f.NY())/float64(f.NX())))))
		}},
		{"lakes", func() *image.RGBA {
			return basin.LakesRender(f, m, s.Altitude, seed, t.Flood.Level, t.Basins, t.Lakes)
		}},
		{"precip", func() *image.RGBA { return climate.PrecipRender(f, m, t.Climate) }},
		{"aridity", func() *image.RGBA { return climate.AridityRender(f, m, t.Climate) }},
	} {
		if !c.rendering() {
			break
		}
		if err := c.Render(v.name, v.draw()); err != nil {
			return err
		}
	}
	p.Target = t
	return nil
}
