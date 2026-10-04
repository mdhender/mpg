// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package config

// Mesh sets the province mesh (DESIGN.md, "Mesh"; pipeline stage 4): the
// sites, the Lloyd relaxation, the short-edge collapse, the degree cap, and
// the area bounds the mesh checks hold cells to. See package mesh.
type Mesh struct {
	// Placement is how the sites are placed. The only placement is
	// "jittered-grid": rows of sites about √A apart over the full cylinder,
	// rim included, each moved at random within its grid box.
	Placement string `json:"placement"`
	// Jitter, in [0, 1), is how far a site may move from its grid box's
	// center, as a fraction of the box: 0 is a regular grid, and values
	// near 1 spread each site uniformly over its box. Below 1, every site
	// stays strictly inside its own box, so no two sites coincide.
	Jitter float64 `json:"jitter"`
	// LloydPasses is the number of Lloyd relaxation passes, in [0, 10]
	// (DESIGN.md: 2–3).
	LloydPasses int `json:"lloyd_passes"`
	// MinEdgeFraction, in [0, 1), is the shortest edge kept, as a fraction
	// of the province side √A; shorter edges are collapsed, or stretched
	// to it where a collapse is unsafe. 0 disables the collapse.
	MinEdgeFraction float64 `json:"min_edge_fraction"`
	// MinEdgeKm is MinEdgeFraction × √A (derived; DESIGN.md: min_edge_km).
	MinEdgeKm float64 `json:"min_edge_km"`
	// AreaMin and AreaMax bound a cell's area in the mesh checks, as
	// multiples of A: AreaMin in (0, 1], AreaMax in [1, 10].
	AreaMin float64 `json:"area_min"`
	AreaMax float64 `json:"area_max"`
	// DegreeCap is the most neighbors a cell may keep, in [3, 8]. A cell
	// needs a distinct compass point for each neighbor, so it is at most 8.
	DegreeCap int `json:"degree_cap"`
}

// PlacementJitteredGrid is the jittered-grid site placement.
const PlacementJitteredGrid = "jittered-grid"

// MaxLloydPasses bounds Mesh.LloydPasses.
const MaxLloydPasses = 10

// DefaultMesh returns the default mesh inputs, with the derived field zero.
func DefaultMesh() Mesh {
	return Mesh{
		Placement:       PlacementJitteredGrid,
		Jitter:          0.8,
		LloydPasses:     2,
		MinEdgeFraction: 0.3,
		AreaMin:         0.5,
		AreaMax:         1.6,
		DegreeCap:       8,
	}
}

// validate appends the problems with the mesh inputs through bad.
func (m *Mesh) validate(bad func(format string, args ...any)) {
	if m.Placement != PlacementJitteredGrid {
		bad("mesh.placement %q must be %q", m.Placement, PlacementJitteredGrid)
	}
	if v := m.Jitter; !(v >= 0 && v < 1) {
		bad("mesh.jitter %v must be in [0, 1)", v)
	}
	if v := m.LloydPasses; v < 0 || v > MaxLloydPasses {
		bad("mesh.lloyd_passes %d must be in [0, %d]", v, MaxLloydPasses)
	}
	if v := m.MinEdgeFraction; !(v >= 0 && v < 1) {
		bad("mesh.min_edge_fraction %v must be in [0, 1)", v)
	}
	if v := m.AreaMin; !(v > 0 && v <= 1) {
		bad("mesh.area_min %v must be in (0, 1]", v)
	}
	if v := m.AreaMax; !(v >= 1 && v <= 10) {
		bad("mesh.area_max %v must be in [1, 10]", v)
	}
	if v := m.DegreeCap; v < 3 || v > 8 {
		bad("mesh.degree_cap %d must be in [3, 8]", v)
	}
}
