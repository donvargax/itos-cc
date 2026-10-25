package mutation

import (
	"context"
	"fmt"
	"strings"

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/coverage"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

type runFile struct {
	File     string `json:"file"`
	Killed   int    `json:"killed"`
	Survived int    `json:"survived"`
	// Excepted is the survivors itos-cc.yaml excepts, not in Survived.
	Excepted  int    `json:"excepted"`
	Uncovered int    `json:"uncovered"`
	Ran       int    `json:"ran"`
	Reused    int    `json:"reused"`
	Baseline  string `json:"baseline"`
	// Judged is there only with --since, empty when no function changed.
	Judged []string `json:"judged,omitzero"`
	// Mutants is every mutant of the functions judged, in site order;
	// empty, never null, when the baseline or a selection of listed tests
	// failed, except with --fail-fast, which lists them with their state.
	Mutants []runMutant `json:"mutants"`
	// State and Work are there with --fail-fast only: whether the run
	// completed the file, stopped inside it, never started its work, or
	// found it blocked by a failing baseline or selection, and how many of
	// its mutants ended in each state.
	State string   `json:"state,omitempty"`
	Work  *runWork `json:"work,omitempty"`
	// Cache is there with --fail-fast only: "complete" when the file's
	// snapshot, as the run leaves it, holds a valid result for every
	// mutant of its functions judged, else "incomplete". A stopped run's
	// own work, State, can be incomplete while the cache is complete.
	Cache string `json:"cache,omitempty"`
}

// runWork counts selected mutants by their fail-fast state, disjointly:
// completed (run, reused or measured uncovered, with an outcome),
// cancelled, unattempted and blocked (with none).
type runWork struct {
	Completed   int `json:"completed"`
	Cancelled   int `json:"cancelled"`
	Unattempted int `json:"unattempted"`
	Blocked     int `json:"blocked"`
}

func (w *runWork) add(o *runWork) {
	w.Completed += o.Completed
	w.Cancelled += o.Cancelled
	w.Unattempted += o.Unattempted
	w.Blocked += o.Blocked
}

// runStop is, with --fail-fast, whether the run stopped early, and at
// which failure: the rule and subject of its problem.
type runStop struct {
	Stopped bool           `json:"stopped"`
	Rule    string         `json:"rule,omitempty"`
	Subject map[string]any `json:"subject,omitempty"`
}

// runMutant is a site, less its file, and how it was decided: a timed-out
// mutant is "timeout" here and counts in "killed".
type runMutant struct {
	Line        int    `json:"line"`
	Column      int    `json:"column"`
	Function    string `json:"function"`
	Original    string `json:"original"`
	Replacement string `json:"replacement"`
	// Outcome and Scope are there for every mutant but one a --fail-fast
	// stop left undecided.
	Outcome string `json:"outcome,omitempty"`
	// Limit names the cap an "over-limit" outcome's run went over,
	// "output" or "memory"; left out for every other outcome.
	Limit  string `json:"limit,omitempty"`
	Reused bool   `json:"reused"`
	// Scope is the scope of the tests that decided the outcome: "own",
	// "all-tests", or the --test-command line.
	Scope string `json:"scope,omitempty"`
	// Excepted is the reason itos-cc.yaml gives, on an excepted survivor
	// only; its outcome stays survived.
	Excepted string `json:"excepted,omitempty"`
	// Coverage names the coverage that executed the mutant's line,
	// "in-process", "integration", or both; there only when this run's
	// coverage executed it.
	Coverage []string `json:"coverage,omitempty"`
	// Tests is, with scope "listed", the IDs of the listed tests that
	// decided the outcome.
	Tests []string `json:"tests,omitempty"`
	// State is there with --fail-fast only: "completed", "cancelled",
	// "unattempted" or "blocked".
	State string `json:"state,omitempty"`
}

type runResult struct {
	Files []runFile `json:"files"`
	// Stop and Work are there with --fail-fast only.
	Stop *runStop `json:"stop,omitempty"`
	Work *runWork `json:"work,omitempty"`
	// Orphans are the snapshots under the run's paths whose source file
	// was gone, which it removed; left out when there were none, so a run
	// with no orphan prints the keys it always did.
	Orphans []Orphan `json:"orphans,omitempty"`
	// NotBuilt are the Go files selected that only another system builds,
	// left out of Files and judged nowhere here; left out when there are
	// none.
	NotBuilt []notBuiltFile `json:"not_built,omitempty"`
}

// given says whether the command line gave flag name.
func (w Written) given(name string) bool { return len(w[name]) > 0 }

// Run refuses what the flags alone refuse before anything is observed,
// hands a counted run to its own handler, assembles the query once, then
// presents each file's result, the stale exceptions, the orphans removed
// and the fail-fast stop, or the result object as far as it got beside the
// failure.
func (c Run) Run(ctx context.Context, ui *cli.UI, h Handler, given cli.Written) int {
	written := Written(given)
	linesOnly := c.FailUncovered != nil && *c.FailUncovered == "lines"
	if linesOnly && !written.given("since") {
		return ui.Emit(c.JSON, nil, ui.Problems(failureError(LinesWithoutSince{}, ui.Rel)))
	}
	if written.given("count") || written.given("seed") {
		return c.counted(ctx, ui, h, written)
	}
	result := runResult{Files: []runFile{}}
	if c.FailUncovered != nil && c.NoCoverage {
		return ui.Emit(c.JSON, result, []*cli.Problem{flagConflict("--no-coverage", "--no-coverage conflicts with --fail-uncovered, which requires measured Go executable coverage")})
	}
	if c.FailFast {
		if h.Platform != "linux" && h.Platform != "darwin" {
			return ui.Emit(c.JSON, result, []*cli.Problem{cli.NewProblem(cli.CodeEnvironment, "fail-fast.platform",
				fmt.Sprintf("mutation run --fail-fast supports Linux and macOS, where every command's process tree is owned and a stop can end it; this is %s", h.Platform),
				"Run it on Linux or macOS, or run mutation run without --fail-fast; native Windows support is #29.").With("platform", h.Platform)})
		}
		result.Stop, result.Work = &runStop{}, &runWork{}
	}
	found, err := h.Run(ctx, RunQuery{
		SelectionQuery:      SelectionQuery{Paths: c.Paths, Changed: c.Changed, Since: written.value("since", c.Since)},
		FailUncovered:       c.FailUncovered != nil,
		LinesOnly:           linesOnly,
		FailFast:            c.FailFast,
		NoCoverage:          c.NoCoverage,
		UseExistingCoverage: c.UseExistingCoverage,
		AllTests:            c.AllTests,
		CoverageCommand:     written.value("coverage-command", c.CoverageCommand),
		CoverageReports:     c.CoverageReport,
		TestCommand:         written.value("test-command", c.TestCommand),
		Workers:             c.Workers,
		TimeoutFactor:       c.TimeoutFactor,
		OutputFactor:        c.OutputFactor,
		MemoryFactor:        c.MemoryFactor,
		MutateAll:           c.MutateAll,
		Annotate:            !c.NoAnnotate,
		Progress:            selectionProgress{Progress: Progress{Log: ui.Stderr, Rel: ui.Rel}, ui: ui},
	})
	p := &Presentation{UI: ui, JSON: c.JSON, Root: found.Root, Tree: h.Tree}
	result.NotBuilt = p.sayNotBuilt(found.NotBuilt)
	switch {
	case found.Gaps != nil:
		p.gaps(*found.Gaps)
		return ui.Emit(c.JSON, result, p.Problems)
	case found.NoSources:
		ui.Line("no source files to mutate")
		p.Elsewhere(ctx, found.Elsewhere)
	case found.Ran:
		// An interrupted run, or one whose owned cleanup failed, reports its
		// files as a fail-fast run does, each with its state and work.
		partial := c.FailFast || found.Interrupted != nil || cleanupFailures(found.Results)
		if partial && result.Work == nil {
			result.Work = &runWork{}
		}
		c.present(ctx, p, found, &result, partial)
		p.Elsewhere(ctx, found.Elsewhere)
		p.cleanupFailed(found.Results)
		if found.Interrupted != nil {
			if stop := found.Stop; stop != nil {
				result.Stop = &runStop{Stopped: true, Rule: stop.Rule, Subject: p.stopSubject(stop)}
			}
			p.Problems = append(p.Problems, ui.Problems(found.Interrupted)...)
			return ui.Emit(c.JSON, result, p.Problems)
		}
	}
	if err != nil {
		return ui.Emit(c.JSON, result, append(p.Problems, ui.Problems(runError(err, ui.Rel))...))
	}
	for _, orphan := range found.Orphans {
		result.Orphans = append(result.Orphans, Orphan{File: ui.Rel(orphan.Path), Snapshot: orphan.Snapshot, Why: orphan.Why})
	}
	p.Orphans(result.Orphans, "was an orphan, and is removed")
	p.stopped(found.Stop, &result)
	return ui.Emit(c.JSON, result, p.Problems)
}

// stopped records stop, what stopped a fail-fast run at its first
// actionable failure, in result, and says so on stdout, naming its file
// slash-separated; a run nothing stopped, stop nil, records nothing.
func (p *Presentation) stopped(stop *mutants.Stop, result *runResult) {
	if stop == nil {
		return
	}
	result.Stop = &runStop{Stopped: true, Rule: stop.Rule, Subject: p.stopSubject(stop)}
	if !p.JSON {
		w := result.Work
		_, _ = fmt.Fprintf(p.UI.Stdout, "stopped early at the first actionable failure, %s %s: %d completed, %d cancelled, %d unattempted, %d blocked; a file it cut short keeps its source as it was, and its snapshot only what was judged\n",
			stop.Rule, p.describeStop(stop), w.Completed, w.Cancelled, w.Unattempted, w.Blocked)
	}
}

// value is the value of flag name the command line gave, nil when it gave
// none, so an empty one given differs from none.
func (w Written) value(name, value string) *string {
	if !w.given(name) {
		return nil
	}
	return &value
}

// gaps reports why strict coverage stopped the run before any mutant.
func (p *Presentation) gaps(g CoverageGaps) {
	for _, check := range g.Unsupported {
		p.CoverageProblem(check.Language, check.File, check.Function, check.Line,
			"mutation.coverage-unsupported", "uses unsupported "+LanguageName(check.Language)+" coverage scope "+strings.Join(check.Changed, ", "))
	}
	p.Problems = append(p.Problems, GapProblems(p.UI, g.Go)...)
	if g.GoNoReport {
		p.report(cli.NewProblem(cli.CodeEnvironment, "coverage.no-report", "Go coverage measurement did not produce executable coverage",
			"Run the built-in Go coverage command successfully, then retry.").With("language", "go"))
	}
	p.Problems = append(p.Problems, GapProblems(p.UI, g.Unmeasured)...)
}

// GapProblems is a problem for each of ms, a report's build roots or
// reports coverage did not measure, in its order, through the coverage
// domain's classifier the UI holds, each root's directory named from the
// working directory, as every other path itos-cc prints is.
func GapProblems(ui *cli.UI, ms []coverage.Unmeasured) []*cli.Problem {
	var out []*cli.Problem
	for _, m := range ms {
		if m.Dir != "" {
			m.Dir = ui.Rel(m.Dir)
		}
		out = append(out, ui.Problems(m)...)
	}
	return out
}

// present prints and reports each file's result a run judged, strict
// coverage's findings first.
func (c Run) present(ctx context.Context, p *Presentation, found RunFound, result *runResult, partial bool) {
	ui, failUncovered, scope := p.UI, c.FailUncovered != nil, found.Lines
	if found.Strict {
		p.strictCoverage(found.Results, scope)
	}
	reported := map[string]bool{}
	for _, r := range found.Results {
		// Every file has the baseline this run actually ran for its
		// command, or "not-run". With --fail-fast every file has its state
		// and its work too, and a file not written still lists its
		// mutants, each with its state.
		var work *runWork
		state, cache := "", ""
		if partial {
			work = workOf(r.Mutants)
			result.Work.add(work)
			state, cache = failFastState(r, work), "incomplete"
			if r.Cached {
				cache = "complete"
			}
		}
		if partial && (r.BaselineFailed || len(r.FailedSelections) > 0) {
			// It counts what it ran and reused before the stop, as its
			// completed work does.
			f := runFile{File: r.Rel, Ran: r.Ran, Reused: r.Reused, Baseline: r.Baseline, Judged: r.Judged, Mutants: jsonMutants(r.Mutants),
				State: state, Work: work, Cache: cache}
			countMutants(&f, r.Mutants)
			result.Files = append(result.Files, f)
			if !c.JSON {
				_, _ = fmt.Fprintf(ui.Stdout, "blocked, %s: the tests of %s, or the listed tests its mutants run, fail without any mutant (ran %d, reused %d; %d completed, %d cancelled, %d unattempted, %d blocked); cache %s\n",
					snapshotKept(r), p.plain(r.Rel), f.Ran, f.Reused, work.Completed, work.Cancelled, work.Unattempted, work.Blocked, cache)
				if r.BaselineFailed {
					_, _ = fmt.Fprintln(ui.Stdout, HeadAndTail(r.BaselineOutput, reportedLines))
				}
			}
			if r.BaselineFailed {
				p.report(p.baselineFailed(r.Rel, r.BaselineCopy, r.BaselineLeftOut, "judged", "mutation run").With("output", HeadAndTail(r.BaselineOutput, reportedLines)))
			}
			p.FailedSelections(r.FailedSelections, reported, "mutation run")
			for _, m := range r.Mutants {
				p.Failed(r.Rel, m.Function, []mutants.Mutant{{Line: m.Line, Column: m.Column, Original: m.Original,
					Replacement: m.Replacement, Outcome: m.Outcome, Excepted: m.Excepted}}, uncoveredFails(failUncovered, scope, r.Rel))
			}
			p.StaleExceptions(ctx, r.Rel, r.StaleExceptions)
			continue
		}
		if len(r.FailedSelections) > 0 && !r.BaselineFailed {
			// A mutant the file holds needed listed tests that fail
			// without it: it is not decided, so neither is the file.
			result.Files = append(result.Files, runFile{File: r.Rel, Baseline: "failed", Judged: r.Judged, Mutants: []runMutant{}})
			if !c.JSON {
				_, _ = fmt.Fprintf(ui.Stdout, "listed tests the mutants of %s run fail without any mutant; snapshot not updated\n", p.plain(r.Rel))
			}
			p.FailedSelections(r.FailedSelections, reported, "mutation run")
			p.StaleExceptions(ctx, r.Rel, r.StaleExceptions)
			continue
		}
		if r.BaselineFailed {
			result.Files = append(result.Files, runFile{File: r.Rel, Baseline: "failed", Judged: r.Judged, Mutants: []runMutant{}})
			if !c.JSON {
				_, _ = fmt.Fprintf(ui.Stdout, "baseline tests of %s fail; snapshot not updated\n%s\n", p.plain(r.Rel), HeadAndTail(r.BaselineOutput, reportedLines))
			}
			p.report(p.baselineFailed(r.Rel, r.BaselineCopy, r.BaselineLeftOut, "judged", "mutation run").With("output", HeadAndTail(r.BaselineOutput, reportedLines)))
			p.StaleExceptions(ctx, r.Rel, r.StaleExceptions)
			continue
		}
		f := runFile{File: r.Rel, Ran: r.Ran, Reused: r.Reused, Baseline: r.Baseline, Judged: r.Judged, Mutants: jsonMutants(r.Mutants),
			State: state, Work: work, Cache: cache}
		// Only the functions judged count: the others keep outcomes no
		// change in the range is to blame for.
		judged := map[string]bool{}
		for _, id := range r.Judged {
			judged[id] = true
		}
		var units []mutants.UnitResult
		for _, u := range r.Snapshot.Units {
			if r.Judged == nil || judged[u.Namespace+"#"+u.Name] {
				units = append(units, u)
			}
		}
		if r.Incomplete {
			// Only this run's judgments count and fail: the snapshot also
			// keeps results of the snapshot before for sites the stop
			// left undecided.
			units = nil
			countMutants(&f, r.Mutants)
		}
		for _, u := range units {
			excepted := ExceptedIn(u.Mutants)
			f.Killed += u.Killed
			f.Survived += u.Survived - excepted
			f.Excepted += excepted
			f.Uncovered += u.Uncovered
		}
		result.Files = append(result.Files, f)
		if !c.JSON {
			// "excepted" only where there is one, so a project with no
			// exceptions reads as it always did.
			counts := fmt.Sprintf("%d killed, %d survived, %d uncovered", f.Killed, f.Survived, f.Uncovered)
			if f.Excepted > 0 {
				counts = fmt.Sprintf("%d killed, %d survived, %d excepted, %d uncovered", f.Killed, f.Survived, f.Excepted, f.Uncovered)
			}
			line := fmt.Sprintf("%s in %s (ran %d, reused %d)", counts, p.plain(r.Rel), f.Ran, f.Reused)
			if r.Judged != nil {
				line += fmt.Sprintf(" (judged %d of %d functions)", len(r.Judged), r.Functions)
			}
			if r.Incomplete {
				line = fmt.Sprintf("stopped early, %s: %s in %s (ran %d, reused %d); %d cancelled, %d unattempted; cache %s",
					snapshotKept(r), counts, p.plain(r.Rel), f.Ran, f.Reused, work.Cancelled, work.Unattempted, cache)
			}
			_, _ = fmt.Fprintln(ui.Stdout, line)
		}
		for _, u := range units {
			p.Failed(r.Rel, u.Namespace+"#"+u.Name, u.Mutants, uncoveredFails(failUncovered, scope, r.Rel))
		}
		if r.Incomplete {
			for _, m := range r.Mutants {
				p.Failed(r.Rel, m.Function, []mutants.Mutant{{Line: m.Line, Column: m.Column, Original: m.Original,
					Replacement: m.Replacement, Outcome: m.Outcome, Excepted: m.Excepted}}, uncoveredFails(failUncovered, scope, r.Rel))
			}
		}
		p.StaleExceptions(ctx, r.Rel, r.StaleExceptions)
	}
}

// cleanupFailures says whether a judgment's owned cleanup failed in any of
// results.
func cleanupFailures(results []mutants.FileResult) bool {
	for _, r := range results {
		if len(r.CleanupFailures) > 0 {
			return true
		}
	}
	return false
}

// cleanupFailed reports mutation.cleanup-failed for each owned process
// group whose cleanup did not finish (ADR-0021, q-37): the internal exit,
// beside the run's other problems, one for each group.
func (p *Presentation) cleanupFailed(results []mutants.FileResult) {
	for _, r := range results {
		for _, c := range r.CleanupFailures {
			p.report(CleanupProblem(c))
		}
	}
}

// CleanupProblem is mutation.cleanup-failed for the owned process group
// whose cleanup c says failed, naming the group and its command.
func CleanupProblem(c mutants.WorkerCleanupFailed) *cli.Problem {
	return cli.NewProblem(cli.CodeInternal, "mutation.cleanup-failed",
		fmt.Sprintf("the owned process group %d of %q did not end by its cleanup deadline: %v", c.ProcessGroup, c.Command, c.Cause),
		"Stop what it left running; a command that detaches from its process group or session is not supported.").
		With("process_group", c.ProcessGroup).With("command", c.Command)
}

// strictCoverage reports, for every judged function of a file of a strict
// language a run decided, a function without complete evidence and each
// uncovered executable block or line; with scope, of those only a function
// with a line scope judges, and a block or line it judges.
func (p *Presentation) strictCoverage(results []mutants.FileResult, scope mutants.LineScope) {
	for _, result := range results {
		language := result.Snapshot.Language
		if !mutants.StrictCoverage(language) {
			continue
		}
		if result.BaselineFailed || len(result.FailedSelections) > 0 {
			continue
		}
		judged := map[string]bool{}
		for _, id := range result.Judged {
			judged[id] = true
		}
		for _, unit := range result.Snapshot.Units {
			id := unit.Namespace + "#" + unit.Name
			if result.Judged != nil && !judged[id] || !scope.Judges(result.Rel, unit.StartLine, unit.EndLine) {
				continue
			}
			if unit.Coverage == nil || !unit.Coverage.Complete {
				p.CoverageProblem(language, result.Rel, id, unit.StartLine, "mutation.coverage-missing", "has no complete measured "+CoverageInventory(language))
				continue
			}
			for _, block := range unit.Coverage.Blocks {
				if !block.Covered && scope.JudgesBlock(result.Rel, block) {
					p.CoverageProblem(language, result.Rel, id, block.Line, "mutation.uncovered-statement", "has an uncovered "+ExecutableUnit(language))
				}
			}
		}
	}
}

// snapshotKept says, in plain output, what became of the snapshot of a file
// a fail-fast stop left incomplete.
func snapshotKept(r mutants.FileResult) string {
	if r.Preserved {
		return "snapshot keeps what was judged, source not annotated"
	}
	return "snapshot not updated"
}

// jsonMutants is the --json form of mutants.
func jsonMutants(items []mutants.MutantResult) []runMutant {
	out := []runMutant{}
	for _, m := range items {
		out = append(out, runMutant{Line: m.Line, Column: m.Column, Function: m.Function,
			Original: m.Original, Replacement: m.Replacement, Outcome: m.Outcome, Limit: m.Limit, Reused: m.Reused, Scope: m.Scope, Excepted: m.Excepted,
			Coverage: m.Coverage, Tests: m.Tests, State: m.State})
	}
	return out
}

// countMutants counts the outcomes of items in f, as a file's units count
// them: a mutant timed out or over a limit is killed, and an excepted
// survivor excepted.
func countMutants(f *runFile, items []mutants.MutantResult) {
	for _, m := range items {
		switch {
		case mutants.Noticed(m.Outcome):
			f.Killed++
		case m.Outcome == mutants.Survived && m.Excepted != "":
			f.Excepted++
		case m.Outcome == mutants.Survived:
			f.Survived++
		case m.Outcome == mutants.Uncovered:
			f.Uncovered++
		}
	}
}

// workOf counts items by their fail-fast state.
func workOf(items []mutants.MutantResult) *runWork {
	w := &runWork{}
	for _, m := range items {
		switch m.State {
		case mutants.StateCompleted:
			w.Completed++
		case mutants.StateCancelled:
			w.Cancelled++
		case mutants.StateUnattempted:
			w.Unattempted++
		case mutants.StateBlocked:
			w.Blocked++
		}
	}
	return w
}

// failFastState is a file's state in a --fail-fast run: "blocked" when its
// baseline or a selection of listed tests it needed failed, "completed"
// when every selected mutant has its outcome, else "stopped" when the stop
// came once its work began, and "unattempted" when it came before.
func failFastState(r mutants.FileResult, w *runWork) string {
	switch {
	case r.BaselineFailed || len(r.FailedSelections) > 0:
		return "blocked"
	case w.Cancelled == 0 && w.Unattempted == 0:
		return "completed"
	case w.Cancelled > 0 || r.Ran > 0:
		return "stopped"
	}
	return "unattempted"
}

// stopSubject is the subject of the problem the stop's failure reports.
func (p *Presentation) stopSubject(s *mutants.Stop) map[string]any {
	switch {
	case s.Site != nil:
		return map[string]any{"file": s.File, "line": s.Site.Line, "column": s.Site.Column, "function": s.Function,
			"original": s.Site.Original, "replacement": s.Site.Replacement}
	case s.Exception != nil:
		return p.StaleSubject(s.File, *s.Exception)
	case s.Selection != nil:
		return map[string]any{"ids": s.Selection.IDs, "command": s.Selection.Command, "exit_code": s.Selection.ExitCode}
	case s.Function != "":
		return map[string]any{"file": s.File, "function": s.Function, "line": s.Line}
	}
	return map[string]any{"file": s.File}
}

// describeStop names the stop's failure in plain output, its file
// slash-separated.
func (p *Presentation) describeStop(s *mutants.Stop) string {
	file := p.plain(s.File)
	switch {
	case s.Site != nil:
		return fmt.Sprintf("%s → %s in %s, %s", Quote(s.Site.Original), Quote(s.Site.Replacement), s.Function, SiteAt(file, s.Site.Line, s.Site.Column))
	case s.Exception != nil && s.Exception.Line != 0:
		return fmt.Sprintf("in %s, %s (%s)", s.Exception.Function, SiteAt(file, s.Exception.Line, s.Exception.Column), s.Exception.Why)
	case s.Exception != nil:
		return fmt.Sprintf("%s in %s (%s)", file, s.Exception.Function, s.Exception.Why)
	case s.Selection != nil:
		return "of the listed tests " + strings.Join(s.Selection.IDs, " ")
	case s.Function != "":
		return fmt.Sprintf("%s:%d in %s", file, s.Line, s.Function)
	}
	return "in " + file
}
