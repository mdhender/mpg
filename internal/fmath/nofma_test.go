// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package fmath

import (
	"os"
	"os/exec"
	"regexp"
	"testing"
)

// fusedOp matches the fused multiply-add family on every target Go fuses for:
// FMADD, FMSUB, FNMADD, FNMSUB (arm64, ppc64, s390x, riscv64, loong64, with
// size suffixes) and VFMADD..., VFNMSUB... (amd64 at GOAMD64=v3 and above).
var fusedOp = regexp.MustCompile(`\tV?FN?M(ADD|SUB)`)

// TestNoFusedMultiplyAdd compiles this package for each architecture whose
// backend contracts a*b + c and checks that the generated code holds no fused
// instruction. It is the static half of the cross-architecture contract; the
// pinned bit patterns are the dynamic half, but they only run on the machine
// at hand. Package math is compiled alongside as a positive control, proving
// the pattern would see fusion if it were there.
func TestNoFusedMultiplyAdd(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles for several architectures")
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool not found")
	}
	const pkg = "github.com/mdhender/mpg/internal/fmath"
	targets := []struct{ arch, env string }{
		{"arm64", ""},
		{"amd64", "GOAMD64=v3"},
		{"ppc64le", ""},
		{"s390x", ""},
		{"riscv64", ""},
		{"loong64", ""},
	}
	for _, tg := range targets {
		t.Run(tg.arch, func(t *testing.T) {
			asm := func(target string) string {
				t.Helper()
				cmd := exec.Command(goTool, "build", "-gcflags="+target+"=-S", target)
				cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+tg.arch, "CGO_ENABLED=0")
				if tg.env != "" {
					cmd.Env = append(cmd.Env, tg.env)
				}
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("go build %s: %v\n%s", target, err, out)
				}
				return string(out)
			}
			if !fusedOp.MatchString(asm("math")) {
				t.Fatalf("no fused instruction found in package math for %s; the pattern is not measuring anything", tg.arch)
			}
			if m := fusedOp.FindAllString(asm(pkg), -1); len(m) != 0 {
				t.Errorf("%s: %d fused multiply-add instructions in %s; build with -gcflags=%s=-S to find them", tg.arch, len(m), pkg, pkg)
			}
		})
	}
}
