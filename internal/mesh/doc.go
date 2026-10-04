// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package mesh builds the province mesh (DESIGN.md, "Mesh"; pipeline stage
// 4): the Voronoi diagram of sites on the cylinder, as a graph of cells,
// corners, and edges with stable ids and wrap-aware geometry, over sites
// relaxed by Lloyd's algorithm, with its short edges collapsed and its cells
// held to the degree cap. Rim flags and the mesh checks are later steps.
//
// # Sites
//
// There are round(W·H / A) sites over the full cylinder, rim included, in a
// jittered grid of near-square boxes about √A on a side (see JitteredGrid),
// drawn from the "mesh" seed stream at algorithm version Version. Site i is
// the site of cell i.
//
// # Lloyd relaxation
//
// New relaxes the sites with cfg.Mesh.LloydPasses passes of Lloyd's
// algorithm (Lloyd) before it builds the graph. Each pass sweeps the
// cylinder Voronoi diagram of the current sites (the same sweep and ghost
// band certificate as Build, without assembling the graph) and moves every
// site, rim sites included, to its cell's wrapped centroid: the centroid of
// the polygon unwrapped about the site, x wrapped back into [0, W). A rim
// cell is clipped at y = 0 or y = H, so its centroid stays strictly inside.
// The coefficient of variation of the cell areas falls with each pass; New
// keeps the value before each pass in Mesh.LloydCV, and Stats reports the
// relaxed mesh's.
//
// # Area
//
// DESIGN.md asks that the relaxed mesh then be scaled so the mean cell area
// is exactly A. The mesh instead keeps the raster's fixed W × H cylinder,
// which the raster and the mesh share, and meets the target by its site
// count: the cells tile the cylinder, so their mean area is exactly W·H/n,
// and n = round(W·H/A) puts it within 1/(2n) of A (about 1.4·10⁻⁵ in the
// default world). Coordinates are never scaled. Stats reports the mean, its
// relative deviation from A, the area CV, and the smallest and largest
// cells as multiples of A.
//
// # Voronoi on a cylinder
//
// Build runs Fortune's sweep (package mesh/voronoi, vendored) over the sites
// plus ghost copies shifted by ±W, and keeps only the original sites' cells.
// The sweep's box is [−m, W + m] × [0, H]: north and south it clips at y = 0
// and y = H, inside the rim, so the clipped edges belong to rim cells and
// clipping never touches a playable cell. Such an edge has one cell and lies
// on the rim boundary.
//
// Only the sites within a ghost band m of the seam are copied: those with
// x < m shifted east by W, and those with x ≥ W − m shifted west. The first
// band is four mean site spacings, 4·√(W·H/n). The result is then proved
// exact rather than assumed: a swept cell can only be too big, never too
// small, and it is the true cell if no site left out of the sweep is nearer
// to any of its vertices than its own site (a convex cell meets the half
// plane nearer some other site only if a vertex does). The nearest left-out
// copies lie at x ≥ W + m and x < −m, so every vertex v of a kept cell, at
// distance r from its site, must satisfy v.x + r < W + m and v.x − r > −m;
// that also catches a cell clipped by the box's east or west side. If any
// vertex fails, the band doubles and the sweep runs again, up to m = W (full
// ghost copies); a cell that still fails is too wide for the cylinder, and
// Build reports it. In the default worlds the first band suffices,
// so the sweep handles about 1.03 n sites instead of 3 n.
//
// # Corners
//
// Every vertex of every kept polygon is wrapped into x in [0, W) and merged
// with the vertices within MergeKm (1e-6 km), across the seam too, by a
// union-find over a bucket grid, visiting vertices in cell order. A corner
// near the seam is computed twice, from sites on one side and from ghosts
// on the other, and the copies differ by rounding; merging makes them one
// corner. A corner takes the position of its first vertex, with y snapped
// to exactly 0 or H within MergeKm. A polygon side whose two ends merged
// (an edge shorter than MergeKm, as where four sites share a circle) is
// dropped, and its two cells meet only at the corner, so they are not
// neighbors.
//
// # Short-edge collapse and degree cap
//
// New then applies Collapse with min_edge_km and the degree cap (default
// 8). Edges are taken from a priority queue shortest first, ties broken by
// edge id; every edge at a corner that moves is re-queued at its new
// length, and stale entries are skipped. An edge shorter than min_edge_km
// is collapsed when that is safe:
//
//   - the merged corner would touch at most 4 cells (off the rim boundary:
//     both ends are still 3-way corners);
//   - each cell beside the edge keeps at least 4 sides and 3 neighbors.
//
// A collapse merges the two corners into one at the edge's wrapped
// midpoint (on the boundary if either end is) and removes the edge: the two
// cells that shared it now touch only at a point and are no longer
// neighbors; the cells at its ends keep their sides, which now meet at the
// 4-way corner.
//
// Otherwise the edge is stretched instead: its ends move apart,
// symmetrically about its midpoint, along the perpendicular bisector of its
// two cells' sites (the direction a Voronoi edge between them runs; along x
// for an edge on the boundary), until it is min_edge_km·(1 + StretchMargin)
// long. A corner on the rim boundary stays put, and its partner moves the
// whole way. The cells stay neighbors and the corners stay 3-way. The
// guard is what keeps every corner at 3–4 cells (literal collapse makes a
// 5-way corner wherever two short edges meet) and keeps a hexagon with
// three alternating short sides from becoming a triangle well under A/2.
// In the default worlds about one short edge in a hundred is stretched,
// and a corner moves at most half of min_edge_km, about 1.35 km, except
// where moves chain (up to about 2.2 km in the test worlds).
//
// Then the degree cap: while some cell (the lowest id first) has more
// neighbors than the cap, its shortest edge that may be collapsed (by the
// rule above; ties by edge id) is collapsed, and the queue is drained
// again. Lloyd-relaxed meshes almost never need it.
//
// The result is renumbered canonically as below and checked: Collapse fails
// rather than return a bad mesh when it needs more than 16 steps per edge,
// when a stretch would push a corner off the map, when a cell over the cap
// has no edge it may collapse, when an edge is still short, and when a
// cell's polygon is not simple or has no positive area. Mesh.Collapses,
// Stretches, DegreeCapHits, MaxShiftKm, and Stretched report the work.
// Rim cells (InRim, by site y, until the rim flag exists) on the boundary
// row can have as few as 1 or 2 neighbors, straight from the Voronoi
// diagram; every other cell keeps 3 to the cap.
//
// # Ids and graph
//
//   - Cell i is site i's cell. Its Corners run clockwise on the map (north
//     up; positive shoelace area in x-east, y-south coordinates), starting at
//     its lowest corner id, and Edges[k] joins Corners[k] to Corners[k+1].
//   - Corners are numbered by y, then x (north to south, west to east).
//   - Edges are numbered by their corner ids, lower first. Each undirected
//     edge is stored once, with Cells[0] < Cells[1], or Cells[1] = Boundary on
//     the rim boundary, and Corners in Cells[0]'s clockwise order.
//
// Build and Collapse check the graph as they build it and fail rather than
// return a bad one: every polygon side must be matched by the opposite side of exactly
// the cell it names, or lie on y = 0 or y = H with no cell beyond; no cell
// may border itself or pass through a corner twice. All ordering is by
// sorting with complete tie-breaks; no result depends on map iteration.
//
// # Geometry
//
// Corners are stored wrapped. Offsets and Polygon unwrap a cell's corners
// about its site with topo's shorter-way displacement, so a cell that spans
// the seam comes out whole; Area and Centroid use them, and EdgeLength uses
// topo's wrapped distance. Products that feed sums go through package
// fmath, and TestNoFusedMultiplyAdd checks the compiled code, so the graph
// and the relaxed sites are bit-identical on every architecture.
//
// # Encoding
//
// AppendBinary writes the canonical byte encoding of the graph that the
// golden hashes cover.
package mesh
