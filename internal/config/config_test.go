// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package config

import (
	"bytes"
	"flag"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mdhender/mpg/internal/topo"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata")

// example returns the design's example, N = 10,000 and f = 0.30 at cinematic
// aspect, resolved with seed 42.
func example(t *testing.T) Config {
	t.Helper()
	c := Default()
	c.Seed = 42
	c.World.LandCells = 10_000
	c.World.LandFraction = 0.30
	c.World.Aspect = "cinematic"
	if err := c.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return c
}

func encode(t *testing.T, c Config) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := c.Encode(&buf); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	return buf.Bytes()
}

func near(got, want, tol float64) bool { return math.Abs(got-want) <= tol }

func TestDesignExample(t *testing.T) {
	c := example(t)
	if !near(c.Province.AreaKm2, 80.75, 0.01) {
		t.Errorf("area = %v km², want about 80.75", c.Province.AreaKm2)
	}
	if c.World.PlayableCells != 33_333 {
		t.Errorf("playable cells = %d, want 33333", c.World.PlayableCells)
	}
	if !near(c.World.PlayableAreaKm2, 2.69e6, 0.01e6) {
		t.Errorf("playable area = %v km², want about 2.69M", c.World.PlayableAreaKm2)
	}
	if !near(c.World.WidthKm, 2540, 10) {
		t.Errorf("width = %v km, want about 2,540", c.World.WidthKm)
	}
	playableHeight := c.World.HeightKm - 2*c.Rim.Km
	if !near(playableHeight, 1060, 5) {
		t.Errorf("height before rim = %v km, want about 1,060", playableHeight)
	}
	cell := math.Sqrt(c.Province.AreaKm2)
	if !near(cell, 8.99, 0.01) {
		t.Errorf("√A = %v km, want about 8.99", cell)
	}
	if !near(c.Rim.Km, 4*cell, 1e-9) || !near(c.Rim.FalloffKm, 12*cell, 1e-9) {
		t.Errorf("rim %v km, falloff %v km; want 4 and 12 × √A", c.Rim.Km, c.Rim.FalloffKm)
	}
	if !near(c.World.WidthKm/playableHeight, 2.39, 1e-9) {
		t.Errorf("aspect = %v, want 2.39", c.World.WidthKm/playableHeight)
	}
	if !near(c.World.WidthKm*playableHeight, c.World.PlayableAreaKm2, 1e-6) {
		t.Errorf("width × playable height = %v, want playable area %v", c.World.WidthKm*playableHeight, c.World.PlayableAreaKm2)
	}

	if !near(c.Mesh.MinEdgeKm, 0.3*cell, 1e-12) || !near(c.Mesh.MinEdgeKm, 2.7, 0.01) {
		t.Errorf("mesh.min_edge_km = %v, want 0.3 × √A, about 2.7", c.Mesh.MinEdgeKm)
	}

	// The exact bits are pinned by the golden file.
	got := encode(t, c)
	golden := filepath.Join("testdata", "example.json")
	if *update {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("encoded example differs from %s:\n%s", golden, got)
	}
}

func TestDefaultIsExample(t *testing.T) {
	c := Default()
	c.Seed = 42
	if err := c.Resolve(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c, example(t)) {
		t.Errorf("Default() resolved = %+v, want the design example", c)
	}
}

func TestResolvedSizesPassTopo(t *testing.T) {
	for _, aspect := range []string{"square", "portrait", "landscape", "widescreen", "cinematic", "4:1", "1:3"} {
		for _, n := range []int{2_000, 10_000, 60_000} {
			c := Default()
			c.World.Aspect = aspect
			c.World.LandCells = n
			if err := c.Resolve(); err != nil {
				t.Errorf("%s, N=%d: %v", aspect, n, err)
				continue
			}
			cyl, err := topo.New(c.World.WidthKm, c.World.HeightKm, c.Rim.Km, c.Rim.FalloffKm)
			if err != nil {
				t.Errorf("%s, N=%d: topo.New: %v", aspect, n, err)
				continue
			}
			if cyl.W() != c.World.WidthKm || cyl.H() != c.World.HeightKm {
				t.Errorf("%s, N=%d: cylinder %v × %v", aspect, n, cyl.W(), cyl.H())
			}
		}
	}
}

