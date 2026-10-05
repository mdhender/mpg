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
