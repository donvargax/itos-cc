package mutation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/donvargax/itos-cc/internal/cli"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// countedBounds says what --count bounds, in the report and in plain output.
const countedBounds = "--count bounds mutant trials, not discovery, coverage, listing, baseline commands or total latency"

// countedResult is the --json object of a counted run.
type countedResult struct {
	Sampling countedSampling    `json:"sampling"`
	Selected []countedSite      `json:"selected"`
	Subjects countedSubjects    `json:"subjects"`
	Stages   []preparationStage `json:"stages"`
	// NotBuilt are the committed Go sources only another system builds,
	// by their paths from the repository's top, none of whose sites is
	// eligible; left out when there are none.
	NotBuilt []notBuiltFile `json:"not_built,omitempty"`
	// cleanups are the owned process groups of trials whose cleanup did
	// not finish, which the run reports each as mutation.cleanup-failed.
	cleanups []mutants.WorkerCleanupFailed
}

// preparationStage is one observed stage of a counted run, in its report.
type preparationStage struct {
	Name  string `json:"name"`
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}

// countedSampling is what was drawn and what of it ran. Assurance is
// "sampled": a pass proves the judgments listed, never the population.
// Uncommitted are the inputs whose uncommitted changes the run left
// unjudged, judging them as Commit holds them.
type countedSampling struct {
	Budget      int      `json:"budget"`
	Eligible    int      `json:"eligible"`
	Selected    int      `json:"selected"`
	Executed    int      `json:"executed"`
	Omitted     int      `json:"omitted"`
	Seed        string   `json:"seed"`
	Algorithm   string   `json:"algorithm"`
	Commit      string   `json:"commit"`
	Since       string   `json:"since,omitempty"`
	SinceBase   string   `json:"since_base,omitempty"`
	Assurance   string   `json:"assurance"`
	Completion  string   `json:"completion"`
	Stop        string   `json:"stop,omitempty"`
	Bounds      string   `json:"bounds"`
	Uncommitted []string `json:"uncommitted"`
}

// countedSite is one selected site. State is "judged" (its trial ran to an
// outcome), "uncovered" (measured, no test reaches it, no trial), "blocked"
// (it survived its own tests, and the clean baseline of the listed tests it
// needs failed, so they could not judge it), "failed" (its trial could not
// run) or "unattempted"; only judged and uncovered sites have an outcome.
// Stages is each stage its trial ran, in order.
type countedSite struct {
	Identity    string         `json:"identity"`
	File        string         `json:"file"`
	Line        int            `json:"line"`
	Column      int            `json:"column"`
	Function    string         `json:"function"`
	Original    string         `json:"original"`
	Replacement string         `json:"replacement"`
	State       string         `json:"state"`
	Outcome     string         `json:"outcome,omitempty"`
	Limit       string         `json:"limit,omitempty"`
	Scope       string         `json:"scope,omitempty"`
	Tests       []string       `json:"tests,omitempty"`
	Excepted    string         `json:"excepted,omitempty"`
	Reason      string         `json:"reason,omitempty"`
	Stages      []countedStage `json:"stages,omitempty"`
	// selection is, for a blocked site, the listed selection that fails
	// without any mutant.
	selection *mutants.FailedSelection
}

// countedStage is one stage of a site's trial: "own", "listed-baseline" or
// "listed" (mutants.FreshStage).
type countedStage struct {
	Name    string   `json:"name"`
	State   string   `json:"state"`
	Outcome string   `json:"outcome,omitempty"`
	Tests   []string `json:"tests,omitempty"`
	Error   string   `json:"error,omitempty"`
}

type countedSubject struct {
	File     string `json:"file"`
	Function string `json:"function"`
}

// countedSubjects are the functions with eligible sites: judged when one of
// their sites was selected, omitted otherwise.
type countedSubjects struct {
	Judged  []countedSubject `json:"judged"`
	Omitted []countedSubject `json:"omitted"`
}

// countedProgress shows a counted run's progress lines on stderr and
// presents its report when the handler has it: printed in plain output,
// its problems kept for Emit.
type countedProgress struct {
	Progress
	p      *Presentation
	report *countedResult
}

