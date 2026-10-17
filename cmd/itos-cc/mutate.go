package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/config"
	covrun "github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
	"github.com/donvargax/itos-cc/internal/lang"
	mutation "github.com/donvargax/itos-cc/internal/mutants"
	mutationslice "github.com/donvargax/itos-cc/internal/mutation"
	"github.com/donvargax/itos-cc/internal/snapshot"
	"github.com/donvargax/itos-cc/project"
)

type mutateFile struct {
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
	Mutants []mutateMutant `json:"mutants"`
	// State and Work are there with --fail-fast only: whether the run
	// completed the file, stopped inside it, never started its work, or
	// found it blocked by a failing baseline or selection, and how many of
	// its mutants ended in each state.
	State string      `json:"state,omitempty"`
	Work  *mutateWork `json:"work,omitempty"`
	// Cache is there with --fail-fast only: "complete" when the file's
	// snapshot, as the run leaves it, holds a valid result for every
	// mutant of its functions judged, else "incomplete". A stopped run's
	// own work, State, can be incomplete while the cache is complete.
	Cache string `json:"cache,omitempty"`
}

// mutateWork counts selected mutants by their fail-fast state, disjointly:
// completed (run, reused or measured uncovered, with an outcome),
// cancelled, unattempted and blocked (with none).
type mutateWork struct {
	Completed   int `json:"completed"`
	Cancelled   int `json:"cancelled"`
	Unattempted int `json:"unattempted"`
	Blocked     int `json:"blocked"`
}

func (w *mutateWork) add(o *mutateWork) {
	w.Completed += o.Completed
	w.Cancelled += o.Cancelled
	w.Unattempted += o.Unattempted
	w.Blocked += o.Blocked
}

// mutateStop is, with --fail-fast, whether the run stopped early, and at
// which failure: the rule and subject of its problem.
type mutateStop struct {
	Stopped bool           `json:"stopped"`
	Rule    string         `json:"rule,omitempty"`
	Subject map[string]any `json:"subject,omitempty"`
}

