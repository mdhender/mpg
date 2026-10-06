// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/mdhender/mpg/world"
)

// Seed ranking (sweep --rank): a sweep orders its rows by a weighted score
// of named measures. A ranking is a property of a sweep, not of a world, so
// it lives in sweep flags and never in config.json.

// defaultRankSpec is the ranking "--rank default" selects: habitable land
// (weighted twice), land touching a river, major straits, necks, and land
// near the coast, all higher-is-better.
const defaultRankSpec = "usability.habitable_share*2,rivers.touch_share,chokepoints.straits_major,chokepoints.necks,usability.coast_within_share"

// Rank limits.
const (
	maxRankKeys   = 32
	maxRankWeight = 1000
)

// Rank scopes: rows are ranked against the other rows of their aspect and
// preset block, or against every row of the sweep.
const (
	rankScopeGroup = "group"
	rankScopeAll   = "all"
)

// rankKey is one term of a ranking: a measure, the direction that is
// better, and its weight.
type rankKey struct {
	measure string
	dir     string  // "max", "min", or "~" (closest to target)
	target  float64 // for dir "~"
	weight  float64 // positive and finite
}

// String returns the key in the --rank syntax, with the defaults (":max",
// "*1") left out.
func (k rankKey) String() string {
	s := k.measure
	switch k.dir {
	case "min":
		s += ":min"
	case "~":
		s += ":~" + strconv.FormatFloat(k.target, 'g', -1, 64)
	}
	if k.weight != 1 {
		s += "*" + strconv.FormatFloat(k.weight, 'g', -1, 64)
	}
	return s
}

// better maps a raw measure value to one where higher is better.
func (k rankKey) better(v float64) float64 {
	switch k.dir {
	case "min":
		return -v
	case "~":
		return -math.Abs(v - k.target)
	}
	return v
}

// parseRankSpec parses --rank: "default", or a comma-separated list of
// measure[:max|:min|:~X][*weight] items. The measure is a scalar measure
// name (world.MeasureNames); the direction defaults to max, and ~X ranks
// the value closest to X best; the weight defaults to 1 and must be
// positive, finite and at most maxRankWeight. Empty items, unknown or
// repeated measures, and more than maxRankKeys keys are errors.
func parseRankSpec(s string) ([]rankKey, error) {
	if strings.TrimSpace(s) == "default" {
		s = defaultRankSpec
	}
	names := world.MeasureNames()
	var keys []rankKey
	for item := range strings.SplitSeq(s, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			return nil, fmt.Errorf("empty item in %q", s)
		}
		if len(keys) == maxRankKeys {
			return nil, fmt.Errorf("more than %d keys", maxRankKeys)
		}
		head, weight, hasWeight := strings.Cut(item, "*")
		measure, dir, hasDir := strings.Cut(head, ":")
		k := rankKey{measure: measure, dir: "max", weight: 1}
		if !slices.Contains(names, measure) {
			return nil, fmt.Errorf("%q: unknown measure %q", item, measure)
		}
		if slices.ContainsFunc(keys, func(o rankKey) bool { return o.measure == measure }) {
			return nil, fmt.Errorf("measure %s repeated", measure)
		}
		if hasDir {
			switch {
			case dir == "max", dir == "min":
				k.dir = dir
			case strings.HasPrefix(dir, "~"):
				t, err := parseFinite(dir[1:])
				if err != nil {
					return nil, fmt.Errorf("%q: target: %v", item, err)
				}
				k.dir, k.target = "~", t
			default:
				return nil, fmt.Errorf("%q: direction %q is not max, min, or ~X", item, dir)
			}
		}
		if hasWeight {
			w, err := parseFinite(weight)
			if err != nil {
				return nil, fmt.Errorf("%q: weight: %v", item, err)
			}
			if w <= 0 || w > maxRankWeight {
				return nil, fmt.Errorf("%q: weight %v outside (0, %d]", item, w, maxRankWeight)
			}
			k.weight = w
		}
		keys = append(keys, k)
	}
	return keys, nil
}

// parseFinite parses a finite decimal float.
func parseFinite(s string) (float64, error) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not a number", s)
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, errors.New("not finite")
	}
	return v, nil
}

// rankSpecString returns keys in the --rank syntax.
func rankSpecString(keys []rankKey) string {
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k.String()
	}
	return strings.Join(parts, ",")
}

// rankItem is one sweep row to rank.
type rankItem struct {
	group  int       // scope group: rows rank only against their group
	values []float64 // raw measure values, one per key
	pass   bool      // no gate failed
}

// rankResult gives each item its rank within its group (1 is best) and
// its score, and the order the items go in: groups in order of first
// appearance, each by rank.
type rankResult struct {
	rank  []int
	score []float64
	order []int
}

