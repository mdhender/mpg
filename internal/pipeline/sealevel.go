// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package pipeline

import (
	"fmt"
	"strings"

	"github.com/mdhender/mpg/internal/cells"
)

// runSeaLevel searches for the sea level that leaves world.land_cells (N)
// land cells, floods the ocean from the rim, logs the target, the achieved
// count and its deviation, the reason the search ended and its trace, and
// renders the land and water. An unmet target is reported, not an error.
func runSeaLevel(c *Context) error {
	m, s := c.Products.Mesh, c.Products.Cells
	sl, err := cells.Search(m, s.Altitude, c.Config.World.LandCells)
	if err != nil {
		return err
	}
	c.Logf("target %d land cells ± %d (%d%%) of %d playable; policy %s, budget %d",
		sl.Target, sl.Tolerance, cells.TolerancePercent, sl.Playable, sl.Policy, sl.Budget)
	c.Logf("level %.3f m (estimate %.3f m): %d land (%+d, %+.2f%%), %d ocean, %d dry basin cells; land area %.0f km²",
		sl.Level, sl.Initial, sl.LandCells, sl.Deviation(), sl.DeviationPercent(), sl.OceanCells, sl.BasinCells, sl.LandAreaKm2)
	probes := make([]string, len(sl.Trace))
	for k, p := range sl.Trace {
		probes[k] = fmt.Sprintf("%s %.1f m → %d", p.Method, p.Level, p.Land)
	}
	c.Logf("%d probes, %s: %s", len(sl.Trace), sl.Reason, strings.Join(probes, "; "))
	if !sl.Met {
		c.Logf("UNMET land target: %d land cells, %d outside %d ± %d (%s)",
			sl.LandCells, abs(sl.Deviation())-sl.Tolerance, sl.Target, sl.Tolerance, sl.Reason)
	}
	if err := c.Render("", cells.SeaLevelRender(c.Products.Elevation, m, s, sl)); err != nil {
		return err
	}
	c.Products.SeaLevel = sl
	return nil
}

func abs(v int) int { return max(v, -v) }
