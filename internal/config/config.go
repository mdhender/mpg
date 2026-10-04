// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package config

import (
	"errors"
	"fmt"
	"math"

	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/internal/topo"
)

// SchemaVersion is the config.json schema this package reads and writes.
const SchemaVersion = 1

// KmPerMile is the international mile in kilometers (exact by definition).
const KmPerMile = 1.609344

// MaxLandCells bounds the requested land cell count. The design targets
// worlds of tens to a few hundred thousand cells; this only rejects absurd
// requests.
const MaxLandCells = 10_000_000

// Config is the complete resolved input to one run: the schema of
// config.json. Fields marked "derived" are filled by Resolve.
type Config struct {
	// Schema is the config.json schema version, SchemaVersion.
	Schema int `json:"schema"`
	// Seed is the world seed, written as a decimal string.
	Seed      Seed      `json:"seed"`
	Province  Province  `json:"province"`
	World     World     `json:"world"`
	Rim       Rim       `json:"rim"`
	Raster    Raster    `json:"raster"`
	Layout    Layout    `json:"layout"`
	Elevation Elevation `json:"elevation"`
	Volcanic  Volcanic  `json:"volcanic"`
	Mesh      Mesh      `json:"mesh"`
	Basin     Basin     `json:"basin"`
	River     River     `json:"river"`
}

// Province sizes one province (one Voronoi cell) by the area of a wilderness
// hex, so exploring a province takes one game day.
type Province struct {
	// HexFlatToFlatMi is the wilderness hex's flat-to-flat width d in miles.
	HexFlatToFlatMi float64 `json:"hex_flat_to_flat_mi"`
	// AreaKm2 is the province area A = (√3/2)·d² in km² (derived).
	AreaKm2 float64 `json:"area_km2"`
}

// World sets the land target and the shape of the playable area.
type World struct {
	// LandCells is the requested number of land cells, N.
	LandCells int `json:"land_cells"`
	// LandFraction is the land fraction f of the playable (non-rim) cells.
	LandFraction float64 `json:"land_fraction"`
	// Aspect is the playable area's width:height, as a name (see
	// ParseAspect) or an explicit "W:H" ratio.
	Aspect string `json:"aspect"`
	// AspectRatio is Aspect as width / height (derived).
	AspectRatio float64 `json:"aspect_ratio"`
	// PlayableCells is round(N / f) (derived).
	PlayableCells int `json:"playable_cells"`
	// PlayableAreaKm2 is PlayableCells × A (derived).
	PlayableAreaKm2 float64 `json:"playable_area_km2"`
	// WidthKm is the east-west circumference W (derived).
	WidthKm float64 `json:"width_km"`
	// HeightKm is the pole-to-pole height H, including both rims (derived).
	HeightKm float64 `json:"height_km"`
}

// Rim sets the polar ice rim and, inside it, the ocean falloff band. Widths
// are play rules, so they are configured in cells and converted to km with
// √A per cell.
type Rim struct {
	// Cells is how many cells deep the ice band reaches in from each pole.
	Cells int `json:"cells"`
	// Km is Cells × √A (derived).
	Km float64 `json:"km"`
	// FalloffCells is how many cells of open ocean, at least, separate the
	// ice from any land.
	FalloffCells int `json:"falloff_cells"`
	// FalloffKm is FalloffCells × √A (derived).
	FalloffKm float64 `json:"falloff_km"`
}

// Raster sets the synthetic DEM and climate grid.
type Raster struct {
	// SpacingKm is the distance between raster samples. It must be less than
	// √A so every province gets samples.
	SpacingKm float64 `json:"spacing_km"`
}

