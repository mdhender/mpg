// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package render

import "fmt"

// StageFile returns the render file name for a stage: its two-digit number
// from the pipeline table, its name, and an optional variant, all joined by
// hyphens, as in
//
//	StageFile(3, "elevation", "") == "03-elevation.png"
//	StageFile(4, "mesh", "area")  == "04-mesh-area.png"
//
// The name depends only on its arguments, never on the seed, time, or path,
// so every run writes the same names and the directory tells runs apart.
// Names and variants are lowercase kebab case: runs of a–z and 0–9 joined by
// single hyphens. StageFile panics on a number outside [1, 99] or an invalid
// name or variant, since stage names are fixed by code.
func StageFile(number int, name, variant string) string {
	if number < 1 || number > 99 {
		panic(fmt.Sprintf("render: stage number %d outside [1, 99]", number))
	}
	if !kebab(name) {
		panic(fmt.Sprintf("render: stage name %q is not lowercase kebab case", name))
	}
	if variant == "" {
		return fmt.Sprintf("%02d-%s.png", number, name)
	}
	if !kebab(variant) {
		panic(fmt.Sprintf("render: stage variant %q is not lowercase kebab case", variant))
	}
	return fmt.Sprintf("%02d-%s-%s.png", number, name, variant)
}

// kebab reports whether s is one or more runs of [a-z0-9] joined by single
// hyphens.
func kebab(s string) bool {
	if s == "" || s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for n := range len(s) {
		switch c := s[n]; {
		case 'a' <= c && c <= 'z', '0' <= c && c <= '9':
		case c == '-' && s[n-1] != '-':
		default:
			return false
		}
	}
	return true
}
