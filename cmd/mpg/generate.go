// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/pipeline"
)

// runGenerate runs the pipeline. Exit codes: 0 on success (including a run
// that stops at a stage not implemented yet), 1 on a config or stage error,
// 2 on a usage error.
func runGenerate(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("generate", stderr)
	var seed uint64
	fs.Func("seed", "world `seed`, a decimal uint64 (overrides --config)", func(s string) error {
		v, err := parseSeed(s)
		seed = v
		return err
	})
	cf := addConfigFlags(fs)
	aspect := fs.String("aspect", "", "playable `aspect`: a name or W:H (overrides --config)")
	preset := fs.String("preset", "", "layout `preset`: pangaea, continents, archipelago, islands, or custom (overrides --config)")
	output := fs.String("output", "", "write config.json and later outputs to `dir` (required; created if missing; files are overwritten)")
	renders := fs.String("renders", "", "write stage renders to `dir`")
	stopAfter := fs.String("stop-after", "", "stop after `stage` (a name or number)")
	if code, stop := parse(fs, args); stop {
		return code
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(stderr, "mpg generate: unexpected argument %q\n", fs.Arg(0))
		return 2
	}
	if *output == "" {
		fmt.Fprintln(stderr, "mpg generate: --output is required")
		return 2
	}
	stages := pipeline.Stages()
	last := -1
	if *stopAfter != "" {
		i, err := pipeline.Lookup(stages, *stopAfter)
		if err != nil {
			fmt.Fprintf(stderr, "mpg generate: --stop-after: %v\n", err)
			return 2
		}
		last = i
	}

	cfg, err := cf.load()
	if err != nil {
		fmt.Fprintf(stderr, "mpg generate: %v\n", err)
		return 1
	}
	if isSet(fs, "seed") {
		cfg.Seed = config.Seed(seed)
	}
	if isSet(fs, "aspect") {
		setAspect(&cfg, *aspect)
	}
	if isSet(fs, "preset") {
		cfg.Layout.Preset = *preset
	}

	ctx, err := pipeline.NewContext(cfg, *output, *renders, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "mpg generate: %v\n", err)
		return 1
	}
	res, err := pipeline.Run(ctx, stages, last)
	if err != nil {
		fmt.Fprintf(stderr, "mpg generate: %v\n", err)
		return 1
	}

	w := ctx.Config.World
	names := make([]string, len(res.Ran))
	for i, st := range res.Ran {
		names[i] = st.Name
	}
	fmt.Fprintf(stdout, "config  %s\n", ctx.ConfigHash)
	fmt.Fprintf(stdout, "seed    %d\n", uint64(ctx.Config.Seed))
	fmt.Fprintf(stdout, "world   %.1f x %.1f km, %d land of %d playable cells, aspect %s\n",
		w.WidthKm, w.HeightKm, w.LandCells, w.PlayableCells, w.Aspect)
	fmt.Fprintf(stdout, "stages  %s\n", strings.Join(names, ", "))
	if len(res.Skipped) > 0 {
		skipped := make([]string, len(res.Skipped))
		for i, st := range res.Skipped {
			skipped[i] = st.Name
		}
		fmt.Fprintf(stderr, "mpg generate: skipped %s: not implemented yet\n", strings.Join(skipped, ", "))
	}
	switch {
	case res.NotImplemented != nil:
		fmt.Fprintf(stderr, "mpg generate: stopped: stage %s not implemented yet\n", res.NotImplemented)
	case last >= 0 && last < len(stages)-1:
		fmt.Fprintf(stderr, "mpg generate: stopped after stage %s\n", stages[last])
	}
	return 0
}

// configFlags are the config-source flags shared by generate and sweep:
// --config and --land-cells.
type configFlags struct {
	fs        *flag.FlagSet
	file      string
	landCells int
}

// addConfigFlags registers --config and --land-cells on fs.
func addConfigFlags(fs *flag.FlagSet) *configFlags {
	cf := &configFlags{fs: fs}
	fs.StringVar(&cf.file, "config", "", "read the config from `file` (strict); default: built-in defaults")
	fs.IntVar(&cf.landCells, "land-cells", 0, "requested land cell `count` (overrides --config)")
	return cf
}

// load returns the config file read strictly (or the built-in defaults
// without --config) with --land-cells applied when it was given. The result
// is not resolved after an override; NewContext or Resolve does that.
func (cf *configFlags) load() (config.Config, error) {
	cfg := config.Default()
	if cf.file != "" {
		c, err := loadConfig(cf.file)
		if err != nil {
			return config.Config{}, err
		}
		cfg = c
	}
	if isSet(cf.fs, "land-cells") {
		cfg.World.LandCells = cf.landCells
		cfg.ClearDerived()
	}
	return cfg, nil
}

// setAspect overrides the config's aspect and clears the derived sizes,
// which depend on it.
func setAspect(cfg *config.Config, aspect string) {
	cfg.World.Aspect = aspect
	cfg.ClearDerived()
}

// isSet reports whether the flag name was given on the command line.
func isSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	return set
}

// parseSeed parses a decimal uint64 seed.
func parseSeed(s string) (uint64, error) {
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, errors.New("want a decimal uint64")
	}
	return v, nil
}

// loadConfig reads and resolves a config file strictly.
func loadConfig(path string) (config.Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return config.Config{}, err
	}
	defer f.Close()
	cfg, err := config.Decode(f)
	if err != nil {
		return config.Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}
