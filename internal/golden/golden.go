// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package golden is the golden-hash template for determinism tests: a test
// feeds a generator's output into a Hasher in a defined order and pins the
// resulting SHA-256 with Check.
//
// A pinned hash is the dynamic half of the cross-architecture contract
// (DESIGN.md, "Determinism"; the static half is each package's
// TestNoFusedMultiplyAdd). It proves something only when it was recorded on
// more than one architecture: a product that fuses into a multiply-add, or a
// math function with per-architecture assembly, changes the bits on one
// machine and not on another, with nothing in the source to see. So:
//
// # Recording a hash
//
// Record a new or changed hash only when both architectures agree on it. The
// hashes print with
//
//	MPG_PRINT_GOLDEN=1 go test -count=1 -v -run Golden ./...
//	GOARCH=amd64 MPG_PRINT_GOLDEN=1 go test -count=1 -v -run Golden ./...
//
// On Apple silicon the second line runs the amd64 binary under Rosetta, which
// has no FMA; the CI workflow also prints them on linux/arm64, linux/amd64,
// and linux/amd64 with GOAMD64=v3, the build that fuses on amd64.
//
//  1. Leave the recorded value as it is (or "" for a new case) and print the
//     hashes on each architecture.
//
//  2. Compare the lines "name: hash (arch)" across architectures. If they
//     differ, the change broke determinism: fix the arithmetic (package
//     fmath), do not record either value.
//
//  3. If they agree, and the change in output was intended, record the shared
//     value and say why it moved in the commit message.
//
// Printing never relaxes the check: a mismatch fails in either mode, and the
// failure message carries the computed hash and architecture.
//
// By convention the tests that use this package are named TestGolden..., so
// the -run filter above collects every hash in the module.
package golden

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"math"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
)

// PrintEnv is the environment variable that, set to "1", makes Check log
// every hash it computes with the architecture that computed it.
const PrintEnv = "MPG_PRINT_GOLDEN"

// Hasher is a SHA-256 over values written in a defined order. Integers and
// float64 bit patterns are written little-endian; byte strings are
// length-prefixed, so consecutive sections cannot run into each other.
type Hasher struct {
	h   hash.Hash
	buf []byte
}

// New returns an empty Hasher.
func New() *Hasher { return &Hasher{h: sha256.New()} }

// Write writes p as is, so a Hasher is an io.Writer. It never fails.
func (h *Hasher) Write(p []byte) (int, error) { return h.h.Write(p) }

// Bytes writes len(b) as a Uint64, then b.
func (h *Hasher) Bytes(b []byte) {
	h.Uint64(uint64(len(b)))
	h.h.Write(b)
}

// String writes s as Bytes does.
func (h *Hasher) String(s string) {
	h.Uint64(uint64(len(s)))
	h.h.Write([]byte(s))
}

// Uint64 writes v as eight little-endian bytes.
func (h *Hasher) Uint64(v uint64) {
	h.buf = binary.LittleEndian.AppendUint64(h.buf[:0], v)
	h.h.Write(h.buf)
}

// Int writes v as a Uint64 of its two's-complement bits.
func (h *Hasher) Int(v int) { h.Uint64(uint64(int64(v))) }

// Float64s writes the IEEE 754 bits of each value (math.Float64bits, so
// -0 and +0 differ) as eight little-endian bytes, in order. It writes no
// length; write one first when the count is not fixed by what precedes it.
func (h *Hasher) Float64s(vs ...float64) {
	const chunk = 4096
	for len(vs) > 0 {
		n := min(len(vs), chunk)
		h.buf = h.buf[:0]
		for _, v := range vs[:n] {
			h.buf = binary.LittleEndian.AppendUint64(h.buf, math.Float64bits(v))
		}
		h.h.Write(h.buf)
		vs = vs[n:]
	}
}

// Sum returns the lowercase hex SHA-256 of everything written so far.
func (h *Hasher) Sum() string { return hex.EncodeToString(h.h.Sum(nil)) }

// Sum returns the lowercase hex SHA-256 of b.
func Sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// Printing reports whether PrintEnv is set to "1".
func Printing() bool { return os.Getenv(PrintEnv) == "1" }

// Arch names the architecture this binary was built for, with its
// microarchitecture level when the build records one (for example
// "amd64 GOAMD64=v3" or "arm64 GOARM64=v8.0").
func Arch() string {
	arch := runtime.GOARCH
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return arch
	}
	for _, s := range bi.Settings {
		if s.Key != "GOARCH" && strings.HasPrefix(s.Key, "GO"+strings.ToUpper(arch)) {
			return arch + " " + s.Key + "=" + s.Value
		}
	}
	return arch
}

// Check compares the computed hash got with the recorded hash want for the
// case name. In print mode (see PrintEnv) it first logs "name: got (arch)".
// It fails the test when want is empty (nothing recorded yet) or differs from
// got; recording a value follows the procedure in the package documentation.
func Check(t testing.TB, name, got, want string) {
	t.Helper()
	if Printing() {
		t.Logf("%s: %s (%s)", name, got, Arch())
	}
	switch {
	case want == "":
		t.Errorf("golden %s: no hash recorded; computed %s (%s). Record it only once arm64 and amd64 agree on it (see package golden)",
			name, got, Arch())
	case got != want:
		t.Errorf("golden %s: hash %s (%s), recorded %s. If the output changed on purpose, print the hashes with %s=1 on arm64 and amd64 and record the value only if they agree (see package golden)",
			name, got, Arch(), want, PrintEnv)
	}
}
