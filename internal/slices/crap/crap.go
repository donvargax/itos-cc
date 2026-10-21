// Package crap is the crap command, a slice named for it: its kong struct
// and Run are the UI, and its Query and Handler the application.
//
// Run assembles the Query from the command line, the one copy of a command
// line's values, and hands it to the Handler with the context cmd made.
// The Handler only wires: the selection domain chooses the sources through
// the Tree port it was given (Select), then the coverage Measurer it was given
// measures them as the coverage flags ask, the selection domain parses
// each, read through the same Tree and parsed by lang, asking the
// Surroundings port what surrounds it, the slice's own domain, its domain
// subpackage, scores each function, and the snapshot domain names each
// file from the project root for its Store port to record (Handle). Run
// writes the worst first, each file named from the working directory, and
// judges the threshold. The slice reads and runs nothing itself. A failure
// is the selection, snapshot or coverage domain's, which cmd's classifiers
// turn into problems, or lang's, an internal one; Run reports it through
// the UI. It has no classifier of its own.
//
// Command declares the command to cmd, which assembles it into its one
// model (ADR-0051), and Ports are the ports its handler is built over,
// which cmd's adapters satisfy.
package crap

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
	langport "github.com/donvargax/itos-cc/internal/lang/port"
	"github.com/donvargax/itos-cc/internal/selection"
	"github.com/donvargax/itos-cc/internal/selection/port"
	"github.com/donvargax/itos-cc/internal/slices/crap/domain"
	"github.com/donvargax/itos-cc/internal/snapshot"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// Command is the crap command as cmd assembles it: its name, the line the
// top-level help lists it by, and a fresh command line for each parser.
var Command = cli.Declaration{
	Name: "crap",
	Help: "CRAP score per function: complexity × untested risk",
	New:  func() any { return &CLI{} },
}

// CLI is the crap command's command line: the files it selects, as every
// command selects them (cli.Selection, first), how coverage is measured,
// the score above which it says no, how many functions it prints, and
// whether it prints lines or the one object a script reads. Threshold is
// nil when it is not given, which differs from a threshold of 0.
// --coverage-report may be given again, each a report, a comma no
// separator (sep none).
type CLI struct {
	cli.Selection
	NoCoverage          bool     `name:"no-coverage" help:"skip coverage; complexity only"`
	UseExistingCoverage bool     `name:"use-existing-coverage" help:"read reports already on disk instead of running tests"`
	AllTests            bool     `name:"all-tests" help:"run the whole test suite, integration and end-to-end tests included"`
	CoverageCommand     string   `name:"coverage-command" placeholder:"CMD" help:"run this shell command instead of the per-language commands (with --coverage-report)"`
	CoverageReport      []string `name:"coverage-report" placeholder:"FILE" sep:"none" help:"read this LCOV, Go cover profile, or JaCoCo XML report"`
	Threshold           *float64 `placeholder:"N" help:"say no (exit 1) when a function scores above N"`
	Top                 int      `placeholder:"N" help:"print only the N worst functions"`
	JSON                bool     `name:"json" help:"print one JSON object on stdout instead of text (shape below)"`
}

// Contract is what crap's help says beyond its flags: what it scores, the
// --json object a script reads, its problem rules, and every exit code it
// can return. A scenario checks the help and a script reads it.
func (CLI) Contract() cli.Contract {
	return cli.Contract{
		About: `
Scores each function: CRAP = CC² × (1 − coverage)³ + CC. 1–5 is low risk,
5–30 is worth a look, 30+ is complex and under-tested. Runs each language's
tests with coverage unless told otherwise, prints the worst first, and writes
.metrics/crap.json.

With paths or --changed, Go and TypeScript coverage runs only the tests that
load those files; Python, Kotlin and Dart run their whole suite. A build root whose coverage could not be measured shows
N/A, not 0%: a warning, and with --threshold a problem, since the threshold
cannot be checked for it. A file no test loads is 0%, untested, when the
coverage run for its language succeeded, even with paths that name it alone.

The sources itos-cc.yaml's exclude names, globs from the project root such
as **/porttest/**, are left out, however they are chosen: a path or
--changed naming one selects nothing.`,
		JSON: `"entries": [{"namespace", "name", "language", "file", "start_line",
   "end_line", "complexity", "coverage", "crap"}], "unmeasured": [{"dir",
   "language", "report", "cause", "reason"}]`,
		Rules: []string{
			"crap.threshold             a function scores above --threshold",
			"coverage.measured-nothing  with --threshold, a coverage run measured none of its files",
			"coverage.tool-missing      with --threshold, a tool coverage needs is not installed",
			"coverage.no-report         with --threshold and --use-existing-coverage, no report measures the files",
			"coverage.report-unreadable with --threshold, a --coverage-report cannot be read",
			"coverage.cleanup-failed    a coverage command's owned process group did not end by its deadline: process_group, command",
			"interrupted                SIGINT or SIGTERM stopped it",
			"config.invalid             itos-cc.yaml cannot be read: file, and key for an exclude glob",
		},
		Exits: []cli.Exit{
			{Code: 0, Meaning: "success"},
			{Code: 1, Meaning: "a function scores above --threshold, or with --threshold, the tests measured nothing"},
			{Code: cli.CodeUsage, Meaning: "a usage or config error: a bad flag, path, or --coverage-report, or an itos-cc.yaml that cannot be read"},
			{Code: cli.CodeEnvironment, Meaning: "with --threshold, a coverage tool or report is missing; --changed outside git"},
		},
		Examples: []string{
			"itos-cc crap --top 20",
			"itos-cc crap --changed --threshold 30 --json",
		},
	}
}

