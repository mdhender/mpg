// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package world

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
)

// MeasuresSchemaVersion is the version of measures.json's layout that this
// package reads and writes. It is versioned apart from world.json: the
// measures grew through milestone 9 (landmasses and chokepoints in S34;
// features, rivers and usability in S35) without touching the game data.
const MeasuresSchemaVersion = 0

// MeasuresFile is the name of the playability report in a world's output
// directory.
const MeasuresFile = "measures.json"

// Measures is the playability report, measures.json (DESIGN.md,
// "Playability measures"): what the run measured, and the configured
// checks against it. It never changes the world; it describes it.
//
// Each group (Land, Mesh, ...) holds scalars and histograms, and the
// landmass, chokepoint and feature groups lists as well. A scalar is
// named by its JSON path, "group.field" (as "directions.error_p95_deg");
// MeasureNames lists them and Measures.Value reads one. Checks in
// config.json and seed ranking refer to measures by these names. Booleans
// read as 0 or 1; histograms and lists have no name. Every measure is defined for
// every world: a ratio whose denominator is 0 is 0, and a count of 0 is a
// valid outcome (possible, not forced).
type Measures struct {
	// Schema is MeasuresSchemaVersion.
	Schema int `json:"schema"`
	// Generator is "mpg"; its version is not recorded, as in world.json.
	Generator string `json:"generator"`
	// Seed is the world seed as a decimal string, and ConfigHash the
	// SHA-256 (lowercase hex) of the resolved config.json.
	Seed       string `json:"seed"`
	ConfigHash string `json:"config_hash"`
	// Pass reports whether no gate failed. GatesFailed and ReportsFailed
	// count the failed checks by mode.
	Pass          bool `json:"pass"`
	GatesFailed   int  `json:"gates_failed"`
	ReportsFailed int  `json:"reports_failed"`
	// Checks lists the configured checks in config order, each with the
	// measured value and its verdict.
	Checks []CheckResult `json:"checks"`

	Land        LandMeasures       `json:"land"`
	Mesh        MeshMeasures       `json:"mesh"`
	Directions  DirectionMeasures  `json:"directions"`
	Grades      GradeMeasures      `json:"grades"`
	Water       WaterMeasures      `json:"water"`
	Landmasses  LandmassMeasures   `json:"landmasses"`
	Chokepoints ChokepointMeasures `json:"chokepoints"`
	Features    FeatureMeasures    `json:"features"`
	Rivers      RiverMeasures      `json:"rivers"`
	Usability   UsabilityMeasures  `json:"usability"`
}

// CheckResult is one configured check and its outcome.
type CheckResult struct {
	// Measure, Op, Value and Mode are the check as configured: the
	// measure's name, the comparison ("<=", ">=", "<", ">", "=="), the
	// bound, and "report" or "gate".
	Measure string  `json:"measure"`
	Op      string  `json:"op"`
	Value   float64 `json:"value"`
	Mode    string  `json:"mode"`
	// Actual is the measured value, and Pass whether Actual Op Value
	// holds.
	Actual float64 `json:"actual"`
	Pass   bool    `json:"pass"`
}

// LandMeasures measures the land contract: the land cell count against N
// and the land's area.
type LandMeasures struct {
	// TargetCells is N; Cells the land cells after lakes; ToleranceCells
	// the largest |Cells − N| that meets the contract; DeviationCells
	// Cells − N and DeviationPercent the same as a percentage of N; Met
	// whether the deviation is within the tolerance.
	TargetCells      int     `json:"target_cells"`
	Cells            int     `json:"cells"`
	ToleranceCells   int     `json:"tolerance_cells"`
	DeviationCells   int     `json:"deviation_cells"`
	DeviationPercent float64 `json:"deviation_percent"`
	Met              bool    `json:"met"`
	// AreaKm2 is the land cells' summed area, and AreaPerTarget that area
	// over N·A.
	AreaKm2       float64 `json:"area_km2"`
	AreaPerTarget float64 `json:"area_per_target"`
}

