// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package elevation

import (
	"math"
	"slices"
	"testing"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/internal/seed"
	"github.com/mdhender/mpg/internal/topo"
)

// meanCount is the expected hotspot count of a resolved config.
func meanCount(c config.Config) float64 {
	return c.Volcanic.HotspotsPerMkm2 * c.World.PlayableAreaKm2 / 1e6
}

// TestHotspotCounts is the first "done when" item: counts vary across
// seeds, zero among them, and average out to the configured rate.
func TestHotspotCounts(t *testing.T) {
	c := resolved(t, 1, "continents", "cinematic")
	mean := meanCount(c)
	if math.Abs(mean-5.38) > 0.01 {
		t.Fatalf("default mean %v, want about 5.38 (DESIGN.md: about 5)", mean)
	}
	const seeds = 4000
	hist := map[int]int{}
	var sum, sum2 float64
	for s := range uint64(seeds) {
		c.Seed = config.Seed(s + 1)
		n := len(Hotspots(c))
		hist[n]++
		sum += float64(n)
		sum2 += float64(n * n)
	}
	m := sum / seeds
	variance := sum2/seeds - m*m
	t.Logf("%d seeds: mean %.3f (want %.3f), variance %.3f, %d with none, histogram %v", seeds, m, mean, variance, hist[0], hist)
	// The mean's standard error is √(5.38/4000) ≈ 0.037.
	if math.Abs(m-mean) > 0.15 {
		t.Errorf("mean count %v over %d seeds, want %v", m, seeds, mean)
	}
	if math.Abs(variance/mean-1) > 0.1 {
		t.Errorf("count variance %v, want about the mean %v (Poisson)", variance, mean)
	}
	// P(0) = e^−5.38 ≈ 0.46%, about 18 of 4000.
	if hist[0] < 5 || hist[0] > 40 {
		t.Errorf("%d seeds of %d drew no hotspots, want about %v", hist[0], seeds, seeds*math.Exp(-mean))
	}
	if len(hist) < 8 {
		t.Errorf("only %d distinct counts: %v", len(hist), hist)
	}

	// A low rate makes zero common; a zero rate makes it certain.
	c.Volcanic.HotspotsPerMkm2 = 0.25
	zeros := 0
	for s := range uint64(200) {
		c.Seed = config.Seed(s + 1)
		if len(Hotspots(c)) == 0 {
			zeros++
		}
	}
	if p := math.Exp(-meanCount(c)); math.Abs(float64(zeros)/200-p) > 0.12 {
		t.Errorf("rate 0.25: %d of 200 seeds drew none, want about %.0f", zeros, 200*p)
	}
	c.Volcanic.HotspotsPerMkm2 = 0
	if hs := Hotspots(c); hs != nil {
		t.Errorf("rate 0 drew %d hotspots", len(hs))
	}
}

// TestPoissonLargeMean checks the chunked draw: a mean of several chunks
// keeps the Poisson mean and variance.
func TestPoissonLargeMean(t *testing.T) {
	const draws, mean = 400, 1000.0
	var sum, sum2 float64
	for s := range uint64(draws) {
		n := float64(poisson(seed.Rand(s, StageVolcanic, "test"), mean))
		sum += n
		sum2 += n * n
	}
	m := sum / draws
	variance := sum2/draws - m*m
	t.Logf("mean %v: drew mean %.2f, variance %.1f", mean, m, variance)
	// Standard error √(1000/400) ≈ 1.6.
	if math.Abs(m-mean) > 6 || math.Abs(variance/mean-1) > 0.25 {
		t.Errorf("mean %v, variance %v; want both about %v", m, variance, mean)
	}
	if n := poisson(seed.Rand(1, StageVolcanic, "test"), 0); n != 0 {
		t.Errorf("mean 0 drew %d", n)
	}
}

