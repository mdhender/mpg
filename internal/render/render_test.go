// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package render

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/topo"
)

// testField returns a 256 × 128 field at 2 km spacing whose heights, in
// meters, come from integer arithmetic only, so they are identical on every
// architecture: a sea floor with periodic ripples and three islands, one of
// which straddles the seam. Columns are shifted west by shift, so
// testField(t, k).At(i, j) == testField(t, 0).At(i+k, j).
func testField(t *testing.T, shift int) *field.Field {
	t.Helper()
	c, err := topo.New(512, 256, 2, 6)
	if err != nil {
		t.Fatalf("topo.New: %v", err)
	}
	f, err := field.New(c, 2)
	if err != nil {
		t.Fatalf("field.New: %v", err)
	}
	nx := f.NX()
	islands := []struct{ cx, cy, r, peak int }{
		{4, 60, 40, 5200},    // across the seam
		{120, 50, 30, 3800},  // round island
		{190, 85, 36, 4600},  // southern island
		{150, 100, 12, 3200}, // islet
	}
	f.SetFunc(func(i, j int, _ topo.Point) float64 {
		i = (i + shift) % nx
		tri := func(v, period int) int { // triangle wave, 0..period/2
			v %= period
			return min(v, period-v)
		}
		h := -3000 + 40*tri(i, 32) + 25*tri(j+tri(i, 64)/2, 20)
		for _, is := range islands {
			dx := abs(i - is.cx)
			dx = min(dx, nx-dx)
			dy := j - is.cy
			if d2, r2 := dx*dx+dy*dy, is.r*is.r; d2 < r2 {
				h += is.peak * (r2 - d2) / r2
				// a ridge of ±300 m bands inside each island
				h += 300 * (tri(dx+2*dy+64, 16) - 4) / 4
			}
		}
		return float64(h)
	})
	return f
}

func abs(v int) int { return max(v, -v) }

// The golden pixel hash of Relief(testField, DefaultReliefOptions()). It must
// be the same on amd64 and arm64; update it only for a deliberate change to
// the ramps, the shading, or the test field.
const goldenRelief = "07433d0d5347a4df4f6c3b414264e9cbad0ea3bdb4a6b5b88b72c4a5e221f93f"

func TestReliefGolden(t *testing.T) {
	f := testField(t, 0)
	img := Relief(f, DefaultReliefOptions())
	if b := img.Bounds(); b.Dx() != f.NX() || b.Dy() != f.NY() {
		t.Fatalf("image is %v, want %d × %d", b.Size(), f.NX(), f.NY())
	}
	if got := PixelHash(img); got != goldenRelief {
		t.Errorf("PixelHash = %s, want %s", got, goldenRelief)
	}
	if dir := os.Getenv("MPG_RENDER_DIR"); dir != "" {
		path := filepath.Join(dir, StageFile(3, "elevation", "test"))
		if err := WritePNGFile(path, img, Meta{Stage: "03-elevation-test", ConfigHash: "test"}); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", path)
	}
}

