package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/crap"
	"github.com/donvargax/itos-cc/metrics"
	"github.com/donvargax/itos-cc/project"
)

type crapSnapshot struct {
	Version int          `json:"version"`
	Entries []crap.Entry `json:"entries"`
}

var crapCommand = &command{
	name:     "crap",
	summary:  "CRAP score per function: complexity × untested risk",
	synopsis: "[options] [path ...]",
	about: `
Scores each function: CRAP = CC² × (1 − coverage)³ + CC. 1–5 is low risk,
5–30 is worth a look, 30+ is complex and under-tested. Runs each language's
tests with coverage unless told otherwise, prints the worst first, and writes
.metrics/crap.json.

With paths or --changed, Go and TypeScript coverage runs only the tests that
load those files. A build root whose coverage could not be measured shows
N/A, not 0%: a warning, and with --threshold a problem, since the threshold
cannot be checked for it. A file no test loads is 0%, untested, when the
coverage run for its language succeeded, even with paths that name it alone.`,
	flags: append(append(append([]flagSpec{}, selectionFlags...), coverageFlags...),
		opt("threshold", floatFlag, "N", "", "say no (exit 1) when a function scores above N"),
		opt("top", intFlag, "N", "", "print only the N worst functions")),
	json: `"entries": [{"namespace", "name", "language", "file", "start_line",
   "end_line", "complexity", "coverage", "crap"}], "unmeasured": [{"dir",
   "language", "report", "cause", "reason"}]`,
	rules: []string{
		"crap.threshold             a function scores above --threshold",
		"coverage.measured-nothing  with --threshold, a coverage run measured none of its files",
		"coverage.tool-missing      with --threshold, a tool coverage needs is not installed",
		"coverage.no-report         with --threshold and --use-existing-coverage, no report measures the files",
		"coverage.report-unreadable with --threshold, a --coverage-report cannot be read",
	},
	exits: []exitDoc{
		{0, "success"},
		{1, "a function scores above --threshold, or with --threshold, the tests measured nothing"},
		{2, "a usage error: a bad flag, path, or --coverage-report"},
		{3, "with --threshold, a coverage tool or report is missing; --changed outside git"},
	},
	examples: []string{
		"itos-cc crap --top 20",
		"itos-cc crap --changed --threshold 30 --json",
	},
	run: runCrap,
}

type crapResult struct {
	Entries    []crap.Entry          `json:"entries"`
	Unmeasured []coverage.Unmeasured `json:"unmeasured"`
}

func runCrap(in *invocation) (any, error) {
	result := crapResult{Entries: []crap.Entry{}, Unmeasured: []coverage.Unmeasured{}}
	files, err := files(in)
	if err != nil {
		return result, err
	}
	if len(files.Sources) == 0 {
		fmt.Fprintln(os.Stderr, "itos-cc: no source files to score")
		return result, nil
	}
	scope := coverage.AllTests
	if len(in.args) > 0 || in.set("changed") {
		scope = coverage.RelatedTests
	}
	report, err := loadCoverage(in, files.Sources, scope, os.Stderr, nil)
	if err != nil {
		return result, err
	}
	entries, err := crap.Analyze(files.Sources, report)
	if err != nil {
		return result, err
	}
	// crap.json names each file from the project root, while output names
	// it from the working directory.
	var recorded []crap.Entry
	for _, e := range entries {
		e.File = project.FromRoot(e.File)
		recorded = append(recorded, e)
	}
	if err := metrics.Write("crap.json", crapSnapshot{Version: metrics.Version, Entries: recorded}); err != nil {
		return result, err
	}

	worst := crap.Worst(entries)
	if n := in.integer("top"); n > 0 && n < len(worst) {
		worst = worst[:n]
	}
	result.Entries = worst
	result.Unmeasured = relativeUnmeasured(report)
	if !in.json {
		printCrapTable(worst)
	}
	threshold := in.float("threshold")
	if !in.set("threshold") {
		for _, m := range result.Unmeasured {
			fmt.Fprintf(os.Stderr, "itos-cc: no coverage for %s\n", m)
		}
		return result, nil
	}
	// A threshold cannot be checked for code coverage did not measure.
	for _, p := range unmeasured(report) {
		in.report(p)
	}
	for _, e := range crap.Worst(entries) {
		if e.CRAP == nil || *e.CRAP <= threshold {
			continue
		}
		in.report(fail(kindNo, "crap.threshold",
			fmt.Sprintf("%s#%s scores %.1f, above the threshold %.1f", e.Namespace, e.Name, *e.CRAP, threshold),
			"Cover it with tests or split it.").
			with("file", e.File).with("line", e.StartLine).with("function", e.Namespace+"#"+e.Name).
			with("crap", *e.CRAP).with("threshold", threshold))
	}
	return result, nil
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
