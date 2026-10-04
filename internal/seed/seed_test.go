// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package seed

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"testing"
)

// golden pins the stage seed and the first three PCG draws for a set of
// (world, stage, version) tuples. These values are a compatibility contract:
// a change here changes every generated world.
var golden = []struct {
	world          uint64
	stage, version string
	seed1, seed2   uint64
	draws          [3]uint64
}{
	{0x0, "layout", "1", 0x45ede7232737fa50, 0xf8c80c40244be2dd, [3]uint64{0x72e5e5fe5120b725, 0xd56955c6cd559a07, 0x1123b87805b5bde6}},
	{math.MaxUint64, "layout", "1", 0x8a6274e96c68f4e6, 0x4bcbe40a0f262744, [3]uint64{0x0a177d0b68c88565, 0x7992db645ee4caa2, 0xe70067ee077a48fc}},
	{42, "layout", "1", 0xfcdbd8380cf4cf12, 0x4f0e9509e2cc2981, [3]uint64{0xbd620759299f5112, 0x0abaaae3646efddc, 0xc347d74f7e5893fa}},
	{42, "elevation", "1", 0xfb67a59c5163dc3f, 0x4cc65a997c13313a, [3]uint64{0x45b8b404a9dcfab5, 0xd9736b9ea5bedc25, 0xb280da1445526a54}},
	{42, "warp", "1", 0x0e2091d7145a22ed, 0x91208ac0d1438147, [3]uint64{0x8546a965411148e5, 0x162c41e79ac86705, 0xac7ad8a15df1902f}},
	{42, "ridges", "1", 0xce4354028c747906, 0x4409c3fb22622ec0, [3]uint64{0xd559a8e74cb0e02e, 0x5b7bd299289db7ab, 0x74c403a24962bac2}},
	{42, "volcanic", "1", 0x4c1c57837f1ef339, 0xe7b770dda55e4d88, [3]uint64{0x47876ea8b8bbf3b2, 0x0bac86fa69f42886, 0x59a05ecb0e0a1376}},
	{42, "mesh", "1", 0x0610bf19e55f18e3, 0x0c926d9d0d5e7894, [3]uint64{0x472377da8ea441c1, 0xa566768d9f94ed26, 0xdee8f542483f473a}},
	{42, "climate", "1", 0x5446b5aebbe93272, 0xe11baa9c552b25e5, [3]uint64{0x966106d85da60445, 0x3dae6d995417849c, 0x3a2094d56887358c}},
	{42, "edge-noise", "1", 0x46c193dbff2fb376, 0x487c20af2701831c, [3]uint64{0x108ab717018ede84, 0xe844313a741e0709, 0x05b005695d782142}},
	{0x0123456789abcdef, "elevation", "2", 0xdd40476cb27d720f, 0x56c5899599c366c7, [3]uint64{0x928d6876579da2c7, 0x8e220d4dc88b1eb0, 0xbf4144077785b632}},
}

func TestDeriveGolden(t *testing.T) {
	for _, g := range golden {
		s1, s2 := Derive(g.world, g.stage, g.version)
		if s1 != g.seed1 || s2 != g.seed2 {
			t.Errorf("Derive(%#x, %q, %q) = (%#016x, %#016x), want (%#016x, %#016x)",
				g.world, g.stage, g.version, s1, s2, g.seed1, g.seed2)
		}
	}
}

func TestPCGGolden(t *testing.T) {
	for _, g := range golden {
		p := PCG(g.world, g.stage, g.version)
		for i, want := range g.draws {
			if got := p.Uint64(); got != want {
				t.Errorf("PCG(%#x, %q, %q) draw %d = %#016x, want %#016x",
					g.world, g.stage, g.version, i, got, want)
			}
		}
	}
}

func TestRandUsesPCG(t *testing.T) {
	for _, g := range golden {
		r := Rand(g.world, g.stage, g.version)
		for i, want := range g.draws {
			if got := r.Uint64(); got != want {
				t.Errorf("Rand(%#x, %q, %q) draw %d = %#016x, want %#016x",
					g.world, g.stage, g.version, i, got, want)
			}
		}
	}
}

