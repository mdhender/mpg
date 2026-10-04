// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package fmath

import (
	"math"
	"strconv"
	"testing"
)

// The bit patterns in this file are the cross-architecture contract: they were
// produced on darwin/arm64 and confirmed identical on amd64, and every build of
// this package on every architecture must reproduce them exactly. A change to
// any of them changes generated worlds and must be deliberate.

var (
	negZero = math.Copysign(0, -1)
	inf     = math.Inf(1)
)

func pin(t *testing.T, call string, got float64, want uint64) {
	t.Helper()
	if bits := math.Float64bits(got); bits != want {
		t.Errorf("%s = %v (%#016x), want %v (%#016x)", call, got, bits, math.Float64frombits(want), want)
	}
}

func TestSinCosBits(t *testing.T) {
	cases := []struct {
		x        float64
		sin, cos uint64
	}{
		{0, 0x0000000000000000, 0x3ff0000000000000},                          // 0, 1
		{negZero, 0x8000000000000000, 0x3ff0000000000000},                    // negZero, 1
		{1e-300, 0x01a56e1fc2f8f359, 0x3ff0000000000000},                     // 1e-300, 1
		{1e-9, 0x3e112e0be826d695, 0x3ff0000000000000},                       // 1e-09, 1
		{0.5, 0x3fdeaee8744b05f0, 0x3fec1528065b7d50},                        // 0.479425538604203, 0.8775825618903728
		{-0.5, 0xbfdeaee8744b05f0, 0x3fec1528065b7d50},                       // -0.479425538604203, 0.8775825618903728
		{1, 0x3feaed548f090cee, 0x3fe14a280fb5068c},                          // 0.8414709848078965, 0.5403023058681398
		{-1, 0xbfeaed548f090cee, 0x3fe14a280fb5068c},                         // -0.8414709848078965, 0.5403023058681398
		{math.Pi / 4, 0x3fe6a09e667f3bcc, 0x3fe6a09e667f3bcd},                // 0.7071067811865475, 0.7071067811865476
		{math.Pi / 2, 0x3ff0000000000000, 0x3c91a62633145c00},                // 1, 6.123233995736757e-17
		{-math.Pi / 2, 0xbff0000000000000, 0x3c91a62633145c00},               // -1, 6.123233995736757e-17
		{math.Pi, 0x3ca1a62633145c00, 0xbff0000000000000},                    // 1.2246467991473515e-16, -1
		{3 * math.Pi / 2, 0xbff0000000000000, 0xbcaa79394c9e8a00},            // -1, -1.8369701987210272e-16
		{2 * math.Pi, 0xbcb1a62633145c00, 0x3ff0000000000000},                // -2.449293598294703e-16, 1
		{100 * math.Pi / 2, 0x3cd1b19140c0c000, 0x3ff0000000000000},          // 9.82193361864194e-16, 1
		{2 * math.Pi * 0.5 / 1000, 0x3f69bc62f04cce08, 0x3feffff5a6a681ff},   // 0.0031415874858795635, 0.9999950652018582
		{2 * math.Pi * 999.5 / 1000, 0xbf69bc62f04cce7a, 0x3feffff5a6a681ff}, // -0.003141587485879613, 0.9999950652018582
		{100, 0xbfe03425b78c4db8, 0x3feb981dbf665fdf},                        // -0.5063656411097588, 0.8623188722876839
		{-100, 0x3fe03425b78c4db8, 0x3feb981dbf665fdf},                       // 0.5063656411097588, 0.8623188722876839
		{1e6, 0xbfd6664b2568d867, 0x3fedf9df9906d32c},                        // -0.34999350217129294, 0.9367521275331447
		{536870911, 0x3fef18d5c013070d, 0xbfce3164dbe8bfd8},                  // 0.9717816115810095, -0.23588238466027067
		{536870912, 0x3fd4e67c0e2622df, 0xbfee3edd2dfeef90},                  // 0.3265676630185634, -0.9451738260608966
		{1e10, 0xbfdf334c7896a4e4, 0x3febf098901c931a},                       // -0.48750602508751073, 0.873119622676856
		{-1e15, 0xbfeb76f88136ceba, 0xbfe06c154609d33e},                      // -0.8582727931702359, -0.5131937377869702
		{1e22, 0xbfeb453ab76bf398, 0x3fe0be2cef01c8f3},                       // -0.8522008497671889, 0.5232147853951389
		{1e300, 0xbfea2c16b010e386, 0xbfe2699022adc4c1},                      // -0.8178819121159087, -0.5753861119575491
		{math.MaxFloat64, 0x3f7452fc98b34eb0, 0xbfefffe62ecfab75},            // 0.004961954789184084, -0.9999876894265599
	}
	for _, c := range cases {
		pin(t, "Sin("+fmtF(c.x)+")", Sin(c.x), c.sin)
		pin(t, "Cos("+fmtF(c.x)+")", Cos(c.x), c.cos)
		s, co := Sincos(c.x)
		pin(t, "Sincos("+fmtF(c.x)+").sin", s, c.sin)
		pin(t, "Sincos("+fmtF(c.x)+").cos", co, c.cos)
	}
}

