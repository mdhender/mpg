// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package noise

import (
	"math"

	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/internal/seed"
)

// Version is the noise algorithm version. It is the version argument to
// seed.Derive for noise keys; any change to the bits this package produces
// must bump it.
const Version = "noise/1"

// Source is seeded 3-D simplex noise. The zero value is a valid source keyed
// by (0, 0), but sources are normally built by New or NewSource.
type Source struct {
	k1, k2 uint64
}

// NewSource returns the noise source keyed by (seed1, seed2), typically the
// two values from seed.Derive.
func NewSource(seed1, seed2 uint64) Source {
	return Source{k1: seed1, k2: seed2}
}

// New returns the noise source for the given world seed and stage name,
// keyed by seed.Derive(world, stage, Version). It panics on an empty stage.
func New(world uint64, stage string) Source {
	return NewSource(seed.Derive(world, stage, Version))
}

// Stream returns an independent source derived from s and n. Octaves and warp
// axes each take their own stream so no two share a lattice.
func (s Source) Stream(n uint64) Source {
	h := mix64(n + streamInit)
	return Source{k1: mix64(s.k1 ^ h), k2: mix64(s.k2 + h)}
}

// mixInit is the SplitMix64 increment (the golden ratio's fraction); it keeps
// a zero key from being a fixed point of the mixer. streamInit separates
// stream derivation from lattice hashing.
const (
	mixInit    uint64 = 0x9e3779b97f4a7c15
	streamInit uint64 = 0x6a09e667f3bcc909 // fraction of sqrt(2)
)

// mix64 is the SplitMix64 finalizer. Integer arithmetic wraps, as intended.
func mix64(x uint64) uint64 {
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return x
}

// hash3 hashes a lattice point with the source's key, after wgva's Hash3.
func (s Source) hash3(i, j, k int64) uint64 {
	h := mix64(s.k1 + mixInit)
	h = mix64(h ^ s.k2)
	h = mix64(h ^ uint64(i))
	h = mix64(h ^ uint64(j))
	h = mix64(h ^ uint64(k))
	return h
}

// gradients are the twelve edge midpoints of the cube, all of length √2, so no
// direction carries more amplitude than another.
var gradients = [12][3]float64{
	{1, 1, 0}, {-1, 1, 0}, {1, -1, 0}, {-1, -1, 0},
	{1, 0, 1}, {-1, 0, 1}, {1, 0, -1}, {-1, 0, -1},
	{0, 1, 1}, {0, -1, 1}, {0, 1, -1}, {0, -1, -1},
}

// The skew constants of the 3-D simplex lattice, F3 = 1/3 and G3 = 1/6, and
// the second and third corner unskews 2·G3 and 3·G3. Go evaluates each
// constant expression exactly and rounds once.
const (
	f3   = 1.0 / 3
	g3   = 1.0 / 6
	g3x2 = 2.0 / 6
	g3x3 = 3.0 / 6
)

// simplexScale maps the raw corner sum onto [−1, 1]. The radial falloff is
// (0.5 − d²)⁴, which reaches zero before a corner's influence crosses a
// simplex boundary, so the noise is continuous. A hill-climbing search from
// four million random starts over 200 keys puts the extreme of the raw sum
// near 0.013007 (scale ≈ 76.9); 76 leaves about 1% headroom, and the clamp
// in Noise3 guarantees the bound. TestNoise3Range repeats a smaller search.
const simplexScale = 76.0

// Noise3 returns simplex noise at (x, y, z) in [−1, 1]. One lattice step is
// one unit. It is continuous, zero at every lattice point, and a pure
// function of the key and the coordinates. A NaN or infinite coordinate, or
// one beyond ±2⁵⁰, returns 0.
func (s Source) Noise3(x, y, z float64) float64 {
	n := s.raw3(x, y, z)
	return min(max(fmath.Mul(simplexScale, n), -1), 1)
}

// raw3 is Noise3 without the scale and clamp, for the range search.
func (s Source) raw3(x, y, z float64) float64 {
	if !inDomain(x) || !inDomain(y) || !inDomain(z) {
		return 0
	}
	// Skew onto the simplex lattice and find the cell.
	sk := fmath.Mul((x+y)+z, f3)
	fi := math.Floor(x + sk)
	fj := math.Floor(y + sk)
	fk := math.Floor(z + sk)
	i, j, k := int64(fi), int64(fj), int64(fk)

	// Unskew the cell origin and take the offset within the cell.
	t := fmath.Mul((fi+fj)+fk, g3)
	x0 := x - (fi - t)
	y0 := y - (fj - t)
	z0 := z - (fk - t)

	// Which of the six simplices holds the point decides corners 1 and 2.
	var i1, j1, k1, i2, j2, k2 int64
	switch {
	case x0 >= y0 && y0 >= z0:
		i1, j1, k1, i2, j2, k2 = 1, 0, 0, 1, 1, 0
	case x0 >= y0 && x0 >= z0:
		i1, j1, k1, i2, j2, k2 = 1, 0, 0, 1, 0, 1
	case x0 >= y0:
		i1, j1, k1, i2, j2, k2 = 0, 0, 1, 1, 0, 1
	case y0 < z0:
		i1, j1, k1, i2, j2, k2 = 0, 0, 1, 0, 1, 1
	case x0 < z0:
		i1, j1, k1, i2, j2, k2 = 0, 1, 0, 0, 1, 1
	default:
		i1, j1, k1, i2, j2, k2 = 0, 1, 0, 1, 1, 0
	}

	x1 := (x0 - float64(i1)) + g3
	y1 := (y0 - float64(j1)) + g3
	z1 := (z0 - float64(k1)) + g3
	x2 := (x0 - float64(i2)) + g3x2
	y2 := (y0 - float64(j2)) + g3x2
	z2 := (z0 - float64(k2)) + g3x2
	x3 := (x0 - 1) + g3x3
	y3 := (y0 - 1) + g3x3
	z3 := (z0 - 1) + g3x3

	// Corners are summed in a fixed order: addition is not associative.
	n := s.corner(i, j, k, x0, y0, z0)
	n += s.corner(i+i1, j+j1, k+k1, x1, y1, z1)
	n += s.corner(i+i2, j+j2, k+k2, x2, y2, z2)
	n += s.corner(i+1, j+1, k+1, x3, y3, z3)
	return n
}

// corner returns one corner's contribution: (0.5 − d²)⁴ times the dot product
// of the corner's gradient with the offset to the point.
func (s Source) corner(i, j, k int64, dx, dy, dz float64) float64 {
	t := ((0.5 - fmath.Mul(dx, dx)) - fmath.Mul(dy, dy)) - fmath.Mul(dz, dz)
	if t <= 0 {
		return 0
	}
	g := &gradients[s.hash3(i, j, k)%12]
	dot := (fmath.Mul(g[0], dx) + fmath.Mul(g[1], dy)) + fmath.Mul(g[2], dz)
	t2 := fmath.Mul(t, t)
	return fmath.Mul(fmath.Mul(t2, t2), dot)
}

// maxCoord bounds Noise3's input. Beyond it the lattice index conversion to
// int64 would not be portable (and float64 has no fraction left to sample).
const maxCoord = 1 << 50

// inDomain reports whether v is finite and within ±maxCoord.
func inDomain(v float64) bool { return math.Abs(v) <= maxCoord }
