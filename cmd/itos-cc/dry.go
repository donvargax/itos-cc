package main

import (
	"context"
	"fmt"
	"os"

	"github.com/donvargax/itos-cc/internal/duplication"
	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/snapshot"
	"github.com/donvargax/itos-cc/internal/workdir"
	"github.com/donvargax/itos-cc/project"
)

type drySnapshot struct {
	Version    int                     `json:"version"`
	Threshold  float64                 `json:"threshold"`
	Candidates []duplication.Candidate `json:"candidates"`
	Groups     []duplication.Group     `json:"groups"`
}

var dryCommand = &command{
	name:     "dry",
	summary:  "functions similar enough to be duplicates",
	synopsis: "[options] [path ...]",
	about: `
Finds functions in the same language whose normalized structure is similar
enough to review as duplicates, grouping those linked by similar pairs.
Local names, field names, and literals do not count; called functions,
operators, and the shape of the code do.

With paths or --changed, those files are compared against every source under
the working directory, so a change is checked against the whole project.
Writes .metrics/dry.json.`,
	flags: append(append([]flagSpec{}, selectionFlags...),
		opt("threshold", floatFlag, "N", fmt.Sprint(duplication.Defaults.Threshold), "minimum similarity, 0–1"),
		opt("min-lines", intFlag, "N", fmt.Sprint(duplication.Defaults.MinLines), "skip functions shorter than N lines"),
		opt("min-nodes", intFlag, "N", fmt.Sprint(duplication.Defaults.MinNodes), "skip functions with fewer than N normalized syntax nodes")),
	json: `"threshold", "candidates": [{"score", "language", "left", "right"}],
   "groups": [{"language", "min_score", "max_score", "members": [{"file",
   "start_line", "end_line", "namespace", "name", "nodes"}]}]`,
	exits: []exitDoc{
		{0, "success, duplicates or not"},
		{2, "a usage error: a bad flag or path"},
		{3, "--changed outside a git repository"},
	},
	examples: []string{
		"itos-cc dry",
		"itos-cc dry --changed --json",
	},
	run: runDry,
}

type dryResult struct {
	Threshold  float64                 `json:"threshold"`
	Candidates []duplication.Candidate `json:"candidates"`
	Groups     []duplication.Group     `json:"groups"`
}

func runDry(in *invocation) (any, error) {
	opt := duplication.Options{Threshold: in.float("threshold"), MinLines: in.integer("min-lines"), MinNodes: in.integer("min-nodes")}
	result := dryResult{Threshold: opt.Threshold, Candidates: []duplication.Candidate{}, Groups: []duplication.Group{}}
	focusFiles, err := files(in)
	if err != nil {
		return result, err
	}
	all := focusFiles
	focus := map[string]bool{}
	if len(in.args) > 0 || in.set("changed") {
		for _, f := range focusFiles.Sources {
			focus[f] = true
		}
		if len(focus) == 0 {
			fmt.Fprintln(os.Stderr, "itos-cc: no source files to check")
			return result, nil
		}
		if all, err = project.Discover([]string{"."}); err != nil {
			return result, err
		}
		all.Sources = union(all.Sources, focusFiles.Sources)
	}
	ctx := context.Background()
	forms, err := dryForms(ctx, all.Sources, opt)
	if err != nil {
		return result, err
	}
	if c := duplication.Compare(forms, focus, opt); c != nil {
		result.Candidates = c
	}
	result.Groups = duplication.Groups(result.Candidates)
	root, failure := snapshots.Root(ctx)
	if failure != nil {
		return result, failure
	}
	candidates, groups := dryFromRoot(result.Candidates, result.Groups, dryKeys(root, all.Sources))
	if failure := snapshots.Write(ctx, "dry.json", drySnapshot{Version: snapshot.Version, Threshold: opt.Threshold,
		Candidates: candidates, Groups: groups}); failure != nil {
		return result, failure
	}
	if in.json {
		return result, nil
	}
	for _, g := range result.Groups {
		score := fmt.Sprintf("%.2f", g.MaxScore)
		if g.MinScore != g.MaxScore {
			score = fmt.Sprintf("%.2f–%.2f", g.MinScore, g.MaxScore)
		}
		fmt.Printf("DUPLICATE score=%s %s\n", score, g.Language)
		for _, s := range g.Members {
			fmt.Printf("  %s:%d-%d  %s#%s\n", s.File, s.StartLine, s.EndLine, s.Namespace, s.Name)
		}
	}
	return result, nil
}

// dryForms is the forms of sources, each read through the selection's tree
// and parsed with internal/lang through workdir's Surroundings, its sides
// named from the working directory (project.Rel), as output prints them. A
// file that cannot be read or parsed stops it.
func dryForms(ctx context.Context, sources []string, opt duplication.Options) ([]duplication.Form, error) {
	var forms []duplication.Form
	for _, path := range sources {
		f, err := parseSource(ctx, path)
		if err != nil {
			return nil, err
		}
		forms = append(forms, duplication.Forms(f, project.Rel(path), opt)...)
		f.Close()
	}
	return forms, nil
}

// parseSource is the source at path parsed: a language lang does not know
// is refused before anything is read, then the selection's tree reads it
// and lang parses it, naming it through workdir's Surroundings.
func parseSource(ctx context.Context, path string) (*lang.File, error) {
	spec := lang.Detect(path)
	if spec == nil {
		return nil, fmt.Errorf("unsupported language: %s", path)
	}
	src, failure := workdir.Tree{}.Read(ctx, path)
	if failure != nil {
		return nil, failure
	}
	return lang.Parse(ctx, workdir.Surroundings{}, spec, path, src)
}

// union merges sorted path lists without duplicates; paths named outside the
// working directory still take part.
func union(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, list := range [][]string{a, b} {
		for _, p := range list {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}

// dryKeys is, by the name dry gives each of sources (project.Rel, from the
// working directory), the source as dry.json records it: from root, the
// project root (snapshot.Key). Every side dry finds is of a file of
// sources, which are absolute as the selection discovered them.
func dryKeys(root string, sources []string) map[string]string {
	keys := make(map[string]string, len(sources))
	for _, path := range sources {
		keys[project.Rel(path)] = snapshot.Key(root, path)
	}
	return keys
}

// dryFromRoot is candidates and groups as dry.json records them: each file
// by its key, from the project root, while output names it from the
// working directory.
func dryFromRoot(candidates []duplication.Candidate, groups []duplication.Group, keys map[string]string) ([]duplication.Candidate, []duplication.Group) {
	side := func(s duplication.Side) duplication.Side {
		s.File = keys[s.File]
		return s
	}
	var cs []duplication.Candidate
	if candidates != nil {
		cs = []duplication.Candidate{}
	}
	for _, c := range candidates {
		c.Left, c.Right = side(c.Left), side(c.Right)
		cs = append(cs, c)
	}
	var gs []duplication.Group
	if groups != nil {
		gs = []duplication.Group{}
	}
	for _, g := range groups {
		members := make([]duplication.Side, 0, len(g.Members))
		for _, m := range g.Members {
			members = append(members, side(m))
		}
		g.Members = members
		gs = append(gs, g)
	}
	return cs, gs
}
