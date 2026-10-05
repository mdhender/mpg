// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package climate

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/mdhender/mpg/internal/cells"
	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/internal/mesh"
)

// Model is the temperature model: a sea-level temperature curve by latitude
// and a lapse rate. Build one with NewModel; the zero value is not valid.
type Model struct {
	latDeg []float64 // the curve's latitudes in degrees, 0 first, 90 last, increasing
	tempC  []float64 // the curve's temperatures in °C, not increasing
	lapse  float64   // °C per km of height above sea level
	p      precip    // precipitation, evaporation and aridity
}

// NewModel returns the model of c. It fails when the curve has fewer than
// two points, does not run from 0° to 90° with latitudes increasing and
// temperatures not increasing, has a non-finite value, or when the lapse
// rate is negative or not finite, and when any other climate input is out
// of range (config.Climate.Validate). A resolved config always passes.
func NewModel(c config.Climate) (Model, error) {
	pts := c.SeaLevelTempC
	n := len(pts)
	if n < 2 {
		return Model{}, fmt.Errorf("climate: sea-level curve has %d points, need at least 2", n)
	}
	if pts[0].LatDeg != 0 || pts[n-1].LatDeg != 90 {
		return Model{}, fmt.Errorf("climate: sea-level curve runs from %v° to %v°, want 0° to 90°", pts[0].LatDeg, pts[n-1].LatDeg)
	}
	m := Model{latDeg: make([]float64, n), tempC: make([]float64, n), lapse: c.LapseRateCPerKm}
	for k, p := range pts {
		if !finite(p.TempC) {
			return Model{}, fmt.Errorf("climate: sea-level curve point %d temperature %v is not finite", k, p.TempC)
		}
		if k > 0 && !(p.LatDeg > pts[k-1].LatDeg && p.TempC <= pts[k-1].TempC) {
			return Model{}, fmt.Errorf("climate: sea-level curve point %d (%v°, %v °C) does not lie poleward and no warmer than point %d", k, p.LatDeg, p.TempC, k-1)
		}
		m.latDeg[k], m.tempC[k] = p.LatDeg, p.TempC
	}
	if !(m.lapse >= 0) || !finite(m.lapse) {
		return Model{}, fmt.Errorf("climate: lapse rate %v °C/km is not finite and non-negative", m.lapse)
	}
	if err := c.Validate(); err != nil {
		return Model{}, fmt.Errorf("climate: %w", err)
	}
	m.p = newPrecip(c)
	return m, nil
}

// LapseRate returns the model's lapse rate in °C per km.
func (m Model) LapseRate() float64 { return m.lapse }

// Degrees returns the latitude in degrees from the equator, north and south
// alike, of the latitude proxy lat (1 − 2·y/H): 90·|lat|, clamped to
// [0, 90]. The proxy's ±1, the map's north and south edges, are the poles.
func Degrees(lat float64) float64 {
	return min(max(fmath.Mul(90, math.Abs(lat)), 0), 90)
}

// SeaLevel returns the sea-level temperature in °C at the latitude proxy
// lat: the curve at Degrees(lat), interpolated linearly between its points
// and kept between them, so it never rises poleward.
func (m Model) SeaLevel(lat float64) float64 {
	x := Degrees(lat)
	k := 1
	for k < len(m.latDeg)-1 && m.latDeg[k] < x {
		k++
	}
	x0, x1, y0, y1 := m.latDeg[k-1], m.latDeg[k], m.tempC[k-1], m.tempC[k]
	switch {
	case x <= x0:
		return y0
	case x >= x1:
		return y1
	}
	t := (x - x0) / (x1 - x0)
	return min(max(y0+fmath.Mul(y1-y0, t), y1), y0)
}

// At returns the temperature in °C at the latitude proxy lat and heightM
// meters above sea level: SeaLevel(lat) minus the lapse rate times the
// height in km. A height at or below sea level (negative) cools nothing.
func (m Model) At(lat, heightM float64) float64 {
	return m.SeaLevel(lat) - fmath.Mul(m.lapse, max(heightM, 0)/1000)
}

// Result is the climate stage's output.
type Result struct {
	// Level is the sea level the temperatures are measured against, in
	// meters (the sea level stage's).
	Level float64
	// Ocean holds, per cell in id order, whether the cell is open salt
	// water to the climate: a rim cell, or a cell the ocean flood reached.
	// Dry basin floors are land.
	Ocean []bool
	// Mask holds, per raster sample in storage order (rows north to south,
	// columns west to east), its cell's Ocean: the cell mask drawn onto the
	// raster through cells.Stats.Owner.
	Mask []bool
	// HeightM holds, per cell, the height above sea level the lapse rate
	// applies to, in meters: altitude − Level for a land cell above the
	// sea, 0 for every other cell (ocean, rim, and dry basin floors at or
	// below the sea). The wind's orographic rules read it too.
	HeightM []float64
	// Temperature holds, per cell, the mean annual temperature in °C:
	// Model.At(latitude, HeightM).
	Temperature []float64

	// RasterPrecipitation holds, per raster sample in storage order, the
	// annual precipitation in mm (see the package documentation).
	RasterPrecipitation []float64
	// Precipitation holds, per cell, the annual precipitation in mm: the
	// mean of its samples' RasterPrecipitation.
	Precipitation []float64
	// Moisture holds, per cell, the mean share of the sea air's moisture
	// the prevailing winds bring to its samples, in (0, 1]; 1 on water.
	Moisture []float64
	// LiftM holds, per cell, the mean height in meters the winds climb
	// onto its samples from the lowest point within the lift window
	// upwind; 0 on water.
	LiftM []float64
	// PET holds, per cell, the potential evapotranspiration in mm per year
	// (Model.PET of its temperature).
	PET []float64
	// Runoff holds, per cell, the annual runoff in mm (Runoff of its
	// precipitation and PET).
	Runoff []float64
	// Aridity holds, per cell, the aridity index P/PET, capped at
	// AridityCap (AridityIndex); AridityOf gives its UNEP class.
	Aridity []float64
}

