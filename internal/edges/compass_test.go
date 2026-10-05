// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package edges

import (
	"math/rand/v2"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"testing"
)

func TestDirectionBasics(t *testing.T) {
	var names []string
	for k, d := range Directions {
		names = append(names, d.String())
		if d.Degrees() != 45*float64(k) {
			t.Errorf("%s.Degrees() = %v", d, d.Degrees())
		}
		if d.Opposite().Opposite() != d || d.Opposite() == d {
			t.Errorf("%s.Opposite() = %s", d, d.Opposite())
		}
	}
	if want := []string{"N", "NE", "E", "SE", "S", "SW", "W", "NW"}; !slices.Equal(names, want) {
		t.Errorf("names %q, want %q", names, want)
	}
	if N.Opposite() != S || NE.Opposite() != SW || W.Opposite() != E {
		t.Error("opposites")
	}
	if Direction(9).String() != "Direction(9)" {
		t.Error("out-of-range name")
	}
	for _, tc := range []struct {
		b    float64
		d    Direction
		want float64
	}{
		{0, N, 0}, {350, N, 10}, {10, N, 10}, {359.5, NW, 44.5}, {180, N, 180}, {100, E, 10}, {200, NE, 155}, {22.5, NE, 22.5},
	} {
		if got := AngularError(tc.b, tc.d); got != tc.want {
			t.Errorf("AngularError(%v, %s) = %v, want %v", tc.b, tc.d, got, tc.want)
		}
	}
	for b, want := range map[float64]Direction{0: N, 22.5: N, 22.6: NE, 337.4: NW, 337.5: N, 359.9: N, 180: S, 202.5: S} {
		if got := Nearest(b); got != want {
			t.Errorf("Nearest(%v) = %s, want %s", b, got, want)
		}
	}
}

// TestAssignHandMade checks hand-made cells of 3 to 8 neighbors against
// assignments worked out by hand, each also cross-checked by brute force.
func TestAssignHandMade(t *testing.T) {
	for _, tc := range []struct {
		name     string
		bearings []float64
		want     []Direction
	}{
		{"3 spread", []float64{10, 130, 250}, []Direction{N, SE, W}},
		// The smallest bearing need not take the smallest code: the order
		// is cyclic, and the best fit wraps through north.
		{"3 wrap through north", []float64{340, 350, 355}, []Direction{NW, N, NE}},
		// Max tie-break: NE E SE and E SE S both total 75°; the second's
		// largest error is 37.5° against 52.5°.
		{"3 max tie-break", []float64{142.5, 97.5, 105}, []Direction{S, E, SE}},
		{"4 square", []float64{0, 90, 180, 270}, []Direction{N, E, S, W}},
		{"4 diagonal, unsorted", []float64{224, 44, 314, 134}, []Direction{SW, NE, NW, SE}},
		// 80° and 100° are both nearest E (the naive label collides).
		// N NE E S W and N E SE S W both total 75° with largest 35°; the
		// sequence tie-break takes the one with NE (codes 0 1 2 4 6 before
		// 0 2 3 4 6).
		{"5 cluster, sequence tie-break", []float64{10, 80, 100, 190, 280}, []Direction{N, NE, E, S, W}},
		{"6 hexagon", []float64{0, 60, 120, 180, 240, 300}, []Direction{N, NE, SE, S, SW, NW}},
		{"6 cluster east", []float64{70, 85, 100, 115, 250, 330}, []Direction{NE, E, SE, S, W, NW}},
		{"7", []float64{0, 45, 90, 135, 180, 225, 300}, []Direction{N, NE, E, SE, S, SW, NW}},
		{"8 aligned", []float64{0, 45, 90, 135, 180, 225, 270, 315}, Directions},
		// Every bearing halfway between two points: the two rotations
		// tie on total and largest error, and the one that starts at the
		// earliest compass point (N for the smallest bearing) wins.
		{"8 halfway, start tie-break", []float64{22.5, 67.5, 112.5, 157.5, 202.5, 247.5, 292.5, 337.5}, Directions},
		// Starting at N for 350° would total 185°; rotating one point
		// back totals 140°.
		{"8 skewed", []float64{350, 30, 50, 100, 150, 190, 240, 290}, []Direction{NW, N, NE, E, SE, S, SW, W}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Assign(tc.bearings)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("Assign(%v) = %v, want %v", tc.bearings, got, tc.want)
			}
			if brute, _ := bruteAssign(tc.bearings); !slices.Equal(got, brute) {
				t.Errorf("Assign(%v) = %v, brute force %v", tc.bearings, got, brute)
			}
			checkAssignment(t, tc.bearings, got)
		})
	}
}

