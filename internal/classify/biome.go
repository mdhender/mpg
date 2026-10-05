// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package classify

import (
	"fmt"

	"github.com/mdhender/mpg/internal/basin"
	"github.com/mdhender/mpg/internal/edges"
	"github.com/mdhender/mpg/internal/fmath"
	"github.com/mdhender/mpg/internal/mesh"
)

// BiomeTableVersion names the biome and surface table: hmz2bio's rules
// (v0.3.0) with mpg's changes, numbered. world.json records it, so a reader
// knows which rules made a world's biomes. Change the number with any change
// to BiomeRules' defaults or to the rules themselves.
const BiomeTableVersion = "hmz2bio-0.3.0+mpg.1"

// Biome is a land cell's ecology, in hmz2bio's vocabulary plus polar desert.
type Biome uint8

// The biomes. BiomeNone is the biome of every cell that is not playable
// land.
const (
	BiomeNone Biome = iota
	Clear           // under permanent ice (surface glacier or ice field)
	PolarDesert
	Tundra
	Alpine
	Desert
	Scrubland
	Steppe
	Grassland
	Savanna
	BorealForest
	TemperateForest
	TemperateRainforest
	TropicalDryForest
	TropicalRainforest
	CloudForest
	numBiomes
)

// Biomes lists the biomes, in code order.
var Biomes = []Biome{Clear, PolarDesert, Tundra, Alpine, Desert, Scrubland, Steppe, Grassland, Savanna,
	BorealForest, TemperateForest, TemperateRainforest, TropicalDryForest, TropicalRainforest, CloudForest}

var biomeNames = [numBiomes]string{
	"", "clear", "polar-desert", "tundra", "alpine", "desert", "scrubland", "steppe", "grassland", "savanna",
	"boreal-forest", "temperate-forest", "temperate-rainforest", "tropical-dry-forest", "tropical-rainforest", "cloud-forest",
}

// String returns the biome's name, as "boreal-forest"; BiomeNone is "".
func (b Biome) String() string {
	if b < numBiomes {
		return biomeNames[b]
	}
	return fmt.Sprintf("Biome(%d)", uint8(b))
}

// IsForest reports whether the biome is a forest (hmz2bio's IsForest).
func (b Biome) IsForest() bool {
	switch b {
	case BorealForest, TemperateForest, TemperateRainforest, TropicalDryForest, TropicalRainforest, CloudForest:
		return true
	}
	return false
}

// Surface is what covers a cell: permanent ice, pack ice, or a wetland.
type Surface uint8

// The surfaces. SurfaceNone is bare ground or open water.
const (
	SurfaceNone Surface = iota
	Glacier
	IceField
	PackIce
	Marshes
	Swamps
	Bogs
	Mangroves
	SaltFlats
	numSurfaces
)

// Surfaces lists the surfaces, in code order.
var Surfaces = []Surface{Glacier, IceField, PackIce, Marshes, Swamps, Bogs, Mangroves, SaltFlats}

var surfaceNames = [numSurfaces]string{"", "glacier", "ice-field", "pack-ice", "marshes", "swamps", "bogs", "mangroves", "salt-flats"}

// String returns the surface's name, as "ice-field"; SurfaceNone is "".
func (s Surface) String() string {
	if s < numSurfaces {
		return surfaceNames[s]
	}
	return fmt.Sprintf("Surface(%d)", uint8(s))
}

// IsIce reports whether the surface is permanent ice on land.
func (s Surface) IsIce() bool { return s == Glacier || s == IceField }

// IsWetland reports whether the surface is a wetland: marshes, swamps,
// bogs, mangroves, or salt flats.
func (s Surface) IsWetland() bool { return s >= Marshes && s < numSurfaces }

// The wetness signals of a land cell (Result.Wetness), as bits.
const (
	// WetRiver: a flats cell beside a river edge of at least
	// BiomeRules.WetRiverClass.
	WetRiver uint8 = 1 << iota
	// WetShore: a flats or plains cell touching a lake or inland sea.
	WetShore
	// WetSurplus: a flats cell with an aridity index of at least
	// BiomeRules.WetAridityMin.
	WetSurplus
)