// mutateMutant is a site, less its file, and how it was decided: a
// timed-out mutant is "timeout" here and counts in "killed".
type mutateMutant struct {
	Line        int    `json:"line"`
	Column      int    `json:"column"`
	Function    string `json:"function"`
	Original    string `json:"original"`
	Replacement string `json:"replacement"`
	// Outcome and Scope are there for every mutant but one a --fail-fast
	// stop left undecided.
	Outcome string `json:"outcome,omitempty"`
	Reused  bool   `json:"reused"`
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

type mutateResult struct {
	Files []mutateFile `json:"files"`
	// Stop and Work are there with --fail-fast only.
	Stop *mutateStop `json:"stop,omitempty"`
	Work *mutateWork `json:"work,omitempty"`
	// Orphans are the snapshots under the run's paths whose source file
	// was gone, which it removed; left out when there were none, so a run
	// with no orphan prints the keys it always did.
	Orphans []orphanEntry `json:"orphans,omitempty"`
}

// mutationSelection is the sources mutation run, check, and sample take:
// the paths and --changed, and with --since the files the range changed or
// renamed, with judge saying which of their functions it changed, renamed
// the path each renamed file had at the ref, "" for the others, and lines
// which lines of a source the range added or changed, as HEAD has them
// (project.ChangedFunctions.Lines). judge, renamed and lines are nil
// without --since.
func mutationSelection(ctx context.Context, in *invocation) (sources []string, judge func(path, function, hash string) bool, renamed func(path string) string, lines mutation.LineScope, err error) {
	var since map[string]mutation.ChangedFunctions
	var moves map[string]string
	if in.set("since") {
		if since, moves, err = changedSince(ctx, in); err != nil {
			return nil, nil, nil, nil, err
		}
	}
	files, err := files(ctx, in)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if since == nil {
		return files.Sources, nil, nil, nil, nil
	}
	// The range selects the files; paths only narrow it, by the path a
	// renamed file has now.
	for _, f := range files.Sources {
		if _, ok := since[f]; ok {
			sources = append(sources, f)
		}
	}
	callerDir := in.adapters.Dir
	return sources, func(path, function, hash string) bool { return since[path].Judges(function, hash) },
		func(path string) string { return moves[path] },
		func(path string, start, end int) bool {
			path = mutationPath(callerDir, path)
			return since[path].Changes(start, end)
		}, nil
}

// changedLinesOnly says whether strict coverage judges only the lines a
// --since range added or changed: --fail-uncovered=lines. Bare
// --fail-uncovered is =functions, whole functions. =lines without --since
// has no lines to judge, and is a usage error.
func changedLinesOnly(in *invocation) (bool, error) {
	if in.str("fail-uncovered") != "lines" {
		return false, nil
	}
	if !in.set("since") {
		return false, cli.NewProblem(cli.CodeUsage, "flags.conflict", "--fail-uncovered=lines judges only the lines a --since range changed, and was given without --since",
			"Add --since REF, such as --since origin/main, or write --fail-uncovered alone to judge whole functions.").With("flag", "--fail-uncovered=lines")
	}
	return true, nil
}

// uncoveredFails is whether an uncovered mutant on a line of the source at
// path fails: never without --fail-uncovered (nil), and with it on every
// line, or with --fail-uncovered=lines on a line the --since range added or
// changed (scope).
func uncoveredFails(failUncovered bool, scope mutation.LineScope, path string) func(line int) bool {
	if !failUncovered {
		return nil
	}
	return func(line int) bool { return scope.Judges(path, line, line) }
}

// importingTests lists the test files that reach each source
// (graph.TestsImporting), as the graph of the project under the project root
// resolves imports, whatever command runs the mutants and wherever it runs:
// what a snapshot records the hashes of. It reads under ctx.
func importingTests(ctx context.Context, in *invocation) (func(path string) []string, error) {
	root := project.Root()
	if in.adapters.Reach == nil {
		return nil, errors.New("mutation Reach binding is missing")
	}
	if in.adapters.Tree == nil {
		return nil, errors.New("mutation source tree binding is missing")
	}
	sources, testFiles, failure := in.adapters.Tree.Discover(ctx, []string{root})
	if failure != nil {
		return nil, failure
	}
	tests, err := in.adapters.Reach.TestsImporting(ctx, root, sources, testFiles)
	if err != nil {
		return nil, err
	}
	callerDir := in.adapters.Dir
	return func(path string) []string {
		path = mutationPath(callerDir, path)
		return tests[path]
	}, nil
}

func runMutate(ctx context.Context, in *invocation) (any, error) {
	linesOnly, err := changedLinesOnly(in)
	if err != nil {
		return nil, err
	}
	if in.set("count") || in.set("seed") {
		return runCountedMutate(ctx, in)
	}
	result := mutateResult{Files: []mutateFile{}}
	if in.set("fail-uncovered") && in.set("no-coverage") {
		return result, flagConflict("--no-coverage", "--no-coverage conflicts with --fail-uncovered, which requires measured Go executable coverage")
	}
	failFast := in.set("fail-fast")
	if failFast {
		if countedPlatform != "linux" && countedPlatform != "darwin" {
			return result, cli.NewProblem(cli.CodeEnvironment, "fail-fast.platform",
				fmt.Sprintf("mutation run --fail-fast supports Linux and macOS, where every command's process tree is owned and a stop can end it; this is %s", countedPlatform),
				"Run it on Linux or macOS, or run mutation run without --fail-fast; native Windows support is #29.").With("platform", countedPlatform)
		}
		result.Stop, result.Work = &mutateStop{}, &mutateWork{}
	}
	cfg, err := loadConfig()
	if err != nil {
		return result, err
	}
	sources, judge, renamed, lines, err := mutationSelection(ctx, in)
	if err != nil {
		return result, err
	}
	// With --fail-uncovered=lines, strict coverage and uncovered mutants
	// are judged on the lines the range changed alone; evidence is still
	// measured and recorded for whole functions.
	var scope mutation.LineScope
	if linesOnly {
		scope = lines
	}
	elsewhere, moved, err := exceptionsElsewhere(ctx, in, cfg.Exceptions, sources, renamed)
	if err != nil {
		return result, err
	}
	if len(sources) == 0 {
		fmt.Fprintln(os.Stderr, "itos-cc: no source files to mutate")
		reportElsewhere(ctx, in, elsewhere)
		result.Orphans, err = removeOrphans(ctx, in)
		return result, err
	}
	// Strict coverage proves the functions of each strict language of the
	// selection (mutation.StrictCoverage) executed.
	var strictLanguages []string
	if in.set("fail-uncovered") {
		strictLanguages = strictLanguagesOf(sources)
	}
	strictGo := slices.Contains(strictLanguages, "go")
	strict := len(strictLanguages) > 0
	if strict {
		for _, flag := range []string{"--coverage-report", "--use-existing-coverage", "--coverage-command"} {
			if flagPresent(in, flag) {
				return result, flagConflict(flag, flag+" is not admitted with strict "+languageName(strictLanguages[0])+" coverage; use a fresh built-in or listed measurement")
			}
		}
	}
	if in.adapters.Tree == nil || in.adapters.Surroundings == nil {
		return result, errors.New("mutation source tree binding is missing")
	}
	tests, err := importingTests(ctx, in)
	if err != nil {
		return result, err
	}
	support, err := supportNow(ctx, in, cfg)
	if err != nil {
		return result, err
	}
	// Python's and Kotlin's own coverage is that of the tests that reach
	// each file, as its own command runs them; --test-command and
	// --all-tests keep the whole suite's.
	var reach covrun.Reach
	if !in.set("test-command") && !in.set("all-tests") {
		reach = tests
	}
	if in.adapters.Toolchain == nil {
		return result, errors.New("mutation Toolchain binding is missing")
	}
	if in.adapters.Fingerprints == nil {
		return result, errors.New("mutation Fingerprints binding is missing")
	}
	if in.adapters.Workers == nil {
		return result, errors.New("mutation Workers binding is missing")
	}
	if in.adapters.Snapshots == nil {
		return result, errors.New("mutation snapshots binding is missing")
	}
	if in.adapters.SourceEdits == nil {
		return result, errors.New("mutation source edits binding is missing")
	}
	if !in.set("no-coverage") && in.adapters.MutationCoverage == nil {
		return result, errors.New("mutation coverage capability is not bound")
	}
	opt := mutation.Options{
		Store: in.adapters.Snapshots, Source: in.adapters.SourceEdits, Tree: in.adapters.Tree, Surroundings: in.adapters.Surroundings,
		Root: project.Root(), Dir: in.adapters.Dir,
		WorkerFactory: in.adapters.Workers,
		Toolchain:     in.adapters.Toolchain,
		Fingerprints:  in.adapters.Fingerprints,
		Support:       support,
		Tests:         tests,
		Workers:       in.integer("workers"),
		MutateAll:     in.set("mutate-all"),
		TimeoutFactor: in.float("timeout-factor"),
		TestCommand:   in.str("test-command"),
		AllTests:      in.set("all-tests"),
		Annotate:      !in.set("no-annotate"),
		Progress:      RunProgress{Log: os.Stderr},
		Judge:         judge,
		Renamed:       renamed,
		// An entry whose function moved to another file still excepts
		// its mutant there.
		Exceptions: append(slices.Clone(cfg.Exceptions), moved...),
	}
	suite := listedConfig(in, cfg)
	if suite != nil {
		opt.Listed = &mutation.Listed{Root: project.Root(), List: suite.List, All: suite.All, Select: suite.Select}
	}
	if strict {
		var cached []mutation.CoverageCheck
		mutationChecks, err := checkMutations(ctx, in, sources, judge, tests, support, append(slices.Clone(cfg.Exceptions), moved...))
		if err != nil {
			return result, err
		}
		cached, err = checkMutationCoverage(ctx, in, sources, judge, strictLanguages, support)
		if err != nil {
			return result, err
		}
		unsupported := false
		for _, check := range cached {
			if check.State == "unsupported" && scope.Judges(check.File, check.Line, check.EndLine) {
				unsupported = true
				reportLanguageCoverageProblem(in, check.Language, check.File, check.Function, check.Line,
					"mutation.coverage-unsupported", "uses unsupported "+languageName(check.Language)+" coverage scope "+strings.Join(check.Changed, ", "))
			}
		}
		if unsupported {
			return result, nil
		}
		canReuse := !mutation.NeedsMutationCoverage(mutationChecks, in.set("mutate-all"))
		allFresh := canReuse
		for _, check := range cached {
			if check.State != "fresh" {
				allFresh = false
			}
		}
		if allFresh {
			byFunction := map[string]*mutation.CoverageEvidence{}
			byFile := map[string][]coverage.GoBlock{}
			byLines := map[string]map[string][]coverage.Line{}
			for _, check := range cached {
				byFunction[check.File+"\x00"+check.Function] = check.Evidence
				path := filepath.Join(project.Root(), filepath.FromSlash(check.File))
				for _, block := range check.Blocks {
					if check.Language != "go" {
						if byLines[check.Language] == nil {
							byLines[check.Language] = map[string][]coverage.Line{}
						}
						byLines[check.Language][path] = append(byLines[check.Language][path], coverage.Line{Line: block.Line, Covered: block.Covered})
						continue
					}
					byFile[path] = append(byFile[path], coverage.GoBlock{
						Span: block.Span, Line: block.Line, Column: block.Column, Weight: block.Weight, Covered: block.Covered,
					})
				}
			}
			opt.CachedCoverage = func(path, function, _ string) *mutation.CoverageEvidence {
				return byFunction[project.Rel(path)+"\x00"+function]
			}
			var report *coverage.Report
			if strictGo {
				report = coverage.FromGoBlocks(byFile)
			}
			for _, language := range strictLanguages {
				if language != "go" {
					report = coverage.Merge(report, coverage.FromLines(language, byLines[language]))
				}
			}
			opt.Coverage = report
		} else {
			// A strict language whose every judged function has fresh
			// evidence and whose mutants all hold is not measured again:
			// its cached evidence stands, as in a run with nothing to
			// measure. Every other language is measured, as before.
			measure := map[string]bool{}
			for _, check := range cached {
				if check.State != "fresh" {
					measure[check.Language] = true
				}
			}
			for i, source := range sources {
				spec := lang.Detect(source)
				if spec != nil && (!mutation.StrictCoverage(spec.Name) ||
					i < len(mutationChecks) && mutation.NeedsMutationCoverage(mutationChecks[i:i+1], in.set("mutate-all"))) {
					measure[spec.Name] = true
				}
			}
			var measured []string
			for _, source := range sources {
				if spec := lang.Detect(source); spec == nil || measure[spec.Name] {
					measured = append(measured, source)
				}
			}
			kept := map[string]*mutation.CoverageEvidence{}
			for _, check := range cached {
				if !measure[check.Language] {
					kept[check.File+"\x00"+check.Function] = check.Evidence
				}
			}
			if len(kept) > 0 {
				opt.CachedCoverage = func(path, function, _ string) *mutation.CoverageEvidence {
					return kept[project.Rel(path)+"\x00"+function]
				}
			}
			report, measuredProducer, _, listedFiles, err := strictCoverage(ctx, in, strictLanguages, measured, suite, support, reach)
			if err != nil {
				return result, err
			}
			if report == nil {
				return result, fmt.Errorf("strict coverage measurement produced no report")
			}
			var missingGo bool
			for _, missing := range report.Missing() {
				missingGo = missingGo || missing.Language == "go"
			}
			// Another language that measured nothing fails closed as Go
			// does, and stops the run with it.
			others := unmeasuredToJudge(report, sources, mutationChecks, in.set("mutate-all"))
			others = append(others, integrationToJudge(report, filesToJudge(sources, mutationChecks, in.set("mutate-all")))...)
			if strictGo && measure["go"] && (missingGo || !report.Measures("go")) {
				for _, p := range unmeasured(report.Missing()) {
					if p.Subject("language") == "go" {
						in.report(p)
					}
				}
				if !missingGo {
					in.report(cli.NewProblem(cli.CodeEnvironment, "coverage.no-report", "Go coverage measurement did not produce executable coverage",
						"Run the built-in Go coverage command successfully, then retry.").With("language", "go"))
				}
				for _, p := range others {
					in.report(p)
				}
				return result, nil
			}
			if len(others) > 0 {
				for _, p := range others {
					in.report(p)
				}
				return result, nil
			}
			opt.StatementCoverage = report
			opt.CoverageProducer = measuredProducer
			opt.RecordCoverageInputs = true
			opt.Coverage = report
			if opt.Listed != nil {
				opt.Listed.Files = listedFiles
			}
		}
	} else if !in.set("no-coverage") {
		opt.Measurement = &mutation.RunMeasurement{Measurer: in.adapters.MutationCoverage, Lister: in.adapters.TestLister,
			Request: covport.Request{Reports: in.strs("coverage-report"), Command: in.str("coverage-command"), Existing: in.set("use-existing-coverage"), AllTests: in.set("all-tests"), Scope: covport.OwnTests, Reach: reach}}

		if in.set("fail-uncovered") && !rawCoverage(in) {
			// Strict: a language the per-language commands measured
			// nothing of fails closed, as Go does, instead of running
			// every mutant, and stops the run before any mutant runs.
			checks, err := checkMutations(ctx, in, sources, judge, tests, support, append(slices.Clone(cfg.Exceptions), moved...))
			if err != nil {
				return result, err
			}
			if mutation.NeedsMutationCoverage(checks, in.set("mutate-all")) {
				report, err := opt.Measure(ctx, sources, true)
				if err != nil {
					return result, mutationRunError(err)
				}
				others := unmeasuredToJudge(report, sources, checks, in.set("mutate-all"))
				others = append(others, integrationToJudge(report, filesToJudge(sources, checks, in.set("mutate-all")))...)
				if len(others) > 0 {
					for _, p := range others {
						in.report(p)
					}
					return result, nil
				}
				opt.Coverage = report
			}
		}
	}

	// Uncovered mutants never run, so without it a function no test
	// executes passes.
	failUncovered := in.set("fail-uncovered")
	var results []mutation.FileResult
	var stop *mutation.Stop
	if failFast {
		ff := mutation.FailFast{FailUncovered: failUncovered, Lines: scope}
		if len(elsewhere) > 0 {
			// Known before anything runs: an entry of a file the run does
			// not select that no longer holds.
			e := elsewhere[0]
			ff.Known = &mutation.Stop{Rule: "mutation.exception-stale", File: project.Rel(project.AtRoot(e.File)), Exception: &e}
		}
		results, stop, err = mutation.RunFailFast(ctx, sources, opt, ff)
	} else {
		results, err = mutation.Run(ctx, sources, opt)
	}
	if err != nil {
		return result, mutationRunError(err)
	}
	if strict {
		reportStrictCoverage(in, results, scope)
	}
	reported := map[string]bool{}
	for _, r := range results {
		// Every file has the baseline this run actually ran for its
		// command, or "not-run". With --fail-fast every file has its state
		// and its work too, and a file not written still lists its
		// mutants, each with its state.
		var work *mutateWork
		state, cache := "", ""
		if failFast {
			work = workOf(r.Mutants)
			result.Work.add(work)
			state, cache = failFastState(r, work), "incomplete"
			if r.Cached {
				cache = "complete"
			}
		}
		if failFast && (r.BaselineFailed || len(r.FailedSelections) > 0) {
			// It counts what it ran and reused before the stop, as its
			// completed work does.
			f := mutateFile{File: r.Rel, Ran: r.Ran, Reused: r.Reused, Baseline: r.Baseline, Judged: r.Judged, Mutants: jsonMutants(r.Mutants),
				State: state, Work: work, Cache: cache}
			countMutants(&f, r.Mutants)
			result.Files = append(result.Files, f)
			if !in.json {
				fmt.Printf("%s: blocked, %s: its tests, or the listed tests its mutants run, fail without any mutant (ran %d, reused %d; %d completed, %d cancelled, %d unattempted, %d blocked); cache %s\n",
					r.Rel, snapshotKept(r), f.Ran, f.Reused, work.Completed, work.Cancelled, work.Unattempted, work.Blocked, cache)
				if r.BaselineFailed {
					fmt.Println(tail(r.BaselineOutput, 20))
				}
			}
			if r.BaselineFailed {
				in.report(cli.NewProblem(cli.CodeNo, "mutation.baseline-failed", r.Rel+": its tests fail before any mutant, so none was judged",
					"Make its tests pass, then run mutation run again.").With("file", r.Rel))
			}
			reportFailedSelections(in, r.FailedSelections, reported, "mutation run")
			for _, m := range r.Mutants {
				reportFailed(in, r.Rel, m.Function, []mutation.Mutant{{Line: m.Line, Column: m.Column, Original: m.Original,
					Replacement: m.Replacement, Outcome: m.Outcome, Excepted: m.Excepted}}, uncoveredFails(failUncovered, scope, r.Rel))
			}
			reportStaleExceptions(ctx, in, r.Rel, r.StaleExceptions)
			continue
		}
		if len(r.FailedSelections) > 0 && !r.BaselineFailed {
			// A mutant the file holds needed listed tests that fail
			// without it: it is not decided, so neither is the file.
			result.Files = append(result.Files, mutateFile{File: r.Rel, Baseline: "failed", Judged: r.Judged, Mutants: []mutateMutant{}})
			if !in.json {
				fmt.Printf("%s: listed tests its mutants run fail without any mutant; snapshot not updated\n", r.Rel)
			}
			reportFailedSelections(in, r.FailedSelections, reported, "mutation run")
			reportStaleExceptions(ctx, in, r.Rel, r.StaleExceptions)
			continue
		}
		if r.BaselineFailed {
			result.Files = append(result.Files, mutateFile{File: r.Rel, Baseline: "failed", Judged: r.Judged, Mutants: []mutateMutant{}})
			if !in.json {
				fmt.Printf("%s: baseline tests fail; snapshot not updated\n%s\n", r.Rel, tail(r.BaselineOutput, 20))
			}
			in.report(cli.NewProblem(cli.CodeNo, "mutation.baseline-failed", r.Rel+": its tests fail before any mutant, so none was judged",
				"Make its tests pass, then run mutation run again.").With("file", r.Rel))
			reportStaleExceptions(ctx, in, r.Rel, r.StaleExceptions)
			continue
		}
		f := mutateFile{File: r.Rel, Ran: r.Ran, Reused: r.Reused, Baseline: r.Baseline, Judged: r.Judged, Mutants: jsonMutants(r.Mutants),
			State: state, Work: work, Cache: cache}
		// Only the functions judged count: the others keep outcomes no
		// change in the range is to blame for.
		judged := map[string]bool{}
		for _, id := range r.Judged {
			judged[id] = true
		}
		var units []mutation.UnitResult
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
			excepted := exceptedIn(u.Mutants)
			f.Killed += u.Killed
			f.Survived += u.Survived - excepted
			f.Excepted += excepted
			f.Uncovered += u.Uncovered
		}
		result.Files = append(result.Files, f)
		if !in.json {
			// "excepted" only where there is one, so a project with no
			// exceptions reads as it always did.
			counts := fmt.Sprintf("%d killed, %d survived, %d uncovered", f.Killed, f.Survived, f.Uncovered)
			if f.Excepted > 0 {
				counts = fmt.Sprintf("%d killed, %d survived, %d excepted, %d uncovered", f.Killed, f.Survived, f.Excepted, f.Uncovered)
			}
			line := fmt.Sprintf("%s: %s (ran %d, reused %d)", r.Rel, counts, f.Ran, f.Reused)
			if r.Judged != nil {
				line += fmt.Sprintf(" (judged %d of %d functions)", len(r.Judged), r.Functions)
			}
			if r.Incomplete {
				line = fmt.Sprintf("%s: stopped early, %s: %s (ran %d, reused %d); %d cancelled, %d unattempted; cache %s",
					r.Rel, snapshotKept(r), counts, f.Ran, f.Reused, work.Cancelled, work.Unattempted, cache)
			}
			fmt.Println(line)
		}
		for _, u := range units {
			reportFailed(in, r.Rel, u.Namespace+"#"+u.Name, u.Mutants, uncoveredFails(failUncovered, scope, r.Rel))
		}
		if r.Incomplete {
			for _, m := range r.Mutants {
				reportFailed(in, r.Rel, m.Function, []mutation.Mutant{{Line: m.Line, Column: m.Column, Original: m.Original,
					Replacement: m.Replacement, Outcome: m.Outcome, Excepted: m.Excepted}}, uncoveredFails(failUncovered, scope, r.Rel))
			}
		}
		reportStaleExceptions(ctx, in, r.Rel, r.StaleExceptions)
	}
	reportElsewhere(ctx, in, elsewhere)
	// Only once the run has written what it judged: a --since run moves a
	// renamed file's snapshot first.
	if result.Orphans, err = removeOrphans(ctx, in); err != nil {
		return result, mutationRunError(err)
	}
	if stop != nil {
		result.Stop = &mutateStop{Stopped: true, Rule: stop.Rule, Subject: stopSubject(stop)}
		if !in.json {
			w := result.Work
			fmt.Printf("stopped early at the first actionable failure, %s %s: %d completed, %d cancelled, %d unattempted, %d blocked; a file it cut short keeps its source as it was, and its snapshot only what was judged\n",
				stop.Rule, describeStop(stop), w.Completed, w.Cancelled, w.Unattempted, w.Blocked)
		}
	}
	return result, nil
}