// MeshMeasures measures the province mesh. Areas are multiples of the
// province area A; lengths are km.
type MeshMeasures struct {
	// Cells counts every cell, PlayableCells those off the rim, and
	// RimCells the rest.
	Cells         int `json:"cells"`
	PlayableCells int `json:"playable_cells"`
	RimCells      int `json:"rim_cells"`
	// CellAreaMeanA and CellAreaCV are the mean and coefficient of
	// variation of every cell's area; LandCellAreaMeanA,
	// LandCellAreaCV, LandCellAreaMinA and LandCellAreaMaxA the same, and
	// the bounds, over the land cells.
	CellAreaMeanA     float64 `json:"cell_area_mean_a"`
	CellAreaCV        float64 `json:"cell_area_cv"`
	LandCellAreaMeanA float64 `json:"land_cell_area_mean_a"`
	LandCellAreaCV    float64 `json:"land_cell_area_cv"`
	LandCellAreaMinA  float64 `json:"land_cell_area_min_a"`
	LandCellAreaMaxA  float64 `json:"land_cell_area_max_a"`
	// MinEdgeKm is the configured shortest edge (mesh.min_edge_km), and
	// EdgeMinKm and EdgeP5Km the shortest and the nearest-rank 5th
	// percentile length of the edges between two playable cells.
	MinEdgeKm float64 `json:"min_edge_km"`
	EdgeMinKm float64 `json:"edge_min_km"`
	EdgeP5Km  float64 `json:"edge_p5_km"`
	// NeighborsPlayable[k] and NeighborsLand[k] count the playable and
	// the land cells with k neighbors, k from 0 to 8 (the degree cap).
	NeighborsPlayable []int `json:"neighbors_playable"`
	NeighborsLand     []int `json:"neighbors_land"`
	// DegreeCapHits counts the collapses the degree cap made; Collapses
	// every short-edge collapse (the cap's included), Stretches the short
	// edges stretched instead, and MaxShiftKm the farthest a corner moved.
	DegreeCapHits int     `json:"degree_cap_hits"`
	Collapses     int     `json:"collapses"`
	Stretches     int     `json:"stretches"`
	MaxShiftKm    float64 `json:"max_shift_km"`
}

// DirectionMeasures measures the compass directions of the half-edges of
// the playable cells. An error is the angle in degrees between a
// half-edge's bearing and its compass point (Exit.ErrorDeg).
type DirectionMeasures struct {
	// HalfEdges counts the half-edges; ErrorMeanDeg, ErrorP95Deg
	// (nearest rank) and ErrorMaxDeg summarize their errors.
	HalfEdges    int     `json:"half_edges"`
	ErrorMeanDeg float64 `json:"error_mean_deg"`
	ErrorP95Deg  float64 `json:"error_p95_deg"`
	ErrorMaxDeg  float64 `json:"error_max_deg"`
	// Pairs counts the edges between two playable cells, NotOpposite
	// those whose two directions are not opposite compass points, and
	// NotOppositeShare NotOpposite / Pairs.
	Pairs            int     `json:"pairs"`
	NotOpposite      int     `json:"not_opposite"`
	NotOppositeShare float64 `json:"not_opposite_share"`
	// NaiveCollisions counts the playable cells in which labelling each
	// edge with its nearest compass point would repeat a point.
	NaiveCollisions int `json:"naive_collisions"`
}