// BiomeRules holds the biome and surface thresholds. Temperatures are °C,
// precipitation and snowfall mm per year, lift m. They are not config: the
// table is versioned (BiomeTableVersion) instead.
type BiomeRules struct {
	// The annual range, warmest month minus coldest, is RangeBaseC +
	// RangePerDegC × |latitude in degrees|, split evenly about the mean:
	// hmz2bio's synthetic seasons, used only by this table.
	RangeBaseC, RangePerDegC float64
	// A land cell whose warmest month is below IceWarmestBelowC is
	// permanent ice when its snowfall is at least SnowMinMM, and polar
	// desert otherwise. Below TreeLineWarmestBelowC it is tundra, or alpine
	// when the warmest month at sea level would reach the tree line.
	IceWarmestBelowC, SnowMinMM, TreeLineWarmestBelowC float64
	// Köppen's aridity limit is AridPerDegMM × T + AridBaseMM; a cell is a
	// desert below DesertShare of it, and scrubland (from HotMinC) or
	// steppe below it.
	AridPerDegMM, AridBaseMM, DesertShare, HotMinC float64
	// Cloud forest and tropical montane forest (below MontaneBelowC; see
	// Biome).
	MontaneBelowC, CloudForestColdestMinC, CloudForestLiftM, CloudForestMinMM, TropicsColdestMinC float64
	// Tropical and temperate forests and grassland.
	RainforestMinMM, DryForestMinMM, BorealBelowC, GrasslandAridity float64
	// IceFieldMinCells is the number of non-mountain cells a connected
	// body of permanent ice needs for its non-mountain cells to be an ice
	// field rather than glacier tongues.
	IceFieldMinCells int
	// Wetness: the least river class beside a flats cell, and the least
	// aridity index of a flats cell with surplus.
	WetRiverClass edges.RiverClass
	WetAridityMin float64
	// Wetland subtypes: mangroves on an ocean coast from
	// MangroveColdestMinC, bogs below BogsBelowC.
	MangroveColdestMinC, BogsBelowC float64
	// Pack ice covers playable water whose warmest month is below
	// PackIceWarmestBelowC.
	PackIceWarmestBelowC float64
}

// DefaultBiomeRules returns the table BiomeTableVersion names: hmz2bio's
// DefaultRules for everything hmz2bio has, and mpg's values for snowfall,
// ice fields, wetness, and pack ice (measured in S32 on 12 worlds).
func DefaultBiomeRules() BiomeRules {
	return BiomeRules{
		RangeBaseC: 1, RangePerDegC: 0.33,
		IceWarmestBelowC: 0, SnowMinMM: 300, TreeLineWarmestBelowC: 10,
		AridPerDegMM: 20, AridBaseMM: 280, DesertShare: 0.5, HotMinC: 18,
		MontaneBelowC: 18, CloudForestColdestMinC: 15, CloudForestLiftM: 100, CloudForestMinMM: 1000, TropicsColdestMinC: 18,
		RainforestMinMM: 2000, DryForestMinMM: 1200, BorealBelowC: 4, GrasslandAridity: 1.5,
		IceFieldMinCells:    5,
		WetRiverClass:       edges.MajorRiver,
		WetAridityMin:       2,
		MangroveColdestMinC: 15, BogsBelowC: 10,
		PackIceWarmestBelowC: 0,
	}
}

// Season returns the mean temperatures of the warmest and coldest months of
// a cell at latDeg degrees from the equator with mean annual temperature t:
// t plus and minus half the annual range.
func (br BiomeRules) Season(latDeg, t float64) (warmest, coldest float64) {
	// The halving is a product the compiler would fuse into the sums, so
	// round it first.
	half := float64(fmath.MulAdd(br.RangePerDegC, latDeg, br.RangeBaseC) / 2)
	return t + half, t - half
}

// Arid returns Köppen's aridity limit in mm for mean temperature t.
func (br BiomeRules) Arid(t float64) float64 {
	return fmath.MulAdd(br.AridPerDegMM, t, br.AridBaseMM)
}

// Snowfall returns the part of precipitation p that falls as snow at
// latDeg degrees with mean temperature t: p times the share of the year
// below freezing, taken as a straight ramp from the coldest month to the
// warmest (1 when the warmest is below 0 °C, 0 when the coldest is at or
// above it).
func (br BiomeRules) Snowfall(latDeg, t, p float64) float64 {
	span := fmath.MulAdd(br.RangePerDegC, latDeg, br.RangeBaseC)
	if !(span > 0) {
		if t < 0 {
			return p
		}
		return 0
	}
	f := min(max((float64(span/2)-t)/span, 0), 1)
	return p * f
}