// snapshotKept says, in plain output, what became of the snapshot of a file
// a fail-fast stop left incomplete.
func snapshotKept(r mutation.FileResult) string {
	if r.Preserved {
		return "snapshot keeps what was judged, source not annotated"
	}
	return "snapshot not updated"
}

// jsonMutants is the --json form of mutation.
func jsonMutants(mutants []mutation.MutantResult) []mutateMutant {
	out := []mutateMutant{}
	for _, m := range mutants {
		out = append(out, mutateMutant{Line: m.Line, Column: m.Column, Function: m.Function,
			Original: m.Original, Replacement: m.Replacement, Outcome: m.Outcome, Reused: m.Reused, Scope: m.Scope, Excepted: m.Excepted,
			Coverage: m.Coverage, Tests: m.Tests, State: m.State})
	}
	return out
}

// countMutants counts the outcomes of mutants in f, as a file's units count
// them: a timed-out mutant is killed, and an excepted survivor excepted.
func countMutants(f *mutateFile, mutants []mutation.MutantResult) {
	for _, m := range mutants {
		switch {
		case m.Outcome == mutation.Killed || m.Outcome == mutation.Timeout:
			f.Killed++
		case m.Outcome == mutation.Survived && m.Excepted != "":
			f.Excepted++
		case m.Outcome == mutation.Survived:
			f.Survived++
		case m.Outcome == mutation.Uncovered:
			f.Uncovered++
		}
	}
}

