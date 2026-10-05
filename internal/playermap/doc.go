// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package playermap draws the player-style map of a world, or a window of
// it, from world.json alone (DESIGN.md, "One game data file": "Rendering a
// player's map segment means selecting cells whose bounding boxes intersect
// the window (taken modulo W) and drawing each polygon by its geography and
// biome, then rivers and coasts. Rim cells are drawn as impassable ice
// instead. No raster is needed.")
//
// Render reads nothing but a *world.World: no stage products, no raster.
// The export stage draws its stage render (14-export.png) with it from the
// world it has just built, and mpg render-stage from a world.json file. The
// drawing code lives here rather than in package world, which is the schema
// the game imports and stays free of image code; a renderer in the game can
// follow the same rules.
//
// # What it draws
//
// In this order, each in a fixed order of ids:
//
//  1. Every cell: rim cells as the ice sheet (IceColor, with no outlines
//     between its cells), salt water by depth band, and the other landforms
//     in the classification stage render's palette (classify.LandformColor,
//     looked up by the world's codebook strings), so the stage render and
//     the player map agree. Biomes come in milestone 8.
//  2. The borders between land cells, a pixel wide, blended faintly.
//  3. The ice front, the edges between the rim and playable cells.
//  4. Rivers by class (none until milestone 7), then the coastlines
//     (world.Coastlines, edge by edge).
//  5. Each volcano cell's site, as a black disc in a white ring.
//
// Line widths and marks scale with the size of a province in pixels. Edge
// seeds (noisy coasts) are not used yet.
//
// # Windows and the pixel lattice
//
// A Lattice is the map's global pixel grid at a scale in pixels per km. Its
// width is W·scale rounded to whole pixels, and the pixels per km actually
// used are that width over W (and likewise north to south), so the
// east–west wrap is a whole number of pixels. A Frame is a rectangle of
// that grid: Lattice.Full is the whole map, and Lattice.Window converts a
// window given in km (x, y, width, height; x taken modulo W, so a window
// may start west of 0 or cross the seam) by snapping its origin down to the
// pixel holding (x, y) and rounding its size to whole pixels. A window is
// at most the map's width and lies within [0, H] north to south.
//
// # A window is a crop of the full map
//
// Rendering a window gives exactly the pixels of the matching crop of the
// full render (Crop), across the seam too. This holds by construction:
//
//   - Every primitive is rasterized in global pixel coordinates, with the
//     same floating-point arithmetic whatever the frame. Only the resulting
//     whole pixels are mapped into the frame, column g to (g − X) mod the
//     map's width; that integer step is the only difference between a
//     window and the full map. Nothing is drawn twice shifted by the map
//     width, so no shifted float can round differently.
//   - Cell fills tile the map exactly: a side's crossing with a scan line
//     is computed from its lower-numbered corner, so both cells sharing it
//     get the same column, and a cell across the seam adds a whole number
//     of map widths after that column is rounded. Each pixel belongs to one
//     cell, so the fill order does not matter.
//   - Lines are drawn from an edge's lower-numbered corner, whichever way a
//     coastline walks it. Primitives are drawn in the same order in both
//     renders, and a window draws every primitive that reaches it (cells
//     selected by bounding box with a margin, lines by their extent), so
//     the last ink on each pixel is the same. The faint borders go through
//     a layer that is a set of pixels, blended in integer arithmetic.
//
// # Determinism
//
// The arithmetic follows package fmath (every product that reaches a sum
// is rounded explicitly; hypot from fmath), so a render is the same on
// every machine, and TestNoFusedMultiplyAdd checks the compiled code.
// Rendering never changes data or hashes.
package playermap
