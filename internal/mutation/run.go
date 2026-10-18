package mutation

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/mutants"
)

// RunQuery carries mutation run's domain values once: the selection, how
// uncovered mutants judge, whether to stop at the first actionable failure,
// how coverage is measured (CoverageCommand and TestCommand nil when
// omitted, so an empty one given differs from none), the run's limits and
// whether it annotates. Progress is where its observations go as they
// happen.
type RunQuery struct {
	SelectionQuery
	FailUncovered, LinesOnly bool
	FailFast                 bool
	NoCoverage               bool
	UseExistingCoverage      bool
	AllTests                 bool
	CoverageCommand          *string
	CoverageReports          []string
	TestCommand              *string
	Workers                  int
	TimeoutFactor            float64
	MutateAll                bool
	Annotate                 bool
	Progress                 SelectionProgress
}

// RunFound is what run observed up to where it stopped: the settings' root,
// whether the selection held no source, the strict coverage gaps that
// stopped it before any mutant, and once it ran, each file's result and the
// fail-fast stop; the exceptions of files it did not select that it judged
// stale, the orphans it removed and the lines --fail-uncovered=lines judges.
type RunFound struct {
	Root      string
	NoSources bool
	Strict    bool
	Gaps      *CoverageGaps
	Ran       bool
	Results   []mutants.FileResult
	Stop      *mutants.Stop
	Elsewhere []mutants.StaleException
	Orphans   []mutants.Orphan
	Lines     mutants.LineScope
	// Interrupted is the cause of the run's context once it stopped while
	// it judged, as an interrupt stops it: Results holds what it kept.
	Interrupted error
}

// CoverageGaps is why strict coverage stopped a run before any mutant: the
// coverage checks of an unsupported scope, the Go build roots coverage
// measured nothing of, and GoNoReport when Go measured nothing with no root
// to name, then the other languages' roots it measured nothing of while a
// file of theirs has mutants to judge.
type CoverageGaps struct {
	Unsupported []mutants.CoverageCheck
	Go          []coverage.Unmeasured
	GoNoReport  bool
	Unmeasured  []coverage.Unmeasured
}

