package mutation

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/internal/snapshot"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// strictCoverage measures sources for strict coverage, with the built-in
// commands of each language, the listed tests' coverage beside it, and
// lists the executable lines of each file of a line-precision language no
// test loaded without running a test (coverage.Report.Inventory). It
// returns the report, Go's producer and the file of each listed test.
func (h Handler) strictCoverage(ctx context.Context, q RunQuery, root string, sources, languages []string, suite *mutants.Tests, reach covport.Reach) (*coverage.Report, string, map[string]string, error) {
	perTest := (*coverage.PerTest)(nil)
	listedFiles := map[string]string{}
	if suite != nil {
		listed, err := h.listTests(ctx, q, root, suite)
		if err != nil {
			return nil, "", nil, err
		}
		perTest = &coverage.PerTest{Root: root, Tests: listed, Select: suite.Select}
		var ids []string
		for _, test := range listed {
			ids = append(ids, test.ID)
			listedFiles[test.ID] = test.File
		}
		perTest.All = suite.All(ids)
	}
	scope := covport.OwnTests
	if q.AllTests {
		scope = covport.AllTests
	}
	producer := mutants.CoverageProducer("go", q.AllTests)
	report, err := h.loadCoverage(ctx, q, sources, scope, perTest, reach)
	if err != nil {
		return nil, "", nil, err
	}
	for _, language := range languages {
		// A command that fails leaves its files without proof, which the
		// run reports as missing evidence; it logged why.
		if h.Coverage == nil {
			return nil, "", nil, fmt.Errorf("mutation coverage capability is not bound")
		}
		if failure := h.Coverage.Inventory(ctx, report, language); failure != nil && ctx.Err() != nil {
			return nil, "", nil, context.Cause(ctx)
		}
	}
	// A stopped run leaves files unlisted for that reason, not for a
	// command's failure: it is reported as the stop.
	if ctx.Err() != nil {
		return nil, "", nil, context.Cause(ctx)
	}
	return report, producer, listedFiles, nil
}

// loadCoverage produces coverage for sources under ctx, or nil with
// --no-coverage, through the mutation coverage binding. scope is the tests
// that run without --all-tests. With perTest, when the coverage commands
// run, it also measures each listed test's coverage, in the run's own
// folder. The binding owns progress and test output. reach, in OwnTests, is
// the test files that reach each source, which measure Python's and
// Kotlin's; nil measures them by the whole suite. A stopped run is
// reported as ctx's cause, as mutation reports a stop. --coverage-command
// without --coverage-report, which says where its report lands, is refused.
func (h Handler) loadCoverage(ctx context.Context, q RunQuery, sources []string, scope covport.Scope, perTest *coverage.PerTest, reach covport.Reach) (*coverage.Report, error) {
	command := given(q.CoverageCommand)
	switch {
	case q.NoCoverage:
		return nil, nil
	case command != "" && len(q.CoverageReports) == 0:
		return nil, ranInto(mutants.CoverageReportRequired{})
	}
	req := covport.Request{Sources: sources, Reports: reports(q.CoverageReports), Command: command, Existing: q.UseExistingCoverage,
		AllTests: q.AllTests, Scope: scope, Reach: reach}
	if h.Coverage == nil {
		return nil, fmt.Errorf("mutation coverage capability is not bound")
	}
	report, failure := h.Coverage.MeasureListed(ctx, req, perTest)
	if failure != nil {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		return nil, failure
	}
	return report, nil
}

// listTests runs the list command of tests at the project root, through the
// TestLister, and reads the tests it prints: one a line, its ID, then,
// after a tab, optionally its file. A command that fails is
// tests.list-failed.
func (h Handler) listTests(ctx context.Context, q RunQuery, root string, tests *mutants.Tests) ([]coverage.Test, error) {
	command := mutants.Command{Root: root, Dir: root, Shell: tests.List}
	if q.Progress != nil {
		q.Progress.Report(mutants.RunEvent{Kind: "listing", Command: command})
	}
	listed, err := mutants.ListedTests(ctx, h.TestLister, command)
	return listed, ranInto(err)
}

