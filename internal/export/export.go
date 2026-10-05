// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package export

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"

	"github.com/mdhender/mpg/internal/cells"
	"github.com/mdhender/mpg/internal/classify"
	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/edges"
	"github.com/mdhender/mpg/internal/mesh"
	"github.com/mdhender/mpg/internal/seed"
	"github.com/mdhender/mpg/internal/topo"
	"github.com/mdhender/mpg/world"
)

// EdgeNoiseStream and EdgeNoiseVersion name the seed stream the per-edge
// noise seeds are drawn from (package seed).
const (
	EdgeNoiseStream  = "edge-noise"
	EdgeNoiseVersion = "1"
)

// Input holds the stage products the export reads.
type Input struct {
	// Config is the resolved config and ConfigHash its hash.
	Config     config.Config
	ConfigHash string
	Mesh       *mesh.Mesh
	Cells      *cells.Stats
	SeaLevel   *cells.SeaLevel
	Classes    *classify.Result
	Edges      *edges.Data
	// Deferred names the pipeline stages the run passed over, in order.
	Deferred []string
}

// Build converts the stage products into the world.json types. See the
// package documentation for the rules.
func Build(in Input) (*world.World, error) {
	m := in.Mesh
	if m == nil || in.Cells == nil || in.SeaLevel == nil || in.Classes == nil || in.Edges == nil {
		return nil, errors.New("export: missing a stage product")
	}
	n := len(m.Cells)
	if in.Cells.Len() != n || len(in.SeaLevel.Land) != n || len(in.Classes.Landform) != n ||
		len(in.Edges.Cells) != n || len(in.Edges.Edges) != len(m.Edges) {
		return nil, errors.New("export: stage products disagree on the mesh size")
	}
	cyl := m.Cylinder()
	b := &builder{in: in, m: m, cyl: cyl}
	b.corners()
	b.cells()
	b.edges()
	if err := b.coastlines(); err != nil {
		return nil, err
	}
	cfg := in.Config
	b.w.Schema = world.SchemaVersion
	b.w.Meta = world.Meta{
		Generator:             "mpg",
		Seed:                  strconv.FormatUint(uint64(cfg.Seed), 10),
		ConfigHash:            in.ConfigHash,
		WidthKm:               cyl.W(),
		HeightKm:              cyl.H(),
		Wrap:                  "east-west",
		Origin:                "northwest",
		RimCells:              cfg.Rim.Cells,
		RimKm:                 cfg.Rim.Km,
		HexFlatToFlatMi:       cfg.Province.HexFlatToFlatMi,
		ProvinceAreaKm2:       cfg.Province.AreaKm2,
		CoordinatePrecisionKm: world.PrecisionKm,
		Units:                 world.DefaultUnits(),
	}
	b.w.Codebooks = world.DefaultCodebooks()
	b.w.Outcomes = outcomes(in.SeaLevel, in.Deferred)
	b.w.Rivers = []world.RiverPath{}
	return &b.w, nil
}

type builder struct {
	in  Input
	m   *mesh.Mesh
	cyl topo.Cylinder
	w   world.World
}

// scale is 1 / world.PrecisionKm.
const scale = 1e6

// round returns v rounded to the nearest multiple of world.PrecisionKm,
// with no negative zero. The product is correctly rounded and feeds no
// addition, so the result is the same on every architecture.
func round(v float64) float64 {
	r := math.Round(v*scale) / scale
	if r == 0 {
		return 0
	}
	return r
}

// roundAngle returns degrees rounded to 0.01°, with no negative zero.
func roundAngle(v float64) float64 {
	r := math.Round(v*100) / 100
	if r == 0 {
		return 0
	}
	return r
}

// wrapX returns x rounded and kept in [0, W): a value that rounds up to W
// wraps to 0.
func (b *builder) wrapX(x float64) float64 {
	r := round(x)
	if r >= b.cyl.W() {
		r = round(r - b.cyl.W())
	}
	return r
}

// point returns p rounded, x kept in [0, W). A y of exactly 0 or H (the
// rim boundary) is kept exactly; any other y is rounded and held in
// [0, H].
func (b *builder) point(p topo.Point) world.Point {
	y := p.Y
	if y != 0 && y != b.cyl.H() {
		y = min(max(round(y), 0), b.cyl.H())
	}
	return world.Point{X: b.wrapX(p.X), Y: y}
}

func (b *builder) isLand(c int) bool { return c != mesh.Boundary && b.in.SeaLevel.Land[c] }

