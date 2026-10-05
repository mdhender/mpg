// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package climate

import (
	"math"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"github.com/mdhender/mpg/internal/config"
)

func defaultModel(t *testing.T) Model {
	t.Helper()
	m, err := NewModel(config.DefaultClimate())
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// TestSeaLevelCurve checks the pole-to-equator gradient at sea level: the
// curve is hmz2bio's at its points, symmetric north and south, never warmer
// poleward over a fine sweep of the latitude proxy, warmest at the equator
// and coldest at the poles, and finite everywhere.
func TestSeaLevelCurve(t *testing.T) {
	m := defaultModel(t)
	for _, p := range config.DefaultClimate().SeaLevelTempC {
		lat := p.LatDeg / 90
		if got := m.SeaLevel(lat); math.Abs(got-p.TempC) > 1e-9 {
			t.Errorf("SeaLevel(%v°) = %v, want %v", p.LatDeg, got, p.TempC)
		}
	}
	if got := m.SeaLevel(0); got != 27 {
		t.Errorf("equator %v °C, want 27", got)
	}
	if n, s := m.SeaLevel(1), m.SeaLevel(-1); n != -22 || s != -22 {
		t.Errorf("poles %v and %v °C, want -22", n, s)
	}
	if got := m.SeaLevel(1.5); got != -22 {
		t.Errorf("beyond the pole %v °C, want -22", got)
	}
	const steps = 100_000
	prev := m.SeaLevel(0)
	for k := 1; k <= steps; k++ {
		lat := float64(k) / steps
		v := m.SeaLevel(lat)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("SeaLevel(%v) = %v", lat, v)
		}
		if v > prev {
			t.Fatalf("SeaLevel(%v) = %v is warmer than SeaLevel(%v) = %v", lat, v, float64(k-1)/steps, prev)
		}
		if w := m.SeaLevel(-lat); w != v {
			t.Fatalf("SeaLevel(%v) = %v but SeaLevel(%v) = %v", lat, v, -lat, w)
		}
		if v > 27 || v < -22 {
			t.Fatalf("SeaLevel(%v) = %v outside [-22, 27]", lat, v)
		}
		prev = v
	}
	// Strictly colder from 10° on: the gradient is everywhere but the
	// tropical plateau.
	for _, d := range []float64{10, 12, 20, 33, 47, 61, 75, 89} {
		if !(m.SeaLevel((d+1)/90) < m.SeaLevel(d/90)) {
			t.Errorf("no gradient between %v° and %v°", d, d+1)
		}
	}
}

// TestAltitudeCooling checks the lapse rate: a cell h meters above sea
// level is 6.5·h/1000 °C colder than sea level at the same latitude, and
// heights at or below sea level cool nothing.
func TestAltitudeCooling(t *testing.T) {
	m := defaultModel(t)
	if m.LapseRate() != 6.5 {
		t.Fatalf("lapse rate %v", m.LapseRate())
	}
	for _, lat := range []float64{0, 0.3, -0.5, 0.8, 1} {
		sea := m.SeaLevel(lat)
		if got := m.At(lat, 0); got != sea {
			t.Errorf("At(%v, 0) = %v, want %v", lat, got, sea)
		}
		if got := m.At(lat, -2500); got != sea {
			t.Errorf("At(%v, -2500) = %v, want sea level %v", lat, got, sea)
		}
		prev := sea
		for _, h := range []float64{1, 100, 1000, 2000, 4500} {
			got := m.At(lat, h)
			if want := sea - 6.5*h/1000; math.Abs(got-want) > 1e-9 {
				t.Errorf("At(%v, %v) = %v, want %v", lat, h, got, want)
			}
			if !(got < prev) {
				t.Errorf("At(%v, %v) = %v is not colder than %v", lat, h, got, prev)
			}
			prev = got
		}
	}
	if got := m.At(0, 1000); got != 20.5 {
		t.Errorf("equator at 1000 m: %v °C, want 20.5", got)
	}
}

func TestNewModelErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*config.Climate)
		want string
	}{
		{"one point", func(c *config.Climate) { c.SeaLevelTempC = c.SeaLevelTempC[:1] }, "at least 2"},
		{"no pole", func(c *config.Climate) { c.SeaLevelTempC = c.SeaLevelTempC[:3] }, "0° to 90°"},
		{"warmer poleward", func(c *config.Climate) { c.SeaLevelTempC[3].TempC = 40 }, "point 3"},
		{"NaN", func(c *config.Climate) { c.SeaLevelTempC[2].TempC = math.NaN() }, "not finite"},
		{"lapse", func(c *config.Climate) { c.LapseRateCPerKm = -1 }, "lapse rate"},
		{"lapse inf", func(c *config.Climate) { c.LapseRateCPerKm = math.Inf(1) }, "lapse rate"},
	} {
		c := config.DefaultClimate()
		tc.edit(&c)
		if _, err := NewModel(c); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: NewModel = %v, want an error containing %q", tc.name, err, tc.want)
		}
	}
	if _, err := Compute(nil, nil, nil, Model{}); err == nil {
		t.Error("Compute with nil inputs succeeded")
	}
}

// TestNoFusedMultiplyAdd compiles this package for the architectures whose Go
// compilers fuse a*b+c into one instruction and fails if any fused
// multiply-add or multiply-subtract appears in its code.
func TestNoFusedMultiplyAdd(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles for several architectures")
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool not found")
	}
	fusedOp := regexp.MustCompile(`\tV?FN?M(ADD|SUB)`)
	const pkg = "github.com/mdhender/mpg/internal/climate"
	for _, tg := range []struct{ arch, env string }{{"arm64", ""}, {"amd64", "GOAMD64=v3"}} {
		t.Run(tg.arch, func(t *testing.T) {
			cmd := exec.Command(goTool, "build", "-gcflags="+pkg+"=-S", pkg)
			cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+tg.arch, "CGO_ENABLED=0")
			if tg.env != "" {
				cmd.Env = append(cmd.Env, tg.env)
			}
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("go build %s: %v\n%s", pkg, err, out)
			}
			if m := fusedOp.FindAllString(string(out), -1); len(m) != 0 {
				t.Errorf("%s: %d fused multiply-add instructions in %s", tg.arch, len(m), pkg)
			}
		})
	}
}
