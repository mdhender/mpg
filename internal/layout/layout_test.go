// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package layout

import (
	"math"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"sync"
	"testing"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/topo"
)

var aspects = []string{"cinematic", "square", "portrait"}

// TestPresetIntent checks that each seeded preset's bias field shows its
// intent across seeds and aspects, by counting the connected regions of
// positive bias (wrapping east–west).
func TestPresetIntent(t *testing.T) {
	if testing.Short() {
		t.Skip("builds many fields")
	}
	var mu sync.Mutex
	mean := map[string][]float64{}
	t.Run("group", func(t *testing.T) {
		for _, aspect := range aspects {
			for _, preset := range []string{"pangaea", "continents", "archipelago", "islands"} {
				t.Run(aspect+"/"+preset, func(t *testing.T) {
					t.Parallel()
					for seed := uint64(1); seed <= 8; seed++ {
						checkIntent(t, aspect, preset, seed, &mu, mean)
					}
				})
			}
		}
	})
	// Mass sizes fall from continents to archipelago to islands.
	avg := func(p string) float64 {
		var s float64
		for _, v := range mean[p] {
			s += v
		}
		return s / float64(len(mean[p]))
	}
	if !(avg("pangaea") > avg("continents") && avg("continents") > 2*avg("archipelago") && avg("archipelago") > 2*avg("islands")) {
		t.Errorf("mean region sizes pangaea %.0f, continents %.0f, archipelago %.0f, islands %.0f do not fall steeply",
			avg("pangaea"), avg("continents"), avg("archipelago"), avg("islands"))
	}
}

// checkIntent checks one seed of TestPresetIntent and records its mean
// positive-region size in mean[preset].
func checkIntent(t *testing.T, aspect, preset string, seed uint64, mu *sync.Mutex, mean map[string][]float64) {
	c := resolved(t, seed, preset, aspect)
	l, f := build(t, c)
	sizes := components(f)
	total := 0
	for _, s := range sizes {
		total += s
	}
	largest := float64(sizes[0]) / float64(total)
	mu.Lock()
	mean[preset] = append(mean[preset], float64(total)/float64(len(sizes)))
	mu.Unlock()
	p, _ := c.Layout.Pattern(preset)
	if l.Masses < p.MassesMin || l.Masses > p.MassesMax {
		t.Errorf("%s %s seed %d: %d masses, want [%d, %d]", aspect, preset, seed, l.Masses, p.MassesMin, p.MassesMax)
	}
	comps := len(sizes)
	var ok bool
	switch preset {
	case "pangaea": // one dominant mass
		ok = l.Masses == 1 && comps == 1
	case "continents": // 3–5 separated masses
		ok = comps >= 3 && comps <= 5 && comps >= l.Masses-1 && largest <= 0.6
	case "archipelago": // many mid-size masses
		ok = comps >= 8 && comps <= 14 && largest <= 0.25
	case "islands": // many small masses
		ok = comps >= 20 && largest <= 0.1
	}
	if !ok {
		t.Errorf("%s %s seed %d: %d masses gave %d positive regions, largest %.2f of the positive area", aspect, preset, seed, l.Masses, comps, largest)
	}
	// The positive area is roughly the land target's share of
	// the band outside the falloff (N·A over the band's area).
	band := c.World.WidthKm * (c.World.HeightKm - 2*(c.Rim.Km+c.Rim.FalloffKm))
	want := float64(c.World.LandCells) * c.Province.AreaKm2 / band
	if got := PositiveShare(f); got < 0.7*want || got > 1.3*want {
		t.Errorf("%s %s seed %d: positive share %.3f, want about %.3f", aspect, preset, seed, got, want)
	}
}

