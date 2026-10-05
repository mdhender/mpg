// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package config

// Classify sets the landform and depth rules (DESIGN.md, "Classification";
// pipeline stage 11). The landform thresholds are rules about the ground,
// so they are in meters and km; the depth bands are rules about play, so
// they are in cell steps. The names follow hmz2ter's rules. See package
// classify.
type Classify struct {
	// A land cell's landform comes from its relief (p95 − p5, in meters):
	// flats below FlatsBelowM, plains below PlainsBelowM, rolling plains
	// below RollingPlainsBelowM, hills below HillsBelowM, and mountains
	// otherwise. The four must not decrease.
	FlatsBelowM         float64 `json:"flats_below_m"`
	PlainsBelowM        float64 `json:"plains_below_m"`
	RollingPlainsBelowM float64 `json:"rolling_plains_below_m"`
	HillsBelowM         float64 `json:"hills_below_m"`
	// A land cell at least PlateauMinAltitudeM above sea level with relief
	// below PlateauBelowM is plateaus, whatever its relief class.
	PlateauMinAltitudeM float64 `json:"plateau_min_altitude_m"`
	PlateauBelowM       float64 `json:"plateau_below_m"`
	// A plateaus cell whose site is within VolcanicRadiusKm of a volcano's
	// peak is volcanic highlands.
	VolcanicRadiusKm float64 `json:"volcanic_radius_km"`
	// Salt water up to ShallowMaxCells cell steps from land is shallow, up
	// to OpenMaxCells open, and farther deep.
	ShallowMaxCells int `json:"shallow_max_cells"`
	OpenMaxCells    int `json:"open_max_cells"`
}

// DefaultClassify returns the default landform and depth rules.
//
// They start from hmz2ter's (relief breaks 20, 60, 150 and 350 m; plateaus
// at 500 m and above with relief under 150 m; volcanic highlands within
// 25 km; shallow to 12 steps, open to 19), retuned for relief measured over
// a whole province of synthetic terrain (S21, seeds 1–8 at cinematic and
// square, and the archipelago and pangaea presets):
//
//   - flats below 35 m, not 20: the noise leaves about 30 m of relief
//     (the 5th percentile of land) in even the flattest province, so 20 m
//     gave under 1% flats; 35 m gives about 8%.
//   - plains below 65 m, not 60, to keep plains about a quarter of the land
//     after the flats took the low end.
//   - hills below 450 m, not 350: at 350 m mountains were about 14% of the
//     land and spread down the flanks of the ridged ranges; at 450 m they
//     are about 10%, the ranges' cores.
//
// Rolling plains (150 m), the plateau rule and the volcanic radius are
// hmz2ter's. The depth bands are play rules, retuned in cell steps:
// shallow up to 3 steps from land and open up to 8, not hmz2ter's 12 and
// 19, which made over half the salt water shallow (a band about 100 km
// wide) and left deep water only in the widest oceans.
func DefaultClassify() Classify {
	return Classify{
		FlatsBelowM:         35,
		PlainsBelowM:        65,
		RollingPlainsBelowM: 150,
		HillsBelowM:         450,
		PlateauMinAltitudeM: 500,
		PlateauBelowM:       150,
		VolcanicRadiusKm:    25,
		ShallowMaxCells:     3,
		OpenMaxCells:        8,
	}
}

// maxClassifyM bounds the landform thresholds in meters; it only rejects
// absurd values.
const maxClassifyM = 20_000

// validate appends the problems with the classification inputs through bad.
func (c *Classify) validate(bad func(format string, args ...any)) {
	inRange := func(name string, v, lo float64) {
		if !(v >= lo && v <= maxClassifyM) {
			bad("classify.%s %v must be in [%v, %d]", name, v, lo, maxClassifyM)
		}
	}
	inRange("flats_below_m", c.FlatsBelowM, 0)
	inRange("plains_below_m", c.PlainsBelowM, c.FlatsBelowM)
	inRange("rolling_plains_below_m", c.RollingPlainsBelowM, c.PlainsBelowM)
	inRange("hills_below_m", c.HillsBelowM, c.RollingPlainsBelowM)
	inRange("plateau_min_altitude_m", c.PlateauMinAltitudeM, 0)
	inRange("plateau_below_m", c.PlateauBelowM, 0)
	if v := c.VolcanicRadiusKm; !(v >= 0 && v <= 10_000) {
		bad("classify.volcanic_radius_km %v must be in [0, 10000]", v)
	}
	if v := c.ShallowMaxCells; v < 0 || v > 1000 {
		bad("classify.shallow_max_cells %d must be in [0, 1000]", v)
	}
	if v := c.OpenMaxCells; v < c.ShallowMaxCells || v > 1000 {
		bad("classify.open_max_cells %d must be in [shallow_max_cells %d, 1000]", v, c.ShallowMaxCells)
	}
}
