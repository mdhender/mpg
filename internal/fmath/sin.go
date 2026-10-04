// Copyright (c) 2026 Michael D Henderson. All rights reserved.
//
// The algorithms in this file are copied from the Go standard library's math
// package (Copyright 2009 The Go Authors, BSD-style license; see
// https://go.dev/LICENSE) and, through it, from the Cephes math library by
// Stephen L. Moshier, with explicit roundings added.

package fmath

import (
	"math"
	"math/bits"
)

// Sin returns the sine of the radian argument x.
//
// Domain: all finite float64. For |x| < 2**29 the argument is reduced by
// Cody-Waite extended-precision subtraction of multiples of Pi/4; above that,
// by Payne-Hanek reduction against 1216 bits of 4/Pi, so even huge arguments
// are reduced to full precision. Accuracy: Cephes reports a peak relative error of
// 2.1e-16 on [-1.07e9, 1.07e9]; within 2 ulp of math.Sin everywhere.
//
// Special cases, as math.Sin:
//
//	Sin(±0) = ±0
//	Sin(±Inf) = NaN
//	Sin(NaN) = NaN
func Sin(x float64) float64 {
	switch {
	case x == 0 || math.IsNaN(x):
		return x
	case math.IsInf(x, 0):
		return math.NaN()
	}
	sign := false
	if x < 0 {
		x = -x
		sign = true
	}
	j, z := reduce(x)
	if j > 3 {
		sign = !sign
		j -= 4
	}
	var y float64
	if j == 1 || j == 2 {
		y = cosPoly(z)
	} else {
		y = sinPoly(z)
	}
	if sign {
		y = -y
	}
	return y
}

// Cos returns the cosine of the radian argument x.
//
// Domain, reduction, and accuracy are those of Sin.
//
// Special cases, as math.Cos:
//
//	Cos(±0) = 1
//	Cos(±Inf) = NaN
//	Cos(NaN) = NaN
func Cos(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return math.NaN()
	}
	sign := false
	x = math.Abs(x)
	j, z := reduce(x)
	if j > 3 {
		j -= 4
		sign = !sign
	}
	if j > 1 {
		sign = !sign
	}
	var y float64
	if j == 1 || j == 2 {
		y = sinPoly(z)
	} else {
		y = cosPoly(z)
	}
	if sign {
		y = -y
	}
	return y
}

// Sincos returns Sin(x), Cos(x), sharing one argument reduction. The results
// are bit-identical to calling Sin and Cos separately.
//
// Special cases, as math.Sincos:
//
//	Sincos(±0) = ±0, 1
//	Sincos(±Inf) = NaN, NaN
//	Sincos(NaN) = NaN, NaN
func Sincos(x float64) (sin, cos float64) {
	switch {
	case x == 0:
		return x, 1
	case math.IsNaN(x) || math.IsInf(x, 0):
		return math.NaN(), math.NaN()
	}
	sinSign, cosSign := false, false
	if x < 0 {
		x = -x
		sinSign = true
	}
	j, z := reduce(x)
	if j > 3 {
		j -= 4
		sinSign, cosSign = !sinSign, !cosSign
	}
	if j > 1 {
		cosSign = !cosSign
	}
	sin, cos = sinPoly(z), cosPoly(z)
	if j == 1 || j == 2 {
		sin, cos = cos, sin
	}
	if cosSign {
		cos = -cos
	}
	if sinSign {
		sin = -sin
	}
	return sin, cos
}

// reduceThreshold is the argument above which Cody-Waite reduction loses
// precision and Payne-Hanek reduction takes over.
const reduceThreshold = 1 << 29

// reduce returns the even octant j in [0, 7] of the non-negative finite x and
// the remainder z = x - j*Pi/4, |z| <= Pi/4.
func reduce(x float64) (j uint64, z float64) {
	const (
		pi4a = 7.85398125648498535156e-1  // 0x3fe921fb40000000, Pi/4 split into three parts
		pi4b = 3.77489470793079817668e-8  // 0x3e64442d00000000,
		pi4c = 2.69515142907905952645e-15 // 0x3ce8469898cc5170,
	)
	if x >= reduceThreshold {
		return trigReduce(x)
	}
	// Integer part of x/(Pi/4). The explicit rounding matters on targets that
	// lower float-to-uint64 conversion with a subtraction of 2**63, which would
	// otherwise fuse with the product (riscv64, ppc64, loong64).
	j = uint64(float64(x * (4 / math.Pi)))
	y := float64(j)
	if j&1 == 1 { // map zeros to origin
		j++
		y++
	}
	j &= 7 // octant modulo 2Pi
	z = ((x - float64(y*pi4a)) - float64(y*pi4b)) - float64(y*pi4c)
	return j, z
}

