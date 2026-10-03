package main

import (
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/donvargax/itos-cc/crap"
	"github.com/donvargax/itos-cc/metrics"
)

type crapSnapshot struct {
	Version int          `json:"version"`
	Entries []crap.Entry `json:"entries"`
}

func runCrap(args []string) int {
	fs := flag.NewFlagSet("crap", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), `usage: itos-cc crap [options] [path ...]

Scores each function: CRAP = CC² × (1 − coverage)³ + CC. 1–5 is low risk,
5–30 is worth a look, 30+ is complex and under-tested. Runs each language's
tests with coverage unless told otherwise, prints the worst first, and writes
.metrics/crap.json.

`)
		fs.PrintDefaults()
	}
	var sel selection
	var cov coverageOptions
	sel.register(fs)
	cov.register(fs)
	threshold := fs.Float64("threshold", 0, "exit 2 when any CRAP score is above this (0 disables)")
	asJSON := fs.Bool("json", false, "print the snapshot as JSON instead of a table")
	top := fs.Int("top", 0, "print only the N worst functions (0 prints all)")
	paths, err := parse(fs, args)
	if err != nil {
		return exitUsage
	}

	files, err := sel.files(paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, "itos-cc:", err)
		return exitUsage
	}
	if len(files.Sources) == 0 {
		fmt.Fprintln(os.Stderr, "itos-cc: no source files to score")
		return exitOK
	}
	report, err := cov.load(files.Sources, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "itos-cc:", err)
		return exitUsage
	}
	entries, err := crap.Analyze(files.Sources, report)
	if err != nil {
		fmt.Fprintln(os.Stderr, "itos-cc:", err)
		return exitUsage
	}
	snapshot := crapSnapshot{Version: metrics.Version, Entries: entries}
	if err := metrics.Write("crap.json", snapshot); err != nil {
		fmt.Fprintln(os.Stderr, "itos-cc:", err)
		return exitUsage
	}

	worst := crap.Worst(entries)
	if *top > 0 && *top < len(worst) {
		worst = worst[:*top]
	}
	if *asJSON {
		printJSON(crapSnapshot{Version: metrics.Version, Entries: worst})
	} else {
		printCrapTable(worst)
	}
	if *threshold > 0 && len(entries) > 0 {
		if w := crap.Worst(entries)[0]; w.CRAP != nil && *w.CRAP > *threshold {
			fmt.Fprintf(os.Stderr, "itos-cc: %s#%s scores %.1f, above the threshold %.1f\n",
				w.Namespace, w.Name, *w.CRAP, *threshold)
			return exitThreshold
		}
	}
	return exitOK
}

func printCrapTable(entries []crap.Entry) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(w, "CRAP\tCC\tCOV%\t \t")
	for _, e := range entries {
		score, pct := "N/A", "N/A"
		if e.CRAP != nil {
			score = fmt.Sprintf("%.1f", *e.CRAP)
			pct = fmt.Sprintf("%.1f", *e.Coverage)
		}
		fmt.Fprintf(w, "%s\t%d\t%s\t \t%s#%s  %s:%d\n", score, e.Complexity, pct, e.Namespace, e.Name, e.File, e.StartLine)
	}
	w.Flush()
}
