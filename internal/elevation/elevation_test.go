// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package elevation

import (
	"fmt"
	"image"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"testing"

	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/layout"
	"github.com/mdhender/mpg/internal/render"
	"github.com/mdhender/mpg/internal/topo"
)

// wellBelow is how far below the target sea level the falloff band and rim
// must stay: the margin that makes "no land in the falloff" robust to the
// sea-level search moving the level about.
const wellBelow = 500

// TestSweep is the "done when" sweep: across seeds, presets, and aspects,
// no land can appear in the falloff or rim, the land fraction lies above
// 0 m, and the land at the target sea level shows each preset's intent.
func TestSweep(t *testing.T) {
	if testing.Short() {
		t.Skip("builds many worlds")
	}
	for _, aspect := range aspects {
		for _, preset := range presets {
			t.Run(aspect+"/"+preset, func(t *testing.T) {
				for seed := uint64(1); seed <= 6; seed++ {
					checkWorld(t, build(t, resolved(t, seed, preset, aspect)))
				}
			})
		}
	}
}

// checkWorld checks one world of TestSweep.
func checkWorld(t *testing.T, w world) {
	t.Helper()
	name := fmt.Sprintf("%s %s seed %d", w.cfg.World.Aspect, w.cfg.Layout.Preset, uint64(w.cfg.Seed))
	fc := w.cfg.Elevation.Falloff
	c := w.f.Cylinder()

	// The falloff guarantee: the band and rim never rise above the ceiling,
	// the rim never above the depth.
	var rimMax = math.Inf(-1)
	w.f.Each(func(_, j int, v float64) {
		if c.InRim(w.f.Y(j)) {
			rimMax = max(rimMax, v)
		}
	})
	band := BandMax(w.f)
	if band > fc.CeilingM || rimMax > fc.DepthM {
		t.Errorf("%s: falloff and rim reach %v m (ceiling %v), rim %v m (depth %v)", name, band, fc.CeilingM, rimMax, fc.DepthM)
	}

	// The datum puts the land fraction above 0 m, so the sea level that
	// meets the land target is near 0 m, far above the falloff band.
	level := SeaLevelForShare(w.f, w.cfg.World.LandFraction)
	if !(band < level-wellBelow) {
		t.Errorf("%s: target sea level %v m is within %v m of the falloff's highest %v m", name, level, wellBelow, band)
	}
	if share := LandShare(w.f, 0); !w.stats.Clamped && math.Abs(share-w.cfg.World.LandFraction) > 0.01 {
		t.Errorf("%s: %.3f of the samples above 0 m, want %v (shift %v)", name, share, w.cfg.World.LandFraction, w.stats.Shift)
	}
	if w.stats.Clamped && math.Abs(level) > 200 {
		t.Errorf("%s: shift clamped at %v, target sea level %v m", name, w.stats.Shift, level)
	}

	// Intent: the land at the target level follows the preset, and agrees
	// with the bias's sign mostly, but not everywhere: noise draws coasts.
	largest, regions1, agree := intent(w, level, 0.01)
	_, regions5, _ := intent(w, level, 0.05)
	var ok bool
	switch w.cfg.Layout.Preset {
	case "pangaea": // one dominant mass
		ok = largest >= 0.85 && regions5 == 1
	case "continents": // a few large masses
		ok = regions5 >= 2 && regions5 <= 6 && largest <= 0.8
	case "archipelago": // many mid-size masses
		ok = regions5 >= 6 && largest <= 0.35
	case "islands": // many small masses
		ok = regions1 >= 15 && largest <= 0.25
	}
	if !ok {
		t.Errorf("%s: largest region %.2f of the land, %d regions over 1%%, %d over 5%%", name, largest, regions1, regions5)
	}
	if agree < 0.8 || agree > 0.99 {
		t.Errorf("%s: land agrees with the bias sign at %.3f of the band, want in [0.8, 0.99]", name, agree)
	}
}

