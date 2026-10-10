package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"

	"github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/mutate"
)

// countedPlatform is the platform counted mode runs on: Linux only, since
// only Linux owns every process tree a trial starts (Windows is #29).
var countedPlatform = runtime.GOOS

// countedBounds says what --count bounds, in the report and in plain output.
const countedBounds = "--count bounds mutant trials, not discovery, coverage, listing, baseline commands or total latency"

// countedResult is the --json object of a counted run.
type countedResult struct {
	Sampling countedSampling    `json:"sampling"`
	Selected []countedSite      `json:"selected"`
	Subjects countedSubjects    `json:"subjects"`
	Stages   []PreparationStage `json:"stages"`
}

// countedSampling is what was drawn and what of it ran. Assurance is
// "sampled": a pass proves the judgments listed, never the population.
type countedSampling struct {
	Budget     int    `json:"budget"`
	Eligible   int    `json:"eligible"`
	Selected   int    `json:"selected"`
	Executed   int    `json:"executed"`
	Omitted    int    `json:"omitted"`
	Seed       string `json:"seed"`
	Algorithm  string `json:"algorithm"`
	Commit     string `json:"commit"`
	Since      string `json:"since,omitempty"`
	SinceBase  string `json:"since_base,omitempty"`
	Assurance  string `json:"assurance"`
	Completion string `json:"completion"`
	Stop       string `json:"stop,omitempty"`
	Bounds     string `json:"bounds"`
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
	Scope       string         `json:"scope,omitempty"`
	Tests       []string       `json:"tests,omitempty"`
	Excepted    string         `json:"excepted,omitempty"`
	Reason      string         `json:"reason,omitempty"`
	Stages      []countedStage `json:"stages,omitempty"`
	// selection is, for a blocked site, the listed selection that fails
	// without any mutant.
	selection *mutate.FailedSelection
}

// countedStage is one stage of a site's trial: "own", "listed-baseline" or
// "listed" (mutate.FreshStage).
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

