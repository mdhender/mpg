// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package config

import (
	"errors"
	"fmt"
	"math"
)

// Climate sets the climate model (pipeline stage 7): the temperature model
// (DESIGN.md: "Latitude plus lapse-rate temperature") and the stylized
// precipitation with prevailing winds and rain shadows, the potential
// evaporation, the runoff and the aridity built on it. See package climate
// for the model; the defaults are hmz2bio's
// (/Users/wraith/Software/mdhender/tpty/hmz2bio/rules.go, DefaultRules)
// except where a field says otherwise.
//
// Latitudes are in degrees from the equator, north and south alike, with
// the latitude proxy's ±1 (the map's north and south edges, the true
// poles) at 90°. Bearings are degrees clockwise from north and name the
// direction a wind blows from, as given for the northern hemisphere; the
// southern hemisphere mirrors them north–south (b → 180° − b).
type Climate struct {
	// SeaLevelTempC is the mean annual temperature at sea level by latitude:
	// points (lat_deg, temp_c) joined by straight lines. The first point
	// must be at 0°, the last at 90°, the latitudes must increase, and the
	// temperatures must not, and must fall overall, so the equator is the
	// warmest place at sea level and the poles (the rim) the coldest.
	SeaLevelTempC []LatTemp `json:"sea_level_temp_c"`
	// LapseRateCPerKm is how much cooler it is per km of altitude above sea
	// level, in °C per km, in [0, 20].
	LapseRateCPerKm float64 `json:"lapse_rate_c_per_km"`

	// WindwardPrecipMm is the annual precipitation on a windward coast at
	// sea level by latitude, in mm: points (lat_deg, mm) joined by straight
	// lines and held flat beyond the last. The first point must be at 0°,
	// the latitudes must increase, and every value must be in (0, 20000].
	WindwardPrecipMm []LatPrecip `json:"windward_precip_mm"`
	// ConvectiveShare is the share of WindwardPrecipMm that falls whatever
	// the wind brings (the tropical wet season's storms), by latitude:
	// points (lat_deg, share) as for WindwardPrecipMm, each share in
	// [0, 1].
	ConvectiveShare []LatShare `json:"convective_share"`

	// TradesFromDeg, WesterliesFromDeg and PolarFromDeg are the bearings the
	// trade winds, the westerlies and the polar easterlies blow from in the
	// northern hemisphere, each in [0, 360). PolarFromDeg is new in mpg
	// (hmz2bio's map stopped at 27°N); its default is the trades' 60°.
	TradesFromDeg     float64 `json:"trades_from_deg"`
	WesterliesFromDeg float64 `json:"westerlies_from_deg"`
	PolarFromDeg      float64 `json:"polar_from_deg"`
	// The wind bands, by latitude: the trades up to TradesMaxLatDeg, the
	// westerlies from WesterliesMinLatDeg to WesterliesMaxLatDeg, and the
	// polar easterlies from PolarMinLatDeg, each pair of neighbors blended
	// linearly in the gap between them. They must not decrease, in that
	// order, within [0, 90]. WesterliesMaxLatDeg and PolarMinLatDeg are new
	// in mpg.
	TradesMaxLatDeg     float64 `json:"trades_max_lat_deg"`
	WesterliesMinLatDeg float64 `json:"westerlies_min_lat_deg"`
	WesterliesMaxLatDeg float64 `json:"westerlies_max_lat_deg"`
	PolarMinLatDeg      float64 `json:"polar_min_lat_deg"`
	// EquatorBlendDeg is the half-width of the band about the equator, in
	// degrees of signed latitude, over which the northern and southern
	// hemispheres' winds are blended linearly, in [0, 45]. 0 switches
	// hemispheres at the equator. New in mpg: a hard switch left a seam
	// along the equator.
	EquatorBlendDeg float64 `json:"equator_blend_deg"`
	// WindRays is the number of rays traced for each wind, in [1, 32],
	// evenly spaced from WindSpreadDeg on one side of its bearing to
	// WindSpreadDeg on the other (one ray follows the bearing), in [0, 90].
	WindRays      int     `json:"wind_rays"`
	WindSpreadDeg float64 `json:"wind_spread_deg"`
	// StepKm is the length of one step of the upwind trace, in (0, 100],
	// and ReachKm the farthest it goes, in [StepKm, 20000], with at most
	// MaxTraceSteps steps.
	StepKm  float64 `json:"step_km"`
	ReachKm float64 `json:"reach_km"`
	// Each step keeps exp(−StepKm/RainoutKm − rise/OrographicM) of the
	// air's moisture, where rise is the height in meters the air gains on
	// the step. RainoutKm is in (0, 1e6] (default 500, tuned in S26 from
	// hmz2bio's 800) and OrographicM in (0, 1e6] (default 1200, from
	// hmz2bio's 1500).
	RainoutKm   float64 `json:"rainout_km"`
	OrographicM float64 `json:"orographic_m"`
	// Lift is a sample's height above the lowest point within LiftWindowKm
	// upwind, in [0, ReachKm]; precipitation is multiplied by 1 +
	// LiftGainPerKm × lift / 1000, LiftGainPerKm in [0, 100].
	LiftWindowKm  float64 `json:"lift_window_km"`
	LiftGainPerKm float64 `json:"lift_gain_per_km"`

	// Precipitation is multiplied by 1 + PrecipNoiseAmp × n, where n is
	// fractal noise in [−1, 1] on the cylinder: PrecipNoiseOctaves octaves
	// in [1, 16] from PrecipNoiseWavelengthKm, in (0, 1e6]. The amplitude
	// is in [0, 1), so precipitation stays positive. New in mpg.
	PrecipNoiseAmp          float64 `json:"precip_noise_amp"`
	PrecipNoiseWavelengthKm float64 `json:"precip_noise_wavelength_km"`
	PrecipNoiseOctaves      int     `json:"precip_noise_octaves"`
	// The latitude that picks the precipitation tables and the wind bands
	// is shifted by BandJitterDeg × n, in [0, 45], where n is fractal noise
	// with BandJitterOctaves octaves in [1, 16] from
	// BandJitterWavelengthKm, in (0, 1e6], so the bands waver with
	// longitude. Temperature is not shifted. New in mpg.
	BandJitterDeg          float64 `json:"band_jitter_deg"`
	BandJitterWavelengthKm float64 `json:"band_jitter_wavelength_km"`
	BandJitterOctaves      int     `json:"band_jitter_octaves"`

	// Potential evaporation follows Holdridge: PETMmPerC × the
	// biotemperature, which without seasons is the mean annual temperature
	// clamped to [0, BiotempMaxC]. PETMmPerC is in (0, 1000] (Holdridge's
	// 58.93 mm per °C), BiotempMaxC in (0, 100] (30 °C). New in mpg:
	// hmz2bio has no evaporation.
	PETMmPerC   float64 `json:"pet_mm_per_c"`
	BiotempMaxC float64 `json:"biotemp_max_c"`
}

