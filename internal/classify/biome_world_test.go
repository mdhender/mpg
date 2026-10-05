// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package classify_test

import (
	"fmt"
	"testing"

	"github.com/mdhender/mpg/internal/basin"
	"github.com/mdhender/mpg/internal/classify"
	"github.com/mdhender/mpg/internal/climate"
	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/pipeline"
)

// coverWorld runs the pipeline through classification on a default-size
// world.
func coverWorld(t *testing.T, seed uint64, aspect, preset string) *pipeline.Context {
	t.Helper()
	c := config.Default()
	c.Seed = config.Seed(seed)
	c.World.Aspect = aspect
	c.Layout.Preset = preset
	ctx, err := pipeline.NewContext(c, t.TempDir(), "", nil)
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
	return ctx
}

// TestDoneWhen checks S32's done-when items on default-size worlds with
// cold high ground and dry interiors:
//
//   - cold deserts without glaciers: polar desert (cold land too dry for
//     ice: warmest month below 0 °C, snowfall below the minimum) stays
//     bare, and Köppen cold deserts (desert below 18 °C) carry no ice;
//   - alpine ice: a glacier on a mountain cell;
//   - humid basins that aren't desert: land in closed basins with a
//     humid aridity index (at least 0.65) is never desert.
//
// It also checks the cover's invariants on the real worlds: biome on
// exactly the playable land, clear exactly under ice, ice only on cold
// snowy land, pack ice only on cold playable water, wetlands only on wet
// flats or plains or playas.
func TestDoneWhen(t *testing.T) {
	if testing.Short() {
		t.Skip("builds default-size worlds")
	}
	br := classify.DefaultBiomeRules()
	var polar, coldDesert, alpineIce, humidBasin int
	for _, tc := range []struct {
		seed           uint64
		aspect, preset string
	}{
		{4, "portrait", "archipelago"},
		{5, "portrait", "pangaea"},
	} {
		t.Run(fmt.Sprintf("seed%d-%s-%s", tc.seed, tc.aspect, tc.preset), func(t *testing.T) {
			ctx := coverWorld(t, tc.seed, tc.aspect, tc.preset)
			p := ctx.Products
			m, r, tg, cl := p.Mesh, p.Classes, p.Target, p.Target.Climate
			playa := map[int]bool{}
			for _, pl := range tg.Lakes.Playas {
				playa[pl.Cell] = true
			}
			var w struct{ polar, coldDesert, alpineIce, humidBasin, humidDesert int }
			for i, c := range m.Cells {
				lat := climate.Degrees(p.Cells.Latitude[i])
				warm, _ := br.Season(lat, cl.Temperature[i])
				land := tg.Land[i] && !c.Rim
				b, s := r.Biome[i], r.Surface[i]
				switch {
				case land && b == classify.BiomeNone, !land && b != classify.BiomeNone:
					t.Fatalf("cell %d: land %v, biome %s", i, land, b)
				case (b == classify.Clear) != s.IsIce():
					t.Fatalf("cell %d: biome %s, surface %s", i, b, s)
				case s.IsIce() && !(warm < br.IceWarmestBelowC && br.Snowfall(lat, cl.Temperature[i], cl.Precipitation[i]) >= br.SnowMinMM):
					t.Fatalf("cell %d: %s at warmest %v °C", i, s, warm)
				case s == classify.PackIce && (land || c.Rim || warm >= 0):
					t.Fatalf("cell %d: pack ice, land %v, rim %v, warmest %v", i, land, c.Rim, warm)
				case c.Rim && s != classify.SurfaceNone:
					t.Fatalf("rim cell %d: %s", i, s)
				case s.IsWetland() && !playa[i] && (r.Wetness[i] == 0 || (r.Landform[i] != classify.Flats && r.Landform[i] != classify.Plains)):
					t.Fatalf("cell %d: %s on %s with wetness %03b", i, s, r.Landform[i], r.Wetness[i])
				case playa[i] && s != classify.SaltFlats:
					t.Fatalf("playa cell %d: %s", i, s)
				}
				if b == classify.PolarDesert {
					w.polar++
					if s != classify.SurfaceNone || warm >= 0 {
						t.Errorf("polar desert cell %d: %s, warmest %v", i, s, warm)
					}
				}
				if b == classify.Desert && cl.Temperature[i] < br.HotMinC {
					w.coldDesert++
				}
				if s == classify.Glacier && r.Landform[i] == classify.Mountains {
					w.alpineIce++
				}
				if land && tg.Basins.Of[i] != basin.None && cl.Aridity[i] >= 0.65 && closed(tg.Basins, tg.Lakes, tg.Basins.Of[i]) {
					w.humidBasin++
					if b == classify.Desert {
						w.humidDesert++
					}
				}
			}
			t.Logf("polar desert %d, cold (BWk) desert %d, glaciers on mountains %d, humid closed-basin land %d (%d desert); %s",
				w.polar, w.coldDesert, w.alpineIce, w.humidBasin, w.humidDesert, r.BiomeShares(true))
			if w.humidDesert != 0 {
				t.Errorf("%d humid closed-basin cells are desert", w.humidDesert)
			}
			polar += w.polar
			coldDesert += w.coldDesert
			alpineIce += w.alpineIce
			humidBasin += w.humidBasin
		})
	}
	if polar == 0 || coldDesert == 0 || alpineIce == 0 || humidBasin == 0 {
		t.Errorf("polar desert %d, cold desert %d, alpine ice %d, humid closed-basin land %d: want each somewhere",
			polar, coldDesert, alpineIce, humidBasin)
	}
}

// closed reports whether basin b's outermost basin does not overflow (is
// not full): its land is in a closed basin.
func closed(bs *basin.Result, lk *basin.Lakes, b int) bool {
	for bs.Basins[b].Parent != basin.None {
		b = bs.Basins[b].Parent
	}
	return lk.Water[b].State != basin.Full
}
