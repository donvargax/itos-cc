package mutation

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
	"github.com/donvargax/itos-cc/internal/lang"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/snapshot"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// strictCoverage measures sources for strict coverage, with the built-in
// commands of each language, the listed tests' coverage beside it, served
// from the cache when s's reuse serves it, and lists the executable lines
// of each file of a line-precision language no test loaded without running
// a test (coverage.Report.Inventory). The listed tests are listing's, the
// run's listing of s's suite. It returns the report, Go's producer and the
// file of each listed test.
func (h Handler) strictCoverage(ctx context.Context, q RunQuery, s strictCoverage, sources []string, listing *mutants.ListOnce) (*coverage.Report, string, map[string]string, error) {
	root := s.root
	perTest := (*coverage.PerTest)(nil)
	var key *mutants.ListedKey
	listedFiles := map[string]string{}
	if suite := s.suite; suite != nil {
		if listing == nil {
			listing = mutants.NewListOnce(h.TestLister, root, suite.List)
		}
		inv, err := listing.List(ctx, q.Progress)
		if err != nil {
			return nil, "", nil, ranInto(err)
		}
		perTest = &coverage.PerTest{Root: root, Tests: inv.Tests, Select: suite.Select}
		var ids []string
		for _, test := range inv.Tests {
			ids = append(ids, test.ID)
			listedFiles[test.ID] = test.File
		}
		perTest.All = suite.All(ids)
		if s.reuse != nil {
			key = mutants.KeyListed(ctx, h.Toolchain, h.Fingerprints, root, sources, inv, s.support)
		}
	}
	scope := covport.OwnTests
	if q.AllTests {
		scope = covport.AllTests
	}
	producer := mutants.CoverageProducer("go", q.AllTests)
	report, err := h.loadCoverage(ctx, q, sources, scope, perTest, s.reach, s.reuse, key)
	if err != nil {
		return nil, "", nil, err
	}
	for _, language := range s.languages {
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
// folder, or serves it from the cache when reuse serves key
// (mutants.MeasureReusing). The binding owns progress and test output.
// reach, in OwnTests, is the test files that reach each source, which
// measure Python's and Kotlin's; nil measures them by the whole suite. A
// stopped run is reported as ctx's cause, as mutation reports a stop, and
// listed tests whose coverage run failed as tests.coverage-failed
// (RunError). Its one caller, strict coverage, has refused every raw
// coverage flag before it (StrictCoverageFlag), --coverage-command among
// them.
func (h Handler) loadCoverage(ctx context.Context, q RunQuery, sources []string, scope covport.Scope, perTest *coverage.PerTest, reach covport.Reach, reuse *mutants.ListedReuse, key *mutants.ListedKey) (*coverage.Report, error) {
	if q.NoCoverage {
		return nil, nil
	}
	req := covport.Request{Sources: sources, Reports: reports(q.CoverageReports), Command: given(q.CoverageCommand), Existing: q.UseExistingCoverage,
		AllTests: q.AllTests, Scope: scope, Reach: reach}
	if h.Coverage == nil {
		return nil, fmt.Errorf("mutation coverage capability is not bound")
	}
	report, failure := mutants.MeasureReusing(ctx, h.Coverage, req, perTest, reuse, key)
	if failure != nil {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		if failed, ok := mutants.MeasureFailure(failure).(mutants.CoverageRunFailed); ok {
			return nil, ranInto(failed)
		}
		return nil, failure
	}
	return report, nil
}

// unmeasuredToJudge is, under --fail-uncovered, each build root of a
// language other than Go that coverage measured nothing of while a file of
// it has mutants this run must judge: the files Run would otherwise fall
// back on, running every mutant. Each fails closed, as Go does (ADR-0017).
// A file of a language measured elsewhere in the run is uncovered instead,
// as before, and a file with nothing to judge needs no coverage. A build
// root whose coverage command failed after writing its report
// (coverage.Report.Failed) measured nothing either, whatever the report
// names: a failed command proves nothing.
func (h Handler) unmeasuredToJudge(report *coverage.Report, sources []string, checks []mutants.FileCheck, mutateAll bool) []coverage.Unmeasured {
	var found []coverage.Unmeasured
	seen := map[string]bool{}
	// under adds each of gaps of spec's language whose build root holds
	// abs, and says whether one does.
	under := func(gaps []coverage.Unmeasured, kind, language, abs string) bool {
		matched := false
		for j, m := range gaps {
			dir, ok := absIn(h.Dir, m.Dir)
			if m.Language != language || m.Dir == "" || !ok {
				continue
			}
			if rel, err := filepath.Rel(dir, abs); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
			matched = true
			if key := kind + strconv.Itoa(j); !seen[key] {
				seen[key] = true
				found = append(found, m)
			}
		}
		return matched
	}
	for i, source := range sources {
		spec := lang.Detect(source)
		if spec == nil || spec.Name == "go" || i >= len(checks) || !mutants.NeedsMutationCoverage(checks[i:i+1], mutateAll) {
			continue
		}
		abs, ok := absIn(h.Dir, source)
		if !ok {
			abs = source
		}
		if under(report.Failed(), "failed", spec.Name, abs) || report.Has(source) || report.Measures(spec.Name) {
			continue
		}
		matched := under(report.Missing(), "missing", spec.Name, abs)
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