// GradeMeasures measures the edges' inclines by |grade|.
type GradeMeasures struct {
	// Buckets names the histogram buckets of |grade| in percent: "0-1"
	// is [0, 1%), and so on, and "cap" the grades at the 100% cap.
	Buckets []string `json:"buckets"`
	// LandLand and Playable count the edges between two land cells and
	// between two playable cells in each bucket.
	LandLand []int `json:"land_land"`
	Playable []int `json:"playable"`
	// LandEdges counts the land–land edges; LandMaxPercent and
	// LandP95Percent are the steepest and the nearest-rank 95th
	// percentile |grade| among them, and LandCapEdges those at the cap.
	LandEdges      int     `json:"land_edges"`
	LandMaxPercent float64 `json:"land_max_percent"`
	LandP95Percent float64 `json:"land_p95_percent"`
	LandCapEdges   int     `json:"land_cap_edges"`
}

// WaterMeasures measures the water and the coasts.
type WaterMeasures struct {
	// OceanCells, LakeCells and InlandSeaCells count the playable cells
	// of each water kind.
	OceanCells     int `json:"ocean_cells"`
	LakeCells      int `json:"lake_cells"`
	InlandSeaCells int `json:"inland_sea_cells"`
	// Lakes and InlandSeas count the water bodies of each kind, and
	// LargestLakeCells and LargestInlandSeaCells the largest's cells (0
	// when there is none).
	Lakes                 int `json:"lakes"`
	InlandSeas            int `json:"inland_seas"`
	LargestLakeCells      int `json:"largest_lake_cells"`
	LargestInlandSeaCells int `json:"largest_inland_sea_cells"`
	// CoastEdges counts the coast edges, CoastEdgesPerLandCell them per
	// land cell, and LandRimEdges the edges between a land cell and the
	// rim, which the rim falloff should prevent.
	CoastEdges            int     `json:"coast_edges"`
	CoastEdgesPerLandCell float64 `json:"coast_edges_per_land_cell"`
	LandRimEdges          int     `json:"land_rim_edges"`
	// SizeBuckets names the buckets of the size histograms in cells
	// (LandmassSizeBuckets), and LakeSizes and InlandSeaSizes count the
	// lakes and the inland seas in each.
	SizeBuckets    []string `json:"size_buckets"`
	LakeSizes      []int    `json:"lake_sizes"`
	InlandSeaSizes []int    `json:"inland_sea_sizes"`
}

// Landmass classes, by cell count (config measures.landmass): an islet has
// at most islet_max_cells cells, a continent at least continent_min_cells,
// and an island lies between.
const (
	ClassContinent = "continent"
	ClassIsland    = "island"
	ClassIslet     = "islet"
)

// LandmassSizeBuckets names the buckets of LandmassMeasures.Sizes: landmass
// sizes in cells, "2-4" counting landmasses of 2 to 4 cells, and so on.
var LandmassSizeBuckets = []string{"1", "2-4", "5-9", "10-19", "20-49", "50-99", "100-199", "200-499", "500-999", "1000-1999", "2000-4999", "5000+"}

// LandmassMeasures measures the landmasses: the connected sets of land
// cells (land after lakes), joined through shared edges. A landmass's id is
// its rank by lowest cell id, so landmass 0 holds the land cell with the
// lowest id.
type LandmassMeasures struct {
	// Count counts the landmasses, and Continents, Islands and Islets them
	// by class.
	Count      int `json:"count"`
	Continents int `json:"continents"`
	Islands    int `json:"islands"`
	Islets     int `json:"islets"`
	// LargestCells is the largest landmass's cells, and LargestShare that
	// over the land cells (0 when there is no land).
	LargestCells int     `json:"largest_cells"`
	LargestShare float64 `json:"largest_share"`
	// IsletMaxCells and ContinentMinCells are the class thresholds
	// configured (measures.landmass), recorded for readers.
	IsletMaxCells     int `json:"islet_max_cells"`
	ContinentMinCells int `json:"continent_min_cells"`
	// SizeBuckets names the buckets (LandmassSizeBuckets), and Sizes
	// counts the landmasses in each.
	SizeBuckets []string `json:"size_buckets"`
	Sizes       []int    `json:"sizes"`
	// List lists the landmasses by id.
	List []Landmass `json:"list"`
}

