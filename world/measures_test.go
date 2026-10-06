// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package world

import (
	"bytes"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestMeasureNames(t *testing.T) {
	names := MeasureNames()
	for _, want := range []string{"land.cells", "land.met", "mesh.edge_p5_km", "directions.error_p95_deg", "grades.land_max_percent", "water.coast_edges_per_land_cell",
		"landmasses.count", "landmasses.continents", "landmasses.islets", "landmasses.largest_share", "chokepoints.straits", "chokepoints.straits_major", "chokepoints.necks", "chokepoints.passes",
		"features.depressions", "features.depressions_below_min", "features.basins", "features.dry_basins", "features.dry_basin_area_cells", "features.playas",
		"features.glacier_cells", "features.ice_field_cells", "features.volcanoes", "features.hotspots",
		"rivers.edges_per_land_cell", "rivers.mouths", "rivers.longest_edges", "rivers.touch_share", "rivers.ends_ocean",
		"usability.habitable_share", "usability.wetland_share", "usability.coast_within_share", "usability.coast_distance_max"} {
		if !slices.Contains(names, want) {
			t.Errorf("no measure %q", want)
		}
	}
	for _, not := range []string{"schema", "pass", "mesh.neighbors_land", "grades.buckets", "checks", "landmasses.sizes", "landmasses.list", "chokepoints.strait_list", "chokepoints.neck_widths",
		"features.depths", "features.depth_buckets", "features.dry_basin_list", "features.dry_basin_cells", "usability.coast_distance", "usability.biomes", "water.lake_sizes"} {
		if slices.Contains(names, not) {
			t.Errorf("%q is named", not)
		}
	}
	if s := slices.Clone(names); len(slices.Compact(slices.Sorted(slices.Values(s)))) != len(names) {
		t.Error("repeated names")
	}
}

func TestMeasuresValue(t *testing.T) {
	m := &Measures{}
	m.Land.Cells = 9999
	m.Land.Met = true
	m.Directions.ErrorP95Deg = 21.5
	for name, want := range map[string]float64{"land.cells": 9999, "land.met": 1, "directions.error_p95_deg": 21.5, "water.lakes": 0} {
		if v, ok := m.Value(name); !ok || v != want {
			t.Errorf("Value(%q) = %v, %v; want %v", name, v, ok, want)
		}
	}
	if _, ok := m.Value("land.nope"); ok {
		t.Error("Value of an unknown name")
	}
}

func TestMeasuresBytes(t *testing.T) {
	m := &Measures{Schema: MeasuresSchemaVersion, Generator: "mpg", Seed: "42",
		Checks: []CheckResult{{Measure: "land.cells", Op: "<=", Value: 1, Mode: "gate", Actual: 2}}}
	m.Mesh.NeighborsLand = []int{0, 1, 2}
	m.Grades.LandMaxPercent = 22.6
	b, err := m.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte(`"op": "<="`)) || !bytes.HasSuffix(b, []byte("}\n")) {
		t.Errorf("Bytes:\n%s", b)
	}
	d, err := DecodeMeasuresBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d, m) {
		t.Errorf("round trip: %+v, want %+v", d, m)
	}
	m.Grades.LandMaxPercent = math.NaN()
	if _, err := m.Bytes(); err == nil {
		t.Error("NaN encoded")
	}
	for _, tc := range []struct{ in, msg string }{
		{`{}`, "no schema version"},
		{`{"schema": 1, "extra": 1}`, "unknown field"},
		{`{"schema": 1} {}`, "after the top-level object"},
		{`{"schema": 0}`, "world: measures: schema 0 is the pre-release layout, with no migration; regenerate the measures with this mpg"},
		{`{"schema": 0, "land": {"old_field": 1}}`, "schema 0 is the pre-release layout"},
		{`{"schema": 2, "new_group": {}}`, "world: measures: schema 2 is newer than this reader (schema 1); update github.com/mdhender/mpg/world"},
	} {
		if _, err := DecodeMeasures(strings.NewReader(tc.in)); err == nil || !strings.Contains(err.Error(), tc.msg) {
			t.Errorf("DecodeMeasures(%s) = %v, want an error mentioning %q", tc.in, err, tc.msg)
		}
	}
}