// TestAttractorsOutsideRimAndFalloff checks that every seeded attractor lies
// in the playable band, inset by the pole margin.
func TestAttractorsOutsideRimAndFalloff(t *testing.T) {
	for _, aspect := range aspects {
		for _, preset := range []string{"pangaea", "continents", "archipelago", "islands"} {
			for seed := uint64(1); seed <= 6; seed++ {
				c := resolved(t, seed, preset, aspect)
				l, err := New(c)
				if err != nil {
					t.Fatal(err)
				}
				cyl := l.Cylinder()
				for k, a := range l.Attractors {
					edge := cyl.RimDistance(a.Y) - cyl.Falloff()
					if cyl.InRim(a.Y) || cyl.InFalloff(a.Y) || edge < c.Layout.PoleMargin*a.RadiusKm*(1-1e-9) {
						t.Errorf("%s %s seed %d: attractor %d at y %.1f is %.1f km from the falloff, radius %.1f", aspect, preset, seed, k, a.Y, edge, a.RadiusKm)
					}
					if !(a.X >= 0 && a.X < cyl.W()) {
						t.Errorf("attractor %d x %v not wrapped", k, a.X)
					}
					if !(a.Weight > 0 && a.Weight <= 1) || !(a.RadiusKm > 0) {
						t.Errorf("attractor %d = %+v", k, a)
					}
				}
			}
		}
	}
}

// custom returns the default config at the given aspect with the custom
// preset, the given attractors and repulsors, and no wobble.
func custom(t *testing.T, attractors []config.CustomAttractor, repulsors []config.CustomRepulsor, repulse float64) config.Config {
	t.Helper()
	c := config.Default()
	c.Seed = 9
	c.Layout.Preset = config.PresetCustom
	c.Layout.Custom.Attractors = attractors
	if repulsors != nil {
		c.Layout.Custom.Repulsors = repulsors
	}
	c.Layout.Custom.Rivals.Wobble = 0
	c.Layout.Custom.Rivals.RepulsorWeight = repulse
	if err := c.Resolve(); err != nil {
		t.Fatal(err)
	}
	return c
}

// TestSeam checks that an attractor near x = 0 raises the bias just west
// of the seam, at x ≈ W, and that distance falls off the same both ways.
func TestSeam(t *testing.T) {
	c := config.Default()
	if err := c.Resolve(); err != nil {
		t.Fatal(err)
	}
	w, h := c.World.WidthKm, c.World.HeightKm
	c = custom(t, []config.CustomAttractor{{XKm: 5, YKm: h / 2, RadiusKm: 100, Weight: 1}}, nil, 0)
	_, f := build(t, c)
	east, west := f.Sample(5+60, h/2), f.Sample(w-55, h/2)
	if !(west > 0) {
		t.Errorf("bias at x = W − 55 is %v; the attractor at x = 5 does not reach across the seam", west)
	}
	if math.Abs(east-west) > 1e-3 {
		t.Errorf("bias 60 km east %v and 60 km west (across the seam) %v differ", east, west)
	}
	if v := f.Sample(w/2, h/2); v != Ocean {
		t.Errorf("bias half way round = %v, want %v", v, Ocean)
	}
	// The last column and the first are neighbors: no step at the seam.
	for j := range f.NY() {
		if d := math.Abs(f.At(f.NX()-1, j) - f.At(0, j)); d > 0.05 {
			t.Fatalf("row %d: seam step %v", j, d)
		}
	}
}

// TestSeamPeriodicSeeded checks a seeded field for a step at the seam no
// larger than the steps between other neighboring columns.
func TestSeamPeriodicSeeded(t *testing.T) {
	_, f := build(t, resolved(t, 3, "islands", "cinematic"))
	nx := f.NX()
	var maxStep, seamStep float64
	for j := range f.NY() {
		for i := range nx - 1 {
			maxStep = max(maxStep, math.Abs(f.At(i+1, j)-f.At(i, j)))
		}
		seamStep = max(seamStep, math.Abs(f.At(0, j)-f.At(nx-1, j)))
	}
	if seamStep > maxStep {
		t.Errorf("seam step %v exceeds the largest interior step %v", seamStep, maxStep)
	}
}