// unmeasuredToJudge is, under --fail-uncovered, each build root of a
// language other than Go that coverage measured nothing of while a file of
// it has mutants this run must judge: the files Run would otherwise fall
// back on, running every mutant. Each fails closed, as Go does (ADR-0017).
// A file of a language measured elsewhere in the run is uncovered instead,
// as before, and a file with nothing to judge needs no coverage.
func (h Handler) unmeasuredToJudge(report *coverage.Report, sources []string, checks []mutants.FileCheck, mutateAll bool) []coverage.Unmeasured {
	var found []coverage.Unmeasured
	seen := map[int]bool{}
	for i, source := range sources {
		spec := lang.Detect(source)
		if spec == nil || spec.Name == "go" || i >= len(checks) {
			continue
		}
		if !mutants.NeedsMutationCoverage(checks[i:i+1], mutateAll) || report.Has(source) || report.Measures(spec.Name) {
			continue
		}
		abs, ok := absIn(h.Dir, source)
		if !ok {
			abs = source
		}
		matched := false
		for j, m := range report.Missing() {
			dir, ok := absIn(h.Dir, m.Dir)
			if m.Language != spec.Name || m.Dir == "" || !ok {
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
	return found
}

// filesToJudge is each of sources whose mutants this run must judge.
func filesToJudge(sources []string, checks []mutants.FileCheck, mutateAll bool) []string {
	var out []string
	for i, source := range sources {
		if i < len(checks) && mutants.NeedsMutationCoverage(checks[i:i+1], mutateAll) {
			out = append(out, source)
		}
	}
	return out
}

// integrationToJudge is, under --fail-uncovered, each build root whose
// collector could not measure the processes its tests start, such as a
// coverage.py that does not start in them, while a file of its language
// under it has mutants to judge (files): its lines such a process runs
// would be judged uncovered. Without --fail-uncovered the run goes on
// without integration coverage, as coverage logged.
func (h Handler) integrationToJudge(report *coverage.Report, files []string) []coverage.Unmeasured {
	var found []coverage.Unmeasured
	for _, m := range report.IntegrationMissing() {
		dir, ok := absIn(h.Dir, m.Dir)
		if !ok {
			continue
		}
		for _, file := range files {
			spec := lang.Detect(file)
			abs, ok := absIn(h.Dir, file)
			if spec == nil || spec.Name != m.Language || !ok {
				continue
			}
			if rel, err := filepath.Rel(dir, abs); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				found = append(found, m)
				break
			}
		}
	}
	return found
}

// removeOrphans removes the orphans the selection's paths cover (orphans)
// from Metrics, and returns them.
func (h Handler) removeOrphans(ctx context.Context, q SelectionQuery) ([]mutants.Orphan, error) {
	found, err := h.orphans(ctx, q)
	if err != nil {
		return nil, err
	}
	if len(found) > 0 && h.Metrics == nil {
		return nil, errors.New("mutation snapshot store binding is missing")
	}
	if err := RemoveOrphans(ctx, h.Metrics, found); err != nil {
		return nil, err
	}
	return found, nil
}

// RemoveOrphans removes the snapshot of each orphan from store, and each
// folder under .metrics/mutate that removing them left empty (store's
// Remove). It removes nothing outside .metrics/mutate, whatever an Orphan
// names.
func RemoveOrphans(ctx context.Context, store snapport.Store, orphans []mutants.Orphan) error {
	for _, o := range orphans {
		name, ok := strings.CutPrefix(o.Snapshot, snapshot.Folder+"/mutate/")
		if !ok || !filepath.IsLocal(filepath.FromSlash(name)) {
			continue
		}
		if failure := store.Remove(ctx, path.Join("mutate", name)); failure != nil {
			return failure
		}
	}
	return nil
}
