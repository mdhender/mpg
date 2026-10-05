// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package world

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
)

// MarshalJSON writes p as [x, y], each number as encoding/json writes a
// float64 (the shortest decimal that reads back to the same bits). NaN and
// infinities are errors.
func (p Point) MarshalJSON() ([]byte, error) {
	b := make([]byte, 0, 32)
	b = append(b, '[')
	b, err := appendFloat(b, p.X)
	if err != nil {
		return nil, err
	}
	b = append(b, ',')
	if b, err = appendFloat(b, p.Y); err != nil {
		return nil, err
	}
	return append(b, ']'), nil
}

// UnmarshalJSON reads [x, y]: an array of exactly two numbers.
func (p *Point) UnmarshalJSON(b []byte) error {
	var v []float64
	if err := json.Unmarshal(b, &v); err != nil || len(v) != 2 {
		return fmt.Errorf("world: a point must be an array of two numbers, not %s", b)
	}
	p.X, p.Y = v[0], v[1]
	return nil
}

// appendFloat appends f as encoding/json formats a float64.
func appendFloat(b []byte, f float64) ([]byte, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil, fmt.Errorf("world: unsupported value %v", f)
	}
	abs := math.Abs(f)
	format := byte('f')
	if abs != 0 && (abs < 1e-6 || abs >= 1e21) {
		format = 'e'
	}
	n := len(b)
	b = strconv.AppendFloat(b, f, format, -1, 64)
	if format == 'e' {
		// Clean up e-09 to e-9, as encoding/json does.
		if m := len(b) - n; m >= 4 && b[len(b)-4] == 'e' && b[len(b)-3] == '-' && b[len(b)-2] == '0' {
			b[len(b)-2] = b[len(b)-1]
			b = b[:len(b)-1]
		}
	}
	return b, nil
}

// Bytes returns the canonical encoding of w: compact JSON with the fields
// in the order of the Go types, followed by a newline. The same world
// always gives the same bytes.
func (w *World) Bytes() ([]byte, error) {
	b, err := json.Marshal(w)
	if err != nil {
		return nil, fmt.Errorf("world: %w", err)
	}
	return append(b, '\n'), nil
}

// Decode reads a world.json strictly: the schema must be present and equal
// to SchemaVersion, unknown fields are errors, and nothing but white space
// may follow the object. It does not validate the world; call Validate.
func Decode(r io.Reader) (*World, error) {
	w := &World{}
	// The outer Schema shadows World.Schema, so the decoder fills it and
	// a missing schema stays nil.
	probe := struct {
		*World
		Schema *int `json:"schema"`
	}{World: w}
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&probe); err != nil {
		return nil, fmt.Errorf("world: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("world: data after the top-level object")
	}
	switch {
	case probe.Schema == nil:
		return nil, errors.New("world: no schema version")
	case *probe.Schema != SchemaVersion:
		return nil, fmt.Errorf("world: schema %d, want %d", *probe.Schema, SchemaVersion)
	}
	w.Schema = *probe.Schema
	return w, nil
}

// DecodeBytes is Decode over b.
func DecodeBytes(b []byte) (*World, error) { return Decode(bytes.NewReader(b)) }