// LatTemp is one point of a temperature-by-latitude curve.
type LatTemp struct {
	// LatDeg is the latitude in degrees from the equator, in [0, 90].
	LatDeg float64 `json:"lat_deg"`
	// TempC is the temperature there in °C.
	TempC float64 `json:"temp_c"`
}

// LatPrecip is one point of a precipitation-by-latitude table.
type LatPrecip struct {
	// LatDeg is the latitude in degrees from the equator, in [0, 90].
	LatDeg float64 `json:"lat_deg"`
	// Mm is the annual precipitation there in mm.
	Mm float64 `json:"mm"`
}

// LatShare is one point of a share-by-latitude table.
type LatShare struct {
	// LatDeg is the latitude in degrees from the equator, in [0, 90].
	LatDeg float64 `json:"lat_deg"`
	// Share is the share there, in [0, 1].
	Share float64 `json:"share"`
}

// Climate input bounds; they only reject absurd values.
const (
	// MaxCurvePoints bounds the number of points in each latitude table.
	MaxCurvePoints = 64
	// MaxAbsTempC bounds the curve's temperatures, in °C.
	MaxAbsTempC = 100
	// MaxLapseRateCPerKm bounds Climate.LapseRateCPerKm.
	MaxLapseRateCPerKm = 20
	// MaxPrecipMm bounds Climate.WindwardPrecipMm's values.
	MaxPrecipMm = 20_000
	// MaxWindRays bounds Climate.WindRays.
	MaxWindRays = 32
	// MaxTraceSteps bounds ReachKm / StepKm.
	MaxTraceSteps = 10_000
	// MaxNoiseOctaves bounds the noise octave counts (noise.MaxOctaves).
	MaxNoiseOctaves = 16
)

