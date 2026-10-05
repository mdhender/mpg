// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package world

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

// PrecisionKm is the grid every position, offset, length, and bounding box
// in the file is rounded to: 1 mm. Validate compares geometry within it.
const PrecisionKm = 1e-6

// AngleToleranceDeg is how far Validate lets a bearing or direction error
// differ from what it recomputes; the file rounds them to 0.01°.
const AngleToleranceDeg = 0.011

// HeightToleranceM is how far Validate lets a corner height differ from the
// mean of its cells' altitudes.
const HeightToleranceM = 1e-6

// MaxIncline is the cap on an incline's magnitude, in tenths of a percent:
// 100%.
const MaxIncline = 1000

// MaxExits is the most neighbors a cell may have: one per compass point.
const MaxExits = 8

// Reasons lists the sea-level search's termination reasons.
var Reasons = []string{"exact", "within-tolerance", "unreachable", "budget-exhausted"}

// Methods lists the sea-level search's probe methods.
var Methods = []string{"estimate", "newton", "bisect", "gallop"}

// maxProblems limits how many problems an Invalid error lists.
const maxProblems = 50

// Invalid is the error Validate returns: the first problems found, in the
// order checked, and the total count.
type Invalid struct {
	Problems []string
	Count    int
}

// Error lists the problems one per line.
func (e *Invalid) Error() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "world: %d problem(s)", e.Count)
	for _, p := range e.Problems {
		sb.WriteString("\n  ")
		sb.WriteString(p)
	}
	if e.Count > len(e.Problems) {
		fmt.Fprintf(&sb, "\n  … and %d more", e.Count-len(e.Problems))
	}
	return sb.String()
}

// Validate checks the structural invariants of w and returns nil, or an
// *Invalid listing the problems. It checks:
//
//   - the schema version, the metadata (generator, seed, config hash, sizes,
//     wrap, origin, precision, units) and that the codebooks are this
//     package's;
//   - ids: every id equals its index, and every reference is in range;
//   - every coded value is in its codebook, every number finite, every
//     position inside the map (x in [0, W), y in [0, H]);
//   - cells: landform, depth, and water agree (salt water has a depth and
//     is ocean or inland sea, fresh water is lake, land has neither);
//     flags in codebook order without repeats; rim if and only if
//     impassable; rim cells deep salt water with no water kind and no
//     salt flag; volcanoes on land; the coast flag exactly on cells with a
//     coast edge; the salt flag only on lake and inland-sea cells; the
//     playa flag only on playable land; biome and surface in their
//     codebooks; a biome on exactly the playable land cells; biome clear
//     exactly under a glacier or ice-field surface; glacier, ice-field and
//     wetland surfaces only on land, wetlands only on flats or plains
//     (except a playa's salt-flats), mangroves only with the coast flag;
//     pack-ice only on playable water; no biome or surface on the rim;
//     every playa salt-flats;
//   - polygons: at least 3 corners, distinct, starting at the lowest id;
//     Sides[k] joins Corners[k] to Corners[k+1] in the orientation its
//     edge records for this cell, so the polygon is closed; each offset
//     leads from the site to its corner modulo W; the offsets have positive
//     area (clockwise on the map, north up); the bounding box is the
//     polygon's;
//   - exits: at most MaxExits, directions in strictly clockwise compass
//     order (so unique), bearings in cyclic clockwise order, one exit for
//     every side that is not on the rim boundary; each exit's edge joins
//     the cell to its neighbor, its incline is the edge's (negated from
//     Cells[1]), its bearing matches the sites and its error the bearing;
//     neighbor symmetry: A lists B through edge e if and only if B lists A
//     through e, with the negated incline; inland water (lake and inland
//     sea) never neighbors the ocean, and neighboring inland-water cells
//     (one lake, since two lakes never touch) share the water kind and
//     salt flag;
//   - corners: the boundary flag exactly on y = 0 or y = H; 3–4 cells (2–4
//     on the boundary), ascending, exactly the cells whose polygons list
//     the corner; edges ascending, exactly the edges that end there; the
//     height is the mean of its cells' altitudes; sink only on corners of
//     a playa cell, and every playa cell's lowest corner (height, then id)
//     a sink; terminal exactly on sink corners and corners touching both
//     land and water or rim; mouth only on terminal corners;
//   - edges: cells ordered (Cells[1] == Boundary on the rim boundary, whose
//     cell is a rim cell and whose corners are boundary corners); each
//     edge is a side of exactly its cells; the length is the wrapped
//     distance between its corners; passable exactly when neither side is
//     the boundary or a rim cell; coast exactly when one side is water and
//     neither is rim, with that side's water kind; rivers only between land
//     cells; the incline within ±MaxIncline, 0 on the boundary, and within
//     one tenth of a percent of the grade recomputed from the altitudes and
//     sites;
//   - coastlines: every coast edge in exactly one chain, once; each chain
//     connected corner to corner with land on its right; closed exactly
//     when it returns to its first corner, then starting at its lowest
//     edge id; chains ordered by first edge id;
//   - rivers: chains of land–land edges whose classes match the edges',
//     corner to corner with no corner twice, classes never falling
//     downstream, ordered by first edge id; every edge with a river class
//     on exactly one chain, and none with a corner touching a rim cell;
//     the mouth flag exactly on the corners where a chain ends and no
//     chain continues (a chain ends at a mouth or at a confluence, a
//     corner inside another chain);
//   - outcomes: the playable, land, ocean, dry basin, lake and inland-sea
//     cell counts match the cells; ocean cells lie at or below the sea
//     level and dry basin floors are the land at or below it; the lake,
//     inland-sea, salt and playa counts match the cells (lakes counted as
//     connected sets of cells), and so do the glacier, ice-field, pack-ice
//     and wetland counts; the biome table is named; met agrees with the
//     target and tolerance; the land area is the land polygons' area; the
//     reason, policy,
//     budget, expected lake cells, climate passes, and trace are well
//     formed.
func Validate(w *World) error {
	v := &validator{w: w}
	v.run()
	if v.count == 0 {
		return nil
	}
	return &Invalid{Problems: v.problems, Count: v.count}
}