func TestPNGRoundTrip(t *testing.T) {
	img := Relief(testField(t, 0), DefaultReliefOptions())
	meta := Meta{
		Stage:      "03-elevation",
		ConfigHash: "0123456789abcdef",
		Extra:      []Text{{"mpg:version", "0.1.0-alpha"}, {"Comment", "two\nlines"}},
	}
	var buf bytes.Buffer
	if err := WritePNG(&buf, img, meta); err != nil {
		t.Fatal(err)
	}

	// the standard decoder still reads the file, and the pixels survive
	dec, err := png.Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("png.Decode: %v", err)
	}
	if got, want := PixelHash(dec), PixelHash(img); got != want {
		t.Errorf("decoded PixelHash = %s, want %s", got, want)
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if got := color.RGBAModel.Convert(dec.At(x, y)); got != img.RGBAAt(x, y) {
				t.Fatalf("pixel (%d, %d) = %v, want %v", x, y, got, img.RGBAAt(x, y))
			}
		}
	}

	texts, err := ReadPNGText(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("ReadPNGText: %v", err)
	}
	want := []Text{
		{KeyStage, "03-elevation"},
		{KeyConfigHash, "0123456789abcdef"},
		{"mpg:version", "0.1.0-alpha"},
		{"Comment", "two\nlines"},
	}
	if !slices.Equal(texts, want) {
		t.Errorf("ReadPNGText = %q, want %q", texts, want)
	}
	got, err := ReadMeta(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}
	if got.Stage != meta.Stage || got.ConfigHash != meta.ConfigHash || !slices.Equal(got.Extra, meta.Extra) {
		t.Errorf("ReadMeta = %+v, want %+v", got, meta)
	}

	// the chunks sit right after IHDR
	if p := 8 + 25; string(buf.Bytes()[p+4:p+8]) != "tEXt" {
		t.Errorf("chunk after IHDR is %q, want tEXt", buf.Bytes()[p+4:p+8])
	}

	// a corrupted text chunk is caught by its CRC
	bad := bytes.Clone(buf.Bytes())
	bad[8+25+8] ^= 1
	if _, err := ReadPNGText(bytes.NewReader(bad)); err == nil {
		t.Error("ReadPNGText accepted a bad CRC")
	}
	if _, err := ReadPNGText(bytes.NewReader(buf.Bytes()[:100])); err == nil {
		t.Error("ReadPNGText accepted a truncated file")
	}
}

func TestWritePNGFile(t *testing.T) {
	img := Sequential(testField(t, 0), Viridis)
	path := filepath.Join(t.TempDir(), StageFile(2, "layout", ""))
	if err := WritePNGFile(path, img, Meta{Stage: "02-layout", ConfigHash: "abc"}); err != nil {
		t.Fatal(err)
	}
	fh, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	m, err := ReadMeta(fh)
	if err != nil || m.Stage != "02-layout" || m.ConfigHash != "abc" {
		t.Errorf("ReadMeta = %+v, %v", m, err)
	}
}

func TestMetaValidation(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for _, m := range []Meta{
		{ConfigHash: "h"},
		{Stage: "s"},
		{Stage: "s", ConfigHash: "h", Extra: []Text{{KeyStage, "again"}}},
		{Stage: "s", ConfigHash: "h", Extra: []Text{{"a", "1"}, {"a", "2"}}},
		{Stage: "s", ConfigHash: "h", Extra: []Text{{"", "v"}}},
		{Stage: "s", ConfigHash: "h", Extra: []Text{{" lead", "v"}}},
		{Stage: "s", ConfigHash: "h", Extra: []Text{{"two  spaces", "v"}}},
		{Stage: "s", ConfigHash: "h", Extra: []Text{{string(make([]byte, 80)), "v"}}},
		{Stage: "s", ConfigHash: "h", Extra: []Text{{"k", "nul\x00"}}},
		{Stage: "s", ConfigHash: "h", Extra: []Text{{"k", "café"}}},
	} {
		if err := WritePNG(&bytes.Buffer{}, img, m); err == nil {
			t.Errorf("WritePNG accepted %+v", m)
		}
	}
}

func TestPixelHashTypes(t *testing.T) {
	rgba := image.NewRGBA(image.Rect(0, 0, 3, 2))
	nrgba := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	for y := range 2 {
		for x := range 3 {
			c := color.RGBA{uint8(10 * x), uint8(20 * y), 7, 255}
			rgba.SetRGBA(x, y, c)
			nrgba.Set(x, y, c)
		}
	}
	if PixelHash(rgba) != PixelHash(nrgba) {
		t.Error("RGBA and NRGBA of the same opaque pixels hash differently")
	}
	// a sub-image hashes its own bounds
	sub := rgba.SubImage(image.Rect(1, 0, 3, 2))
	other := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := range 2 {
		for x := range 2 {
			other.SetRGBA(x, y, rgba.RGBAAt(x+1, y))
		}
	}
	if PixelHash(sub) != PixelHash(other) {
		t.Error("sub-image hashes differently from a copy")
	}
	// dimensions are part of the hash
	if PixelHash(image.NewRGBA(image.Rect(0, 0, 2, 3))) == PixelHash(image.NewRGBA(image.Rect(0, 0, 3, 2))) {
		t.Error("2 × 3 and 3 × 2 blank images hash equally")
	}
}

