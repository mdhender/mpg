// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package pipeline

import (
	"github.com/mdhender/mpg/internal/elevation"
)

// runElevation builds the bedrock elevation from the layout's bias, leaves
// it and the volcanic hotspots in Products, and renders it split at 0 m with
// the hotspots marked. The datum shift puts the land fraction of the raster
// above 0 m, so the render shows about the coasts stage 6's sea-level search
// will find; the log reports the raster's own estimate of that level.
func runElevation(c *Context) error {
	e, err := elevation.New(c.Config, c.Products.Bias)
	if err != nil {
		return err
	}
	f, st := e.Field()
	hs := e.Hotspots()
	c.Products.Elevation = f
	c.Products.Hotspots = hs
	lo, hi := f.MinMax()
	target := elevation.SeaLevelForShare(f, c.Config.World.LandFraction)
	clamped := ""
	if st.Clamped {
		clamped = " (clamped: the land fraction is out of reach)"
	}
	c.Logf("datum shift %+.3f%s; %.1f%% of the non-rim signal positive", st.Shift, clamped, 100*st.Share)
	c.Logf("range %.0f to %.0f m; %.1f%% of the non-rim samples above 0 m; %.1f%% land (target) at %.0f m; falloff and rim at most %.0f m",
		lo, hi, 100*elevation.LandShare(f, 0), 100*c.Config.World.LandFraction, target, elevation.BandMax(f))
	above := 0
	for _, h := range hs {
		if f.Sample(h.X, h.Y) > 0 {
			above++
		}
	}
	c.Logf("%d volcanic hotspots (mean %.2f); %d peaks above 0 m",
		len(hs), c.Config.Volcanic.HotspotsPerMkm2*c.Config.World.PlayableAreaKm2/1e6, above)
	return c.Render("", elevation.StageRender(f, 0, hs))
}
