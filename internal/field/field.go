// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package field

import (
	"fmt"
	"math"
	"slices"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/internal/topo"
)

// MaxSamples is the largest raster New will allocate.
const MaxSamples = 1 << 28

// Field is a raster of finite float64 samples over a cylinder. Build one with
// New or FromConfig; the zero value is not valid. See the package
// documentation for the grid, lookup, and sampling conventions.
type Field struct {
	cyl    topo.Cylinder
	nx, ny int
	px, py float64
	data   []float64
}

// New returns a zero-filled field over c with a nominal sample spacing of
// spacingKm km. The spacing must be positive and finite, and the raster must
// hold at most MaxSamples samples.
func New(c topo.Cylinder, spacingKm float64) (*Field, error) {
	if !(spacingKm > 0) || math.IsInf(spacingKm, 1) {
		return nil, fmt.Errorf("field: spacing %v km must be positive and finite", spacingKm)
	}
	if c.W() == 0 {
		return nil, fmt.Errorf("field: cylinder is not initialized")
	}
	nxf := max(2, math.Round(c.W()/spacingKm))
	nyf := max(2, math.Round(c.H()/spacingKm))
	if !(nxf*nyf <= MaxSamples) {
		return nil, fmt.Errorf("field: spacing %v km over %v × %v km needs %v × %v samples, more than %d", spacingKm, c.W(), c.H(), nxf, nyf, MaxSamples)
	}
	nx, ny := int(nxf), int(nyf)
	return &Field{
		cyl:  c,
		nx:   nx,
		ny:   ny,
		px:   c.W() / nxf,
		py:   c.H() / nyf,
		data: make([]float64, nx*ny),
	}, nil
}

// FromConfig returns a zero-filled field over the resolved config's world at
// its raster spacing.
func FromConfig(cfg config.Config) (*Field, error) {
	c, err := topo.New(cfg.World.WidthKm, cfg.World.HeightKm, cfg.Rim.Km, cfg.Rim.FalloffKm)
	if err != nil {
		return nil, fmt.Errorf("field: %w", err)
	}
	return New(c, cfg.Raster.SpacingKm)
}

// Cylinder returns the world the field covers.
func (f *Field) Cylinder() topo.Cylinder { return f.cyl }

// NX returns the number of columns.
func (f *Field) NX() int { return f.nx }

// NY returns the number of rows.
func (f *Field) NY() int { return f.ny }

// Len returns the number of samples, NX·NY.
func (f *Field) Len() int { return len(f.data) }

// PitchX returns the east-west distance between columns, W/NX km.
func (f *Field) PitchX() float64 { return f.px }

// PitchY returns the north-south distance between rows, H/NY km.
func (f *Field) PitchY() float64 { return f.py }

// X returns the x coordinate of column i, (i + 0.5)·PitchX. It does not wrap
// i, so X(-1) is half a pitch west of 0.
func (f *Field) X(i int) float64 { return fmath.Mul(float64(i)+0.5, f.px) }

// Y returns the y coordinate of row j, (j + 0.5)·PitchY.
func (f *Field) Y(j int) float64 { return fmath.Mul(float64(j)+0.5, f.py) }

// Point returns the location of sample (i, j), with i wrapped.
func (f *Field) Point(i, j int) topo.Point {
	return topo.Point{X: f.X(fmath.FloorMod(i, f.nx)), Y: f.Y(j)}
}

// index returns the storage index of (i, j), wrapping i and panicking on a
// row outside [0, NY).
func (f *Field) index(i, j int) int {
	if j < 0 || j >= f.ny {
		panic(fmt.Sprintf("field: row %d outside [0, %d)", j, f.ny))
	}
	return j*f.nx + fmath.FloorMod(i, f.nx)
}

// At returns sample (i, j). The column wraps; a row outside [0, NY) panics.
func (f *Field) At(i, j int) float64 { return f.data[f.index(i, j)] }

// Set stores v at sample (i, j). The column wraps; a row outside [0, NY)
// panics, as does a NaN or infinite v.
func (f *Field) Set(i, j int, v float64) {
	checkFinite(v)
	f.data[f.index(i, j)] = v
}

