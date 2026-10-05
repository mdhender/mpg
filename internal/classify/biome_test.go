// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package classify

import (
	"bytes"
	"image/color"
	"slices"
	"testing"

	"github.com/mdhender/mpg/internal/basin"
	"github.com/mdhender/mpg/internal/edges"
	"github.com/mdhender/mpg/internal/mesh"
	wj "github.com/mdhender/mpg/world"
)

// TestBiomeNames checks the biome and surface names against world.json's
// codebooks, in the same order.
func TestBiomeNames(t *testing.T) {
	var bs []wj.Biome
	for _, b := range Biomes {
		bs = append(bs, wj.Biome(b.String()))
	}
	if !slices.Equal(bs, wj.Biomes) {
		t.Errorf("biomes %q, world codebook %q", bs, wj.Biomes)
	}
	var ss []wj.Surface
	for _, s := range Surfaces {
		ss = append(ss, wj.Surface(s.String()))
	}
	if !slices.Equal(ss, wj.Surfaces) {
		t.Errorf("surfaces %q, world codebook %q", ss, wj.Surfaces)
	}
	if BiomeNone.String() != "" || SurfaceNone.String() != "" || Biome(99).String() != "Biome(99)" || Surface(99).String() != "Surface(99)" {
		t.Error("unset or out-of-range names")
	}
	for _, b := range Biomes {
		if b.Abbrev() == "" {
			t.Errorf("%s has no abbreviation", b)
		}
	}
	for _, s := range Surfaces {
		if s.IsIce() != (s == Glacier || s == IceField) || s.IsWetland() != (s >= Marshes) {
			t.Errorf("%s: ice %v, wetland %v", s, s.IsIce(), s.IsWetland())
		}
	}
	if !CloudForest.IsForest() || !BorealForest.IsForest() || Savanna.IsForest() || Clear.IsForest() {
		t.Error("IsForest")
	}
}

// TestSeasonAndSnowfall checks hmz2bio's synthetic annual range and the
// snow ramp: at 60° the range is 1 + 0.33·60 = 20.8 °C, so the warmest
// month is T + 10.4 and the coldest T − 10.4; snow is all of P when the
// warmest month is at or below 0 °C, none when the coldest is at or above
// it, and half at a mean of 0 °C.
func TestSeasonAndSnowfall(t *testing.T) {
	br := DefaultBiomeRules()
	w, c := br.Season(60, 5)
	if w != 15.4 || c != 5-10.4 {
		t.Errorf("Season(60°, 5 °C) = %v, %v; want 15.4, -5.4", w, c)
	}
	if w, c := br.Season(0, 27); w != 27.5 || c != 26.5 {
		t.Errorf("Season(0°, 27 °C) = %v, %v; want 27.5, 26.5", w, c)
	}
	for _, tc := range []struct{ t, want float64 }{
		{-30, 1000}, {-10.4, 1000}, {0, 500}, {10.4, 0}, {20, 0},
	} {
		if got := br.Snowfall(60, tc.t, 1000); got < tc.want-1e-9 || got > tc.want+1e-9 {
			t.Errorf("Snowfall(60°, %v °C, 1000 mm) = %v, want %v", tc.t, got, tc.want)
		}
	}
	// Ramp: monotone, in [0, P].
	prev := 2000.0
	for tc := -20.0; tc <= 20; tc += 0.5 {
		s := br.Snowfall(45, tc, 2000)
		if s > prev || s < 0 || s > 2000 {
			t.Fatalf("Snowfall(45°, %v) = %v after %v", tc, s, prev)
		}
		prev = s
	}
	// With no annual range, snow is all or nothing at 0 °C.
	flat := br
	flat.RangeBaseC, flat.RangePerDegC = 0, 0
	if flat.Snowfall(10, -0.1, 300) != 300 || flat.Snowfall(10, 0, 300) != 0 {
		t.Error("no-range snowfall")
	}
	if br.Arid(10) != 480 || br.Arid(-14) != 0 {
		t.Errorf("Arid(10) = %v, Arid(-14) = %v", br.Arid(10), br.Arid(-14))
	}
}

