// Package deps is the dependency graph between catalog products. A release
// or a vulnerability of a library (PJSIP, Sofia-SIP, libre) is an early
// signal for every product that uses it.
package deps

import (
	"sort"
	"strings"

	"github.com/napadailo/voip-info/internal/catalog"
)

// Kinds of dependencies and how a fix reaches the consumer.
const (
	Bundled  = "bundled"  // a copy of the sources inside: fixed only by a release of the consumer
	Linked   = "linked"   // a library it is built with
	Optional = "optional" // an optional module
	Runtime  = "runtime"  // a separate service next to it, updated on its own
	Platform = "platform" // the product is built on top of the other
)

// Strong kinds are those a vulnerability flows through.
func Strong(kind string) bool { return kind == Bundled || kind == Linked || kind == Platform }

// Edge: Src depends on Dst.
type Edge struct{ Src, Dst, Kind string }

// Graph of the catalog.
type Graph struct {
	Edges []Edge
	names map[string]string
	up    map[string][]Edge
	down  map[string][]Edge
}

// Hop is an edge reached at a depth.
type Hop struct {
	Edge  Edge
	Depth int
}

// New builds the graph; dependencies outside the catalog are ignored.
func New(products []*catalog.Product) *Graph {
	g := &Graph{names: map[string]string{}, up: map[string][]Edge{}, down: map[string][]Edge{}}
	for _, p := range products {
		g.names[p.ID] = p.Name
	}
	for _, p := range products {
		for _, d := range p.Deps {
			dst, kind, _ := strings.Cut(d, ":")
			if kind == "" {
				kind = Linked
			}
			if _, ok := g.names[dst]; !ok {
				continue
			}
			e := Edge{p.ID, dst, kind}
			g.Edges = append(g.Edges, e)
			g.up[p.ID] = append(g.up[p.ID], e)
			g.down[dst] = append(g.down[dst], e)
		}
	}
	sort.Slice(g.Edges, func(i, j int) bool {
		if g.names[g.Edges[i].Src] != g.names[g.Edges[j].Src] {
			return strings.ToLower(g.names[g.Edges[i].Src]) < strings.ToLower(g.names[g.Edges[j].Src])
		}
		return g.Edges[i].Dst < g.Edges[j].Dst
	})
	return g
}

func (g *Graph) walk(id string, index map[string][]Edge, next func(Edge) string, strongOnly bool) []Hop {
	type item struct {
		id    string
		depth int
	}
	var out []Hop
	seen := map[string]bool{id: true}
	queue := []item{{id, 1}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, e := range index[cur.id] {
			if strongOnly && !Strong(e.Kind) {
				continue
			}
			n := next(e)
			if seen[n] {
				continue
			}
			seen[n] = true
			out = append(out, Hop{e, cur.depth})
			queue = append(queue, item{n, cur.depth + 1})
		}
	}
	return out
}

// Upstream are the dependencies of id, transitively.
func (g *Graph) Upstream(id string) []Hop {
	return g.walk(id, g.up, func(e Edge) string { return e.Dst }, false)
}

// Downstream are the products depending on id, transitively.
func (g *Graph) Downstream(id string, strongOnly bool) []Hop {
	return g.walk(id, g.down, func(e Edge) string { return e.Src }, strongOnly)
}

// Impact are the products a vulnerability of id flows into.
func (g *Graph) Impact(id string) []string {
	var out []string
	for _, h := range g.Downstream(id, true) {
		out = append(out, h.Edge.Src)
	}
	return out
}

// Direct are the direct dependencies of id.
func (g *Graph) Direct(id string) []Edge { return g.up[id] }

// Tree draws the dependencies (up) or consumers (down) of id.
func (g *Graph) Tree(id string, up bool) []string {
	index := g.down
	if up {
		index = g.up
	}
	lines := []string{g.names[id]}
	var rec func(cur, prefix string, seen map[string]bool)
	rec = func(cur, prefix string, seen map[string]bool) {
		edges := index[cur]
		for i, e := range edges {
			n := e.Src
			if up {
				n = e.Dst
			}
			last := i == len(edges)-1
			branch, more := "├─ ", "│  "
			if last {
				branch, more = "└─ ", "   "
			}
			cyc := ""
			if seen[n] {
				cyc = " ↺"
			}
			lines = append(lines, prefix+branch+g.names[n]+" ("+e.Kind+")"+cyc)
			if cyc == "" {
				next := map[string]bool{n: true}
				for k := range seen {
					next[k] = true
				}
				rec(n, prefix+more, next)
			}
		}
	}
	rec(id, "", map[string]bool{id: true})
	return lines
}