// Volcanic sets the volcanic hotspots (DESIGN.md, "Volcanic hotspots"):
// each adds a cone on a broad, low swell to the bedrock. See package
// elevation for the shapes.
type Volcanic struct {
	// HotspotsPerMkm2 is the mean hotspot count per million km² of playable
	// area (DESIGN.md: volcanic_hotspots_per_mkm2), in [0, 1000]. The count
	// is a Poisson draw, so any count, including zero, can occur.
	HotspotsPerMkm2 float64 `json:"hotspots_per_mkm2"`
	// ConePeakMinM and ConePeakMaxM bound a cone's peak height in meters
	// above the ground it stands on; each hotspot draws its own, uniformly.
	ConePeakMinM float64 `json:"cone_peak_min_m"`
	ConePeakMaxM float64 `json:"cone_peak_max_m"`
	// ConeRadiusMinKm and ConeRadiusMaxKm bound a cone's radius in km;
	// each hotspot draws its own, uniformly.
	ConeRadiusMinKm float64 `json:"cone_radius_min_km"`
	ConeRadiusMaxKm float64 `json:"cone_radius_max_km"`
	// SwellM is the height of the broad swell under every cone, in meters,
	// and SwellRadiusKm its radius in km. The swell makes the plateau
	// around a volcano.
	SwellM        float64 `json:"swell_m"`
	SwellRadiusKm float64 `json:"swell_radius_km"`
}

// MaxHotspotsPerMkm2 bounds Volcanic.HotspotsPerMkm2. Hotspots are meant to
// be rare; the bound only rejects requests that would bury the map in cones.
const MaxHotspotsPerMkm2 = 1000

// DefaultVolcanic returns the default volcanic hotspot inputs.
func DefaultVolcanic() Volcanic {
	return Volcanic{
		HotspotsPerMkm2: 2,
		ConePeakMinM:    1500, ConePeakMaxM: 3000,
		ConeRadiusMinKm: 15, ConeRadiusMaxKm: 30,
		SwellM: 300, SwellRadiusKm: 100,
	}
}

// validate appends the problems with the volcanic inputs through bad.
func (v *Volcanic) validate(bad func(format string, args ...any)) {
	if x := v.HotspotsPerMkm2; !(x >= 0 && x <= MaxHotspotsPerMkm2) {
		bad("volcanic.hotspots_per_mkm2 %v must be in [0, %d]", x, MaxHotspotsPerMkm2)
	}
	if x := v.ConePeakMinM; !(x >= 0 && x <= 20_000) {
		bad("volcanic.cone_peak_min_m %v must be in [0, 20000]", x)
	}
	if x := v.ConePeakMaxM; !(x >= v.ConePeakMinM && x <= 20_000) {
		bad("volcanic.cone_peak_max_m %v must be in [cone_peak_min_m %v, 20000]", x, v.ConePeakMinM)
	}
	if x := v.ConeRadiusMinKm; !(x > 0 && x <= 1000) {
		bad("volcanic.cone_radius_min_km %v must be in (0, 1000]", x)
	}
	if x := v.ConeRadiusMaxKm; !(x >= v.ConeRadiusMinKm && x <= 1000) {
		bad("volcanic.cone_radius_max_km %v must be in [cone_radius_min_km %v, 1000]", x, v.ConeRadiusMinKm)
	}
	if x := v.SwellM; !(x >= 0 && x <= 20_000) {
		bad("volcanic.swell_m %v must be in [0, 20000]", x)
	}
	if x := v.SwellRadiusKm; !(x > 0 && x <= 10_000) {
		bad("volcanic.swell_radius_km %v must be in (0, 10000]", x)
	}
}

// Basin sets closed basins and lakes.
type Basin struct {
	// MinDepthM is how far a depression's spill level must rise above its
	// lowest cell to count as a basin, in meters (DESIGN.md:
	// basin_min_depth_m).
	MinDepthM float64 `json:"min_depth_m"`
	// InlandSeaMinCells is the smallest inland sea; smaller water bodies are
	// lakes (DESIGN.md: inland_sea_min_cells).
	InlandSeaMinCells int `json:"inland_sea_min_cells"`
}

// River sets river selection.
type River struct {
	// ThresholdKm2 is the drainage area at which an edge becomes a river
	// (DESIGN.md: river_threshold_km2).
	ThresholdKm2 float64 `json:"threshold_km2"`
}