type validator struct {
	w        *World
	problems []string
	count    int
	// wd and ht are W and H when they are usable, else 0.
	wd, ht float64
}

func (v *validator) bad(format string, args ...any) {
	v.count++
	if len(v.problems) < maxProblems {
		v.problems = append(v.problems, fmt.Sprintf(format, args...))
	}
}

func finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }

// mul returns a*b rounded to float64, so it cannot fuse into a following
// addition.
func mul(a, b float64) float64 { return float64(a * b) }

// dx returns the east–west displacement from a to b the shorter way around
// a cylinder of circumference wd.
func dx(a, b, wd float64) float64 {
	d := b - a
	if d > wd/2 {
		d -= wd
	} else if d < -wd/2 {
		d += wd
	}
	return d
}

// angleDiff returns the angle between a and b in degrees, the smaller way
// around, in [0, 180].
func angleDiff(a, b float64) float64 {
	d := math.Mod(math.Abs(a-b), 360)
	return min(d, 360-d)
}

// bearing returns the bearing in degrees clockwise from north, in
// [0, 360), of a displacement (x east, y south).
func bearing(x, y float64) float64 {
	deg := mul(math.Atan2(x, -y), 180/math.Pi)
	if deg < 0 {
		deg += 360
	}
	if deg >= 360 {
		deg = 0
	}
	return deg
}

// shoelace returns the area of the polygon q: positive when it runs
// clockwise on the map (x east, y south).
func shoelace(q []Point) float64 {
	var sum float64
	for k, a := range q {
		b := q[(k+1)%len(q)]
		sum += mul(a.X, b.Y) - mul(b.X, a.Y)
	}
	return float64(sum / 2)
}

func (v *validator) run() {
	w := v.w
	if w.Schema != SchemaVersion {
		v.bad("schema %d, want %d", w.Schema, SchemaVersion)
	}
	v.meta()
	if !w.Codebooks.equal(new(DefaultCodebooks())) {
		v.bad("codebooks differ from schema %d's", SchemaVersion)
	}
	if v.wd == 0 {
		return // geometry cannot be checked without the map's size
	}
	if len(w.Cells) == 0 || len(w.Corners) == 0 || len(w.Edges) == 0 {
		v.bad("%d cells, %d corners, %d edges: want some of each", len(w.Cells), len(w.Corners), len(w.Edges))
		return
	}
	if !v.ids() {
		return // references cannot be followed
	}
	v.cells()
	v.exits()
	v.corners()
	v.playaSinks()
	v.edges()
	v.coastlines()
	v.rivers()
	v.outcomes()
}

func (v *validator) meta() {
	m := &v.w.Meta
	if m.Generator != "mpg" {
		v.bad("meta.generator %q, want \"mpg\"", m.Generator)
	}
	if s, err := strconv.ParseUint(m.Seed, 10, 64); err != nil || strconv.FormatUint(s, 10) != m.Seed {
		v.bad("meta.seed %q is not a canonical decimal uint64", m.Seed)
	}
	if len(m.ConfigHash) != 64 || strings.Trim(m.ConfigHash, "0123456789abcdef") != "" {
		v.bad("meta.config_hash %q is not a lowercase hex SHA-256", m.ConfigHash)
	}
	if finite(m.WidthKm) && finite(m.HeightKm) && m.WidthKm > 0 && m.HeightKm > 0 {
		v.wd, v.ht = m.WidthKm, m.HeightKm
	} else {
		v.bad("meta size %v x %v km: want positive finite", m.WidthKm, m.HeightKm)
	}
	if m.Wrap != "east-west" {
		v.bad("meta.wrap %q, want \"east-west\"", m.Wrap)
	}
	if m.Origin != "northwest" {
		v.bad("meta.origin %q, want \"northwest\"", m.Origin)
	}
	if m.RimCells < 0 || !finite(m.RimKm) || m.RimKm < 0 || (v.ht > 0 && 2*m.RimKm >= v.ht) {
		v.bad("meta rim %d cells, %v km: want non-negative and less than half the height", m.RimCells, m.RimKm)
	}
	if !finite(m.HexFlatToFlatMi) || m.HexFlatToFlatMi <= 0 || !finite(m.ProvinceAreaKm2) || m.ProvinceAreaKm2 <= 0 {
		v.bad("meta province %v mi, %v km²: want positive finite", m.HexFlatToFlatMi, m.ProvinceAreaKm2)
	}
	if m.CoordinatePrecisionKm != PrecisionKm {
		v.bad("meta.coordinate_precision_km %v, want %v", m.CoordinatePrecisionKm, PrecisionKm)
	}
	if m.Units != DefaultUnits() {
		v.bad("meta.units %+v, want %+v", m.Units, DefaultUnits())
	}
}