// TestIndependentRecompute rebuilds one vector from the documented byte
// layout, without calling the package's encoder. The expected message and
// digest were also checked with Python's hashlib.
func TestIndependentRecompute(t *testing.T) {
	var msg []byte
	msg = append(msg, 0, 0, 0, 0, 0, 0, 0, 17) // len("mpg/stage-seed/v1")
	msg = append(msg, "mpg/stage-seed/v1"...)
	msg = append(msg, 0, 0, 0, 0, 0, 0, 0, 42) // world seed 42
	msg = append(msg, 0, 0, 0, 0, 0, 0, 0, 9)  // len("elevation")
	msg = append(msg, "elevation"...)
	msg = append(msg, 0, 0, 0, 0, 0, 0, 0, 1) // len("1")
	msg = append(msg, "1"...)

	const wantMsg = "0000000000000011" + "6d70672f73746167652d736565642f7631" +
		"000000000000002a" +
		"0000000000000009" + "656c65766174696f6e" +
		"0000000000000001" + "31"
	if got := hex.EncodeToString(msg); got != wantMsg {
		t.Fatalf("message = %s, want %s", got, wantMsg)
	}

	sum := sha256.Sum256(msg)
	const wantSum = "fb67a59c5163dc3f4cc65a997c13313a63428526215fcbf9f1c45db02449951e"
	if got := hex.EncodeToString(sum[:]); got != wantSum {
		t.Fatalf("digest = %s, want %s", got, wantSum)
	}

	want1 := binary.BigEndian.Uint64(sum[0:8])
	want2 := binary.BigEndian.Uint64(sum[8:16])
	s1, s2 := Derive(42, "elevation", "1")
	if s1 != want1 || s2 != want2 {
		t.Errorf("Derive(42, elevation, 1) = (%#016x, %#016x), want (%#016x, %#016x)", s1, s2, want1, want2)
	}
}

func TestDistinct(t *testing.T) {
	type in struct {
		world          uint64
		stage, version string
	}
	base := in{42, "elevation", "1"}
	variants := []in{
		base,
		{43, base.stage, base.version},          // world
		{1 << 56, base.stage, base.version},     // world, high byte
		{base.world, "elevatioN", base.version}, // stage
		{base.world, "warp", base.version},      // stage
		{base.world, base.stage, "2"},           // version
		{base.world, base.stage, "1 "},          // version, trailing byte
		// Length prefixing: the same concatenated bytes split differently
		// between stage and version must differ.
		{base.world, "ab", "c"},
		{base.world, "a", "bc"},
		{base.world, "elevation1", "x"},
		{base.world, "elevation", "1x"},
		// Stage and version swapped.
		{base.world, "1", "elevation"},
	}
	seen := make(map[[2]uint64]in)
	for _, v := range variants {
		s1, s2 := Derive(v.world, v.stage, v.version)
		key := [2]uint64{s1, s2}
		if prev, ok := seen[key]; ok {
			t.Errorf("Derive collision: %+v and %+v both give (%#016x, %#016x)", prev, v, s1, s2)
		}
		seen[key] = v
	}

	// Every design stage name gives a distinct seed for the same world.
	stages := []string{"layout", "elevation", "warp", "ridges", "volcanic", "mesh", "climate", "edge-noise"}
	byStage := make(map[[2]uint64]string)
	for _, s := range stages {
		s1, s2 := Derive(0, s, "1")
		if prev, ok := byStage[[2]uint64{s1, s2}]; ok {
			t.Errorf("stages %q and %q share a seed", prev, s)
		}
		byStage[[2]uint64{s1, s2}] = s
	}
}

func TestIndependentSources(t *testing.T) {
	// Two sources for the same stage are separate values that replay the
	// same stream; drawing from one does not advance the other.
	a := PCG(7, "mesh", "1")
	b := PCG(7, "mesh", "1")
	if a == b {
		t.Fatal("PCG returned a shared source")
	}
	first := a.Uint64()
	a.Uint64()
	if got := b.Uint64(); got != first {
		t.Errorf("second source first draw = %#016x, want %#016x", got, first)
	}
}

func TestEmptyPanics(t *testing.T) {
	for _, tc := range []struct{ stage, version string }{
		{"", "1"},
		{"layout", ""},
		{"", ""},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Derive(0, %q, %q) did not panic", tc.stage, tc.version)
				}
			}()
			Derive(0, tc.stage, tc.version)
		}()
	}
}