// workOf counts mutants by their fail-fast state.
func workOf(mutants []mutation.MutantResult) *mutateWork {
	w := &mutateWork{}
	for _, m := range mutants {
		switch m.State {
		case mutation.StateCompleted:
			w.Completed++
		case mutation.StateCancelled:
			w.Cancelled++
		case mutation.StateUnattempted:
			w.Unattempted++
		case mutation.StateBlocked:
			w.Blocked++
		}
	}
	return w
}

// failFastState is a file's state in a --fail-fast run: "blocked" when its
// baseline or a selection of listed tests it needed failed, "completed"
// when every selected mutant has its outcome, else "stopped" when the stop
// came once its work began, and "unattempted" when it came before.
func failFastState(r mutation.FileResult, w *mutateWork) string {
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
func stopSubject(s *mutation.Stop) map[string]any {
	switch {
	case s.Site != nil:
		return map[string]any{"file": s.File, "line": s.Site.Line, "column": s.Site.Column, "function": s.Function,
			"original": s.Site.Original, "replacement": s.Site.Replacement}
	case s.Exception != nil:
		return staleSubject(s.File, *s.Exception)
	case s.Selection != nil:
		return map[string]any{"ids": s.Selection.IDs, "command": s.Selection.Command, "exit_code": s.Selection.ExitCode}
	case s.Function != "":
		return map[string]any{"file": s.File, "function": s.Function, "line": s.Line}
	}
	return map[string]any{"file": s.File}
}

// describeStop names the stop's failure in plain output.
func describeStop(s *mutation.Stop) string {
	switch {
	case s.Site != nil:
		return fmt.Sprintf("%s:%d:%d %s → %s in %s", s.File, s.Site.Line, s.Site.Column, quote(s.Site.Original), quote(s.Site.Replacement), s.Function)
	case s.Exception != nil && s.Exception.Line != 0:
		return fmt.Sprintf("%s:%d:%d in %s (%s)", s.File, s.Exception.Line, s.Exception.Column, s.Exception.Function, s.Exception.Why)
	case s.Exception != nil:
		return fmt.Sprintf("%s in %s (%s)", s.File, s.Exception.Function, s.Exception.Why)
	case s.Selection != nil:
		return "of the listed tests " + strings.Join(s.Selection.IDs, " ")
	case s.Function != "":
		return fmt.Sprintf("%s:%d in %s", s.File, s.Line, s.Function)
	}
	return "in " + s.File
}

// exceptionsElsewhere is each exception of itos-cc.yaml for a file that is
// none of sources and that the command judges all the same, stale, and the
// entries of those whose function moved to one of sources, as they read
// there (mutation.Elsewhere). With no paths, no --changed, and no --since, it
// judges every entry for a file under the working directory; with --since,
// each whose file the range deleted, unless paths narrow it, or renamed to
// one of sources, renamed giving the path each had at the ref; with paths
// or --changed alone, none. An entry it judges whose file itos-cc.yaml
// excludes is stale as excluded.
func exceptionsElsewhere(ctx context.Context, in *invocation, exceptions []config.Exception, sources []string, renamed func(path string) string) ([]mutation.StaleException, []config.Exception, error) {
	if len(exceptions) == 0 {
		return nil, nil, nil
	}
	root, dir := project.Root(), in.adapters.Dir
	key := func(path string) string { return snapshot.Key(root, sourcePath(dir, path)) }
	var judge func(file string) bool
	switch {
	case in.set("since"):
		removed := map[string]bool{}
		for _, path := range sources {
			if old := renamed(path); old != "" {
				removed[key(old)] = true
			}
		}
		if len(in.args) == 0 {
			if in.adapters.History == nil {
				return nil, nil, fmt.Errorf("mutation history binding is missing")
			}
			deleted, err := in.adapters.History.DeletedSince(ctx, in.str("since"))
			if err != nil {
				return nil, nil, err
			}
			for _, path := range deleted {
				removed[key(path)] = true
			}
		}
		judge = func(file string) bool { return removed[file] }
	case len(in.args) == 0 && !in.set("changed"):
		judge = func(file string) bool {
			rel, err := filepath.Rel(dir, sourcePath(root, file))
			return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
		}
	default:
		return nil, nil, nil
	}
	exclude, err := exclusion()
	if err != nil {
		return nil, nil, err
	}
	return mutation.Elsewhere(ctx, exceptions, sources, judge, func(file string) bool { return exclude.Excludes(sourcePath(root, file)) }, mutationCheckOptions(in, nil, nil, nil, nil))
}

// reportFailedSelections reports each selection of listed tests in failed
// that reported does not hold yet, through the slice's shared presentation.
func reportFailedSelections(in *invocation, failed []mutation.FailedSelection, reported map[string]bool, cmd string) {
	p := presentationFor(in)
	p.FailedSelections(failed, reported, cmd)
	in.problems = append(in.problems, p.Problems...)
}

// loadExceptions is the survivors itos-cc.yaml excepts. A file that cannot
// be read is a config error.
func loadExceptions() ([]config.Exception, error) {
	c, err := loadConfig()
	if err != nil {
		return nil, err
	}
	return c.Exceptions, nil
}

// loadConfig reads itos-cc.yaml. A file that cannot be read is a config
// error.
func loadConfig() (*config.Config, error) {
	c, err := config.Load()
	if err != nil {
		return nil, invalidConfig(err)
	}
	return c, nil
}

// invalidConfig is err, from reading itos-cc.yaml, as a person reads it: a
// file that cannot be read as a config is config.invalid, saying what each
// key needs, and any other error is err.
func invalidConfig(err error) error {
	var invalid *config.InvalidError
	if errors.As(err, &invalid) {
		return configProblem(invalid,
			"Fix it: exclude lists globs of sources from the project root, such as **/porttest/**; each entry under mutation.exceptions needs file, function, hash, line_in_function, column, original, replacement, and reason, and 'itos-cc mutation except' writes one; mutation.tests needs list, run with {pattern}, ids_pattern with {ids}, and join with each, holding {id}, and sep.")
	}
	return err
}

// configProblem is config.invalid for invalid, an itos-cc.yaml that cannot
// be read, with fix: its subject the file, and the key at fault when one
// is.
func configProblem(invalid *config.InvalidError, fix string) *cli.Problem {
	p := cli.NewProblem(cli.CodeUsage, "config.invalid", invalid.Error(), fix).With("file", config.File)
	if invalid.Key != "" {
		p.With("key", invalid.Key)
	}
	return p
}

// mutationInputsBound guards actual command composition. Direct legacy callers
// retain their documented defaults until orchestration moves; handlers do not.
func mutationInputsBound(a adapters) error {
	if a.Toolchain == nil {
		return errors.New("mutation Toolchain binding is missing")
	}
	if a.Fingerprints == nil {
		return errors.New("mutation Fingerprints binding is missing")
	}
	return nil
}

// supportNow is the hashes of the support files of the tests cfg lists,
// now: what a kill by listed tests rests on, with the files of its tests.
func supportNow(ctx context.Context, in *invocation, cfg *config.Config) (map[string]string, error) {
	if cfg.Tests == nil {
		return nil, nil
	}
	if in.adapters.Fingerprints == nil {
		return nil, errors.New("mutation Fingerprints binding is missing")
	}
	return in.adapters.Fingerprints.Support(ctx, project.Root(), cfg.Tests.Support)
}

func flagConflict(flag, message string) error {
	return cli.NewProblem(cli.CodeUsage, "flags.conflict", message, "Drop the incompatible flag and run again.").With("flag", flag)
}

func flagPresent(in *invocation, flag string) bool {
	switch flag {
	case "--coverage-report":
		return len(in.strs("coverage-report")) > 0
	case "--coverage-command":
		return in.set("coverage-command")
	case "--use-existing-coverage":
		return in.set("use-existing-coverage")
	}
	return false
}

// rawCoverage says whether coverage comes from reports the run is given or
// told to read, not from the per-language commands. Every language strict
// coverage proves (mutation.StrictCoverage) refuses them under
// --fail-uncovered, so only a language it does not prove reaches this.
func rawCoverage(in *invocation) bool {
	return in.set("use-existing-coverage") || in.str("coverage-command") != "" || len(in.strs("coverage-report")) > 0
}

// unmeasuredToJudge is, under --fail-uncovered, a problem for each build
// root of a language other than Go that coverage measured nothing of while
// a file of it has mutants this run must judge: the files Run would
// otherwise fall back on, running every mutant. Each fails closed with the
// problem unmeasured gives it, as Go does (ADR-0017). A file of a language
// measured elsewhere in the run is uncovered instead, as before, and a file
// with nothing to judge needs no coverage.
func unmeasuredToJudge(report *coverage.Report, sources []string, checks []mutation.FileCheck, mutateAll bool) []*cli.Problem {
	var found []coverage.Unmeasured
	seen := map[int]bool{}
	for i, source := range sources {
		spec := lang.Detect(source)
		if spec == nil || spec.Name == "go" || i >= len(checks) {
			continue
		}
		if !mutation.NeedsMutationCoverage(checks[i:i+1], mutateAll) || report.Has(source) || report.Measures(spec.Name) {
			continue
		}
		abs, err := filepath.Abs(source)
		if err != nil {
			abs = source
		}
		matched := false
		for j, m := range report.Missing() {
			dir, err := filepath.Abs(m.Dir)
			if m.Language != spec.Name || m.Dir == "" || err != nil {
				continue
			}
			if rel, err := filepath.Rel(dir, abs); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
			matched = true
			if !seen[j] {
				seen[j] = true
				found = append(found, m)
			}
		}
		if !matched {
			found = append(found, coverage.Unmeasured{Dir: filepath.Dir(abs), Language: spec.Name, Cause: covport.MeasuredNothing{},
				Reason: "its coverage run measured none of its files"})
		}
	}
	return unmeasured(found)
}

// filesToJudge is each of sources whose mutants this run must judge.
func filesToJudge(sources []string, checks []mutation.FileCheck, mutateAll bool) []string {
	var out []string
	for i, source := range sources {
		if i < len(checks) && mutation.NeedsMutationCoverage(checks[i:i+1], mutateAll) {
			out = append(out, source)
		}
	}
	return out
}

// integrationToJudge is, under --fail-uncovered, coverage.tool-missing for
// each build root whose collector could not measure the processes its
// tests start, such as a coverage.py that does not start in them, while a
// file of its language under it has mutants to judge (files): its lines
// such a process runs would be judged uncovered. Without --fail-uncovered
// the run goes on without integration coverage, as coverage logged.
func integrationToJudge(report *coverage.Report, files []string) []*cli.Problem {
	var found []coverage.Unmeasured
	for _, m := range report.IntegrationMissing() {
		dir, err := filepath.Abs(m.Dir)
		if err != nil {
			continue
		}
		for _, file := range files {
			spec := lang.Detect(file)
			abs, err := filepath.Abs(file)
			if spec == nil || spec.Name != m.Language || err != nil {
				continue
			}
			if rel, err := filepath.Rel(dir, abs); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				found = append(found, m)
				break
			}
		}
	}
	return unmeasured(found)
}

