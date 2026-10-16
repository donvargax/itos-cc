package mutate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/internal/coverage"
	langport "github.com/donvargax/itos-cc/internal/lang/port"
	"github.com/donvargax/itos-cc/internal/metricsdir"
	"github.com/donvargax/itos-cc/internal/mutants"
	selport "github.com/donvargax/itos-cc/internal/selection/port"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
	"github.com/donvargax/itos-cc/internal/testinputs"
	"github.com/donvargax/itos-cc/internal/testtools"
	"github.com/donvargax/itos-cc/internal/workdir"
	"github.com/donvargax/itos-cc/project"
)

// Options control a mutation run.
type Options struct {
	// Store holds the project's snapshots: each file's is read from it, and
	// written back to it, under the run's context.
	Store        snapport.Store
	Snapshots    mutants.MutationSnapshots
	Source       mutants.SourceEdits
	Tree         selport.Tree
	Surroundings langport.Surroundings
	Root, Dir    string
	// WorkerFactory owns private command copies; cmd supplies the real binding.
	WorkerFactory mutants.Workers
	// Toolchain and Fingerprints are actual observation bindings. Direct legacy
	// fixtures retain real defaults until T-92; cmd must supply both.
	Toolchain     mutants.Toolchain
	Fingerprints  mutants.Fingerprints
	Workers       int
	MutateAll     bool    // rerun killed mutants of unchanged functions too
	TimeoutFactor float64 // a mutant may run this many times the baseline
	TestCommand   string  // shell command that replaces the per-language default
	AllTests      bool    // kill mutants with the whole suite, integration and end-to-end tests included
	Annotate      bool    // write a summary comment at the end of each file
	Coverage      *coverage.Report
	Measurement   *mutants.RunMeasurement
	// StatementCoverage is a successful independently admitted coverage
	// measurement for strict mode, of every strict language
	// (StrictCoverage). It is attached per unit of a file of such a
	// language, including units with no mutation sites.
	StatementCoverage    *coverage.Report
	CoverageProducer     string
	RecordCoverageInputs bool
	CachedCoverage       func(path, function, hash string) *CoverageEvidence
	// Support is the hashes of the support files of the listed tests now
	// (SupportHashes): what a listed outcome records, and holds while they
	// are unchanged.
	Support map[string]string
	// Listed, when set, runs the listed tests whose coverage reaches a
	// mutant's line (coverage.Report.LineTests) once the file's own tests
	// survived it.
	Listed *Listed
	// Judge, when set, says which functions of the file at path to judge,
	// by namespace#name and, to tell apart functions sharing a name, hash
	// (UnitHash). The others never run: they keep what their snapshot
	// records.
	Judge func(path, function, hash string) bool
	// Renamed, when set, is the path the file at path had before a rename,
	// or "": its snapshot moves to the new path, and the old one is removed.
	Renamed func(path string) string
	// Tests, when set, lists the test files that reach the file at path
	// (graph.TestsImporting), whose hashes its snapshot records: a kill is
	// reused only while they are those recorded. In Python they are also
	// the tests its own command runs (TestCommand). Unset, a file has none.
	Tests func(path string) []string
	// Exceptions are the survivors itos-cc.yaml excepts: each one of a
	// function judged that still survives is excepted, and one that no
	// longer holds is stale.
	Exceptions []config.Exception
	Log        io.Writer
}

// Listed is the domain's actual shared listed-ID/file inventory.
type Listed = mutants.Listed

// domainOptions retains direct legacy library composition until those callers
// move. Actual cmd composition checks every required binding before this entry.
func domainOptions(opt Options) mutants.Options {
	if opt.Snapshots == nil && opt.Store != nil {
		opt.Snapshots = metricsdir.Mutations{Store: opt.Store}
	}
	if opt.Toolchain == nil {
		opt.Toolchain = testtools.Toolchain{}
	}
	if opt.Fingerprints == nil {
		opt.Fingerprints = testinputs.Fingerprints{Runtime: testinputs.CurrentRuntime()}
	}
	if opt.Tree == nil {
		opt.Tree = workdir.Tree{}
	}
	if opt.Surroundings == nil {
		opt.Surroundings = workdir.Surroundings{}
	}
	if opt.Source == nil {
		opt.Source = workdir.SourceEdits{}
	}
	if opt.Root == "" {
		opt.Root = project.Root()
	}
	if opt.Dir == "" {
		opt.Dir, _ = os.Getwd()
	}
	return mutants.Options{Store: opt.Snapshots, Source: opt.Source, Root: opt.Root, Dir: opt.Dir, Tree: opt.Tree, Surroundings: opt.Surroundings,
		WorkerFactory: legacyWorkers(opt.WorkerFactory), Toolchain: opt.Toolchain, Fingerprints: opt.Fingerprints,
		Workers: opt.Workers, MutateAll: opt.MutateAll, TimeoutFactor: opt.TimeoutFactor, TestCommand: opt.TestCommand, AllTests: opt.AllTests, Annotate: opt.Annotate,
		Coverage: opt.Coverage, Measurement: opt.Measurement, StatementCoverage: opt.StatementCoverage, CoverageProducer: opt.CoverageProducer,
		RecordCoverageInputs: opt.RecordCoverageInputs, CachedCoverage: opt.CachedCoverage, Support: opt.Support, Listed: opt.Listed,
		Judge: opt.Judge, Renamed: opt.Renamed, Tests: opt.Tests, Exceptions: opt.Exceptions, Progress: RunProgress{Log: opt.Log}}
}

func runError(err error) error {
	var read mutants.SnapshotReadFailure
	if errors.As(err, &read) {
		return fmt.Errorf("%s: %w", SnapshotName(read.Key), read.Cause)
	}
	return err
}

// Run delegates existing mutation orchestration to its domain owner.
func Run(ctx context.Context, files []string, opt Options) ([]FileResult, error) {
	results, err := mutants.Run(ctx, files, domainOptions(opt))
	return results, runError(err)
}
func RunFailFast(ctx context.Context, files []string, opt Options, ff FailFast) ([]FileResult, *Stop, error) {
	results, stop, err := mutants.RunFailFast(ctx, files, domainOptions(opt), ff)
	return results, stop, runError(err)
}

// MeasureCoverage lets the legacy strict admission path make the same bound
// request before entering Run, while lazy ordinary measurement stays domain-owned.
func MeasureCoverage(ctx context.Context, opt Options, sources []string, strict bool) (*coverage.Report, error) {
	return domainOptions(opt).Measure(ctx, sources, strict)
}
