// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package measure

import (
	"fmt"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/world"
)

// Evaluate runs checks against m in order, replacing m.Checks with their
// results and setting m.Pass, m.GatesFailed and m.ReportsFailed. A check
// naming no measure, or with an unknown op or mode, is an error; config
// validation rejects those first.
func Evaluate(m *world.Measures, checks []config.Check) error {
	m.Checks = make([]world.CheckResult, len(checks))
	m.GatesFailed, m.ReportsFailed = 0, 0
	for k, c := range checks {
		v, ok := m.Value(c.Measure)
		if !ok {
			return fmt.Errorf("measure: checks[%d]: no measure %q", k, c.Measure)
		}
		pass, err := compare(v, c.Op, c.Value)
		if err != nil {
			return fmt.Errorf("measure: checks[%d]: %w", k, err)
		}
		switch c.Mode {
		case config.ModeReport:
			if !pass {
				m.ReportsFailed++
			}
		case config.ModeGate:
			if !pass {
				m.GatesFailed++
			}
		default:
			return fmt.Errorf("measure: checks[%d]: unknown mode %q", k, c.Mode)
		}
		m.Checks[k] = world.CheckResult{Measure: c.Measure, Op: c.Op, Value: c.Value, Mode: c.Mode, Actual: v, Pass: pass}
	}
	m.Pass = m.GatesFailed == 0
	return nil
}

// compare reports whether v op bound holds.
func compare(v float64, op string, bound float64) (bool, error) {
	switch op {
	case "<=":
		return v <= bound, nil
	case ">=":
		return v >= bound, nil
	case "<":
		return v < bound, nil
	case ">":
		return v > bound, nil
	case "==":
		return v == bound, nil
	}
	return false, fmt.Errorf("unknown op %q", op)
}

// Failed returns the failed checks of m, in order.
func Failed(m *world.Measures) []world.CheckResult {
	var out []world.CheckResult
	for _, c := range m.Checks {
		if !c.Pass {
			out = append(out, c)
		}
	}
	return out
}

// CheckLine formats a check's result for the summary and the logs, as
// "FAIL  gate   directions.error_max_deg <= 45 (actual 47.1032)".
func CheckLine(c world.CheckResult) string {
	verdict := "pass"
	if !c.Pass {
		verdict = "FAIL"
	}
	return fmt.Sprintf("%s  %-6s %s %s %v (actual %.6g)", verdict, c.Mode, c.Measure, c.Op, c.Value, c.Actual)
}

// Verdict summarizes the checks of m in one line, as
// "14 checks: 13 pass, 1 report failed, 0 gates failed".
func Verdict(m *world.Measures) string {
	failed := m.GatesFailed + m.ReportsFailed
	return fmt.Sprintf("%d checks: %d pass, %d report failed, %d gates failed",
		len(m.Checks), len(m.Checks)-failed, m.ReportsFailed, m.GatesFailed)
}
