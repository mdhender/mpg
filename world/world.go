// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package world

// SchemaVersion is the version of world.json's layout that this package
// reads and writes. Version 0 is the first playable export (milestone 4);
// it is not frozen, and may change without migration until version 1
// (milestone 9).
const SchemaVersion = 0

// File is the name of the game data file in a world's output directory.
const File = "world.json"

// Boundary stands for the rim boundary in Edge.Cells: the far side of an
// edge on the north (y = 0) or south (y = H) edge of the map.
const Boundary = -1

// World is the game data file, world.json.
type World struct {
	// Schema is SchemaVersion.
	Schema int `json:"schema"`
	// Meta describes the map: its size, wrap, rim, province size, and
	// units, and the config it was generated from.
	Meta Meta `json:"meta"`
	// Codebooks lists the values of every coded field.
	Codebooks Codebooks `json:"codebooks"`
	// Outcomes reports what the run found: the sea level and land count,
	// and how the search got there.
	Outcomes Outcomes `json:"outcomes"`
	// Cells lists every cell (province), rim cells included; Cells[i].ID
	// is i.
	Cells []Cell `json:"cells"`
	// Corners lists every corner; Corners[i].ID is i. Corners are
	// numbered north to south, then west to east.
	Corners []Corner `json:"corners"`
	// Edges lists every edge once; Edges[i].ID is i. Edges are numbered by
	// their corner ids.
	Edges []Edge `json:"edges"`
	// Coastlines lists the chains of coast edges, each with land on its
	// right, ordered by their first edge id.
	Coastlines []Coastline `json:"coastlines"`
	// Rivers lists the river polylines. It is empty until milestone 7.
	Rivers []RiverPath `json:"rivers"`
}

// Meta describes the map.
type Meta struct {
	// Generator is "mpg". The generator's version is not recorded: the
	// file must be byte-identical for every build that generates the same
	// world.
	Generator string `json:"generator"`
	// Seed is the world seed as a decimal string, so readers that parse
	// numbers as doubles keep every bit.
	Seed string `json:"seed"`
	// ConfigHash is the SHA-256 (lowercase hex) of the resolved
	// config.json the world was generated from.
	ConfigHash string `json:"config_hash"`
	// WidthKm is the east–west circumference W and HeightKm the
	// pole-to-pole height H, rims included.
	WidthKm  float64 `json:"width_km"`
	HeightKm float64 `json:"height_km"`
	// Wrap names the axis that wraps: "east-west". x wraps modulo W; y
	// never wraps.
	Wrap string `json:"wrap"`
	// Origin describes the coordinates: "northwest". x runs east in
	// [0, W) and y south in [0, H].
	Origin string `json:"origin"`
	// RimCells is how many cells deep the polar ice sheet reaches in from
	// each pole, and RimKm the same in km.
	RimCells int     `json:"rim_cells"`
	RimKm    float64 `json:"rim_km"`
	// HexFlatToFlatMi is the wilderness hex whose area a province has,
	// and ProvinceAreaKm2 that area, A.
	HexFlatToFlatMi float64 `json:"hex_flat_to_flat_mi"`
	ProvinceAreaKm2 float64 `json:"province_area_km2"`
	// CoordinatePrecisionKm is the grid every position, offset, length and
	// bounding box is rounded to: 0.000001 km (1 mm).
	CoordinatePrecisionKm float64 `json:"coordinate_precision_km"`
	// Units names the unit of each kind of quantity.
	Units Units `json:"units"`
}

// Units names the units of the file's quantities. Field names carry the
// unit as a suffix too (altitude_m, length_km).
type Units struct {
	Length  string `json:"length"`  // "km"
	Area    string `json:"area"`    // "km2"
	Height  string `json:"height"`  // "m"
	Angle   string `json:"angle"`   // "deg", clockwise from north
	Incline string `json:"incline"` // "permille": tenths of a percent of grade
}

// DefaultUnits returns the units this schema uses.
func DefaultUnits() Units {
	return Units{Length: "km", Area: "km2", Height: "m", Angle: "deg", Incline: "permille"}
}

// Point is a position or offset in km. In JSON it is an array [x, y].
type Point struct {
	X, Y float64
}

// BBox is an axis-aligned bounding box in world km. For a cell it bounds
// the unwrapped polygon, so Min.X may be below 0 or Max.X above W for a
// cell that spans the seam.
type BBox struct {
	Min Point `json:"min"`
	Max Point `json:"max"`
}

