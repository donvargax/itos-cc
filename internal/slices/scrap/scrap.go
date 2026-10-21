// Package scrap is the scrap command, a slice named for it: its kong struct
// and Run are the UI, and its Query and Handler the application.
//
// Run assembles the Query from the command line, the one copy of a command
// line's values, and hands it to Handle with the context cmd made. Handle
// only wires: the selection domain chooses the test files through the Tree
// port it was given and parses each one, read through the same port and
// parsed by lang, asking the Surroundings port it was given what surrounds
// it, and the slice's own domain, its domain subpackage, measures it and
// merges the run into the last snapshot, which the snapshot domain names
// from the project root and its Store port reads and records. Run writes
// what came back, each file named from the working directory. The slice
// reads no file itself. A failure is the selection domain's or the
// snapshot domain's, which cmd's classifiers turn into problems, or
// lang's, an internal one; Run reports it through the UI. It has no
// classifier of its own.
//
// Command declares the command to cmd, which assembles it into its one
// model (ADR-0051), and Ports are the ports its handler is built over,
// which cmd's adapters satisfy.
package scrap

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/donvargax/itos-cc/internal/cli"
	langport "github.com/donvargax/itos-cc/internal/lang/port"
	"github.com/donvargax/itos-cc/internal/selection"
	"github.com/donvargax/itos-cc/internal/selection/port"
	"github.com/donvargax/itos-cc/internal/slices/scrap/domain"
	"github.com/donvargax/itos-cc/internal/snapshot"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// Command is the scrap command as cmd assembles it: its name, the line the
// top-level help lists it by, and a fresh command line for each parser.
var Command = cli.Declaration{
	Name: "scrap",
	Help: "test-code structure: which tests to leave, table-drive, refactor, or split",
	New:  func() any { return &CLI{} },
}

// CLI is the scrap command's command line: the files it selects, as every
// command selects them (cli.Selection, first), whether it prints every
// example's measurements, and whether it prints lines or the one object a
// script reads.
type CLI struct {
	cli.Selection
	Verbose bool `help:"print every example's measurements"`
	JSON    bool `name:"json" help:"print one JSON object on stdout instead of text (shape below)"`
}

// Contract is what scrap's help says beyond its flags: what it measures,
// the actions it gives, the --json object a script reads, and every exit
// code it can return. A scenario checks the help and a script reads it.
func (CLI) Contract() cli.Contract {
	return cli.Contract{
		About: `
Measures test code: examples that are too large, branch or loop, mock heavily,
assert nothing or too much, or repeat each other. Each file gets an action
for an assistant:

  LEAVE_ALONE       fine as it is; change it only if asked
  AUTO_TABLE_DRIVE  fold repeated examples into one table-driven test
  AUTO_REFACTOR     clean up the listed examples in place
  MANUAL_SPLIT      split the file by responsibility first, then rerun
  REVIEW_FIRST      unstable, but not clear enough to change unattended

Recommendations are decision support, not orders: check each against what
the test is for before changing it. Each run is compared with the previous
.metrics/scrap.json, so rerun after a refactor to see whether it helped.`,
		JSON: `"files": [{"file", "language", "examples", "action", "pressure",
   "average_score", "max_score", "recommendations", "clusters", "details",
   "compare"}]`,
		Exits: []cli.Exit{
			{Code: 0, Meaning: "success, whatever the actions"},
			{Code: cli.CodeUsage, Meaning: "a usage error: a bad flag or path"},
			{Code: cli.CodeEnvironment, Meaning: "--changed outside a git repository"},
		},
		Examples: []string{
			"itos-cc scrap",
			"itos-cc scrap --changed --verbose",
		},
	}
}

// Query is what the scrap command asks: the structure of the test files
// paths and changed select.
type Query struct {
	Paths   []string
	Changed bool
}

// Handler is the scrap command's application: the working tree it selects
// and reads files through, what surrounds those files, which lang asks
// about to parse them, the store its snapshot is read from and recorded
// in, and Name, the name a person reads a file by, from the working
// directory. cmd gives each through Ports (NewHandler), choosing the
// adapters behind them. No value of a command line is read here.
type Handler struct {
	Tree         port.Tree
	Surroundings langport.Surroundings
	Store        snapport.Store
	Name         func(path string) string
}

// Ports are what the scrap command reaches the world through, which cmd's
// adapters give it: the working tree it selects and reads test files
// through, what surrounds those files, the store its snapshot is read from
// and recorded in, and Rel, a path named from the working directory as a
// person reads it. It selects only tests, which itos-cc.yaml's exclude
// never leaves out, so it reads no settings.
type Ports interface {
	SelectionTree() port.Tree
	LangSurroundings() langport.Surroundings
	SnapshotStore() snapport.Store
	Rel(path string) string
}

// NewHandler is the handler over p's tree, surroundings and store, naming
// each file by p's Rel. It reads nothing, so it never fails; its error is
// there for the one shape every slice's constructor has.
func NewHandler(_ context.Context, p Ports) (Handler, error) {
	return Handler{Tree: p.SelectionTree(), Surroundings: p.LangSurroundings(), Store: p.SnapshotStore(), Name: p.Rel}, nil
}

