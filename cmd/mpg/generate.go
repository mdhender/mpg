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
		v, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return errors.New("want a decimal uint64")
		}
		seed = v
		return nil
	})
	landCells := fs.Int("land-cells", 0, "requested land cell `count` (overrides --config)")
	aspect := fs.String("aspect", "", "playable `aspect`: a name or W:H (overrides --config)")
	configFile := fs.String("config", "", "read the config from `file` (strict); default: built-in defaults")
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

	cfg := config.Default()
	if *configFile != "" {
		c, err := loadConfig(*configFile)
		if err != nil {
			fmt.Fprintf(stderr, "mpg generate: %v\n", err)
			return 1
		}
		cfg = c
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if set["seed"] {
		cfg.Seed = config.Seed(seed)
	}
	if set["land-cells"] {
		cfg.World.LandCells = *landCells
	}
	if set["aspect"] {
		cfg.World.Aspect = *aspect
	}
	if set["land-cells"] || set["aspect"] {
		cfg.ClearDerived()
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
	switch {
	case res.NotImplemented != nil:
		fmt.Fprintf(stderr, "mpg generate: stopped: stage %s not implemented yet\n", res.NotImplemented)
	case last >= 0 && last < len(stages)-1:
		fmt.Fprintf(stderr, "mpg generate: stopped after stage %s\n", stages[last])
	}
	return 0
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