// Cell is one province.
type Cell struct {
	ID int `json:"id"`
	// Landform is the cell's geography. Rim cells are salt-water.
	Landform Landform `json:"landform"`
	// Depth is the depth band of a salt-water cell, empty otherwise. Rim
	// cells are deep.
	Depth Depth `json:"depth,omitempty"`
	// Water is the water kind of a water cell that is not on the rim,
	// empty otherwise.
	Water Water `json:"water,omitempty"`
	// AltitudeM is the median elevation of the cell's raster samples, in
	// meters: the one height every height rule uses.
	AltitudeM float64 `json:"altitude_m"`
	// Flags lists the cell's flags in CellFlags order.
	Flags []CellFlag `json:"flags,omitempty"`
	// Site is the cell's generating point, with X in [0, W).
	Site Point `json:"site"`
	// Centroid is the polygon's center of mass, with X in [0, W).
	Centroid Point `json:"centroid"`
	// Corners lists the polygon's corner ids clockwise on the map (north
	// up), starting at the lowest id.
	Corners []int `json:"corners"`
	// Sides lists the polygon's edge ids: Sides[k] joins Corners[k] and
	// Corners[k+1] (cyclically).
	Sides []int `json:"sides"`
	// Polygon is the polygon unwrapped about the site: Polygon[k] is the
	// displacement in km from Site to Corners[k], taken the shorter way
	// around the cylinder, so a cell that spans the seam draws without
	// special cases.
	Polygon []Point `json:"polygon"`
	// BBox bounds Site + Polygon[k].
	BBox BBox `json:"bbox"`
	// Exits lists the cell's half-edges, one per neighbor, in compass
	// order (N, NE, …, NW): clockwise, starting from the one nearest N.
	// Rim-boundary edges have no neighbor and no exit.
	Exits []Exit `json:"exits"`
}

// HasFlag reports whether the cell has flag f.
func (c *Cell) HasFlag(f CellFlag) bool {
	for _, g := range c.Flags {
		if g == f {
			return true
		}
	}
	return false
}

// Exit is a cell's half-edge: its side of an edge to a neighbor.
type Exit struct {
	// Direction is the exit's compass point, unique within the cell. The
	// neighbor's exit back is usually, but not always, the opposite point.
	Direction Direction `json:"direction"`
	// Neighbor is the cell across the edge, and Edge the edge's id.
	Neighbor int `json:"neighbor"`
	Edge     int `json:"edge"`
	// InclinePermille is the signed grade from this cell to the neighbor
	// in tenths of a percent: (neighbor altitude − this altitude) / site
	// distance, capped at ±1000 (100%). The neighbor's exit back holds
	// its exact negation.
	InclinePermille int `json:"incline_permille"`
	// BearingDeg is the direction from this cell's site to the
	// neighbor's, in degrees clockwise from north, rounded to 0.01°.
	BearingDeg float64 `json:"bearing_deg"`
	// ErrorDeg is the angle between the bearing and Direction, rounded to
	// 0.01°.
	ErrorDeg float64 `json:"error_deg"`
}

// Corner is a point where cell polygons meet.
type Corner struct {
	ID int `json:"id"`
	// Point is the corner's position, with X in [0, W) and Y in [0, H].
	Point Point `json:"point"`
	// HeightM is the mean altitude of the cells that meet at the corner.
	HeightM float64 `json:"height_m"`
	// Flags lists the corner's flags in CornerFlags order.
	Flags []CornerFlag `json:"flags,omitempty"`
	// Cells lists the cells whose polygons pass through the corner, and
	// Edges the edges that end at it, ascending.
	Cells []int `json:"cells"`
	Edges []int `json:"edges"`
}

// HasFlag reports whether the corner has flag f.
func (c *Corner) HasFlag(f CornerFlag) bool {
	for _, g := range c.Flags {
		if g == f {
			return true
		}
	}
	return false
}

// Edge is one undirected edge, a side of one or two cell polygons.
type Edge struct {
	ID int `json:"id"`
	// Cells holds the cells on either side, Cells[0] < Cells[1], or
	// Cells[1] == Boundary for an edge on the north or south edge of the
	// map.
	Cells [2]int `json:"cells"`
	// Corners holds the edge's ends in Cells[0]'s clockwise order, so
	// Cells[0] lies to the right of the walk from Corners[0] to
	// Corners[1] (north up).
	Corners [2]int `json:"corners"`
	// LengthKm is the distance between the corners, across the seam if
	// the edge spans it.
	LengthKm float64 `json:"length_km"`
	// Seed is the edge's noise seed, so a renderer can draw deterministic
	// noisy edges. It is below 2³², so every JSON reader keeps it exactly.
	Seed uint32 `json:"seed"`
	// Passable is false on the rim boundary and on every edge of a rim
	// cell. The game may add rules on top.
	Passable bool `json:"passable"`
	// Coast is true when exactly one side is water and neither side is a
	// rim cell; Water is then the water side's kind.
	Coast bool  `json:"coast,omitzero"`
	Water Water `json:"water,omitempty"`
	// River is the class of the river along the edge, empty for none.
	River RiverClass `json:"river,omitempty"`
	// InclinePermille is the grade from Cells[0] to Cells[1] in tenths of
	// a percent; 0 on the rim boundary.
	InclinePermille int `json:"incline_permille"`
}

// OnBoundary reports whether the edge lies on the rim boundary.
func (e *Edge) OnBoundary() bool { return e.Cells[1] == Boundary }