func (b *builder) corners() {
	m, alt := b.m, b.in.Cells.Altitude
	b.w.Corners = make([]world.Corner, len(m.Corners))
	for i, k := range m.Corners {
		var sum float64
		land, wet := false, false
		for _, c := range k.Cells {
			sum += alt[c]
			if b.isLand(c) {
				land = true
			} else {
				wet = true // playable water or rim
			}
		}
		var flags []world.CornerFlag
		if k.Boundary {
			flags = append(flags, world.FlagBoundary)
		}
		if land && wet {
			flags = append(flags, world.FlagTerminal)
		}
		b.w.Corners[i] = world.Corner{
			ID:      i,
			Point:   b.point(k.Point),
			HeightM: sum / float64(len(k.Cells)),
			Flags:   flags,
			Cells:   slices.Clone(k.Cells),
			Edges:   slices.Clone(k.Edges),
		}
	}
}

func (b *builder) cells() {
	m, in := b.m, b.in
	b.w.Cells = make([]world.Cell, len(m.Cells))
	for i, c := range m.Cells {
		site := b.point(c.Site)
		poly := make([]world.Point, len(c.Corners))
		box := world.BBox{
			Min: world.Point{X: math.Inf(1), Y: math.Inf(1)},
			Max: world.Point{X: math.Inf(-1), Y: math.Inf(-1)},
		}
		coast := false
		for k, id := range c.Corners {
			q := b.w.Corners[id].Point
			off := world.Point{X: round(b.cyl.DX(site.X, q.X)), Y: round(q.Y - site.Y)}
			poly[k] = off
			x, y := site.X+off.X, site.Y+off.Y
			box.Min.X, box.Max.X = min(box.Min.X, x), max(box.Max.X, x)
			box.Min.Y, box.Max.Y = min(box.Min.Y, y), max(box.Max.Y, y)
			coast = coast || in.Edges.Edges[c.Edges[k]].Coast
		}
		box.Min.X, box.Min.Y, box.Max.X, box.Max.Y = round(box.Min.X), round(box.Min.Y), round(box.Max.X), round(box.Max.Y)
		var flags []world.CellFlag
		if c.Rim {
			flags = append(flags, world.FlagRim)
		}
		if c.Impassable {
			flags = append(flags, world.FlagImpassable)
		}
		if in.Classes.Volcano[i] {
			flags = append(flags, world.FlagVolcano)
		}
		if coast {
			flags = append(flags, world.FlagCoast)
		}
		var water world.Water
		if !c.Rim && in.SeaLevel.Ocean[i] {
			water = world.Ocean
		}
		hs := in.Edges.Cells[i]
		exits := make([]world.Exit, len(hs))
		for k, h := range hs {
			bearing := roundAngle(h.Bearing)
			if bearing >= 360 {
				bearing = 0
			}
			exits[k] = world.Exit{
				Direction:       world.Direction(h.Direction.String()),
				Neighbor:        h.Neighbor,
				Edge:            h.Edge,
				InclinePermille: int(h.Incline),
				BearingDeg:      bearing,
				ErrorDeg:        roundAngle(h.Error),
			}
		}
		depth := world.Depth(in.Classes.Depth[i].String())
		b.w.Cells[i] = world.Cell{
			ID:        i,
			Landform:  world.Landform(in.Classes.Landform[i].String()),
			Depth:     depth,
			Water:     water,
			AltitudeM: in.Cells.Altitude[i],
			Flags:     flags,
			Site:      site,
			Centroid:  b.point(m.Centroid(i)),
			Corners:   slices.Clone(c.Corners),
			Sides:     slices.Clone(c.Edges),
			Polygon:   poly,
			BBox:      box,
			Exits:     exits,
		}
	}
}

func (b *builder) edges() {
	m, in := b.m, b.in
	r := seed.Rand(uint64(in.Config.Seed), EdgeNoiseStream, EdgeNoiseVersion)
	b.w.Edges = make([]world.Edge, len(m.Edges))
	for i, e := range m.Edges {
		d := in.Edges.Edges[i]
		p, q := b.w.Corners[e.Corners[0]].Point, b.w.Corners[e.Corners[1]].Point
		b.w.Edges[i] = world.Edge{
			ID:              i,
			Cells:           e.Cells,
			Corners:         e.Corners,
			LengthKm:        round(b.cyl.Distance(topo.Point{X: p.X, Y: p.Y}, topo.Point{X: q.X, Y: q.Y})),
			Seed:            r.Uint32(),
			Passable:        d.Passable,
			Coast:           d.Coast,
			Water:           world.Water(d.Water.String()),
			River:           world.RiverClass(d.River.String()),
			InclinePermille: int(d.Incline),
		}
	}
}

