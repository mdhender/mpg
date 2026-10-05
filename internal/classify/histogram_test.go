// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package classify_test

import (
	"testing"

	"github.com/mdhender/mpg/internal/classify"
	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/pipeline"
)

// TestLandformHistogramPlausible runs default worlds through classification
// and holds the land landform shares to the loose bounds of the first
// retune (S21): over seeds 1–8 at cinematic and square and the archipelago
// and pangaea presets, flats, plains and rolling plains together were 53%
// to 76% of the land, hills 13% to 28%, mountains 6% to 16%, plateaus 1.5%
// to 6.6%, and flats at least 3.5%. The bounds leave room around those, so
// they catch a broken rule or a lost retune, not a seed's character.
func TestLandformHistogramPlausible(t *testing.T) {
	if testing.Short() {
		t.Skip("runs default worlds")
	}
	for _, tc := range []struct {
		name   string
		seed   config.Seed
		aspect string
		preset string
	}{
		{"seed42-cinematic", 42, "cinematic", "continents"},
		{"seed3-cinematic-archipelago", 3, "cinematic", "archipelago"}, // the most hills and mountains
		{"seed7-cinematic-pangaea", 7, "cinematic", "pangaea"},         // the most flats
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := config.Default()
			cfg.Seed = tc.seed
			cfg.World.Aspect = tc.aspect
			cfg.Layout.Preset = tc.preset
			ctx, err := pipeline.NewContext(cfg, t.TempDir(), "", nil)
			if err != nil {
				t.Fatal(err)
			}
			stages := pipeline.Stages()
			last, err := pipeline.Lookup(stages, "classify")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pipeline.Run(ctx, stages, last); err != nil {
				t.Fatal(err)
			}
			counts, total := ctx.Products.Classes.LandHistogram()
			if total != ctx.Products.SeaLevel.LandCells {
				t.Fatalf("%d land landforms for %d land cells", total, ctx.Products.SeaLevel.LandCells)
			}
			pct := func(k int) float64 { return 100 * float64(counts[k]) / float64(total) }
			flats, plains, rolling, hills, mountains, plateaus := pct(0), pct(1), pct(2), pct(3), pct(4), pct(5)
			for _, b := range []struct {
				name   string
				v      float64
				lo, hi float64
			}{
				{"flats + plains + rolling plains", flats + plains + rolling, 50, 85},
				{"flats", flats, 2, 50},
				{"plains", plains, 10, 45},
				{"rolling plains", rolling, 5, 50},
				{"hills", hills, 8, 35},
				{"mountains", mountains, 3, 20},
				{"plateaus", plateaus, 0.5, 12},
			} {
				if b.v < b.lo || b.v > b.hi {
					t.Errorf("%s %.1f%% of the land, want %v%% to %v%%", b.name, b.v, b.lo, b.hi)
				}
			}
			t.Logf("%v", ctx.Products.Classes.LandShares(false))
			for _, l := range ctx.Products.Classes.Landform {
				if l == classify.LandformNone {
					t.Fatal("a cell has no landform")
				}
			}
		})
	}
}
