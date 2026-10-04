// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package golden

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"runtime"
	"strings"
	"testing"
)

func TestHasherEncoding(t *testing.T) {
	h := New()
	h.Bytes([]byte("ab"))
	h.String("c")
	h.Uint64(0x0102030405060708)
	h.Int(-1)
	h.Float64s(1, math.Copysign(0, -1))
	_, _ = h.Write([]byte{0xff})

	want := []byte{
		2, 0, 0, 0, 0, 0, 0, 0, 'a', 'b',
		1, 0, 0, 0, 0, 0, 0, 0, 'c',
		8, 7, 6, 5, 4, 3, 2, 1,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0, 0, 0, 0, 0, 0, 0xf0, 0x3f, // 1.0
		0, 0, 0, 0, 0, 0, 0, 0x80, // -0
		0xff,
	}
	sum := sha256.Sum256(want)
	if got := h.Sum(); got != hex.EncodeToString(sum[:]) {
		t.Errorf("Sum = %s, want %x", got, sum)
	}
	if got := Sum(want); got != h.Sum() {
		t.Errorf("Sum(bytes) = %s, want %s", got, h.Sum())
	}
}

// TestFloat64sChunks checks that a long slice hashes as its values written
// one at a time, across the chunk boundary.
func TestFloat64sChunks(t *testing.T) {
	vs := make([]float64, 10_001)
	for i := range vs {
		vs[i] = float64(i) / 3
	}
	a, b := New(), New()
	a.Float64s(vs...)
	for _, v := range vs {
		b.Float64s(v)
	}
	if a.Sum() != b.Sum() {
		t.Errorf("chunked %s, one at a time %s", a.Sum(), b.Sum())
	}
}

func TestArch(t *testing.T) {
	if got := Arch(); !strings.HasPrefix(got, runtime.GOARCH) {
		t.Errorf("Arch() = %q, want it to start with %q", got, runtime.GOARCH)
	}
}

// recorder is a testing.TB that records what Check reports.
type recorder struct {
	testing.TB
	logs, errs []string
}

func (r *recorder) Helper() {}
func (r *recorder) Logf(format string, args ...any) {
	r.logs = append(r.logs, fmt.Sprintf(format, args...))
}
func (r *recorder) Errorf(format string, args ...any) {
	r.errs = append(r.errs, fmt.Sprintf(format, args...))
}

func TestCheck(t *testing.T) {
	for _, printing := range []bool{false, true} {
		if printing {
			t.Setenv(PrintEnv, "1")
		} else {
			t.Setenv(PrintEnv, "")
		}
		for _, tc := range []struct {
			got, want string
			fail      string
		}{
			{"abc", "abc", ""},
			{"abc", "def", "hash abc"},
			{"abc", "", "no hash recorded"},
		} {
			r := &recorder{TB: t}
			Check(r, "case", tc.got, tc.want)
			if tc.fail == "" && len(r.errs) != 0 {
				t.Errorf("print=%v Check(%q, %q) failed: %q", printing, tc.got, tc.want, r.errs)
			}
			if tc.fail != "" && (len(r.errs) != 1 || !strings.Contains(r.errs[0], tc.fail) || !strings.Contains(r.errs[0], Arch())) {
				t.Errorf("print=%v Check(%q, %q) errors %q, want one mentioning %q and the arch", printing, tc.got, tc.want, r.errs, tc.fail)
			}
			wantLog := "case: abc (" + Arch() + ")"
			if printing != (len(r.logs) == 1 && r.logs[0] == wantLog) || len(r.logs) > 1 {
				t.Errorf("print=%v logs %q, want %q only when printing", printing, r.logs, wantLog)
			}
		}
	}
}