// Query is what the crap command asks: the scores of the sources paths and
// changed select, their coverage measured as Coverage asks, or none
// without Measure (--no-coverage).
type Query struct {
	Paths   []string
	Changed bool
	Measure bool
	// Coverage is how to measure: its Sources are what Select chose.
	Coverage covport.Request
}

// Handler is the crap command's application: the working tree it selects
// and reads files through, what surrounds those files, which lang asks
// about to parse them, the store its snapshot is recorded in, the coverage
// port it measures through, Name, the name a person reads a file by, from
// the working directory, and Exclude, the sources the project's settings
// leave out. cmd gives each through Ports (NewHandler), choosing the
// adapters behind them and reading the settings. No value of a command
// line is read here.
type Handler struct {
	Tree         port.Tree
	Surroundings langport.Surroundings
	Store        snapport.Store
	Coverage     coverage.Measurer
	Name         func(path string) string
	Exclude      selection.Exclusion
}

// Ports are what the crap command reaches the world through, which cmd's
// adapters give it: the working tree it selects and reads files through,
// what surrounds those files, the store its snapshot is recorded in, the
// coverage domain's Measurer, which runs each language's tests or reads
// the reports the coverage flags name, Rel, a path named from the working
// directory as a person reads it, and the sources the project's settings
// exclude, which cmd reads under the context of the Run that asks for the
// handler and whose failure it words as a person reads it.
type Ports interface {
	SelectionTree() port.Tree
	LangSurroundings() langport.Surroundings
	SnapshotStore() snapport.Store
	CoverageMeasurer() coverage.Measurer
	Rel(path string) string
	SelectionExclusion(ctx context.Context) (selection.Exclusion, error)
}

// NewHandler is the handler over p's tree, surroundings, store and coverage
// Measurer, naming each file by p's Rel and leaving out the sources p's
// settings exclude, read under ctx; their failure is returned as p gave it,
// never wrapped, so the problem cmd worded is the one the person reads.
func NewHandler(ctx context.Context, p Ports) (Handler, error) {
	exclude, err := p.SelectionExclusion(ctx)
	if err != nil {
		return Handler{}, err
	}
	return Handler{Tree: p.SelectionTree(), Surroundings: p.LangSurroundings(), Store: p.SnapshotStore(),
		Coverage: p.CoverageMeasurer(), Name: p.Rel, Exclude: exclude}, nil
}

// Select is the sources and tests q's paths and changed select, less the
// sources Exclude leaves out, which it counts, under ctx.
func (h Handler) Select(ctx context.Context, q Query) (selection.Files, error) {
	return selection.Select(ctx, h.Tree, q.Paths, q.Changed, h.Exclude)
}

// Found is what Handle found: every function of the sources scored, each
// file by Name, and each build root or report coverage did not measure.
type Found struct {
	Entries    []domain.Entry
	Unmeasured []coverage.Unmeasured
}

// Handle scores every function of sources, which Select chose, by the
// coverage q asks for, measured through the coverage Measurer, and records
// the scores in crap.json, each file by its path from the project root
// (snapshot.Key), passing ctx on to the domains, lang and the ports. A
// failure to measure, read or record stops it with nothing found.
func (h Handler) Handle(ctx context.Context, q Query, sources []string) (Found, error) {
	var report *coverage.Report
	if q.Measure {
		req := q.Coverage
		req.Sources = sources
		measured, failure := h.Coverage.Measure(ctx, req)
		if failure != nil {
			return Found{}, failure
		}
		report = measured
	}
	root, failure := h.Store.Root(ctx)
	if failure != nil {
		return Found{}, failure
	}
	measured := domain.Measured(report, sources)
	var entries, recorded []domain.Entry
	for _, path := range sources {
		f, err := selection.Parse(ctx, h.Tree, h.Surroundings, path)
		if err != nil {
			return Found{}, err
		}
		scored := domain.Analyze(f, h.Name(path), report, measured)
		f.Close()
		entries = append(entries, scored...)
		for _, e := range scored {
			e.File = snapshot.Key(root, path)
			recorded = append(recorded, e)
		}
	}
	if failure := h.Store.Write(ctx, "crap.json", snapshotOf{Version: snapshot.Version, Entries: recorded}); failure != nil {
		return Found{}, failure
	}
	return Found{Entries: entries, Unmeasured: report.Missing()}, nil
}

