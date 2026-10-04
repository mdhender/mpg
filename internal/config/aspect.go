// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package config

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// aspects maps each aspect name to its width:height. The design fixes
// widescreen (16:9) and cinematic (2.39:1); landscape (3:2) and portrait
// (2:3) are the conventional photographic ratios.
var aspects = []struct {
	Name          string
	Width, Height float64
}{
	{"square", 1, 1},
	{"portrait", 2, 3},
	{"landscape", 3, 2},
	{"widescreen", 16, 9},
	{"cinematic", 2.39, 1},
}

// ParseAspect returns the width / height ratio named by s: one of the names
// square (1:1), portrait (2:3), landscape (3:2), widescreen (16:9), and
// cinematic (2.39:1), or an explicit "W:H" with a positive finite number on each side
// (for example "16:9" or "2.39:1"). Names are lowercase and exact; no
// surrounding space is allowed.
func ParseAspect(s string) (float64, error) {
	for _, a := range aspects {
		if s == a.Name {
			return a.Width / a.Height, nil
		}
	}
	ws, hs, ok := strings.Cut(s, ":")
	if !ok {
		return 0, fmt.Errorf("aspect %q is not a name (square, portrait, landscape, widescreen, cinematic) or a W:H ratio", s)
	}
	w, err := aspectSide(ws)
	if err != nil {
		return 0, fmt.Errorf("aspect %q: width: %w", s, err)
	}
	h, err := aspectSide(hs)
	if err != nil {
		return 0, fmt.Errorf("aspect %q: height: %w", s, err)
	}
	r := w / h
	if !positive(r) {
		return 0, fmt.Errorf("aspect %q: ratio %v must be positive and finite", s, r)
	}
	return r, nil
}

// aspectSide parses one side of a W:H ratio.
func aspectSide(s string) (float64, error) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("%q is not a finite number", s)
	}
	if !(v > 0) {
		return 0, fmt.Errorf("%v must be positive", v)
	}
	return v, nil
}