// TestBiomeRows checks each row of the table with a climate that reaches
// it and no earlier row.
func TestBiomeRows(t *testing.T) {
	br := DefaultBiomeRules()
	cc := func(lat, sea, temp, p, lift float64) CellClimate {
		return CellClimate{LatitudeDeg: lat, SeaLevelC: sea, TemperatureC: temp, PrecipitationMM: p, LiftM: lift}
	}
	for _, tc := range []struct {
		name string
		c    CellClimate
		l    Landform
		want Biome
		ice  bool
	}{
		// 70°: range 24.1, half 12.05.
		{"permanent ice", cc(70, -8, -15, 400, 0), Mountains, Clear, true},
		{"ice needs snow: polar desert", cc(70, -8, -15, 250, 0), Mountains, PolarDesert, false},
		{"snow at the limit", cc(70, -8, -15, 300, 0), Plains, Clear, true},
		{"tundra", cc(70, -8, -5, 400, 0), Plains, Tundra, false},
		// 30°: range 10.9; the sea-level warmest month (25.45) is above
		// the tree line, so a cold cell is cold from height.
		{"alpine", cc(30, 20, -2, 600, 0), Mountains, Alpine, false},
		{"alpine glacier", cc(30, 20, -6, 600, 0), Mountains, Clear, true},
		// 25°: aridity limit at 22 °C is 720 mm.
		{"desert", cc(25, 22.5, 22, 300, 0), Plains, Desert, false},
		{"scrubland", cc(25, 22.5, 22, 600, 0), Plains, Scrubland, false},
		// 45°: at 10 °C the limit is 480 mm.
		{"steppe", cc(45, 12, 10, 400, 0), Plains, Steppe, false},
		{"cold desert", cc(45, 12, 10, 200, 0), Plains, Desert, false},
		// 10°: sea-level coldest month 24.85 °C, the tropics.
		{"cloud forest", cc(10, 27, 15, 1500, 200), Mountains, CloudForest, false},
		{"cloud forest needs lift", cc(10, 27, 15, 1500, 50), Mountains, TemperateForest, false},
		{"tropical montane forest", cc(10, 27, 15, 1500, 200), Plains, TemperateForest, false},
		{"tropical rainforest", cc(5, 27, 26, 2500, 0), Plains, TropicalRainforest, false},
		{"tropical dry forest", cc(5, 27, 26, 1500, 0), Plains, TropicalDryForest, false},
		{"savanna", cc(5, 27, 26, 1000, 0), Plains, Savanna, false},
		{"boreal forest", cc(55, 4, 2, 600, 0), Hills, BorealForest, false},
		{"temperate rainforest", cc(45, 12, 10, 2100, 0), Plains, TemperateRainforest, false},
		{"grassland", cc(45, 12, 10, 600, 0), Plains, Grassland, false},
		{"temperate forest", cc(45, 12, 10, 1000, 0), Plains, TemperateForest, false},
	} {
		b, ice := br.Biome(tc.c, tc.l)
		if b != tc.want || ice != tc.ice {
			t.Errorf("%s: %s (ice %v), want %s (ice %v)", tc.name, b, ice, tc.want, tc.ice)
		}
	}
}

// TestWetland checks the wetland subtypes in hmz2bio's order.
func TestWetland(t *testing.T) {
	br := DefaultBiomeRules()
	hot := CellClimate{LatitudeDeg: 5, SeaLevelC: 27, TemperatureC: 26, PrecipitationMM: 2500}
	cool := CellClimate{LatitudeDeg: 50, SeaLevelC: 8, TemperatureC: 6, PrecipitationMM: 900}
	temperate := CellClimate{LatitudeDeg: 40, SeaLevelC: 14.5, TemperatureC: 14, PrecipitationMM: 1200}
	dry := CellClimate{LatitudeDeg: 25, SeaLevelC: 22.5, TemperatureC: 22, PrecipitationMM: 500}
	for _, tc := range []struct {
		name  string
		c     CellClimate
		b     Biome
		coast bool
		want  Surface
	}{
		{"dry is salt flats, coast or not", dry, Scrubland, true, SaltFlats},
		{"tropical coast", hot, TropicalRainforest, true, Mangroves},
		{"tropical inland forest", hot, TropicalRainforest, false, Swamps},
		{"cool coast", cool, BorealForest, true, Bogs},
		{"temperate forest", temperate, TemperateForest, false, Swamps},
		{"temperate grassland", temperate, Grassland, true, Marshes},
	} {
		if got := br.Wetland(tc.c, tc.b, tc.coast); got != tc.want {
			t.Errorf("%s: %s, want %s", tc.name, got, tc.want)
		}
	}
}