// Landmass is one landmass: its cell count, its class, and its lowest cell
// id.
type Landmass struct {
	Cells     int    `json:"cells"`
	Class     string `json:"class"`
	FirstCell int    `json:"first_cell"`
}

// ChokepointMeasures measures the chokepoints, in cells (config
// measures.chokepoints and measures.passes):
//
//   - a strait is a water crossing of at most MaxCells playable water cells
//     (ocean, lake or inland sea; never the rim) between two landmasses, or
//     between two shores of one landmass that no land path of at most
//     detour_cells steps joins; the water cells on such crossings, grouped
//     by landmass pair and by water adjacency, make one strait each;
//   - a neck is a land isthmus: a cut of at most MaxCells land cells whose
//     removal splits its landmass into two regions of at least
//     neck_min_region_cells cells each (a cut of 2 or more cells is a path
//     whose end cells touch water); touching cuts make one neck;
//   - a mountain chain is a connected set of at least chain_min_cells
//     mountain cells, and a pass a route through at most passes.max_cells
//     of its cells, every edge at most max_grade_percent steep, joining
//     land off the chain on two sides that no land path of at most
//     passes.detour_cells steps avoiding the chain joins; the chain cells
//     on such routes, grouped by adjacency, make one pass each.
type ChokepointMeasures struct {
	// MaxCells is k, the widest strait and neck (configured).
	MaxCells int `json:"max_cells"`
	// Straits counts the straits: StraitsBetween those between two
	// landmasses and StraitsWithin those within one; StraitsMajor those
	// with no islet shore. StraitCells counts the water cells in straits.
	Straits        int `json:"straits"`
	StraitsBetween int `json:"straits_between"`
	StraitsWithin  int `json:"straits_within"`
	StraitsMajor   int `json:"straits_major"`
	StraitCells    int `json:"strait_cells"`
	// Necks counts the necks and NeckCells their cells.
	Necks     int `json:"necks"`
	NeckCells int `json:"neck_cells"`
	// Chains counts the mountain chains and ChainCells their cells;
	// Passes counts the passes, PassCells their cells, and ChainsWithPass
	// the chains with at least one.
	Chains         int `json:"chains"`
	ChainCells     int `json:"chain_cells"`
	Passes         int `json:"passes"`
	PassCells      int `json:"pass_cells"`
	ChainsWithPass int `json:"chains_with_pass"`
	// StraitWidths[w−1] and NeckWidths[w−1] count the straits and necks
	// of width w, 1 to MaxCells.
	StraitWidths []int `json:"strait_widths"`
	NeckWidths   []int `json:"neck_widths"`
	// StraitList, NeckList and PassList list the chokepoints, ordered by
	// landmass ids and then lowest cell id.
	StraitList []Chokepoint `json:"strait_list"`
	NeckList   []Chokepoint `json:"neck_list"`
	PassList   []Chokepoint `json:"pass_list"`
}

// Chokepoint is one strait, neck or pass.
type Chokepoint struct {
	// Width is the narrowest crossing in cells: the fewest water cells
	// joining the two shores of a strait, the fewest cells in a cut of a
	// neck, the fewest chain cells on a pass's route.
	Width int `json:"width"`
	// Landmasses holds the ids of the landmasses joined: two (ascending)
	// for a strait between landmasses, one for a strait within a
	// landmass, a neck, or a pass (its chain's landmass).
	Landmasses []int `json:"landmasses"`
	// Cells lists the chokepoint's cells, ascending: a strait's water
	// cells, a neck's land cells, a pass's chain cells.
	Cells []int `json:"cells"`
}

// DepthBuckets names the buckets of FeatureMeasures.Depths: depression
// depths in meters, "10-25" counting depressions at least 10 m and less
// than 25 m deep, and so on. The default basin.min_depth_m, 50 m, is a
// bucket edge.
var DepthBuckets = []string{"0-10", "10-25", "25-50", "50-100", "100-200", "200-500", "500-1000", "1000+"}

