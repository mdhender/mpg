// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package config

import "math"

// Climate sets the temperature model (DESIGN.md: "Latitude plus lapse-rate
// temperature"; pipeline stage 7). A cell's mean annual temperature is the
// sea-level curve at its latitude minus LapseRateCPerKm for every km of its
// altitude above sea level. See package climate.
type Climate struct {
	// SeaLevelTempC is the mean annual temperature at sea level by latitude:
	// points (lat_deg, temp_c) joined by straight lines. Latitude is in
	// degrees from the equator, north and south alike, with the latitude
	// proxy's ±1 (the map's north and south edges, the true poles) at 90°.
	// The first point must be at 0°, the last at 90°, the latitudes must
	// increase, and the temperatures must not, and must fall overall, so
	// the equator is the warmest place at sea level and the poles (the rim)
	// the coldest.
	SeaLevelTempC []LatTemp `json:"sea_level_temp_c"`
	// LapseRateCPerKm is how much cooler it is per km of altitude above sea
	// level, in °C per km, in [0, 20].
	LapseRateCPerKm float64 `json:"lapse_rate_c_per_km"`
}

// LatTemp is one point of a temperature-by-latitude curve.
type LatTemp struct {
	// LatDeg is the latitude in degrees from the equator, in [0, 90].
	LatDeg float64 `json:"lat_deg"`
	// TempC is the temperature there in °C.
	TempC float64 `json:"temp_c"`
}

// Climate input bounds; they only reject absurd values.
const (
	// MaxCurvePoints bounds the number of points in Climate.SeaLevelTempC.
	MaxCurvePoints = 64
	// MaxAbsTempC bounds the curve's temperatures, in °C.
	MaxAbsTempC = 100
	// MaxLapseRateCPerKm bounds Climate.LapseRateCPerKm.
	MaxLapseRateCPerKm = 20
)

// DefaultClimate returns the default temperature model: hmz2bio's
// (/Users/wraith/Software/mdhender/tpty/hmz2bio/rules.go, DefaultRules), an
// Earth-like zonal mean annual sea-level temperature, 27 °C from the
// equator to 10°, falling through 20 °C at 30°, 8 °C at 50°, 0 °C at 60°
// and −16 °C at 80° to −22 °C at the pole, and the standard atmosphere's
// lapse rate, 6.5 °C per km.
func DefaultClimate() Climate {
	return Climate{
		SeaLevelTempC: []LatTemp{
			{0, 27}, {10, 27}, {15, 26.5}, {20, 25}, {25, 22.5}, {30, 20}, {35, 17}, {40, 14.5},
			{45, 12}, {50, 8}, {55, 4}, {60, 0}, {70, -8}, {80, -16}, {90, -22},
		},
		LapseRateCPerKm: 6.5,
	}
}

// validate appends the problems with the climate inputs through bad.
func (c *Climate) validate(bad func(format string, args ...any)) {
	pts := c.SeaLevelTempC
	switch n := len(pts); {
	case n < 2 || n > MaxCurvePoints:
		bad("climate.sea_level_temp_c has %d points; it needs 2 to %d", n, MaxCurvePoints)
	default:
		for k, p := range pts {
			if !(p.LatDeg >= 0 && p.LatDeg <= 90) {
				bad("climate.sea_level_temp_c[%d].lat_deg %v must be in [0, 90]", k, p.LatDeg)
			}
			if !(math.Abs(p.TempC) <= MaxAbsTempC) {
				bad("climate.sea_level_temp_c[%d].temp_c %v must be in [-%d, %d]", k, p.TempC, MaxAbsTempC, MaxAbsTempC)
			}
			if k == 0 {
				continue
			}
			if q := pts[k-1]; !(p.LatDeg > q.LatDeg) {
				bad("climate.sea_level_temp_c[%d].lat_deg %v must be greater than the previous point's %v", k, p.LatDeg, q.LatDeg)
			} else if !(p.TempC <= q.TempC) {
				bad("climate.sea_level_temp_c[%d].temp_c %v must not be warmer than the previous point's %v", k, p.TempC, q.TempC)
			}
		}
		if p := pts[0]; p.LatDeg != 0 {
			bad("climate.sea_level_temp_c[0].lat_deg %v must be 0", p.LatDeg)
		}
		if p := pts[n-1]; p.LatDeg != 90 {
			bad("climate.sea_level_temp_c[%d].lat_deg %v must be 90", n-1, p.LatDeg)
		}
		if !(pts[n-1].TempC < pts[0].TempC) {
			bad("climate.sea_level_temp_c: the pole (%v °C) must be colder than the equator (%v °C)", pts[n-1].TempC, pts[0].TempC)
		}
	}
	if v := c.LapseRateCPerKm; !(v >= 0 && v <= MaxLapseRateCPerKm) {
		bad("climate.lapse_rate_c_per_km %v must be in [0, %d]", v, MaxLapseRateCPerKm)
	}
}