// coverWorld is a hand-built cover input over a small mesh: every playable
// cell plains land at 45° with a temperate, humid climate, the rim water.
type coverWorld struct {
	m     *mesh.Mesh
	res   *Result
	in    CoverInput
	lakes *basin.Lakes
}

func newCoverWorld(t *testing.T) *coverWorld {
	t.Helper()
	m, _ := smallWorld(t, 2, "square", 600)
	n := len(m.Cells)
	w := &coverWorld{m: m, res: &Result{Landform: make([]Landform, n), Depth: make([]Depth, n), SeaSteps: make([]int, n), Volcano: make([]bool, n)}}
	w.lakes = &basin.Lakes{Lake: make([]int, n)}
	w.in = CoverInput{
		Land: make([]bool, n), Lakes: w.lakes,
		LatitudeDeg: make([]float64, n), SeaLevelC: make([]float64, n), TemperatureC: make([]float64, n),
		PrecipitationMM: make([]float64, n), Aridity: make([]float64, n), LiftM: make([]float64, n),
		RiverClass: make([]edges.RiverClass, len(m.Edges)),
	}
	for i, c := range m.Cells {
		w.lakes.Lake[i] = basin.None
		w.in.LatitudeDeg[i], w.in.SeaLevelC[i], w.in.TemperatureC[i], w.in.PrecipitationMM[i], w.in.Aridity[i] = 45, 12, 12, 1000, 1
		if c.Rim {
			w.res.Landform[i], w.res.Depth[i] = SaltWater, Deep
			continue
		}
		w.in.Land[i] = true
		w.res.Landform[i] = Plains
	}
	return w
}

// cold makes cell i cold enough for permanent ice at 70° (warmest month
// −2.95 °C) with p mm of precipitation, all snow.
func (w *coverWorld) cold(i int, p float64) {
	w.in.LatitudeDeg[i], w.in.SeaLevelC[i], w.in.TemperatureC[i], w.in.PrecipitationMM[i] = 70, -8, -15, p
}

func (w *coverWorld) cover(t *testing.T) *Result {
	t.Helper()
	if err := w.res.Cover(w.m, w.in, DefaultBiomeRules()); err != nil {
		t.Fatal(err)
	}
	return w.res
}

// picker hands out interior cells whose neighborhoods (the cells and their
// neighbors) do not touch any earlier pick's, so hand-set features never
// meet.
type picker struct {
	m     *mesh.Mesh
	cands []int
	used  map[int]bool
}

func newPicker(m *mesh.Mesh) *picker {
	return &picker{m: m, cands: interior(m), used: map[int]bool{}}
}

// pick returns a cell whose radius-2 neighborhood is free, and reserves it.
func (p *picker) pick(t *testing.T) int {
	t.Helper()
	for len(p.cands) > 0 {
		i := p.cands[0]
		p.cands = p.cands[1:]
		ring := p.ring(i, 2)
		if slices.ContainsFunc(ring, func(j int) bool { return p.used[j] || p.m.Cells[j].Rim }) {
			continue
		}
		for _, j := range ring {
			p.used[j] = true
		}
		return i
	}
	t.Fatal("out of free cells")
	return -1
}

func (p *picker) ring(i, r int) []int {
	out := []int{i}
	for range r {
		for _, u := range slices.Clone(out) {
			for _, v := range p.m.Cells[u].Neighbors {
				if !slices.Contains(out, v) {
					out = append(out, v)
				}
			}
		}
	}
	return out
}

