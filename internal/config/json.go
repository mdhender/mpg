// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
)

// Decode reads a config.json strictly and resolves it.
//
// The schema version must be present and equal SchemaVersion. Unknown and
// duplicate fields are errors that name the field (with its path) and suggest
// the closest known field. Nothing but white space may follow the object.
// Fields the file leaves out keep their defaults (see Default), and derived
// fields it leaves out are computed; derived fields it sets must agree with
// its inputs (see Resolve).
func Decode(r io.Reader) (Config, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Config{}, fmt.Errorf("config: read: %w", err)
	}
	if err := checkSchema(data); err != nil {
		return Config{}, err
	}
	if err := checkFields(data); err != nil {
		return Config{}, err
	}
	c := Default()
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return Config{}, decodeError(err)
	}
	if err := c.Resolve(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Encode writes c as canonical config.json: two-space indented JSON in field
// order, with a trailing newline. c must be resolved; Encode resolves a copy
// and fails if that changes anything.
func (c *Config) Encode(w io.Writer) error {
	b, err := c.Bytes()
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

// Bytes returns the canonical config.json bytes of c, as Encode writes them:
// two-space indented JSON in field order, with no HTML escaping, and a
// trailing newline.
func (c *Config) Bytes() ([]byte, error) {
	r := *c
	if err := r.Resolve(); err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(r, *c) {
		return nil, errors.New("config: not resolved; call Resolve before encoding")
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // check ops such as "<=" stay readable
	enc.SetIndent("", "  ")
	if err := enc.Encode(c); err != nil {
		return nil, fmt.Errorf("config: encode: %w", err)
	}
	return buf.Bytes(), nil
}

// Hash returns the hex SHA-256 of the canonical config.json bytes of c (see
// Bytes). The bytes hold no timestamps or paths, so equal configs hash
// equally on every machine.
func (c *Config) Hash() (string, error) {
	b, err := c.Bytes()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// checkSchema rejects data that is not one JSON object with a supported
// schema version.
func checkSchema(data []byte) error {
	var head struct {
		Schema json.RawMessage `json:"schema"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return decodeError(err)
	}
	switch v := string(head.Schema); {
	case v == "":
		return fmt.Errorf("config: missing \"schema\"; this generator reads schema %d", SchemaVersion)
	case v != fmt.Sprint(SchemaVersion):
		return fmt.Errorf("config: schema %s is not supported; this generator reads schema %d (an older schema needs an explicit migration)", v, SchemaVersion)
	}
	return nil
}

// checkFields walks data's tokens against the Config type and rejects unknown
// and duplicate fields, and anything after the top-level value.
func checkFields(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := walk(dec, reflect.TypeFor[Config](), ""); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("config: unexpected data after the config object")
	}
	return nil
}

// walk consumes one JSON value from dec. When t is a struct and the value an
// object, it checks the object's keys against t's JSON fields, recursively;
// when t is a slice and the value an array, it walks each element.
// Type mismatches are left to the decoder.
func walk(dec *json.Decoder, t reflect.Type, path string) error {
	tok, err := dec.Token()
	if err != nil {
		return decodeError(err)
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil // a scalar
	}
	if delim == '[' && t.Kind() == reflect.Slice {
		elem := strings.TrimSuffix(path, ".")
		for k := 0; dec.More(); k++ {
			if err := walk(dec, t.Elem(), fmt.Sprintf("%s[%d].", elem, k)); err != nil {
				return err
			}
		}
		_, err = dec.Token() // ']'
		return decodeError(err)
	}
	if delim != '{' || t.Kind() != reflect.Struct {
		return skip(dec)
	}
	fields := jsonFields(t)
	seen := map[string]bool{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return decodeError(err)
		}
		key := tok.(string)
		name := path + key
		ft, ok := fields[key]
		if !ok {
			return unknownField(name, key, fields)
		}
		if seen[key] {
			return fmt.Errorf("config: duplicate field %q", name)
		}
		seen[key] = true
		if err := walk(dec, ft, name+"."); err != nil {
			return err
		}
	}
	_, err = dec.Token() // '}'
	return decodeError(err)
}

// skip consumes tokens up to and including the delimiter that closes the
// array or object whose opening delimiter was just read.
func skip(dec *json.Decoder) error {
	for depth := 1; depth > 0; {
		tok, err := dec.Token()
		if err != nil {
			return decodeError(err)
		}
		if d, ok := tok.(json.Delim); ok {
			if d == '{' || d == '[' {
				depth++
			} else {
				depth--
			}
		}
	}
	return nil
}

// jsonFields maps the JSON names of struct type t's fields to their types.
func jsonFields(t reflect.Type) map[string]reflect.Type {
	m := map[string]reflect.Type{}
	for f := range t.Fields() {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name != "" && name != "-" {
			m[name] = f.Type
		}
	}
	return m
}

// unknownField returns the error for an unknown field, suggesting the known
// field at the same level that is closest by edit distance, if any is close.
func unknownField(name, key string, fields map[string]reflect.Type) error {
	best, bestDist := "", -1
	for k := range fields {
		d := editDistance(key, k)
		if bestDist < 0 || d < bestDist || (d == bestDist && k < best) {
			best, bestDist = k, d
		}
	}
	if bestDist >= 0 && bestDist <= max(2, len(key)/3) {
		return fmt.Errorf("config: unknown field %q; did you mean %q?", name, strings.TrimSuffix(name, key)+best)
	}
	return fmt.Errorf("config: unknown field %q", name)
}

// editDistance returns the Levenshtein distance between a and b, in bytes.
func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := range len(a) {
		cur[0] = i + 1
		for j := range len(b) {
			cost := 1
			if a[i] == b[j] {
				cost = 0
			}
			cur[j+1] = min(prev[j+1]+1, cur[j]+1, prev[j]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// decodeError prefixes a JSON error for the reader. It returns nil for nil.
func decodeError(err error) error {
	switch {
	case err == nil:
		return nil
	case strings.HasPrefix(err.Error(), "config: "):
		return err
	case err == io.EOF || err == io.ErrUnexpectedEOF:
		return errors.New("config: unexpected end of input")
	}
	return fmt.Errorf("config: %w", err)
}
