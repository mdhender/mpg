// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package world

import "slices"

// Landform is a cell's physical geography, in hmz2ter's vocabulary.
type Landform string

// The landforms.
const (
	SaltWater         Landform = "salt-water"  // ocean, inland seas (fresh or salt: FlagSalt), and the rim
	FreshWater        Landform = "fresh-water" // lakes, fresh or salt (FlagSalt)
	Flats             Landform = "flats"
	Plains            Landform = "plains"
	RollingPlains     Landform = "rolling-plains"
	Hills             Landform = "hills"
	Mountains         Landform = "mountains"
	Plateaus          Landform = "plateaus"
	VolcanicHighlands Landform = "volcanic-highlands"
)

// Landforms lists every landform, water first.
var Landforms = []Landform{SaltWater, FreshWater, Flats, Plains, RollingPlains, Hills, Mountains, Plateaus, VolcanicHighlands}

// IsLand reports whether the landform is land.
func (l Landform) IsLand() bool { return slices.Contains(Landforms[2:], l) }

// IsWater reports whether the landform is water.
func (l Landform) IsWater() bool { return l == SaltWater || l == FreshWater }

// Depth is a salt-water cell's depth band, from its distance in cell steps
// to the nearest cell that is not salt water. It is empty on every other
// cell.
type Depth string

// The depth bands.
const (
	DepthNone Depth = ""
	Shallow   Depth = "shallow"
	Open      Depth = "open"
	Deep      Depth = "deep"
)

// Depths lists the depth bands, shallowest first.
var Depths = []Depth{Shallow, Open, Deep}

// Water is the kind of a water cell, and of the water side of a coast edge.
// It is empty on land, on rim cells (the ice sheet), and on edges that are
// not coasts.
type Water string

// The water kinds.
const (
	WaterNone Water = ""
	Ocean     Water = "ocean"      // water connected to the rim
	Lake      Water = "lake"       // inland water of fewer than basin.inland_sea_min_cells cells
	InlandSea Water = "inland-sea" // larger inland water
)

// Waters lists the water kinds.
var Waters = []Water{Ocean, Lake, InlandSea}

// Direction is one of the 8 compass points. A cell gives each of its
// neighbors a different one.
type Direction string

// The compass points, clockwise from north.
const (
	N  Direction = "N"
	NE Direction = "NE"
	E  Direction = "E"
	SE Direction = "SE"
	S  Direction = "S"
	SW Direction = "SW"
	W  Direction = "W"
	NW Direction = "NW"
)

// Directions lists the compass points clockwise from north; a point's index
// times 45° is its angle clockwise from north.
var Directions = []Direction{N, NE, E, SE, S, SW, W, NW}

// Index returns the compass point's index in Directions, or −1 for a value
// that is not a compass point.
func (d Direction) Index() int { return slices.Index(Directions, d) }

// RiverClass is the class of the river along an edge. It is empty on an
// edge with no river.
type RiverClass string

// The river classes (from milestone 7).
const (
	RiverNone  RiverClass = ""
	Stream     RiverClass = "stream"
	River      RiverClass = "river"
	MajorRiver RiverClass = "major-river"
)

// RiverClasses lists the river classes, smallest first.
var RiverClasses = []RiverClass{Stream, River, MajorRiver}

// CellFlag marks a property of a cell.
type CellFlag string

// The cell flags.
const (
	// FlagRim marks the polar ice sheet. It overrides everything else
	// about the cell: the game treats it as impassable, and a renderer
	// draws it as ice whatever its landform says.
	FlagRim CellFlag = "rim"
	// FlagImpassable marks a cell the game may not enter. Only rim cells
	// are impassable.
	FlagImpassable CellFlag = "impassable"
	// FlagVolcano marks the land cell holding a volcano's peak.
	FlagVolcano CellFlag = "volcano"
	// FlagCoast marks a cell with at least one coast edge.
	FlagCoast CellFlag = "coast"
	// FlagSalt marks a cell of a salt lake or salt inland sea: closed (no
	// outflow) and evaporation-dominated, a salinity proxy. The water
	// kind (lake or inland sea, and with it the landform, fresh-water or
	// salt-water) is set by size; salinity only by this flag. The ocean
	// and the rim are salt by definition and do not carry it.
	FlagSalt CellFlag = "salt"
	// FlagPlaya marks the land cell at the bottom of a basin with no
	// stable water level: a dry lake bed. Its sink corner is a dry sink
	// for rivers.
	FlagPlaya CellFlag = "playa"
)

// CellFlags lists the cell flags in the order a cell lists them.
var CellFlags = []CellFlag{FlagRim, FlagImpassable, FlagVolcano, FlagCoast, FlagSalt, FlagPlaya}

// CornerFlag marks a property of a corner.
type CornerFlag string

// The corner flags.
const (
	// FlagBoundary marks a corner on the north (y = 0) or south (y = H)
	// edge of the map.
	FlagBoundary CornerFlag = "boundary"
	// FlagTerminal marks a corner where rivers end (DESIGN.md, "Rivers on
	// edges"): a shore corner, one that touches both a land cell and a
	// water or rim cell, or a dry sink (FlagSink).
	FlagTerminal CornerFlag = "terminal"
	// FlagMouth marks a terminal corner where a river ends: the last
	// corner of a river polyline that no polyline continues through.
	FlagMouth CornerFlag = "mouth"
	// FlagSink marks a dry sink: the lowest corner (by height, ties to the
	// lower id) of a playa cell, where rivers draining into a dry basin
	// end. A sink corner is terminal too.
	FlagSink CornerFlag = "sink"
)

// CornerFlags lists the corner flags in the order a corner lists them.
var CornerFlags = []CornerFlag{FlagBoundary, FlagTerminal, FlagMouth, FlagSink}

// Codebooks lists every value each coded field may take, in a fixed order,
// so a reader can check a file against them. They are written into the
// file; Validate checks that they equal this package's.
type Codebooks struct {
	Landforms    []Landform   `json:"landforms"`
	Depths       []Depth      `json:"depths"`
	Waters       []Water      `json:"waters"`
	Directions   []Direction  `json:"directions"`
	RiverClasses []RiverClass `json:"river_classes"`
	CellFlags    []CellFlag   `json:"cell_flags"`
	CornerFlags  []CornerFlag `json:"corner_flags"`
}

// DefaultCodebooks returns this schema's codebooks.
func DefaultCodebooks() Codebooks {
	return Codebooks{
		Landforms:    slices.Clone(Landforms),
		Depths:       slices.Clone(Depths),
		Waters:       slices.Clone(Waters),
		Directions:   slices.Clone(Directions),
		RiverClasses: slices.Clone(RiverClasses),
		CellFlags:    slices.Clone(CellFlags),
		CornerFlags:  slices.Clone(CornerFlags),
	}
}

// equal reports whether c and d list the same values in the same order.
func (c *Codebooks) equal(d *Codebooks) bool {
	return slices.Equal(c.Landforms, d.Landforms) &&
		slices.Equal(c.Depths, d.Depths) &&
		slices.Equal(c.Waters, d.Waters) &&
		slices.Equal(c.Directions, d.Directions) &&
		slices.Equal(c.RiverClasses, d.RiverClasses) &&
		slices.Equal(c.CellFlags, d.CellFlags) &&
		slices.Equal(c.CornerFlags, d.CornerFlags)
}