// TestCoverIce checks permanent ice and the G3 split: a mountain with one
// cold neighbor (a glacier tongue), a mountain inside a broad cold plain
// (glacier, the plain an ice field), a small cold plain with no mountain
// (an ice field), cold dry land (polar desert, bare), and a cold snowy
// playa (salt flats, never ice).
func TestCoverIce(t *testing.T) {
	w := newCoverWorld(t)
	p := newPicker(w.m)
	nb := func(i int) []int { return w.m.Cells[i].Neighbors }

	tongueM := p.pick(t)
	tongue := nb(tongueM)[0]
	w.res.Landform[tongueM] = Mountains
	w.cold(tongueM, 500)
	w.cold(tongue, 500)

	broadM := p.pick(t)
	w.res.Landform[broadM] = Mountains
	w.cold(broadM, 500)
	broad := nb(broadM)
	for _, j := range broad {
		w.cold(j, 500)
	}

	capA := p.pick(t)
	capB := nb(capA)[0]
	w.cold(capA, 400)
	w.cold(capB, 400)

	dryM := p.pick(t)
	w.res.Landform[dryM] = Mountains
	w.cold(dryM, 200)

	pl := p.pick(t)
	w.cold(pl, 500)
	w.res.Landform[pl] = Hills
	w.lakes.Playas = []basin.Playa{{Cell: pl}}

	r := w.cover(t)
	check := func(name string, i int, b Biome, s Surface) {
		t.Helper()
		if r.Biome[i] != b || r.Surface[i] != s {
			t.Errorf("%s (cell %d): %s/%s, want %s/%s", name, i, r.Biome[i], r.Surface[i], b, s)
		}
	}
	check("tongue mountain", tongueM, Clear, Glacier)
	check("tongue", tongue, Clear, Glacier)
	check("broad mountain", broadM, Clear, Glacier)
	if len(broad) < 5 {
		t.Fatalf("broad plain of %d cells", len(broad))
	}
	for _, j := range broad {
		check("broad plain", j, Clear, IceField)
	}
	check("small cap", capA, Clear, IceField)
	check("small cap", capB, Clear, IceField)
	check("cold dry mountain", dryM, PolarDesert, SurfaceNone)
	check("cold playa", pl, PolarDesert, SaltFlats)
	ice := 0
	for i := range w.m.Cells {
		if r.Surface[i].IsIce() {
			ice++
			if r.Biome[i] != Clear || r.Wetness[i] != 0 {
				t.Errorf("ice cell %d: biome %s, wetness %b", i, r.Biome[i], r.Wetness[i])
			}
		} else if r.Biome[i] == Clear {
			t.Errorf("cell %d: biome clear without ice", i)
		}
	}
	if want := 2 + 1 + len(broad) + 2; ice != want || r.SurfaceCount(Glacier)+r.SurfaceCount(IceField) != want {
		t.Errorf("%d ice cells, want %d", ice, want)
	}
}

