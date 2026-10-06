// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package config

import (
	"bytes"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/mdhender/mpg/world"
)

// TestDefaultMeasures checks that every default check names a measure and
// is report-only (DESIGN.md: "Checks begin as report-only").
func TestDefaultMeasures(t *testing.T) {
	m := DefaultMeasures()
	if len(m.Checks) == 0 {
		t.Fatal("no default checks")
	}
	names := world.MeasureNames()
	for k, c := range m.Checks {
		if !slices.Contains(names, c.Measure) || !slices.Contains(CheckOps, c.Op) || c.Mode != ModeReport {
			t.Errorf("default check %d %+v", k, c)
		}
	}
	// Default returns a fresh list each time.
	a, b := Default(), Default()
	a.Measures.Checks[0].Value = 99
	if b.Measures.Checks[0].Value == 99 {
		t.Error("Default shares its check list")
	}
}

// TestMeasuresChecksResolve checks that a bad check fails when the config
// resolves: an unknown measure (with the closest name suggested), an
// unknown op or mode, and a non-finite value.
func TestMeasuresChecksResolve(t *testing.T) {
	for _, tc := range []struct {
		name  string
		check Check
		want  string
	}{
		{"unknown measure", Check{"directions.error_p95", "<=", 1, ModeReport}, `"directions.error_p95" is not a measure; did you mean "directions.error_p95_deg"?`},
		{"far measure", Check{"zzz", "<=", 1, ModeGate}, `"zzz" is not a measure`},
		{"histogram", Check{"mesh.neighbors_land", "<=", 1, ModeGate}, `"mesh.neighbors_land" is not a measure`},
		{"op", Check{"land.cells", "=<", 1, ModeReport}, `op "=<" must be one of`},
		{"mode", Check{"land.cells", "<=", 1, "warn"}, `mode "warn" must be one of`},
		{"value", Check{"land.cells", "<=", math.Inf(1), ModeReport}, `value +Inf must be finite`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := Default()
			c.Measures.Checks = append(c.Measures.Checks, tc.check)
			err := c.Resolve()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Resolve: %v, want %q", err, tc.want)
			}
		})
	}
	// Decode rejects it too.
	_, err := Decode(strings.NewReader(`{"schema": 1, "measures": {"checks": [{"measure": "land.cell", "op": "<=", "value": 1, "mode": "gate"}]}}`))
	if err == nil || !strings.Contains(err.Error(), `did you mean "land.cells"?`) {
		t.Errorf("Decode unknown measure: %v", err)
	}
}