// Found is what Handle found: each measured file's report, the most
// pressing first, compared with the last snapshot's. NoTests is set when
// the selection holds no test file, so nothing was measured or recorded.
// Ignored is the last snapshot when it could not be read, which the run
// went on without.
type Found struct {
	Reports []domain.FileReport
	NoTests bool
	Ignored *snapport.Unreadable
}

// Handle measures the test files q selects and records them in scrap.json,
// beside the files the last snapshot holds that this run did not measure,
// passing ctx on to the domains, lang and the ports. Each report names its
// file by Name, and scrap.json by its path from the project root
// (snapshot.Key). A last snapshot that cannot be read is Ignored; any other
// failure stops Handle with no report, and Ignored still set.
func (h Handler) Handle(ctx context.Context, q Query) (Found, error) {
	chosen, err := selection.Select(ctx, h.Tree, q.Paths, q.Changed, selection.Exclusion{})
	if err != nil {
		return Found{}, err
	}
	if len(chosen.Tests) == 0 {
		return Found{NoTests: true}, nil
	}
	var found Found
	var previous recorded
	if _, failure := h.Store.Read(ctx, "scrap.json", &previous); failure != nil {
		if !errors.As(failure, &found.Ignored) {
			return Found{}, failure
		}
	}
	root, failure := h.Store.Root(ctx)
	if failure != nil {
		return Found{Ignored: found.Ignored}, failure
	}
	history := domain.HistoryOf(previous.Files)
	reports := []domain.FileReport{}
	for _, path := range chosen.Tests {
		f, err := selection.Parse(ctx, h.Tree, h.Surroundings, path)
		if err != nil {
			return Found{Ignored: found.Ignored}, err
		}
		r := domain.Analyze(f, h.Name(path))
		f.Close()
		if r.Examples == 0 {
			continue
		}
		reports = append(reports, history.Record(snapshot.Key(root, path), r))
	}
	if failure := h.Store.Write(ctx, "scrap.json", recorded{Version: snapshot.Version, Files: history.Files()}); failure != nil {
		return Found{Ignored: found.Ignored}, failure
	}
	sort.SliceStable(reports, func(i, j int) bool { return reports[i].Pressure > reports[j].Pressure })
	found.Reports = reports
	return found, nil
}

// recorded is scrap.json: the format version and each file's last report,
// named from the project root.
type recorded struct {
	Version int                 `json:"version"`
	Files   []domain.FileReport `json:"files"`
}

// result is the object --json prints beside schema and ok: the reports, an
// empty list rather than none.
type result struct {
	Files []domain.FileReport `json:"files"`
}

// Run measures the test files the command line selects, through h and with
// the context cmd made, and prints each file's report, or the object, or
// reports the failure it met, and returns the exit code the UI gives it. A
// last snapshot it could not read, and a selection with no test file, are
// said on stderr, with --json too.
func (c CLI) Run(ctx context.Context, ui *cli.UI, h Handler) int {
	found, err := h.Handle(ctx, Query{Paths: c.Paths, Changed: c.Changed})
	if found.Ignored != nil {
		ui.Line("ignoring unreadable .metrics/scrap.json: " + found.Ignored.Error())
	}
	out := result{Files: []domain.FileReport{}}
	if err != nil {
		return ui.Emit(c.JSON, out, ui.Problems(err))
	}
	if found.NoTests {
		ui.Line("no test files to measure")
		return ui.Emit(c.JSON, out, nil)
	}
	out.Files = found.Reports
	if !c.JSON {
		for _, r := range found.Reports {
			printReport(ui.Stdout, r, c.Verbose)
		}
	}
	return ui.Emit(c.JSON, out, nil)
}

// printReport writes r's lines for people on w: its summary, a warning
// when the last change made it worse, its recommendations unless it is to
// be left alone, and with verbose every example's measurements.
func printReport(w io.Writer, r domain.FileReport, verbose bool) {
	change := ""
	if c := r.Compare; c != nil && c.Verdict != "unchanged" {
		change = fmt.Sprintf("  %s from %.1f", c.Verdict, c.PreviousPressure)
	}
	_, _ = fmt.Fprintf(w, "%s  %s  pressure=%.1f  examples=%d  avg=%.1f  max=%.1f%s\n",
		r.File, r.Action, r.Pressure, r.Examples, r.AverageScore, r.MaxScore, change)
	if c := r.Compare; c != nil && c.Verdict == "worse" {
		_, _ = fmt.Fprintln(w, "  the last change made this file worse: check new helpers and duplication before keeping it")
	}
	// A file to leave alone keeps its minor hints out of the way.
	if r.Action != domain.LeaveAlone || verbose {
		for _, rec := range r.Recommendations {
			_, _ = fmt.Fprintf(w, "  %-6s %s\n", rec.Confidence, rec.Text)
		}
	}
	if verbose {
		for _, e := range r.Details {
			_, _ = fmt.Fprintf(w, "    %5.1f  %s:%d-%d %q lines=%d assertions=%d decisions=%d mocks=%d %v\n",
				e.Score, r.File, e.StartLine, e.EndLine, e.Name, e.Lines, e.Assertions, e.Decisions, e.Mocks, e.Smells)
		}
	}
}