// ids checks that ids equal indexes and every reference is in range, and
// reports whether references can be followed.
func (v *validator) ids() bool {
	w := v.w
	nc, nk, ne := len(w.Cells), len(w.Corners), len(w.Edges)
	before := v.count
	for i := range w.Cells {
		c := &w.Cells[i]
		if c.ID != i {
			v.bad("cells[%d].id %d", i, c.ID)
		}
		for _, k := range c.Corners {
			if k < 0 || k >= nk {
				v.bad("cell %d: corner %d out of range", i, k)
			}
		}
		for _, e := range c.Sides {
			if e < 0 || e >= ne {
				v.bad("cell %d: side %d out of range", i, e)
			}
		}
		for _, x := range c.Exits {
			if x.Neighbor < 0 || x.Neighbor >= nc || x.Edge < 0 || x.Edge >= ne {
				v.bad("cell %d: exit to %d through edge %d out of range", i, x.Neighbor, x.Edge)
			}
		}
	}
	for i := range w.Corners {
		k := &w.Corners[i]
		if k.ID != i {
			v.bad("corners[%d].id %d", i, k.ID)
		}
		for _, c := range k.Cells {
			if c < 0 || c >= nc {
				v.bad("corner %d: cell %d out of range", i, c)
			}
		}
		for _, e := range k.Edges {
			if e < 0 || e >= ne {
				v.bad("corner %d: edge %d out of range", i, e)
			}
		}
	}
	for i := range w.Edges {
		e := &w.Edges[i]
		if e.ID != i {
			v.bad("edges[%d].id %d", i, e.ID)
		}
		a, b := e.Cells[0], e.Cells[1]
		if a < 0 || a >= nc || b >= nc || (b < 0 && b != Boundary) || (b != Boundary && a >= b) {
			v.bad("edge %d: cells %v: want 0 ≤ a < b < %d, or b = %d", i, e.Cells, nc, Boundary)
		}
		for _, k := range e.Corners {
			if k < 0 || k >= nk {
				v.bad("edge %d: corner %d out of range", i, k)
			}
		}
	}
	return v.count == before
}

// inMap reports whether p lies on the map: x in [0, W), y in [0, H].
func (v *validator) inMap(p Point) bool {
	return finite(p.X) && finite(p.Y) && p.X >= 0 && p.X < v.wd && p.Y >= 0 && p.Y <= v.ht
}

// isWater reports whether the cell is playable water, as the coast rule
// sees it: water that is not on the rim.
func (c *Cell) isWater() bool { return c.Landform.IsWater() && !c.HasFlag(FlagRim) }

// isInland reports whether the cell is inland water: a lake or inland sea.
func (c *Cell) isInland() bool { return c.Water == Lake || c.Water == InlandSea }

func (v *validator) cells() {
	w := v.w
	for i := range w.Cells {
		c := &w.Cells[i]
		rim := c.HasFlag(FlagRim)
		switch {
		case !slices.Contains(Landforms, c.Landform):
			v.bad("cell %d: landform %q not in the codebook", i, c.Landform)
		case c.Landform == SaltWater && !slices.Contains(Depths, c.Depth):
			v.bad("cell %d: salt water with depth %q", i, c.Depth)
		case c.Landform != SaltWater && c.Depth != DepthNone:
			v.bad("cell %d: %s with depth %q", i, c.Landform, c.Depth)
		}
		switch {
		case rim:
			if c.Landform != SaltWater || c.Depth != Deep || c.Water != WaterNone || c.HasFlag(FlagSalt) {
				v.bad("cell %d: rim cell is %s, depth %q, water %q, salt flag %v: want deep salt water with no water kind or flag",
					i, c.Landform, c.Depth, c.Water, c.HasFlag(FlagSalt))
			}
		case c.Landform.IsLand() && c.Water != WaterNone:
			v.bad("cell %d: land with water kind %q", i, c.Water)
		case c.Landform == SaltWater && c.Water != Ocean && c.Water != InlandSea:
			v.bad("cell %d: salt water with water kind %q", i, c.Water)
		case c.Landform == FreshWater && c.Water != Lake:
			v.bad("cell %d: fresh water with water kind %q", i, c.Water)
		}
		if !finite(c.AltitudeM) {
			v.bad("cell %d: altitude %v", i, c.AltitudeM)
		}
		last := -1
		for _, f := range c.Flags {
			k := slices.Index(CellFlags, f)
			if k <= last {
				v.bad("cell %d: flags %q not distinct codebook values in codebook order", i, c.Flags)
				break
			}
			last = k
		}
		if rim != c.HasFlag(FlagImpassable) {
			v.bad("cell %d: rim %v but impassable %v", i, rim, !rim)
		}
		if c.HasFlag(FlagVolcano) && !c.Landform.IsLand() {
			v.bad("cell %d: volcano on %s", i, c.Landform)
		}
		if c.HasFlag(FlagSalt) && !c.isInland() {
			v.bad("cell %d: salt flag on %s with water %q: want a lake or inland sea", i, c.Landform, c.Water)
		}
		if c.HasFlag(FlagPlaya) && (rim || !c.Landform.IsLand()) {
			v.bad("cell %d: playa on %s (rim %v): want playable land", i, c.Landform, rim)
		}
		v.cover(i, c, rim)
		if !v.inMap(c.Site) || c.Site.Y == 0 || c.Site.Y == v.ht {
			v.bad("cell %d: site %v off the map", i, c.Site)
		}
		if !v.inMap(c.Centroid) {
			v.bad("cell %d: centroid %v off the map", i, c.Centroid)
		}
		v.polygon(i, c)
	}
}