// CellClimate is what the biome table reads of a land cell.
type CellClimate struct {
	// LatitudeDeg is degrees from the equator, north and south alike.
	LatitudeDeg float64
	// SeaLevelC is the sea-level temperature at that latitude, and
	// TemperatureC the cell's mean annual temperature.
	SeaLevelC, TemperatureC float64
	// PrecipitationMM is annual precipitation, and LiftM the height the
	// wind climbs onto the cell.
	PrecipitationMM, LiftM float64
}

// Biome returns a land cell's biome by the table (the first row that
// applies wins), given its landform, and whether the cell is permanent ice:
//
//  1. warmest month below IceWarmestBelowC with snowfall at least
//     SnowMinMM: Clear, permanent ice;
//  2. warmest month below IceWarmestBelowC: PolarDesert, bare;
//  3. warmest month below TreeLineWarmestBelowC: Tundra if the sea-level
//     warmest month at the latitude is below it too, else Alpine;
//  4. P below DesertShare × the aridity limit: Desert;
//  5. P below the aridity limit, T at least HotMinC: Scrubland;
//  6. P below the aridity limit: Steppe;
//  7. T below MontaneBelowC, on hills, mountains, plateaus or volcanic
//     highlands, sea-level coldest month at least CloudForestColdestMinC,
//     lift at least CloudForestLiftM, P at least CloudForestMinMM:
//     CloudForest;
//  8. T below MontaneBelowC, sea-level coldest month at least
//     TropicsColdestMinC: TemperateForest;
//  9. T at least HotMinC: TropicalRainforest from RainforestMinMM,
//     TropicalDryForest from DryForestMinMM, else Savanna;
//  10. T below BorealBelowC: BorealForest;
//  11. P at least RainforestMinMM: TemperateRainforest;
//  12. P below GrasslandAridity × the aridity limit: Grassland;
//  13. otherwise TemperateForest.
//
// Rows 3 to 13 are hmz2bio's Rules.Biome; rows 1 and 2 split its glacial
// ice by snowfall.
func (br BiomeRules) Biome(c CellClimate, l Landform) (b Biome, permanentIce bool) {
	warm, _ := br.Season(c.LatitudeDeg, c.TemperatureC)
	seaWarm, seaCold := br.Season(c.LatitudeDeg, c.SeaLevelC)
	t, p := c.TemperatureC, c.PrecipitationMM
	switch {
	case warm < br.IceWarmestBelowC && br.Snowfall(c.LatitudeDeg, t, p) >= br.SnowMinMM:
		return Clear, true
	case warm < br.IceWarmestBelowC:
		return PolarDesert, false
	case warm < br.TreeLineWarmestBelowC:
		if seaWarm < br.TreeLineWarmestBelowC {
			return Tundra, false
		}
		return Alpine, false
	}
	arid := br.Arid(t)
	switch {
	case p < br.DesertShare*arid:
		return Desert, false
	case p < arid && t >= br.HotMinC:
		return Scrubland, false
	case p < arid:
		return Steppe, false
	}
	if t < br.MontaneBelowC {
		switch l {
		case Hills, Mountains, Plateaus, VolcanicHighlands:
			if seaCold >= br.CloudForestColdestMinC && c.LiftM >= br.CloudForestLiftM && p >= br.CloudForestMinMM {
				return CloudForest, false
			}
		}
		if seaCold >= br.TropicsColdestMinC {
			return TemperateForest, false
		}
	}
	switch {
	case t >= br.HotMinC && p >= br.RainforestMinMM:
		return TropicalRainforest, false
	case t >= br.HotMinC && p >= br.DryForestMinMM:
		return TropicalDryForest, false
	case t >= br.HotMinC:
		return Savanna, false
	case t < br.BorealBelowC:
		return BorealForest, false
	case p >= br.RainforestMinMM:
		return TemperateRainforest, false
	case p < br.GrasslandAridity*arid:
		return Grassland, false
	}
	return TemperateForest, false
}