// TestHotspotPlacement checks every peak lies in the playable band outside
// the rim and the falloff, wrapped into [0, W), with parameters in their
// ranges, and that the peaks spread uniformly over the band.
func TestHotspotPlacement(t *testing.T) {
	for _, aspect := range []string{"cinematic", "square"} {
		c := resolved(t, 1, "continents", aspect)
		c.Volcanic.HotspotsPerMkm2 = 100
		v := c.Volcanic
		w, h := c.World.WidthKm, c.World.HeightKm
		lo := c.Rim.Km + c.Rim.FalloffKm
		cyl, err := topo.New(w, h, c.Rim.Km, c.Rim.FalloffKm)
		if err != nil {
			t.Fatal(err)
		}
		const bins = 4
		var grid [bins][bins]int
		total := 0
		for s := range uint64(40) {
			c.Seed = config.Seed(s + 1)
			for _, p := range Hotspots(c) {
				if !(p.X >= 0 && p.X < w) || cyl.InRim(p.Y) || cyl.InFalloff(p.Y) || p.Y < lo || p.Y > h-lo {
					t.Fatalf("%s: hotspot at (%v, %v) outside the band", aspect, p.X, p.Y)
				}
				if p.PeakM < v.ConePeakMinM || p.PeakM > v.ConePeakMaxM ||
					p.ConeRadiusKm < v.ConeRadiusMinKm || p.ConeRadiusKm > v.ConeRadiusMaxKm ||
					p.SwellM != v.SwellM || p.SwellRadiusKm != v.SwellRadiusKm {
					t.Fatalf("%s: hotspot %+v outside the configured ranges", aspect, p)
				}
				bx := int(p.X / w * bins)
				by := int((p.Y - lo) / (h - 2*lo) * bins)
				grid[min(by, bins-1)][min(bx, bins-1)]++
				total++
			}
		}
		want := float64(total) / (bins * bins)
		for by := range bins {
			for bx := range bins {
				if n := float64(grid[by][bx]); math.Abs(n-want) > 0.15*want {
					t.Errorf("%s: bin (%d, %d) holds %v peaks, want about %.0f of %d", aspect, bx, by, n, want, total)
				}
			}
		}
	}
}

// TestHotspotShape checks the cone and swell: one hotspot in the playable
// band raises the sample nearest its peak by about the cone's height plus
// the swell's, raises the ground at moderate distance, outside the cone, by
// about the swell's height, and adds nothing beyond the swell. With the
// datum shift held at 0, the rise is purely additive away from the falloff.
func TestHotspotShape(t *testing.T) {
	c := resolved(t, 5, "continents", "cinematic")
	c.Volcanic.HotspotsPerMkm2 = 0
	c.Elevation.DatumMaxShift = 0
	without := build(t, c)
	hs := Hotspot{X: c.World.WidthKm / 3, Y: c.World.HeightKm / 2, PeakM: 2500, ConeRadiusKm: 20, SwellM: 300, SwellRadiusKm: 100}
	w := withHotspots(t, c, []Hotspot{hs})
	f, cyl := w.f, w.f.Cylinder()
	var nearest float64 = math.Inf(1)
	var peakRise float64
	f.Each(func(i, j int, v float64) {
		d := cyl.Distance(hs.Point(), f.Point(i, j))
		rise := v - without.f.At(i, j)
		if want := hs.Rise(d); math.Abs(rise-want) > 1e-6 {
			t.Fatalf("(%d, %d) at %v km: rose %v, want %v", i, j, d, rise, want)
		}
		switch {
		case d < nearest:
			nearest, peakRise = d, rise
		case d >= hs.SwellRadiusKm && rise != 0:
			t.Fatalf("(%d, %d) at %v km, beyond the swell, rose %v", i, j, d, rise)
		case d >= 25 && d <= 35 && math.Abs(rise-hs.SwellM) > 0.06*hs.SwellM:
			t.Errorf("(%d, %d) at %v km rose %v, want about the swell's %v", i, j, d, rise, hs.SwellM)
		}
	})
	if want := hs.PeakM + hs.SwellM; math.Abs(peakRise-want) > 0.03*want {
		t.Errorf("sample nearest the peak (%v km) rose %v, want about %v", nearest, peakRise, want)
	}
	if hs.Rise(0) != hs.PeakM+hs.SwellM || hs.Rise(hs.ConeRadiusKm) >= hs.SwellM || hs.Rise(hs.SwellRadiusKm) != 0 {
		t.Errorf("Rise(0) %v, Rise(cone) %v, Rise(swell) %v", hs.Rise(0), hs.Rise(hs.ConeRadiusKm), hs.Rise(hs.SwellRadiusKm))
	}
}

// TestHotspotsOff checks that a zero rate leaves the field bit for bit as
// it is with the hotspots removed, and that the default rate does change
// it.
func TestHotspotsOff(t *testing.T) {
	c := resolved(t, 9, "islands", "square")
	with := build(t, c)
	if len(with.elev.Hotspots()) == 0 {
		t.Fatal("seed 9 drew no hotspots; pick another seed")
	}
	stripped := withHotspots(t, c, nil)
	c.Volcanic.HotspotsPerMkm2 = 0
	off := build(t, c)
	if len(off.elev.Hotspots()) != 0 {
		t.Fatal("rate 0 drew hotspots")
	}
	if !slices.Equal(bits(off.f.Values()), bits(stripped.f.Values())) || off.stats != stripped.stats {
		t.Error("rate 0 differs from the same world with its hotspots removed")
	}
	if slices.Equal(with.f.Values(), off.f.Values()) {
		t.Error("the hotspots changed nothing")
	}
}