// DefaultClimate returns the default climate model.
//
// Temperature is hmz2bio's: an Earth-like zonal mean annual sea-level
// temperature, 27 °C from the equator to 10°, falling through 20 °C at 30°,
// 8 °C at 50°, 0 °C at 60° and −16 °C at 80° to −22 °C at the pole, and the
// standard atmosphere's lapse rate, 6.5 °C per km.
//
// Precipitation uses hmz2bio's windward-coast table, convective share,
// trades and westerlies, fan of rays, step, reach, lift window and lift
// gain. S26 measured real worlds and added the polar easterlies (from 60°
// beyond 63°, blended from 57°), the equator blend (±5°), shorter rainout
// (500 km) and orographic (1,200 m) lengths, so the lee sides and
// interiors dry (windward coasts ≈ 0.93–0.98 of the latitude's windward
// value, lee sides and interiors ≈ 0.33–0.49 on five worlds), and the
// periodic variability (±15% at 800 km, ±4° of band jitter at 2,000 km).
// PET is Holdridge's.
func DefaultClimate() Climate {
	return Climate{
		SeaLevelTempC: []LatTemp{
			{0, 27}, {10, 27}, {15, 26.5}, {20, 25}, {25, 22.5}, {30, 20}, {35, 17}, {40, 14.5},
			{45, 12}, {50, 8}, {55, 4}, {60, 0}, {70, -8}, {80, -16}, {90, -22},
		},
		LapseRateCPerKm: 6.5,
		WindwardPrecipMm: []LatPrecip{
			{0, 3100}, {5, 3300}, {10, 2700}, {15, 2000}, {20, 1400}, {25, 1000}, {30, 850}, {35, 1000},
			{40, 1250}, {45, 1400}, {50, 1400}, {55, 1250}, {60, 1000}, {70, 500}, {80, 250}, {90, 150},
		},
		ConvectiveShare:         []LatShare{{0, 0.5}, {10, 0.45}, {20, 0.3}, {30, 0.2}},
		TradesFromDeg:           60,
		WesterliesFromDeg:       255,
		PolarFromDeg:            60,
		TradesMaxLatDeg:         27,
		WesterliesMinLatDeg:     33,
		WesterliesMaxLatDeg:     57,
		PolarMinLatDeg:          63,
		EquatorBlendDeg:         5,
		WindRays:                5,
		WindSpreadDeg:           20,
		StepKm:                  5,
		ReachKm:                 1000,
		RainoutKm:               500,
		OrographicM:             1200,
		LiftWindowKm:            30,
		LiftGainPerKm:           1.5,
		PrecipNoiseAmp:          0.15,
		PrecipNoiseWavelengthKm: 800,
		PrecipNoiseOctaves:      4,
		BandJitterDeg:           4,
		BandJitterWavelengthKm:  2000,
		BandJitterOctaves:       3,
		PETMmPerC:               58.93,
		BiotempMaxC:             30,
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

	lats := make([]float64, len(c.WindwardPrecipMm))
	for k, p := range c.WindwardPrecipMm {
		lats[k] = p.LatDeg
		if !(p.Mm > 0 && p.Mm <= MaxPrecipMm) {
			bad("climate.windward_precip_mm[%d].mm %v must be in (0, %d]", k, p.Mm, MaxPrecipMm)
		}
	}
	validateLatitudes("climate.windward_precip_mm", lats, bad)
	lats = make([]float64, len(c.ConvectiveShare))
	for k, p := range c.ConvectiveShare {
		lats[k] = p.LatDeg
		if !(p.Share >= 0 && p.Share <= 1) {
			bad("climate.convective_share[%d].share %v must be in [0, 1]", k, p.Share)
		}
	}
	validateLatitudes("climate.convective_share", lats, bad)

	for _, b := range []struct {
		name string
		v    float64
	}{{"trades_from_deg", c.TradesFromDeg}, {"westerlies_from_deg", c.WesterliesFromDeg}, {"polar_from_deg", c.PolarFromDeg}} {
		if !(b.v >= 0 && b.v < 360) {
			bad("climate.%s %v must be in [0, 360)", b.name, b.v)
		}
	}
	bands := []struct {
		name string
		v    float64
	}{{"trades_max_lat_deg", c.TradesMaxLatDeg}, {"westerlies_min_lat_deg", c.WesterliesMinLatDeg}, {"westerlies_max_lat_deg", c.WesterliesMaxLatDeg}, {"polar_min_lat_deg", c.PolarMinLatDeg}}
	for k, b := range bands {
		if !(b.v >= 0 && b.v <= 90) {
			bad("climate.%s %v must be in [0, 90]", b.name, b.v)
		} else if k > 0 && !(b.v >= bands[k-1].v) {
			bad("climate.%s %v must not be less than climate.%s %v", b.name, b.v, bands[k-1].name, bands[k-1].v)
		}
	}
	if v := c.EquatorBlendDeg; !(v >= 0 && v <= 45) {
		bad("climate.equator_blend_deg %v must be in [0, 45]", v)
	}
	if v := c.WindRays; v < 1 || v > MaxWindRays {
		bad("climate.wind_rays %d must be in [1, %d]", v, MaxWindRays)
	}
	if v := c.WindSpreadDeg; !(v >= 0 && v <= 90) {
		bad("climate.wind_spread_deg %v must be in [0, 90]", v)
	}
	if v := c.StepKm; !(v > 0 && v <= 100) {
		bad("climate.step_km %v must be in (0, 100]", v)
	} else if r := c.ReachKm; !(r >= v && r <= 20_000) {
		bad("climate.reach_km %v must be in [step_km %v, 20000]", r, v)
	} else if !(r/v <= MaxTraceSteps) {
		bad("climate.reach_km %v / step_km %v must be at most %d steps", r, v, MaxTraceSteps)
	}
	if v := c.RainoutKm; !(v > 0 && v <= 1e6) {
		bad("climate.rainout_km %v must be in (0, 1e6]", v)
	}
	if v := c.OrographicM; !(v > 0 && v <= 1e6) {
		bad("climate.orographic_m %v must be in (0, 1e6]", v)
	}
	if v := c.LiftWindowKm; !(v >= 0 && v <= c.ReachKm) {
		bad("climate.lift_window_km %v must be in [0, reach_km %v]", v, c.ReachKm)
	}
	if v := c.LiftGainPerKm; !(v >= 0 && v <= 100) {
		bad("climate.lift_gain_per_km %v must be in [0, 100]", v)
	}
	if v := c.PrecipNoiseAmp; !(v >= 0 && v < 1) {
		bad("climate.precip_noise_amp %v must be in [0, 1)", v)
	}
	if v := c.BandJitterDeg; !(v >= 0 && v <= 45) {
		bad("climate.band_jitter_deg %v must be in [0, 45]", v)
	}
	for _, w := range []struct {
		name string
		v    float64
	}{{"precip_noise_wavelength_km", c.PrecipNoiseWavelengthKm}, {"band_jitter_wavelength_km", c.BandJitterWavelengthKm}} {
		if !(w.v > 0 && w.v <= 1e6) {
			bad("climate.%s %v must be in (0, 1e6]", w.name, w.v)
		}
	}
	for _, o := range []struct {
		name string
		v    int
	}{{"precip_noise_octaves", c.PrecipNoiseOctaves}, {"band_jitter_octaves", c.BandJitterOctaves}} {
		if o.v < 1 || o.v > MaxNoiseOctaves {
			bad("climate.%s %d must be in [1, %d]", o.name, o.v, MaxNoiseOctaves)
		}
	}
	if v := c.PETMmPerC; !(v > 0 && v <= 1000) {
		bad("climate.pet_mm_per_c %v must be in (0, 1000]", v)
	}
	if v := c.BiotempMaxC; !(v > 0 && v <= MaxAbsTempC) {
		bad("climate.biotemp_max_c %v must be in (0, %d]", v, MaxAbsTempC)
	}
}

// validateLatitudes checks a latitude table's latitudes: 1 to
// MaxCurvePoints points, each in [0, 90], the first at 0, increasing.
func validateLatitudes(name string, lats []float64, bad func(format string, args ...any)) {
	if n := len(lats); n < 1 || n > MaxCurvePoints {
		bad("%s has %d points; it needs 1 to %d", name, n, MaxCurvePoints)
		return
	}
	for k, v := range lats {
		switch {
		case !(v >= 0 && v <= 90):
			bad("%s[%d].lat_deg %v must be in [0, 90]", name, k, v)
		case k > 0 && !(v > lats[k-1]):
			bad("%s[%d].lat_deg %v must be greater than the previous point's %v", name, k, v, lats[k-1])
		}
	}
	if lats[0] != 0 {
		bad("%s[0].lat_deg %v must be 0", name, lats[0])
	}
}

// Validate reports every problem with the climate inputs, joined, or nil.
// Config.Validate includes it; package climate calls it to check a model's
// inputs on their own.
func (c *Climate) Validate() error {
	var errs []error
	c.validate(func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) })
	return errors.Join(errs...)
}
