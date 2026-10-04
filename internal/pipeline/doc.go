// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package pipeline runs the generator's stages in order.
//
// The stages and their names and numbers are fixed by the pipeline table in
// DESIGN.md ("Pipeline"); Stages returns them. A stage whose Run is nil is
// registered but not implemented yet: the runner stops cleanly when it
// reaches one, so a partly built pipeline still produces its early outputs.
//
// A stage is a function of a *Context, which carries the resolved config
// and its hash, the output and render directories, the stage seed helpers, a
// writer for log lines, and Products, where stages leave what later stages
// read. A stage that repeats others (the land-target check repeats sea level
// and basins) calls their functions directly.
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
