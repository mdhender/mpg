// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package pipeline runs the generator's stages in order.
//
// The stages and their names and numbers are fixed by the pipeline table in
// DESIGN.md ("Pipeline"); Stages returns them. A stage whose Run is nil is
// registered but not implemented yet: the runner stops cleanly when it
// reaches one, so a partly built pipeline still produces its early outputs.
// A stage marked Deferred is the exception: it is not implemented, but the
// implemented stages after it do not need it yet, so the runner passes over
// it and lists it in Result.Skipped. Stages 8 to 10 (basins, the
// land-target check, rivers) are deferred, so classification (11) runs on
// the sea level stage's land and water until they exist; so is stage 13
// (measures), so a full run reaches export (14), which writes world.json
// and records the stages passed over in its outcomes; its render is the
// player-style map (package playermap), drawn from the world alone.
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
