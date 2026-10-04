// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mdhender/mpg"
)

func TestVersion(t *testing.T) {
	want := mpg.Version().String() + "\n"
	for _, args := range [][]string{{"-version"}, {"version"}} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 0 {
			t.Errorf("run(%q) = %d, want 0; stderr %q", args, code, stderr.String())
		}
		if got := stdout.String(); got != want {
			t.Errorf("run(%q) stdout = %q, want %q", args, got, want)
		}
	}
}

func TestStubs(t *testing.T) {
	for _, name := range []string{"generate", "sweep", "render-stage", "validate"} {
		var stdout, stderr bytes.Buffer
		if code := run([]string{name}, &stdout, &stderr); code == 0 {
			t.Errorf("run(%q) = 0, want non-zero", name)
		}
		if !strings.Contains(stderr.String(), "not implemented") {
			t.Errorf("run(%q) stderr = %q, want a not-implemented message", name, stderr.String())
		}
	}
}

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{nil, {"bogus"}, {"-no-such-flag"}} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 2 {
			t.Errorf("run(%q) = %d, want 2", args, code)
		}
		if !strings.Contains(stderr.String(), "usage: mpg") {
			t.Errorf("run(%q) stderr = %q, want usage", args, stderr.String())
		}
	}
}