func TestExpBits(t *testing.T) {
	cases := []struct {
		x    float64
		want uint64
	}{
		{0, 0x3ff0000000000000},        // 1
		{negZero, 0x3ff0000000000000},  // 1
		{1e-300, 0x3ff0000000000000},   // 1
		{3e-9, 0x3ff0000000ce288f},     // 1.000000003
		{0.5, 0x3ffa61298e1e069c},      // 1.6487212707001282
		{1, 0x4005bf0a8b145769},        // 2.718281828459045
		{-1, 0x3fd78b56362cef38},       // 0.36787944117144233
		{math.Ln2, 0x4000000000000000}, // 2
		{10, 0x40d5829dcf950560},       // 22026.465794806718
		{-10, 0x3f07cd79b5647c9a},      // 4.539992976248485e-05
		{100, 0x48f3494a9b171bf5},      // 2.6881171418161356e+43
		{-100, 0x36ea8c1f14e2af5d},     // 3.720075976020836e-44
		{700, 0x7f0d945df4f8ec8e},      // 1.0142320547350045e+304
		{709.78, 0x7fefe9ce5c4c52b4},   // 1.7928227943945155e+308
		{709.79, 0x7ff0000000000000},   // inf
		{-708, 0x0017c8ab2288c9ac},     // 3.3075530036384083e-308
		{-740, 0x0000000000000055},     // 4.2e-322
		{-745.1, 0x0000000000000001},   // 5e-324
		{-746, 0x0000000000000000},     // 0
		{-inf, 0x0000000000000000},     // 0
		{inf, 0x7ff0000000000000},      // inf
	}
	for _, c := range cases {
		pin(t, "Exp("+fmtF(c.x)+")", Exp(c.x), c.want)
	}
}

func TestAtanBits(t *testing.T) {
	cases := []struct {
		x    float64
		want uint64
	}{
		{0, 0x0000000000000000},       // 0
		{negZero, 0x8000000000000000}, // negZero
		{1e-300, 0x01a56e1fc2f8f359},  // 1e-300
		{0.5, 0x3fddac670561bb4f},     // 0.4636476090008061
		{0.66, 0x3fe2aafdde4d0c9f},    // 0.583373006993856
		{0.67, 0x3fe2e3caf996421e},    // 0.590306746935372
		{1, 0x3fe921fb54442d18},       // 0.7853981633974483
		{-1, 0xbfe921fb54442d18},      // -0.7853981633974483
		{2.4, 0x3ff2d0ead6066395},     // 1.176005207095135
		{2.5, 0x3ff30b6d796a4da8},     // 1.1902899496825317
		{-10, 0xbff789bd2c160053},     // -1.4711276743037345
		{1e300, 0x3ff921fb54442d18},   // 1.5707963267948966
		{inf, 0x3ff921fb54442d18},     // 1.5707963267948966
		{-inf, 0xbff921fb54442d18},    // -1.5707963267948966
	}
	for _, c := range cases {
		pin(t, "Atan("+fmtF(c.x)+")", Atan(c.x), c.want)
	}
}