// Wetland returns the wetland subtype of a wet land cell, in hmz2bio's
// order: SaltFlats when it is dry (P below the aridity limit), Mangroves on
// an ocean coast whose coldest month is at least MangroveColdestMinC, Bogs
// below BogsBelowC, Swamps in a forest biome, and Marshes otherwise.
func (br BiomeRules) Wetland(c CellClimate, b Biome, oceanCoast bool) Surface {
	_, cold := br.Season(c.LatitudeDeg, c.TemperatureC)
	switch {
	case c.PrecipitationMM < br.Arid(c.TemperatureC):
		return SaltFlats
	case oceanCoast && cold >= br.MangroveColdestMinC:
		return Mangroves
	case c.TemperatureC < br.BogsBelowC:
		return Bogs
	case b.IsForest():
		return Swamps
	}
	return Marshes
}

// CoverInput is what Cover reads besides the landforms: the land and
// water, the final climate, and the rivers. Per-cell slices are indexed by
// cell id, RiverClass by edge id.
type CoverInput struct {
	// Land marks the land cells after lakes (never rim cells).
	Land []bool
	// Lakes is the water balance: its Lake gives each lake and
	// inland-sea cell's lake, and its Playas the playa cells. Every other
	// playable cell that is not land is ocean.
	Lakes *basin.Lakes
	// LatitudeDeg, SeaLevelC, TemperatureC, PrecipitationMM, Aridity and
	// LiftM are each cell's (see CellClimate); Aridity is P/PET (UNEP),
	// capped.
	LatitudeDeg, SeaLevelC, TemperatureC, PrecipitationMM, Aridity, LiftM []float64
	// RiverClass is each edge's river class.
	RiverClass []edges.RiverClass
}

// Cover gives every playable land cell of m its biome, and land and water
// cells their surface, by the rules br, and records each land cell's
// wetness signals, in r (whose landforms it reads). See the package
// documentation for the rules.
func (r *Result) Cover(m *mesh.Mesh, in CoverInput, br BiomeRules) error {
	n := len(m.Cells)
	if len(r.Landform) != n {
		return fmt.Errorf("classify: %d landforms for %d cells", len(r.Landform), n)
	}
	if in.Lakes == nil || len(in.Land) != n || len(in.Lakes.Lake) != n || len(in.LatitudeDeg) != n || len(in.SeaLevelC) != n ||
		len(in.TemperatureC) != n || len(in.PrecipitationMM) != n || len(in.Aridity) != n || len(in.LiftM) != n {
		return fmt.Errorf("classify: cover input does not match %d cells", n)
	}
	if len(in.RiverClass) != len(m.Edges) {
		return fmt.Errorf("classify: %d river classes for %d edges", len(in.RiverClass), len(m.Edges))
	}
	for i := range n {
		for _, v := range []float64{in.LatitudeDeg[i], in.SeaLevelC[i], in.TemperatureC[i], in.PrecipitationMM[i], in.Aridity[i], in.LiftM[i]} {
			if !finite(v) {
				return fmt.Errorf("classify: cell %d climate value %v", i, v)
			}
		}
	}
	r.Biome = make([]Biome, n)
	r.Surface = make([]Surface, n)
	r.Wetness = make([]uint8, n)
	playa := make([]bool, n)
	for _, p := range in.Lakes.Playas {
		playa[p.Cell] = true
	}
	climateOf := func(i int) CellClimate {
		return CellClimate{LatitudeDeg: in.LatitudeDeg[i], SeaLevelC: in.SeaLevelC[i], TemperatureC: in.TemperatureC[i],
			PrecipitationMM: in.PrecipitationMM[i], LiftM: in.LiftM[i]}
	}
	land := func(i int) bool { return in.Land[i] && !m.Cells[i].Rim }
	lake := func(i int) bool { return in.Lakes.Lake[i] != basin.None }

	// Biomes, and the permanent ice. A playa is never ice: it is salt flats.
	ice := make([]bool, n)
	for i := range n {
		if !land(i) {
			continue
		}
		b, perm := br.Biome(climateOf(i), r.Landform[i])
		if perm && playa[i] {
			b, perm = PolarDesert, false
		}
		r.Biome[i], ice[i] = b, perm
	}

	// Glaciers and ice fields: connected bodies of permanent ice, found
	// breadth first in id order.
	seen := make([]bool, n)
	var queue []int
	for i := range n {
		if !ice[i] || seen[i] {
			continue
		}
		seen[i] = true
		queue = append(queue[:0], i)
		plain, mountain := 0, false
		for k := 0; k < len(queue); k++ {
			u := queue[k]
			if r.Landform[u] == Mountains {
				mountain = true
			} else {
				plain++
			}
			for _, v := range m.Cells[u].Neighbors {
				if ice[v] && !seen[v] {
					seen[v] = true
					queue = append(queue, v)
				}
			}
		}
		for _, u := range queue {
			switch {
			case r.Landform[u] == Mountains:
				r.Surface[u] = Glacier
			case plain >= br.IceFieldMinCells || !mountain:
				r.Surface[u] = IceField
			default:
				r.Surface[u] = Glacier // a tongue reaching down from the mountains
			}
		}
	}

	// Wetness signals and wetlands; playas are salt flats.
	for i := range n {
		if !land(i) || ice[i] {
			continue
		}
		l := r.Landform[i]
		var sig uint8
		oceanCoast := false
		for _, j := range m.Cells[i].Neighbors {
			if m.Cells[j].Rim || in.Land[j] {
				continue
			}
			if lake(j) {
				if l == Flats || l == Plains {
					sig |= WetShore
				}
			} else {
				oceanCoast = true
			}
		}
		if l == Flats {
			for _, e := range m.Cells[i].Edges {
				if in.RiverClass[e] >= br.WetRiverClass {
					sig |= WetRiver
					break
				}
			}
			if in.Aridity[i] >= br.WetAridityMin {
				sig |= WetSurplus
			}
		}
		r.Wetness[i] = sig
		switch {
		case playa[i]:
			r.Surface[i] = SaltFlats
		case sig != 0:
			r.Surface[i] = br.Wetland(climateOf(i), r.Biome[i], oceanCoast)
		}
	}

	// Pack ice on playable water: the ocean, inland seas, and lakes (at
	// their surface's temperature).
	for i, c := range m.Cells {
		if c.Rim || in.Land[i] {
			continue
		}
		if warm, _ := br.Season(in.LatitudeDeg[i], in.TemperatureC[i]); warm < br.PackIceWarmestBelowC {
			r.Surface[i] = PackIce
		}
	}
	return nil
}

