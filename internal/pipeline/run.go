// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package pipeline

import (
	"fmt"
	"os"
)

// Result reports what a run did.
type Result struct {
	// Ran lists the stages that ran, in order.
	Ran []Stage
	// Skipped lists the deferred stages the run passed over, in order.
	Skipped []Stage
	// NotImplemented is the first unimplemented stage the run reached, if
	// it stopped there.
	NotImplemented *Stage
}

// Run creates the output and render directories and runs stages in order,
// through the stage at index stopAfter (or all of them when stopAfter is
// negative). It passes over a deferred stage that is not implemented,
// listing it in Result.Skipped, stops early, without error, at the first
// other stage that is not implemented, and returns at the first stage
// error.
func Run(c *Context, stages []Stage, stopAfter int) (Result, error) {
	var res Result
	if stopAfter >= len(stages) {
		return res, fmt.Errorf("pipeline: stop-after index %d out of range", stopAfter)
	}
	if stopAfter < 0 {
		stopAfter = len(stages) - 1
	}
	for _, dir := range []string{c.OutputDir, c.RendersDir} {
		if dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return res, fmt.Errorf("pipeline: %w", err)
		}
	}
	for _, st := range stages[:stopAfter+1] {
		if !st.Implemented() {
			if st.Deferred {
				res.Skipped = append(res.Skipped, st)
				continue
			}
			res.NotImplemented = &st
			return res, nil
		}
		c.stage = st
		err := st.Run(c)
		c.stage = Stage{}
		if err != nil {
			return res, fmt.Errorf("stage %s: %w", st, err)
		}
		res.Ran = append(res.Ran, st)
	}
	return res, nil
}