// runCountedMutate is mutation run --count: a fresh judgment of at most N
// committed sites, drawn with the seed from the HEAD commit's inputs. It
// writes no snapshot, annotation or coverage cache.
func runCountedMutate(in *invocation) (any, error) {
	result := &countedResult{Selected: []countedSite{}, Subjects: countedSubjects{Judged: []countedSubject{}, Omitted: []countedSubject{}},
		Stages: []PreparationStage{}}
	if !in.set("count") {
		return nil, flagConflict("--seed", "--seed seeds the selection of --count, and is given without it")
	}
	count := in.integer("count")
	if count < 1 {
		return nil, fail(kindUsage, "flags.value-invalid", fmt.Sprintf("--count needs a whole number of at least 1, and was given %q", in.str("count")),
			"Give it 1 or more.").with("flag", "--count").with("value", in.str("count"))
	}
	if in.set("seed") && in.str("seed") == "" {
		return nil, fail(kindUsage, "flags.value-invalid", "--seed needs a non-empty TEXT", "Give it a seed, or leave it out to seed with HEAD.").
			with("flag", "--seed").with("value", "")
	}
	for _, flag := range []struct{ name, why string }{
		{"changed", "--changed judges the working tree, and --count judges committed inputs only; use --since"},
		{"no-coverage", "--count measures coverage freshly, so strict coverage and listed reach are never bypassed"},
		{"use-existing-coverage", "--count measures coverage freshly from the committed inputs"},
		{"coverage-command", "--count measures coverage freshly with the built-in commands"},
		{"coverage-report", "--count measures coverage freshly from the committed inputs"},
		{"test-command", "--count runs each mutant's own tests, the per-language default"},
		{"mutate-all", "--count runs every selected site fresh already"},
	} {
		if in.set(flag.name) {
			return nil, flagConflict("--"+flag.name, flag.why)
		}
	}
	if countedPlatform != "linux" {
		return nil, fail(kindMissing, "count.platform",
			fmt.Sprintf("mutation run --count supports Linux only, where every command's process tree is owned; this is %s", countedPlatform),
			"Run it on Linux, or run mutation run without --count; native Windows support is #29.").with("platform", countedPlatform)
	}
	root, err := countedRoot()
	if err != nil {
		return nil, err
	}
	paths, err := countedPaths(root, in.args)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	plan, err := mutate.PlanFreshContext(ctx, root, paths, in.str("since"), count, in.str("seed"))
	if err != nil {
		return nil, countedPlanError(in, err)
	}
	defer plan.Close()
	s := &result.Sampling
	*s = countedSampling{Budget: count, Eligible: len(plan.Eligible), Selected: len(plan.Selected), Omitted: len(plan.Omitted),
		Seed: plan.Seed, Algorithm: plan.Algorithm, Commit: plan.Commit, Since: plan.SinceRef, SinceBase: plan.SinceBase,
		Assurance: "sampled", Completion: "completed", Bounds: countedBounds}
	result.Subjects = countedSubjectsOf(plan)
	for _, c := range plan.Selected {
		result.Selected = append(result.Selected, countedSite{Identity: c.Identity, File: c.Path, Line: c.Site.Line, Column: c.Site.Column,
			Function: c.Function, Original: c.Site.Original, Replacement: c.Site.Replacement, State: "unattempted"})
	}
	strict := in.set("fail-uncovered")
	strictGo := strict && hasGoUnit(plan.Units)
	if len(plan.Eligible) == 0 {
		s.Assurance, s.Completion = "not-applicable", "not-applicable"
	}
	if len(plan.Eligible) == 0 && !strictGo {
		result.Stages = append(result.Stages, PreparationStage{Name: "preparation", State: "skipped"})
		printCounted(in, result)
		return result, nil
	}

	scope := coverage.OwnTests
	if in.set("all-tests") {
		scope = coverage.AllTests
	}
	prep, err := prepareFreshContext(ctx, plan, freshPreparationOptions{Scope: scope, Log: os.Stderr})
	if prep != nil {
		result.Stages = append(result.Stages, prep.Stages...)
	}
	if err != nil {
		failed := "preparation"
		if prep != nil && len(prep.Stages) > 0 {
			failed = prep.Stages[len(prep.Stages)-1].Name
		}
		if s.Completion != "not-applicable" {
			s.Completion = "stopped"
		}
		s.Stop = "preparation failed at " + failed
		k := kindNo
		if failed == "tools" {
			k = kindMissing
		}
		in.report(fail(k, "count.preparation-failed", fmt.Sprintf("counted preparation failed at %s, so no mutant was judged: %v", failed, err),
			"Make the committed tests and measurement succeed, then run again.").with("stage", failed))
		printCounted(in, result)
		return result, nil
	}

	exceptions, err := mutate.PlanExceptions(plan, prep.Exceptions)
	if err != nil {
		return nil, err
	}
	stale := slices.Clone(exceptions.Stale)
	var listed *mutate.Listed
	if prep.Tests != nil {
		listed = &mutate.Listed{Root: plan.FrozenRoot, Select: prep.Tests.Select}
	}
	var trials []mutate.FreshTrial
	var trialSites []int
	for i, c := range plan.Selected {
		path := filepath.Join(plan.FrozenRoot, filepath.FromSlash(c.Path))
		site := &result.Selected[i]
		if countedUncovered(prep.Report, path, c.Site.Line) {
			site.State, site.Outcome = "uncovered", mutate.Uncovered
			continue
		}
		baseline, ok := baselineOf(prep.Baselines, c.Path)
		if !ok {
			site.State, site.Reason = "failed", "no clean baseline was prepared for its file"
			continue
		}
		var reach []string
		if listed != nil {
			reach = prep.Report.LineTests(path, c.Site.Line)
		}
		trials = append(trials, mutate.FreshTrial{Candidate: c, Baseline: baseline, Reach: reach})
		trialSites = append(trialSites, i)
	}
	if len(trials) == 0 {
		result.Stages = append(result.Stages, PreparationStage{Name: "trials", State: "skipped"})
	} else {
		results, err := mutate.RunFreshTrials(ctx, plan, trials, mutate.FreshTrialOptions{
			Workers: in.integer("workers"), TimeoutFactor: in.float("timeout-factor"), Scope: mutate.RunScope("", in.set("all-tests")),
			Listed: listed, Prepare: countedCommandEnv, Log: os.Stderr})
		if err != nil {
			return nil, err
		}
		trialStage := PreparationStage{Name: "trials", State: "complete"}
		var selectionStage *PreparationStage
		for n, r := range results {
			site := &result.Selected[trialSites[n]]
			for _, stage := range r.Stages {
				site.Stages = append(site.Stages, countedStage{Name: stage.Name, State: stage.State, Outcome: stage.Outcome, Tests: stage.Tests, Error: stage.Error})
				switch {
				case stage.Name == "own" && stage.State == "complete":
					// The mutant ran: its trial spent the budget, whatever
					// its later stages could judge.
					s.Executed++
				case stage.Name == "listed-baseline" && selectionStage == nil:
					selectionStage = &PreparationStage{Name: stage.Name, State: stage.State, Error: stage.Error}
				case stage.Name == "listed-baseline" && stage.State == "failed" && selectionStage.State != "failed":
					selectionStage.State, selectionStage.Error = stage.State, stage.Error
				}
			}
			switch {
			case r.Err != nil:
				site.State, site.Reason = "failed", r.Err.Error()
			case r.FailedSelection != nil:
				site.State, site.selection = "blocked", r.FailedSelection
				site.Reason = fmt.Sprintf("it survived its own tests, and its listed tests %s fail without any mutant, so they could not judge it",
					strings.Join(r.FailedSelection.IDs, " "))
			default:
				site.State, site.Outcome, site.Scope, site.Tests = "judged", r.Outcome, r.Scope, r.Tests
				reason, gone := exceptions.Judge(r.Trial.Candidate, r.Outcome)
				site.Excepted = reason
				if gone != nil {
					stale = append(stale, *gone)
				}
				continue
			}
			if trialStage.State == "complete" {
				trialStage.State, trialStage.Error = "failed", site.Reason
			}
		}
		if selectionStage != nil {
			result.Stages = append(result.Stages, *selectionStage)
		}
		result.Stages = append(result.Stages, trialStage)
	}
	for _, site := range result.Selected {
		switch {
		case s.Stop != "":
		case site.State == "failed" || site.State == "unattempted":
			s.Completion, s.Stop = "stopped", "a selected trial could not run"
		case site.State == "blocked":
			s.Completion, s.Stop = "stopped", "the listed tests a selected site needs fail without any mutant"
		}
	}
	printCounted(in, result)
	reportCounted(in, result, strict)
	for _, e := range stale {
		reportStaleExceptions(in, e.File, []mutate.StaleException{e})
	}
	if strictGo {
		reportCountedStrictGo(in, plan, prep)
	}
	return result, nil
}

