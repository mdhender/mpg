// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"cmp"
	"fmt"
	"image"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mdhender/mpg/internal/basin"
	"github.com/mdhender/mpg/internal/classify"
	"github.com/mdhender/mpg/internal/climate"
	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/mesh"
	"github.com/mdhender/mpg/internal/pipeline"
	"github.com/mdhender/mpg/internal/render"
	"github.com/mdhender/mpg/world"
)

// Sweep limits.
const (
	maxSweepSeeds   = 256
	maxSweepAspects = 8
	maxSweepPresets = 8
	minSweepTile    = 16
	maxSweepTile    = 4096
	defaultTile     = 320
	captionLines    = 2
	maxLabelChars   = 26
)

// runSweep builds a contact sheet: one row per aspect, preset, and seed
// (aspects outermost, seeds innermost), one column per requested stage, each tile a stage render shrunk to
// --tile pixels wide. When the run reaches the sea level stage, each row's
// label shows the land count achieved against N (and UNMET when it misses
// the 1% tolerance): after lakes once the run reaches the land-target
// stage, and the sea level stage's otherwise. Each sea-level tile's caption
// gives the count against its target (N plus the pre-pass's lake cells),
// the deviation, and the search's reason; each land-target tile's the land
// after lakes against N, the deviation, the reason, the probes (p) and the
// lake cells (lk), its "lakes" variant the lakes caption below, and its
// "precip" and "aridity" variants the final climate's, as for climate. Each climate tile's caption depends on its variant: for
// the temperature and the mask, the temperature range in °C of the rim,
// the land, and the land's median (land lo..hi ~median); for precip, pet
// and runoff, the land's p5/p50/p95 in mm; for moisture, the land's
// p10/p50/p90; for aridity, the land's UNEP class shares in percent (ha, a,
// sa, ds, hu: hyper-arid to humid). Each classify tile's caption gives the land landform
// shares in percent (fl, pl, rp, hi, mt, pt, vh: flats to volcanic
// highlands) and the volcanoes on land (v); a classify:biome tile's the
// four largest biome shares in percent (de desert, sv savanna, …: see
// classify.Result.BiomeShares); a classify:surface tile's the glacier (gl),
// ice-field (if), polar-desert (pd) and pack-ice (pk) cells; and a
// classify:wetness tile's the wetland cells and their share of the land.
// Each basins tile's caption gives
// the depressions found (dep), the basins at least the minimum deep (bas),
// the deepest nesting (nest), and the cells in basins (cells); a
// basins:lakes tile's gives the lakes (lk), inland seas (sea), the salt ones
// among both (salt), the playas (pl), the full basins (full), and the lake
// cells (cells). Each edges tile's caption gives
// the direction error's mean, p95 and max in degrees (err), the share of
// edges whose reverse direction is not the opposite point (rev), and the
// coast edges per land cell (coast). The measures stage has no render yet,
// so a measures tile is blank; its caption gives the checks that passed
// against those run (ok), the failed report-only checks (rpt), and the
// failed gates (gate). When the run reaches the measures stage and a gate
// failed, the row label's land line ends in GATE; a failed gate never
// stops the sweep. Rows run one after another, so the sheet does not
// depend on scheduling. Exit codes: 0 on success, 1 on a config error, an
// unimplemented stage, or a run error, 2 on a usage error.
func runSweep(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("sweep", stderr)
	seedsFlag := fs.String("seeds", "", "world `seeds`: decimal uint64 values and ranges, as in 1-16 or 1,5,9-12 (required)")
	stagesFlag := fs.String("stage", "", "comma-separated `stages`, one column each in order: pipeline stage names or numbers, optionally stage:variant (required)")
	aspectsFlag := fs.String("aspect", "", "comma-separated playable `aspects`, one block of rows each (overrides --config)")
	presetsFlag := fs.String("preset", "", "comma-separated layout `presets`, one block of rows each within an aspect (overrides --config)")
	cf := addConfigFlags(fs)
	output := fs.String("output", "", "write the contact sheet PNG to `file` (required)")
	tile := fs.Int("tile", defaultTile, "tile width in `pixels`; the height follows the world's shape")
	if code, stop := parse(fs, args); stop {
		return code
	}
	usageErr := func(format string, args ...any) int {
		fmt.Fprintf(stderr, "mpg sweep: "+format+"\n", args...)
		return 2
	}
	if fs.NArg() != 0 {
		return usageErr("unexpected argument %q", fs.Arg(0))
	}
	if *output == "" {
		return usageErr("--output is required")
	}
	if *seedsFlag == "" {
		return usageErr("--seeds is required")
	}
	if *stagesFlag == "" {
		return usageErr("--stage is required")
	}
	if *tile < minSweepTile || *tile > maxSweepTile {
		return usageErr("--tile %d outside [%d, %d]", *tile, minSweepTile, maxSweepTile)
	}
	seeds, err := parseSeeds(*seedsFlag)
	if err != nil {
		return usageErr("--seeds: %v", err)
	}
	registry := pipeline.Stages()
	cols, err := parseSweepStages(*stagesFlag, registry)
	if err != nil {
		return usageErr("--stage: %v", err)
	}
	var aspects, presets []string
	if isSet(fs, "aspect") {
		if aspects, err = parseList(*aspectsFlag, maxSweepAspects); err != nil {
			return usageErr("--aspect: %v", err)
		}
	}
	if isSet(fs, "preset") {
		if presets, err = parseList(*presetsFlag, maxSweepPresets); err != nil {
			return usageErr("--preset: %v", err)
		}
	}

	fail := func(format string, args ...any) int {
		fmt.Fprintf(stderr, "mpg sweep: "+format+"\n", args...)
		return 1
	}
	last, err := checkImplemented(cols, registry)
	if err != nil {
		return fail("%v", err)
	}

	// Resolve the base config and every row's config before doing work.
	base, err := cf.load()
	if err != nil {
		return fail("%v", err)
	}
	if err := base.Resolve(); err != nil {
		return fail("%v", err)
	}
	baseHash, err := base.Hash()
	if err != nil {
		return fail("%v", err)
	}
	if aspects == nil {
		aspects = []string{base.World.Aspect}
	}
	if presets == nil {
		presets = []string{base.Layout.Preset}
	}
	type sweepRow struct {
		cfg  config.Config
		hash string
	}
	var rows []sweepRow
	for _, aspect := range aspects {
		for _, preset := range presets {
			for _, seed := range seeds {
				cfg := base
				cfg.Seed = config.Seed(seed)
				if isSet(fs, "aspect") {
					setAspect(&cfg, aspect)
				}
				cfg.Layout.Preset = preset
				if err := cfg.Resolve(); err != nil {
					return fail("aspect %s, preset %s: %v", aspect, preset, err)
				}
				hash, err := cfg.Hash()
				if err != nil {
					return fail("%v", err)
				}
				rows = append(rows, sweepRow{cfg, hash})
			}
		}
	}

	// The pipeline needs an output directory for config.json; sweep keeps
	// nothing from it.
	workDir, err := os.MkdirTemp("", "mpg-sweep-")
	if err != nil {
		return fail("%v", err)
	}
	defer os.RemoveAll(workDir)

	sheet := render.Sheet{TileWidth: *tile, CaptionLines: captionLines}
	for _, c := range cols {
		sheet.Columns = append(sheet.Columns, c.name)
	}
	start := time.Now()
	for n, row := range rows {
		t0 := time.Now()
		w := row.cfg.World
		tileH := max(1, int(math.Round(float64(*tile)*w.HeightKm/w.WidthKm)))
		tiles := make([]render.SheetTile, len(cols))
		size := make([]image.Point, len(cols)) // source render sizes
		keep := func(k int, img image.Image) {
			tiles[k].Image = render.Downscale(img, *tile, tileH)
			size[k] = img.Bounds().Size()
		}
		ctx, err := pipeline.NewContext(row.cfg, workDir, "", stderr)
		if err != nil {
			return fail("%v", err)
		}
		ctx.Sink = func(st pipeline.Stage, variant string, img image.Image) error {
			for k, c := range cols {
				if registry[c.index].Name == st.Name && c.variant == variant && tiles[k].Image == nil {
					keep(k, img)
				}
			}
			return nil
		}
		res, err := pipeline.Run(ctx, registry, last)
		if err != nil {
			return fail("seed %d: %v", uint64(row.cfg.Seed), err)
		}
		if res.NotImplemented != nil {
			return fail("stage %s is not implemented yet", res.NotImplemented)
		}
		for k, c := range cols {
			tiles[k].Caption = []string{
				fmt.Sprintf("seed %d %s", uint64(row.cfg.Seed), c.name),
				fmt.Sprintf("%.0f x %.0f km", w.WidthKm, w.HeightKm),
			}
			if tiles[k].Image != nil {
				tiles[k].Caption[1] += fmt.Sprintf(", %dx%d px", size[k].X, size[k].Y)
			}
			if sl := ctx.Products.SeaLevel; sl != nil && registry[c.index].Name == "sea-level" {
				tiles[k].Caption[1] = fmt.Sprintf("land %d/%d %+.2f%% %s", sl.LandCells, sl.Target, sl.DeviationPercent(), sl.Reason)
			}
			if cr := ctx.Products.Climate; cr != nil && registry[c.index].Name == "climate" {
				tiles[k].Caption[1] = climateCaption(ctx.Products.Mesh.Cells, cr, c.variant)
			}
			if br := ctx.Products.Basins; br != nil && registry[c.index].Name == "basins" {
				tiles[k].Caption[1] = basinsCaption(br)
				if lk := ctx.Products.Lakes; lk != nil && c.variant == "lakes" {
					tiles[k].Caption[1] = lakesCaption(lk)
				}
			}
			if lt := ctx.Products.Target; lt != nil && registry[c.index].Name == "land-target" {
				tiles[k].Caption[1] = targetCaption(lt)
				switch c.variant {
				case "lakes":
					tiles[k].Caption[1] = lakesCaption(lt.Lakes)
				case "precip", "aridity":
					tiles[k].Caption[1] = climateCaption(ctx.Products.Mesh.Cells, lt.Climate, c.variant)
				}
			}
			if cl := ctx.Products.Classes; cl != nil && registry[c.index].Name == "classify" {
				tiles[k].Caption[1] = classifyCaption(cl, c.variant)
			}
			if es := ctx.Products.EdgeStats; es != nil && registry[c.index].Name == "edges" {
				rev := 0.0
				if es.Pairs > 0 {
					rev = 100 * float64(es.NotOpposite) / float64(es.Pairs)
				}
				tiles[k].Caption[1] = fmt.Sprintf("err %.0f/%.0f/%.0f rev %.1f%% coast %.2f", es.ErrorMean, es.ErrorP95, es.ErrorMax, rev, es.CoastPerLand())
			}
			if ms := ctx.Products.Measures; ms != nil && registry[c.index].Name == "measures" {
				tiles[k].Caption[1] = measuresCaption(ms)
			}
		}
		label := []string{fmt.Sprintf("seed %d", uint64(row.cfg.Seed)), w.Aspect, row.cfg.Layout.Preset, fmt.Sprintf("%d land", w.LandCells)}
		if lt := ctx.Products.Target; lt != nil {
			label[3] = fmt.Sprintf("land %d/%d", lt.LandCells, lt.Search.Target)
			if !lt.Met() {
				label[3] += " UNMET"
			}
		} else if sl := ctx.Products.SeaLevel; sl != nil {
			label[3] = fmt.Sprintf("land %d/%d", sl.LandCells, sl.Target)
			if !sl.Met {
				label[3] += " UNMET"
			}
		}
		if ms := ctx.Products.Measures; ms != nil && !ms.Pass {
			label[3] += " GATE"
		}
		for _, l := range label {
			sheet.LabelChars = min(max(sheet.LabelChars, len(l)), maxLabelChars)
		}
		sheet.Rows = append(sheet.Rows, render.SheetRow{Label: label, TileHeight: tileH, Tiles: tiles})
		fmt.Fprintf(stderr, "sweep: [%d/%d] seed %d %s %s: %.1fs\n", n+1, len(rows), uint64(row.cfg.Seed), w.Aspect, row.cfg.Layout.Preset, time.Since(t0).Seconds())
	}

	img := sheet.Image()
	seedText := make([]string, len(seeds))
	for k, s := range seeds {
		seedText[k] = strconv.FormatUint(s, 10)
	}
	var rowText []string
	for _, row := range rows {
		rowText = append(rowText, fmt.Sprintf("%d %s %s %s", uint64(row.cfg.Seed), row.cfg.World.Aspect, row.cfg.Layout.Preset, row.hash))
	}
	meta := render.Meta{
		Stage:      "sweep",
		ConfigHash: baseHash,
		Extra: []render.Text{
			{Key: "mpg:seeds", Value: strings.Join(seedText, ",")},
			{Key: "mpg:stages", Value: strings.Join(sheet.Columns, ",")},
			{Key: "mpg:aspects", Value: strings.Join(aspects, ",")},
			{Key: "mpg:presets", Value: strings.Join(presets, ",")},
			{Key: "mpg:tile-width", Value: strconv.Itoa(*tile)},
			{Key: "mpg:rows", Value: strings.Join(rowText, "\n")},
		},
	}
	if dir := filepath.Dir(*output); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fail("%v", err)
		}
	}
	if err := render.WritePNGFile(*output, img, meta); err != nil {
		return fail("%v", err)
	}
	fmt.Fprintf(stdout, "sheet   %s (%d x %d px)\n", *output, img.Rect.Dx(), img.Rect.Dy())
	fmt.Fprintf(stdout, "config  %s\n", baseHash)
	fmt.Fprintf(stdout, "seeds   %d: %s\n", len(seeds), *seedsFlag)
	fmt.Fprintf(stdout, "stages  %s\n", strings.Join(sheet.Columns, ", "))
	fmt.Fprintf(stdout, "aspects %s\n", strings.Join(aspects, ", "))
	fmt.Fprintf(stdout, "presets %s\n", strings.Join(presets, ", "))
	fmt.Fprintf(stdout, "pixels  %s\n", render.PixelHash(img))
	fmt.Fprintf(stderr, "sweep: %d rows x %d stages in %.1fs\n", len(rows), len(cols), time.Since(start).Seconds())
	return 0
}

