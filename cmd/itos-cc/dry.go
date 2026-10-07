package main

import (
	"flag"
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

func runDry(args []string) int {
	fs := flag.NewFlagSet("dry", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), `usage: itos-cc dry [options] [path ...]

Finds functions in the same language whose normalized structure is similar
enough to review as duplicates, grouping those linked by similar pairs. Local names, field names, and literals
do not count; called functions, operators, and the shape of the code do.

With paths or --changed, those files are compared against every source under
the working directory, so a change is checked against the whole project.
Writes .metrics/dry.json.

`)
		fs.PrintDefaults()
	}
	var sel selection
	sel.register(fs)
	opt := dry.Defaults
	fs.Float64Var(&opt.Threshold, "threshold", opt.Threshold, "minimum similarity, 0–1")
	fs.IntVar(&opt.MinLines, "min-lines", opt.MinLines, "skip functions shorter than this many lines")
	fs.IntVar(&opt.MinNodes, "min-nodes", opt.MinNodes, "skip functions with fewer normalized syntax nodes")
	asJSON := fs.Bool("json", false, "print the snapshot as JSON instead of text")
	paths, err := parse(fs, args)
	if err != nil {
		return parseExit(err)
	}

	focusFiles, err := sel.files(paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, "itos-cc:", err)
		return exitUsage
	}
	all := focusFiles
	focus := map[string]bool{}
	if len(paths) > 0 || sel.changed {
		for _, f := range focusFiles.Sources {
			focus[f] = true
		}
		if len(focus) == 0 {
			fmt.Fprintln(os.Stderr, "itos-cc: no source files to check")
			return exitOK
		}
		if all, err = project.Discover([]string{"."}); err != nil {
			fmt.Fprintln(os.Stderr, "itos-cc:", err)
			return exitUsage
		}
		all.Sources = union(all.Sources, focusFiles.Sources)
	}
	forms, err := dry.Forms(all.Sources, opt)
	if err != nil {
		fmt.Fprintln(os.Stderr, "itos-cc:", err)
		return exitUsage
	}
	snapshot := drySnapshot{Version: metrics.Version, Threshold: opt.Threshold, Candidates: dry.Compare(forms, focus, opt)}
	if snapshot.Candidates == nil {
		snapshot.Candidates = []dry.Candidate{}
	}
	snapshot.Groups = dry.Groups(snapshot.Candidates)
	if err := metrics.Write("dry.json", snapshot); err != nil {
		fmt.Fprintln(os.Stderr, "itos-cc:", err)
		return exitUsage
	}
	if *asJSON {
		printJSON(snapshot)
		return exitOK
	}
	for _, g := range snapshot.Groups {
		score := fmt.Sprintf("%.2f", g.MaxScore)
		if g.MinScore != g.MaxScore {
			score = fmt.Sprintf("%.2f–%.2f", g.MinScore, g.MaxScore)
		}
		fmt.Printf("DUPLICATE score=%s %s\n", score, g.Language)
		for _, s := range g.Members {
			fmt.Printf("  %s:%d-%d  %s#%s\n", s.File, s.StartLine, s.EndLine, s.Namespace, s.Name)
		}
	}
	return exitOK
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