// cover checks a cell's biome and surface.
func (v *validator) cover(i int, c *Cell, rim bool) {
	land := !rim && c.Landform.IsLand()
	water := !rim && c.Landform.IsWater()
	if c.Biome != BiomeNone && !slices.Contains(Biomes, c.Biome) {
		v.bad("cell %d: biome %q not in the codebook", i, c.Biome)
	}
	if c.Surface != SurfaceNone && !slices.Contains(Surfaces, c.Surface) {
		v.bad("cell %d: surface %q not in the codebook", i, c.Surface)
	}
	switch {
	case rim && (c.Biome != BiomeNone || c.Surface != SurfaceNone):
		v.bad("cell %d: rim cell with biome %q, surface %q: want neither", i, c.Biome, c.Surface)
	case land && c.Biome == BiomeNone:
		v.bad("cell %d: land with no biome", i)
	case !land && c.Biome != BiomeNone:
		v.bad("cell %d: biome %q on %s: want land", i, c.Biome, c.Landform)
	}
	if land && (c.Biome == Clear) != c.Surface.IsIce() {
		v.bad("cell %d: biome %q with surface %q: biome clear exactly under glacier or ice-field", i, c.Biome, c.Surface)
	}
	switch {
	case c.Surface.IsIce() && !land:
		v.bad("cell %d: %s on %s: want land", i, c.Surface, c.Landform)
	case c.Surface == PackIce && !water:
		v.bad("cell %d: pack-ice on %s (rim %v): want playable water", i, c.Landform, rim)
	case c.Surface.IsWetland() && !land:
		v.bad("cell %d: %s on %s: want land", i, c.Surface, c.Landform)
	case c.Surface.IsWetland() && c.Landform != Flats && c.Landform != Plains && !(c.Surface == SaltFlats && c.HasFlag(FlagPlaya)):
		v.bad("cell %d: %s on %s: want flats or plains (or a playa's salt-flats)", i, c.Surface, c.Landform)
	case c.Surface == Mangroves && !c.HasFlag(FlagCoast):
		v.bad("cell %d: mangroves without the coast flag", i)
	}
	if c.HasFlag(FlagPlaya) && c.Surface != SaltFlats {
		v.bad("cell %d: playa with surface %q, want salt-flats", i, c.Surface)
	}
}

func (v *validator) polygon(i int, c *Cell) {
	w := v.w
	n := len(c.Corners)
	if n < 3 || len(c.Sides) != n || len(c.Polygon) != n {
		v.bad("cell %d: %d corners, %d sides, %d polygon points: want at least 3 of each, as many of each", i, n, len(c.Sides), len(c.Polygon))
		return
	}
	if slices.Min(c.Corners) != c.Corners[0] {
		v.bad("cell %d: polygon starts at corner %d, not its lowest", i, c.Corners[0])
	}
	if s := slices.Sorted(slices.Values(c.Corners)); len(slices.Compact(s)) != n {
		v.bad("cell %d: polygon repeats a corner", i)
	}
	coast := false
	for k, e := range c.Sides {
		a, b := c.Corners[k], c.Corners[(k+1)%n]
		ed := &w.Edges[e]
		coast = coast || ed.Coast
		switch i {
		case ed.Cells[0]:
			if ed.Corners != [2]int{a, b} {
				v.bad("cell %d: side %d joins corners %v, want %d→%d", i, e, ed.Corners, a, b)
			}
		case ed.Cells[1]:
			if ed.Corners != [2]int{b, a} {
				v.bad("cell %d: side %d joins corners %v, want %d→%d", i, e, ed.Corners, b, a)
			}
		default:
			v.bad("cell %d: side %d belongs to cells %v", i, e, ed.Cells)
		}
		p, k0 := c.Polygon[k], w.Corners[a].Point
		if !finite(p.X) || !finite(p.Y) {
			v.bad("cell %d: polygon point %d is %v", i, k, p)
			return
		}
		if math.Abs(dx(k0.X, c.Site.X+p.X, v.wd)) > PrecisionKm || math.Abs(c.Site.Y+p.Y-k0.Y) > PrecisionKm {
			v.bad("cell %d: site %v + offset %v does not reach corner %d at %v", i, c.Site, p, a, k0)
		}
		if bx := &c.BBox; c.Site.X+p.X < bx.Min.X-PrecisionKm || c.Site.X+p.X > bx.Max.X+PrecisionKm ||
			c.Site.Y+p.Y < bx.Min.Y-PrecisionKm || c.Site.Y+p.Y > bx.Max.Y+PrecisionKm {
			v.bad("cell %d: bbox %v does not contain polygon point %d", i, *bx, k)
		}
	}
	if coast != c.HasFlag(FlagCoast) {
		v.bad("cell %d: coast flag %v, but has a coast edge %v", i, !coast, coast)
	}
	if shoelace(c.Polygon) <= 0 {
		v.bad("cell %d: polygon is not clockwise", i)
	}
	minX, maxX, minY, maxY := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
	for _, p := range c.Polygon {
		minX, maxX = min(minX, c.Site.X+p.X), max(maxX, c.Site.X+p.X)
		minY, maxY = min(minY, c.Site.Y+p.Y), max(maxY, c.Site.Y+p.Y)
	}
	bx := &c.BBox
	if math.Abs(bx.Min.X-minX) > PrecisionKm || math.Abs(bx.Max.X-maxX) > PrecisionKm ||
		math.Abs(bx.Min.Y-minY) > PrecisionKm || math.Abs(bx.Max.Y-maxY) > PrecisionKm {
		v.bad("cell %d: bbox %v is not the polygon's [%v %v]–[%v %v]", i, *bx, minX, minY, maxX, maxY)
	}
}

