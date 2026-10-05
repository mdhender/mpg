// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package pipeline

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mdhender/mpg/internal/export"
	"github.com/mdhender/mpg/world"
)

// runExport converts the stage products to the world.json types (package
// export), validates them (world.Validate), writes world.json, and logs its
// size and counts. The deferred stages the run passed over are recorded in
// the outcomes. Its stage render, the player-style map, comes in S24.
func runExport(c *Context) error {
	p := &c.Products
	deferred := make([]string, len(c.skipped))
	for k, st := range c.skipped {
		deferred[k] = st.Name
	}
	w, err := export.Build(export.Input{
		Config:     c.Config,
		ConfigHash: c.ConfigHash,
		Mesh:       p.Mesh,
		Cells:      p.Cells,
		SeaLevel:   p.SeaLevel,
		Classes:    p.Classes,
		Edges:      p.Edges,
		Deferred:   deferred,
	})
	if err != nil {
		return err
	}
	if err := world.Validate(w); err != nil {
		return err
	}
	b, err := w.Bytes()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(c.OutputDir, world.File), b, 0o644); err != nil {
		return fmt.Errorf("pipeline: %w", err)
	}
	c.Logf("%s: %d bytes; %d cells, %d corners, %d edges, %d coastlines (%d closed)",
		world.File, len(b), len(w.Cells), len(w.Corners), len(w.Edges), len(w.Coastlines), closed(w.Coastlines))
	p.World = w
	p.WorldBytes = b
	return nil
}

func closed(cls []world.Coastline) int {
	n := 0
	for _, cl := range cls {
		if cl.Closed {
			n++
		}
	}
	return n
}