// TestFalloffShape checks the falloff alone: for any bedrock height it is
// continuous across the band's edge, never rises, and holds the band under
// the ceiling and the rim under the depth.
func TestFalloffShape(t *testing.T) {
	w := build(t, resolved(t, 1, "continents", "cinematic"))
	e, c, fc := w.elev, w.f.Cylinder(), w.cfg.Elevation.Falloff
	edge := c.Rim() + c.Falloff() // y of the band's inner edge (north)
	for _, h := range []float64{-6000, -1500, -1000, -200, 0, 50, 800, 3000, 9000} {
		prev := e.falloff(h, 0)
		for k := range 4001 {
			y := edge + fc.TaperKm + 10 - float64(k)*(edge+fc.TaperKm+10)/4000
			v := e.falloff(h, y)
			if v > h {
				t.Fatalf("h %v: falloff raised it to %v at y %v", h, v, y)
			}
			if (c.InRim(y) || c.InFalloff(y)) && v > fc.CeilingM {
				t.Fatalf("h %v: %v above the ceiling at y %v", h, v, y)
			}
			if c.InRim(y) && v > fc.DepthM {
				t.Fatalf("h %v: %v above the depth in the rim at y %v", h, v, y)
			}
			if k > 0 && math.Abs(v-prev) > 0.02*math.Max(math.Abs(h), 3000) {
				t.Fatalf("h %v: step %v → %v at y %v", h, prev, v, y)
			}
			prev = v
		}
		if v := e.falloff(h, edge+fc.TaperKm+1); v != h {
			t.Errorf("h %v: changed to %v inside the playable band beyond the taper", h, v)
		}
	}
}

