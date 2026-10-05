// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/world"
)

// runValidate reads a world.json strictly and checks its invariants
// (world.Validate), and with --config that it was generated from that
// config. PATH is the file or the directory holding it. Exit codes: 0 when
// the world is valid, 1 when it is not or cannot be read, 2 on a usage
// error.
func runValidate(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("validate", stderr)
	cfgFile := fs.String("config", "", "also check the world against the config in `file`")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: mpg validate [--config file] PATH")
		fmt.Fprintln(stderr, "PATH is a world.json file or a directory holding one.")
		fs.PrintDefaults()
	}
	if code, stop := parse(fs, args); stop {
		return code
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "mpg validate: want exactly one PATH")
		fs.Usage()
		return 2
	}
	path := fs.Arg(0)
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		path = filepath.Join(path, world.File)
	}
	w, err := readWorld(path)
	if err != nil {
		fmt.Fprintf(stderr, "mpg validate: %v\n", err)
		return 1
	}
	verr := world.Validate(w)
	var cerr error
	if *cfgFile != "" {
		cfg, err := loadConfig(*cfgFile)
		if err != nil {
			fmt.Fprintf(stderr, "mpg validate: %v\n", err)
			return 1
		}
		cerr = matchConfig(w, cfg)
	}
	if err := errors.Join(verr, cerr); err != nil {
		fmt.Fprintf(stderr, "mpg validate: %s: invalid\n%v\n", path, err)
		return 1
	}
	o := &w.Outcomes
	met := "met"
	if !o.Met {
		met = "UNMET"
	}
	fmt.Fprintf(stdout, "%s: valid world schema %d, config %s\n", path, w.Schema, w.Meta.ConfigHash)
	fmt.Fprintf(stdout, "  %d cells (%d playable), %d corners, %d edges, %d coastlines, %d rivers\n",
		len(w.Cells), o.PlayableCells, len(w.Corners), len(w.Edges), len(w.Coastlines), len(w.Rivers))
	fmt.Fprintf(stdout, "  land %d of target %d ± %d (%s, %s), sea level %.3f m\n",
		o.LandCells, o.TargetLandCells, o.ToleranceCells, met, o.Reason, o.SeaLevelM)
	if *cfgFile != "" {
		fmt.Fprintf(stdout, "  matches config %s\n", *cfgFile)
	}
	return 0
}

// readWorld reads and decodes a world.json strictly.
func readWorld(path string) (*world.World, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	w, err := world.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return w, nil
}

// matchConfig checks that w was generated from cfg: the config hash, the
// seed, and the sizes and targets the world repeats.
func matchConfig(w *world.World, cfg config.Config) error {
	hash, err := cfg.Hash()
	if err != nil {
		return err
	}
	var errs []error
	check := func(name string, got, want any) {
		if got != want {
			errs = append(errs, fmt.Errorf("config: %s is %v in world.json, %v in the config", name, got, want))
		}
	}
	m := &w.Meta
	check("config hash", m.ConfigHash, hash)
	check("seed", m.Seed, strconv.FormatUint(uint64(cfg.Seed), 10))
	check("width_km", m.WidthKm, cfg.World.WidthKm)
	check("height_km", m.HeightKm, cfg.World.HeightKm)
	check("rim_cells", m.RimCells, cfg.Rim.Cells)
	check("rim_km", m.RimKm, cfg.Rim.Km)
	check("hex_flat_to_flat_mi", m.HexFlatToFlatMi, cfg.Province.HexFlatToFlatMi)
	check("province_area_km2", m.ProvinceAreaKm2, cfg.Province.AreaKm2)
	check("target land cells", w.Outcomes.TargetLandCells, cfg.World.LandCells)
	return errors.Join(errs...)
}