func hasGoSource(sources []string) bool {
	for _, source := range sources {
		if spec := lang.Detect(source); spec != nil && spec.Name == "go" {
			return true
		}
	}
	return false
}

// strictCoverageInputs is, by each of languages, the producer and current
// input fingerprint of a source's strict coverage evidence, measured with
// the tests that reach it, or with allTests the whole suite. With
// testCommand, Python's is the whole suite's too: its coverage plans then
// run every test, as for --all-tests, while Go's own tests measure Go.
func strictCoverageInputs(in *invocation, languages []string, allTests, testCommand bool, support map[string]string) map[string]coverageInputs {
	out := map[string]coverageInputs{}
	for _, language := range languages {
		out[language] = func(ctx context.Context, source string) (string, map[string]string, error) {
			return mutation.CoverageInputsFor(ctx, in.adapters.Toolchain, in.adapters.Fingerprints, source, project.Root(), allTests, testCommand, support)
		}
	}
	return out
}

// strictCoverage measures sources for strict coverage, with the built-in
// commands of each language, the listed tests' coverage beside it, and
// lists the executable lines of each file of a line-precision language no
// test loaded without running a test (coverage.Report.Inventory). It
// returns the report, Go's producer, the producer and fingerprint of each
// source's evidence by its language, and the file of each listed test.
func strictCoverage(ctx context.Context, in *invocation, languages []string, sources []string, suite *config.Tests, support map[string]string, reach covrun.Reach) (*coverage.Report, string, coverageInputs, map[string]string, error) {
	perTest := (*coverage.PerTest)(nil)
	listedFiles := map[string]string{}
	if suite != nil {
		listed, err := listTests(ctx, in, suite)
		if err != nil {
			return nil, "", nil, nil, err
		}
		perTest = &coverage.PerTest{Root: project.Root(), Tests: listed, Select: suite.Select}
		var ids []string
		for _, test := range listed {
			ids = append(ids, test.ID)
			listedFiles[test.ID] = test.File
		}
		perTest.All = suite.All(ids)
	}
	scope := covrun.OwnTests
	if in.set("all-tests") {
		scope = covrun.AllTests
	}
	producer := mutation.CoverageProducer("go", in.set("all-tests"))
	report, err := loadCoverage(ctx, in, sources, scope, perTest, reach)
	if err != nil {
		return nil, "", nil, nil, err
	}
	for _, language := range languages {
		// A command that fails leaves its files without proof, which the
		// run reports as missing evidence; it logged why.
		if in.adapters.MutationCoverage == nil {
			return nil, "", nil, nil, fmt.Errorf("mutation coverage capability is not bound")
		}
		if failure := in.adapters.MutationCoverage.Inventory(ctx, report, language); failure != nil && ctx.Err() != nil {
			return nil, "", nil, nil, context.Cause(ctx)
		}
	}
	// A stopped run leaves files unlisted for that reason, not for a
	// command's failure: it is reported as the stop.
	if ctx.Err() != nil {
		return nil, "", nil, nil, context.Cause(ctx)
	}
	inputs := strictCoverageInputs(in, languages, in.set("all-tests"), in.set("test-command"), support)
	fingerprint := func(ctx context.Context, source string) (string, map[string]string, error) {
		if spec := lang.Detect(source); spec != nil && inputs[spec.Name] != nil {
			return inputs[spec.Name](ctx, source)
		}
		return "", nil, fmt.Errorf("%s: no strict coverage inputs for its language", source)
	}
	return report, producer, fingerprint, listedFiles, nil
}

