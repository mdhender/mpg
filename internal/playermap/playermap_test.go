// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package playermap_test

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"testing"

	"github.com/mdhender/mpg/internal/classify"
	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/internal/pipeline"
	"github.com/mdhender/mpg/internal/playermap"
	"github.com/mdhender/mpg/internal/render"
	"github.com/mdhender/mpg/world"
)

// testWorld generates a small world once and returns it decoded from its
// world.json bytes, so the tests see only what the file holds.
var testWorld = sync.OnceValues(func() (*world.World, error) {
	c := config.Default()
	c.Seed = 42
	c.World.LandCells = 600
	c.Rim.FalloffCells = 4
	dir, err := os.MkdirTemp("", "playermap-test-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	ctx, err := pipeline.NewContext(c, dir, "", nil)
	if err != nil {
		return nil, err
	}
	res, err := pipeline.Run(ctx, pipeline.Stages(), -1)
	if err != nil {
		return nil, err
	}
	if res.NotImplemented != nil {
		return nil, fmt.Errorf("run stopped at %s", res.NotImplemented)
	}
	return world.Decode(bytes.NewReader(ctx.Products.WorldBytes))
})

func loadWorld(t *testing.T) *world.World {
	t.Helper()
	w, err := testWorld()
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func full(t *testing.T, w *world.World, scale float64) (*image.RGBA, playermap.Lattice) {
	t.Helper()
	img, l, err := playermap.RenderFull(w, scale)
	if err != nil {
		t.Fatal(err)
	}
	return img, l
}

// TestWindowMatchesCrop is S24's done-when: a window, across the seam or
// not, is pixel for pixel the matching crop of the full render.
func TestWindowMatchesCrop(t *testing.T) {
	w := loadWorld(t)
	W, H := w.Meta.WidthKm, w.Meta.HeightKm
	for _, scale := range []float64{1, 2.37, 4} {
		img, l := full(t, w, scale)
		if l.Width != int(fmath.Mul(W, scale)+0.5) {
			t.Errorf("scale %v: map width %d px for %v km", scale, l.Width, W)
		}
		for _, win := range []struct {
			name       string
			x, y, w, h float64
		}{
			{"seam", W - 100, 0.3 * H, 300, 0.25 * H},
			{"negative-x", -150, 0.4 * H, 260, 0.2 * H},
			{"far-negative-x", -3*W + 10, 0.1 * H, 50, 40},
			{"north-pole", W / 3, 0, 400, 150},
			{"south-pole-seam", W - 50, H - 150, 120, 150},
			{"interior", W / 2, H / 2, 200, 120},
			{"whole-width-from-middle", W / 2, 0, W, H},
			{"thin", W - 0.3, 10, 0.5, H - 20},
		} {
			f, err := l.Window(win.x, win.y, win.w, win.h)
			if err != nil {
				t.Fatalf("scale %v %s: %v", scale, win.name, err)
			}
			got, err := playermap.Render(w, l, f)
			if err != nil {
				t.Fatal(err)
			}
			want, err := playermap.Crop(img, l, f)
			if err != nil {
				t.Fatal(err)
			}
			if render.PixelHash(got) != render.PixelHash(want) {
				t.Errorf("scale %v %s (frame %+v): window differs from the crop of the full render in %d pixels",
					scale, win.name, f, diff(got, want))
			}
		}
	}
}

func diff(a, b *image.RGBA) int {
	n := 0
	for k := 0; k+3 < len(a.Pix) && k+3 < len(b.Pix); k += 4 {
		if !bytes.Equal(a.Pix[k:k+4], b.Pix[k:k+4]) {
			n++
		}
	}
	return n
}

// TestFullCoverage checks that the cell fills tile the map: every pixel is
// painted (the canvas starts transparent), and the render is the same
// twice.
func TestFullCoverage(t *testing.T) {
	w := loadWorld(t)
	for _, scale := range []float64{0.5, 1, 3.3} {
		img, _ := full(t, w, scale)
		for k := 3; k < len(img.Pix); k += 4 {
			if img.Pix[k] != 0xff {
				p := k / 4
				t.Fatalf("scale %v: pixel (%d, %d) not painted", scale, p%img.Rect.Dx(), p/img.Rect.Dx())
			}
		}
		again, _ := full(t, w, scale)
		if render.PixelHash(img) != render.PixelHash(again) {
			t.Errorf("scale %v: two renders differ", scale)
		}
	}
}

// TestRimIsIce checks that the polar band is a sheet of ice: every pixel
// of the rows within half the rim's depth of either pole is IceColor (no
// outlines between rim cells), and every rim cell's site pixel is ice.
func TestRimIsIce(t *testing.T) {
	w := loadWorld(t)
	img, l := full(t, w, 2)
	band := int(fmath.Mul(w.Meta.RimKm/2, l.SY))
	if band < 2 {
		t.Fatalf("rim band of %d rows is too thin to test", band)
	}
	for _, j := range []int{0, band - 1, l.Height - band, l.Height - 1} {
		for i := range l.Width {
			if c := img.RGBAAt(i, j); c != playermap.IceColor {
				t.Fatalf("pixel (%d, %d) = %v, want ice %v", i, j, c, playermap.IceColor)
			}
		}
	}
	rim := 0
	for i := range w.Cells {
		c := &w.Cells[i]
		if !c.HasFlag(world.FlagRim) {
			continue
		}
		rim++
		x, y := int(fmath.Mul(c.Site.X, l.SX)), int(fmath.Mul(c.Site.Y, l.SY))
		if got := img.RGBAAt(x, min(y, l.Height-1)); got != playermap.IceColor {
			t.Errorf("rim cell %d site pixel = %v, want ice", i, got)
		}
	}
	if rim == 0 {
		t.Error("world has no rim cells")
	}
}

// TestCoastline checks that the pixel at the midpoint of every coast edge
// is drawn in the coast ink (except under a volcano mark), and that the
// cells either side keep their fills at their sites.
func TestCoastline(t *testing.T) {
	w := loadWorld(t)
	img, l := full(t, w, 2)
	if len(w.Coastlines) == 0 {
		t.Fatal("world has no coastlines")
	}
	checked := 0
	for _, cl := range w.Coastlines {
		for _, e := range cl.Edges {
			ed := &w.Edges[e]
			if w.Cells[ed.Cells[0]].HasFlag(world.FlagVolcano) || w.Cells[ed.Cells[1]].HasFlag(world.FlagVolcano) {
				continue
			}
			a, b := w.Corners[ed.Corners[0]].Point, w.Corners[ed.Corners[1]].Point
			dx := b.X - a.X
			if dx > w.Meta.WidthKm/2 {
				dx -= w.Meta.WidthKm
			} else if dx < -w.Meta.WidthKm/2 {
				dx += w.Meta.WidthKm
			}
			mx := fmath.FloorModFloat(a.X+dx/2, w.Meta.WidthKm)
			my := (a.Y + b.Y) / 2
			x := fmath.FloorMod(int(fmath.Mul(mx, l.SX)), l.Width)
			y := int(fmath.Mul(my, l.SY))
			if got := img.RGBAAt(x, y); got != playermap.CoastColor {
				t.Errorf("coast edge %d midpoint pixel (%d, %d) = %v, want coast ink", e, x, y, got)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Error("no coast edge checked")
	}
	for i := range w.Cells {
		c := &w.Cells[i]
		if c.HasFlag(world.FlagRim) || c.HasFlag(world.FlagCoast) || c.HasFlag(world.FlagVolcano) || !c.Landform.IsLand() {
			continue
		}
		x, y := int(fmath.Mul(c.Site.X, l.SX)), int(fmath.Mul(c.Site.Y, l.SY))
		if got, want := img.RGBAAt(x, y), playermap.CellColor(c); got != want {
			// a site may sit on a faint border, which only darkens the fill
			if got.R > want.R || got.G > want.G || got.B > want.B {
				t.Errorf("land cell %d site pixel = %v, want %v", i, got, want)
			}
		}
	}
}

// TestPaletteMatchesClassify checks that the player map colors each code
// as the classification stage render does.
func TestPaletteMatchesClassify(t *testing.T) {
	for _, l := range classify.Landforms {
		for _, d := range append([]classify.Depth{classify.DepthNone}, classify.Depths...) {
			if l != classify.SaltWater && d != classify.DepthNone {
				continue
			}
			got := playermap.LandformColor(world.Landform(l.String()), world.Depth(d.String()))
			if want := classify.LandformColor(l, d); got != want {
				t.Errorf("%s/%s: %v, want %v", l, d, got, want)
			}
		}
	}
	if got := playermap.LandformColor("tundra", ""); got != playermap.UnknownColor {
		t.Errorf("unknown landform: %v", got)
	}
	rim := world.Cell{Landform: world.SaltWater, Depth: world.Deep, Flags: []world.CellFlag{world.FlagRim, world.FlagImpassable}}
	if got := playermap.CellColor(&rim); got != playermap.IceColor {
		t.Errorf("rim cell: %v, want ice", got)
	}
}

// TestInlandWaterColors checks the flags' colors: playas, salt lakes, and
// salt inland seas tinted over their depth band; fresh lakes and inland
// seas keep the landform's colors.
func TestInlandWaterColors(t *testing.T) {
	for _, tc := range []struct {
		name string
		c    world.Cell
		want color.RGBA
	}{
		{"playa", world.Cell{Landform: world.Flats, Flags: []world.CellFlag{world.FlagPlaya}}, playermap.PlayaColor},
		{"fresh lake", world.Cell{Landform: world.FreshWater, Water: world.Lake}, classify.LandformColor(classify.FreshWater, classify.DepthNone)},
		{"salt lake", world.Cell{Landform: world.FreshWater, Water: world.Lake, Flags: []world.CellFlag{world.FlagSalt}}, playermap.SaltLakeColor},
		{"fresh inland sea", world.Cell{Landform: world.SaltWater, Depth: world.Shallow, Water: world.InlandSea}, classify.LandformColor(classify.SaltWater, classify.Shallow)},
	} {
		if got := playermap.CellColor(&tc.c); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
	fresh := world.Cell{Landform: world.SaltWater, Depth: world.Open, Water: world.InlandSea}
	salt := fresh
	salt.Flags = []world.CellFlag{world.FlagSalt}
	f, s := playermap.CellColor(&fresh), playermap.CellColor(&salt)
	if f == s || s.A != 0xff {
		t.Errorf("salt inland sea %v, fresh %v: want a distinct opaque tint", s, f)
	}
}

func TestLatticeWindow(t *testing.T) {
	w := &world.World{Meta: world.Meta{WidthKm: 1000.4, HeightKm: 500.2, ProvinceAreaKm2: 80}}
	l, err := playermap.NewLattice(w, 1)
	if err != nil {
		t.Fatal(err)
	}
	if l.Width != 1000 || l.Height != 500 || l.SX != 1000/1000.4 || l.SY != 500/500.2 {
		t.Errorf("lattice = %+v", l)
	}
	kx := func(px float64) float64 { return px * 1000.4 / 1000 } // km at px pixels east
	ky := func(px float64) float64 { return px * 500.2 / 500 }
	if f := l.Full(); f != (playermap.Frame{Width: 1000, Height: 500}) {
		t.Errorf("full = %+v", f)
	}
	for _, tc := range []struct {
		x, y, w, h float64
		want       playermap.Frame
	}{
		{0, 0, 1000.4, 500.2, playermap.Frame{0, 0, 1000, 500}},
		{kx(900.5), ky(100.5), kx(300), ky(50), playermap.Frame{900, 100, 300, 50}},
		{kx(-99.5), 0, kx(200), ky(10), playermap.Frame{900, 0, 200, 10}},
		{kx(2010.5), 0, 1, 1, playermap.Frame{10, 0, 1, 1}},
		{kx(-2989.5), ky(7.9), 0.1, 0.1, playermap.Frame{10, 7, 1, 1}},
	} {
		f, err := l.Window(tc.x, tc.y, tc.w, tc.h)
		if err != nil || f != tc.want {
			t.Errorf("Window(%v, %v, %v, %v) = %+v, %v; want %+v", tc.x, tc.y, tc.w, tc.h, f, err, tc.want)
		}
	}
	for _, bad := range [][4]float64{{0, -1, 10, 10}, {0, 495, 10, 10}, {0, 0, 1100, 10}, {0, 0, 0, 10}, {0, 0, 10, -1}} {
		if f, err := l.Window(bad[0], bad[1], bad[2], bad[3]); err == nil {
			t.Errorf("Window(%v) = %+v, want an error", bad, f)
		}
	}
	for _, s := range []float64{0, -1, 1e9} {
		if _, err := playermap.NewLattice(w, s); err == nil {
			t.Errorf("NewLattice(scale %v) succeeded", s)
		}
	}
	if _, err := playermap.Render(w, l, playermap.Frame{Width: 1001, Height: 1}); err == nil {
		t.Error("Render accepted a frame wider than the map")
	}
}

// TestDamagedWorld checks that a world with a dangling reference gives an
// error rather than a panic.
func TestDamagedWorld(t *testing.T) {
	w := *loadWorld(t)
	w.Cells = append([]world.Cell(nil), w.Cells...)
	c := w.Cells[0]
	c.Corners = append([]int(nil), c.Corners...)
	c.Corners[0] = len(w.Corners)
	w.Cells[0] = c
	if _, _, err := playermap.RenderFull(&w, 1); err == nil {
		t.Error("RenderFull accepted a cell with a dangling corner")
	}
}

// TestWriteSample writes the test world's player map to MPG_RENDER_DIR
// when it is set, for inspection.
func TestWriteSample(t *testing.T) {
	dir := os.Getenv("MPG_RENDER_DIR")
	if dir == "" {
		t.Skip("MPG_RENDER_DIR not set")
	}
	w := loadWorld(t)
	img, _ := full(t, w, 2)
	path := filepath.Join(dir, "player-test.png")
	if err := render.WritePNGFile(path, img, render.Meta{Stage: "player", ConfigHash: w.Meta.ConfigHash}); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", path)
}

// TestNoFusedMultiplyAdd compiles this package for the architectures that
// fuse a*b + c and checks the code holds no fused instruction.
func TestNoFusedMultiplyAdd(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles for several architectures")
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool not found")
	}
	fusedOp := regexp.MustCompile(`\tV?FN?M(ADD|SUB)`)
	const pkg = "github.com/mdhender/mpg/internal/playermap"
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