// sinPoly approximates sin(z) for |z| <= Pi/4.
func sinPoly(z float64) float64 {
	const (
		s0 = 1.58962301576546568060e-10 // 0x3de5d8fd1fd19ccd
		s1 = -2.50507477628578072866e-8 // 0xbe5ae5e5a9291f5d
		s2 = 2.75573136213857245213e-6  // 0x3ec71de3567d48a1
		s3 = -1.98412698295895385996e-4 // 0xbf2a01a019bfdf03
		s4 = 8.33333333332211858878e-3  // 0x3f8111111110f7d0
		s5 = -1.66666666666666307295e-1 // 0xbfc5555555555548
	)
	zz := z * z
	p := float64(s0*zz) + s1
	p = float64(p*zz) + s2
	p = float64(p*zz) + s3
	p = float64(p*zz) + s4
	p = float64(p*zz) + s5
	return z + float64(float64(z*zz)*p)
}

// cosPoly approximates cos(z) for |z| <= Pi/4.
func cosPoly(z float64) float64 {
	const (
		c0 = -1.13585365213876817300e-11 // 0xbda8fa49a0861a9b
		c1 = 2.08757008419747316778e-9   // 0x3e21ee9d7b4e3f05
		c2 = -2.75573141792967388112e-7  // 0xbe927e4f7eac4bc6
		c3 = 2.48015872888517045348e-5   // 0x3efa01a019c844f5
		c4 = -1.38888888888730564116e-3  // 0xbf56c16c16c14f91
		c5 = 4.16666666666665929218e-2   // 0x3fa555555555554b
	)
	zz := z * z
	p := float64(c0*zz) + c1
	p = float64(p*zz) + c2
	p = float64(p*zz) + c3
	p = float64(p*zz) + c4
	p = float64(p*zz) + c5
	return (1.0 - float64(0.5*zz)) + float64(float64(zz*zz)*p)
}

// trigReduce implements Payne-Hanek reduction for x >= reduceThreshold,
// returning the even octant j and z = (x mod Pi/4 remainder) in radians.
// The multiplication by 4/Pi is done in exact integer arithmetic; the only
// floating-point steps are a subtraction of 1 and a final product, neither of
// which can fuse.
func trigReduce(x float64) (j uint64, z float64) {
	const (
		pi4   = math.Pi / 4
		mask  = 0x7FF
		shift = 64 - 11 - 1
		bias  = 1023
	)
	if x < pi4 {
		return 0, x
	}
	// Extract the integer and exponent such that x = ix * 2 ** exp.
	ix := math.Float64bits(x)
	exp := int(ix>>shift&mask) - bias - shift
	ix &^= mask << shift
	ix |= 1 << shift
	// Use the exponent to extract the 3 appropriate uint64 digits from mPi4,
	// B ~ (z0, z1, z2), such that the product leading digit has the exponent -61.
	digit, bitshift := uint(exp+61)/64, uint(exp+61)%64
	z0 := (mPi4[digit] << bitshift) | (mPi4[digit+1] >> (64 - bitshift))
	z1 := (mPi4[digit+1] << bitshift) | (mPi4[digit+2] >> (64 - bitshift))
	z2 := (mPi4[digit+2] << bitshift) | (mPi4[digit+3] >> (64 - bitshift))
	// Multiply mantissa by the digits and extract the upper two digits (hi, lo).
	z2hi, _ := bits.Mul64(z2, ix)
	z1hi, z1lo := bits.Mul64(z1, ix)
	z0lo := z0 * ix
	lo, c := bits.Add64(z1lo, z2hi, 0)
	hi, _ := bits.Add64(z0lo, z1hi, c)
	// The top 3 bits of hi give the octant.
	j = hi >> 61
	// Clear them and shift the remainder to a 52-bit mantissa.
	hi = hi<<3 | lo>>61
	lz := uint(bits.LeadingZeros64(hi))
	e := uint64(bias - (lz + 1))
	hi = (hi << (lz + 1)) | (lo >> (64 - (lz + 1)))
	hi >>= 64 - shift
	hi |= e << shift
	z = math.Float64frombits(hi)
	// Map zeros to origin.
	if j&1 == 1 {
		j++
		j &= 7
		z--
	}
	// Multiply the fractional part by Pi/4.
	return j, float64(z * pi4) // feeds z + ... in sinPoly
}

// mPi4 is the binary digits of 4/Pi as a uint64 array: 4/Pi = Sum
// mPi4[i]*2^(-64*i), with 19 64-bit digits and the integer part. That is
// enough to reduce any finite float64.
var mPi4 = [...]uint64{
	0x0000000000000001,
	0x45f306dc9c882a53,
	0xf84eafa3ea69bb81,
	0xb6c52b3278872083,
	0xfca2c757bd778ac3,
	0x6e48dc74849ba5c0,
	0x0c925dd413a32439,
	0xfc3bd63962534e7d,
	0xd1046bea5d768909,
	0xd338e04d68befc82,
	0x7323ac7306a673e9,
	0x3908bf177bf25076,
	0x3ff12fffbc0b301f,
	0xde5e2316b414da3e,
	0xda6cfd9e4f96136e,
	0x9e8c7ecd3cbfd45a,
	0xea4f758fd7cbe2f6,
	0x7a0e73ef14a525d4,
	0xd7f6bf623f1aba10,
	0xac06608df8f6d757,
}
