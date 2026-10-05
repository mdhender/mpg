// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package playermap

import (
	"image/color"

	"github.com/mdhender/mpg/internal/basin"
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

// The inland water and playa inks, the basins stage's lakes render's (package
// basin), so the stage render and the player map read alike.
var (
	// SaltLakeColor fills a lake cell with the salt flag; a fresh lake
	// keeps the fresh-water landform's teal.
	SaltLakeColor = basin.SaltLakeColor
	// SaltSeaTint is mixed half and half into the depth band's blue of an
	// inland-sea cell with the salt flag; a fresh inland sea is drawn as
	// the ocean is, by depth band.
	SaltSeaTint = basin.SaltSeaColor
	// PlayaColor fills a playa cell: a dry lake bed.
	PlayaColor = basin.PlayaColor
)

// CellColor returns the fill of a cell: ice for a rim cell, a playa in
// PlayaColor, a salt lake in SaltLakeColor, a salt inland sea its depth
// band's blue mixed with SaltSeaTint, other salt water by its depth band,
// and every other landform by classify.LandformColor.
func CellColor(c *world.Cell) color.RGBA {
	switch {
	case c.HasFlag(world.FlagRim):
		return IceColor
	case c.HasFlag(world.FlagPlaya):
		return PlayaColor
	case c.Water == world.Lake && c.HasFlag(world.FlagSalt):
		return SaltLakeColor
	case c.Water == world.InlandSea && c.HasFlag(world.FlagSalt):
		return mix(LandformColor(c.Landform, c.Depth), SaltSeaTint)
	}
	return LandformColor(c.Landform, c.Depth)
}

// mix returns the even mix of a and b, rounded down, opaque.
func mix(a, b color.RGBA) color.RGBA {
	m := func(x, y uint8) uint8 { return uint8((uint16(x) + uint16(y)) / 2) }
	return color.RGBA{R: m(a.R, b.R), G: m(a.G, b.G), B: m(a.B, b.B), A: 0xff}
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
