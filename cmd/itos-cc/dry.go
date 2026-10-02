package main

import (
	"flag"
	"fmt"
	"os"

	"itos-cc/dry"
	"itos-cc/metrics"
	"itos-cc/project"
)

type drySnapshot struct {
	Version    int             `json:"version"`
	Threshold  float64         `json:"threshold"`
	Candidates []dry.Candidate `json:"candidates"`
}

func runDry(args []string) int {
	fs := flag.NewFlagSet("dry", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), `usage: itos-cc dry [options] [path ...]

Finds pairs of functions in the same language whose normalized structure is
similar enough to review as duplicates. Local names, field names, and literals
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
		return exitUsage
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
	if err := metrics.Write("dry.json", snapshot); err != nil {
		fmt.Fprintln(os.Stderr, "itos-cc:", err)
		return exitUsage
	}
	if *asJSON {
		printJSON(snapshot)
		return exitOK
	}
	for _, c := range snapshot.Candidates {
		fmt.Printf("DUPLICATE score=%.2f %s\n", c.Score, c.Language)
		for _, s := range []dry.Side{c.Left, c.Right} {
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