// FeatureMeasures measures the features that are possible, not forced:
// depressions and basins, dry basins and playas, permanent ice, and
// volcanoes. They are reported, never required.
//
// The basins are those of the land-target stage's chosen sea level. A dry
// basin is a basin with no water in it or in any basin nested in it, whose
// parent (if any) holds water: the largest waterless closed ground. Every
// dry basin holds at least one playa. Its cells are not world.json's
// outcomes.dry_basin_cells, which counts the land at or below the sea
// level.
type FeatureMeasures struct {
	// MinDepthM is basin.min_depth_m, recorded for readers. Depressions
	// counts every depression of the cell graph, DepressionsBelowMin those
	// shallower than MinDepthM, and Basins the rest. BasinsFull,
	// BasinsPartial and BasinsDry count the basins by water state, and
	// DeepestBasinM is the deepest basin's depth (0 with none).
	MinDepthM           float64 `json:"min_depth_m"`
	Depressions         int     `json:"depressions"`
	DepressionsBelowMin int     `json:"depressions_below_min"`
	Basins              int     `json:"basins"`
	BasinsFull          int     `json:"basins_full"`
	BasinsPartial       int     `json:"basins_partial"`
	BasinsDry           int     `json:"basins_dry"`
	DeepestBasinM       float64 `json:"deepest_basin_m"`
	// DepthBuckets names the buckets (DepthBuckets), and Depths counts
	// every depression, those below the minimum included, in each.
	DepthBuckets []string `json:"depth_buckets"`
	Depths       []int    `json:"depths"`
	// Playas counts the playas (dry leaf basins; one cell each). DryBasins
	// counts the dry basins, DryBasinAreaCells their cells (nested basins
	// included), LargestDryBasinCells the largest's cells, and
	// DeepestDryBasinM the deepest's depth (0 with none).
	Playas               int     `json:"playas"`
	DryBasins            int     `json:"dry_basins"`
	DryBasinAreaCells    int     `json:"dry_basin_area_cells"`
	LargestDryBasinCells int     `json:"largest_dry_basin_cells"`
	DeepestDryBasinM     float64 `json:"deepest_dry_basin_m"`
	// DryBasinList lists the dry basins by basin id.
	DryBasinList []DryBasin `json:"dry_basin_list"`
	// GlacierCells and IceFieldCells count the land cells under permanent
	// ice by surface, PolarDesertCells the land cells of biome
	// polar-desert, and PackIceCells the playable water cells with pack
	// ice (as world.json's outcomes).
	GlacierCells     int `json:"glacier_cells"`
	IceFieldCells    int `json:"ice_field_cells"`
	PolarDesertCells int `json:"polar_desert_cells"`
	PackIceCells     int `json:"pack_ice_cells"`
	// Hotspots counts the volcanic hotspots, Volcanoes those whose peak is
	// on land, and VolcanicHighlandCells the cells of landform
	// volcanic-highlands.
	Hotspots              int `json:"hotspots"`
	Volcanoes             int `json:"volcanoes"`
	VolcanicHighlandCells int `json:"volcanic_highland_cells"`
}

// DryBasin is one dry basin: its basin id (among the basins at least the
// minimum depth deep), its cells (nested basins included), its depth, and
// its bottom cell.
type DryBasin struct {
	Basin  int     `json:"basin"`
	Cells  int     `json:"cells"`
	DepthM float64 `json:"depth_m"`
	Bottom int     `json:"bottom"`
}

