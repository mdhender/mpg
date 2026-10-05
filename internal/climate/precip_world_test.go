// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package climate_test

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/mdhender/mpg/internal/climate"
	"github.com/mdhender/mpg/internal/mesh"
	"github.com/mdhender/mpg/internal/pipeline"
)

// exposure is the precipitation of a group of land cells.
type exposure struct {
	n        int
	sumP     float64 // mm
	sumRatio float64 // precipitation / the windward table at the cell's latitude
}

func (e *exposure) add(p, ratio float64) { e.n++; e.sumP += p; e.sumRatio += ratio }
func (e exposure) meanP() float64        { return e.sumP / float64(e.n) }
func (e exposure) ratio() float64        { return e.sumRatio / float64(e.n) }

// exposures sorts a world's land cells by how the prevailing wind reaches
// them, tracing upwind from each site along the strongest wind's bearing
// (before jitter) in 5 km steps for up to 1,000 km: the fetch is how far
// upwind the sea is, and the barrier how far the highest ground on the way
// rises above the cell. Windward coasts have a fetch of at most 20 km; lee
// cells a barrier of at least 500 m; interior cells a fetch of at least
// 400 km and no such barrier. Each cell's precipitation is also divided by
// the windward-coast table at its latitude, so the groups compare across
// latitude bands.
func exposures(t *testing.T, ctx *pipeline.Context) (windward, lee, interior exposure) {
	t.Helper()
	m, s, r, f := ctx.Products.Mesh, ctx.Products.Cells, ctx.Products.Climate, ctx.Products.Elevation
	model, err := climate.NewModel(ctx.Config.Climate)
	if err != nil {
		t.Fatal(err)
	}
	cyl := f.Cylinder()
	cellAt := func(x, y float64) int {
		if y < 0 || y >= cyl.H() {
			return -1
		}
		x = math.Mod(math.Mod(x, cyl.W())+cyl.W(), cyl.W())
		i, j := min(int(x/f.PitchX()), f.NX()-1), min(int(y/f.PitchY()), f.NY()-1)
		return s.Owner[j*f.NX()+i]
	}
	for i, c := range m.Cells {
		if r.Ocean[i] {
			continue
		}
		lat := 90 * s.Latitude[i]
		from, weight := model.WindsAt(lat)
		best := 0
		for k := range weight {
			if weight[k] > weight[best] {
				best = k
			}
		}
		sin, cos := math.Sincos(from[best] * math.Pi / 180)
		fetch, barrier := 1000.0, 0.0
		for k := 1; k <= 200; k++ {
			cell := cellAt(c.Site.X+float64(k)*5*sin, c.Site.Y-float64(k)*5*cos)
			if cell < 0 || r.Ocean[cell] {
				fetch = float64(k) * 5
				break
			}
			barrier = max(barrier, r.HeightM[cell]-r.HeightM[i])
		}
		p := r.Precipitation[i]
		ratio := p / model.WindwardAt(math.Abs(lat))
		isLee := barrier >= 500
		if fetch <= 20 {
			windward.add(p, ratio)
		}
		if isLee {
			lee.add(p, ratio)
		} else if fetch >= 400 {
			interior.add(p, ratio)
		}
	}
	return windward, lee, interior
}

// TestWetWindwardDryLee checks S26's "done when" on real worlds: windward
// coasts are wet and lee sides and interiors dry. Measured relative to the
// windward-coast table at each cell's latitude, windward coasts get at
// least 0.9 of it (measured 0.93 to 0.98), lee sides and interiors (where
// there are at least 30 interior cells) at most 0.55 (measured 0.33 to
// 0.49), and windward coasts at least 1.8 times the lee sides' share and
// more rain in mm.
func TestWetWindwardDryLee(t *testing.T) {
	for _, tc := range []struct {
		seed           uint64
		aspect, preset string
	}{
		{42, "cinematic", "continents"},
		{7, "square", "continents"},
		{5, "portrait", "continents"},
		{7, "cinematic", "pangaea"},
		{3, "cinematic", "archipelago"},
	} {
		name := fmt.Sprintf("%d/%s/%s", tc.seed, tc.aspect, tc.preset)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := world(t, tc.seed, tc.aspect, tc.preset, 10_000)
			ww, lee, in := exposures(t, ctx)
			t.Logf("windward coast %d cells %.0f mm (%.2f); lee %d cells %.0f mm (%.2f); interior %d cells %.0f mm (%.2f)",
				ww.n, ww.meanP(), ww.ratio(), lee.n, lee.meanP(), lee.ratio(), in.n, in.meanP(), in.ratio())
			if ww.n < 100 || lee.n < 100 {
				t.Fatalf("too few windward (%d) or lee (%d) cells to compare", ww.n, lee.n)
			}
			if !(ww.ratio() >= 0.9) {
				t.Errorf("windward coasts get %.2f of the windward table, want at least 0.9", ww.ratio())
			}
			if !(lee.ratio() <= 0.55) {
				t.Errorf("lee sides get %.2f of the windward table, want at most 0.55", lee.ratio())
			}
			if in.n >= 30 && !(in.ratio() <= 0.55) {
				t.Errorf("interiors get %.2f of the windward table, want at most 0.55", in.ratio())
			}
			if !(ww.ratio() >= 1.8*lee.ratio()) || !(ww.meanP() > lee.meanP()) {
				t.Errorf("windward %.2f (%.0f mm) is not clearly wetter than lee %.2f (%.0f mm)", ww.ratio(), ww.meanP(), lee.ratio(), lee.meanP())
			}
		})
	}
}