// SurfaceCount returns the number of cells with surface s.
func (r *Result) SurfaceCount(s Surface) int {
	n := 0
	for _, v := range r.Surface {
		if v == s {
			n++
		}
	}
	return n
}

// WetlandCount returns the number of cells with a wetland surface.
func (r *Result) WetlandCount() int {
	n := 0
	for _, v := range r.Surface {
		if v.IsWetland() {
			n++
		}
	}
	return n
}

// BiomeHistogram returns the number of land cells with each biome, in the
// order of Biomes, and their total.
func (r *Result) BiomeHistogram() (counts []int, total int) {
	counts = make([]int, len(Biomes))
	for _, b := range r.Biome {
		if b != BiomeNone && b < numBiomes {
			counts[b-Clear]++
			total++
		}
	}
	return counts, total
}

// biomeAbbrev abbreviates the biomes for compact summaries, in code order.
var biomeAbbrev = [numBiomes]string{"", "ice", "pd", "tu", "al", "de", "sc", "st", "gr", "sv", "bo", "tf", "tr", "td", "tp", "cf"}

// Abbrev returns the biome's abbreviation for compact summaries, as "de"
// for desert and "ice" for clear (under permanent ice).
func (b Biome) Abbrev() string {
	if b < numBiomes {
		return biomeAbbrev[b]
	}
	return b.String()
}

// BiomeShares returns, for each biome with any cells in the order of
// Biomes, its count and share of the land, as "desert 123 (1.2%)", or with
// compact true its abbreviation and rounded percentage, as "de1".
func (r *Result) BiomeShares(compact bool) []string {
	counts, total := r.BiomeHistogram()
	var out []string
	for k, b := range Biomes {
		if counts[k] == 0 {
			continue
		}
		pct := 100 * float64(counts[k]) / float64(total)
		if compact {
			out = append(out, fmt.Sprintf("%s%.0f", b.Abbrev(), pct))
		} else {
			out = append(out, fmt.Sprintf("%s %d (%.1f%%)", b, counts[k], pct))
		}
	}
	return out
}