// rankItems scores and orders items. Within each group an item's score is
// 100 · Σ wₖ·pₖ / Σ wₖ, where pₖ is the item's mid-rank percentile on key k
// among the group: (below + (equal − 1)/2) / (n − 1) of the better-is-higher
// values, 0.5 for a group of one. Items whose gates failed rank after every
// passing item of their group; ties go to the order given. Arithmetic is
// rounded at each step, so no fused multiply-add changes a score, and no
// score is NaN or Inf.
func rankItems(items []rankItem, keys []rankKey) rankResult {
	res := rankResult{rank: make([]int, len(items)), score: make([]float64, len(items))}
	var groups [][]int
	slot := map[int]int{} // group id to index in groups
	for i, it := range items {
		g, ok := slot[it.group]
		if !ok {
			g = len(groups)
			slot[it.group] = g
			groups = append(groups, nil)
		}
		groups[g] = append(groups[g], i)
	}
	var wsum float64
	for _, k := range keys {
		wsum = float64(wsum + k.weight)
	}
	for _, members := range groups {
		n := len(members)
		acc := make([]float64, n)
		for ki, k := range keys {
			p := percentiles(members, func(i int) float64 { return k.better(items[i].values[ki]) })
			for j := range n {
				acc[j] = float64(acc[j] + float64(k.weight*p[j]))
			}
		}
		for j, i := range members {
			if wsum > 0 {
				res.score[i] = float64(100 * float64(acc[j]/wsum))
			}
		}
		sorted := slices.Clone(members)
		slices.SortStableFunc(sorted, func(a, b int) int {
			if items[a].pass != items[b].pass {
				if items[a].pass {
					return -1
				}
				return 1
			}
			return cmp.Compare(res.score[b], res.score[a])
		})
		for r, i := range sorted {
			res.rank[i] = r + 1
		}
		res.order = append(res.order, sorted...)
	}
	return res
}

// percentiles returns the mid-rank percentile in [0, 1] of value(i) for
// each member: the share of the other members it beats, counting each tie
// as half. A single member gets 0.5.
func percentiles(members []int, value func(int) float64) []float64 {
	n := len(members)
	p := make([]float64, n)
	if n == 1 {
		p[0] = 0.5
		return p
	}
	idx := make([]int, n)
	for j := range n {
		idx[j] = j
	}
	slices.SortStableFunc(idx, func(a, b int) int { return cmp.Compare(value(members[a]), value(members[b])) })
	for lo := 0; lo < n; {
		hi := lo + 1
		for hi < n && value(members[idx[hi]]) == value(members[idx[lo]]) {
			hi++
		}
		// lo values below; hi-lo equal, the member itself among them.
		v := float64(float64(lo)+float64(hi-lo-1)/2) / float64(n-1)
		for _, j := range idx[lo:hi] {
			p[j] = v
		}
		lo = hi
	}
	return p
}

// rankRow is a sweep row as the rank table shows it.
type rankRow struct {
	seed           uint64
	aspect, preset string
	rank           int
	score          float64
	pass, landMet  bool
	values         []float64
}

// rankTable returns the Markdown rank table: a heading with the spec,
// scope and base config hash, then one row per sweep row, in sheet order.
func rankTable(keys []rankKey, scope, configHash string, rows []rankRow) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# mpg sweep ranking\n\n")
	fmt.Fprintf(&b, "- rank: `%s`\n- scope: %s\n- config: `%s`\n\n", rankSpecString(keys), scope, configHash)
	head := []string{"rank", "seed", "aspect", "preset", "score", "pass", "land met"}
	for _, k := range keys {
		head = append(head, k.String())
	}
	b.WriteString("| " + strings.Join(head, " | ") + " |\n")
	b.WriteString("|" + strings.Repeat(" ---: |", 2) + strings.Repeat(" --- |", 2) + " ---: |" + strings.Repeat(" --- |", 2) + strings.Repeat(" ---: |", len(keys)) + "\n")
	yes := func(ok bool) string {
		if ok {
			return "yes"
		}
		return "no"
	}
	for _, r := range rows {
		cells := []string{strconv.Itoa(r.rank), strconv.FormatUint(r.seed, 10), r.aspect, r.preset,
			fmt.Sprintf("%.1f", r.score), yes(r.pass), yes(r.landMet)}
		for _, v := range r.values {
			cells = append(cells, formatMeasure(v))
		}
		b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
	}
	return b.String()
}

// formatMeasure formats a measure value: whole numbers as integers, others
// with four decimals.
func formatMeasure(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return strconv.FormatFloat(v+0, 'f', 0, 64) // +0 turns -0 into 0
	}
	return strconv.FormatFloat(v, 'f', 4, 64)
}
