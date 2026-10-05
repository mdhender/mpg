// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package classify

import (
	"image"
	"image/color"

	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/mesh"
)

// The landform render's palette: land in earth tones from pale straw
// (flats) through greens (plains) to browns (hills, mountains), plateaus in
// ochre and volcanic highlands in brick red, so each class reads apart at
// sweep-tile scale; salt water in three blues, lighter nearer land; fresh
// water in teal.
var (
	landformColors = [numLandforms]color.RGBA{
		LandformNone:      {0xff, 0x00, 0xff, 0xff}, // never drawn: an unset cell shows magenta
		SaltWater:         {0x4a, 0x8c, 0xc4, 0xff},
		FreshWater:        {0x3c, 0xb4, 0xb4, 0xff},
		Flats:             {0xec, 0xe6, 0xb0, 0xff},
		Plains:            {0xb9, 0xd9, 0x84, 0xff},
		RollingPlains:     {0x7d, 0xb0, 0x5c, 0xff},
		Hills:             {0xb8, 0x93, 0x5a, 0xff},
		Mountains:         {0x6b, 0x4a, 0x3a, 0xff},
		Plateaus:          {0xd8, 0x9b, 0x3e, 0xff},
		VolcanicHighlands: {0xb4, 0x3a, 0x2c, 0xff},
	}
	depthColors = [numDepths]color.RGBA{
		DepthNone: {0x4a, 0x8c, 0xc4, 0xff},
		Shallow:   {0x9c, 0xcb, 0xea, 0xff},
		Open:      {0x4a, 0x8c, 0xc4, 0xff},
		Deep:      {0x1d, 0x4a, 0x86, 0xff},
	}
	volcanoRing = color.RGBA{0xff, 0xff, 0xff, 0xff}
	volcanoInk  = color.RGBA{0x10, 0x10, 0x10, 0xff}
)

// LandformColor returns the landform render's fill for a cell with
// landform l and depth band d: salt water by its depth band, the rest by
// landform.
func LandformColor(l Landform, d Depth) color.RGBA {
	if l == SaltWater && d < numDepths {
		return depthColors[d]
	}
	if l < numLandforms {
		return landformColors[l]
	}
	return landformColors[LandformNone]
}

// LandformRender draws the classification stage render over mesh.CellRender:
// every playable cell filled with LandformColor, the rim as the ice sheet,
// and each volcano cell marked at its site by a black disc in a white ring,
// about half a cell across and never under a few pixels, so it shows on a
// sweep tile.
func LandformRender(f *field.Field, m *mesh.Mesh, r *Result) *image.RGBA {
	img := mesh.CellRender(f, m, func(i int) color.RGBA {
		if m.Cells[i].Rim {
			return mesh.IceColor
		}
		return LandformColor(r.Landform[i], r.Depth[i])
	})
	for i, v := range r.Volcano {
		if v {
			mesh.Mark(img, f, m, m.Cells[i].Site, 0.45, 4, volcanoRing)
			mesh.Mark(img, f, m, m.Cells[i].Site, 0.3, 2.5, volcanoInk)
		}
	}
	return img
}