// counted refuses what the flags alone refuse before anything is observed,
// assembles the counted query once, then presents the report, or the
// failure with no result object.
func (c Run) counted(ctx context.Context, ui *cli.UI, h Handler, written Written) int {
	if !written.given("count") {
		return ui.Emit(c.JSON, nil, []*cli.Problem{flagConflict("--seed", "--seed seeds the selection of --count, and is given without it")})
	}
	if c.Count < 1 {
		value := written.spelling("count", "")
		return ui.Emit(c.JSON, nil, []*cli.Problem{cli.NewProblem(cli.CodeUsage, "flags.value-invalid", fmt.Sprintf("--count needs a whole number of at least 1, and was given %q", value),
			"Give it 1 or more.").With("flag", "--count").With("value", value)})
	}
	if written.given("seed") && c.Seed == "" {
		return ui.Emit(c.JSON, nil, []*cli.Problem{cli.NewProblem(cli.CodeUsage, "flags.value-invalid", "--seed needs a non-empty TEXT", "Give it a seed, or leave it out to seed with HEAD.").
			With("flag", "--seed").With("value", "")})
	}
	for _, flag := range []struct{ name, why string }{
		{"changed", "--changed judges the working tree, and --count judges committed inputs only; use --since"},
		{"no-coverage", "--count measures coverage freshly, so strict coverage and listed reach are never bypassed"},
		{"use-existing-coverage", "--count measures coverage freshly from the committed inputs"},
		{"coverage-command", "--count measures coverage freshly with the built-in commands"},
		{"coverage-report", "--count measures coverage freshly from the committed inputs"},
		{"test-command", "--count runs each mutant's own tests, the per-language default"},
		{"mutate-all", "--count runs every selected site fresh already"},
		{"fail-fast", "--count judges every selected site and stays aggregate; --fail-fast stops complete runs"},
	} {
		if written.given(flag.name) {
			return ui.Emit(c.JSON, nil, []*cli.Problem{flagConflict("--"+flag.name, flag.why)})
		}
	}
	if h.Platform != "linux" && h.Platform != "darwin" {
		return ui.Emit(c.JSON, nil, []*cli.Problem{cli.NewProblem(cli.CodeEnvironment, "count.platform",
			fmt.Sprintf("mutation run --count supports Linux and macOS, where every command's process tree is owned; this is %s", h.Platform),
			"Run it on Linux or macOS, or run mutation run without --count; native Windows support is #29.").With("platform", h.Platform)})
	}
	progress := &countedProgress{Progress: Progress{Log: ui.Stderr, Rel: ui.Rel}, p: &Presentation{UI: ui, JSON: c.JSON, Tree: h.Tree}}
	err := h.Counted(ctx, CountedQuery{Paths: c.Paths, Since: c.Since, Seed: c.Seed, Count: c.Count, AllTests: c.AllTests,
		FailUncovered: c.FailUncovered != nil, LinesOnly: c.FailUncovered != nil && *c.FailUncovered == "lines",
		Workers: c.Workers, TimeoutFactor: c.TimeoutFactor, OutputFactor: c.OutputFactor, MemoryFactor: c.MemoryFactor, Progress: progress})
	if err != nil {
		return ui.Emit(c.JSON, nil, ui.Problems(countedError(err, ui.Rel)))
	}
	return ui.Emit(c.JSON, progress.report, progress.p.Problems)
}

// Unjudged warns that the run judges commit, so the uncommitted changes
// to paths, its inputs, go unjudged: a line for the run, then one for each
// path.
func (r *countedProgress) Unjudged(commit string, paths []string) {
	if r.Log == nil {
		return
	}
	_, _ = fmt.Fprintf(r.Log, "itos-cc: warning: this run judges commit %s, so the uncommitted changes to %d of its inputs go unjudged; commit them to judge them\n", commit, len(paths))
	for _, path := range paths {
		_, _ = fmt.Fprintf(r.Log, "itos-cc: unjudged uncommitted change: %s\n", path)
	}
}

