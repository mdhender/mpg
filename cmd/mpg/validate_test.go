// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/world"
)

// validate runs "mpg validate args..." and returns the exit code, stdout,
// and stderr.
func validate(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(append([]string{"validate"}, args...), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// smallWorld generates a small world (300 land cells) with seed s into a
// new directory and returns it.
func smallWorld(t *testing.T, s uint64) string {
	t.Helper()
	cfg := config.Default()
	cfg.Seed = config.Seed(s)
	cfg.World.LandCells = 300
	cfg.Rim.FalloffCells = 4
	if err := cfg.Resolve(); err != nil {
		t.Fatal(err)
	}
	b, err := cfg.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "in.json")
	if err := os.WriteFile(in, b, 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	if code, _, stderr := generate(t, "--config", in, "--output", out); code != 0 {
		t.Fatalf("generate: exit %d; stderr %q", code, stderr)
	}
	return out
}

func TestValidateGenerated(t *testing.T) {
	out := smallWorld(t, 3)
	file := filepath.Join(out, world.File)
	for _, path := range []string{out, file} {
		code, stdout, stderr := validate(t, path)
		if code != 0 || !strings.Contains(stdout, "valid world schema 0") || !strings.Contains(stdout, "land ") {
			t.Errorf("validate %s: exit %d, stdout %q, stderr %q", path, code, stdout, stderr)
		}
	}
	code, stdout, stderr := validate(t, "--config", filepath.Join(out, "config.json"), out)
	if code != 0 || !strings.Contains(stdout, "matches config") {
		t.Errorf("validate --config: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}

	// Another seed's config does not match.
	other := smallWorld(t, 4)
	code, _, stderr = validate(t, "--config", filepath.Join(other, "config.json"), out)
	if code != 1 || !strings.Contains(stderr, "config hash") || !strings.Contains(stderr, "seed") {
		t.Errorf("validate against another config: exit %d, stderr %q", code, stderr)
	}
}

func TestValidateCorrupt(t *testing.T) {
	out := smallWorld(t, 3)
	good, err := os.ReadFile(filepath.Join(out, world.File))
	if err != nil {
		t.Fatal(err)
	}
	// mutate decodes the good file, applies f, and re-encodes it.
	mutate := func(f func(w *world.World)) []byte {
		w, err := world.DecodeBytes(good)
		if err != nil {
			t.Fatal(err)
		}
		f(w)
		b, err := w.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	for _, tc := range []struct {
		name string
		data []byte
		msg  string
	}{
		{"unknown field", bytes.Replace(good, []byte(`{"schema"`), []byte(`{"bogus":1,"schema"`), 1), "unknown field"},
		{"misspelled field", bytes.Replace(good, []byte(`"altitude_m"`), []byte(`"altitude"`), 1), "unknown field"},
		{"truncated", good[:len(good)/2], "world:"},
		{"trailing data", append(bytes.Clone(good), []byte("{}")...), "after the top-level object"},
		{"no schema", bytes.Replace(good, []byte(`{"schema":0,`), []byte(`{`), 1), "no schema"},
		{"future schema", bytes.Replace(good, []byte(`{"schema":0,`), []byte(`{"schema":1,`), 1), "schema 1"},
		{"bad point", bytes.Replace(good, []byte(`"site":[`), []byte(`"site":[1,`), 1), "two numbers"},
		{"incline", mutate(func(w *world.World) { w.Edges[len(w.Edges)/2].InclinePermille += 7 }), "incline"},
		{"landform", mutate(func(w *world.World) { w.Cells[0].Landform = "swamp" }), "not in the codebook"},
		{"land count", mutate(func(w *world.World) { w.Outcomes.LandCells++ }), "outcomes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), world.File)
			if err := os.WriteFile(path, tc.data, 0o644); err != nil {
				t.Fatal(err)
			}
			code, stdout, stderr := validate(t, path)
			if code != 1 || !strings.Contains(stderr, tc.msg) || stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr %q; want exit 1 mentioning %q", code, stdout, stderr, tc.msg)
			}
		})
	}
}

func TestValidateUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"a", "b"}, {"--no-such-flag", "a"}} {
		if code, _, stderr := validate(t, args...); code != 2 {
			t.Errorf("validate %q: exit %d, want 2; stderr %q", args, code, stderr)
		}
	}
	if code, _, stderr := validate(t, filepath.Join(t.TempDir(), "missing.json")); code != 1 || !strings.Contains(stderr, "missing.json") {
		t.Errorf("validate a missing file: exit %d, stderr %q", code, stderr)
	}
	if code, _, stderr := validate(t, t.TempDir()); code != 1 || !strings.Contains(stderr, world.File) {
		t.Errorf("validate a directory without world.json: exit %d, stderr %q", code, stderr)
	}
}