// TestPrecipitationFields checks the new fields on real worlds: every
// value finite and in range; each cell's precipitation the mean of its
// samples' in storage order; open water saturated (moisture 1, lift 0);
// land moisture in (0, 1]; PET, runoff and aridity those of the cell's
// temperature and precipitation; and runoff within [0, P].
func TestPrecipitationFields(t *testing.T) {
	for _, tc := range []struct {
		seed           uint64
		aspect, preset string
		land           int
	}{
		{42, "cinematic", "continents", 10_000},
		{5, "portrait", "pangaea", 10_000},
		{7, "square", "islands", 600},
	} {
		ctx := world(t, tc.seed, tc.aspect, tc.preset, tc.land)
		m, s, r, f := ctx.Products.Mesh, ctx.Products.Cells, ctx.Products.Climate, ctx.Products.Elevation
		model, err := climate.NewModel(ctx.Config.Climate)
		if err != nil {
			t.Fatal(err)
		}
		name := fmt.Sprintf("%d/%s/%s", tc.seed, tc.aspect, tc.preset)
		if len(r.RasterPrecipitation) != f.Len() {
			t.Fatalf("%s: %d raster values for %d samples", name, len(r.RasterPrecipitation), f.Len())
		}
		sum := make([]float64, len(m.Cells))
		for k, v := range r.RasterPrecipitation {
			if !(v > 0) || math.IsInf(v, 0) {
				t.Fatalf("%s: sample %d precipitation %v", name, k, v)
			}
			sum[s.Owner[k]] += v
		}
		for i := range m.Cells {
			p := r.Precipitation[i]
			if s.Samples[i] > 0 && p != sum[i]/float64(s.Samples[i]) {
				t.Fatalf("%s: cell %d precipitation %v is not its samples' mean %v", name, i, p, sum[i]/float64(s.Samples[i]))
			}
			if !(p > 0 && p < 20_000) {
				t.Fatalf("%s: cell %d precipitation %v", name, i, p)
			}
			if r.Ocean[i] {
				if r.Moisture[i] != 1 || r.LiftM[i] != 0 {
					t.Fatalf("%s: water cell %d moisture %v lift %v", name, i, r.Moisture[i], r.LiftM[i])
				}
			} else if !(r.Moisture[i] > 0 && r.Moisture[i] <= 1) || !(r.LiftM[i] >= 0 && r.LiftM[i] <= r.HeightM[i]*(1+1e-12)) {
				t.Fatalf("%s: land cell %d moisture %v lift %v (height %v)", name, i, r.Moisture[i], r.LiftM[i], r.HeightM[i])
			}
			if r.PET[i] != model.PET(r.Temperature[i]) || r.Runoff[i] != climate.Runoff(p, r.PET[i]) || r.Aridity[i] != climate.AridityIndex(p, r.PET[i]) {
				t.Fatalf("%s: cell %d PET %v runoff %v aridity %v do not follow from T %v and P %v", name, i, r.PET[i], r.Runoff[i], r.Aridity[i], r.Temperature[i], p)
			}
			if !(r.Runoff[i] >= 0 && r.Runoff[i] <= p) || !(r.Aridity[i] >= 0 && r.Aridity[i] <= climate.AridityCap) {
				t.Fatalf("%s: cell %d runoff %v aridity %v", name, i, r.Runoff[i], r.Aridity[i])
			}
		}
	}
}

