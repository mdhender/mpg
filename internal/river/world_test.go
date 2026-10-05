// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package river_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"testing"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/pipeline"
	"github.com/mdhender/mpg/internal/river"
)

// TestWorlds builds the tree on real worlds, through the pipeline's land
// target, and checks every invariant of CheckTree (acyclic; every land
// corner reaches a terminal; tree edges land–land only, never beside the
// rim; outlets never re-enter their lake), that the pipeline's river stage
// builds the same tree, that no lake needed the fallback, and that the
// catchments agreement is reported. Small worlds have a narrow falloff, so
// their land comes closest to the rim.
func TestWorlds(t *testing.T) {
	for _, tc := range []struct {
		seed           uint64
		aspect, preset string
		land, falloff  int
	}{
		{42, "cinematic", "continents", 10_000, 12},
		{7, "square", "pangaea", 10_000, 12},
		{5, "cinematic", "pangaea", 10_000, 12},
		{2, "cinematic", "pangaea", 10_000, 12}, // an outlet between two lakes
		{8, "square", "pangaea", 10_000, 12},
		{3, "cinematic", "archipelago", 2_000, 4},
		{5, "portrait", "pangaea", 800, 4},
		{12, "cinematic", "continents", 800, 0},
	} {
		t.Run(fmt.Sprintf("seed%d-%s-%s-%d", tc.seed, tc.aspect, tc.preset, tc.land), func(t *testing.T) {
			if testing.Short() && tc.land > 2_000 {
				t.Skip("large world")
			}
			t.Parallel()
			c := config.Default()
			c.Seed = config.Seed(tc.seed)
			c.World.Aspect = tc.aspect
			c.Layout.Preset = tc.preset
			c.World.LandCells = tc.land
			c.Rim.FalloffCells = tc.falloff
			ctx, err := pipeline.NewContext(c, t.TempDir(), "", nil)
			if err != nil {
				t.Fatal(err)
			}
			stages := pipeline.Stages()
			last, err := pipeline.Lookup(stages, "rivers")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pipeline.Run(ctx, stages, last); err != nil {
				t.Fatal(err)
			}
			p := ctx.Products
			in := pipeline.RiverInput(p.Mesh, p.Cells.Altitude, p.Target)
			tr, err := river.Build(in)
			if err != nil {
				t.Fatal(err)
			}
			river.CheckTree(t, in, tr)
			b1, _ := tr.AppendBinary(nil)
			b2, _ := p.Rivers.AppendBinary(nil)
			if !bytes.Equal(b1, b2) {
				t.Error("the river stage's tree differs from a second build")
			}
			if tr.Fallbacks != 0 {
				t.Errorf("%d fallback outlets", tr.Fallbacks)
			}
			a := tr.Agreement
			if a == nil || a.Cells != p.Target.LandCells || a.Differ > a.Cells || a.FinalDiffer > a.Cells {
				t.Errorf("agreement %+v for %d land cells", a, p.Target.LandCells)
			}
			s := tr.Stats(p.Mesh)
			t.Logf("%d land corners, %d tree edges (%d seam, %d flat, %d climb); %d overflowing lakes, %d at the spill corner, drains %v; differ %d/%d, finally %d",
				s.LandCorners, s.Edges, s.Seam, s.Flat, s.Climb, s.Overflowing, s.AtSpill, s.Drains, a.Differ, a.Cells, a.FinalDiffer)
		})
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
	const pkg = "github.com/mdhender/mpg/internal/river"
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