// RiverMeasures measures the river network (the river stage's
// statistics; DESIGN.md, "Rivers on edges").
type RiverMeasures struct {
	// RiverEdges counts the river edges, and StreamEdges,
	// RiverClassEdges and MajorRiverEdges them by class;
	// EdgesPerLandCell is RiverEdges per land cell.
	RiverEdges       int     `json:"river_edges"`
	StreamEdges      int     `json:"stream_edges"`
	RiverClassEdges  int     `json:"river_class_edges"`
	MajorRiverEdges  int     `json:"major_river_edges"`
	EdgesPerLandCell float64 `json:"edges_per_land_cell"`
	// Km is the river edges' length, and KmPer1000Km2 that per 1,000 km²
	// of land. TouchCells counts the land cells with a river edge on their
	// border, and TouchShare their share of the land cells.
	Km           float64 `json:"km"`
	KmPer1000Km2 float64 `json:"km_per_1000_km2"`
	TouchCells   int     `json:"touch_cells"`
	TouchShare   float64 `json:"touch_share"`
	// Polylines counts the river polylines and Mouths the mouth corners
	// (several polylines may end at one). EndsOcean, EndsLake and EndsSink
	// count the polylines ending at a mouth on the sea, a lake or a dry
	// sink, and EndsConfluence those ending on another polyline;
	// Confluences counts the corners where they do. OutletSources counts
	// the polylines starting at a lake's outlet.
	Polylines      int `json:"polylines"`
	Mouths         int `json:"mouths"`
	EndsOcean      int `json:"ends_ocean"`
	EndsLake       int `json:"ends_lake"`
	EndsSink       int `json:"ends_sink"`
	EndsConfluence int `json:"ends_confluence"`
	Confluences    int `json:"confluences"`
	OutletSources  int `json:"outlet_sources"`
	// LongestEdges and LongestKm size the longest polyline, by edges (ties
	// to the longer); LongestFlowEdges and LongestFlowKm the longest path
	// from a polyline's source down the drainage tree to where its water
	// ends.
	LongestEdges     int     `json:"longest_edges"`
	LongestKm        float64 `json:"longest_km"`
	LongestFlowEdges int     `json:"longest_flow_edges"`
	LongestFlowKm    float64 `json:"longest_flow_km"`
	// SeamEdges counts the river edges crossing the east–west seam;
	// MaxDrainageKm2 and MaxDischargeM3s are the largest drainage and
	// discharge on any drainage tree edge.
	SeamEdges       int     `json:"seam_edges"`
	MaxDrainageKm2  float64 `json:"max_drainage_km2"`
	MaxDischargeM3s float64 `json:"max_discharge_m3s"`
}

// UsabilityMeasures measures how usable the land is. Shares are of the
// land cells (land after lakes).
type UsabilityMeasures struct {
	// HabitableCells counts the land cells that are not under permanent
	// ice (biome clear: glacier or ice field), not polar-desert or desert,
	// and not mountains; wetlands are habitable. HabitableShare is their
	// share. WetlandCells counts the land cells with a wetland surface,
	// and WetlandShare their share; DesertCells and MountainCells count
	// the land cells of biome desert and of landform mountains.
	HabitableCells int     `json:"habitable_cells"`
	HabitableShare float64 `json:"habitable_share"`
	WetlandCells   int     `json:"wetland_cells"`
	WetlandShare   float64 `json:"wetland_share"`
	DesertCells    int     `json:"desert_cells"`
	MountainCells  int     `json:"mountain_cells"`
	// A land cell's coast distance is the fewest steps through land from
	// it to playable water (ocean, lake or inland sea; never the rim): 1
	// for a land cell touching water. CoastCells is d
	// (measures.usability.coast_cells); CoastWithinCells counts the land
	// cells at most d from the coast, and CoastWithinShare their share.
	CoastCells       int     `json:"coast_cells"`
	CoastWithinCells int     `json:"coast_within_cells"`
	CoastWithinShare float64 `json:"coast_within_share"`
	// CoastDistance[k] counts the land cells at coast distance k (index 0
	// is always 0); CoastDistanceMax and CoastDistanceMean are the largest
	// and the mean over the land cells that reach water.
	// CoastUnreachedCells counts the land cells with no land path to
	// water (0 on every world measured).
	CoastDistance       []int   `json:"coast_distance"`
	CoastDistanceMax    int     `json:"coast_distance_max"`
	CoastDistanceMean   float64 `json:"coast_distance_mean"`
	CoastUnreachedCells int     `json:"coast_unreached_cells"`
	// BiomeNames and LandformNames name the land's biomes and landforms in
	// code order, and Biomes and Landforms count the land cells of each.
	BiomeNames    []string `json:"biome_names"`
	Biomes        []int    `json:"biomes"`
	LandformNames []string `json:"landform_names"`
	Landforms     []int    `json:"landforms"`
}

