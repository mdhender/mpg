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
// measures grow through milestone 9 (landmasses and chokepoints, features,
// rivers and usability join the groups below) without touching the game
// data.
const MeasuresSchemaVersion = 0

// MeasuresFile is the name of the playability report in a world's output
// directory.
const MeasuresFile = "measures.json"

// Measures is the playability report, measures.json (DESIGN.md,
// "Playability measures"): what the run measured, and the configured
// checks against it. It never changes the world; it describes it.
//
// Each group (Land, Mesh, ...) holds scalars and histograms. A scalar is
// named by its JSON path, "group.field" (as "directions.error_p95_deg");
// MeasureNames lists them and Measures.Value reads one. Checks in
// config.json and seed ranking refer to measures by these names. Booleans
// read as 0 or 1; histograms have no name. Every measure is defined for
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

	Land       LandMeasures      `json:"land"`
	Mesh       MeshMeasures      `json:"mesh"`
	Directions DirectionMeasures `json:"directions"`
	Grades     GradeMeasures     `json:"grades"`
	Water      WaterMeasures     `json:"water"`
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