// TestCoverWetness checks the wetness signals and the wetlands they make:
// a flats cell beside a major river (but not one beside a stream), a
// plains cell on a lake shore, a flats cell with surplus (but not a plains
// or hills cell with surplus); and pack ice on cold water but never on the
// rim.
func TestCoverWetness(t *testing.T) {
	w := newCoverWorld(t)
	p := newPicker(w.m)
	m := w.m

	major := p.pick(t)
	w.res.Landform[major] = Flats
	w.in.RiverClass[m.Cells[major].Edges[0]] = edges.MajorRiver
	stream := p.pick(t)
	w.res.Landform[stream] = Flats
	w.in.RiverClass[m.Cells[stream].Edges[0]] = edges.River

	lake := p.pick(t)
	shore := m.Cells[lake].Neighbors[0]
	w.in.Land[lake] = false
	w.res.Landform[lake] = FreshWater
	w.lakes.Lake[lake] = 0
	w.lakes.Lakes = []basin.Lake{{Cells: []int{lake}, Kind: basin.KindLake}}
	hillShore := m.Cells[lake].Neighbors[1]
	w.res.Landform[hillShore] = Hills

	wetFlats := p.pick(t)
	w.res.Landform[wetFlats] = Flats
	w.in.Aridity[wetFlats] = 2
	wetPlains := p.pick(t)
	w.in.Aridity[wetPlains] = 5
	almost := p.pick(t)
	w.res.Landform[almost] = Flats
	w.in.Aridity[almost] = 1.99

	frozen := p.pick(t)
	w.in.Land[frozen] = false
	w.res.Landform[frozen] = SaltWater
	w.cold(frozen, 300)
	w.cold(lake, 300)
	rim := slices.IndexFunc(m.Cells, func(c mesh.Cell) bool { return c.Rim })
	w.cold(rim, 300)

	r := w.cover(t)
	for _, tc := range []struct {
		name string
		i    int
		sig  uint8
		s    Surface
	}{
		{"major river flats", major, WetRiver, Swamps}, // temperate forest
		{"river flats", stream, 0, SurfaceNone},
		{"lake shore plains", shore, WetShore, Swamps},
		{"lake shore hills", hillShore, 0, SurfaceNone},
		{"surplus flats", wetFlats, WetSurplus, Swamps},
		{"surplus plains", wetPlains, 0, SurfaceNone},
		{"almost surplus", almost, 0, SurfaceNone},
		{"frozen sea", frozen, 0, PackIce},
		{"frozen lake", lake, 0, PackIce},
		{"rim", rim, 0, SurfaceNone},
	} {
		if r.Wetness[tc.i] != tc.sig || r.Surface[tc.i] != tc.s {
			t.Errorf("%s (cell %d): wetness %03b, %s; want %03b, %s", tc.name, tc.i, r.Wetness[tc.i], r.Surface[tc.i], tc.sig, tc.s)
		}
	}
	if r.Biome[rim] != BiomeNone || r.Biome[lake] != BiomeNone || r.Biome[frozen] != BiomeNone {
		t.Error("water or rim with a biome")
	}
	// Every plains neighbor of the lake is a shore wetland, the hills one
	// not.
	if want := 2 + len(m.Cells[lake].Neighbors) - 1; r.WetlandCount() != want || r.SurfaceCount(PackIce) != 2 {
		t.Errorf("%d wetlands, %d pack ice; want %d, 2", r.WetlandCount(), r.SurfaceCount(PackIce), want)
	}
	counts, total := r.BiomeHistogram()
	if total != len(m.Cells)-r.Count(m, SaltWater, false)-r.Count(m, FreshWater, false) || counts[TemperateForest-Clear] != total {
		t.Errorf("histogram %v of %d", counts, total)
	}
	if got := r.BiomeShares(true); !slices.Equal(got, []string{"tf100"}) {
		t.Errorf("BiomeShares = %q", got)
	}
}

// TestCoverMangroves checks that a hot ocean-coast wetland is mangroves and
// a hot lake-shore one is not.
func TestCoverMangroves(t *testing.T) {
	w := newCoverWorld(t)
	p := newPicker(w.m)
	m := w.m
	sea := p.pick(t)
	coast := m.Cells[sea].Neighbors[0]
	w.in.Land[sea] = false
	w.res.Landform[sea] = SaltWater
	lake := p.pick(t)
	shore := m.Cells[lake].Neighbors[0]
	w.in.Land[lake] = false
	w.res.Landform[lake] = FreshWater
	w.lakes.Lake[lake] = 0
	w.lakes.Lakes = []basin.Lake{{Cells: []int{lake}, Kind: basin.KindLake}}
	for _, i := range []int{coast, shore} {
		w.res.Landform[i] = Flats
		w.in.Aridity[i] = 3
		w.in.LatitudeDeg[i], w.in.SeaLevelC[i], w.in.TemperatureC[i], w.in.PrecipitationMM[i] = 5, 27, 26, 2500
	}
	r := w.cover(t)
	if r.Surface[coast] != Mangroves || r.Surface[shore] != Swamps {
		t.Errorf("ocean coast %s, lake shore %s; want mangroves, swamps", r.Surface[coast], r.Surface[shore])
	}
}