func (v *validator) exits() {
	w := v.w
	for i := range w.Cells {
		c := &w.Cells[i]
		if len(c.Exits) > MaxExits {
			v.bad("cell %d: %d exits, more than %d", i, len(c.Exits), MaxExits)
		}
		var want []int
		for _, e := range c.Sides {
			if !w.Edges[e].OnBoundary() {
				want = append(want, e)
			}
		}
		var got []int
		last, descents := -1, 0
		for k, x := range c.Exits {
			got = append(got, x.Edge)
			d := x.Direction.Index()
			if d <= last {
				v.bad("cell %d: exit directions %v not distinct compass points in clockwise order", i, directions(c.Exits))
			}
			last = max(last, d)
			if x.BearingDeg > c.Exits[(k+1)%len(c.Exits)].BearingDeg {
				descents++
			}
			ed := &w.Edges[x.Edge]
			g := ed.InclinePermille
			switch {
			case ed.Cells == [2]int{i, x.Neighbor}:
			case ed.Cells == [2]int{x.Neighbor, i}:
				g = -g
			default:
				v.bad("cell %d: exit %s to %d through edge %d, which joins %v", i, x.Direction, x.Neighbor, x.Edge, ed.Cells)
				continue
			}
			if x.InclinePermille != g {
				v.bad("cell %d: exit %s incline %d, edge %d gives %d", i, x.Direction, x.InclinePermille, x.Edge, g)
			}
			if !finite(x.BearingDeg) || x.BearingDeg < 0 || x.BearingDeg >= 360 {
				v.bad("cell %d: exit %s bearing %v", i, x.Direction, x.BearingDeg)
				continue
			}
			o := w.Cells[x.Neighbor].Site
			if b := bearing(dx(c.Site.X, o.X, v.wd), o.Y-c.Site.Y); angleDiff(b, x.BearingDeg) > AngleToleranceDeg {
				v.bad("cell %d: exit %s bearing %v, sites give %v", i, x.Direction, x.BearingDeg, b)
			}
			if d >= 0 && math.Abs(angleDiff(x.BearingDeg, float64(45*d))-x.ErrorDeg) > AngleToleranceDeg {
				v.bad("cell %d: exit %s error %v for bearing %v", i, x.Direction, x.ErrorDeg, x.BearingDeg)
			}
			if o := &w.Cells[x.Neighbor]; c.isInland() {
				switch {
				case o.Water == Ocean:
					v.bad("cell %d: %s next to ocean cell %d", i, c.Water, x.Neighbor)
				case o.isInland() && (o.Water != c.Water || o.HasFlag(FlagSalt) != c.HasFlag(FlagSalt)):
					v.bad("cell %d: %s (salt %v) next to %s cell %d (salt %v) of the same lake",
						i, c.Water, c.HasFlag(FlagSalt), o.Water, x.Neighbor, o.HasFlag(FlagSalt))
				}
			}
			back := slices.IndexFunc(w.Cells[x.Neighbor].Exits, func(y Exit) bool { return y.Neighbor == i })
			if back < 0 {
				v.bad("cell %d lists %d as a neighbor, but %d does not list %d", i, x.Neighbor, x.Neighbor, i)
			} else if y := w.Cells[x.Neighbor].Exits[back]; y.Edge != x.Edge || y.InclinePermille != -x.InclinePermille {
				v.bad("cell %d: exit to %d through edge %d incline %d, but back through edge %d incline %d",
					i, x.Neighbor, x.Edge, x.InclinePermille, y.Edge, y.InclinePermille)
			}
		}
		if descents > 1 {
			v.bad("cell %d: exit bearings not in clockwise order", i)
		}
		slices.Sort(want)
		slices.Sort(got)
		if !slices.Equal(got, want) {
			v.bad("cell %d: exits through edges %v, want one through each side off the boundary %v", i, got, want)
		}
	}
}

func directions(xs []Exit) []Direction {
	out := make([]Direction, len(xs))
	for k, x := range xs {
		out[k] = x.Direction
	}
	return out
}

func (v *validator) corners() {
	w := v.w
	cellsOf := make([][]int, len(w.Corners))
	for i := range w.Cells {
		for _, k := range w.Cells[i].Corners {
			cellsOf[k] = append(cellsOf[k], i)
		}
	}
	edgesOf := make([][]int, len(w.Corners))
	for e := range w.Edges {
		for _, k := range w.Edges[e].Corners {
			edgesOf[k] = append(edgesOf[k], e)
		}
	}
	for i := range w.Corners {
		k := &w.Corners[i]
		if !v.inMap(k.Point) {
			v.bad("corner %d: point %v off the map", i, k.Point)
		}
		boundary := k.Point.Y == 0 || k.Point.Y == v.ht
		if boundary != k.HasFlag(FlagBoundary) {
			v.bad("corner %d: boundary flag %v at y = %v", i, !boundary, k.Point.Y)
		}
		last := -1
		for _, f := range k.Flags {
			x := slices.Index(CornerFlags, f)
			if x <= last {
				v.bad("corner %d: flags %q not distinct codebook values in codebook order", i, k.Flags)
				break
			}
			last = x
		}
		if lo := 3 - btoi(boundary); len(k.Cells) < lo || len(k.Cells) > 4 {
			v.bad("corner %d: %d cells, want %d–4", i, len(k.Cells), lo)
		}
		if !slices.Equal(k.Cells, cellsOf[i]) {
			v.bad("corner %d: cells %v, but the polygons through it are %v", i, k.Cells, cellsOf[i])
		}
		if !slices.Equal(k.Edges, edgesOf[i]) {
			v.bad("corner %d: edges %v, but the edges ending there are %v", i, k.Edges, edgesOf[i])
		}
		var sum float64
		land, wet, playa := false, false, false
		for _, c := range k.Cells {
			cell := &w.Cells[c]
			sum += cell.AltitudeM
			land = land || (cell.Landform.IsLand() && !cell.HasFlag(FlagRim))
			wet = wet || cell.Landform.IsWater()
			playa = playa || cell.HasFlag(FlagPlaya)
		}
		sink := k.HasFlag(FlagSink)
		if sink && !playa {
			v.bad("corner %d: sink, but touches no playa cell", i)
		}
		if len(k.Cells) > 0 {
			if mean := sum / float64(len(k.Cells)); !finite(k.HeightM) || math.Abs(k.HeightM-mean) > HeightToleranceM {
				v.bad("corner %d: height %v, its cells' mean altitude %v", i, k.HeightM, mean)
			}
		}
		if terminal := land && wet || sink; terminal != k.HasFlag(FlagTerminal) {
			v.bad("corner %d: terminal flag %v, but touches land %v and water %v, sink %v", i, !terminal, land, wet, sink)
		}
		if k.HasFlag(FlagMouth) && !k.HasFlag(FlagTerminal) {
			v.bad("corner %d: mouth that is not terminal", i)
		}
	}
}