// TestRepulsorLowersBias checks that rival masses get a repulsor in the gap
// between them, which lowers the bias there, and that kin attractors do not.
func TestRepulsorLowersBias(t *testing.T) {
	c := config.Default()
	if err := c.Resolve(); err != nil {
		t.Fatal(err)
	}
	h := c.World.HeightKm
	attractors := []config.CustomAttractor{
		{XKm: 1000, YKm: h / 2, RadiusKm: 100, Weight: 1, Mass: 7},
		{XKm: 1150, YKm: h / 2, RadiusKm: 100, Weight: 1, Mass: 7}, // kin
		{XKm: 1400, YKm: h / 2, RadiusKm: 120, Weight: 1, Mass: 3}, // rival
	}
	with, fWith := build(t, custom(t, attractors, nil, 0.8))
	without, fWithout := build(t, custom(t, attractors, nil, 0))
	if len(without.Repulsors) != 0 {
		t.Fatalf("repulsor weight 0 placed %d repulsors", len(without.Repulsors))
	}
	if len(with.Repulsors) != 1 {
		t.Fatalf("got %d repulsors, want 1 (between the rivals only): %+v", len(with.Repulsors), with.Repulsors)
	}
	r := with.Repulsors[0]
	// Masses are renumbered in id order: 3 → 0, 7 → 1.
	if r.MassA != 0 || r.MassB != 1 {
		t.Errorf("repulsor separates masses %d and %d, want 0 and 1", r.MassA, r.MassB)
	}
	// The closest rival pair is (1150, r 100) and (1400, r 120): a 30 km
	// gap from 1250 to 1280, so the repulsor sits at 1265.
	if math.Abs(r.X-1265) > 1e-9 || r.Y != h/2 {
		t.Errorf("repulsor at (%v, %v), want (1265, %v)", r.X, r.Y, h/2)
	}
	if b, a := fWithout.Sample(1265, h/2), fWith.Sample(1265, h/2); !(a < b-0.3) {
		t.Errorf("bias in the gap %v with the repulsor, %v without; want it lowered", a, b)
	}
	// Far from the gap nothing changes.
	if a, b := fWith.Sample(1000, h/2), fWithout.Sample(1000, h/2); a != b {
		t.Errorf("repulsor changed the bias at a far attractor: %v vs %v", a, b)
	}
	// Seeded presets: every repulsor joins two different masses.
	l, _ := build(t, resolved(t, 2, "archipelago", "cinematic"))
	if len(l.Repulsors) == 0 {
		t.Error("archipelago placed no repulsors")
	}
	for _, r := range l.Repulsors {
		if !(r.MassA >= 0 && r.MassA < r.MassB && r.MassB < l.Masses) {
			t.Errorf("repulsor %+v does not join two rival masses", r)
		}
	}
}

// TestDeterminism checks that the same seed gives identical layouts and
// fields, and that a different seed gives a different one.
func TestDeterminism(t *testing.T) {
	for _, preset := range []string{"pangaea", "continents", "archipelago", "islands"} {
		l1, f1 := build(t, resolved(t, 11, preset, "cinematic"))
		l2, f2 := build(t, resolved(t, 11, preset, "cinematic"))
		l3, f3 := build(t, resolved(t, 12, preset, "cinematic"))
		if !slices.Equal(l1.Attractors, l2.Attractors) || !slices.Equal(l1.Repulsors, l2.Repulsors) {
			t.Errorf("%s: the same seed placed different layouts", preset)
		}
		if !slices.Equal(f1.Values(), f2.Values()) {
			t.Errorf("%s: the same seed gave different fields", preset)
		}
		if slices.Equal(l1.Attractors, l3.Attractors) || slices.Equal(f1.Values(), f3.Values()) {
			t.Errorf("%s: seeds 11 and 12 gave the same layout", preset)
		}
	}
}

