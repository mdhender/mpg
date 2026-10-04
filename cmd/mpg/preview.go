// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"image"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/internal/noise"
	"github.com/mdhender/mpg/internal/render"
)

// previewNoise names the noise tuning preview, a sweep column that is not a
// pipeline stage.
const previewNoise = "noise"

// noisePreview renders the "noise" sweep column: a TUNING PREVIEW, not a
// pipeline stage. It is accepted only by sweep's --stage list (never by
// --stop-after or the stage registry) and has no products. It fills the
// seed's raster with the default composite from package noise (the same
// composite as noise's seam test, at the default ladders): warp the point,
// then 0.65·fBm + 0.35·(2·ridged − 1), seeded with noise.New(seed,
// "elevation"), and colors it with the Viridis ramp stretched from the
// field's minimum to its maximum. The real elevation stage (milestone 2)
// supersedes it.
func noisePreview(cfg config.Config) (*image.RGBA, error) {
	f, err := noiseField(cfg)
	if err != nil {
		return nil, err
	}
	return render.Sequential(f, render.Viridis), nil
}

// noiseField fills the seed's raster with the noise preview's composite (see
// noisePreview). The golden test hashes it directly, so the preview's
// arithmetic is pinned independently of the color ramp.
func noiseField(cfg config.Config) (*field.Field, error) {
	f, err := field.FromConfig(cfg)
	if err != nil {
		return nil, err
	}
	m := noise.NewCylinder(f.Cylinder())
	s := noise.New(uint64(cfg.Seed), "elevation")
	warpSrc, fbmSrc, ridgedSrc := s.Stream(0), s.Stream(1), s.Stream(2)
	warp, fbm, ridged := noise.DefaultWarp(), noise.DefaultFBM(), noise.DefaultRidged()
	noise.Fill(f, func(x, y float64) float64 {
		wx, wy := warp.Apply(warpSrc, m, x, y)
		q := m.Point(wx, wy)
		a := fbm.Sample(fbmSrc, q)
		r := fmath.MulAdd(2, ridged.Sample(ridgedSrc, q), -1)
		return fmath.MulAdd(0.65, a, fmath.Mul(0.35, r))
	})
	return f, nil
}
