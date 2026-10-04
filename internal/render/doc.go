// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package render draws stage renders and contact sheets.
//
// Every pipeline stage has a render (DESIGN.md, "Pipeline" and "Tuning:
// early and often"). This package holds the shared harness: color ramps,
// field coloring, hillshade, simple drawing primitives, stage file names,
// and a PNG writer that records the stage and config hash in the file.
//
// # Images
//
// A field renders at one pixel per raster sample: column i is pixel x = i
// (west to east) and row j is pixel y = j (north to south), so a 1268 × 567
// field gives a 1268 × 567 image. Pixel (x, y) covers [x, x+1) × [y, y+1)
// in pixel coordinates, and sample (i, j)'s center is at (i + 0.5, j + 0.5);
// see ToPixel.
//
// # Provenance
//
// WritePNG inserts tEXt chunks after IHDR: "mpg:stage" and "mpg:config-hash"
// first, then any extra entries in the order given. Standard PNG decoders
// ignore them; ReadPNGText and ReadMeta read them back. Renders are not
// hashed content, so the chunks never affect world hashes, and they may carry
// build-dependent values such as a version.
//
// # Determinism
//
// Renders never feed world data, but a render of the same field should be
// the same picture on every machine, so the render-pixel hash test can pin
// it. Color and shading arithmetic follows the package fmath rules: every
// product that reaches a sum is rounded explicitly, transcendentals come from
// fmath, and channel values are rounded with math.Floor(v + 0.5). PixelHash
// hashes decoded pixels, never PNG bytes, because the compressor is not part
// of the contract.
package render