// TestCustomHonorsList checks that the custom preset uses exactly the
// configured attractors and repulsors.
func TestCustomHonorsList(t *testing.T) {
	c := config.Default()
	if err := c.Resolve(); err != nil {
		t.Fatal(err)
	}
	h := c.World.HeightKm
	attractors := []config.CustomAttractor{
		{XKm: 300, YKm: h / 2, RadiusKm: 150, Weight: 0.9, Mass: 2},
		{XKm: 1800, YKm: h/2 + 100, RadiusKm: 80, Weight: 0.5, Mass: 1},
		{XKm: 400, YKm: h/2 - 50, RadiusKm: 120, Weight: 0.7, Mass: 2},
	}
	repulsors := []config.CustomRepulsor{{XKm: 1000, YKm: h / 2, RadiusKm: 60, Weight: 0.5}}
	cc := custom(t, attractors, repulsors, 0)
	l, f := build(t, cc)
	if l.Preset != config.PresetCustom || l.Masses != 2 {
		t.Errorf("preset %q, %d masses", l.Preset, l.Masses)
	}
	// Grouped by mass (1 → 0, 2 → 1), configured order kept within a mass.
	want := []Attractor{
		{X: 1800, Y: h/2 + 100, RadiusKm: 80, Weight: 0.5, Mass: 0},
		{X: 300, Y: h / 2, RadiusKm: 150, Weight: 0.9, Mass: 1},
		{X: 400, Y: h/2 - 50, RadiusKm: 120, Weight: 0.7, Mass: 1},
	}
	if !slices.Equal(l.Attractors, want) {
		t.Errorf("attractors = %+v, want %+v", l.Attractors, want)
	}
	if want := []Repulsor{{X: 1000, Y: h / 2, RadiusKm: 60, Weight: 0.5, MassA: -1, MassB: -1}}; !slices.Equal(l.Repulsors, want) {
		t.Errorf("repulsors = %+v, want %+v", l.Repulsors, want)
	}
	// The bias peaks at each attractor's weight and crosses zero at its
	// radius (no wobble).
	near := func(x, y float64) float64 {
		i := int(x / f.PitchX())
		j := int(y / f.PitchY())
		return f.At(i, j)
	}
	if v := near(1800, h/2+100); math.Abs(v-0.5) > 0.01 {
		t.Errorf("bias at the 0.5 attractor = %v", v)
	}
	if v := f.Sample(1800+80, h/2+100); math.Abs(v) > 0.02 {
		t.Errorf("bias at the radius = %v, want about 0", v)
	}
	if v := f.Sample(1000, h/2); !(v < Ocean+1e-12) {
		t.Errorf("bias at the explicit repulsor = %v, want the floor", v)
	}
	// A custom preset needs attractors, and they must avoid the falloff.
	bad := config.Default()
	bad.Layout.Preset = config.PresetCustom
	if err := bad.Resolve(); err == nil {
		t.Error("custom preset with no attractors resolved")
	}
	bad.Layout.Custom.Attractors = []config.CustomAttractor{{XKm: 10, YKm: bad.Rim.Km + 1, RadiusKm: 50, Weight: 1}}
	bad.ClearDerived()
	if err := bad.Resolve(); err == nil {
		t.Error("custom attractor in the falloff resolved")
	}
}

// TestBiasRange checks every value is finite and in [−1, 1], and that the
// rim is ocean.
func TestBiasRange(t *testing.T) {
	for _, preset := range []string{"pangaea", "continents", "archipelago", "islands"} {
		_, f := build(t, resolved(t, 4, preset, "square"))
		cyl := f.Cylinder()
		f.Each(func(i, j int, v float64) {
			if math.IsNaN(v) || v < Ocean || v > Continental {
				t.Fatalf("%s: bias %v at (%d, %d)", preset, v, i, j)
			}
			if cyl.InRim(f.Y(j)) && v > 0 {
				t.Fatalf("%s: positive bias %v in the rim at (%d, %d)", preset, v, i, j)
			}
		})
		if lo, hi := f.MinMax(); lo != Ocean || hi <= 0.5 {
			t.Errorf("%s: bias range [%v, %v]", preset, lo, hi)
		}
	}
}

// TestRender checks the render's size and that it marks the attractors.
func TestRender(t *testing.T) {
	l, f := build(t, resolved(t, 1, "continents", "cinematic"))
	img := Render(f, l)
	if img.Bounds().Dx() != f.NX() || img.Bounds().Dy() != f.NY() {
		t.Fatalf("render is %v, field %d × %d", img.Bounds(), f.NX(), f.NY())
	}
	for _, a := range l.Attractors {
		p := topo.Point{X: a.X, Y: a.Y}
		i, j := int(p.X/f.PitchX()), int(p.Y/f.PitchY())
		if got := img.RGBAAt(i, j); got != attractorDot {
			t.Errorf("attractor at pixel (%d, %d) is %v, want the dot color", i, j, got)
		}
	}
}

// TestNoFusedMultiplyAdd compiles this package for the architectures whose Go
// backends contract a*b + c and checks the code holds no fused instruction.
// Package fmath's test of the same name shows the pattern would see one.
func TestNoFusedMultiplyAdd(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles for several architectures")
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool not found")
	}
	fusedOp := regexp.MustCompile(`\tV?FN?M(ADD|SUB)`)
	const pkg = "github.com/mdhender/mpg/internal/layout"
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