// snapshotOf is crap.json: the format version and every function scored,
// each file named from the project root.
type snapshotOf struct {
	Version int            `json:"version"`
	Entries []domain.Entry `json:"entries"`
}

// result is the object --json prints beside schema and ok: the functions
// printed, worst first, and each build root or report coverage did not
// measure, its folder from the working directory.
type result struct {
	Entries    []domain.Entry        `json:"entries"`
	Unmeasured []coverage.Unmeasured `json:"unmeasured"`
}

// Run scores the functions the command line selects, through h and with
// the context cmd made, and prints the worst first, or the object; with
// --threshold it reports each function above it, and each build root or
// report coverage did not measure, since the threshold cannot be checked
// for it, and without it warns of those. A failure is reported with the
// empty object. It returns the exit code the UI gives it.
func (c CLI) Run(ctx context.Context, ui *cli.UI, h Handler) int {
	out := result{Entries: []domain.Entry{}, Unmeasured: []coverage.Unmeasured{}}
	q := c.query()
	chosen, err := h.Select(ctx, q)
	if err != nil {
		return ui.Emit(c.JSON, out, ui.Problems(err))
	}
	if chosen.Excluded > 0 {
		ui.Line(fmt.Sprintf("%d sources left out by exclude in itos-cc.yaml", chosen.Excluded))
	}
	if len(chosen.Sources) == 0 {
		ui.Line("no source files to score")
		return ui.Emit(c.JSON, out, nil)
	}
	if q.Measure && c.CoverageCommand != "" && len(c.CoverageReport) == 0 {
		return ui.Emit(c.JSON, out, []*cli.Problem{cli.CommandNeedsReport()})
	}
	found, err := h.Handle(ctx, q, chosen.Sources)
	if err != nil {
		return ui.Emit(c.JSON, out, ui.Problems(err))
	}
	worst := domain.Worst(found.Entries)
	if c.Top > 0 && c.Top < len(worst) {
		worst = worst[:c.Top]
	}
	out.Entries = worst
	for _, m := range found.Unmeasured {
		if m.Dir != "" {
			m.Dir = ui.Rel(m.Dir)
		}
		out.Unmeasured = append(out.Unmeasured, m)
	}
	if !c.JSON {
		printTable(ui.Stdout, worst)
	}
	if c.Threshold == nil {
		// A coverage command whose owned group did not end is a problem
		// with or without one (ADR-0021).
		var problems []*cli.Problem
		for _, m := range out.Unmeasured {
			if _, failed := m.Cause.(covport.CleanupFailed); failed {
				problems = append(problems, ui.Problems(m)...)
				continue
			}
			ui.Line("no coverage for " + m.String())
		}
		return ui.Emit(c.JSON, out, problems)
	}
	// A threshold cannot be checked for code coverage did not measure.
	var problems []*cli.Problem
	for _, m := range out.Unmeasured {
		problems = append(problems, ui.Problems(m)...)
	}
	threshold := *c.Threshold
	for _, e := range domain.Worst(found.Entries) {
		if e.CRAP == nil || *e.CRAP <= threshold {
			continue
		}
		problems = append(problems, cli.NewProblem(cli.CodeNo, "crap.threshold",
			fmt.Sprintf("%s#%s scores %.1f, above the threshold %.1f", e.Namespace, e.Name, *e.CRAP, threshold),
			"Cover it with tests or split it.").
			With("file", e.File).With("line", e.StartLine).With("function", e.Namespace+"#"+e.Name).
			With("crap", *e.CRAP).With("threshold", threshold))
	}
	return ui.Emit(c.JSON, out, problems)
}

// query is the Query of the command line: with paths or --changed, Go and
// TypeScript coverage runs only the tests that load the sources, and
// otherwise every test.
func (c CLI) query() Query {
	scope := covport.AllTests
	if len(c.Paths) > 0 || c.Changed {
		scope = covport.RelatedTests
	}
	return Query{Paths: c.Paths, Changed: c.Changed, Measure: !c.NoCoverage, Coverage: covport.Request{
		Reports: c.CoverageReport, Command: c.CoverageCommand, Existing: c.UseExistingCoverage, AllTests: c.AllTests, Scope: scope,
	}}
}

// printTable writes entries to w as crap's table: the CRAP, CC and COV%
// columns aligned right, then the function and where it starts.
func printTable(w io.Writer, entries []domain.Entry) {
	t := tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.AlignRight)
	_, _ = fmt.Fprintln(t, "CRAP\tCC\tCOV%\t \t")
	for _, e := range entries {
		score, pct := "N/A", "N/A"
		if e.CRAP != nil {
			score = fmt.Sprintf("%.1f", *e.CRAP)
			pct = fmt.Sprintf("%.1f", *e.Coverage)
		}
		_, _ = fmt.Fprintf(t, "%s\t%d\t%s\t \t%s#%s  %s:%d\n", score, e.Complexity, pct, e.Namespace, e.Name, e.File, e.StartLine)
	}
	_ = t.Flush()
}
