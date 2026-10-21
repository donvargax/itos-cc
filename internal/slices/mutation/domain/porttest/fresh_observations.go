package porttest

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/internal/coverage"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// projectedPath changes only semantic root-owned path identities. External tool
// paths, literal commands/environment values and raw collector bytes stay exact.
func (f *Fresh) projectedPath(p *mutants.FreshPlan, path string) string {
	if !filepath.IsAbs(path) || !inside(p.Root, path) || inside(filepath.Join(p.Root, ".git"), path) {
		return path
	}
	rel, _ := filepath.Rel(p.Root, path)
	return filepath.Join(p.FrozenRoot, rel)
}

func (f *Fresh) projectObservations(p *mutants.FreshPlan) {
	for path, edges := range maps.Clone(f.Inputs.Imports) {
		projected := f.projectedPath(p, path)
		if projected == path {
			continue
		}
		if _, held := f.Inputs.Imports[projected]; held {
			continue
		}
		// Only committed bytes are frozen: a live file the commit lacks has
		// no frozen adjacency, and resolves no frozen import.
		if _, frozen := f.Inputs.Files[projected]; !frozen {
			continue
		}
		var kept []string
		for _, edge := range edges {
			if _, frozen := f.Inputs.Files[f.projectedPath(p, edge)]; frozen {
				kept = append(kept, f.projectedPath(p, edge))
			}
		}
		f.Inputs.Imports[projected] = append([]string{}, kept...)
	}
	for key, outcome := range maps.Clone(f.Inputs.Probes) {
		next := key
		next.Dir = f.projectedPath(p, key.Dir)
		next.Interpreter = f.projectedPath(p, key.Interpreter)
		if next != key {
			f.Inputs.Probes[next] = outcome
		}
	}
	if f.Coverage != nil {
		for group, d := range maps.Clone(f.Coverage.Declarations) {
			next := group
			next.Dir = f.projectedPath(p, group.Dir)
			if next == group {
				continue
			}
			d.Metadata = slices.Clone(d.Metadata)
			if d.Listed != nil {
				d.Listed = maps.Clone(d.Listed)
				for collector, reports := range d.Listed {
					d.Listed[collector] = map[string]*coverage.Report{}
					for id, report := range reports {
						d.Listed[collector][id] = f.projectedReport(p, collector, report)
					}
				}
			}
			f.Coverage.Declarations[next] = d
		}
	}
	if f.Commands != nil {
		for dir, commands := range maps.Clone(f.Commands.Tests) {
			next := f.projectedPath(p, dir)
			if next != dir {
				f.Commands.Tests[next] = cloneFreshCommands(commands)
			}
		}
	}
	if listing, ok := f.Lister.(*Lister); ok {
		for dir, commands := range maps.Clone(listing.Listings) {
			next := f.projectedPath(p, dir)
			if next != dir {
				listing.Listings[next] = maps.Clone(commands)
			}
		}
	}
}

// projectedReport is a listed test's report of the live sources, as it
// measures their committed bytes in p's frozen inputs: the same lines, by
// each frozen source's path.
func (f *Fresh) projectedReport(p *mutants.FreshPlan, language string, report *coverage.Report) *coverage.Report {
	lines := map[string][]coverage.Line{}
	for _, source := range p.Files.Sources {
		rel, err := filepath.Rel(p.FrozenRoot, source)
		if err != nil {
			continue
		}
		if measured, ok := report.Lines(filepath.Join(p.Root, rel)); ok {
			lines[source] = measured
		}
	}
	return coverage.FromLines(language, lines)
}

func cloneFreshCommands(commands map[string]Tests) map[string]Tests {
	out := maps.Clone(commands)
	for key, facts := range out {
		facts.Verdicts = slices.Clone(facts.Verdicts)
		if facts.Trace != nil {
			trace := *facts.Trace
			trace.Tests = maps.Clone(trace.Tests)
			facts.Trace = &trace
		}
		out[key] = facts
	}
	return out
}

func (f *Fresh) releaseObservations(root string) {
	for path := range f.Inputs.Imports {
		if inside(root, path) {
			delete(f.Inputs.Imports, path)
		}
	}
	for key := range f.Inputs.Probes {
		if inside(root, key.Dir) {
			delete(f.Inputs.Probes, key)
		}
	}
	if f.Coverage != nil {
		for group := range f.Coverage.Declarations {
			if inside(root, group.Dir) {
				delete(f.Coverage.Declarations, group)
			}
		}
	}
	if f.Commands != nil {
		for dir := range f.Commands.Tests {
			if inside(root, dir) {
				delete(f.Commands.Tests, dir)
			}
		}
	}
	if listing, ok := f.Lister.(*Lister); ok {
		for dir := range listing.Listings {
			if inside(root, dir) {
				delete(listing.Listings, dir)
			}
		}
	}
}

// A root-owned executable follows its frozen installed-link identity. Only the
// requested argv operand is translated for lookup; Shell and other args remain
// literal facts and never undergo blanket replacement.
func (f *Fresh) bindBaseline(p *mutants.FreshPlan, c mutants.Command) {
	if f.Commands == nil || len(c.Args) == 0 || !filepath.IsAbs(c.Args[0]) || !inside(p.FrozenRoot, c.Args[0]) {
		return
	}
	commands := f.Commands.Tests[c.Dir]
	if _, ok := commands[c.String()]; ok {
		return
	}
	rel, _ := filepath.Rel(p.FrozenRoot, c.Args[0])
	args := slices.Clone(c.Args)
	args[0] = filepath.Join(p.Root, rel)
	if facts, ok := commands[strings.Join(args, " ")]; ok {
		commands[c.String()] = facts
	}
}