// Run observes in the cmd handler's order: settings, the selection (the
// --since refusal and range, then exclude), the exceptions of files it does
// not select, then with no source the orphans it removes; otherwise strict
// coverage's flag refusal, the tests that reach each source, the listed
// tests' support files and the bindings, then strict coverage's checks and
// measurement, the run, and the orphans it removes. Settings are read at
// run's own admission, at the selection's exclude, at the exceptions'
// exclude and at the orphans' exclude. A refusal is a Failure the UI words.
func (h Handler) Run(ctx context.Context, q RunQuery) (RunFound, error) {
	found := RunFound{}
	root, cfg, err := h.settings(ctx)
	if err != nil {
		return found, err
	}
	found.Root = root
	sources, judge, renamed, lines, excluded, err := h.selectSources(ctx, q.SelectionQuery)
	if err != nil {
		return found, err
	}
	if q.Progress != nil {
		q.Progress.Excluded(excluded)
	}
	// With --fail-uncovered=lines, strict coverage and uncovered mutants
	// are judged on the lines the range changed alone; evidence is still
	// measured and recorded for whole functions.
	var scope mutants.LineScope
	if q.LinesOnly {
		scope = lines
	}
	found.Lines = scope
	elsewhere, moved, err := h.elsewhere(ctx, q.SelectionQuery, root, cfg.Exceptions, sources, renamed)
	if err != nil {
		return found, err
	}
	found.Elsewhere = elsewhere
	if len(sources) == 0 {
		found.NoSources = true
		found.Orphans, err = h.removeOrphans(ctx, q.SelectionQuery)
		return found, err
	}
	// Strict coverage proves the functions of each strict language of the
	// selection (mutants.StrictCoverage) executed.
	var strictLanguages []string
	if q.FailUncovered {
		strictLanguages = StrictLanguages(sources)
	}
	strictGo := slices.Contains(strictLanguages, "go")
	strict := len(strictLanguages) > 0
	found.Strict = strict
	if strict {
		for _, flag := range []struct {
			name  string
			given bool
		}{{"--coverage-report", len(q.CoverageReports) > 0}, {"--use-existing-coverage", q.UseExistingCoverage}, {"--coverage-command", q.CoverageCommand != nil}} {
			if flag.given {
				return found, StrictCoverageFlag{Flag: flag.name, Language: strictLanguages[0]}
			}
		}
	}
	if h.Tree == nil || h.Surroundings == nil {
		return found, errors.New("mutation source tree binding is missing")
	}
	tests, err := h.importingTests(ctx, root)
	if err != nil {
		return found, err
	}
	var support map[string]string
	if cfg.Tests != nil {
		if h.Fingerprints == nil {
			return found, errors.New("mutation Fingerprints binding is missing")
		}
		var failure mutants.InputFailure
		if support, failure = h.Fingerprints.Support(ctx, root, cfg.Tests.Support); failure != nil {
			return found, failure
		}
	}
	// Python's and Kotlin's own coverage is that of the tests that reach
	// each file, as its own command runs them; --test-command and
	// --all-tests keep the whole suite's.
	var reach covport.Reach
	if q.TestCommand == nil && !q.AllTests {
		reach = tests
	}
	if err := h.runBound(q); err != nil {
		return found, err
	}
	opt := mutants.Options{
		Store: h.Snapshots, Source: h.SourceEdits, Tree: h.Tree, Surroundings: h.Surroundings,
		Root: root, Dir: h.Dir,
		WorkerFactory: h.Workers,
		Toolchain:     h.Toolchain,
		Fingerprints:  h.Fingerprints,
		Support:       support,
		Tests:         tests,
		Workers:       q.Workers,
		MutateAll:     q.MutateAll,
		TimeoutFactor: q.TimeoutFactor,
		TestCommand:   given(q.TestCommand),
		AllTests:      q.AllTests,
		Annotate:      q.Annotate,
		Progress:      q.Progress,
		Judge:         judge,
		Renamed:       renamed,
		// An entry whose function moved to another file still excepts
		// its mutant there.
		Exceptions: append(slices.Clone(cfg.Exceptions), moved...),
	}
	suite := listedSuite(q, cfg)
	if suite != nil {
		opt.Listed = &mutants.Listed{Root: root, List: suite.List, All: suite.All, Select: suite.Select}
	}
	exceptions := append(slices.Clone(cfg.Exceptions), moved...)
	if strict {
		gaps, err := h.strict(ctx, q, &opt, strictCoverage{root: root, languages: strictLanguages, strictGo: strictGo, sources: sources, judge: judge,
			tests: tests, support: support, exceptions: exceptions, scope: scope, suite: suite, reach: reach})
		if gaps != nil || err != nil {
			found.Gaps = gaps
			return found, err
		}
	} else if !q.NoCoverage {
		opt.Measurement = &mutants.RunMeasurement{Measurer: h.Coverage, Lister: h.TestLister,
			Request: covport.Request{Reports: reports(q.CoverageReports), Command: given(q.CoverageCommand), Existing: q.UseExistingCoverage, AllTests: q.AllTests, Scope: covport.OwnTests, Reach: reach}}
		if q.FailUncovered && !rawCoverage(q) {
			// Strict: a language the per-language commands measured
			// nothing of fails closed, as Go does, instead of running
			// every mutant, and stops the run before any mutant runs.
			checks, err := mutants.Check(ctx, sources, h.options(root, judge, tests, support, exceptions))
			if err != nil {
				return found, ranInto(err)
			}
			if mutants.NeedsMutationCoverage(checks, q.MutateAll) {
				report, err := opt.Measure(ctx, sources, true)
				if err != nil {
					return found, ranInto(err)
				}
				others := h.unmeasuredToJudge(report, sources, checks, q.MutateAll)
				others = append(others, h.integrationToJudge(report, filesToJudge(sources, checks, q.MutateAll))...)
				if len(others) > 0 {
					found.Gaps = &CoverageGaps{Unmeasured: others}
					return found, nil
				}
				opt.Coverage = report
			}
		}
	}
	if q.FailFast {
		ff := mutants.FailFast{FailUncovered: q.FailUncovered, Lines: scope}
		if len(elsewhere) > 0 {
			// Known before anything runs: an entry of a file the run does
			// not select that no longer holds.
			e := elsewhere[0]
			at := filepath.FromSlash(e.File)
			if !filepath.IsAbs(at) {
				at = filepath.Join(root, at)
			}
			ff.Known = &mutants.Stop{Rule: "mutation.exception-stale", File: relTo(h.Dir, at), Exception: &e}
		}
		found.Results, found.Stop, err = mutants.RunFailFast(ctx, sources, opt, ff)
	} else {
		found.Results, err = mutants.Run(ctx, sources, opt)
	}
	var stopped mutants.Stopped
	if errors.As(err, &stopped) {
		// It kept what it finished; no orphan is judged of a run cut short.
		found.Ran, found.Interrupted = true, stopped.Cause
		return found, nil
	}
	if err != nil {
		found.Results, found.Stop = nil, nil
		return found, ranInto(err)
	}
	found.Ran = true
	// Only once the run has written what it judged: a --since run moves a
	// renamed file's snapshot first.
	found.Orphans, err = h.removeOrphans(ctx, q.SelectionQuery)
	return found, ranInto(err)
}