// TestSeam checks there is no seam: the step from the last column to the
// first is like the step between any two neighboring columns, and the stage
// render of the field rolled half way round is the render rolled, so the
// seam shades like any column.
func TestSeam(t *testing.T) {
	for _, preset := range []string{"islands", "pangaea"} {
		w := build(t, resolved(t, 3, preset, "cinematic"))
		f := w.f
		nx, ny := f.NX(), f.NY()
		mean := func(i int) float64 {
			var s float64
			for j := range ny {
				s += math.Abs(f.At(i+1, j) - f.At(i, j))
			}
			return s / float64(ny)
		}
		seam := mean(nx - 1)
		var means []float64
		for i := range nx - 1 {
			means = append(means, mean(i))
		}
		slices.Sort(means)
		var sum float64
		for _, m := range means {
			sum += m
		}
		interior := sum / float64(len(means))
		if seam > means[len(means)-1] || seam/interior > 2 || seam/interior < 0.5 {
			t.Errorf("%s: seam mean step %v, interior mean %v (largest %v)", preset, seam, interior, means[len(means)-1])
		}

		rolled := f.Clone()
		half := nx / 2
		rolled.SetFunc(func(i, j int, _ topo.Point) float64 { return f.At(i+half, j) })
		a, b := Render(f, 0), Render(rolled, 0)
		for j := range ny {
			for i := range nx {
				if a.RGBAAt((i+half)%nx, j) != b.RGBAAt(i, j) {
					t.Fatalf("%s: rolled render differs at (%d, %d)", preset, i, j)
				}
			}
		}
		if dir := os.Getenv("MPG_RENDER_DIR"); dir != "" {
			for _, out := range []struct {
				name string
				img  image.Image
			}{{"seam-original", a}, {"seam-rolled", b}} {
				path := filepath.Join(dir, "elevation-"+preset+"-"+out.name+".png")
				if err := render.WritePNGFile(path, out.img, render.Meta{Stage: "elevation-" + out.name, ConfigHash: "test"}); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

// TestDeterminism checks that the same config gives the same bits, at any
// GOMAXPROCS, and that another seed gives another field.
func TestDeterminism(t *testing.T) {
	c := resolved(t, 11, "archipelago", "square")
	a := build(t, c)
	prev := runtime.GOMAXPROCS(1)
	b := build(t, c)
	runtime.GOMAXPROCS(prev)
	if !slices.Equal(a.f.Values(), b.f.Values()) || a.stats != b.stats || !slices.Equal(a.elev.Hotspots(), b.elev.Hotspots()) {
		t.Error("the same config gave different fields or hotspots at GOMAXPROCS 1 and default")
	}
	d := build(t, resolved(t, 12, "archipelago", "square"))
	if slices.Equal(a.f.Values(), d.f.Values()) {
		t.Error("seeds 11 and 12 gave the same field")
	}
}

// TestRidgesStayOnLand checks that ridges and the warp's choices never move
// a coast: with the ridges off, the land at 0 m is the same everywhere
// outside the falloff's taper, and with them on the land is higher.
func TestRidgesStayOnLand(t *testing.T) {
	c := resolved(t, 5, "continents", "cinematic")
	with := build(t, c)
	c.Elevation.Ridges.HeightM = 0
	without := build(t, c)
	if with.stats != without.stats {
		t.Fatalf("ridges changed the datum: %+v vs %+v", with.stats, without.stats)
	}
	cyl := with.f.Cylinder()
	taper := cyl.Falloff() + c.Elevation.Falloff.TaperKm
	var higher int
	with.f.Each(func(i, j int, v float64) {
		u := without.f.At(i, j)
		if cyl.RimDistance(with.f.Y(j)) >= taper && (v > 0) != (u > 0) {
			t.Fatalf("ridges moved the coast at (%d, %d): %v vs %v", i, j, v, u)
		}
		if v < u {
			t.Fatalf("ridges lowered (%d, %d): %v vs %v", i, j, v, u)
		}
		if v > u {
			higher++
		}
	})
	if higher == 0 {
		t.Error("ridges raised nothing")
	}
	if lo, hi := with.f.MinMax(); hi < 2000 || lo > -3000 {
		t.Errorf("range %v to %v m: want mountains and deep ocean", lo, hi)
	}
}

// TestNew checks New rejects a bias field off the config's world.
func TestNew(t *testing.T) {
	c := resolved(t, 1, "islands", "square")
	other := resolved(t, 1, "islands", "cinematic")
	l, err := layout.New(other)
	if err != nil {
		t.Fatal(err)
	}
	bias, err := l.Bias(other.Raster.SpacingKm)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(c, bias); err == nil {
		t.Error("New accepted a bias on another world")
	}
	if _, err := New(c, nil); err == nil {
		t.Error("New accepted a nil bias")
	}
}

// TestDatumOff checks that DatumMaxShift 0 leaves the signal unshifted.
func TestDatumOff(t *testing.T) {
	c := resolved(t, 2, "islands", "cinematic")
	c.Elevation.DatumMaxShift = 0
	w := build(t, c)
	if w.stats.Shift != 0 || w.stats.Clamped {
		t.Errorf("stats %+v, want no shift", w.stats)
	}
	if got := LandShare(w.f, 0); math.Abs(got-w.stats.Share) > 0.01 {
		t.Errorf("land share %v, signal share %v", got, w.stats.Share)
	}
}

// TestSeaLevelForShare checks the raster sea-level estimate on a ramp.
func TestSeaLevelForShare(t *testing.T) {
	c := resolved(t, 1, "islands", "square")
	f, err := field.FromConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	f.SetFunc(func(i, j int, _ topo.Point) float64 { return float64(i) })
	for _, share := range []float64{0.1, 0.3, 0.5} {
		level := SeaLevelForShare(f, share)
		if got := LandShare(f, level); math.Abs(got-share) > 2.0/float64(f.NX()) {
			t.Errorf("share %v: level %v leaves %v", share, level, got)
		}
	}
}

// TestRender checks the stage render's size, that land and water take their
// tints, that the hotspots are marked, and writes it when MPG_RENDER_DIR is
// set.
func TestRender(t *testing.T) {
	w := build(t, resolved(t, 1, "continents", "cinematic"))
	hs := w.elev.Hotspots()
	if len(hs) == 0 {
		t.Fatal("seed 1 drew no hotspots; pick another seed")
	}
	img := StageRender(w.f, 0, hs)
	plain := Render(w.f, 0)
	for _, h := range hs {
		p := render.ToPixel(w.f, h.Point())
		if got := img.RGBAAt(int(p.X), int(p.Y)); got != peakDot {
			t.Errorf("hotspot at (%v, %v): pixel %v, want the peak mark", h.X, h.Y, got)
		}
	}
	if img.Bounds().Dx() != w.f.NX() || img.Bounds().Dy() != w.f.NY() {
		t.Fatalf("render %v, field %d × %d", img.Bounds(), w.f.NX(), w.f.NY())
	}
	// The rim is deep water: blue dominates.
	if p := img.RGBAAt(0, 0); !(p.B > p.R && p.B > p.G) {
		t.Errorf("rim pixel %v is not water", p)
	}
	if dir := os.Getenv("MPG_RENDER_DIR"); dir != "" {
		for name, im := range map[string]image.Image{"03-elevation-test.png": img, "03-elevation-test-unmarked.png": plain} {
			if err := render.WritePNGFile(filepath.Join(dir, name), im, render.Meta{Stage: "03-elevation", ConfigHash: "test"}); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// TestMarkHotspotsSeam checks a mark that crosses the seam is drawn on both
// sides: a hotspot on the seam marks the first and last columns alike.
func TestMarkHotspotsSeam(t *testing.T) {
	w := build(t, resolved(t, 1, "continents", "square"))
	y := w.f.Cylinder().H() / 2
	hs := []Hotspot{{X: 0, Y: y, PeakM: 2000, ConeRadiusKm: 20, SwellM: 300, SwellRadiusKm: 100}}
	img := StageRender(w.f, 0, hs)
	nx := w.f.NX()
	j := int(render.ToPixel(w.f, hs[0].Point()).Y)
	if img.RGBAAt(0, j) != peakDot || img.RGBAAt(nx-1, j) != peakDot {
		t.Errorf("peak mark at the seam: column 0 %v, column %d %v", img.RGBAAt(0, j), nx-1, img.RGBAAt(nx-1, j))
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
	const pkg = "github.com/mdhender/mpg/internal/elevation"
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