// Coastline is a chain of coast edges with land on its right, walking
// from Corners[k] to Corners[k+1] along Edges[k]. A closed chain (an
// island's or a lake's shore) has Corners[len(Edges)] == Corners[0]: it
// runs clockwise around land, counterclockwise around enclosed water.
type Coastline struct {
	Closed  bool  `json:"closed"`
	Edges   []int `json:"edges"`
	Corners []int `json:"corners"`
}

// RiverPath is a river polyline from source to mouth: a chain of land–land
// edges, Edges[k] joining Corners[k] and Corners[k+1], with Classes[k]
// the class of Edges[k]. There are none until milestone 7.
type RiverPath struct {
	Edges   []int        `json:"edges"`
	Corners []int        `json:"corners"`
	Classes []RiverClass `json:"classes"`
}

// Outcomes reports what the run found. Outcomes are never inputs: they go
// in world.json, not config.json.
type Outcomes struct {
	// SeaLevelM is the chosen sea level in meters. A playable cell is a
	// water candidate when its altitude is at or below it.
	SeaLevelM float64 `json:"sea_level_m"`
	// InitialEstimateM is the sea-level search's first probe.
	InitialEstimateM float64 `json:"initial_estimate_m"`
	// TargetLandCells is N, and ToleranceCells the largest |land − N|
	// that meets the land contract (TolerancePercent of N).
	TargetLandCells  int `json:"target_land_cells"`
	ToleranceCells   int `json:"tolerance_cells"`
	TolerancePercent int `json:"tolerance_percent"`
	// PlayableCells counts the non-rim cells; LandCells, OceanCells and
	// DryBasinCells the land after lakes (playas and dry basin floors
	// included), ocean, and land at or below the sea level (basin floors
	// left dry) among them; LakeCells and InlandSeaCells the cells of
	// water kind lake and inland sea.
	PlayableCells  int `json:"playable_cells"`
	LandCells      int `json:"land_cells"`
	OceanCells     int `json:"ocean_cells"`
	DryBasinCells  int `json:"dry_basin_cells"`
	LakeCells      int `json:"lake_cells"`
	InlandSeaCells int `json:"inland_sea_cells"`
	// LandAreaKm2 is the land cells' summed area.
	LandAreaKm2 float64 `json:"land_area_km2"`
	// Lakes and InlandSeas count the lakes and inland seas (connected sets
	// of cells of that water kind); SaltLakes and SaltInlandSeas those of
	// them with the salt flag; Playas the playa cells. Possible, not
	// forced: any of them may be 0.
	Lakes          int `json:"lakes"`
	InlandSeas     int `json:"inland_seas"`
	SaltLakes      int `json:"salt_lakes"`
	SaltInlandSeas int `json:"salt_inland_seas"`
	Playas         int `json:"playas"`
	// Met reports whether LandCells is within ToleranceCells of
	// TargetLandCells. An unmet target is reported, not an error.
	Met bool `json:"met"`
	// Reason is why the search ended: "exact", "within-tolerance",
	// "unreachable", or "budget-exhausted".
	Reason string `json:"reason"`
	// Policy and Budget name the search and its probe limit.
	Policy string `json:"policy"`
	Budget int    `json:"budget"`
	// ExpectedLakeCells is the lake cells the search's first probe
	// expected to lose to lakes (the basins stage's, at the first sea
	// level): the quantile estimate leaves TargetLandCells +
	// ExpectedLakeCells land candidates.
	ExpectedLakeCells int `json:"expected_lake_cells"`
	// PrePassLakeCells is the lake cells of the elevation stage's
	// pre-pass (stages 3 to 8 run once with no lake allowance), and
	// DatumLandShare the share of the raster outside the rim the final
	// elevation datum puts above 0 m to allow for them: the land fraction
	// times (N + PrePassLakeCells)/N.
	PrePassLakeCells int     `json:"prepass_lake_cells"`
	DatumLandShare   float64 `json:"datum_land_share"`
	// Trace lists the search's probes in the order made.
	Trace []Probe `json:"trace"`
	// ClimatePasses counts the climate passes: one with the ocean of the
	// first sea level, one with the final lakes.
	ClimatePasses int `json:"climate_passes"`
	// Deferred lists the pipeline stages not implemented yet, which the
	// run passed over, in pipeline order.
	Deferred []string `json:"deferred"`
}

// Probe is one level the sea-level search measured, with the basins and
// lakes at that level.
type Probe struct {
	// Method is how the level was chosen: "estimate", "newton",
	// "bisect", or "gallop".
	Method string `json:"method"`
	// LevelM is the level in meters.
	LevelM float64 `json:"level_m"`
	// Land, Ocean, DryBasin and Lake count the playable cells of each
	// kind at the level: land after lakes, ocean, basin floors left dry,
	// and lake and inland-sea cells.
	Land     int `json:"land"`
	Ocean    int `json:"ocean"`
	DryBasin int `json:"dry_basin"`
	Lake     int `json:"lake"`
}