// parseSeeds parses a comma-separated list of decimal uint64 seeds and
// inclusive ranges lo-hi, as in "1-16" or "1,5,9-12", in the order given.
// Empty items, reversed ranges, repeated seeds, and more than maxSweepSeeds
// seeds in all are errors.
func parseSeeds(s string) ([]uint64, error) {
	var seeds []uint64
	seen := map[uint64]bool{}
	for item := range strings.SplitSeq(s, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			return nil, fmt.Errorf("empty item in %q", s)
		}
		los, his, isRange := strings.Cut(item, "-")
		lo, err := parseSeed(los)
		if err != nil {
			return nil, fmt.Errorf("%q: %v", item, err)
		}
		hi := lo
		if isRange {
			if hi, err = parseSeed(his); err != nil {
				return nil, fmt.Errorf("%q: %v", item, err)
			}
			if hi < lo {
				return nil, fmt.Errorf("range %q is reversed", item)
			}
		}
		if hi-lo >= uint64(maxSweepSeeds-len(seeds)) {
			return nil, fmt.Errorf("more than %d seeds", maxSweepSeeds)
		}
		for v := lo; ; v++ {
			if seen[v] {
				return nil, fmt.Errorf("seed %d repeated", v)
			}
			seen[v] = true
			seeds = append(seeds, v)
			if v == hi {
				break
			}
		}
	}
	return seeds, nil
}

