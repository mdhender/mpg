// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package topo provides the cylinder math shared by raster and mesh: east-west
// wrap, distance, bearing, latitude, and the polar rim.
//
// Coordinates are kilometers from the northwest corner. x runs east in [0, W)
// and wraps; y runs south in [0, H) and never wraps, so the poles are far
// apart. A Cylinder, built by New, carries W, H, and the rim and falloff
// widths in km.
//
// Conventions:
//
//   - DX, the wrapped east-west delta, lies in [−W/2, W/2). A target exactly
//     half way around is −W/2: the tie goes west.
//   - Bearing is degrees clockwise from north (−y) in [0, 360), from the
//     wrapped delta. A point's bearing to itself is 0.
//   - Latitude is 1 − 2·y/H: +1 at the north edge, −1 at the south edge.
//   - Bands are measured by PoleDistance(y) = min(y, H − y), so both poles are
//     treated alike: the rim is PoleDistance < Rim, and the falloff band
//     Rim ≤ PoleDistance < Rim + Falloff. A y on a band's inner boundary
//     belongs to the band inside it.
//
// Every non-exact result is computed with package fmath, so it has the same
// bits on every architecture.
package topo