// TestMeasuresChecksDecode checks how a file's check list combines with
// the defaults: absent keeps them, a list replaces them as a whole, and []
// runs none and is written back as [].
func TestMeasuresChecksDecode(t *testing.T) {
	c, err := Decode(strings.NewReader(`{"schema": 1}`))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.Measures.Checks, DefaultMeasures().Checks) {
		t.Errorf("absent measures: checks %v, want the defaults", c.Measures.Checks)
	}
	c, err = Decode(strings.NewReader(`{"schema": 1, "measures": {"checks": [{"measure": "land.met", "op": "==", "value": 1, "mode": "gate"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if want := []Check{{"land.met", "==", 1, ModeGate}}; !slices.Equal(c.Measures.Checks, want) {
		t.Errorf("one check: %v, want %v", c.Measures.Checks, want)
	}
	b, err := c.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte(`"op": "=="`)) {
		t.Errorf("ops are not written as is:\n%s", b)
	}
	c, err = Decode(strings.NewReader(`{"schema": 1, "measures": {"checks": []}}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Measures.Checks == nil || len(c.Measures.Checks) != 0 {
		t.Errorf("empty list: %#v", c.Measures.Checks)
	}
	if b, err = c.Bytes(); err != nil || !bytes.Contains(b, []byte(`"checks": []`)) {
		t.Errorf("empty list encodes as:\n%s (%v)", b, err)
	}
	// A nil list resolves to an empty one, and the result round-trips.
	d := Default()
	d.Measures.Checks = nil
	if err := d.Resolve(); err != nil || d.Measures.Checks == nil {
		t.Errorf("nil list: %v, %#v", err, d.Measures.Checks)
	}
	if _, err := Decode(strings.NewReader(`{"schema": 1, "measures": {"checks": [{"measure": "land.met", "op": "==", "value": 1, "mode": "gate", "why": 1}]}}`)); err == nil || !strings.Contains(err.Error(), "why") {
		t.Errorf("unknown check field: %v", err)
	}
}

// TestMeasuresParameters checks the landmass, chokepoint and pass
// parameters: their defaults, that a file setting one keeps the others,
// and that bad values fail when the config resolves.
func TestMeasuresParameters(t *testing.T) {
	d := DefaultMeasures()
	if d.Landmass != (Landmass{IsletMaxCells: 9, ContinentMinCells: 1000}) ||
		d.Chokepoints != (Chokepoints{MaxCells: 3, DetourCells: 20, NeckMinRegionCells: 10}) ||
		d.Passes != (Passes{ChainMinCells: 10, MaxCells: 3, MaxGradePercent: 3, DetourCells: 20}) ||
		d.Usability != (Usability{CoastCells: 3}) {
		t.Errorf("defaults %+v %+v %+v %+v", d.Landmass, d.Chokepoints, d.Passes, d.Usability)
	}
	c, err := Decode(strings.NewReader(`{"schema": 1, "measures": {"chokepoints": {"max_cells": 2}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Measures.Chokepoints.MaxCells != 2 || c.Measures.Chokepoints.DetourCells != 20 || c.Measures.Landmass != d.Landmass || c.Measures.Passes != d.Passes ||
		c.Measures.Usability != d.Usability ||
		!slices.Equal(c.Measures.Checks, d.Checks) {
		t.Errorf("one parameter set: %+v", c.Measures)
	}
	for _, tc := range []struct {
		name string
		set  func(m *Measures)
		want string
	}{
		{"islet", func(m *Measures) { m.Landmass.IsletMaxCells = -1 }, "measures.landmass.islet_max_cells -1"},
		{"continent", func(m *Measures) { m.Landmass.ContinentMinCells = 10 }, "measures.landmass.continent_min_cells 10"},
		{"k low", func(m *Measures) { m.Chokepoints.MaxCells = 0 }, "measures.chokepoints.max_cells 0"},
		{"k high", func(m *Measures) { m.Chokepoints.MaxCells = MaxChokepointCells + 1 }, "measures.chokepoints.max_cells 5"},
		{"detour", func(m *Measures) { m.Chokepoints.DetourCells = 0 }, "measures.chokepoints.detour_cells 0"},
		{"region", func(m *Measures) { m.Chokepoints.NeckMinRegionCells = 0 }, "measures.chokepoints.neck_min_region_cells 0"},
		{"chain", func(m *Measures) { m.Passes.ChainMinCells = 0 }, "measures.passes.chain_min_cells 0"},
		{"pass cells", func(m *Measures) { m.Passes.MaxCells = MaxPassCells + 1 }, "measures.passes.max_cells 17"},
		{"grade", func(m *Measures) { m.Passes.MaxGradePercent = math.NaN() }, "measures.passes.max_grade_percent NaN"},
		{"pass detour", func(m *Measures) { m.Passes.DetourCells = MaxDetourCells + 1 }, "measures.passes.detour_cells 1001"},
		{"coast low", func(m *Measures) { m.Usability.CoastCells = 0 }, "measures.usability.coast_cells 0"},
		{"coast high", func(m *Measures) { m.Usability.CoastCells = MaxCoastCells + 1 }, "measures.usability.coast_cells 1001"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := Default()
			tc.set(&c.Measures)
			if err := c.Resolve(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Resolve: %v, want %q", err, tc.want)
			}
		})
	}
}