// parseList splits a comma-separated list, rejecting empty and repeated
// items and more than limit of them.
func parseList(s string, limit int) ([]string, error) {
	var items []string
	for item := range strings.SplitSeq(s, ",") {
		item = strings.TrimSpace(item)
		switch {
		case item == "":
			return nil, fmt.Errorf("empty item in %q", s)
		case len(items) == limit:
			return nil, fmt.Errorf("more than %d items", limit)
		case slices.Contains(items, item):
			return nil, fmt.Errorf("%q repeated", item)
		}
		items = append(items, item)
	}
	return items, nil
}

// sweepStage is one sheet column.
type sweepStage struct {
	name    string // header: "elevation" or "mesh:area"
	index   int    // index in the registry
	variant string // render variant of a registry stage, "" for the main one
}

var kebabRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// parseSweepStages parses --stage: registry stage names or numbers, each
// optionally with ":variant" to pick a render variant.
// It does not check that the stages are implemented; see checkImplemented.
func parseSweepStages(s string, registry []pipeline.Stage) ([]sweepStage, error) {
	items, err := parseList(s, 32)
	if err != nil {
		return nil, err
	}
	var cols []sweepStage
	for _, item := range items {
		name, variant, hasVariant := strings.Cut(item, ":")
		if hasVariant && !kebabRE.MatchString(variant) {
			return nil, fmt.Errorf("variant %q in %q is not lowercase kebab case", variant, item)
		}
		i, err := pipeline.Lookup(registry, name)
		if err != nil {
			return nil, err
		}
		col := sweepStage{name: registry[i].Name, index: i, variant: variant}
		if hasVariant {
			col.name += ":" + variant
		}
		for _, c := range cols {
			if c.name == col.name {
				return nil, fmt.Errorf("stage %s repeated", col.name)
			}
		}
		cols = append(cols, col)
	}
	return cols, nil
}

