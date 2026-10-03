package graph

import (
	"sort"

	"github.com/donvargax/itos-cc/lang"
)

// endpoint is a route or request of one module.
type endpoint struct {
	module string
	lang.Endpoint
}

// httpEdges links each module's HTTP requests to the modules that serve a
// matching route, across every repository: a frontend in one repository
// calling a service in another is one edge. A request that matches routes in
// several modules links to each, since the host it is sent to is rarely
// known from source.
func httpEdges(repos []*repoGraph) []Edge {
	var serves, calls []endpoint
	for _, g := range repos {
		for _, f := range g.files {
			id := g.moduleID(f)
			for _, e := range f.serves {
				serves = append(serves, endpoint{id, e})
			}
			for _, e := range f.calls {
				calls = append(calls, endpoint{id, e})
			}
		}
	}
	type key struct{ from, to string }
	found := map[key]*Edge{}
	for _, c := range calls {
		for _, s := range serves {
			if s.module == c.module || !lang.Matches(s.Endpoint, c.Endpoint) {
				continue
			}
			k := key{c.module, s.module}
			e, ok := found[k]
			if !ok {
				e = &Edge{From: c.module, To: s.module, Kind: "http"}
				found[k] = e
			}
			e.Count++
			e.Via = appendUnique(e.Via, s.Endpoint.String())
		}
	}
	var out []Edge
	for _, e := range found {
		sort.Strings(e.Via)
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	return out
}
