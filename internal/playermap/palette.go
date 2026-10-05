// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package playermap

import (
	"image/color"

	"github.com/mdhender/mpg/internal/classify"
	"github.com/mdhender/mpg/internal/mesh"
	"github.com/mdhender/mpg/world"
)

// The player map's inks. Cell fills are the classification stage render's
// (classify.LandformColor), looked up by the world's codebook strings, and
// the ice sheet and its front are the mesh renders', so the stage renders
// and the player map read alike.
var (
	// IceColor fills the rim, the polar ice sheet.
	IceColor = mesh.IceColor
	// IceFrontColor draws the edges between the rim and playable cells.
	IceFrontColor = mesh.IceFrontColor
	// CoastColor draws the coastlines.
	CoastColor = color.RGBA{R: 0x1b, G: 0x2c, B: 0x45, A: 0xff}
	// BorderColor is blended at BorderAlpha over the borders between two
	// land cells.
	BorderColor = color.RGBA{R: 0x10, G: 0x10, B: 0x10, A: 0xff}
	// RiverColor draws rivers (none until milestone 7).
	RiverColor = color.RGBA{R: 0x2a, G: 0x6f, B: 0xc0, A: 0xff}
	// VolcanoRing and VolcanoInk mark a volcano cell's site: a black disc
	// in a white ring, as the classification render marks them.
	VolcanoRing = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	VolcanoInk  = color.RGBA{R: 0x10, G: 0x10, B: 0x10, A: 0xff}
	// UnknownColor fills a cell whose landform or depth is not in the
	// codebooks; a valid world has none.
	UnknownColor = color.RGBA{R: 0xff, G: 0x00, B: 0xff, A: 0xff}
)

// BorderAlpha is the opacity of the land cell borders, out of 255.
const BorderAlpha = 60

var (
	landforms = map[world.Landform]classify.Landform{}
	depths    = map[world.Depth]classify.Depth{world.DepthNone: classify.DepthNone}
)

func init() {
	for _, l := range classify.Landforms {
		landforms[world.Landform(l.String())] = l
	}
	for _, d := range classify.Depths {
		depths[world.Depth(d.String())] = d
	}
}

// CellColor returns the fill of a cell: ice for a rim cell, salt water by
// its depth band, and every other landform by classify.LandformColor.
func CellColor(c *world.Cell) color.RGBA {
	if c.HasFlag(world.FlagRim) {
		return IceColor
	}
	return LandformColor(c.Landform, c.Depth)
}

// LandformColor returns the fill for landform l with depth band d (salt
// water only), the classification render's color for the same codes, or
// UnknownColor for a code outside the codebooks.
func LandformColor(l world.Landform, d world.Depth) color.RGBA {
	cl, ok := landforms[l]
	cd, okd := depths[d]
	if !ok || !okd {
		return UnknownColor
	}
	return classify.LandformColor(cl, cd)
}

// riverWidth returns a river class's line width as a fraction of a cell's
// side.
func riverWidth(c world.RiverClass) float64 {
	switch c {
	case world.Stream:
		return 1.0 / 16
	case world.River:
		return 1.0 / 10
	case world.MajorRiver:
		return 1.0 / 6
	}
	return 0
}