func TestRoundTripFixedPoint(t *testing.T) {
	first := encode(t, example(t))
	c, err := Decode(bytes.NewReader(first))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if err := c.Resolve(); err != nil {
		t.Fatalf("re-Resolve: %v", err)
	}
	second := encode(t, c)
	if !bytes.Equal(first, second) {
		t.Errorf("round trip changed the file:\n%s\n---\n%s", first, second)
	}
	if !reflect.DeepEqual(c, example(t)) {
		t.Errorf("round trip changed the config")
	}
}

func TestPartialFileGetsDefaults(t *testing.T) {
	c, err := Decode(strings.NewReader(`{"schema": 1, "seed": "42"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c, example(t)) {
		t.Errorf("partial file resolved to %+v", c)
	}
}

// TestClimatePartial checks that a partial climate group keeps the default
// curve, that a curve in the file replaces the default one whole, and that
// a misspelled point field is rejected with its path; and the same for the
// precipitation settings: a windward table replaces the default whole, the
// other settings keep their defaults, and a misspelled or out-of-range
// setting is rejected.
func TestClimatePartial(t *testing.T) {
	c, err := Decode(strings.NewReader(`{"schema": 1, "climate": {"lapse_rate_c_per_km": 5}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.Climate.SeaLevelTempC, DefaultClimate().SeaLevelTempC) || c.Climate.LapseRateCPerKm != 5 {
		t.Errorf("climate = %+v", c.Climate)
	}
	c, err = Decode(strings.NewReader(`{"schema": 1, "climate": {"sea_level_temp_c": [{"lat_deg": 0, "temp_c": 30}, {"lat_deg": 90, "temp_c": -30}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if want := []LatTemp{{0, 30}, {90, -30}}; !reflect.DeepEqual(c.Climate.SeaLevelTempC, want) || c.Climate.LapseRateCPerKm != 6.5 {
		t.Errorf("climate = %+v, want curve %v and the default lapse rate", c.Climate, want)
	}
	_, err = Decode(strings.NewReader(`{"schema": 1, "climate": {"sea_level_temp_c": [{"lat_deg": 0, "temp": 30}, {"lat_deg": 90, "temp_c": -30}]}}`))
	if err == nil || !strings.Contains(err.Error(), "temp") {
		t.Errorf("misspelled curve field: %v", err)
	}
	c, err = Decode(strings.NewReader(`{"schema": 1, "climate": {"rainout_km": 700, "windward_precip_mm": [{"lat_deg": 0, "mm": 900}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if want := []LatPrecip{{0, 900}}; !reflect.DeepEqual(c.Climate.WindwardPrecipMm, want) || c.Climate.RainoutKm != 700 ||
		!reflect.DeepEqual(c.Climate.ConvectiveShare, DefaultClimate().ConvectiveShare) || c.Climate.OrographicM != 1200 {
		t.Errorf("climate = %+v, want the windward table %v, rainout 700 and the other defaults", c.Climate, want)
	}
	_, err = Decode(strings.NewReader(`{"schema": 1, "climate": {"rainout": 700}}`))
	if err == nil || !strings.Contains(err.Error(), "rainout") {
		t.Errorf("misspelled climate field: %v", err)
	}
	_, err = Decode(strings.NewReader(`{"schema": 1, "climate": {"precip_noise_amp": 1.5}}`))
	if err == nil || !strings.Contains(err.Error(), "precip_noise_amp") {
		t.Errorf("out-of-range noise amplitude: %v", err)
	}
}

func TestHash(t *testing.T) {
	c := example(t)
	h, err := c.Hash()
	if err != nil {
		t.Fatal(err)
	}
	const want = "a94d88cd17f4c88ac481456ea950c621d3a2a274446397056770cee7dddbe1e3"
	if h != want {
		t.Errorf("Hash = %s, want %s", h, want)
	}
	c.Seed = 43
	if h2, _ := c.Hash(); h2 == h {
		t.Error("hash ignores the seed")
	}
}

func TestEncodeUnresolved(t *testing.T) {
	c := Default()
	if err := c.Encode(&bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "not resolved") {
		t.Errorf("Encode(unresolved) = %v, want a not-resolved error", err)
	}
}

func TestDecodeErrors(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"misspelled", `{"schema":1,"world":{"land_fracton":0.3}}`, `unknown field "world.land_fracton"; did you mean "world.land_fraction"?`},
		{"misspelled top", `{"schema":1,"provinces":{}}`, `unknown field "provinces"; did you mean "province"?`},
		{"unknown far", `{"schema":1,"zzzzzzzzzzzz":1}`, `unknown field "zzzzzzzzzzzz"`},
		{"duplicate", `{"schema":1,"seed":"1","seed":"2"}`, `duplicate field "seed"`},
		{"no schema", `{"seed":"1"}`, `missing "schema"`},
		{"schema 2", `{"schema":2,"newfield":1}`, `schema 2 is not supported`},
		{"schema 0", `{"schema":0}`, `schema 0 is not supported`},
		{"schema string", `{"schema":"1"}`, `schema "1" is not supported`},
		{"trailing", `{"schema":1} {}`, `after top-level value`},
		{"trailing junk", `{"schema":1} x`, `invalid character`},
		{"truncated", `{"schema":1,`, `unexpected end`},
		{"not object", `[1]`, `cannot unmarshal`},
		{"seed number", `{"schema":1,"seed":42}`, `seed must be a decimal string`},
		{"seed null", `{"schema":1,"seed":null}`, `seed must be a decimal string`},
		{"seed negative", `{"schema":1,"seed":"-1"}`, `seed "-1" must be a decimal integer`},
		{"seed leading zero", `{"schema":1,"seed":"007"}`, `without sign or leading zeros`},
		{"seed overflow", `{"schema":1,"seed":"18446744073709551616"}`, `must be a decimal integer`},
		{"wrong type", `{"schema":1,"world":{"land_cells":"x"}}`, `cannot unmarshal string`},
		{"inconsistent", `{"schema":1,"world":{"land_cells":20000,"width_km":2536.3057312095275}}`, `world.width_km is 2536.3057312095275 but the inputs give`},
		{"invalid input", `{"schema":1,"world":{"land_fraction":1}}`, `world.land_fraction 1 must be greater than 0 and less than 1`},
		{"misspelled layout", `{"schema":1,"layout":{"islands":{"lobes_mx":2}}}`, `unknown field "layout.islands.lobes_mx"; did you mean "layout.islands.lobes_max"?`},
		{"mesh jitter", `{"schema":1,"mesh":{"jitter":1}}`, `mesh.jitter 1 must be in [0, 1)`},
		{"mesh degree cap", `{"schema":1,"mesh":{"degree_cap":9}}`, `mesh.degree_cap 9 must be in [3, 8]`},
		{"mesh placement", `{"schema":1,"mesh":{"placement":"poisson"}}`, `mesh.placement "poisson" must be "jittered-grid"`},
		{"mesh area bounds", `{"schema":1,"mesh":{"area_min":0,"area_max":0.9}}`, `mesh.area_max 0.9 must be in [1, 10]`},
		{"mesh lloyd", `{"schema":1,"mesh":{"lloyd_passes":-1}}`, `mesh.lloyd_passes -1 must be in [0, 10]`},
		{"mesh min edge derived", `{"schema":1,"mesh":{"min_edge_km":2.7}}`, `mesh.min_edge_km is 2.7 but the inputs give`},
		{"classify unknown field", `{"schema":1,"classify":{"hills_below":400}}`, `hills_below`},
		{"unknown custom field", `{"schema":1,"layout":{"custom":{"attractors":[{"x_km":1,"radius_kn":5}]}}}`, `unknown field "layout.custom.attractors[0].radius_kn"; did you mean "layout.custom.attractors[0].radius_km"?`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Decode(strings.NewReader(tc.in))
			if err == nil {
				t.Fatalf("Decode(%s) succeeded", tc.in)
			}
			if !strings.Contains(err.Error(), tc.want) || !strings.HasPrefix(err.Error(), "config: ") {
				t.Errorf("Decode(%s) = %q, want it to contain %q", tc.in, err, tc.want)
			}
		})
	}
}

func TestSeedRoundTrip(t *testing.T) {
	for _, s := range []Seed{0, 1, 42, math.MaxUint64} {
		c := example(t)
		c.Seed = s
		got, err := Decode(bytes.NewReader(encode(t, c)))
		if err != nil {
			t.Fatalf("seed %d: %v", uint64(s), err)
		}
		if got.Seed != s {
			t.Errorf("seed %d round-tripped to %d", uint64(s), uint64(got.Seed))
		}
	}
	c := example(t)
	c.Seed = math.MaxUint64
	if b := encode(t, c); !bytes.Contains(b, []byte(`"seed": "18446744073709551615"`)) {
		t.Errorf("max seed encoded as:\n%s", b)
	}
}

func TestParseAspect(t *testing.T) {
	good := []struct {
		in   string
		want float64
	}{
		{"square", 1},
		{"portrait", 2.0 / 3},
		{"landscape", 1.5},
		{"widescreen", 16.0 / 9},
		{"cinematic", 2.39},
		{"16:9", 16.0 / 9},
		{"2.39:1", 2.39},
		{"1:1", 1},
		{"1:2.5", 0.4},
		{"1e1:5", 2},
	}
	for _, tc := range good {
		got, err := ParseAspect(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("ParseAspect(%q) = %v, %v; want %v", tc.in, got, err, tc.want)
		}
	}
	bad := []string{
		"", "abc", "Square", " square", "16x9", "1:", ":1", "1:2:3",
		"0:1", "1:0", "-1:2", "2:-1", "inf:1", "1:Inf", "NaN:1", "1:nan",
		"1e400:1", "1e300:1e-300", "a:b",
	}
	for _, in := range bad {
		if got, err := ParseAspect(in); err == nil {
			t.Errorf("ParseAspect(%q) = %v, want an error", in, got)
		}
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Config)
		want string
	}{
		{"f zero", func(c *Config) { c.World.LandFraction = 0 }, "world.land_fraction"},
		{"f one", func(c *Config) { c.World.LandFraction = 1 }, "world.land_fraction"},
		{"f negative", func(c *Config) { c.World.LandFraction = -0.3 }, "world.land_fraction"},
		{"f NaN", func(c *Config) { c.World.LandFraction = math.NaN() }, "world.land_fraction"},
		{"N zero", func(c *Config) { c.World.LandCells = 0 }, "world.land_cells"},
		{"N negative", func(c *Config) { c.World.LandCells = -5 }, "world.land_cells"},
		{"N huge", func(c *Config) { c.World.LandCells = MaxLandCells + 1 }, "world.land_cells"},
		{"aspect", func(c *Config) { c.World.Aspect = "0:1" }, "world.aspect"},
		{"hex zero", func(c *Config) { c.Province.HexFlatToFlatMi = 0 }, "province.hex_flat_to_flat_mi"},
		{"hex inf", func(c *Config) { c.Province.HexFlatToFlatMi = math.Inf(1) }, "province.hex_flat_to_flat_mi"},
		{"hex NaN", func(c *Config) { c.Province.HexFlatToFlatMi = math.NaN() }, "province.hex_flat_to_flat_mi"},
		{"spacing zero", func(c *Config) { c.Raster.SpacingKm = 0 }, "raster.spacing_km"},
		{"spacing negative", func(c *Config) { c.Raster.SpacingKm = -2 }, "raster.spacing_km"},
		{"spacing inf", func(c *Config) { c.Raster.SpacingKm = math.Inf(1) }, "raster.spacing_km"},
		{"spacing too coarse", func(c *Config) { c.Raster.SpacingKm = 9 }, "raster.spacing_km 9 must be less than the province side"},
		{"rim zero", func(c *Config) { c.Rim.Cells = 0 }, "rim.cells"},
		{"falloff negative", func(c *Config) { c.Rim.FalloffCells = -1 }, "rim.falloff_cells"},
		{"falloff too wide", func(c *Config) { c.World.LandCells = 30; c.Rim.FalloffCells = 12 }, "falloff bands"},
		{"hotspots negative", func(c *Config) { c.Volcanic.HotspotsPerMkm2 = -1 }, "volcanic.hotspots_per_mkm2"},
		{"hotspots huge", func(c *Config) { c.Volcanic.HotspotsPerMkm2 = 1001 }, "volcanic.hotspots_per_mkm2"},
		{"hotspots NaN", func(c *Config) { c.Volcanic.HotspotsPerMkm2 = math.NaN() }, "volcanic.hotspots_per_mkm2"},
		{"cone peak negative", func(c *Config) { c.Volcanic.ConePeakMinM = -1 }, "volcanic.cone_peak_min_m"},
		{"cone peaks reversed", func(c *Config) { c.Volcanic.ConePeakMaxM = 1000 }, "volcanic.cone_peak_max_m"},
		{"cone radius zero", func(c *Config) { c.Volcanic.ConeRadiusMinKm = 0 }, "volcanic.cone_radius_min_km"},
		{"cone radii reversed", func(c *Config) { c.Volcanic.ConeRadiusMaxKm = 10 }, "volcanic.cone_radius_max_km"},
		{"swell negative", func(c *Config) { c.Volcanic.SwellM = -1 }, "volcanic.swell_m"},
		{"swell radius zero", func(c *Config) { c.Volcanic.SwellRadiusKm = 0 }, "volcanic.swell_radius_km"},
		{"basin depth NaN", func(c *Config) { c.Basin.MinDepthM = math.NaN() }, "basin.min_depth_m"},
		{"inland sea 1", func(c *Config) { c.Basin.InlandSeaMinCells = 1 }, "basin.inland_sea_min_cells"},
		{"river zero", func(c *Config) { c.River.ThresholdKm2 = 0 }, "river.threshold_km2"},
		{"classify flats negative", func(c *Config) { c.Classify.FlatsBelowM = -1 }, "classify.flats_below_m"},
		{"classify breaks decrease", func(c *Config) { c.Classify.HillsBelowM = 100 }, "classify.hills_below_m 100 must be in [150"},
		{"classify plains NaN", func(c *Config) { c.Classify.PlainsBelowM = math.NaN() }, "classify.plains_below_m"},
		{"classify plateau", func(c *Config) { c.Classify.PlateauMinAltitudeM = math.Inf(1) }, "classify.plateau_min_altitude_m"},
		{"classify plateau relief", func(c *Config) { c.Classify.PlateauBelowM = -5 }, "classify.plateau_below_m"},
		{"classify volcanic radius", func(c *Config) { c.Classify.VolcanicRadiusKm = -1 }, "classify.volcanic_radius_km"},
		{"classify shallow", func(c *Config) { c.Classify.ShallowMaxCells = -1 }, "classify.shallow_max_cells"},
		{"classify open below shallow", func(c *Config) { c.Classify.OpenMaxCells = 2 }, "classify.open_max_cells 2 must be in [shallow_max_cells 3"},
		{"climate one point", func(c *Config) { c.Climate.SeaLevelTempC = []LatTemp{{0, 20}} }, "climate.sea_level_temp_c has 1 points"},
		{"climate no equator", func(c *Config) { c.Climate.SeaLevelTempC = []LatTemp{{5, 20}, {90, -20}} }, "climate.sea_level_temp_c[0].lat_deg 5 must be 0"},
		{"climate no pole", func(c *Config) { c.Climate.SeaLevelTempC = []LatTemp{{0, 20}, {80, -20}} }, "climate.sea_level_temp_c[1].lat_deg 80 must be 90"},
		{"climate lat order", func(c *Config) { c.Climate.SeaLevelTempC = []LatTemp{{0, 20}, {50, 0}, {50, -5}, {90, -20}} }, "climate.sea_level_temp_c[2].lat_deg 50 must be greater"},
		{"climate lat range", func(c *Config) { c.Climate.SeaLevelTempC = []LatTemp{{0, 20}, {95, -20}} }, "climate.sea_level_temp_c[1].lat_deg 95 must be in [0, 90]"},
		{"climate warmer poleward", func(c *Config) { c.Climate.SeaLevelTempC = []LatTemp{{0, 20}, {40, 22}, {90, -20}} }, "climate.sea_level_temp_c[1].temp_c 22 must not be warmer"},
		{"climate flat", func(c *Config) { c.Climate.SeaLevelTempC = []LatTemp{{0, 10}, {90, 10}} }, "the pole (10 °C) must be colder than the equator"},
		{"climate temp NaN", func(c *Config) { c.Climate.SeaLevelTempC = []LatTemp{{0, math.NaN()}, {90, -20}} }, "climate.sea_level_temp_c[0].temp_c NaN"},
		{"climate temp huge", func(c *Config) { c.Climate.SeaLevelTempC = []LatTemp{{0, 200}, {90, -20}} }, "climate.sea_level_temp_c[0].temp_c 200"},
		{"climate lapse negative", func(c *Config) { c.Climate.LapseRateCPerKm = -1 }, "climate.lapse_rate_c_per_km"},
		{"climate lapse NaN", func(c *Config) { c.Climate.LapseRateCPerKm = math.NaN() }, "climate.lapse_rate_c_per_km"},
		{"windward empty", func(c *Config) { c.Climate.WindwardPrecipMm = nil }, "climate.windward_precip_mm has 0 points"},
		{"windward zero", func(c *Config) { c.Climate.WindwardPrecipMm[2].Mm = 0 }, "climate.windward_precip_mm[2].mm 0 must be in (0, 20000]"},
		{"windward NaN", func(c *Config) { c.Climate.WindwardPrecipMm[2].Mm = math.NaN() }, "climate.windward_precip_mm[2].mm NaN"},
		{"windward no equator", func(c *Config) { c.Climate.WindwardPrecipMm[0].LatDeg = 1 }, "climate.windward_precip_mm[0].lat_deg 1 must be 0"},
		{"windward lat order", func(c *Config) { c.Climate.WindwardPrecipMm[3].LatDeg = 5 }, "climate.windward_precip_mm[3].lat_deg 5 must be greater"},
		{"share above 1", func(c *Config) { c.Climate.ConvectiveShare[1].Share = 1.5 }, "climate.convective_share[1].share 1.5 must be in [0, 1]"},
		{"share lat range", func(c *Config) { c.Climate.ConvectiveShare[3].LatDeg = 95 }, "climate.convective_share[3].lat_deg 95 must be in [0, 90]"},
		{"trades bearing", func(c *Config) { c.Climate.TradesFromDeg = 360 }, "climate.trades_from_deg 360 must be in [0, 360)"},
		{"polar bearing NaN", func(c *Config) { c.Climate.PolarFromDeg = math.NaN() }, "climate.polar_from_deg NaN"},
		{"bands order", func(c *Config) { c.Climate.WesterliesMinLatDeg = 20 }, "climate.westerlies_min_lat_deg 20 must not be less than climate.trades_max_lat_deg 27"},
		{"bands range", func(c *Config) { c.Climate.PolarMinLatDeg = 91 }, "climate.polar_min_lat_deg 91 must be in [0, 90]"},
		{"equator blend", func(c *Config) { c.Climate.EquatorBlendDeg = -1 }, "climate.equator_blend_deg -1"},
		{"wind rays", func(c *Config) { c.Climate.WindRays = 0 }, "climate.wind_rays 0 must be in [1, 32]"},
		{"wind spread", func(c *Config) { c.Climate.WindSpreadDeg = 91 }, "climate.wind_spread_deg 91"},
		{"step zero", func(c *Config) { c.Climate.StepKm = 0 }, "climate.step_km 0 must be in (0, 100]"},
		{"reach below step", func(c *Config) { c.Climate.ReachKm = 1 }, "climate.reach_km 1 must be in [step_km 5, 20000]"},
		{"too many steps", func(c *Config) { c.Climate.StepKm, c.Climate.ReachKm = 0.5, 20000 }, "must be at most 10000 steps"},
		{"rainout", func(c *Config) { c.Climate.RainoutKm = 0 }, "climate.rainout_km 0"},
		{"orographic", func(c *Config) { c.Climate.OrographicM = math.Inf(1) }, "climate.orographic_m +Inf"},
		{"lift window", func(c *Config) { c.Climate.LiftWindowKm = 2000 }, "climate.lift_window_km 2000 must be in [0, reach_km 1000]"},
		{"lift gain", func(c *Config) { c.Climate.LiftGainPerKm = -1 }, "climate.lift_gain_per_km -1"},
		{"noise amp 1", func(c *Config) { c.Climate.PrecipNoiseAmp = 1 }, "climate.precip_noise_amp 1 must be in [0, 1)"},
		{"noise wavelength", func(c *Config) { c.Climate.PrecipNoiseWavelengthKm = 0 }, "climate.precip_noise_wavelength_km 0"},
		{"noise octaves", func(c *Config) { c.Climate.PrecipNoiseOctaves = 17 }, "climate.precip_noise_octaves 17 must be in [1, 16]"},
		{"jitter", func(c *Config) { c.Climate.BandJitterDeg = 50 }, "climate.band_jitter_deg 50"},
		{"jitter wavelength", func(c *Config) { c.Climate.BandJitterWavelengthKm = -5 }, "climate.band_jitter_wavelength_km -5"},
		{"jitter octaves", func(c *Config) { c.Climate.BandJitterOctaves = 0 }, "climate.band_jitter_octaves 0"},
		{"pet", func(c *Config) { c.Climate.PETMmPerC = 0 }, "climate.pet_mm_per_c 0"},
		{"biotemp", func(c *Config) { c.Climate.BiotempMaxC = math.NaN() }, "climate.biotemp_max_c NaN"},
		{"schema", func(c *Config) { c.Schema = 2 }, "schema 2 is not supported"},
		{"preset", func(c *Config) { c.Layout.Preset = "isles" }, "layout.preset"},
		{"pole margin", func(c *Config) { c.Layout.PoleMargin = -1 }, "layout.pole_margin"},
		{"masses reversed", func(c *Config) { c.Layout.Continents.MassesMax = 2 }, "layout.continents.masses_min 3 and masses_max 2"},
		{"lobes zero", func(c *Config) { c.Layout.Islands.LobesMin = 0 }, "layout.islands.lobes_min"},
		{"coverage", func(c *Config) { c.Layout.Pangaea.Coverage = 0 }, "layout.pangaea.coverage"},
		{"size ratio", func(c *Config) { c.Layout.Archipelago.SizeRatio = 0.5 }, "layout.archipelago.size_ratio"},
		{"weights", func(c *Config) { c.Layout.Continents.WeightMax = 1.5 }, "layout.continents.weight_min"},
		{"wobble", func(c *Config) { c.Layout.Islands.Rivals.Wobble = 1 }, "layout.islands.rivals.wobble"},
		{"repulsor radius", func(c *Config) { c.Layout.Custom.Rivals.RepulsorRadius = 0 }, "layout.custom.rivals.repulsor_radius"},
		{"relief zero", func(c *Config) { c.Elevation.ReliefScaleM = 0 }, "elevation.relief_scale_m"},
		{"ocean NaN", func(c *Config) { c.Elevation.OceanDepthM = math.NaN() }, "elevation.ocean_depth_m"},
		{"datum shift", func(c *Config) { c.Elevation.DatumMaxShift = -0.1 }, "elevation.datum_max_shift"},
		{"flatten", func(c *Config) { c.Elevation.BiasFlatten = 1.5 }, "elevation.bias_flatten"},
		{"continental octaves", func(c *Config) { c.Elevation.Continental.Octaves = 0 }, "elevation.continental.octaves"},
		{"continental gain", func(c *Config) { c.Elevation.Continental.Gain = 1 }, "elevation.continental.gain"},
		{"continental amplitude", func(c *Config) { c.Elevation.Continental.Amplitude = math.Inf(1) }, "elevation.continental.amplitude"},
		{"warp strength", func(c *Config) { c.Elevation.Warp.StrengthKm = -1 }, "elevation.warp.strength_km"},
		{"warp lacunarity", func(c *Config) { c.Elevation.Warp.Lacunarity = 1 }, "elevation.warp.lacunarity"},
		{"ridge wavelength", func(c *Config) { c.Elevation.Ridges.WavelengthKm = 0 }, "elevation.ridges.wavelength_km"},
		{"ridge threshold", func(c *Config) { c.Elevation.Ridges.Threshold = 1 }, "elevation.ridges.threshold"},
		{"ridge land ramp", func(c *Config) { c.Elevation.Ridges.LandRamp = 0 }, "elevation.ridges.land_ramp"},
		{"belt threshold", func(c *Config) { c.Elevation.Ridges.BeltThreshold = 3 }, "elevation.ridges.belt_threshold"},
		{"ceiling at sea level", func(c *Config) { c.Elevation.Falloff.CeilingM = 0 }, "elevation.falloff.ceiling_m"},
		{"depth above ceiling", func(c *Config) { c.Elevation.Falloff.DepthM = -500 }, "elevation.falloff.depth_m"},
		{"pull km", func(c *Config) { c.Elevation.Falloff.PullKm = 0 }, "elevation.falloff.pull_km"},
		{"jitter", func(c *Config) { c.Elevation.Falloff.JitterKm = -1 }, "elevation.falloff.jitter_km"},
		{"taper", func(c *Config) { c.Elevation.Falloff.TaperKm = math.NaN() }, "elevation.falloff.taper_km"},
		{"custom empty", func(c *Config) { c.Layout.Preset = PresetCustom }, "layout.custom.attractors is empty"},
		{"custom weight", func(c *Config) {
			c.Layout.Custom.Attractors = []CustomAttractor{{XKm: 100, YKm: 500, RadiusKm: 50, Weight: 0}}
		}, "layout.custom.attractors[0].weight"},
		{"custom x", func(c *Config) {
			c.Layout.Custom.Attractors = []CustomAttractor{{XKm: 1e6, YKm: 500, RadiusKm: 50, Weight: 1}}
		}, "layout.custom.attractors[0].x_km"},
		{"custom in falloff", func(c *Config) {
			c.Layout.Custom.Attractors = []CustomAttractor{{XKm: 100, YKm: 100, RadiusKm: 50, Weight: 1}}
		}, "layout.custom.attractors[0].y_km"},
		{"custom repulsor in rim", func(c *Config) {
			c.Layout.Custom.Repulsors = []CustomRepulsor{{XKm: 100, YKm: 1, RadiusKm: 50, Weight: 1}}
		}, "layout.custom.repulsors[0].y_km"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := Default()
			tc.edit(&c)
			err := c.Resolve()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Resolve = %v, want an error containing %q", err, tc.want)
			}
			if c.Province.AreaKm2 != 0 || c.World.WidthKm != 0 {
				t.Error("a failed Resolve filled derived fields")
			}
		})
	}

	// Validate reports every problem, not just the first.
	c := Default()
	c.World.LandCells = 0
	c.Raster.SpacingKm = 0
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "world.land_cells") || !strings.Contains(err.Error(), "raster.spacing_km") {
		t.Errorf("Validate = %v, want both errors", err)
	}
}