// checkImplemented returns the registry index of the last pipeline stage the
// columns need, or an error when a column's stage is not implemented yet,
// or a stage before the last is neither implemented nor deferred.
func checkImplemented(cols []sweepStage, registry []pipeline.Stage) (int, error) {
	last := -1
	for _, c := range cols {
		last = max(last, c.index)
		if st := registry[c.index]; !st.Implemented() {
			return -1, fmt.Errorf("stage %s is not implemented yet", st)
		}
	}
	for _, st := range registry[:last+1] {
		if !st.Implemented() && !st.Deferred {
			return -1, fmt.Errorf("stage %s is not implemented yet; sweep cannot run through stage %s", st, registry[last])
		}
	}
	return last, nil
}

// climateCaption returns a climate tile's caption for render variant
// variant (see runSweep).
func climateCaption(cells []mesh.Cell, r *climate.Result, variant string) string {
	land := func(v []float64) []float64 {
		var out []float64
		for i, c := range cells {
			if !c.Rim && !r.Ocean[i] {
				out = append(out, v[i])
			}
		}
		slices.Sort(out)
		return out
	}
	q := func(v []float64, p int) float64 { return v[max(0, (len(v)*p+99)/100-1)] } // nearest rank
	switch variant {
	case "precip", "pet", "runoff":
		v := land(map[string][]float64{"precip": r.Precipitation, "pet": r.PET, "runoff": r.Runoff}[variant])
		if len(v) == 0 {
			return "no land"
		}
		name := map[string]string{"precip": "P", "pet": "PET", "runoff": "R"}[variant]
		return fmt.Sprintf("land %s %.0f/%.0f/%.0f mm", name, q(v, 5), q(v, 50), q(v, 95))
	case "moisture":
		v := land(r.Moisture)
		if len(v) == 0 {
			return "no land"
		}
		return fmt.Sprintf("land moisture %.2f/%.2f/%.2f", q(v, 10), q(v, 50), q(v, 90))
	case "aridity":
		v := land(r.Aridity)
		if len(v) == 0 {
			return "no land"
		}
		var n [climate.NumAridity]int
		for _, a := range v {
			n[climate.AridityOf(a)]++
		}
		parts := make([]string, climate.NumAridity)
		for a, name := range []string{"ha", "a", "sa", "ds", "hu"} {
			parts[a] = fmt.Sprintf("%s%.0f", name, 100*float64(n[a])/float64(len(v)))
		}
		return strings.Join(parts, " ") + " %"
	}
	rlo, rhi := math.Inf(1), math.Inf(-1)
	var temps []float64
	for i, c := range cells {
		t := r.Temperature[i]
		switch {
		case c.Rim:
			rlo, rhi = min(rlo, t), max(rhi, t)
		case !r.Ocean[i]:
			temps = append(temps, t)
		}
	}
	s := fmt.Sprintf("rim %.0f..%.0f", rlo, rhi)
	if len(temps) > 0 {
		slices.Sort(temps)
		s += fmt.Sprintf(" land %.0f..%.0f ~%.0f", temps[0], temps[len(temps)-1], temps[len(temps)/2])
	}
	return s + " C"
}

