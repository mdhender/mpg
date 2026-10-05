// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package measure

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/world"
)

func TestCompare(t *testing.T) {
	for _, tc := range []struct {
		v, bound float64
		op       string
		want     bool
	}{
		{1, 1, "<=", true}, {2, 1, "<=", false},
		{1, 1, ">=", true}, {0, 1, ">=", false},
		{1, 1, "<", false}, {0, 1, "<", true},
		{1, 1, ">", false}, {2, 1, ">", true},
		{1, 1, "==", true}, {1.5, 1, "==", false},
	} {
		got, err := compare(tc.v, tc.op, tc.bound)
		if err != nil || got != tc.want {
			t.Errorf("%v %s %v = %v, %v; want %v", tc.v, tc.op, tc.bound, got, err, tc.want)
		}
	}
	if _, err := compare(1, "=<", 1); err == nil {
		t.Error("unknown op accepted")
	}
}

func TestEvaluate(t *testing.T) {
	m := &world.Measures{}
	m.Land.Cells = 100
	m.Directions.ErrorMaxDeg = 47
	checks := []config.Check{
		{Measure: "land.cells", Op: ">=", Value: 100, Mode: config.ModeGate},
		{Measure: "directions.error_max_deg", Op: "<=", Value: 45, Mode: config.ModeReport},
		{Measure: "land.met", Op: "==", Value: 1, Mode: config.ModeGate},
	}
	if err := Evaluate(m, checks); err != nil {
		t.Fatal(err)
	}
	if m.Pass || m.GatesFailed != 1 || m.ReportsFailed != 1 || len(m.Checks) != 3 {
		t.Fatalf("pass %v, gates %d, reports %d, checks %d", m.Pass, m.GatesFailed, m.ReportsFailed, len(m.Checks))
	}
	want := world.CheckResult{Measure: "directions.error_max_deg", Op: "<=", Value: 45, Mode: config.ModeReport, Actual: 47}
	if m.Checks[1] != want {
		t.Errorf("check 1 %+v, want %+v", m.Checks[1], want)
	}
	if f := Failed(m); len(f) != 2 || f[0].Measure != "directions.error_max_deg" || f[1].Measure != "land.met" {
		t.Errorf("Failed %+v", f)
	}
	if got := CheckLine(m.Checks[2]); got != "FAIL  gate   land.met == 1 (actual 0)" {
		t.Errorf("CheckLine %q", got)
	}
	if got := Verdict(m); got != "3 checks: 1 pass, 1 report failed, 1 gates failed" {
		t.Errorf("Verdict %q", got)
	}
	// Re-evaluating resets the counts.
	if err := Evaluate(m, checks[:1]); err != nil || !m.Pass || m.GatesFailed != 0 || m.ReportsFailed != 0 || len(m.Checks) != 1 {
		t.Errorf("re-evaluate: %v %+v", err, m)
	}
	for _, bad := range []config.Check{
		{Measure: "land.nope", Op: "<=", Value: 1, Mode: config.ModeReport},
		{Measure: "land.cells", Op: "!=", Value: 1, Mode: config.ModeReport},
		{Measure: "land.cells", Op: "<=", Value: 1, Mode: "warn"},
	} {
		if err := Evaluate(m, []config.Check{bad}); err == nil {
			t.Errorf("Evaluate accepted %+v", bad)
		}
	}
}

func TestHelpers(t *testing.T) {
	xs := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	if got := nearestRank(xs, 5); got != 1 {
		t.Errorf("p5 %v", got)
	}
	if got := nearestRank(xs, 95); got != 19 {
		t.Errorf("p95 %v", got)
	}
	if got := nearestRank([]int{7}, 95); got != 7 {
		t.Errorf("p95 of one %v", got)
	}
	mean, cv := meanCV([]float64{1, 3})
	if mean != 2 || cv != 0.5 {
		t.Errorf("meanCV = %v, %v", mean, cv)
	}
	if _, cv := meanCV([]float64{0, 0}); cv != 0 {
		t.Errorf("CV of zeros %v", cv)
	}
	if ratio(1, 0) != 0 || ratio(1, 4) != 0.25 {
		t.Error("ratio")
	}
}

func TestComputeMissing(t *testing.T) {
	if _, err := Compute(Input{}); err == nil {
		t.Error("Compute accepted no products")
	}
}

func TestLines(t *testing.T) {
	m := &world.Measures{}
	m.Mesh.NeighborsPlayable = []int{0, 0, 0, 0, 2, 5}
	m.Grades.Buckets = []string{"0-1", "cap"}
	m.Grades.LandLand = []int{3, 1}
	if err := Evaluate(m, []config.Check{{Measure: "land.cells", Op: ">", Value: 0, Mode: config.ModeReport}}); err != nil {
		t.Fatal(err)
	}
	s := string(Summary(m))
	for _, want := range []string{"land     0 / 0 cells (+0, +0.00%, UNMET", "nbrs     playable 4:2 5:5;", "land-land 0-1 75.0%, cap 25.0%", "check    FAIL  report land.cells > 0 (actual 0)", "checks   1 checks: 0 pass, 1 report failed, 0 gates failed\n"} {
		if !strings.Contains(s, want) {
			t.Errorf("summary lacks %q:\n%s", want, s)
		}
	}
}

// TestNoFusedMultiplyAdd compiles the package for arm64 and for amd64 with
// GOAMD64=v3 (which has FMA) and fails if the compiler fused any multiply
// and add (DESIGN.md, "Determinism").
func TestNoFusedMultiplyAdd(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles for several architectures")
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool not found")
	}
	fusedOp := regexp.MustCompile(`\tV?FN?M(ADD|SUB)`)
	const pkg = "github.com/mdhender/mpg/internal/measure"
	for _, tg := range []struct{ arch, env string }{{"arm64", ""}, {"amd64", "GOAMD64=v3"}} {
		t.Run(tg.arch, func(t *testing.T) {
			cmd := exec.Command(goTool, "build", "-gcflags="+pkg+"=-S", pkg)
			cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+tg.arch, "CGO_ENABLED=0")
			if tg.env != "" {
				cmd.Env = append(cmd.Env, tg.env)
			}
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("go build %s: %v\n%s", pkg, err, out)
			}
			if m := fusedOp.FindAllString(string(out), -1); len(m) != 0 {
				t.Errorf("%s: %d fused multiply-add instructions in %s", tg.arch, len(m), pkg)
			}
		})
	}
}