// countedRoot is the top of the Git repository holding the working
// directory, physical path, or count.no-git.
func countedRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", countedNoGit(err)
	}
	root, err := filepath.EvalSymlinks(strings.TrimSpace(string(out)))
	if err != nil {
		return "", countedNoGit(err)
	}
	return root, nil
}

func countedNoGit(err error) error {
	return fail(kindMissing, "count.no-git", "mutation run --count needs a Git repository with a commit at HEAD: "+err.Error(),
		"Run it inside a Git repository, after committing the code to judge.")
}

// countedPaths turns the paths given, relative to the working directory,
// into slash paths relative to root.
func countedPaths(root string, args []string) ([]string, error) {
	if len(args) == 0 {
		return nil, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if physical, err := filepath.EvalSymlinks(wd); err == nil {
		wd = physical
	}
	var paths []string
	for _, arg := range args {
		abs := arg
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(wd, arg)
		}
		rel, err := filepath.Rel(root, filepath.Clean(abs))
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, fail(kindUsage, "paths.unmatched", fmt.Sprintf("%q is outside the repository that --count judges", arg),
				"Name paths inside the repository.").with("argument", arg)
		}
		paths = append(paths, filepath.ToSlash(rel))
	}
	return paths, nil
}

// countedPlanError is the problem a refused fresh plan reports.
func countedPlanError(in *invocation, err error) error {
	switch {
	case errors.Is(err, mutate.ErrFreshRepository):
		return countedNoGit(err)
	case errors.Is(err, mutate.ErrFreshSince):
		return fail(kindUsage, "since.bad-ref", fmt.Sprintf("--since %s: not a commit in this repository", in.str("since")),
			"Name a commit, branch, or tag that exists, such as origin/main.").with("ref", in.str("since"))
	case errors.Is(err, mutate.ErrFreshPath):
		return fail(kindUsage, "paths.unmatched", err.Error(), "Name committed source paths.")
	case errors.Is(err, mutate.ErrFreshUnsupported):
		return fail(kindUsage, "count.unsupported-scope", "mutation run --count cannot judge this repository's committed inputs: "+err.Error(),
			"Judge paths without it, or run mutation run without --count.")
	}
	return err
}