// coastlines chains the coast edges. Each coast edge is walked with its
// land cell on the right; the edge after it leaves its end corner along
// the land: the land cell's next side clockwise, or, where that side leads
// to another land cell, that cell's next side, and so on around the corner
// until a side has water across it. Land that meets only at a corner is
// therefore kept apart, as cell adjacency keeps it. A side against the rim
// or the map edge ends the chain, leaving it open.
func (b *builder) coastlines() error {
	m, data := b.m, b.in.Edges
	const none = -1
	next := make([]int, len(m.Edges))
	from := make([]int, len(m.Edges))
	preds := make([]int, len(m.Edges))
	var coast []int
	for e, me := range m.Edges {
		next[e] = none
		if !data.Edges[e].Coast {
			continue
		}
		coast = append(coast, e)
		land, end := me.Cells[0], me.Corners[1]
		from[e] = me.Corners[0]
		if !b.isLand(land) {
			land, end, from[e] = me.Cells[1], me.Corners[0], me.Corners[1]
		}
		nx, err := b.nextCoast(land, end)
		if err != nil {
			return fmt.Errorf("export: coast edge %d: %w", e, err)
		}
		next[e] = nx
		if nx != none {
			if !data.Edges[nx].Coast {
				return fmt.Errorf("export: coast edge %d leads to edge %d, which is not a coast", e, nx)
			}
			preds[nx]++
		}
	}
	end := func(e int) int { // the corner a coast edge walks to
		if from[e] == m.Edges[e].Corners[0] {
			return m.Edges[e].Corners[1]
		}
		return m.Edges[e].Corners[0]
	}
	done := make([]bool, len(m.Edges))
	chain := func(start int) world.Coastline {
		cl := world.Coastline{Corners: []int{from[start]}}
		for e := start; e != none && !done[e]; e = next[e] {
			done[e] = true
			cl.Edges = append(cl.Edges, e)
			cl.Corners = append(cl.Corners, end(e))
		}
		cl.Closed = cl.Corners[len(cl.Corners)-1] == cl.Corners[0]
		return cl
	}
	var out []world.Coastline
	for _, e := range coast {
		if preds[e] > 1 {
			return fmt.Errorf("export: coast edge %d follows %d coast edges", e, preds[e])
		}
		if preds[e] == 0 {
			out = append(out, chain(e))
		}
	}
	for _, e := range coast {
		if !done[e] {
			out = append(out, chain(e))
		}
	}
	for _, cl := range out {
		if last := cl.Edges[len(cl.Edges)-1]; next[last] != none && !cl.Closed {
			return fmt.Errorf("export: coastline from edge %d breaks at edge %d", cl.Edges[0], last)
		}
	}
	slices.SortFunc(out, func(a, b world.Coastline) int { return a.Edges[0] - b.Edges[0] })
	if out == nil {
		out = []world.Coastline{}
	}
	b.w.Coastlines = out
	return nil
}

// nextCoast returns the coast edge that leaves corner k with land on its
// right, hugging land cell c, whose side ends at k, or −1 when the walk
// reaches the rim or the map edge.
func (b *builder) nextCoast(c, k int) (int, error) {
	m := b.m
	for range 8 {
		cell := &m.Cells[c]
		j := slices.Index(cell.Corners, k)
		if j < 0 {
			return 0, fmt.Errorf("cell %d does not pass through corner %d", c, k)
		}
		e := cell.Edges[j]
		o := m.Edges[e].Other(c)
		switch {
		case o == mesh.Boundary || m.Cells[o].Rim:
			return -1, nil
		case !b.isLand(o):
			return e, nil
		}
		c = o
	}
	return 0, fmt.Errorf("no way round corner %d", k)
}

// outcomes returns the sea level search's outcomes.
func outcomes(sl *cells.SeaLevel, deferred []string) world.Outcomes {
	trace := make([]world.Probe, len(sl.Trace))
	for k, p := range sl.Trace {
		trace[k] = world.Probe{Method: p.Method.String(), LevelM: p.Level, Land: p.Land, Ocean: p.Ocean, DryBasin: p.Basin}
	}
	return world.Outcomes{
		SeaLevelM:        sl.Level,
		InitialEstimateM: sl.Initial,
		TargetLandCells:  sl.Target,
		ToleranceCells:   sl.Tolerance,
		TolerancePercent: cells.TolerancePercent,
		PlayableCells:    sl.Playable,
		LandCells:        sl.LandCells,
		OceanCells:       sl.OceanCells,
		DryBasinCells:    sl.BasinCells,
		LandAreaKm2:      sl.LandAreaKm2,
		Met:              sl.Met,
		Reason:           string(sl.Reason),
		Policy:           sl.Policy,
		Budget:           sl.Budget,
		Trace:            trace,
		Deferred:         append([]string{}, deferred...),
	}
}