// Default returns the default inputs, with the derived fields zero. The
// seed defaults to 0; callers normally set it.
func Default() Config {
	return Config{
		Schema:   SchemaVersion,
		Province: Province{HexFlatToFlatMi: 6},
		World: World{
			LandCells:    10_000,
			LandFraction: 0.30,
			Aspect:       "cinematic",
		},
		Rim:       Rim{Cells: 4, FalloffCells: 12},
		Raster:    Raster{SpacingKm: 2},
		Layout:    DefaultLayout(),
		Elevation: DefaultElevation(),
		Volcanic:  DefaultVolcanic(),
		Mesh:      DefaultMesh(),
		Basin:     Basin{MinDepthM: 50, InlandSeaMinCells: 20},
		River:     River{ThresholdKm2: 500},
	}
}

// ClearDerived zeroes the derived fields, so the next Resolve recomputes them.
// Call it after changing an input on a resolved config.
func (c *Config) ClearDerived() {
	c.Province.AreaKm2 = 0
	c.World.AspectRatio = 0
	c.World.PlayableCells = 0
	c.World.PlayableAreaKm2 = 0
	c.World.WidthKm = 0
	c.World.HeightKm = 0
	c.Rim.Km = 0
	c.Rim.FalloffKm = 0
	c.Mesh.MinEdgeKm = 0
}

// Validate checks the inputs (not the derived fields) and returns every
// problem it finds, joined.
func (c *Config) Validate() error {
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("config: "+format, args...))
	}
	if c.Schema != SchemaVersion {
		bad("schema %d is not supported; this generator reads schema %d", c.Schema, SchemaVersion)
	}
	if v := c.Province.HexFlatToFlatMi; !positive(v) {
		bad("province.hex_flat_to_flat_mi %v must be positive and finite", v)
	}
	if v := c.World.LandCells; v < 1 || v > MaxLandCells {
		bad("world.land_cells %d must be in [1, %d]", v, MaxLandCells)
	}
	if v := c.World.LandFraction; !(v > 0 && v < 1) {
		bad("world.land_fraction %v must be greater than 0 and less than 1", v)
	}
	if _, err := ParseAspect(c.World.Aspect); err != nil {
		bad("world.aspect: %v", err)
	}
	if v := c.Rim.Cells; v < 1 {
		bad("rim.cells %d must be at least 1", v)
	}
	if v := c.Rim.FalloffCells; v < 0 {
		bad("rim.falloff_cells %d must not be negative", v)
	}
	if v := c.Raster.SpacingKm; !positive(v) {
		bad("raster.spacing_km %v must be positive and finite", v)
	}
	c.Layout.validate(bad)
	c.Elevation.validate(bad)
	c.Volcanic.validate(bad)
	c.Mesh.validate(bad)
	if v := c.Basin.MinDepthM; !nonNegative(v) {
		bad("basin.min_depth_m %v must be non-negative and finite", v)
	}
	if v := c.Basin.InlandSeaMinCells; v < 2 {
		bad("basin.inland_sea_min_cells %d must be at least 2 (smaller water bodies are lakes of 1 or more cells)", v)
	}
	if v := c.River.ThresholdKm2; !positive(v) {
		bad("river.threshold_km2 %v must be positive and finite", v)
	}
	return errors.Join(errs...)
}

// Resolve validates the inputs and fills the derived fields. A derived field
// that is already set must equal the value the inputs give; otherwise Resolve
// fails and leaves c unchanged. Resolving a resolved config is a no-op.
func (c *Config) Resolve() error {
	if err := c.Validate(); err != nil {
		return err
	}
	d, err := c.derive()
	if err != nil {
		return err
	}
	var errs []error
	check := func(name string, have, want float64) {
		if have != 0 && math.Float64bits(have) != math.Float64bits(want) {
			errs = append(errs, fmt.Errorf("config: %s is %v but the inputs give %v; the file was edited inconsistently: fix the inputs, or delete %s to recompute it", name, have, want, name))
		}
	}
	check("province.area_km2", c.Province.AreaKm2, d.Province.AreaKm2)
	check("world.aspect_ratio", c.World.AspectRatio, d.World.AspectRatio)
	check("world.playable_cells", float64(c.World.PlayableCells), float64(d.World.PlayableCells))
	check("world.playable_area_km2", c.World.PlayableAreaKm2, d.World.PlayableAreaKm2)
	check("world.width_km", c.World.WidthKm, d.World.WidthKm)
	check("world.height_km", c.World.HeightKm, d.World.HeightKm)
	check("rim.km", c.Rim.Km, d.Rim.Km)
	check("rim.falloff_km", c.Rim.FalloffKm, d.Rim.FalloffKm)
	check("mesh.min_edge_km", c.Mesh.MinEdgeKm, d.Mesh.MinEdgeKm)
	if err := errors.Join(errs...); err != nil {
		return err
	}
	*c = d
	return nil
}