// runBound refuses a run whose bindings are missing, in the cmd handler's
// order.
func (h Handler) runBound(q RunQuery) error {
	switch {
	case h.Toolchain == nil:
		return errors.New("mutation Toolchain binding is missing")
	case h.Fingerprints == nil:
		return errors.New("mutation Fingerprints binding is missing")
	case h.Workers == nil:
		return errors.New("mutation Workers binding is missing")
	case h.Snapshots == nil:
		return errors.New("mutation snapshots binding is missing")
	case h.SourceEdits == nil:
		return errors.New("mutation source edits binding is missing")
	case !q.NoCoverage && h.Coverage == nil:
		return errors.New("mutation coverage capability is not bound")
	}
	return nil
}

// strictCoverage is what strict coverage judges a run's sources with.
type strictCoverage struct {
	root       string
	languages  []string
	strictGo   bool
	sources    []string
	judge      func(string, string, string) bool
	tests      func(string) []string
	support    map[string]string
	exceptions []mutants.Exception
	scope      mutants.LineScope
	suite      *mutants.Tests
	reach      covport.Reach
}

// strict sets opt's coverage for strict coverage of s's languages: the
// cached evidence when every judged function has it fresh and every mutant
// holds, else a fresh measurement of the languages that need one. It
// returns the gaps that stop the run before any mutant.
func (h Handler) strict(ctx context.Context, q RunQuery, opt *mutants.Options, s strictCoverage) (*CoverageGaps, error) {
	mutationChecks, err := mutants.Check(ctx, s.sources, h.options(s.root, s.judge, s.tests, s.support, s.exceptions))
	if err != nil {
		return nil, ranInto(err)
	}
	enabled := map[string]bool{}
	for _, language := range s.languages {
		enabled[language] = true
	}
	cached, err := mutants.CheckCoverage(ctx, s.sources, h.options(s.root, s.judge, nil, s.support, nil), enabled, false, false)
	if err != nil {
		return nil, ranInto(err)
	}
	var unsupported []mutants.CoverageCheck
	for _, check := range cached {
		if check.State == "unsupported" && s.scope.Judges(check.File, check.Line, check.EndLine) {
			unsupported = append(unsupported, check)
		}
	}
	if len(unsupported) > 0 {
		return &CoverageGaps{Unsupported: unsupported}, nil
	}
	canReuse := !mutants.NeedsMutationCoverage(mutationChecks, q.MutateAll)
	allFresh := canReuse
	for _, check := range cached {
		if check.State != "fresh" {
			allFresh = false
		}
	}
	if allFresh {
		h.reuseCoverage(opt, s, cached)
		return nil, nil
	}
	// A strict language whose every judged function has fresh evidence and
	// whose mutants all hold is not measured again: its cached evidence
	// stands, as in a run with nothing to measure. Every other language is
	// measured, as before.
	measure := map[string]bool{}
	for _, check := range cached {
		if check.State != "fresh" {
			measure[check.Language] = true
		}
	}
	for i, source := range s.sources {
		spec := lang.Detect(source)
		if spec != nil && (!mutants.StrictCoverage(spec.Name) ||
			i < len(mutationChecks) && mutants.NeedsMutationCoverage(mutationChecks[i:i+1], q.MutateAll)) {
			measure[spec.Name] = true
		}
	}
	var measured []string
	for _, source := range s.sources {
		if spec := lang.Detect(source); spec == nil || measure[spec.Name] {
			measured = append(measured, source)
		}
	}
	kept := map[string]*mutants.CoverageEvidence{}
	for _, check := range cached {
		if !measure[check.Language] {
			kept[check.File+"\x00"+check.Function] = check.Evidence
		}
	}
	if len(kept) > 0 {
		dir := h.Dir
		opt.CachedCoverage = func(path, function, _ string) *mutants.CoverageEvidence {
			return kept[relTo(dir, path)+"\x00"+function]
		}
	}
	report, measuredProducer, listedFiles, err := h.strictCoverage(ctx, q, s.root, measured, s.languages, s.suite, s.reach)
	if err != nil {
		return nil, err
	}
	if report == nil {
		return nil, fmt.Errorf("strict coverage measurement produced no report")
	}
	var missingGo bool
	var goMissing []coverage.Unmeasured
	for _, missing := range report.Missing() {
		missingGo = missingGo || missing.Language == "go"
		if missing.Language == "go" {
			goMissing = append(goMissing, missing)
		}
	}
	// Another language that measured nothing fails closed as Go does, and
	// stops the run with it.
	others := h.unmeasuredToJudge(report, s.sources, mutationChecks, q.MutateAll)
	others = append(others, h.integrationToJudge(report, filesToJudge(s.sources, mutationChecks, q.MutateAll))...)
	if s.strictGo && measure["go"] && (missingGo || !report.Measures("go")) {
		return &CoverageGaps{Go: goMissing, GoNoReport: !missingGo, Unmeasured: others}, nil
	}
	if len(others) > 0 {
		return &CoverageGaps{Unmeasured: others}, nil
	}
	opt.StatementCoverage = report
	opt.CoverageProducer = measuredProducer
	opt.RecordCoverageInputs = true
	opt.Coverage = report
	if opt.Listed != nil {
		opt.Listed.Files = listedFiles
	}
	return nil, nil
}