// playaSinks checks that every playa cell's lowest corner, by height and
// then id, is a sink.
func (v *validator) playaSinks() {
	w := v.w
	for i := range w.Cells {
		c := &w.Cells[i]
		if !c.HasFlag(FlagPlaya) || len(c.Corners) == 0 {
			continue
		}
		low := c.Corners[0]
		for _, k := range c.Corners {
			if hk, hl := w.Corners[k].HeightM, w.Corners[low].HeightM; hk < hl || hk == hl && k < low {
				low = k
			}
		}
		if !w.Corners[low].HasFlag(FlagSink) {
			v.bad("cell %d: playa whose lowest corner %d is not a sink", i, low)
		}
	}
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (v *validator) edges() {
	w := v.w
	sideOf := make([][]int, len(w.Edges))
	for i := range w.Cells {
		for _, e := range w.Cells[i].Sides {
			sideOf[e] = append(sideOf[e], i)
		}
	}
	for i := range w.Edges {
		e := &w.Edges[i]
		a := &w.Cells[e.Cells[0]]
		p, q := w.Corners[e.Corners[0]].Point, w.Corners[e.Corners[1]].Point
		if e.Corners[0] == e.Corners[1] {
			v.bad("edge %d: both ends at corner %d", i, e.Corners[0])
		}
		want := []int{e.Cells[0]}
		if !e.OnBoundary() {
			want = append(want, e.Cells[1])
		}
		if !slices.Equal(sideOf[i], want) {
			v.bad("edge %d: cells %v, but it is a side of %v", i, e.Cells, sideOf[i])
		}
		dist := math.Sqrt(mul(dx(p.X, q.X, v.wd), dx(p.X, q.X, v.wd)) + mul(q.Y-p.Y, q.Y-p.Y))
		if !finite(e.LengthKm) || e.LengthKm <= 0 || math.Abs(e.LengthKm-dist) > PrecisionKm {
			v.bad("edge %d: length %v km, its corners are %v km apart", i, e.LengthKm, dist)
		}
		if !slices.Contains(RiverClasses, e.River) && e.River != RiverNone {
			v.bad("edge %d: river %q not in the codebook", i, e.River)
		}
		if e.OnBoundary() {
			if !a.HasFlag(FlagRim) || !w.Corners[e.Corners[0]].HasFlag(FlagBoundary) || !w.Corners[e.Corners[1]].HasFlag(FlagBoundary) {
				v.bad("edge %d: on the rim boundary, but its cell or corners are not", i)
			}
			if e.Passable || e.Coast || e.Water != WaterNone || e.River != RiverNone || e.InclinePermille != 0 {
				v.bad("edge %d: on the rim boundary, but passable %v, coast %v, water %q, river %q, incline %d",
					i, e.Passable, e.Coast, e.Water, e.River, e.InclinePermille)
			}
			continue
		}
		b := &w.Cells[e.Cells[1]]
		rim := a.HasFlag(FlagRim) || b.HasFlag(FlagRim)
		if e.Passable == rim {
			v.bad("edge %d: passable %v between cells %v (rim %v)", i, e.Passable, e.Cells, rim)
		}
		var water Water
		coast := false
		if !rim && a.isWater() != b.isWater() {
			coast = true
			water = a.Water
			if b.isWater() {
				water = b.Water
			}
		}
		if e.Coast != coast || e.Water != water {
			v.bad("edge %d: coast %v water %q, want coast %v water %q", i, e.Coast, e.Water, coast, water)
		}
		if e.River != RiverNone && (rim || !a.Landform.IsLand() || !b.Landform.IsLand()) {
			v.bad("edge %d: river %q not between two land cells", i, e.River)
		}
		if e.InclinePermille < -MaxIncline || e.InclinePermille > MaxIncline {
			v.bad("edge %d: incline %d beyond ±%d", i, e.InclinePermille, MaxIncline)
		}
		sd := math.Sqrt(mul(dx(a.Site.X, b.Site.X, v.wd), dx(a.Site.X, b.Site.X, v.wd)) + mul(b.Site.Y-a.Site.Y, b.Site.Y-a.Site.Y))
		if sd > 0 {
			g := min(max((b.AltitudeM-a.AltitudeM)/sd, -MaxIncline), MaxIncline)
			if math.Abs(g-float64(e.InclinePermille)) > 1 {
				v.bad("edge %d: incline %d, altitudes and sites give %.2f", i, e.InclinePermille, g)
			}
		}
	}
}

func (v *validator) coastlines() {
	w := v.w
	seen := make([]int, len(w.Edges))
	lastFirst := -1
	for i, cl := range w.Coastlines {
		n := len(cl.Edges)
		if n == 0 || len(cl.Corners) != n+1 {
			v.bad("coastline %d: %d edges and %d corners: want some edges and one more corner", i, n, len(cl.Corners))
			continue
		}
		if cl.Edges[0] <= lastFirst {
			v.bad("coastline %d: first edge %d, not after the previous chain's %d", i, cl.Edges[0], lastFirst)
		}
		lastFirst = cl.Edges[0]
		if closed := cl.Corners[n] == cl.Corners[0]; closed != cl.Closed {
			v.bad("coastline %d: closed %v, but it ends at corner %d and starts at %d", i, cl.Closed, cl.Corners[n], cl.Corners[0])
		}
		if cl.Closed && slices.Min(cl.Edges) != cl.Edges[0] {
			v.bad("coastline %d: closed, but does not start at its lowest edge", i)
		}
		for k, e := range cl.Edges {
			if e < 0 || e >= len(w.Edges) {
				v.bad("coastline %d: edge %d out of range", i, e)
				return
			}
			seen[e]++
			ed := &w.Edges[e]
			if !ed.Coast {
				v.bad("coastline %d: edge %d is not a coast", i, e)
				continue
			}
			from, to := cl.Corners[k], cl.Corners[k+1]
			var right int
			switch [2]int{from, to} {
			case ed.Corners:
				right = ed.Cells[0]
			case [2]int{ed.Corners[1], ed.Corners[0]}:
				right = ed.Cells[1]
			default:
				v.bad("coastline %d: edge %d joins corners %v, not %d→%d", i, e, ed.Corners, from, to)
				continue
			}
			if !w.Cells[right].Landform.IsLand() {
				v.bad("coastline %d: edge %d has water on its right", i, e)
			}
		}
	}
	for e, n := range seen {
		if w.Edges[e].Coast && n != 1 {
			v.bad("edge %d: coast edge in %d coastlines, want 1", e, n)
		}
	}
}

func (v *validator) rivers() {
	w := v.w
	nk := len(w.Corners)
	onPath := make([]int, len(w.Edges)) // polylines holding each edge
	ends := make([]bool, nk)            // the last corner of some polyline
	within := make([]bool, nk)          // a corner of some polyline before its last
	lastFirst := -1
	for i, r := range w.Rivers {
		n := len(r.Edges)
		if n == 0 || len(r.Corners) != n+1 || len(r.Classes) != n {
			v.bad("river %d: %d edges, %d corners, %d classes", i, n, len(r.Corners), len(r.Classes))
			continue
		}
		if r.Edges[0] <= lastFirst {
			v.bad("river %d: first edge %d, not after the previous river's %d", i, r.Edges[0], lastFirst)
		}
		lastFirst = r.Edges[0]
		ok := true
		for _, k := range r.Corners {
			if k < 0 || k >= nk {
				v.bad("river %d: corner %d out of range", i, k)
				ok = false
			}
		}
		if !ok {
			continue
		}
		seen := map[int]bool{}
		for k, c := range r.Corners {
			if seen[c] {
				v.bad("river %d: corner %d twice", i, c)
			}
			seen[c] = true
			if k < n {
				within[c] = true
			}
		}
		ends[r.Corners[n]] = true
		prev := -1
		for k, e := range r.Edges {
			if e < 0 || e >= len(w.Edges) {
				v.bad("river %d: edge %d out of range", i, e)
				continue
			}
			onPath[e]++
			ed := &w.Edges[e]
			if ed.River == RiverNone || ed.River != r.Classes[k] {
				v.bad("river %d: edge %d class %q, river says %q", i, e, ed.River, r.Classes[k])
			}
			if c := [2]int{r.Corners[k], r.Corners[k+1]}; c != ed.Corners && c != [2]int{ed.Corners[1], ed.Corners[0]} {
				v.bad("river %d: edge %d joins corners %v, not %v", i, e, ed.Corners, c)
			}
			x := slices.Index(RiverClasses, r.Classes[k])
			if x < prev {
				v.bad("river %d: class falls from %q to %q at edge %d", i, RiverClasses[prev], r.Classes[k], e)
			}
			prev = max(prev, x)
		}
	}
	for e := range w.Edges {
		ed := &w.Edges[e]
		if ed.River != RiverNone && onPath[e] != 1 {
			v.bad("edge %d: river %q on %d river polylines, want 1", e, ed.River, onPath[e])
		}
		if ed.River == RiverNone {
			continue
		}
		for _, k := range ed.Corners {
			for _, c := range w.Corners[k].Cells {
				if w.Cells[c].HasFlag(FlagRim) {
					v.bad("edge %d: river %q beside rim cell %d", e, ed.River, c)
				}
			}
		}
	}
	for k := range w.Corners {
		if mouth := ends[k] && !within[k]; mouth != w.Corners[k].HasFlag(FlagMouth) {
			v.bad("corner %d: mouth flag %v, but rivers end there %v and continue %v", k, !mouth, ends[k], within[k])
		}
	}
}

func (v *validator) outcomes() {
	w := v.w
	o := &w.Outcomes
	var playable, land, ocean, basin, lake, sea, playas int
	var glacier, iceField, packIce, wetland int
	var area float64
	for i := range w.Cells {
		c := &w.Cells[i]
		switch {
		case c.Surface == Glacier:
			glacier++
		case c.Surface == IceField:
			iceField++
		case c.Surface == PackIce:
			packIce++
		case c.Surface.IsWetland():
			wetland++
		}
		if c.HasFlag(FlagRim) {
			continue
		}
		playable++
		switch c.Water {
		case Lake:
			lake++
		case InlandSea:
			sea++
		}
		if c.HasFlag(FlagPlaya) {
			playas++
		}
		if c.Water == Ocean {
			ocean++
			if c.AltitudeM > o.SeaLevelM {
				v.bad("cell %d: ocean at %v m, above the sea level %v m", i, c.AltitudeM, o.SeaLevelM)
			}
		}
		if c.Landform.IsLand() {
			land++
			area += shoelace(c.Polygon)
			if c.AltitudeM <= o.SeaLevelM {
				basin++
			}
		}
	}
	if playable != o.PlayableCells || land != o.LandCells || ocean != o.OceanCells || basin != o.DryBasinCells ||
		lake != o.LakeCells || sea != o.InlandSeaCells {
		v.bad("outcomes: %d playable, %d land, %d ocean, %d dry basin, %d lake, %d inland-sea cells; the cells give %d, %d, %d, %d, %d, %d",
			o.PlayableCells, o.LandCells, o.OceanCells, o.DryBasinCells, o.LakeCells, o.InlandSeaCells, playable, land, ocean, basin, lake, sea)
	}
	lakes, seas, saltLakes, saltSeas := v.lakes()
	if lakes != o.Lakes || seas != o.InlandSeas || saltLakes != o.SaltLakes || saltSeas != o.SaltInlandSeas || playas != o.Playas {
		v.bad("outcomes: %d lakes, %d inland seas, %d and %d salt, %d playas; the cells give %d, %d, %d and %d, %d",
			o.Lakes, o.InlandSeas, o.SaltLakes, o.SaltInlandSeas, o.Playas, lakes, seas, saltLakes, saltSeas, playas)
	}
	if glacier != o.GlacierCells || iceField != o.IceFieldCells || packIce != o.PackIceCells || wetland != o.WetlandCells {
		v.bad("outcomes: %d glacier, %d ice-field, %d pack-ice, %d wetland cells; the cells give %d, %d, %d, %d",
			o.GlacierCells, o.IceFieldCells, o.PackIceCells, o.WetlandCells, glacier, iceField, packIce, wetland)
	}
	if o.BiomeTable == "" {
		v.bad("outcomes: no biome table")
	}
	if o.ExpectedLakeCells < 0 || o.PrePassLakeCells < 0 || o.ClimatePasses < 1 || !(o.DatumLandShare > 0 && o.DatumLandShare < 1) {
		v.bad("outcomes: expected lake cells %d, pre-pass lake cells %d, datum land share %v, climate passes %d",
			o.ExpectedLakeCells, o.PrePassLakeCells, o.DatumLandShare, o.ClimatePasses)
	}
	if !finite(o.LandAreaKm2) || math.Abs(o.LandAreaKm2-area) > 1e-6*max(area, 1) {
		v.bad("outcomes: land area %v km², the land polygons give %v", o.LandAreaKm2, area)
	}
	if !finite(o.SeaLevelM) || !finite(o.InitialEstimateM) {
		v.bad("outcomes: sea level %v, initial estimate %v", o.SeaLevelM, o.InitialEstimateM)
	}
	if o.TargetLandCells <= 0 || o.TolerancePercent < 0 || o.ToleranceCells != o.TolerancePercent*o.TargetLandCells/100 {
		v.bad("outcomes: target %d, tolerance %d cells (%d%%)", o.TargetLandCells, o.ToleranceCells, o.TolerancePercent)
	}
	if met := max(o.LandCells-o.TargetLandCells, o.TargetLandCells-o.LandCells) <= o.ToleranceCells; met != o.Met {
		v.bad("outcomes: met %v with %d land cells for %d ± %d", o.Met, o.LandCells, o.TargetLandCells, o.ToleranceCells)
	}
	if !slices.Contains(Reasons, o.Reason) {
		v.bad("outcomes: reason %q", o.Reason)
	}
	if o.Policy == "" || len(o.Trace) == 0 || len(o.Trace) > o.Budget {
		v.bad("outcomes: policy %q, budget %d, %d probes", o.Policy, o.Budget, len(o.Trace))
	}
	for k, p := range o.Trace {
		if !slices.Contains(Methods, p.Method) || !finite(p.LevelM) || p.Land < 0 || p.Ocean < 0 || p.DryBasin < 0 || p.Lake < 0 {
			v.bad("outcomes: probe %d %+v", k, p)
		}
	}
}

// lakes counts the lakes and inland seas, each a connected set of inland
// water cells through the cells' exits, and the salt ones among each, by
// the flag of the set's lowest cell.
func (v *validator) lakes() (lakes, seas, saltLakes, saltSeas int) {
	w := v.w
	seen := make([]bool, len(w.Cells))
	var queue []int
	for i := range w.Cells {
		c := &w.Cells[i]
		if seen[i] || !c.isInland() || c.HasFlag(FlagRim) {
			continue
		}
		seen[i] = true
		queue = append(queue[:0], i)
		for k := 0; k < len(queue); k++ {
			for _, x := range w.Cells[queue[k]].Exits {
				if o := &w.Cells[x.Neighbor]; !seen[x.Neighbor] && o.Water == c.Water && !o.HasFlag(FlagRim) {
					seen[x.Neighbor] = true
					queue = append(queue, x.Neighbor)
				}
			}
		}
		salt := c.HasFlag(FlagSalt)
		if c.Water == Lake {
			lakes++
			saltLakes += btoi(salt)
		} else {
			seas++
			saltSeas += btoi(salt)
		}
	}
	return
}
