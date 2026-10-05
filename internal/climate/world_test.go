// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package climate_test

import (
	"bytes"
	"fmt"
	"image/color"
	"math"
	"testing"

	"github.com/mdhender/mpg/internal/cells"
	"github.com/mdhender/mpg/internal/climate"
	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/mesh"
	"github.com/mdhender/mpg/internal/pipeline"
)

// world runs the pipeline through the climate stage and returns its
// context. land < 1000 makes a small world with a narrow falloff.
func world(t *testing.T, seed uint64, aspect, preset string, land int) *pipeline.Context {
	t.Helper()
	c := config.Default()
	c.Seed = config.Seed(seed)
	c.World.Aspect = aspect
	c.Layout.Preset = preset
	c.World.LandCells = land
	if land < 1000 {
		c.Rim.FalloffCells = 4
	}
	ctx, err := pipeline.NewContext(c, t.TempDir(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	stages := pipeline.Stages()
	last, err := pipeline.Lookup(stages, "climate")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.Run(ctx, stages, last); err != nil {
		t.Fatal(err)
	}
	if ctx.Products.Climate == nil {
		t.Fatal("no climate")
	}
	return ctx
}

// TestWorlds checks the climate of real worlds:
//
//   - the mask: every raster sample is ocean exactly when its cell
//     (cells.Stats.Owner) is a rim cell or a cell the sea level stage's
//     ocean flood reached; dry basin floors are land;
//   - every temperature is finite and is the model at the cell's latitude
//     and its altitude above sea level (0 for water, the rim and dry basin
//     floors);
//   - the rim is coldest at sea level: every rim cell is at least as cold
//     as the sea-level temperature of every playable cell (the latitude
//     curve at the cell's latitude), and strictly colder than every
//     playable cell the lapse rate does not cool (water, dry basin floors,
//     land at sea level). Only altitude can take a playable cell below the
//     rim, and it does: seed 5 portrait pangaea has high land near the
//     south pole down to about −25 °C, colder than any rim cell (about
//     −21.8 °C at the coldest). The test checks that case is still there,
//     so the claim is not silently narrowed.
func TestWorlds(t *testing.T) {
	for _, tc := range []struct {
		seed           uint64
		aspect, preset string
		land           int
	}{
		{42, "cinematic", "continents", 10_000},
		{7, "square", "continents", 10_000},
		{3, "cinematic", "archipelago", 10_000},
		{7, "cinematic", "pangaea", 10_000},
		{5, "portrait", "continents", 600},
		{5, "portrait", "pangaea", 10_000}, // high land colder than the rim
	} {
		ctx := world(t, tc.seed, tc.aspect, tc.preset, tc.land)
		m, s, sl, r := ctx.Products.Mesh, ctx.Products.Cells, ctx.Products.SeaLevel, ctx.Products.Climate
		model, err := climate.NewModel(ctx.Config.Climate)
		if err != nil {
			t.Fatal(err)
		}
		name := fmt.Sprintf("%d/%s/%s", tc.seed, tc.aspect, tc.preset)

		if len(r.Mask) != ctx.Products.Elevation.Len() || r.Level != sl.Level {
			t.Fatalf("%s: mask %d samples, level %v", name, len(r.Mask), r.Level)
		}
		for k, c := range s.Owner {
			want := m.Cells[c].Rim || sl.Ocean[c]
			if r.Mask[k] != want || r.Ocean[c] != want {
				t.Fatalf("%s: sample %d (cell %d): mask %v, want %v", name, k, c, r.Mask[k], want)
			}
		}
		basins := 0
		for i := range m.Cells {
			if sl.Basin[i] {
				basins++
				if r.Ocean[i] {
					t.Errorf("%s: dry basin cell %d is ocean", name, i)
				}
			}
		}

		rimMax, rimMin := math.Inf(-1), math.Inf(1)
		seaMin, waterMin, coldest := math.Inf(1), math.Inf(1), -1
		for i, c := range m.Cells {
			v := r.Temperature[i]
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Fatalf("%s: cell %d temperature %v", name, i, v)
			}
			h := 0.0
			if !c.Rim && sl.Land[i] && s.Altitude[i] > sl.Level {
				h = s.Altitude[i] - sl.Level
			}
			if r.HeightM[i] != h || v != model.At(s.Latitude[i], h) {
				t.Fatalf("%s: cell %d height %v temperature %v, want %v and %v", name, i, r.HeightM[i], v, h, model.At(s.Latitude[i], h))
			}
			if coldest < 0 || v < r.Temperature[coldest] {
				coldest = i
			}
			if c.Rim {
				rimMax, rimMin = max(rimMax, v), min(rimMin, v)
				continue
			}
			seaMin = min(seaMin, model.SeaLevel(s.Latitude[i]))
			if r.HeightM[i] == 0 {
				waterMin = min(waterMin, v)
			}
		}
		if !(rimMax <= seaMin) || !(rimMax < waterMin) {
			t.Errorf("%s: warmest rim cell %.3f °C, coldest playable sea-level temperature %.3f °C, coldest playable water %.3f °C", name, rimMax, seaMin, waterMin)
		}
		if wantLandColdest := tc.seed == 5 && tc.preset == "pangaea"; m.Cells[coldest].Rim == wantLandColdest {
			t.Errorf("%s: coldest cell %d (%.2f °C, %.0f m above sea, rim %v); rim %.2f to %.2f °C", name, coldest, r.Temperature[coldest], r.HeightM[coldest], m.Cells[coldest].Rim, rimMin, rimMax)
		} else if !m.Cells[coldest].Rim {
			t.Logf("%s: coldest cell %d is land, %.2f °C at %.0f m above sea, latitude %.3f", name, coldest, r.Temperature[coldest], r.HeightM[coldest], s.Latitude[coldest])
		}
		t.Logf("%s: rim %.2f to %.2f °C; playable sea level ≥ %.2f °C; coldest playable %.2f °C; %d dry basin cells", name, rimMin, rimMax, seaMin, coldestPlayable(m, r), basins)
	}
}

func coldestPlayable(m *mesh.Mesh, r *climate.Result) float64 {
	v := math.Inf(1)
	for i, c := range m.Cells {
		if !c.Rim {
			v = min(v, r.Temperature[i])
		}
	}
	return v
}

// TestGradientAndCooling checks the done-when items on the default world:
// at sea level the cells cool from the equator to the poles band by band,
// and on land altitude cools them: in every latitude band with high land,
// the land 1000 m or more above the sea is colder on average than the
// band's water, by at least the lapse rate's 6.5 °C less the band's
// latitude spread.
func TestGradientAndCooling(t *testing.T) {
	ctx := world(t, 42, "cinematic", "continents", 10_000)
	m, s, r := ctx.Products.Mesh, ctx.Products.Cells, ctx.Products.Climate
	const bands = 10 // by |latitude|, 0.1 wide
	var waterSum, highSum [bands]float64
	var waterN, highN [bands]int
	var waterLo, waterHi [bands]float64
	for b := range bands {
		waterLo[b], waterHi[b] = math.Inf(1), math.Inf(-1)
	}
	for i := range m.Cells {
		b := min(int(math.Abs(s.Latitude[i])*bands), bands-1)
		v := r.Temperature[i]
		switch h := r.HeightM[i]; {
		case h == 0:
			waterSum[b] += v
			waterN[b]++
			waterLo[b], waterHi[b] = min(waterLo[b], v), max(waterHi[b], v)
		case h >= 1000:
			highSum[b] += v
			highN[b]++
		}
	}
	prev := math.Inf(1)
	highBands := 0
	for b := range bands {
		if waterN[b] == 0 {
			t.Fatalf("band %d has no sea-level cells", b)
		}
		mean := waterSum[b] / float64(waterN[b])
		t.Logf("|lat| %.1f–%.1f (%2.0f–%2.0f°): sea level %6.2f to %6.2f °C, mean %6.2f (%d cells); ≥1000 m land mean %6.2f (%d cells)",
			float64(b)/bands, float64(b+1)/bands, 9*float64(b), 9*float64(b+1), waterLo[b], waterHi[b], mean, waterN[b], highSum[b]/max(1, float64(highN[b])), highN[b])
		if b > 0 && !(mean < prev) {
			t.Errorf("band %d sea-level mean %.2f °C is not colder than band %d's %.2f °C", b, mean, b-1, prev)
		}
		prev = mean
		if highN[b] > 0 {
			highBands++
			if high := highSum[b] / float64(highN[b]); !(high <= mean-6.5+(waterHi[b]-waterLo[b])) {
				t.Errorf("band %d: land ≥ 1000 m averages %.2f °C, sea level %.2f °C (spread %.2f)", b, high, mean, waterHi[b]-waterLo[b])
			}
		}
	}
	if highBands == 0 {
		t.Error("no land 1000 m above the sea")
	}
}

// TestDeterminism checks that the climate is the same on a second
// computation and that its renders are the same on a second draw, have
// the mesh render's (temperature) and the raster's (mask) size, and show
// a rim cell in the ramp's color for its temperature.
func TestDeterminism(t *testing.T) {
	ctx := world(t, 7, "cinematic", "continents", 600)
	m, s, sl, f := ctx.Products.Mesh, ctx.Products.Cells, ctx.Products.SeaLevel, ctx.Products.Elevation
	model, err := climate.NewModel(ctx.Config.Climate)
	if err != nil {
		t.Fatal(err)
	}
	r, err := climate.Compute(m, s, &sl.Flood, model)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := ctx.Products.Climate.AppendBinary(nil)
	b, _ := r.AppendBinary(nil)
	if !bytes.Equal(a, b) {
		t.Error("climate differs between computations")
	}

	img, again := climate.TemperatureRender(f, m, r), climate.TemperatureRender(f, m, r)
	sc := mesh.RenderScale(f, m)
	if !bytes.Equal(img.Pix, again.Pix) || img.Rect.Dx() != f.NX()*sc || img.Rect.Dy() != f.NY()*sc {
		t.Errorf("temperature render %v differs between draws or has the wrong size", img.Rect)
	}
	// A rim cell's site, far from its edges, shows its temperature's color.
	for i, c := range m.Cells {
		if !c.Rim || c.Site.Y < 2 {
			continue
		}
		x, y := int(c.Site.X*float64(sc)/f.PitchX()), int(c.Site.Y*float64(sc)/f.PitchY())
		if got, want := img.RGBAAt(x, y), climate.TemperatureRamp.At(r.Temperature[i]); got != want {
			t.Errorf("rim cell %d site pixel %v, want %v", i, got, want)
		}
		break
	}
	mask, mask2 := climate.MaskRender(f, r), climate.MaskRender(f, r)
	if !bytes.Equal(mask.Pix, mask2.Pix) || mask.Rect.Dx() != f.NX() || mask.Rect.Dy() != f.NY() {
		t.Errorf("mask render %v differs between draws or has the wrong size", mask.Rect)
	}
	if mask.RGBAAt(0, 0) == mask.RGBAAt(f.NX()/2, f.NY()/2) && !r.Mask[f.NY()/2*f.NX()+f.NX()/2] {
		t.Error("mask render shows land as ocean")
	}

	// Colder is bluer, warmer redder.
	cold, warm := climate.TemperatureRamp.At(-20), climate.TemperatureRamp.At(25)
	if !(cold.B > cold.R && warm.R > warm.B) || cold == (color.RGBA{}) {
		t.Errorf("ramp cold %v warm %v", cold, warm)
	}
}

// TestComputeErrors checks Compute's input checks.
func TestComputeErrors(t *testing.T) {
	ctx := world(t, 7, "cinematic", "continents", 600)
	m, s, sl := ctx.Products.Mesh, ctx.Products.Cells, ctx.Products.SeaLevel
	model, err := climate.NewModel(ctx.Config.Climate)
	if err != nil {
		t.Fatal(err)
	}
	short := &cells.Flood{Ocean: sl.Ocean[1:], Land: sl.Land[1:]}
	if _, err := climate.Compute(m, s, short, model); err == nil {
		t.Error("short flood accepted")
	}
	if _, err := climate.Compute(m, s, &sl.Flood, climate.Model{}); err == nil {
		t.Error("zero model accepted")
	}
}