// TestHotspotDatum checks the datum counts the land the hotspots raise, so
// even a world crowded with them keeps the land fraction above 0 m, and
// reports it in Stats.Share.
func TestHotspotDatum(t *testing.T) {
	for _, seed := range []uint64{1, 2} {
		c := resolved(t, seed, "archipelago", "cinematic")
		c.Volcanic.HotspotsPerMkm2 = 30
		w := build(t, c)
		got := LandShare(w.f, 0)
		if w.stats.Clamped || math.Abs(got-c.World.LandFraction) > 0.003 || math.Abs(got-w.stats.Share) > 0.003 {
			t.Errorf("seed %d, %d hotspots: %.4f above 0 m, stats %+v; want %v", seed, len(w.elev.Hotspots()), got, w.stats, c.World.LandFraction)
		}
		off := withHotspots(t, c, nil)
		if !(w.stats.Shift < off.stats.Shift) {
			t.Errorf("seed %d: shift %v with hotspots, %v without; want the hotspots' land to lower it", seed, w.stats.Shift, off.stats.Shift)
		}
	}
}

// TestHotspotSeam checks that a hotspot straddling the seam raises both
// sides of it alike: the rise at equal distances east and west of the peak
// is the same, and the seam step is like any other column step.
func TestHotspotSeam(t *testing.T) {
	c := resolved(t, 3, "pangaea", "cinematic")
	c.Volcanic.HotspotsPerMkm2 = 0
	c.Elevation.DatumMaxShift = 0 // hold the datum, so rises compare
	without := build(t, c)
	f0 := without.f
	px := f0.PitchX()
	// The peak sits on the seam, half a pitch west of sample 0's center,
	// so samples nx−1−k and k are equally far from it.
	hs := Hotspot{X: 0, Y: c.World.HeightKm / 2, PeakM: 3000, ConeRadiusKm: 30, SwellM: 300, SwellRadiusKm: 100}
	f := withHotspots(t, c, []Hotspot{hs}).f
	nx := f.NX()
	j := int(hs.Y / f.PitchY())
	for k := range int(hs.SwellRadiusKm/px) + 2 {
		east := f.At(k, j) - f0.At(k, j)
		west := f.At(nx-1-k, j) - f0.At(nx-1-k, j)
		if math.Abs(east-west) > 1e-6 {
			t.Fatalf("column %d east of the seam rose %v, west %v", k, east, west)
		}
		if k == 0 && east < 0.9*(hs.PeakM+hs.SwellM) {
			t.Errorf("next to the peak rose %v, want about %v", east, hs.PeakM+hs.SwellM)
		}
	}
	// Moved east by a whole number of samples, about a quarter turn, the
	// same hotspot raises the same shape.
	shift := nx / 4
	moved := hs
	moved.X = float64(shift) * px
	g := withHotspots(t, c, []Hotspot{moved}).f
	for k := -60; k < 60; k++ {
		a := f.At(fmath.FloorMod(k, nx), j) - f0.At(fmath.FloorMod(k, nx), j)
		b := g.At(fmath.FloorMod(k+shift, nx), j) - f0.At(fmath.FloorMod(k+shift, nx), j)
		if math.Abs(a-b) > 1e-6 {
			t.Fatalf("offset %d: seam hotspot rose %v, moved one %v", k, a, b)
		}
	}
}

// TestHotspotFalloff checks a hotspot at the falloff band's edge cannot lift
// the band or the rim above the falloff's limits.
func TestHotspotFalloff(t *testing.T) {
	c := resolved(t, 2, "pangaea", "square")
	edge := c.Rim.Km + c.Rim.FalloffKm
	hs := []Hotspot{
		{X: 100, Y: edge, PeakM: 20000, ConeRadiusKm: 200, SwellM: 20000, SwellRadiusKm: 500},
		{X: 900, Y: c.World.HeightKm - edge, PeakM: 20000, ConeRadiusKm: 200, SwellM: 20000, SwellRadiusKm: 500},
	}
	w := withHotspots(t, c, hs)
	fc := c.Elevation.Falloff
	if band := BandMax(w.f); band > fc.CeilingM {
		t.Errorf("falloff band reaches %v m, ceiling %v", band, fc.CeilingM)
	}
}

// withHotspots builds the world of c with the given hotspots in place of the
// drawn ones.
func withHotspots(t *testing.T, c config.Config, hs []Hotspot) world {
	t.Helper()
	w := build(t, c)
	w.elev.hotspots = hs
	f := w.bias.Clone()
	w.stats = w.elev.Fill(f)
	w.f = f
	return w
}

// bits returns v's float64 bit patterns, so comparisons tell −0 from +0.
func bits(v []float64) []uint64 {
	b := make([]uint64, len(v))
	for i, x := range v {
		b[i] = math.Float64bits(x)
	}
	return b
}
