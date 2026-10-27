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

// namedBounds says what --site bounds, in the report and in plain output.
const namedBounds = "--site bounds mutant trials to the named mutants, not discovery, coverage, listing, baseline commands or total latency"

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
	// Named is, for a --site run, each argument in the order given with
	// the identities of the mutants it named; left out otherwise.
	Named []countedNamed `json:"named,omitempty"`
	// OwnTests is whether a --site run judged the named mutants by their
	// own tests alone (--own-tests): the tests mutation.tests lists did not
	// run, so a mutant they would kill can show as survived; left out
	// otherwise.
	OwnTests bool `json:"own_tests,omitempty"`
	// cleanups are the owned process groups of trials whose cleanup did
	// not finish, which the run reports each as mutation.cleanup-failed.
	cleanups []mutants.WorkerCleanupFailed
}

// countedNamed is an argument of a --site run and the identities it named.
type countedNamed struct {
	Argument   string   `json:"argument"`
	Identities []string `json:"identities"`
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
// unjudged, judging them as Commit holds them. A --site run draws nothing:
// its Seed is left out and its Algorithm is "named".
type countedSampling struct {
	Budget      int      `json:"budget"`
	Eligible    int      `json:"eligible"`
	Selected    int      `json:"selected"`
	Executed    int      `json:"executed"`
	Omitted     int      `json:"omitted"`
	Seed        string   `json:"seed,omitempty"`
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
	// Outcomes is, for a site judged more than once (--repeat), every
	// outcome its runs gave, in run order, Outcome their first, and Flaky
	// whether they are not all alike; left out otherwise.
	Outcomes []string `json:"outcomes,omitempty"`
	Flaky    bool     `json:"flaky,omitempty"`
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
// failure with no result object. A --site run is a counted run whose
// sites are named, not drawn: it refuses what --count refuses, and --count,
// --seed, --since and paths too.
func (c Run) counted(ctx context.Context, ui *cli.UI, h Handler, written Written) int {
	named := written.given("site")
	mode := "--count"
	counts := map[string]int{"count": c.Count, "repeat": c.Repeat}
	refused := []struct{ name, why string }{
		{"seed", "--seed seeds the selection of --count, and is given without it"},
		{"repeat", "--repeat repeats the trials of --count, and is given without it"},
	}
	if named {
		mode = "--site"
		delete(counts, "count")
		refused = []struct{ name, why string }{
			{"count", "--site names the mutants it judges, and --count draws them: give one or the other"},
			{"seed", "--seed seeds the draw of --count, and --site draws nothing"},
			{"since", "--site names the mutants it judges, and --since chooses functions: name the sites alone"},
		}
	}
	for _, flag := range refused {
		if (named || !written.given("count")) && written.given(flag.name) {
			return ui.Emit(c.JSON, nil, []*cli.Problem{flagConflict("--"+flag.name, flag.why)})
		}
	}
	if c.OwnTests && c.AllTests {
		return ui.Emit(c.JSON, nil, []*cli.Problem{flagConflict("--own-tests",
			"--own-tests judges the named mutants by their own tests alone, and --all-tests by the whole suite: give one or the other")})
	}
	if named && len(c.Paths) > 0 {
		return ui.Emit(c.JSON, nil, []*cli.Problem{cli.NewProblem(cli.CodeUsage, "args.unexpected",
			fmt.Sprintf("mutation run --site takes no path, as each site names its file, but was given %q", c.Paths[0]),
			"Drop the path; name each mutant with --site.").With("argument", c.Paths[0])})
	}
	if problem := written.belowOne(counts); problem != nil {
		return ui.Emit(c.JSON, nil, []*cli.Problem{problem})
	}
	if written.given("seed") && c.Seed == "" {
		return ui.Emit(c.JSON, nil, []*cli.Problem{cli.NewProblem(cli.CodeUsage, "flags.value-invalid", "--seed needs a non-empty TEXT", "Give it a seed, or leave it out to seed with HEAD.").
			With("flag", "--seed").With("value", "")})
	}
	var sites []mutants.FreshSite
	for _, arg := range c.Site {
		site, ok := namedSite(arg)
		if !ok {
			return ui.Emit(c.JSON, nil, []*cli.Problem{cli.NewProblem(cli.CodeUsage, "flags.value-invalid",
				fmt.Sprintf("--site needs a site, <file>:<line>:<column>, or an identity as a counted run reports it, and was given %q", arg),
				"Name a mutant as mutation run lists it, such as board/board.go:5:11, or by the identity a counted run's report gives it.").
				With("flag", "--site").With("value", arg)})
		}
		sites = append(sites, site)
	}
	changed := "--changed judges the working tree, and --count judges committed inputs only; use --since"
	if named {
		changed = "--changed judges the working tree, and --site judges committed inputs only: commit the work, then name its sites"
	}
	for _, flag := range []struct{ name, why string }{
		{"changed", changed},
		{"no-coverage", mode + " measures coverage freshly, so strict coverage and listed reach are never bypassed"},
		{"use-existing-coverage", mode + " measures coverage freshly from the committed inputs"},
		{"coverage-command", mode + " measures coverage freshly with the built-in commands"},
		{"coverage-report", mode + " measures coverage freshly from the committed inputs"},
		{"test-command", mode + " runs each mutant's own tests, the per-language default"},
		{"mutate-all", mode + " runs every selected site fresh already"},
		{"fail-fast", mode + " judges every selected site and stays aggregate; --fail-fast stops complete runs"},
	} {
		if written.given(flag.name) {
			return ui.Emit(c.JSON, nil, []*cli.Problem{flagConflict("--"+flag.name, flag.why)})
		}
	}
	if h.Platform != "linux" && h.Platform != "darwin" {
		return ui.Emit(c.JSON, nil, []*cli.Problem{cli.NewProblem(cli.CodeEnvironment, "count.platform",
			fmt.Sprintf("mutation run %s supports Linux and macOS, where every command's process tree is owned; this is %s", mode, h.Platform),
			fmt.Sprintf("Run it on Linux or macOS, or run mutation run without %s; native Windows support is #29.", mode)).With("platform", h.Platform)})
	}
	progress := &countedProgress{Progress: Progress{Log: ui.Stderr, Rel: ui.Rel}, p: &Presentation{UI: ui, JSON: c.JSON, Tree: h.Tree}}
	err := h.Counted(ctx, CountedQuery{Paths: c.Paths, Since: c.Since, Seed: c.Seed, Count: c.Count, Sites: sites, OwnTestsOnly: c.OwnTests, Repeat: c.Repeat, AllTests: c.AllTests,
		FailUncovered: c.FailUncovered != nil, LinesOnly: c.FailUncovered != nil && *c.FailUncovered == "lines",
		Workers: c.Workers, TimeoutFactor: c.TimeoutFactor, OutputFactor: c.OutputFactor, MemoryFactor: c.MemoryFactor, Progress: progress})
	if err != nil {
		return ui.Emit(c.JSON, nil, ui.Problems(countedError(err, ui.Rel)))
	}
	return ui.Emit(c.JSON, progress.report, progress.p.Problems)
}

// namedSite reads a --site value: an identity, told apart as mutation
// except tells it, its file from the repository's top, or a site,
// <file>:<line>:<column>, its file as written, from the working directory.
func namedSite(arg string) (mutants.FreshSite, bool) {
	if mutants.IsIdentity(arg) {
		id, ok := mutants.ParseIdentity(arg)
		return mutants.FreshSite{Argument: arg, File: id.File, Identity: &id}, ok
	}
	file, line, column, ok := parseSite(arg)
	return mutants.FreshSite{Argument: arg, File: file, Line: line, Column: column}, ok
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
		_, _ = fmt.Fprintf(r.Log, "itos-cc: not built on %s, so %s is not judged here\n", f.Platform, r.p.plain(f.Path))
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
		// coverage is judged, on the lines the range changed alone; a named
		// site that is uncovered fails, strict or not.
		plan := found.Plan
		p.reportCounted(found.Report, func(site countedSite) bool {
			return found.Named || found.Strict && (!found.LinesOnly || plan.ChangedLines(site.File, site.Line, site.Line))
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

// printCounted writes the plain report: the draw, or the sites named, each
// selected site, and what a sampled pass does and does not prove.
func (p *Presentation) printCounted(r *countedResult) {
	if p.JSON {
		return
	}
	out := p.UI.Stdout
	s := r.Sampling
	switch {
	case s.Algorithm == mutants.FreshNamedAlgorithm:
		_, _ = fmt.Fprintf(out, "named mutation run at %s: %d named, %d executed, each judged alone\n", s.Commit, s.Selected, s.Executed)
	case s.Completion == "not-applicable":
		_, _ = fmt.Fprintf(out, "counted mutation run at %s: seed %s, algorithm %s\n", s.Commit, s.Seed, s.Algorithm)
		_, _ = fmt.Fprintf(out, "not applicable: the committed selection has no eligible mutation site (%d eligible, %d selected)\n", s.Eligible, s.Selected)
	default:
		_, _ = fmt.Fprintf(out, "counted mutation run at %s: seed %s, algorithm %s\n", s.Commit, s.Seed, s.Algorithm)
		_, _ = fmt.Fprintf(out, "budget %d: %d of %d eligible sites selected, %d executed, %d omitted\n", s.Budget, s.Selected, s.Eligible, s.Executed, s.Omitted)
	}
	for _, site := range r.Selected {
		what := site.State
		if site.Outcome != "" {
			what = site.Outcome
		}
		if site.Flaky {
			what = "flaky"
		}
		line := fmt.Sprintf("  %s %s → %s in %s, %s", what, Quote(site.Original), Quote(site.Replacement), site.Function, SiteAt(p.plain(site.File), site.Line, site.Column))
		if site.Flaky {
			line += ": " + strings.Join(site.Outcomes, ", ")
		}
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
	assurance := "sampled assurance: only the judgments above are proven; this is not a complete mutation result, and no snapshot, annotation or coverage cache was written"
	if r.OwnTests {
		assurance += "; own tests only: the tests mutation.tests lists did not run"
	}
	_, _ = fmt.Fprintln(out, assurance)
	_, _ = fmt.Fprintln(out, s.Bounds)
}

// reportCounted reports each selected site that fails the run: a flaky
// site, whose runs under --repeat gave outcomes not all alike, a survivor
// no valid exception excepts, an uncovered site failUncovered says fails
// (with --fail-uncovered, every one, or with =lines one on a changed line),
// the listed selection a blocked site needs, which fails without any
// mutant, once, and a site whose trial could not run.
func (p *Presentation) reportCounted(r *countedResult, failUncovered func(countedSite) bool, interrupted bool) {
	selections := map[string]bool{}
	for _, site := range r.Selected {
		m := mutants.Mutant{Line: site.Line, Column: site.Column, Original: site.Original, Replacement: site.Replacement}
		switch {
		case site.State == "judged" && site.Flaky:
			// Its tests decide it differently with nothing changed: the fix
			// is in them, as for a flaky mutant sample finds.
			p.report(countedMutantProblem(site, m, "mutation.flaky",
				fmt.Sprintf("gave %s in its %d runs: its tests decide it differently with nothing changed", strings.Join(site.Outcomes, ", "), len(site.Outcomes)),
				flakyFix).With("outcome", site.Outcome).With("outcomes", site.Outcomes))
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