func TestCustomLayoutRoundTrip(t *testing.T) {
	c := example(t)
	c.Layout.Preset = PresetCustom
	c.Layout.Custom.Attractors = []CustomAttractor{
		{XKm: 100, YKm: 500, RadiusKm: 150, Weight: 0.9, Mass: 1},
		{XKm: 2500, YKm: 600, RadiusKm: 80, Weight: 0.5, Mass: 2},
	}
	c.Layout.Custom.Repulsors = []CustomRepulsor{{XKm: 1200, YKm: 560, RadiusKm: 60, Weight: 0.4}}
	if err := c.Resolve(); err != nil {
		t.Fatal(err)
	}
	first := encode(t, c)
	d, err := Decode(bytes.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c, d) || !bytes.Equal(first, encode(t, d)) {
		t.Errorf("custom layout round trip changed the config:\n%s", first)
	}
	// Absent lists decode as empty ones.
	e, err := Decode(strings.NewReader(`{"schema":1,"seed":"42","layout":{"custom":{"attractors":null}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(e, example(t)) {
		t.Errorf("null attractors resolved to %+v", e.Layout.Custom)
	}
}

func TestClearDerived(t *testing.T) {
	c := example(t)
	c.World.LandCells = 20_000
	if err := c.Resolve(); err == nil {
		t.Fatal("changing an input on a resolved config resolved without ClearDerived")
	}
	c.ClearDerived()
	if err := c.Resolve(); err != nil {
		t.Fatal(err)
	}
	if c.World.PlayableCells != 66_667 {
		t.Errorf("playable cells = %d, want 66667", c.World.PlayableCells)
	}
}
