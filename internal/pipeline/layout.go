// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package pipeline

import (
	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/layout"
)

// runLayout places the preset's attractors and repulsors, fills the
// continental bias field, and renders it with the attractors marked.
func runLayout(c *Context) error {
	l, err := layout.New(c.Config)
	if err != nil {
		return err
	}
	bias, err := field.FromConfig(c.Config)
	if err != nil {
		return err
	}
	l.Fill(bias)
	c.Products.Layout = l
	c.Products.Bias = bias
	c.Logf("preset %s: %d masses, %d attractors, %d repulsors, spacing %.3g of %.3g (%d relaxations), %.1f%% of the band positive",
		l.Preset, l.Masses, len(l.Attractors), len(l.Repulsors), l.Spacing, l.SpacingWanted, l.Relaxations, 100*layout.PositiveShare(bias))
	return c.Render("", layout.Render(bias, l))
}