// TestAtan2Bits covers all four quadrants, both axes with both signed zeros,
// and the infinite cases.
func TestAtan2Bits(t *testing.T) {
	cases := []struct {
		y, x float64
		want uint64
	}{
		{1, 1, 0x3fe921fb54442d18},           // 0.7853981633974483
		{1, -1, 0x4002d97c7f3321d2},          // 2.356194490192345
		{-1, -1, 0xc002d97c7f3321d2},         // -2.356194490192345
		{-1, 1, 0xbfe921fb54442d18},          // -0.7853981633974483
		{0, 1, 0x0000000000000000},           // 0
		{0, -1, 0x400921fb54442d18},          // 3.141592653589793
		{negZero, 1, 0x8000000000000000},     // negZero
		{negZero, -1, 0xc00921fb54442d18},    // -3.141592653589793
		{1, 0, 0x3ff921fb54442d18},           // 1.5707963267948966
		{-1, 0, 0xbff921fb54442d18},          // -1.5707963267948966
		{1, negZero, 0x3ff921fb54442d18},     // 1.5707963267948966
		{3, 4, 0x3fe4978fa3269ee1},           // 0.6435011087932844
		{-4, -3, 0xc001b6e192ebbe44},         // -2.214297435588181
		{1e-300, 1, 0x01a56e1fc2f8f359},      // 1e-300
		{1, 1e-300, 0x3ff921fb54442d18},      // 1.5707963267948966
		{1e300, -1e-300, 0x3ff921fb54442d18}, // 1.5707963267948966
		{inf, inf, 0x3fe921fb54442d18},       // 0.7853981633974483
		{-inf, -inf, 0xc002d97c7f3321d2},     // -2.356194490192345
		{inf, -inf, 0x4002d97c7f3321d2},      // 2.356194490192345
		{2, inf, 0x0000000000000000},         // 0
		{2, -inf, 0x400921fb54442d18},        // 3.141592653589793
		{-2, -inf, 0xc00921fb54442d18},       // -3.141592653589793
		{inf, 2, 0x3ff921fb54442d18},         // 1.5707963267948966
	}
	for _, c := range cases {
		pin(t, "Atan2("+fmtF(c.y)+", "+fmtF(c.x)+")", Atan2(c.y, c.x), c.want)
	}
}

func TestHypotBits(t *testing.T) {
	cases := []struct {
		p, q float64
		want uint64
	}{
		{3, 4, 0x4014000000000000},                             // 5
		{1, 1, 0x3ff6a09e667f3bcd},                             // 1.4142135623730951
		{0, 0, 0x0000000000000000},                             // 0
		{negZero, 0, 0x0000000000000000},                       // 0
		{-3, 0, 0x4008000000000000},                            // 3
		{0.1, 0.2, 0x3fcc9f25c5bfedda},                         // 0.223606797749979
		{-1e300, 1e300, 0x7e40e4d50f99b211},                    // 1.4142135623730952e+300
		{1e-310, 1e-310, 0x00001a088b6bf34f},                   // 1.4142135623731e-310
		{1e-200, 1e-200, 0x167151f68876f410},                   // 1.414213562373095e-200
		{5, 12, 0x402a000000000000},                            // 13
		{math.MaxFloat64, math.MaxFloat64, 0x7ff0000000000000}, // inf
		{inf, 1, 0x7ff0000000000000},                           // inf
	}
	for _, c := range cases {
		pin(t, "Hypot("+fmtF(c.p)+", "+fmtF(c.q)+")", Hypot(c.p, c.q), c.want)
	}
}

func TestNaNCases(t *testing.T) {
	nan := math.NaN()
	sinNaN, cosNaN := Sincos(nan)
	sinInf, cosInf := Sincos(-inf)
	for _, c := range []struct {
		name string
		got  float64
	}{
		{"Sin(NaN)", Sin(nan)},
		{"Sin(+Inf)", Sin(inf)},
		{"Sin(-Inf)", Sin(-inf)},
		{"Cos(NaN)", Cos(nan)},
		{"Cos(+Inf)", Cos(inf)},
		{"Cos(-Inf)", Cos(-inf)},
		{"Sincos(NaN).sin", sinNaN},
		{"Sincos(NaN).cos", cosNaN},
		{"Sincos(-Inf).sin", sinInf},
		{"Sincos(-Inf).cos", cosInf},
		{"Exp(NaN)", Exp(nan)},
		{"Atan(NaN)", Atan(nan)},
		{"Atan2(NaN, 1)", Atan2(nan, 1)},
		{"Atan2(1, NaN)", Atan2(1, nan)},
		{"Hypot(NaN, 1)", Hypot(nan, 1)},
		{"Hypot(1, NaN)", Hypot(1, nan)},
	} {
		if !math.IsNaN(c.got) {
			t.Errorf("%s = %v, want NaN", c.name, c.got)
		}
	}
	// An infinity dominates a NaN in Hypot.
	if got := Hypot(nan, -inf); !math.IsInf(got, 1) {
		t.Errorf("Hypot(NaN, -Inf) = %v, want +Inf", got)
	}
}

func fmtF(x float64) string {
	if x == 0 && math.Signbit(x) {
		return "-0"
	}
	return strconv.FormatFloat(x, 'g', -1, 64)
}