// reportStrictCoverage reports, for every judged function of a file of a
// strict language a run decided, a function without complete evidence
// and each uncovered executable block or line; with scope, of those only a
// function with a line scope judges, and a block or line it judges.
func reportStrictCoverage(in *invocation, results []mutation.FileResult, scope mutation.LineScope) {
	for _, result := range results {
		language := result.Snapshot.Language
		if !mutation.StrictCoverage(language) {
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
				reportLanguageCoverageProblem(in, language, result.Rel, id, unit.StartLine, "mutation.coverage-missing", "has no complete measured "+coverageInventory(language))
				continue
			}
			for _, block := range unit.Coverage.Blocks {
				if !block.Covered && scope.JudgesBlock(result.Rel, block) {
					reportLanguageCoverageProblem(in, language, result.Rel, id, block.Line, "mutation.uncovered-statement", "has an uncovered "+executableUnit(language))
				}
			}
		}
	}
}

func reportCoverageProblem(in *invocation, file, function string, line int, rule, message string) {
	reportLanguageCoverageProblem(in, "go", file, function, line, rule, message)
}

// listedConfig is the tests itos-cc.yaml lists, when the mutants of this run
// are to run them: coverage is measured by running the per-language
// commands, and the mutants run their own tests, not --all-tests or a
// --test-command. Otherwise nil.
func listedConfig(in *invocation, cfg *config.Config) *config.Tests {
	if cfg.Tests == nil || in.set("no-coverage") || in.set("all-tests") || in.str("test-command") != "" ||
		in.set("use-existing-coverage") || in.str("coverage-command") != "" || len(in.strs("coverage-report")) > 0 {
		return nil
	}
	return cfg.Tests
}