// derive returns a copy of c, whose inputs are valid, with the derived fields
// computed.
func (c *Config) derive() (Config, error) {
	d := *c
	aspect, err := ParseAspect(c.World.Aspect)
	if err != nil {
		return Config{}, fmt.Errorf("config: world.aspect: %w", err)
	}

	dkm := fmath.Mul(c.Province.HexFlatToFlatMi, KmPerMile)
	area := fmath.Mul(math.Sqrt(3)/2, fmath.Mul(dkm, dkm))
	cellKm := math.Sqrt(area) // km per cell step: the side of a square of area A

	cells := math.Round(float64(c.World.LandCells) / c.World.LandFraction)
	playableArea := fmath.Mul(cells, area)
	playableHeight := math.Sqrt(playableArea / aspect)
	rimKm := fmath.Mul(float64(c.Rim.Cells), cellKm)
	falloffKm := fmath.Mul(float64(c.Rim.FalloffCells), cellKm)

	d.Province.AreaKm2 = area
	d.World.AspectRatio = aspect
	d.World.PlayableCells = int(cells)
	d.World.PlayableAreaKm2 = playableArea
	d.World.WidthKm = fmath.Mul(aspect, playableHeight)
	d.World.HeightKm = playableHeight + fmath.Mul(2, rimKm)
	d.Rim.Km = rimKm
	d.Rim.FalloffKm = falloffKm
	d.Mesh.MinEdgeKm = fmath.Mul(c.Mesh.MinEdgeFraction, cellKm)

	for _, f := range []struct {
		name string
		v    float64
		zero bool // zero allowed
	}{
		{"province.area_km2", area, false},
		{"world.playable_area_km2", playableArea, false},
		{"world.width_km", d.World.WidthKm, false},
		{"world.height_km", d.World.HeightKm, false},
		{"rim.km", rimKm, false},
		{"rim.falloff_km", falloffKm, true},
	} {
		if !positive(f.v) && !(f.zero && f.v == 0) {
			return Config{}, fmt.Errorf("config: derived %s is %v; the inputs are out of range", f.name, f.v)
		}
	}
	if !(d.Raster.SpacingKm < cellKm) {
		return Config{}, fmt.Errorf("config: raster.spacing_km %v must be less than the province side √A = %v km, so every province gets samples", d.Raster.SpacingKm, cellKm)
	}
	if !(2*falloffKm < playableHeight) {
		return Config{}, fmt.Errorf("config: the falloff bands (2 × %d cells = %v km) do not fit in the playable height %v km; request more land cells or fewer falloff cells", c.Rim.FalloffCells, 2*falloffKm, playableHeight)
	}
	cyl, err := topo.New(d.World.WidthKm, d.World.HeightKm, rimKm, falloffKm)
	if err != nil {
		return Config{}, fmt.Errorf("config: resolved sizes: %w", err)
	}
	if err := d.Layout.derive(cyl); err != nil {
		return Config{}, err
	}
	return d, nil
}

// positive reports whether v is positive and finite.
func positive(v float64) bool { return v > 0 && !math.IsInf(v, 1) }

// nonNegative reports whether v is non-negative and finite.
func nonNegative(v float64) bool { return v >= 0 && !math.IsInf(v, 1) }