func countedSubjectsOf(plan *mutate.FreshPlan) countedSubjects {
	out := countedSubjects{Judged: []countedSubject{}, Omitted: []countedSubject{}}
	judged := map[countedSubject]bool{}
	for _, c := range plan.Selected {
		judged[countedSubject{File: c.Path, Function: c.Function}] = true
	}
	omitted := map[countedSubject]bool{}
	for _, c := range plan.Omitted {
		if subject := (countedSubject{File: c.Path, Function: c.Function}); !judged[subject] {
			omitted[subject] = true
		}
	}
	for subject := range judged {
		out.Judged = append(out.Judged, subject)
	}
	for subject := range omitted {
		out.Omitted = append(out.Omitted, subject)
	}
	for _, list := range [][]countedSubject{out.Judged, out.Omitted} {
		sort.Slice(list, func(i, j int) bool {
			if list[i].File != list[j].File {
				return list[i].File < list[j].File
			}
			return list[i].Function < list[j].Function
		})
	}
	return out
}

// countedUncovered says whether the fresh measurement shows no test, own
// or listed, executing line of path, as complete mode marks a mutant
// uncovered.
func countedUncovered(report *coverage.Report, path string, line int) bool {
	if len(report.LineTests(path, line)) > 0 {
		return false
	}
	if !report.Has(path) {
		spec := lang.Detect(path)
		return spec != nil && report.Measures(spec.Name)
	}
	covered, measured := report.LineCovered(path, line)
	return measured && !covered
}

func baselineOf(baselines []mutate.FreshBaseline, path string) (mutate.FreshBaseline, bool) {
	for _, b := range baselines {
		for _, p := range b.Paths {
			if p == path {
				return b, true
			}
		}
	}
	return mutate.FreshBaseline{}, false
}

func hasGoUnit(units []mutate.FreshUnit) bool {
	for _, unit := range units {
		if strings.HasSuffix(unit.Path, ".go") {
			return true
		}
	}
	return false
}

// countedCommandEnv gives a trial the environment preparation commands
// get: no parent Git directory, work-tree or index override, and Go
// commands offline with the local toolchain.
func countedCommandEnv(cmd *exec.Cmd) {
	cmd.Env = withoutGitOverrides(cmd.Env)
	if strings.EqualFold(filepath.Base(cmd.Path), "go") {
		cmd.Env = withEnvOverrides(cmd.Env, map[string]string{"GOTOOLCHAIN": "local", "GOPROXY": "off", "GOSUMDB": "off"})
	}
}