// checkAssignment checks the invariants of an assignment: distinct
// directions, in the cyclic order of the bearings.
func checkAssignment(t *testing.T, bearings []float64, dirs []Direction) {
	t.Helper()
	seen := map[Direction]bool{}
	for _, d := range dirs {
		if d >= numDirections || seen[d] {
			t.Fatalf("directions %v repeat or are out of range", dirs)
		}
		seen[d] = true
	}
	order := sortedByBearing(bearings)
	gaps := 0
	for j := range order {
		a, b := dirs[order[j]], dirs[order[(j+1)%len(order)]]
		gaps += (int(b) - int(a) + 8) % 8
	}
	if len(order) > 1 && gaps != 8 {
		t.Fatalf("directions %v for bearings %v are not in cyclic order (gaps sum %d)", dirs, bearings, gaps)
	}
}

// TestAssignBruteForce cross-checks the dynamic program against an
// enumeration of every order-preserving assignment, on random bearing sets
// of 0 to 8 edges: continuous bearings, and bearings on a 7.5° grid, which
// makes ties in the total and the largest error common.
func TestAssignBruteForce(t *testing.T) {
	r := rand.New(rand.NewPCG(22, 32))
	binom := func(n, k int) int {
		v := 1
		for i := range k {
			v = v * (n - i) / (i + 1)
		}
		return v
	}
	n := 4000
	if testing.Short() {
		n = 500
	}
	ties := 0
	for trial := range n {
		k := r.IntN(MaxDegree + 1)
		b := make([]float64, k)
		for i := range b {
			if trial%2 == 0 {
				b[i] = 7.5 * float64(r.IntN(48))
			} else {
				b[i] = 360 * r.Float64()
			}
		}
		got, err := Assign(b)
		if err != nil {
			t.Fatal(err)
		}
		want, count := bruteAssign(b)
		if !slices.Equal(got, want) {
			t.Fatalf("Assign(%v) = %v, brute force %v", b, got, want)
		}
		if k > 0 && count != binom(8, k)*k {
			t.Fatalf("%d order-preserving assignments of %d edges, want C(8,%d)·%d = %d", count, k, k, k, binom(8, k)*k)
		}
		checkAssignment(t, b, got)
		if k > 0 {
			var c costs
			for i, j := range sortedByBearing(b) {
				for _, d := range Directions {
					c[i][d] = cost(b[j], d)
				}
			}
			if dp(&c, k, 1<<62).count > 1 {
				ties++
			}
		}
	}
	if ties == 0 {
		t.Error("no trial had tied totals; the tie-breaks went unexercised")
	}
	t.Logf("%d trials, %d with tied totals", n, ties)
}

func TestAssignErrors(t *testing.T) {
	if _, err := Assign(make([]float64, 9)); err == nil {
		t.Error("Assign accepted 9 edges")
	}
	for _, b := range []float64{-1, 360, 400} {
		if _, err := Assign([]float64{b}); err == nil {
			t.Errorf("Assign accepted bearing %v", b)
		}
	}
	if got, err := Assign(nil); err != nil || len(got) != 0 {
		t.Errorf("Assign(nil) = %v, %v", got, err)
	}
	if got, _ := Assign([]float64{22.5}); !slices.Equal(got, []Direction{N}) {
		t.Errorf("Assign(22.5) = %v, want N (tie to the earliest point)", got)
	}
}

// TestNoFusedMultiplyAdd compiles this package for the architectures whose Go
// compilers fuse a*b+c into one instruction and fails if any fused
// multiply-add or multiply-subtract appears in its code.
func TestNoFusedMultiplyAdd(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles for several architectures")
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool not found")
	}
	fusedOp := regexp.MustCompile(`\tV?FN?M(ADD|SUB)`)
	const pkg = "github.com/mdhender/mpg/internal/edges"
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
