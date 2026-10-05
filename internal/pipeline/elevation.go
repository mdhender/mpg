// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package pipeline

import (
	"fmt"
	"io"

	"github.com/mdhender/mpg/internal/elevation"
)

// PrePass is the elevation stage's pre-pass (DESIGN.md, "Climate
// coupling"): stages 3 to 8 run once with no lake allowance, to count the
// lake cells the datum then allows for.
type PrePass struct {
	// SeaLevelM is the pre-pass's sea level (stage 6, for N land cells
	// before lakes), and LakeCells the lake and inland-sea cells its basins
	// stage found there.
	SeaLevelM float64
	LakeCells int
	// LandShare is the share of the samples outside the rim the final
	// datum puts above 0 m: world.land_fraction · (N + LakeCells)/N.
	LandShare float64
}

// prePass runs stages 3 to 8 (elevation with no lake allowance, mesh, cells,
// sea level, climate, basins) on a private context: no renders, no log, no
// outputs. It returns the pre-pass record, and leaves the mesh, which does
// not depend on the elevation, for the mesh stage to reuse. A mesh that
// fails its checks ends the pre-pass with neither: the mesh stage then
// fails on it, with its renders.
func (c *Context) prePass() (*PrePass, error) {
	pc := &Context{
		Config:     c.Config,
		ConfigHash: c.ConfigHash,
		Log:        io.Discard,
		prepass:    true,
	}
	pc.Products.Layout, pc.Products.Bias = c.Products.Layout, c.Products.Bias
	for _, st := range Stages() {
		if st.Number < 3 || st.Number > 8 {
			continue
		}
		pc.stage = st
		if err := st.Run(pc); err != nil {
			if st.Name == "mesh" {
				// The mesh stage itself will fail on the same mesh, with
				// its renders; the pre-pass gives up without an allowance.
				return nil, nil
			}
			return nil, fmt.Errorf("pre-pass stage %s: %w", st, err)
		}
	}
	p := pc.Products
	c.preMesh = p.Mesh
	return &PrePass{SeaLevelM: p.SeaLevel.Level, LakeCells: p.Lakes.Cells()}, nil
}

// runElevation builds the bedrock elevation from the layout's bias, leaves
// it and the volcanic hotspots in Products, and renders it split at 0 m with
// the hotspots marked. The datum puts the land fraction of the raster above
// 0 m, raised by an allowance for the lakes the land-target stage will turn
// into water: before the final field, a pre-pass (Context.prePass) runs
// stages 3 to 8 once with no allowance and counts the lake cells, and the
// datum then puts (N + those cells) over the playable cells above 0 m
// (elevation.NewWithLakes), so that the land left after lakes is about N at
// about 0 m. Exactly one pre-pass, recorded in Products.PrePass; in the
// pre-pass itself this stage has no allowance. The log reports the
// allowance and the raster's own estimate of the level.
func runElevation(c *Context) error {
	lakes := 0
	if !c.prepass {
		pp, err := c.prePass()
		if err != nil {
			return err
		}
		if pp == nil {
			c.Logf("pre-pass stopped at a mesh that fails its checks: no lake allowance")
			pp = &PrePass{}
		}
		lakes = pp.LakeCells
		pp.LandShare = elevation.LandShareWithLakes(c.Config, lakes)
		c.Products.PrePass = pp
		c.Logf("pre-pass (stages 3–8, no lake allowance): sea level %.3f m, %d lake cells; datum land share %.4f (%.4f + %d lake cells)",
			pp.SeaLevelM, pp.LakeCells, pp.LandShare, c.Config.World.LandFraction, lakes)
	}
	e, err := elevation.NewWithLakes(c.Config, c.Products.Bias, lakes)
	if err != nil {
		return err
	}
	f, st := e.Field()
	hs := e.Hotspots()
	c.Products.Elevation = f
	c.Products.Hotspots = hs
	lo, hi := f.MinMax()
	target := elevation.SeaLevelForShare(f, e.LandShare())
	clamped := ""
	if st.Clamped {
		clamped = " (clamped: the land share is out of reach)"
	}
	c.Logf("datum shift %+.3f%s; %.1f%% of the non-rim signal positive", st.Shift, clamped, 100*st.Share)
	c.Logf("range %.0f to %.0f m; %.1f%% of the non-rim samples above 0 m; %.1f%% land (target) at %.0f m; falloff and rim at most %.0f m",
		lo, hi, 100*elevation.LandShare(f, 0), 100*e.LandShare(), target, elevation.BandMax(f))
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