// printCounted writes the plain report: the draw, each selected site, and
// what a sampled pass does and does not prove.
func printCounted(in *invocation, r *countedResult) {
	if in.json {
		return
	}
	s := r.Sampling
	fmt.Printf("counted mutation run at %s: seed %s, algorithm %s\n", s.Commit, s.Seed, s.Algorithm)
	if s.Completion == "not-applicable" {
		fmt.Printf("not applicable: the committed selection has no eligible mutation site (%d eligible, %d selected)\n", s.Eligible, s.Selected)
	} else {
		fmt.Printf("budget %d: %d of %d eligible sites selected, %d executed, %d omitted\n", s.Budget, s.Selected, s.Eligible, s.Executed, s.Omitted)
	}
	for _, site := range r.Selected {
		what := site.State
		if site.Outcome != "" {
			what = site.Outcome
		}
		line := fmt.Sprintf("  %s %s:%d:%d %s → %s in %s", what, site.File, site.Line, site.Column, quote(site.Original), quote(site.Replacement), site.Function)
		if site.Scope == mutate.ScopeListed {
			line += " by listed tests " + strings.Join(site.Tests, " ")
		}
		if site.Excepted != "" {
			line += " (excepted: " + site.Excepted + ")"
		}
		if site.Reason != "" {
			line += ": " + site.Reason
		}
		fmt.Println(line)
	}
	if s.Stop != "" {
		fmt.Printf("stopped: %s\n", s.Stop)
	}
	fmt.Println("sampled assurance: only the judgments above are proven; this is not a complete mutation result, and no snapshot, annotation or coverage cache was written")
	fmt.Println(countedBounds)
}

// reportCounted reports each selected site that fails the run: a survivor
// no valid exception excepts, an uncovered site with --fail-uncovered, the
// listed selection a blocked site needs, which fails without any mutant,
// once, and a site whose trial could not run.
func reportCounted(in *invocation, r *countedResult, failUncovered bool) {
	selections := map[string]bool{}
	for _, site := range r.Selected {
		m := mutate.Mutant{Line: site.Line, Column: site.Column, Original: site.Original, Replacement: site.Replacement}
		switch {
		case site.State == "judged" && site.Outcome == mutate.Survived && site.Excepted == "":
			p := countedMutantProblem(site, m, "mutation.survived", "survived", "Add a test that fails with this change.")
			in.report(p)
		case site.State == "uncovered" && failUncovered:
			in.report(countedMutantProblem(site, m, "mutation.uncovered", "is uncovered: no test executes its line",
				"Add a test that executes this line and fails with this change."))
		case site.State == "blocked" && site.selection != nil:
			reportFailedSelections(in, []mutate.FailedSelection{*site.selection}, selections, "mutation run --count")
		case site.State == "failed" || site.State == "unattempted" || site.State == "blocked":
			reason := site.Reason
			if reason == "" {
				reason = "it was not attempted"
			}
			in.report(countedMutantProblem(site, m, "count.trial-failed", "was not judged: "+reason,
				"Fix what stopped its trial, then run again."))
		}
	}
}

func countedMutantProblem(site countedSite, m mutate.Mutant, rule, verdict, fix string) *problem {
	return fail(kindNo, rule,
		fmt.Sprintf("%s:%d:%d: %s → %s in %s %s", site.File, m.Line, m.Column, quote(m.Original), quote(m.Replacement), site.Function, verdict), fix).
		with("file", site.File).with("line", m.Line).with("column", m.Column).with("function", site.Function).
		with("original", m.Original).with("replacement", m.Replacement).with("identity", site.Identity)
}

// reportCountedStrictGo reports, for every admitted Go function, sites or
// none, what complete strict mode reports: a function without complete
// evidence and each uncovered executable block.
func reportCountedStrictGo(in *invocation, plan *mutate.FreshPlan, prep *FreshPreparation) {
	for _, unit := range plan.Units {
		if !strings.HasSuffix(unit.Path, ".go") {
			continue
		}
		evidence := prep.GoCoverage[unit.Identity]
		if evidence == nil || !evidence.Complete {
			reportCoverageProblem(in, unit.Path, unit.Function, unit.StartLine, "mutation.coverage-missing", "has no complete measured Go coverage inventory")
			continue
		}
		for _, block := range evidence.Blocks {
			if !block.Covered {
				reportCoverageProblem(in, unit.Path, unit.Function, block.Line, "mutation.uncovered-statement", "has an uncovered executable Go coverage block")
			}
		}
	}
}
