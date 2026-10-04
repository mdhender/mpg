// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/pipeline"
	"github.com/mdhender/mpg/internal/render"
)

// sweep runs "mpg sweep args..." and returns the exit code, stdout, and
// stderr.
func sweep(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(append([]string{"sweep"}, args...), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestParseSeeds(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []uint64
	}{
		{"7", []uint64{7}},
		{"1-4", []uint64{1, 2, 3, 4}},
		{"1,5,9-12", []uint64{1, 5, 9, 10, 11, 12}},
		{" 3 , 2 ", []uint64{3, 2}},
		{"5-5", []uint64{5}},
		{"0-255", nil}, // 256 seeds: checked by length below
		{"18446744073709551614-18446744073709551615", []uint64{18446744073709551614, 18446744073709551615}},
	} {
		got, err := parseSeeds(tc.in)
		if err != nil {
			t.Errorf("parseSeeds(%q): %v", tc.in, err)
			continue
		}
		if tc.want == nil {
			if len(got) != maxSweepSeeds {
				t.Errorf("parseSeeds(%q) gave %d seeds", tc.in, len(got))
			}
			continue
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("parseSeeds(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
	for _, in := range []string{
		"", ",", "1,", "1,,2", "-1", "1-", "3-1", "x", "0x10", "+1", "1.5",
		"18446744073709551616", "1-2-3", "0-256", "0-18446744073709551615",
		"1-200,201-300", "1,1", "1-5,3",
	} {
		if got, err := parseSeeds(in); err == nil {
			t.Errorf("parseSeeds(%q) = %v, want an error", in, got)
		}
	}
}

func TestParseSweepStages(t *testing.T) {
	registry := pipeline.Stages()
	cols, err := parseSweepStages("noise,config,3,mesh:area", registry)
	if err != nil {
		t.Fatal(err)
	}
	want := []sweepStage{{"noise", -1, ""}, {"config", 0, ""}, {"elevation", 2, ""}, {"mesh:area", 3, "area"}}
	if !slices.Equal(cols, want) {
		t.Errorf("cols = %+v, want %+v", cols, want)
	}
	for _, in := range []string{"bogus", "noise,", "noise,noise", "elevation,3", "mesh:", "mesh:Area", "noise:x", "0"} {
		if _, err := parseSweepStages(in, registry); err == nil {
			t.Errorf("parseSweepStages(%q) succeeded", in)
		}
	}
	if _, err := parseSweepStages("bogus", registry); !strings.Contains(err.Error(), "noise") {
		t.Errorf("unknown-stage error %q does not offer noise", err)
	}

	// Only the noise preview needs no pipeline; config is implemented;
	// elevation is not, nor is layout before it.
	for _, tc := range []struct {
		in   string
		last int
		ok   bool
	}{{"noise", -1, true}, {"noise,config", 0, true}, {"config,elevation", -1, false}, {"layout", -1, false}} {
		cols, err := parseSweepStages(tc.in, registry)
		if err != nil {
			t.Fatal(err)
		}
		last, err := checkImplemented(cols, registry)
		if (err == nil) != tc.ok || last != tc.last {
			t.Errorf("checkImplemented(%q) = %d, %v", tc.in, last, err)
		}
	}
}

func TestSweepErrors(t *testing.T) {
	out := filepath.Join(t.TempDir(), "sheet.png")
	for _, tc := range []struct {
		args []string
		code int
		msg  string
	}{
		{[]string{"--stage", "noise", "--output", out}, 2, "--seeds is required"},
		{[]string{"--seeds", "1", "--output", out}, 2, "--stage is required"},
		{[]string{"--seeds", "1", "--stage", "noise"}, 2, "--output is required"},
		{[]string{"--seeds", "2-1", "--stage", "noise", "--output", out}, 2, "reversed"},
		{[]string{"--seeds", "1", "--stage", "bogus", "--output", out}, 2, "sea-level"},
		{[]string{"--seeds", "1", "--stage", "noise", "--tile", "8", "--output", out}, 2, "--tile"},
		{[]string{"--seeds", "1", "--stage", "noise", "--aspect", "square,", "--output", out}, 2, "--aspect"},
		{[]string{"--seeds", "1", "--stage", "noise", "--output", out, "extra"}, 2, "unexpected"},
		{[]string{"--seeds", "1", "--stage", "noise,elevation", "--output", out}, 1, "stage 2 layout is not implemented"},
		{[]string{"--seeds", "1", "--stage", "noise", "--aspect", "squarish", "--output", out}, 1, "world.aspect"},
		{[]string{"--seeds", "1", "--stage", "noise", "--config", "no-such-file.json", "--output", out}, 1, "no-such-file"},
	} {
		code, _, stderr := sweep(t, tc.args...)
		if code != tc.code || !strings.Contains(stderr, tc.msg) {
			t.Errorf("sweep %q: exit %d, stderr %q; want exit %d mentioning %q", tc.args, code, stderr, tc.code, tc.msg)
		}
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("sheet written on an error: %v", err)
	}
}

// sweepPixelHash pins the small sweep below: seeds 1 and 7, the noise
// preview and the (render-less) config stage, cinematic and square, 600
// land cells, 64-pixel tiles.
const sweepPixelHash = "a6118fc4f5177c9d9be1c40157b25ac2aa7b68d387ee9eb419e0974d03ed0c8e"

func TestSweepSheet(t *testing.T) {
	dir := t.TempDir()
	args := []string{"--seeds", "1,7", "--stage", "noise,config", "--aspect", "cinematic,square",
		"--land-cells", "600", "--tile", "64"}
	var hashes []string
	for n := range 2 {
		out := filepath.Join(dir, "sub", strings.Repeat("x", n+1)+".png")
		code, stdout, stderr := sweep(t, append(args, "--output", out)...)
		if code != 0 {
			t.Fatalf("exit %d; stderr %q", code, stderr)
		}
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		hash := render.PixelHash(img)
		hashes = append(hashes, hash)
		if !strings.Contains(stdout, "pixels  "+hash) {
			t.Errorf("stdout %q does not report the pixel hash", stdout)
		}

		// Layout: label column, two 64-pixel tile columns; four rows whose
		// tile heights follow each world's shape.
		cfg := config.Default()
		cfg.World.LandCells = 600
		var tileHs []int
		for _, aspect := range []string{"cinematic", "square"} {
			c := cfg
			c.World.Aspect = aspect
			if err := c.Resolve(); err != nil {
				t.Fatal(err)
			}
			h := int(float64(64)*c.World.HeightKm/c.World.WidthKm + 0.5)
			tileHs = append(tileHs, h, h)
		}
		labelW := len("cinematic") * render.LabelAdvance
		wantW := 8 + labelW + 8 + 2*(64+8)
		wantH := 8 + render.LabelHeight + 4
		for _, h := range tileHs {
			wantH += h + 2*render.LabelHeight + 2 + 8
		}
		if img.Bounds() != image.Rect(0, 0, wantW, wantH) {
			t.Errorf("sheet is %v, want %d x %d (tile heights %v)", img.Bounds(), wantW, wantH, tileHs)
		}

		meta, err := render.ReadMeta(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		base := config.Default()
		base.World.LandCells = 600
		if err := base.Resolve(); err != nil {
			t.Fatal(err)
		}
		baseHash, _ := base.Hash()
		if meta.Stage != "sweep" || meta.ConfigHash != baseHash {
			t.Errorf("meta = %+v, want stage sweep and base hash %s", meta, baseHash)
		}
		extra := map[string]string{}
		for _, e := range meta.Extra {
			extra[e.Key] = e.Value
		}
		for k, v := range map[string]string{
			"mpg:seeds": "1,7", "mpg:stages": "noise,config", "mpg:aspects": "cinematic,square", "mpg:tile-width": "64",
		} {
			if extra[k] != v {
				t.Errorf("%s = %q, want %q", k, extra[k], v)
			}
		}
		if rows := strings.Split(extra["mpg:rows"], "\n"); len(rows) != 4 || !strings.HasPrefix(rows[0], "1 cinematic ") || !strings.HasPrefix(rows[3], "7 square ") {
			t.Errorf("mpg:rows = %q", extra["mpg:rows"])
		}
	}
	if hashes[0] != hashes[1] {
		t.Errorf("two runs gave pixel hashes %s and %s", hashes[0], hashes[1])
	}
	if hashes[0] != sweepPixelHash {
		t.Errorf("pixel hash = %s, want %s", hashes[0], sweepPixelHash)
	}
}
