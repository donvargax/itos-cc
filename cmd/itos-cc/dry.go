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
	if err := metrics.Write("dry.json", drySnapshot{Version: metrics.Version, Threshold: opt.Threshold,
		Candidates: result.Candidates, Groups: result.Groups}); err != nil {
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
