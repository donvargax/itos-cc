package main

import (
	"fmt"
	"os"

	"github.com/donvargax/itos-cc/dry"
	"github.com/donvargax/itos-cc/metrics"
	"github.com/donvargax/itos-cc/project"
)

type drySnapshot struct {
	Version    int             `json:"version"`
	Threshold  float64         `json:"threshold"`
	Candidates []dry.Candidate `json:"candidates"`
	Groups     []dry.Group     `json:"groups"`
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
		opt("threshold", floatFlag, "N", fmt.Sprint(dry.Defaults.Threshold), "minimum similarity, 0–1"),
		opt("min-lines", intFlag, "N", fmt.Sprint(dry.Defaults.MinLines), "skip functions shorter than N lines"),
		opt("min-nodes", intFlag, "N", fmt.Sprint(dry.Defaults.MinNodes), "skip functions with fewer than N normalized syntax nodes")),
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
	Threshold  float64         `json:"threshold"`
	Candidates []dry.Candidate `json:"candidates"`
	Groups     []dry.Group     `json:"groups"`
}

func runDry(in *invocation) (any, error) {
	opt := dry.Options{Threshold: in.float("threshold"), MinLines: in.integer("min-lines"), MinNodes: in.integer("min-nodes")}
	result := dryResult{Threshold: opt.Threshold, Candidates: []dry.Candidate{}, Groups: []dry.Group{}}
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
	forms, err := dry.Forms(all.Sources, opt)
	if err != nil {
		return result, err
	}
	if c := dry.Compare(forms, focus, opt); c != nil {
		result.Candidates = c
	}
	result.Groups = dry.Groups(result.Candidates)
	candidates, groups := dryFromRoot(result.Candidates, result.Groups)
	if err := metrics.Write("dry.json", drySnapshot{Version: metrics.Version, Threshold: opt.Threshold,
		Candidates: candidates, Groups: groups}); err != nil {
		return result, err
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

// dryFromRoot is candidates and groups as dry.json records them: each file
// from the project root, while output names it from the working directory.
func dryFromRoot(candidates []dry.Candidate, groups []dry.Group) ([]dry.Candidate, []dry.Group) {
	side := func(s dry.Side) dry.Side {
		s.File = project.FromRoot(s.File)
		return s
	}
	var cs []dry.Candidate
	if candidates != nil {
		cs = []dry.Candidate{}
	}
	for _, c := range candidates {
		c.Left, c.Right = side(c.Left), side(c.Right)
		cs = append(cs, c)
	}
	var gs []dry.Group
	if groups != nil {
		gs = []dry.Group{}
	}
	for _, g := range groups {
		members := make([]dry.Side, 0, len(g.Members))
		for _, m := range g.Members {
			members = append(members, side(m))
		}
		g.Members = members
		gs = append(gs, g)
	}
	return cs, gs
}
