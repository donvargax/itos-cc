package architecture_test

import "strings"

// graphAllowed is what the graph slice imports of ours: the shared domains
// and ports it reads whole projects through, the engines it resolves their
// imports and reads its tests' fixtures with, and metricsdir, which names
// the folder it watches for serve.
var graphAllowed = []string{"internal/lang", "internal/lang/port", "internal/selection", "internal/selection/port", "internal/snapshot", "internal/snapshot/port", "internal/imports", "internal/project", "internal/metricsdir"}

// addGraph writes the graph slice's arrow through TestArchitecture's add and
// addAny (ADR-0051, q-95, T-208), its code and tests alike: it walks, parses
// and resolves whole projects through graphAllowed, so slice-packages and
// slice-fakes leave it to a rule of its own. Mutation's judgment reaches it
// through its own port, which cmd binds: the mutation domain, its fake,
// another slice or its domain, an adapter, a port's fake, the engines it
// does not read through, the build's version and cmd are refused, a
// mutation adapter by slice-adapters too, and another slice's package by
// architecture-check's sibling rule too (siblings_test.go).
func addGraph(module, mutation string, fakes []string, add func(name, layer, imported, symbol, rule string, test bool), addAny func(name, layer, imported, symbol string, rules []string, test bool)) {
	const graph = "internal/slices/graph"
	for _, test := range []bool{false, true} {
		kind := "code"
		if test {
			kind = "test"
		}
		for _, target := range []string{mutation, "internal/slices/crap/domain", "internal/slices/units", "internal/slices/mutation", "internal/workdir", "internal/coveragetools", "internal/processes", "internal/covrun", "internal/version", "internal/release", "internal/next", "cmd/probe"} {
			add(strings.ReplaceAll(target, "/", "_")+"_"+kind, graph, module+"/"+target, "Value", "graph", test)
		}
		for _, adapter := range []string{"snapshots", "settings", "testtools"} {
			target := "internal/slices/mutation/adapters/" + adapter
			addAny(strings.ReplaceAll(target, "/", "_")+"_"+kind, graph, module+"/"+target, "Value", []string{"graph", "slice-adapters"}, test)
		}
		for _, fake := range fakes {
			add(strings.ReplaceAll(fake, "/", "_")+"_"+kind, graph, module+"/"+fake, "Value", "graph", test)
		}
		for _, target := range graphAllowed {
			add(strings.ReplaceAll(target, "/", "_")+"_allowed_"+kind, graph, module+"/"+target, "Value", "", test)
		}
		add("os_allowed_"+kind, graph, "os", "ReadFile", "", test)
	}
}
