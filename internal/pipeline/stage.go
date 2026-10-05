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
	// Deferred marks a stage that is not implemented yet but that the
	// implemented stages after it do not need: the runner passes over it
	// (Result.Skipped) instead of stopping there. It has no effect on an
	// implemented stage.
	Deferred bool
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
		{Number: 5, Name: "cells", Run: runCells},
		{Number: 6, Name: "sea-level", Run: runSeaLevel},
		{Number: 7, Name: "climate", Run: runClimate},
		{Number: 8, Name: "basins", Run: runBasins},
		{Number: 9, Name: "land-target", Run: runLandTarget},
		// Stage 10 is deferred to milestone 7: no rivers yet.
		{Number: 10, Name: "rivers", Deferred: true},
		{Number: 11, Name: "classify", Run: runClassify},
		{Number: 12, Name: "edges", Run: runEdges},
		// Stage 13 is deferred to milestone 9; export does not need it.
		{Number: 13, Name: "measures", Deferred: true},
		{Number: 14, Name: "export", Run: runExport},
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
