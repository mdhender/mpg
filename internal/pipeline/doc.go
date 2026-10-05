// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package pipeline runs the generator's stages in order.
//
// The stages and their names and numbers are fixed by the pipeline table in
// DESIGN.md ("Pipeline"); Stages returns them. A stage whose Run is nil is
// registered but not implemented yet: the runner stops cleanly when it
// reaches one, so a partly built pipeline still produces its early outputs.
// A stage marked Deferred is the exception: it is not implemented, but the
// implemented stages after it do not need it yet, so the runner passes over
// it and lists it in Result.Skipped. Stage 13 (measures) is deferred, so a
// full run reaches export (14), which writes world.json and records the
// stages passed over in its outcomes; its render is the player-style map
// (package playermap), drawn from the world alone.
//
// # Lakes and the land target
//
// DESIGN.md's climate coupling, with an allowance for lakes in the datum:
//
//   - The elevation stage (3) first runs a pre-pass: stages 3 to 8 once on
//     a private context (no renders, logs or outputs) with no lake
//     allowance, counting the lake cells L of its basins stage at its sea
//     level (Products.PrePass). The final field's datum then puts the land
//     fraction times (N + L)/N above 0 m (elevation.NewWithLakes), so that
//     the land left after lakes is about N at about 0 m. Exactly one
//     pre-pass; the mesh, which does not depend on the elevation, is built
//     once and reused by stage 4.
//   - The sea level stage (6) searches for N + L land cells before lakes,
//     so its ocean, which the first climate pass (7) reads, is about the
//     final one; the basins stage (8) balances the lakes there.
//   - The land-target stage (9) runs the sea-level search again with
//     stages 6 and 8 inside it, against the fixed first climate pass,
//     counting land after lakes (lake and inland-sea cells are water,
//     playas and dry basin floors land), its first probe expecting the
//     basins stage's lake cells; then the final climate pass with the
//     lakes, in which inland seas recharge the air and lakes do not. It
//     saves the search record and the pass count (2) in Products.Target.
//   - The rivers (10), classification (11), edges (12) and export (14) read
//     the land target's land and water. The river stage builds the corner
//     drainage tree (package river) on it, with each overflowing lake's
//     spill corner and pass cell from its basins, and compares the tree's
//     catchments with the water balance's cell-level ones (RiverInput).
//     It then accumulates the final climate pass's runoff down the tree,
//     with each overflowing lake's water-balance overflow at its outlet
//     (RiverFlow), and selects and classes the river edges by the config's
//     breaks (RiverParams). The edge stage puts the classes on the edges,
//     and export writes the polylines and mouths. The classification stage
//     gives the land its biomes and the cells their surfaces from the final
//     climate pass, the lakes and playas, and the river classes
//     (CoverInput; classify.Result.Cover).
//
// A stage is a function of a *Context, which carries the resolved config
// and its hash, the output and render directories, the stage seed helpers, a
// writer for log lines, and Products, where stages leave what later stages
// read. A stage that repeats others (the elevation pre-pass repeats stages
// 3 to 8, the land-target search the sea level flood and the basins) calls
// their functions directly.
//
// # Renders
//
// When Context.RendersDir is set, Context.Render writes a stage's PNG there
// under the name render.StageFile gives, with the stage and config hash in
// its text chunks. When Context.Sink is set, Render also hands it each
// render first, so a caller (sweep) can collect renders without reading
// files back. With neither, Render does nothing. Renders never change data
// or hashes.
package pipeline