func TestStageFile(t *testing.T) {
	for _, tc := range []struct {
		n             int
		name, variant string
		want          string
	}{
		{3, "elevation", "", "03-elevation.png"},
		{4, "mesh", "area", "04-mesh-area.png"},
		{4, "mesh", "short-edges", "04-mesh-short-edges.png"},
		{6, "sea-level", "", "06-sea-level.png"},
		{14, "export", "", "14-export.png"},
	} {
		if got := StageFile(tc.n, tc.name, tc.variant); got != tc.want {
			t.Errorf("StageFile(%d, %q, %q) = %q, want %q", tc.n, tc.name, tc.variant, got, tc.want)
		}
	}
	for _, tc := range []struct {
		n             int
		name, variant string
	}{
		{0, "x", ""}, {100, "x", ""}, {1, "", ""}, {1, "Mesh", ""}, {1, "a_b", ""},
		{1, "-a", ""}, {1, "a-", ""}, {1, "a--b", ""}, {1, "a", "B"}, {1, "a", "-"}, {1, "a.png", ""},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("StageFile(%d, %q, %q) did not panic", tc.n, tc.name, tc.variant)
				}
			}()
			StageFile(tc.n, tc.name, tc.variant)
		}()
	}
}

func TestRamp(t *testing.T) {
	r := NewRamp(
		Stop{0, color.RGBA{0, 0, 0, 255}},
		Stop{10, color.RGBA{100, 200, 50, 255}},
		Stop{20, color.RGBA{100, 0, 250, 255}},
	)
	for _, tc := range []struct {
		v    float64
		want color.RGBA
	}{
		{-5, color.RGBA{0, 0, 0, 255}},
		{0, color.RGBA{0, 0, 0, 255}},
		{5, color.RGBA{50, 100, 25, 255}},
		{2.5, color.RGBA{25, 50, 13, 255}}, // 12.5 rounds half up
		{10, color.RGBA{100, 200, 50, 255}},
		{15, color.RGBA{100, 100, 150, 255}},
		{20, color.RGBA{100, 0, 250, 255}},
		{99, color.RGBA{100, 0, 250, 255}},
	} {
		if got := r.At(tc.v); got != tc.want {
			t.Errorf("At(%v) = %v, want %v", tc.v, got, tc.want)
		}
	}
	if got, want := r.Frac(0.25), r.At(5); got != want {
		t.Errorf("Frac(0.25) = %v, want %v", got, want)
	}
	for _, ramp := range []Ramp{Land, Water, Gray, Viridis, BlueRed} {
		stops := ramp.Stops()
		lo, hi := ramp.Domain()
		if ramp.At(lo) != stops[0].Color || ramp.At(hi) != stops[len(stops)-1].Color {
			t.Errorf("ramp %v endpoints do not match its stops", stops)
		}
	}
	for _, bad := range [][]Stop{nil, {{1, color.RGBA{}}, {1, color.RGBA{}}}, {{2, color.RGBA{}}, {1, color.RGBA{}}}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("NewRamp(%v) did not panic", bad)
				}
			}()
			NewRamp(bad...)
		}()
	}
}

