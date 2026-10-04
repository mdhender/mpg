// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package elevation

import (
	"image"

	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/render"
)

// ZFactor is the hillshade's height exaggeration in the stage render. At 1
// the relief of a 2 km raster barely shows; this makes ranges and hills
// read at a glance. It changes only the picture.
const ZFactor = 10

// WaterShade is the hillshade strength on the sea floor: low, so the floor's
// noise does not distract from the coasts.
const WaterShade = 0.3

// Render draws the elevation stage render: f, in meters, as a hypsometric
// map split at seaLevel (render.Land above, render.Water below) with a
// northwest hillshade, at full strength on land and WaterShade on the sea
// floor. The hillshade wraps east–west, so the seam shades like any other
// column.
func Render(f *field.Field, seaLevel float64) *image.RGBA {
	opts := render.DefaultReliefOptions()
	opts.SeaLevel = seaLevel
	opts.Light.ZFactor = ZFactor
	opts.WaterShade = WaterShade
	return render.Relief(f, opts)
}