// targetCaption summarizes a land-target search for a land-target sweep
// tile: land after lakes against N, the deviation, the reason, the probes
// (p), and the lake cells (lk).
func targetCaption(t *pipeline.LandTarget) string {
	r := t.Search
	return fmt.Sprintf("land %d/%d %+.2f%% %s p%d lk %d", t.LandCells, r.Target, r.DeviationPercent(), r.Reason, len(r.Trace), t.LakeCells)
}

// lakesCaption summarizes a water balance for a basins:lakes sweep tile:
// lakes (lk), inland seas (sea), the salt ones among both (salt), playas
// (pl), full basins (full), and lake cells (cells).
func lakesCaption(l *basin.Lakes) string {
	lakes, seas, salt, full := l.Counts()
	return fmt.Sprintf("lk %d sea %d salt %d pl %d full %d cells %d", lakes, seas, salt, len(l.Playas), full, l.Cells())
}

// basinsCaption summarizes a basin hierarchy for a sweep tile: depressions,
// basins, deepest nesting, and cells in basins.
func basinsCaption(r *basin.Result) string {
	nest, cells := 0, 0
	for b := range r.Basins {
		nest = max(nest, r.Nesting(b))
	}
	for _, b := range r.Of {
		if b != basin.None {
			cells++
		}
	}
	return fmt.Sprintf("dep %d bas %d nest %d cells %d", len(r.Depressions), len(r.Basins), nest, cells)
}

