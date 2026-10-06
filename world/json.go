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
// may follow the object. A file of another version is rejected with an
// error naming it (see schemaError), ahead of any field error its layout
// causes. It does not validate the world; call Validate.
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
	err := dec.Decode(&probe)
	if probe.Schema != nil && *probe.Schema != SchemaVersion {
		// An unknown or mistyped field does not stop the decoder, so the
		// schema is known even when another version's layout failed.
		return nil, schemaError("world", "world", *probe.Schema, SchemaVersion)
	}
	if err != nil {
		return nil, fmt.Errorf("world: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("world: data after the top-level object")
	}
	if probe.Schema == nil {
		return nil, errors.New("world: no schema version")
	}
	w.Schema = *probe.Schema
	return w, nil
}

// schemaError is the error for a file of schema got where this package
// reads want: prefix names the file in the message and what the thing it
// holds. Version 0 was the pre-release layout, which changed without
// migration, so it is never migrated: the file must be generated again. A
// newer version needs a newer reader. (Readers of later versions migrate
// each older frozen version explicitly; see the package documentation.)
func schemaError(prefix, what string, got, want int) error {
	switch {
	case got > want:
		return fmt.Errorf("%s: schema %d is newer than this reader (schema %d); update github.com/mdhender/mpg/world", prefix, got, want)
	case got == 0:
		return fmt.Errorf("%s: schema 0 is the pre-release layout, with no migration; regenerate the %s with this mpg", prefix, what)
	default:
		return fmt.Errorf("%s: schema %d is not a version of this file (schema %d)", prefix, got, want)
	}
}

// DecodeBytes is Decode over b.
func DecodeBytes(b []byte) (*World, error) { return Decode(bytes.NewReader(b)) }