// NotBuilt says on stderr, once for each, that a committed Go source is
// not built on this platform, so not judged here.
func (r *countedProgress) NotBuilt(files []mutants.NotBuilt) {
	if r.Log == nil {
		return
	}
	for _, f := range files {
		_, _ = fmt.Fprintf(r.Log, "itos-cc: not built on %s, so %s is not judged here\n", f.Platform, f.Path)
	}
}

// Counted prints the report, then reports what stopped the run, or each
// selected site that fails it, the interruption, the stale exceptions and
// strict Go coverage's findings, in the cmd handler's order.
func (r *countedProgress) Counted(ctx context.Context, found CountedFound) {
	p := r.p
	r.report = found.Report
	if found.Plan != nil {
		p.Root = found.Plan.Root
	}
	p.printCounted(found.Report)
	switch {
	case found.Judged:
		// With --fail-uncovered=lines, an uncovered site fails, and strict Go
		// coverage is judged, on the lines the range changed alone.
		plan := found.Plan
		p.reportCounted(found.Report, func(site countedSite) bool {
			return found.Strict && (!found.LinesOnly || plan.ChangedLines(site.File, site.Line, site.Line))
		}, found.Interrupted != nil)
		if found.Interrupted != nil {
			p.report(countedInterrupted(found.Interrupted))
		}
		for _, c := range found.Report.cleanups {
			p.report(CleanupProblem(c))
		}
		for _, e := range found.Stale {
			p.StaleExceptions(ctx, e.File, []mutants.StaleException{e})
		}
		if found.StrictGo {
			p.countedStrictGo(plan, found.Prep, found.LinesOnly)
		}
	case found.Interrupted != nil:
		p.report(countedInterrupted(found.Interrupted))
	case found.PreparationFailure != nil:
		failed := found.PreparationStage
		k := cli.CodeNo
		if failed == "tools" {
			k = cli.CodeEnvironment
		}
		p.report(cli.NewProblem(k, "count.preparation-failed", fmt.Sprintf("counted preparation failed at %s, so no mutant was judged: %v", failed, freshPreparationMessage(found.PreparationFailure)),
			"Make the committed tests and measurement succeed, then run again.").With("stage", failed))
	case len(found.Integration) > 0:
		p.Problems = append(p.Problems, GapProblems(p.UI, found.Integration)...)
	}
}

// countedInterrupted is the problem of a counted run that cause stopped:
// an interrupt's, the interrupted problem every command reports (decision
// 38), which count.interrupted was; any other cause, which nothing in a
// counted run gives, the internal failure it would be.
func countedInterrupted(cause error) *cli.Problem {
	if errors.Is(cause, cli.ErrInterrupted) {
		return cli.Interrupted()
	}
	return cli.Internal(cause)
}

// printCounted writes the plain report: the draw, each selected site, and
// what a sampled pass does and does not prove.
func (p *Presentation) printCounted(r *countedResult) {
	if p.JSON {
		return
	}
	out := p.UI.Stdout
	s := r.Sampling
	_, _ = fmt.Fprintf(out, "counted mutation run at %s: seed %s, algorithm %s\n", s.Commit, s.Seed, s.Algorithm)
	if s.Completion == "not-applicable" {
		_, _ = fmt.Fprintf(out, "not applicable: the committed selection has no eligible mutation site (%d eligible, %d selected)\n", s.Eligible, s.Selected)
	} else {
		_, _ = fmt.Fprintf(out, "budget %d: %d of %d eligible sites selected, %d executed, %d omitted\n", s.Budget, s.Selected, s.Eligible, s.Executed, s.Omitted)
	}
	for _, site := range r.Selected {
		what := site.State
		if site.Outcome != "" {
			what = site.Outcome
		}
		line := fmt.Sprintf("  %s %s → %s in %s, %s", what, Quote(site.Original), Quote(site.Replacement), site.Function, SiteAt(site.File, site.Line, site.Column))
		if site.Scope == mutants.ScopeListed {
			line += " by listed tests " + strings.Join(site.Tests, " ")
		}
		if site.Excepted != "" {
			line += " (excepted: " + site.Excepted + ")"
		}
		if site.Reason != "" {
			line += ": " + site.Reason
		}
		_, _ = fmt.Fprintln(out, line)
	}
	if s.Stop != "" {
		_, _ = fmt.Fprintf(out, "stopped: %s\n", s.Stop)
	}
	_, _ = fmt.Fprintln(out, "sampled assurance: only the judgments above are proven; this is not a complete mutation result, and no snapshot, annotation or coverage cache was written")
	_, _ = fmt.Fprintln(out, countedBounds)
}