// Bytes returns the canonical encoding of m: two-space indented JSON in the
// field order of the Go types, floats as encoding/json writes them (the
// shortest decimal that reads back to the same bits), no HTML escaping, and
// a trailing newline. NaN and infinities are errors.
func (m *Measures) Bytes() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // check ops such as "<=" stay readable
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		return nil, fmt.Errorf("world: measures: %w", err)
	}
	return buf.Bytes(), nil
}

// DecodeMeasures reads a measures.json strictly: the schema must be present
// and equal MeasuresSchemaVersion, unknown fields are errors, and nothing
// but white space may follow the object.
func DecodeMeasures(r io.Reader) (*Measures, error) {
	m := &Measures{}
	probe := struct {
		*Measures
		Schema *int `json:"schema"`
	}{Measures: m}
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&probe); err != nil {
		return nil, fmt.Errorf("world: measures: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("world: measures: data after the top-level object")
	}
	switch {
	case probe.Schema == nil:
		return nil, errors.New("world: measures: no schema version")
	case *probe.Schema != MeasuresSchemaVersion:
		return nil, fmt.Errorf("world: measures: schema %d, want %d", *probe.Schema, MeasuresSchemaVersion)
	}
	m.Schema = *probe.Schema
	return m, nil
}

// DecodeMeasuresBytes is DecodeMeasures over b.
func DecodeMeasuresBytes(b []byte) (*Measures, error) { return DecodeMeasures(bytes.NewReader(b)) }

// measureField locates one named scalar: the index of its group among
// Measures' fields and its own index in the group.
type measureField struct {
	name         string
	group, field int
}

// measureFields lists the named scalars in field order: every int,
// float64 or bool field of every struct-typed field of Measures.
var measureFields = func() []measureField {
	var out []measureField
	t := reflect.TypeFor[Measures]()
	for g := range t.NumField() {
		gf := t.Field(g)
		if gf.Type.Kind() != reflect.Struct {
			continue
		}
		for f := range gf.Type.NumField() {
			ff := gf.Type.Field(f)
			switch ff.Type.Kind() {
			case reflect.Int, reflect.Float64, reflect.Bool:
				out = append(out, measureField{name: jsonName(gf) + "." + jsonName(ff), group: g, field: f})
			}
		}
	}
	return out
}()

// jsonName returns a struct field's JSON name.
func jsonName(f reflect.StructField) string {
	name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
	return name
}

// MeasureNames returns the names of the scalar measures, "group.field" as
// in measures.json, in the order of the file.
func MeasureNames() []string {
	names := make([]string, len(measureFields))
	for k, f := range measureFields {
		names[k] = f.name
	}
	return names
}

// Value returns the scalar measure called name, a boolean as 0 or 1, and
// false when there is no such measure.
func (m *Measures) Value(name string) (float64, bool) {
	for _, f := range measureFields {
		if f.name != name {
			continue
		}
		v := reflect.ValueOf(m).Elem().Field(f.group).Field(f.field)
		switch v.Kind() {
		case reflect.Int:
			return float64(v.Int()), true
		case reflect.Float64:
			return v.Float(), true
		case reflect.Bool:
			if v.Bool() {
				return 1, true
			}
			return 0, true
		}
	}
	return 0, false
}