// Fill stores v in every sample. It panics on a NaN or infinite v.
func (f *Field) Fill(v float64) {
	checkFinite(v)
	for k := range f.data {
		f.data[k] = v
	}
}

// SetFunc stores fn(i, j, p) at every sample, where p is the sample's
// location, calling fn in storage order (rows north to south, columns west to
// east). It panics if fn returns NaN or an infinity.
func (f *Field) SetFunc(fn func(i, j int, p topo.Point) float64) {
	for j := range f.ny {
		y := f.Y(j)
		for i := range f.nx {
			v := fn(i, j, topo.Point{X: f.X(i), Y: y})
			checkFinite(v)
			f.data[j*f.nx+i] = v
		}
	}
}

// Each calls fn for every sample in storage order: rows north to south,
// columns west to east.
func (f *Field) Each(fn func(i, j int, v float64)) {
	for j := range f.ny {
		for i := range f.nx {
			fn(i, j, f.data[j*f.nx+i])
		}
	}
}

// Values returns a copy of the samples in storage order.
func (f *Field) Values() []float64 { return slices.Clone(f.data) }

// Clone returns an independent copy of the field.
func (f *Field) Clone() *Field {
	g := *f
	g.data = slices.Clone(f.data)
	return &g
}

// Min returns the smallest sample.
func (f *Field) Min() float64 { return slices.Min(f.data) }

// Max returns the largest sample.
func (f *Field) Max() float64 { return slices.Max(f.data) }

// MinMax returns the smallest and largest samples.
func (f *Field) MinMax() (lo, hi float64) { return f.Min(), f.Max() }

// Percentile returns the nearest-rank p-th percentile of every sample; see
// the package Percentile.
func (f *Field) Percentile(p float64) (float64, error) {
	return Percentile(f.data, p)
}

// Sample returns the field at (x, y) km by bilinear interpolation: x wraps
// across the seam, and y is clamped to the first and last rows. It panics on
// a NaN or infinite coordinate.
func (f *Field) Sample(x, y float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) || math.IsNaN(y) || math.IsInf(y, 0) {
		panic(fmt.Sprintf("field: Sample(%v, %v): coordinate is not finite", x, y))
	}
	x = f.cyl.WrapX(x)
	y = min(max(y, f.Y(0)), f.Y(f.ny-1))

	// Column: the largest i in [−1, NX−1] with X(i) ≤ x. X(−1) < 0 ≤ x and
	// X(NX) > W > x bound the search.
	i0 := f.locate(x, f.px, f.X, -1, f.nx-1)
	tx := min((x-f.X(i0))/f.px, 1)
	i1 := i0 + 1

	// Row: the largest j in [0, NY−1] with Y(j) ≤ y.
	j0 := f.locate(y, f.py, f.Y, 0, f.ny-1)
	j1, ty := j0, 0.0
	if j0 < f.ny-1 {
		j1 = j0 + 1
		ty = min((y-f.Y(j0))/f.py, 1)
	}

	north := lerp(f.At(i0, j0), f.At(i1, j0), tx)
	south := lerp(f.At(i0, j1), f.At(i1, j1), tx)
	return lerp(north, south, ty)
}

// locate returns the largest k in [lo, hi] with pos(k) ≤ v, starting from the
// estimate ⌊v/pitch − 0.5⌋ and correcting it for rounding. v must lie in
// [pos(lo), pos(hi+1)).
func (f *Field) locate(v, pitch float64, pos func(int) float64, lo, hi int) int {
	k := min(max(int(math.Floor(v/pitch-0.5)), lo), hi)
	for k < hi && pos(k+1) <= v {
		k++
	}
	for k > lo && pos(k) > v {
		k--
	}
	return k
}

// lerp returns a + t·(b − a) with the product rounded before the sum, and a
// itself when t is 0.
func lerp(a, b, t float64) float64 {
	if t == 0 {
		return a
	}
	return fmath.MulAdd(t, b-a, a)
}

func checkFinite(v float64) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		panic(fmt.Sprintf("field: value %v is not finite", v))
	}
}