func TestFieldColorings(t *testing.T) {
	f := testField(t, 0)
	lo, hi := f.MinMax()
	seq := Sequential(f, Viridis)
	div := Diverging(f, BlueRed, 0)
	hyp := Hypsometric(f, 0)
	var sawLo, sawHi bool
	f.Each(func(i, j int, v float64) {
		if v == lo {
			sawLo = true
			if seq.RGBAAt(i, j) != Viridis.Frac(0) {
				t.Errorf("Sequential min at (%d, %d) is %v", i, j, seq.RGBAAt(i, j))
			}
		}
		if v == hi {
			sawHi = true
			if seq.RGBAAt(i, j) != Viridis.Frac(1) {
				t.Errorf("Sequential max at (%d, %d) is %v", i, j, seq.RGBAAt(i, j))
			}
		}
		want := Water.At(-v)
		if v > 0 {
			want = Land.At(v)
		}
		if hyp.RGBAAt(i, j) != want {
			t.Fatalf("Hypsometric (%d, %d) = %v, want %v", i, j, hyp.RGBAAt(i, j), want)
		}
	})
	if !sawLo || !sawHi {
		t.Error("extremes not visited")
	}
	// the sea floor (−3000 m) is the farthest from 0, so it is pure blue
	if lo >= 0 || -lo < hi {
		t.Fatalf("test field range [%v, %v] is not deeper than it is high", lo, hi)
	}
	f.Each(func(i, j int, v float64) {
		if v == lo && div.RGBAAt(i, j) != BlueRed.Frac(0) {
			t.Errorf("Diverging min at (%d, %d) is %v", i, j, div.RGBAAt(i, j))
		}
	})

	// constant fields take the start (Sequential) or middle (Diverging) color
	g := f.Clone()
	g.Fill(7)
	if c := Sequential(g, Viridis).RGBAAt(3, 3); c != Viridis.Frac(0) {
		t.Errorf("constant Sequential = %v", c)
	}
	if c := Diverging(g, BlueRed, 7).RGBAAt(3, 3); c != BlueRed.Frac(0.5) {
		t.Errorf("constant Diverging = %v", c)
	}
}

func TestHillshade(t *testing.T) {
	f := testField(t, 0)
	s := Hillshade(f, DefaultLight)
	if s.NX() != f.NX() || s.NY() != f.NY() {
		t.Fatalf("shade is %d × %d", s.NX(), s.NY())
	}

	// level ground shades at sin(altitude)
	g := f.Clone()
	g.Fill(123)
	flat := Hillshade(g, DefaultLight)
	for j := range flat.NY() {
		for i := range flat.NX() {
			if flat.At(i, j) != flat.Flat() {
				t.Fatalf("flat shade (%d, %d) = %v, want %v", i, j, flat.At(i, j), flat.Flat())
			}
		}
	}
	if d := flat.Flat() - 0.7071067811865476; d > 1e-15 || d < -1e-15 {
		t.Errorf("Flat = %v, want sin 45°", flat.Flat())
	}

	// a slope rising to the east faces west: lit by a western sun, dark under
	// an eastern one; a steeper z-factor exaggerates both
	ramp := f.Clone()
	ramp.SetFunc(func(_, j int, p topo.Point) float64 { return p.X * 100 }) // 100 m per km
	west := Hillshade(ramp, Light{Azimuth: 270, Altitude: 45, ZFactor: 1})
	east := Hillshade(ramp, Light{Azimuth: 90, Altitude: 45, ZFactor: 1})
	steep := Hillshade(ramp, Light{Azimuth: 90, Altitude: 45, ZFactor: 5})
	if !(west.At(50, 50) > west.Flat() && east.At(50, 50) < east.Flat() && steep.At(50, 50) < east.At(50, 50)) {
		t.Errorf("east-rising slope: west sun %v, east sun %v, steep %v, flat %v",
			west.At(50, 50), east.At(50, 50), steep.At(50, 50), west.Flat())
	}

	for _, l := range []Light{{315, 0, 1}, {315, 91, 1}, {315, 45, 0}, {315, 45, -1}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Hillshade(%+v) did not panic", l)
				}
			}()
			Hillshade(f, l)
		}()
	}
}

