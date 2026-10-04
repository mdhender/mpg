// MIT License: See https://github.com/pzsz/voronoi/LICENSE.md

// Author: Przemyslaw Szczepaniak (przeszczep@gmail.com)
// Port of Raymond Hill's (rhill@raymondhill.net) javascript implementation
// of Steven Forune's algorithm to compute Voronoi diagrams

package voronoi

import (
	"math/rand/v2"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"testing"
)

func verifyDiagram(t *testing.T, diagram *Diagram, err error, edgesCount, cellsCount, perCellCount int) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if len(diagram.Edges) != edgesCount {
		t.Errorf("Expected %d edges not %d", edgesCount, len(diagram.Edges))
	}

	if len(diagram.Cells) != cellsCount {
		t.Errorf("Expected %d cells not %d", cellsCount, len(diagram.Cells))
	}

	if perCellCount > 0 {
		for _, cell := range diagram.Cells {
			if len(cell.Halfedges) != perCellCount {
				t.Errorf("Expected per cell edge count expected %d, not %d", perCellCount, len(cell.Halfedges))
			}
		}
	}
}

func TestVoronoi2Points(t *testing.T) {
	sites := []Vertex{
		{4, 5},
		{6, 5},
	}

	d, err := ComputeDiagram(sites, NewBBox(0, 10, 0, 10), true)
	verifyDiagram(t, d, err, 7, 2, 4)
	d, err = ComputeDiagram(sites, NewBBox(0, 10, 0, 10), false)
	verifyDiagram(t, d, err, 1, 2, 1)
}

func TestVoronoi3Points(t *testing.T) {
	sites := []Vertex{
		{4, 5},
		{6, 5},
		{5, 8},
	}

	d, err := ComputeDiagram(sites, NewBBox(0, 10, 0, 10), true)
	verifyDiagram(t, d, err, 10, 3, -1)
	d, err = ComputeDiagram(sites, NewBBox(0, 10, 0, 10), false)
	verifyDiagram(t, d, err, 3, 3, 2)
}

// TestIndexAndInput checks the mpg changes: the input is not reordered,
// each cell carries its site's index, and a duplicate site keeps only the
// lower index.
func TestIndexAndInput(t *testing.T) {
	sites := []Vertex{{5, 8}, {6, 5}, {4, 5}, {6, 5}}
	in := slices.Clone(sites)
	d, err := ComputeDiagram(sites, NewBBox(0, 10, 0, 10), true)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(sites, in) {
		t.Errorf("ComputeDiagram reordered its input: %v", sites)
	}
	var got []int
	for _, c := range d.Cells {
		if sites[c.Index] != c.Site {
			t.Errorf("cell index %d has site %v, want %v", c.Index, c.Site, sites[c.Index])
		}
		got = append(got, c.Index)
	}
	slices.Sort(got)
	if !slices.Equal(got, []int{0, 1, 2}) {
		t.Errorf("cell indices %v, want [0 1 2] (the duplicate 3 dropped)", got)
	}
}

// TestDeterministic runs the same random sites twice, once shuffled, and
// requires the same cells with the same half-edge geometry.
func TestDeterministic(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	sites := make([]Vertex, 500)
	for i := range sites {
		sites[i] = Vertex{r.Float64() * 100, r.Float64() * 100}
	}
	ring := func(d *Diagram) map[int][]Vertex {
		m := map[int][]Vertex{}
		for _, c := range d.Cells {
			for _, h := range c.Halfedges {
				m[c.Index] = append(m[c.Index], h.GetStartpoint())
			}
		}
		return m
	}
	a, err := ComputeDiagram(sites, NewBBox(0, 100, 0, 100), true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ComputeDiagram(sites, NewBBox(0, 100, 0, 100), true)
	if err != nil {
		t.Fatal(err)
	}
	ra, rb := ring(a), ring(b)
	if len(ra) != len(sites) {
		t.Fatalf("%d cells for %d sites", len(ra), len(sites))
	}
	for i := range sites {
		if !slices.Equal(ra[i], rb[i]) {
			t.Fatalf("cell %d differs between runs", i)
		}
	}
}

// TestNoFusedMultiplyAdd compiles this package for the architectures whose Go
// backends contract a*b + c and checks the code holds no fused instruction.
func TestNoFusedMultiplyAdd(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles for several architectures")
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool not found")
	}
	fusedOp := regexp.MustCompile(`\tV?FN?M(ADD|SUB)`)
	const pkg = "github.com/mdhender/mpg/internal/mesh/voronoi"
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

func BenchmarkCompute1000(b *testing.B) {
	random := rand.New(rand.NewPCG(1234567, 0))
	sites := make([]Vertex, 1000)
	for j := range sites {
		sites[j].X = random.Float64() * 100
		sites[j].Y = random.Float64() * 100
	}
	for b.Loop() {
		ComputeDiagram(sites, NewBBox(0, 100, 0, 100), true)
	}
}