// classifyCaption returns a classify tile's caption for render variant
// variant (see runSweep).
func classifyCaption(cl *classify.Result, variant string) string {
	switch variant {
	case "biome":
		counts, total := cl.BiomeHistogram()
		order := make([]int, len(counts))
		for k := range order {
			order[k] = k
		}
		slices.SortStableFunc(order, func(a, b int) int { return cmp.Compare(counts[b], counts[a]) })
		var parts []string
		for _, k := range order[:min(4, len(order))] {
			if counts[k] > 0 && total > 0 {
				parts = append(parts, fmt.Sprintf("%s%.0f", classify.Biomes[k].Abbrev(), 100*float64(counts[k])/float64(total)))
			}
		}
		return strings.Join(parts, " ")
	case "surface":
		counts, _ := cl.BiomeHistogram()
		return fmt.Sprintf("gl %d if %d pd %d pk %d", cl.SurfaceCount(classify.Glacier), cl.SurfaceCount(classify.IceField),
			counts[classify.PolarDesert-classify.Clear], cl.SurfaceCount(classify.PackIce))
	case "wetness":
		_, total := cl.BiomeHistogram()
		pct := 0.0
		if total > 0 {
			pct = 100 * float64(cl.WetlandCount()) / float64(total)
		}
		return fmt.Sprintf("wet %d (%.1f%%)", cl.WetlandCount(), pct)
	}
	return fmt.Sprintf("%s v%d", strings.Join(cl.LandShares(true), " "), cl.Volcanoes())
}

// measuresCaption summarizes the checks for a measures sweep tile: the
// checks that passed against those run (ok), the failed report-only checks
// (rpt), and the failed gates (gate).
func measuresCaption(m *world.Measures) string {
	failed := m.GatesFailed + m.ReportsFailed
	return fmt.Sprintf("ok %d/%d rpt %d gate %d", len(m.Checks)-failed, len(m.Checks), m.ReportsFailed, m.GatesFailed)
}