// TestHillshadeWraps shifts the test field by k columns and checks that the
// shading shifts with it bit for bit, which holds only if the window at the
// seam reads across it.
func TestHillshadeWraps(t *testing.T) {
	f := testField(t, 0)
	s := Hillshade(f, DefaultLight)
	for _, k := range []int{1, 5, 128, 255} {
		g := testField(t, k)
		for i := range f.NX() {
			for j := range f.NY() {
				if g.At(i, j) != f.At(i+k, j) {
					t.Fatalf("shift %d: test field is not shifted at (%d, %d)", k, i, j)
				}
			}
		}
		sg := Hillshade(g, DefaultLight)
		for j := range f.NY() {
			for i := range f.NX() {
				if sg.At(i, j) != s.At(i+k, j) {
					t.Fatalf("shift %d: shade (%d, %d) = %v, want %v", k, i, j, sg.At(i, j), s.At(i+k, j))
				}
			}
		}
		if PixelHash(Relief(g, DefaultReliefOptions())) == PixelHash(Relief(f, DefaultReliefOptions())) {
			t.Errorf("shift %d: relief did not change", k)
		}
	}
	// the island across the seam really is shaded there: its seam columns are
	// not flat
	if s.At(0, 60+10) == s.Flat() && s.At(-1, 60+10) == s.Flat() {
		t.Error("seam island is flat at the seam")
	}
}

func TestApplyShade(t *testing.T) {
	f := testField(t, 0)
	s := Hillshade(f, DefaultLight)
	base := Hypsometric(f, 0)
	img := Hypsometric(f, 0)
	ApplyShade(img, s, 0, nil)
	if PixelHash(img) != PixelHash(base) {
		t.Error("strength 0 changed the image")
	}
	ApplyShade(img, s, 1, func(i, j int) bool { return false })
	if PixelHash(img) != PixelHash(base) {
		t.Error("an all-false mask changed the image")
	}
	ApplyShade(img, s, 1, nil)
	for j := range s.NY() {
		for i := range s.NX() {
			a, b := base.RGBAAt(i, j), img.RGBAAt(i, j)
			switch v := s.At(i, j); {
			case v < s.Flat() && (b.R > a.R || b.G > a.G || b.B > a.B):
				t.Fatalf("shadow at (%d, %d) brightened %v to %v", i, j, a, b)
			case v > s.Flat() && (b.R < a.R || b.G < a.G || b.B < a.B):
				t.Fatalf("lit slope at (%d, %d) darkened %v to %v", i, j, a, b)
			}
		}
	}
}

func TestDraw(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	red := color.RGBA{255, 0, 0, 255}
	count := func() int {
		n := 0
		for k := 0; k < len(img.Pix); k += 4 {
			if img.Pix[k] == 255 {
				n++
			}
		}
		return n
	}
	FillPolygon(img, []Pt{{2, 2}, {6, 2}, {6, 5}, {2, 5}}, red)
	if n := count(); n != 12 {
		t.Errorf("4 × 3 square filled %d pixels", n)
	}
	clear(img.Pix)
	Line(img, Pt{0, 10.5}, Pt{20, 10.5}, 1, red)
	if n := count(); n != 20 {
		t.Errorf("1-pixel horizontal line painted %d pixels", n)
	}
	clear(img.Pix)
	Disc(img, Pt{10, 10}, 2, red)
	if n := count(); n != 12 {
		t.Errorf("radius-2 disc painted %d pixels", n)
	}
	// clipping: shapes off the image paint nothing and do not panic
	clear(img.Pix)
	Disc(img, Pt{-50, -50}, 3, red)
	FillPolygon(img, []Pt{{30, 30}, {40, 30}, {40, 40}}, red)
	if n := count(); n != 0 {
		t.Errorf("off-image shapes painted %d pixels", n)
	}

	f := testField(t, 0)
	p := ToPixel(f, f.Point(7, 9))
	if p.X != 7.5 || p.Y != 9.5 {
		t.Errorf("ToPixel(sample (7, 9)) = %v, want (7.5, 9.5)", p)
	}
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
	const pkg = "github.com/mdhender/mpg/internal/render"
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
