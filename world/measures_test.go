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
		"landmasses.count", "landmasses.continents", "landmasses.islets", "landmasses.largest_share", "chokepoints.straits", "chokepoints.straits_major", "chokepoints.necks", "chokepoints.passes"} {
		if !slices.Contains(names, want) {
			t.Errorf("no measure %q", want)
		}
	}
	for _, not := range []string{"schema", "pass", "mesh.neighbors_land", "grades.buckets", "checks", "landmasses.sizes", "landmasses.list", "chokepoints.strait_list", "chokepoints.neck_widths"} {
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
	for _, bad := range []string{`{}`, `{"schema": 1}`, `{"schema": 0, "extra": 1}`, `{"schema": 0} {}`} {
		if _, err := DecodeMeasures(strings.NewReader(bad)); err == nil {
			t.Errorf("DecodeMeasures(%s) accepted", bad)
		}
	}
}
