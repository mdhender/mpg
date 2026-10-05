// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package river_test

import (
	"bytes"
	"fmt"
	"image"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"testing"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/edges"
	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/pipeline"
	"github.com/mdhender/mpg/internal/river"
)

// TestWorlds builds the tree on real worlds, through the pipeline's land
// target, and checks every invariant of CheckTree (acyclic; every land
// corner reaches a terminal; tree edges land–land only, never beside the
// rim; outlets never re-enter their lake), that the pipeline's river stage
// builds the same tree, that no lake needed the fallback, and that the
// catchments agreement is reported. It then accumulates the river network
// and checks every invariant of CheckNetwork (drainage cell by cell and
// conserved, rivers only on land–land edges and never beside the rim,
// polylines covering every river edge once, split by main stem, ending at
// mouths or confluences), that the stage built the same network, and that
// the world has rivers; and on the first world, the river render. Small
// worlds have a narrow falloff, so their land comes closest to the rim.
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
			net, err := river.Accumulate(in, tr, pipeline.RiverFlow(p.Target), pipeline.RiverParams(&ctx.Config))
			if err != nil {
				t.Fatal(err)
			}
			river.CheckNetwork(t, in, tr, pipeline.RiverFlow(p.Target), net)
			n1, _ := net.AppendBinary(nil)
			n2, _ := p.Network.AppendBinary(nil)
			if !bytes.Equal(n1, n2) {
				t.Error("the river stage's network differs from a second accumulation")
			}
			ns := net.Stats(in, tr)
			if ns.RiverEdges == 0 || ns.Paths == 0 || ns.Mouths == 0 {
				t.Errorf("no rivers: %+v", ns)
			}
			if tc.seed == 42 {
				checkRender(t, p.Elevation, in, tr, net, p.Target.Flood.Level)
			}
			t.Logf("rivers %d (stream %d, river %d, major %d; seam %d): %.3f edges per land cell, %.1f km per 1000 km², %.1f%% of land cells; %d polylines, ends %v, %d mouths; longest %d edges %.0f km",
				ns.RiverEdges, ns.Edges[1], ns.Edges[2], ns.Edges[3], ns.Seam, ns.EdgesPerLandCell(), ns.KmPer1000Km2(), 100*ns.TouchShare(),
				ns.Paths, ns.Ends, ns.Mouths, ns.LongestEdges, ns.LongestKm)
			s := tr.Stats(p.Mesh)
			t.Logf("%d land corners, %d tree edges (%d seam, %d flat, %d climb); %d overflowing lakes, %d at the spill corner, drains %v; differ %d/%d, finally %d",
				s.LandCorners, s.Edges, s.Seam, s.Flat, s.Climb, s.Overflowing, s.AtSpill, s.Drains, a.Differ, a.Cells, a.FinalDiffer)
		})
	}
}

// checkRender checks the river render: it draws river ink exactly when
// there are rivers, wider by class, and the tree variant draws none.
func checkRender(t *testing.T, f *field.Field, in river.Input, tr *river.Tree, net *river.Network, level float64) {
	t.Helper()
	ink := func(img *image.RGBA) int {
		n := 0
		for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
			for x := img.Rect.Min.X; x < img.Rect.Max.X; x++ {
				if img.RGBAAt(x, y) == river.RiverInk {
					n++
				}
			}
		}
		return n
	}
	if n := ink(river.Render(f, in, tr, net, level)); n == 0 {
		t.Error("the river render draws no river ink")
	}
	none := *net
	none.Class = make([]edges.RiverClass, len(net.Class))
	if n := ink(river.Render(f, in, tr, &none, level)); n != 0 {
		t.Errorf("the river render without rivers draws %d river pixels", n)
	}
	if n := ink(river.TreeRender(f, in, tr, level)); n != 0 {
		t.Errorf("the tree render draws %d river pixels", n)
	}
	if w := river.RiverWidths(f, in.Mesh); !(w[0] >= 1 && w[0] < w[1] && w[1] < w[2]) {
		t.Errorf("river widths %v not increasing by class", w)
	}
	// Major rivers alone draw more ink at their width than at a stream's.
	major := *net
	major.Class = slices.Clone(net.Class)
	for e, c := range major.Class {
		if c != edges.MajorRiver {
			major.Class[e] = edges.RiverNone
		}
	}
	thin := *net
	thin.Class = slices.Clone(major.Class)
	for e, c := range thin.Class {
		if c == edges.MajorRiver {
			thin.Class[e] = edges.Stream
		}
	}
	if a, b := ink(river.Render(f, in, tr, &major, level)), ink(river.Render(f, in, tr, &thin, level)); a <= b {
		t.Errorf("major rivers draw %d pixels, as streams %d", a, b)
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