// TestCoverDeterminism covers a world twice and compares the encodings,
// which now end with each cell's biome and surface.
func TestCoverDeterminism(t *testing.T) {
	enc := func() []byte {
		w := newCoverWorld(t)
		for i := range w.m.Cells {
			w.in.TemperatureC[i] = float64(i%60) - 25
			w.in.PrecipitationMM[i] = float64((i*37)%3000) + 100
			w.in.Aridity[i] = float64(i%7) / 2
			w.in.LatitudeDeg[i] = float64(i % 90)
			w.res.Landform[i] = LandLandforms[i%len(LandLandforms)]
			if w.m.Cells[i].Rim {
				w.res.Landform[i] = SaltWater
			}
		}
		b, _ := w.cover(t).AppendBinary(nil)
		return b
	}
	a, b := enc(), enc()
	if !bytes.Equal(a, b) {
		t.Error("two covers differ")
	}
	w := newCoverWorld(t)
	n := len(w.m.Cells)
	if len(a) != 8+n*11+8+8+n*2 {
		t.Errorf("encoding is %d bytes for %d cells", len(a), n)
	}
}

func TestCoverErrors(t *testing.T) {
	w := newCoverWorld(t)
	br := DefaultBiomeRules()
	in := w.in
	in.Aridity = in.Aridity[1:]
	if err := w.res.Cover(w.m, in, br); err == nil {
		t.Error("short aridity accepted")
	}
	in = w.in
	in.RiverClass = in.RiverClass[1:]
	if err := w.res.Cover(w.m, in, br); err == nil {
		t.Error("short river classes accepted")
	}
	in = w.in
	in.Lakes = nil
	if err := w.res.Cover(w.m, in, br); err == nil {
		t.Error("no lakes accepted")
	}
	in = w.in
	in.TemperatureC = slices.Clone(in.TemperatureC)
	in.TemperatureC[3] = 1 / zero()
	if err := w.res.Cover(w.m, in, br); err == nil {
		t.Error("infinite temperature accepted")
	}
}

func zero() float64 { return 0 }

// TestBiomeRenders checks the three cover renders' size and a cell's fill
// at its site in each.
func TestBiomeRenders(t *testing.T) {
	w := newCoverWorld(t)
	m := w.m
	_, f := smallWorld(t, 2, "square", 600)
	p := newPicker(m)
	glacier := p.pick(t)
	w.res.Landform[glacier] = Mountains
	w.cold(glacier, 500)
	wet := p.pick(t)
	w.res.Landform[wet] = Flats
	w.in.Aridity[wet] = 3
	plain := p.pick(t)
	r := w.cover(t)
	base := mesh.CellRender(f, m, func(int) color.RGBA { return color.RGBA{} })
	s := float64(mesh.RenderScale(f, m))
	at := func(img interface{ RGBAAt(x, y int) color.RGBA }, i int) color.RGBA {
		site := m.Cells[i].Site
		return img.RGBAAt(int(site.X*s/f.PitchX()), int(site.Y*s/f.PitchY()))
	}
	bio := BiomeRender(f, m, r)
	sur := SurfaceRender(f, m, r)
	wtn := WetnessRender(f, m, r, w.in.RiverClass, edges.MajorRiver)
	if bio.Rect != base.Rect || sur.Rect != base.Rect || wtn.Rect != base.Rect {
		t.Fatalf("render sizes %v %v %v, want %v", bio.Rect, sur.Rect, wtn.Rect, base.Rect)
	}
	for _, tc := range []struct {
		name string
		got  color.RGBA
		want color.RGBA
	}{
		{"biome plain", at(bio, plain), BiomeColor(TemperateForest)},
		{"biome glacier", at(bio, glacier), BiomeColor(Clear)},
		{"surface glacier", at(sur, glacier), SurfaceColor(Glacier)},
		{"surface wetland", at(sur, wet), SurfaceColor(Swamps)},
		{"surface bare", at(sur, plain), bareLandInk},
		{"wetness surplus", at(wtn, wet), wetSurplusInk},
		{"wetness dry", at(wtn, plain), dryLandInk},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, tc.got, tc.want)
		}
	}
	if Shade(BiomeColor(Desert), Mountains) == BiomeColor(Desert) || Shade(BiomeColor(Desert), Flats) != BiomeColor(Desert) {
		t.Error("Shade")
	}
	if BiomeColor(Biome(200)) != BiomeColor(BiomeNone) || SurfaceColor(Surface(200)) != SurfaceColor(SurfaceNone) {
		t.Error("out-of-range colors")
	}
}