// listTests runs the list command of tests at the project root, through the
// platform shell, and reads the tests it prints: one a line, its ID, then,
// after a tab, optionally its file. A command that fails is
// tests.list-failed.
func listTests(ctx context.Context, in *invocation, tests *config.Tests) ([]covrun.Test, error) {
	root := project.Root()
	command := mutation.Command{Root: root, Dir: root, Shell: tests.List}
	RunProgress{Log: os.Stderr}.Report(mutation.RunEvent{Kind: "listing", Command: command})
	listed, err := mutation.ListedTests(ctx, in.adapters.TestLister, command)
	return listed, mutationRunError(err)
}

// mutationRunError words the domain's run failures through the slice,
// which sample shares, until run moves.
func mutationRunError(err error) error { return mutationslice.RunError(err) }

func mutationPath(dir, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(dir, path)
}

// changedSince is the functions the commits since --since's ref changed, by
// file. --changed is refused beside it: it judges whole files of the working
// tree, --since functions of commits, and together they would judge neither.
func changedSince(ctx context.Context, in *invocation) (map[string]mutation.ChangedFunctions, map[string]string, error) {
	if in.set("changed") {
		return nil, nil, cli.NewProblem(cli.CodeUsage, "flags.conflict", "--since and --changed cannot be combined: --since judges the functions of commits, --changed whole files of the working tree",
			"Drop --changed; commit the work to judge it with --since.").With("flag", "--changed")
	}
	ref := in.str("since")
	if in.adapters.History == nil {
		return nil, nil, fmt.Errorf("mutation history binding is missing")
	}
	changed, renamed, failure := in.adapters.History.ChangedSince(ctx, ref)
	if failure == nil {
		return changed, renamed, nil
	}
	switch failure := failure.(type) {
	case mutation.BadReference:
		return nil, nil, cli.NewProblem(cli.CodeUsage, "since.bad-ref", fmt.Sprintf("--since %s: not a commit in this repository", ref),
			"Name a branch, tag, or commit, such as origin/main; fetch a remote one first.").With("ref", ref)
	case mutation.NoRepository:
		return nil, nil, cli.NewProblem(cli.CodeEnvironment, "since.no-git", "--since needs a git repository: "+failure.Reason,
			"Run it inside a git repository, or name the paths instead.")
	case mutation.HistoryCancelled:
		return nil, nil, failure.Cause
	case mutation.HistoryFailed:
		return nil, nil, failure.Cause
	}
	return nil, nil, failure
}

func sourcePath(dir, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(dir, path)
}
