// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package pipeline

import (
	"image"

	"github.com/mdhender/mpg/internal/basin"
	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/edges"
	"github.com/mdhender/mpg/internal/mesh"
	"github.com/mdhender/mpg/internal/river"
)

// RiverInput returns the river stage's input at the land target t: the
// land and lakes after the search, each lake's surface, spill corner and
// pass cell, the playas' dry sinks, and the water balance's cell-level
// catchments (basin Lakes.Sink), immediate and after lake overflow, for
// the agreement.
func RiverInput(m *mesh.Mesh, alt []float64, t *LandTarget) river.Input {
	l := t.Lakes
	in := river.Input{
		Mesh:     m,
		Altitude: alt,
		Land:     t.Land,
		Lake:     l.Lake,
		Lakes:    make([]river.LakeIn, len(l.Lakes)),
	}
	for k, lk := range l.Lakes {
		in.Lakes[k] = river.LakeIn{SurfaceM: lk.SurfaceM, Spill: lk.Outlet, Pass: river.None}
		if lk.Outlet != basin.None {
			in.Lakes[k].Pass = t.Basins.Basins[lk.Basin].SpillCell
		}
	}
	playa := make(map[int]int, len(l.Playas)) // basin -> sink corner
	for _, p := range l.Playas {
		in.Sinks = append(in.Sinks, p.Corner)
		playa[p.Basin] = p.Corner
	}
	// dest is where water reaching basin b rests, in the cell-level
	// model: the sea (None), b's lake, b's playa, or unknown (a dry basin
	// that is neither).
	dest := func(b int) river.Dest {
		switch {
		case b == basin.None:
			return river.Dest{Kind: river.Ocean, ID: river.None}
		case l.Water[b].Lake != basin.None:
			return river.Dest{Kind: river.Lake, ID: l.Water[b].Lake}
		}
		if k, ok := playa[b]; ok {
			return river.Dest{Kind: river.Sink, ID: k}
		}
		return river.Dest{Kind: river.Interior, ID: b}
	}
	final := func(b int) river.Dest {
		d := dest(b)
		for n := 0; d.Kind == river.Lake && n <= len(l.Lakes); n++ {
			lk := &l.Lakes[d.ID]
			if lk.Outlet == basin.None {
				break
			}
			d = dest(l.Water[lk.Basin].OverflowTo)
		}
		return d
	}
	in.Catchment = make([]river.Dest, len(m.Cells))
	in.FinalCatchment = make([]river.Dest, len(m.Cells))
	for c, land := range t.Land {
		if land {
			in.Catchment[c] = dest(l.Sink[c])
			in.FinalCatchment[c] = final(l.Sink[c])
		}
	}
	return in
}

// RiverFlow returns the water the river network accumulates at the land
// target t: the final climate pass's runoff, and each lake's overflow from
// the water balance.
func RiverFlow(t *LandTarget) river.Flow {
	f := river.Flow{Runoff: t.Climate.Runoff, Overflow: make([]float64, len(t.Lakes.Lakes))}
	for l, lk := range t.Lakes.Lakes {
		f.Overflow[l] = lk.Overflow
	}
	return f
}

// RiverParams returns the river selection settings of cfg.
func RiverParams(cfg *config.Config) river.Params {
	r := cfg.River
	return river.Params{ThresholdKm2: r.ThresholdKm2, RiverKm2: r.RiverKm2, MajorRiverKm2: r.MajorRiverKm2}
}

// runRivers builds the corner drainage tree (package river) on the land
// target's land and lakes, and the river network on it: runoff accumulated
// down the tree, the river edges and their classes, and the polylines. It
// logs the tree's corners, terminals, edges, lake drains and agreement with
// the water balance's cell-level catchments, then the network's classes,
// density, polylines, endings and longest river; and renders the rivers
// by class, as variant "tree" the drainage tree, and as variant
// "catchments" where each land cell's runoff ends.
func runRivers(c *Context) error {
	p := &c.Products
	m, t := p.Mesh, p.Target
	in := RiverInput(m, p.Cells.Altitude, t)
	tr, err := river.Build(in)
	if err != nil {
		return err
	}
	net, err := river.Accumulate(in, tr, RiverFlow(t), RiverParams(&c.Config))
	if err != nil {
		return err
	}
	s := tr.Stats(m)
	c.Logf("%d land corners: %d interior, %d ocean, %d lake, %d sink terminals; %d tree edges (%d crossing the seam, %d across flats, %d climbing out of pits)",
		s.LandCorners, s.Terminals[river.Interior], s.Terminals[river.Ocean], s.Terminals[river.Lake], s.Terminals[river.Sink],
		s.Edges, s.Seam, s.Flat, s.Climb)
	c.Logf("%d overflowing lakes: %d by an outlet (%d at the spill corner), %d straight to the sea, %d straight into another lake; %d fallback outlets",
		s.Overflowing, s.Drains[river.Outlet], s.AtSpill, s.Drains[river.DirectSea], s.Drains[river.DirectLake], tr.Fallbacks)
	if a := tr.Agreement; a != nil && a.Cells > 0 {
		c.Logf("catchments against the water balance: %d of %d land cells (%.2f%%, %.2f%% of the area) end elsewhere, %d (%.2f%%) finally",
			a.Differ, a.Cells, 100*float64(a.Differ)/float64(a.Cells), 100*a.DifferKm2/a.AreaKm2,
			a.FinalDiffer, 100*float64(a.FinalDiffer)/float64(a.Cells))
	}
	ns := net.Stats(in, tr)
	c.Logf("%d river edges of %d tree edges (%d stream, %d river, %d major-river; %d crossing the seam); largest drainage %.0f km², discharge %.1f m³/s",
		ns.RiverEdges, ns.RiverEdges+ns.Edges[edges.RiverNone], ns.Edges[edges.Stream], ns.Edges[edges.River], ns.Edges[edges.MajorRiver], ns.Seam,
		ns.MaxDrainageKm2, ns.MaxDischargeM3s)
	c.Logf("river density: %.3f river edges per land cell, %.1f km per 1000 km² of land, %.1f%% of land cells touching a river",
		ns.EdgesPerLandCell(), ns.KmPer1000Km2(), 100*ns.TouchShare())
	c.Logf("%d river polylines (%d from lake outlets): %d end at the sea, %d at lakes, %d at dry sinks, %d at confluences; %d mouths; longest %d edges (%.0f km), longest flow %d edges (%.0f km)",
		ns.Paths, ns.OutletSources, ns.Ends[river.Ocean], ns.Ends[river.Lake], ns.Ends[river.Sink], ns.Ends[river.Interior], ns.Mouths,
		ns.LongestEdges, ns.LongestKm, ns.FlowEdges, ns.FlowKm)
	f := p.Elevation
	for _, v := range []struct {
		name string
		draw func() *image.RGBA
	}{
		{"", func() *image.RGBA { return river.Render(f, in, tr, net, t.Flood.Level) }},
		{"tree", func() *image.RGBA { return river.TreeRender(f, in, tr, t.Flood.Level) }},
		{"catchments", func() *image.RGBA { return river.CatchmentRender(f, in, tr) }},
	} {
		if !c.rendering() {
			break
		}
		if err := c.Render(v.name, v.draw()); err != nil {
			return err
		}
	}
	p.Rivers = tr
	p.Network = net
	p.RiverStats = &ns
	return nil
}