// reuseCoverage sets opt's coverage to the cached strict evidence of each
// judged function, every one of them fresh.
func (h Handler) reuseCoverage(opt *mutants.Options, s strictCoverage, cached []mutants.CoverageCheck) {
	byFunction := map[string]*mutants.CoverageEvidence{}
	byFile := map[string][]coverage.GoBlock{}
	byLines := map[string]map[string][]coverage.Line{}
	for _, check := range cached {
		byFunction[check.File+"\x00"+check.Function] = check.Evidence
		path := filepath.Join(s.root, filepath.FromSlash(check.File))
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
	dir := h.Dir
	opt.CachedCoverage = func(path, function, _ string) *mutants.CoverageEvidence {
		return byFunction[relTo(dir, path)+"\x00"+function]
	}
	var report *coverage.Report
	if s.strictGo {
		report = coverage.FromGoBlocks(byFile)
	}
	for _, language := range s.languages {
		if language != "go" {
			report = coverage.Merge(report, coverage.FromLines(language, byLines[language]))
		}
	}
	opt.Coverage = report
}

// given is the value of a flag given, or "" when it is not.
func given(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// reports is the coverage reports given, nil when none is.
func reports(given []string) []string {
	if len(given) == 0 {
		return nil
	}
	return given
}

// listedSuite is the tests itos-cc.yaml lists, when the mutants of this run
// are to run them: coverage is measured by running the per-language
// commands, and the mutants run their own tests, not --all-tests or a
// --test-command. Otherwise nil.
func listedSuite(q RunQuery, cfg *mutants.Config) *mutants.Tests {
	if cfg.Tests == nil || q.NoCoverage || q.AllTests || given(q.TestCommand) != "" ||
		q.UseExistingCoverage || given(q.CoverageCommand) != "" || len(q.CoverageReports) > 0 {
		return nil
	}
	return cfg.Tests
}

// rawCoverage says whether coverage comes from reports the run is given or
// told to read, not from the per-language commands. Every language strict
// coverage proves (mutants.StrictCoverage) refuses them under
// --fail-uncovered, so only a language it does not prove reaches this.
func rawCoverage(q RunQuery) bool {
	return q.UseExistingCoverage || given(q.CoverageCommand) != "" || len(q.CoverageReports) > 0
}