// reportCounted reports each selected site that fails the run: a survivor
// no valid exception excepts, an uncovered site failUncovered says fails
// (with --fail-uncovered, every one, or with =lines one on a changed line),
// the listed selection a blocked site needs, which fails without any
// mutant, once, and a site whose trial could not run.
func (p *Presentation) reportCounted(r *countedResult, failUncovered func(countedSite) bool, interrupted bool) {
	selections := map[string]bool{}
	for _, site := range r.Selected {
		m := mutants.Mutant{Line: site.Line, Column: site.Column, Original: site.Original, Replacement: site.Replacement}
		switch {
		case site.State == "judged" && site.Outcome == mutants.Survived && site.Excepted == "":
			// Its identity is in the message, which a CI log prints, and its
			// except in the fix: no mutation run is needed to except it.
			p.report(countedMutantProblem(site, m, "mutation.survived", "survived, as "+site.Identity,
				fmt.Sprintf("Add a test that fails with this change. If no test can, as the change changes no behaviour, except it with itos-cc mutation except '%s' --reason '…', which runs nothing.", site.Identity)))
		case site.State == "uncovered" && failUncovered(site):
			p.report(countedMutantProblem(site, m, "mutation.uncovered", "is uncovered: no test executes its line",
				"Add a test that executes this line and fails with this change."))
		case site.State == "blocked" && site.selection != nil:
			p.FailedSelections([]mutants.FailedSelection{*site.selection}, selections, "mutation run --count")
		case site.State == "cancelled" || (site.State == "unattempted" && interrupted):
			// The interrupt that stopped it, or its owned cleanup that
			// failed, is reported as itself.
		case site.State == "failed" || site.State == "unattempted" || site.State == "blocked":
			reason := site.Reason
			if reason == "" {
				reason = "it was not attempted"
			}
			p.report(countedMutantProblem(site, m, "count.trial-failed", "was not judged: "+reason,
				"Fix what stopped its trial, then run again."))
		}
	}
}

func countedMutantProblem(site countedSite, m mutants.Mutant, rule, verdict, fix string) *cli.Problem {
	return cli.NewProblem(cli.CodeNo, rule,
		fmt.Sprintf("%s:%d:%d: %s → %s in %s %s", site.File, m.Line, m.Column, Quote(m.Original), Quote(m.Replacement), site.Function, verdict), fix).
		With("file", site.File).With("line", m.Line).With("column", m.Column).With("function", site.Function).
		With("original", m.Original).With("replacement", m.Replacement).With("identity", site.Identity)
}

// countedStrictGo reports, for every admitted Go function, sites or none,
// what complete strict mode reports: a function without complete evidence
// and each uncovered executable block; with linesOnly
// (--fail-uncovered=lines), only for a function the range added or changed
// a line of, and a block overlapping such a line.
func (p *Presentation) countedStrictGo(plan *mutants.FreshPlan, prep *mutants.FreshPreparation, linesOnly bool) {
	for _, unit := range plan.Units {
		if !strings.HasSuffix(unit.Path, ".go") || linesOnly && !plan.ChangedLines(unit.Path, unit.StartLine, unit.EndLine) {
			continue
		}
		evidence := prep.GoCoverage[unit.Identity]
		if evidence == nil || !evidence.Complete {
			p.CoverageProblem("go", unit.Path, unit.Function, unit.StartLine, "mutation.coverage-missing", "has no complete measured Go coverage inventory")
			continue
		}
		for _, block := range evidence.Blocks {
			start, end := block.Lines()
			if !block.Covered && (!linesOnly || plan.ChangedLines(unit.Path, start, end)) {
				p.CoverageProblem("go", unit.Path, unit.Function, block.Line, "mutation.uncovered-statement", "has an uncovered executable Go coverage block")
			}
		}
	}
}
