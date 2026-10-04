// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package pipeline

import (
	"errors"
	"fmt"
	"image"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/layout"
	"github.com/mdhender/mpg/internal/render"
	"github.com/mdhender/mpg/internal/seed"
)

// Context is the state shared by the stages of one run.
type Context struct {
	// Config is the resolved config. Stages must not change it.
	Config config.Config
	// ConfigHash is Config.Hash(), recorded in every render.
	ConfigHash string
	// OutputDir is the directory that receives config.json, world.json,
	// and measures.json.
	OutputDir string
	// RendersDir is the directory that receives stage renders, or "" for
	// none.
	RendersDir string
	// Log receives progress and diagnostic lines. It is never nil.
	Log io.Writer
	// Products holds what stages produce for later stages.
	Products Products
	// Sink, when not nil, receives every render, before it is written to
	// RendersDir (if set), with the running stage and the render's variant.
	// It lets a caller such as sweep collect renders in memory. It must not
	// modify img or keep it past the call, since a stage may reuse it. An
	// error from Sink fails the render.
	Sink func(st Stage, variant string, img image.Image) error

	// stage is the stage running now.
	stage Stage
}

// Products holds the stage products. Each stage adds the fields it fills as
// it is implemented; a field is zero until its stage has run.
type Products struct {
	// Layout is the layout stage's attractors and repulsors.
	Layout *layout.Layout
	// Bias is the layout stage's continental bias field, in [−1, 1].
	Bias *field.Field
}

// NewContext resolves cfg and returns a context for a run writing to
// outputDir, with renders in rendersDir ("" for none) and log lines to log
// (nil for none).
func NewContext(cfg config.Config, outputDir, rendersDir string, log io.Writer) (*Context, error) {
	if outputDir == "" {
		return nil, errors.New("pipeline: no output directory")
	}
	if err := cfg.Resolve(); err != nil {
		return nil, err
	}
	hash, err := cfg.Hash()
	if err != nil {
		return nil, err
	}
	if log == nil {
		log = io.Discard
	}
	return &Context{
		Config:     cfg,
		ConfigHash: hash,
		OutputDir:  outputDir,
		RendersDir: rendersDir,
		Log:        log,
	}, nil
}

// Stage returns the stage running now.
func (c *Context) Stage() Stage { return c.stage }

// Seed returns the two stage-seed values for the named seed stream (such as
// "elevation" or "warp") at an algorithm version; see package seed.
func (c *Context) Seed(stream, version string) (seed1, seed2 uint64) {
	return seed.Derive(uint64(c.Config.Seed), stream, version)
}

// Rand returns a new generator for the named seed stream at an algorithm
// version. The caller owns it.
func (c *Context) Rand(stream, version string) *rand.Rand {
	return seed.Rand(uint64(c.Config.Seed), stream, version)
}

// Render passes img, the running stage's render with an optional kebab-case
// variant, to Sink when it is set, then writes it to RendersDir when that is
// set. With neither, it does nothing.
func (c *Context) Render(variant string, img image.Image) error {
	name := render.StageFile(c.stage.Number, c.stage.Name, variant)
	if c.Sink != nil {
		if err := c.Sink(c.stage, variant, img); err != nil {
			return err
		}
	}
	if c.RendersDir == "" {
		return nil
	}
	meta := render.Meta{Stage: strings.TrimSuffix(name, ".png"), ConfigHash: c.ConfigHash}
	return render.WritePNGFile(filepath.Join(c.RendersDir, name), img, meta)
}

// Logf writes one log line, prefixed with the running stage's name.
func (c *Context) Logf(format string, args ...any) {
	fmt.Fprintf(c.Log, "%s: %s\n", c.stage.Name, fmt.Sprintf(format, args...))
}

// runConfig is stage 1. NewContext has already resolved the config; this
// writes its canonical bytes to config.json, replacing any existing file.
func runConfig(c *Context) error {
	b, err := c.Config.Bytes()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(c.OutputDir, ConfigFile), b, 0o644); err != nil {
		return fmt.Errorf("pipeline: %w", err)
	}
	return nil
}

// ConfigFile is the name of the resolved config in the output directory.
const ConfigFile = "config.json"