// Compute returns the climate of m's cells on the raster f (its grid and
// cylinder; its values are not read), with statistics s (altitude,
// latitude, and each sample's cell) and the sea level stage's land and
// water fl, under model, with world the world seed that keys the climate
// noise. The raster samples are spread over GOMAXPROCS goroutines; the
// result does not depend on how many. It fails when the inputs do not
// match or a result is not finite.
func Compute(f *field.Field, m *mesh.Mesh, s *cells.Stats, fl *cells.Flood, model Model, world uint64) (*Result, error) {
	return compute(f, m, s, fl, model, world, defaultWorkers())
}

func compute(f *field.Field, m *mesh.Mesh, s *cells.Stats, fl *cells.Flood, model Model, world uint64, workers int) (*Result, error) {
	if f == nil || m == nil || s == nil || fl == nil {
		return nil, errors.New("climate: nil field, mesh, statistics, or flood")
	}
	n := len(m.Cells)
	if s.Len() != n || len(s.Latitude) != n || len(s.Samples) != n || len(fl.Ocean) != n || len(fl.Land) != n {
		return nil, fmt.Errorf("climate: %d cells, %d statistics, %d ocean flags", n, s.Len(), len(fl.Ocean))
	}
	if len(s.Owner) != f.Len() {
		return nil, fmt.Errorf("climate: %d sample owners for %d raster samples", len(s.Owner), f.Len())
	}
	if len(model.latDeg) == 0 {
		return nil, errors.New("climate: zero Model")
	}
	r := &Result{
		Level:       fl.Level,
		Ocean:       make([]bool, n),
		Mask:        make([]bool, len(s.Owner)),
		HeightM:     make([]float64, n),
		Temperature: make([]float64, n),
	}
	for i, c := range m.Cells {
		r.Ocean[i] = c.Rim || fl.Ocean[i]
		if !c.Rim && fl.Land[i] && s.Altitude[i] > fl.Level {
			r.HeightM[i] = s.Altitude[i] - fl.Level
		}
		t := model.At(s.Latitude[i], r.HeightM[i])
		if !finite(t) {
			return nil, fmt.Errorf("climate: cell %d temperature %v is not finite", i, t)
		}
		r.Temperature[i] = t
	}
	for k, c := range s.Owner {
		if c < 0 || c >= n {
			return nil, fmt.Errorf("climate: sample %d owner %d out of range", k, c)
		}
		r.Mask[k] = r.Ocean[c]
	}
	g := newGrid(f, s.Owner, r)
	siteX, siteY := make([]float64, n), make([]float64, n)
	for i, c := range m.Cells {
		siteX[i], siteY[i] = c.Site.X, c.Site.Y
	}
	r.precipitate(f, g, siteX, siteY, s.Samples, model, world, workers)
	for _, vs := range [][]float64{r.RasterPrecipitation, r.Precipitation, r.Moisture, r.LiftM, r.PET, r.Runoff, r.Aridity} {
		for k, v := range vs {
			if !finite(v) || v < 0 {
				return nil, fmt.Errorf("climate: value %v at %d is not finite and non-negative", v, k)
			}
		}
	}
	return r, nil
}

// AppendBinary appends the result's canonical encoding to b; it is the
// input to the golden hashes. Integers are little-endian uint64; floats are
// their IEEE 754 bits (math.Float64bits) as little-endian uint64; flags are
// one byte, 0 or 1. In order:
//
//	number of cells, number of samples           integers
//	Level                                        float
//	per cell, in id order:
//	    Ocean                                    flag
//	    HeightM, Temperature                     floats
//	Mask, in sample storage order                flags
//	per cell, in id order:
//	    Precipitation, Moisture, LiftM,
//	    PET, Runoff, Aridity                     floats
//	RasterPrecipitation, in storage order        floats
//
// The first part is S25's encoding unchanged. The error is always nil; the
// signature is encoding.BinaryAppender's.
func (r *Result) AppendBinary(b []byte) ([]byte, error) {
	f := func(v float64) { b = binary.LittleEndian.AppendUint64(b, math.Float64bits(v)) }
	i := func(v int) { b = binary.LittleEndian.AppendUint64(b, uint64(int64(v))) }
	flag := func(v bool) {
		if v {
			b = append(b, 1)
		} else {
			b = append(b, 0)
		}
	}
	i(len(r.Temperature))
	i(len(r.Mask))
	f(r.Level)
	for c := range r.Temperature {
		flag(r.Ocean[c])
		f(r.HeightM[c])
		f(r.Temperature[c])
	}
	for _, v := range r.Mask {
		flag(v)
	}
	for c := range r.Precipitation {
		f(r.Precipitation[c])
		f(r.Moisture[c])
		f(r.LiftM[c])
		f(r.PET[c])
		f(r.Runoff[c])
		f(r.Aridity[c])
	}
	for _, v := range r.RasterPrecipitation {
		f(v)
	}
	return b, nil
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
