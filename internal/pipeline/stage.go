// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package pipeline

import (
	"fmt"
	"strconv"
	"strings"
)

// Stage is one row of the pipeline table.
type Stage struct {
	// Number is the stage's row in the pipeline table, used in render file
	// names.
	Number int
	// Name is the stage's fixed lowercase kebab-case name, used by
	// --stop-after and in render file names.
	Name string
	// Run runs the stage. It is nil while the stage is not implemented.
	Run func(*Context) error
}

// Implemented reports whether the stage has a Run function.
func (s Stage) Implemented() bool { return s.Run != nil }

// String returns "N name", as in "3 elevation".
func (s Stage) String() string { return fmt.Sprintf("%d %s", s.Number, s.Name) }

// Stages returns the pipeline's stages in order, as in DESIGN.md's pipeline
// table. The caller may modify the returned slice.
func Stages() []Stage {
	return []Stage{
		{Number: 1, Name: "config", Run: runConfig},
		{Number: 2, Name: "layout", Run: runLayout},
		{Number: 3, Name: "elevation", Run: runElevation},
		{Number: 4, Name: "mesh", Run: runMesh},
		{Number: 5, Name: "cells"},
		{Number: 6, Name: "sea-level"},
		{Number: 7, Name: "climate"},
		{Number: 8, Name: "basins"},
		{Number: 9, Name: "land-target"},
		{Number: 10, Name: "rivers"},
		{Number: 11, Name: "classify"},
		{Number: 12, Name: "edges"},
		{Number: 13, Name: "measures"},
		{Number: 14, Name: "export"},
	}
}

// Lookup returns the index in stages of the stage named s, which is a stage
// name or its number in decimal. The error lists the valid names.
func Lookup(stages []Stage, s string) (int, error) {
	n, err := strconv.Atoi(s)
	for i, st := range stages {
		if st.Name == s || (err == nil && st.Number == n) {
			return i, nil
		}
	}
	return -1, fmt.Errorf("unknown stage %q; valid stages are %s (or their numbers)", s, Names(stages))
}

// Names returns the stage names joined by ", ".
func Names(stages []Stage) string {
	names := make([]string, len(stages))
	for i, st := range stages {
		names[i] = st.Name
	}
	return strings.Join(names, ", ")
}