// TestSeam checks that the climate does not see the east–west seam: the
// samples are periodic (a sample of the first column evaluated a world
// width further east, and one of the last column a world width further
// west, give what Compute stored, to within 1e-12 relative: x ± W is not
// exactly representable, so the noise and the trace points can move in
// the last place), and no precipitation step across
// the seam is larger than the largest between any other two neighboring
// columns.
func TestSeam(t *testing.T) {
	ctx := world(t, 42, "cinematic", "continents", 10_000)
	s, r, f := ctx.Products.Cells, ctx.Products.Climate, ctx.Products.Elevation
	model, err := climate.NewModel(ctx.Config.Climate)
	if err != nil {
		t.Fatal(err)
	}
	nx, w := f.NX(), f.Cylinder().W()
	land := 0
	for j := range f.NY() {
		for _, c := range []struct {
			i     int
			shift float64
		}{{0, w}, {nx - 1, -w}} {
			k := j*nx + c.i
			if !r.Mask[k] {
				land++
			}
			p, _, _ := climate.SampleAt(f, s, r, model, uint64(ctx.Config.Seed), f.X(c.i)+c.shift, f.Y(j), s.Owner[k])
			if math.Abs(p-r.RasterPrecipitation[k]) > 1e-12*r.RasterPrecipitation[k] {
				t.Fatalf("row %d column %d: %v shifted by %v km, %v in place", j, c.i, p, c.shift, r.RasterPrecipitation[k])
			}
		}
	}
	seam, inner := 0.0, 0.0
	for j := range f.NY() {
		row := r.RasterPrecipitation[j*nx : (j+1)*nx]
		seam = max(seam, math.Abs(row[0]-row[nx-1]))
		for i := range nx - 1 {
			inner = max(inner, math.Abs(row[i+1]-row[i]))
		}
	}
	t.Logf("%d land samples on the seam columns; largest step across the seam %.1f mm, elsewhere %.1f mm", land, seam, inner)
	if land == 0 {
		t.Error("no land on the seam columns; the test checks nothing about traces")
	}
	if !(seam <= inner) {
		t.Errorf("largest step across the seam %.1f mm exceeds the largest elsewhere %.1f mm", seam, inner)
	}
}

// TestWorkers checks that the climate does not depend on how many
// goroutines compute it: 1, 3 and 16 workers give the same encoding as the
// stage's.
func TestWorkers(t *testing.T) {
	ctx := world(t, 7, "cinematic", "continents", 600)
	m, s, sl, f := ctx.Products.Mesh, ctx.Products.Cells, ctx.Products.SeaLevel, ctx.Products.Elevation
	model, err := climate.NewModel(ctx.Config.Climate)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := ctx.Products.Climate.AppendBinary(nil)
	for _, workers := range []int{1, 3, 16} {
		r, err := climate.ComputeWorkers(f, m, s, &sl.Flood, model, uint64(ctx.Config.Seed), workers)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := r.AppendBinary(nil); !bytes.Equal(got, want) {
			t.Errorf("%d workers give a different climate", workers)
		}
	}
	other, err := climate.ComputeWorkers(f, m, s, &sl.Flood, model, uint64(ctx.Config.Seed)+1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := other.AppendBinary(nil); bytes.Equal(got, want) {
		t.Error("another seed gives the same climate: the noise is not keyed by the seed")
	}
}

// TestPrecipRenders checks the new renders: the same picture on a second
// draw, the mesh render's size, a rim cell's site in its precipitation's
// and PET's colors, and water flat in the land-only renders.
func TestPrecipRenders(t *testing.T) {
	ctx := world(t, 7, "cinematic", "continents", 600)
	m, r, f := ctx.Products.Mesh, ctx.Products.Climate, ctx.Products.Elevation
	sc := mesh.RenderScale(f, m)
	rim := -1
	for i, c := range m.Cells {
		if c.Rim && c.Site.Y >= 2 {
			rim = i
			break
		}
	}
	site := m.Cells[rim].Site
	x, y := int(site.X*float64(sc)/f.PitchX()), int(site.Y*float64(sc)/f.PitchY())
	for _, tc := range []struct {
		name string
		draw func() *image.RGBA
		rim  color.RGBA
	}{
		{"precip", func() *image.RGBA { return climate.PrecipRender(f, m, r) }, climate.PrecipRamp.At(r.Precipitation[rim])},
		{"moisture", func() *image.RGBA { return climate.MoistureRender(f, m, r) }, color.RGBA{0x2c, 0x3e, 0x5c, 0xff}},
		{"pet", func() *image.RGBA { return climate.PETRender(f, m, r) }, climate.PETRamp.At(r.PET[rim])},
		{"runoff", func() *image.RGBA { return climate.RunoffRender(f, m, r) }, color.RGBA{0x2c, 0x3e, 0x5c, 0xff}},
		{"aridity", func() *image.RGBA { return climate.AridityRender(f, m, r) }, color.RGBA{0x2c, 0x3e, 0x5c, 0xff}},
	} {
		img, again := tc.draw(), tc.draw()
		if !bytes.Equal(img.Pix, again.Pix) || img.Rect.Dx() != f.NX()*sc || img.Rect.Dy() != f.NY()*sc {
			t.Errorf("%s render %v differs between draws or has the wrong size", tc.name, img.Rect)
		}
		if got := img.RGBAAt(x, y); got != tc.rim {
			t.Errorf("%s render: rim cell %d site pixel %v, want %v", tc.name, rim, got, tc.rim)
		}
	}
}
