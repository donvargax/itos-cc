package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/coveragetools"
	langport "github.com/donvargax/itos-cc/internal/lang/port"
	"github.com/donvargax/itos-cc/internal/metricsdir"
	"github.com/donvargax/itos-cc/internal/processes"
	"github.com/donvargax/itos-cc/internal/project"
	"github.com/donvargax/itos-cc/internal/selection"
	selport "github.com/donvargax/itos-cc/internal/selection/port"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/exceptions"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/freshenv"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/githistory"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/importgraph"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/settings"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/snapshots"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/sources"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/testinputs"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/testtools"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/worktrees"
	mutation "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
	"github.com/donvargax/itos-cc/internal/version"
	"github.com/donvargax/itos-cc/internal/workdir"
)

// adapters are what the moved commands reach the world through, which main
// gives run: the working directory, the ports of the domains they use and
// itos-cc.yaml. A test runs itos-cc in process with porttest's fakes and a
// file held in memory instead, which enter cmd through its _test.go files
// alone (decision 44). The hand-parsed commands reach the disk themselves
// until they move, but for crap's coverage, which it measures through the
// port here.
type adapters struct {
	Settings        mutation.SettingsReader
	SnapshotReaders func(root string) mutation.MutationSnapshotReader
	Fresh           mutation.Fresh
	FreshWorkers    mutation.Workers
	// Exceptions constructs a capability from an already discovered root, without I/O.
	Exceptions     func(root string) mutation.Exceptions
	SourceStatus   mutation.SourceStatus
	SnapshotReader mutation.MutationSnapshotReader
	Catalog        mutation.MutationCatalog
	Presence       mutation.SourcePresence
	Snapshots      mutation.MutationSnapshots
	SourceEdits    mutation.SourceEdits
	TestLister     mutation.TestLister
	// Workers owns command copies and process lifetimes for mutation.
	History      mutation.History
	Workers      mutation.Workers
	Toolchain    mutation.Toolchain
	Fingerprints mutation.Fingerprints
	Reach        mutation.Reach
	// Dir is the working directory, absolute, which the UI names the files
	// a person reads from; "" when it cannot be told.
	Dir string
	// Tree is selection's port, the working tree the commands select from.
	Tree selport.Tree
	// Surroundings is lang's port, what surrounds a file whose units it
	// names.
	Surroundings langport.Surroundings
	// Store is the snapshot port, the snapshots under .metrics/.
	Store snapport.Store
	// Coverage is coverage's Measurer, which measures coverage for crap.
	Coverage coverage.Measurer
	// MutationCoverage supplies listed evidence and executable inventories.
	MutationCoverage coverage.MutationMeasurer
	// Selection is the selection engine's settings, itos-cc.yaml's exclude
	// and the root it is at, which crap, dry and units select through.
	Selection selport.SettingsReader
	// Repository and PhysicalDir are what a counted mutation run starts
	// from: the top of the Git repository holding the working directory,
	// and the working directory, both physical.
	Repository  func(ctx context.Context) (string, error)
	PhysicalDir func(ctx context.Context) (string, error)
	// Platform is the system itos-cc runs on, which mutation run's --count
	// and --fail-fast check (countedPlatform).
	Platform string
	// Origin is the URL of the git remote origin of the repository holding
	// a folder, "" for none, which graph names the repository by.
	Origin func(ctx context.Context, dir string) string
}

// SelectionTree is the selection port's tree, a slice's Ports method.
func (a adapters) SelectionTree() selport.Tree { return a.Tree }

// LangSurroundings is lang's port, a slice's Ports method.
func (a adapters) LangSurroundings() langport.Surroundings { return a.Surroundings }

// SnapshotStore is the snapshot port, the snapshots under .metrics/, a
// slice's Ports method.
func (a adapters) SnapshotStore() snapport.Store { return a.Store }

// CoverageMeasurer is the coverage domain's Measurer, a slice's Ports
// method: coveragetools' over the coverage runner, its commands run in
// process groups internal/processes' executor owns (coverageCommand), never
// through a mutation adapter.
func (a adapters) CoverageMeasurer() coverage.Measurer { return a.Coverage }

// Rel is path as a person reads it, from the working directory, Dir, the
// one the UI names paths from (cli.Rel), a slice's Ports method.
func (a adapters) Rel(path string) string { return cli.Rel(a.Dir, path) }

// BuildVersion is the version this binary says it is, internal/version's,
// the one source a release's stamp and Go's module version reach, a slice's
// Ports method.
func (adapters) BuildVersion() string { return version.Version() }

// SelectionExclusion is the sources itos-cc.yaml's exclude names, read under
// ctx through the selection engine's settings, a slice's Ports method: a
// file that cannot be read is config.invalid (settingsError).
func (a adapters) SelectionExclusion(ctx context.Context) (selection.Exclusion, error) {
	return exclusion(ctx, a.Selection)
}

// MutationSettings is the mutation domain's settings reader, itos-cc.yaml
// as mutation reads it at each admission point, a slice's Ports method.
func (a adapters) MutationSettings() mutation.SettingsReader { return a.Settings }

// MutationSnapshotReader is the reader of the mutation snapshots under
// .metrics/, a slice's Ports method.
func (a adapters) MutationSnapshotReader() mutation.MutationSnapshotReader {
	return a.SnapshotReader
}

// MutationCatalog is the catalog of the mutation snapshots, a slice's Ports
// method.
func (a adapters) MutationCatalog() mutation.MutationCatalog { return a.Catalog }

// MutationSourcePresence says whether a source is still there, a slice's
// Ports method.
func (a adapters) MutationSourcePresence() mutation.SourcePresence { return a.Presence }

// MutationSourceStatus is the sources' status in the working tree, a
// slice's Ports method.
func (a adapters) MutationSourceStatus() mutation.SourceStatus { return a.SourceStatus }

// MutationHistory is the committed history mutation judges --since by, a
// slice's Ports method.
func (a adapters) MutationHistory() mutation.History { return a.History }

// MutationToolchain is the test tools mutation observes, a slice's Ports
// method.
func (a adapters) MutationToolchain() mutation.Toolchain { return a.Toolchain }

// MutationFingerprints is the fingerprints of a mutant's inputs, a slice's
// Ports method.
func (a adapters) MutationFingerprints() mutation.Fingerprints { return a.Fingerprints }

// MutationReach is which tests reach a source, a slice's Ports method.
func (a adapters) MutationReach() mutation.Reach { return a.Reach }

// MutationWorkers runs mutants in copies of the working tree, a slice's
// Ports method.
func (a adapters) MutationWorkers() mutation.Workers { return a.Workers }

// MutationSnapshots publishes what mutation run judges, a slice's Ports
// method.
func (a adapters) MutationSnapshots() mutation.MutationSnapshots { return a.Snapshots }

// MutationSourceEdits annotates the sources, a slice's Ports method.
func (a adapters) MutationSourceEdits() mutation.SourceEdits { return a.SourceEdits }

// MutationTestLister lists the tests itos-cc.yaml names, a slice's Ports
// method.
func (a adapters) MutationTestLister() mutation.TestLister { return a.TestLister }

// MutationFresh prepares a counted run's committed inputs, a slice's Ports
// method.
func (a adapters) MutationFresh() mutation.Fresh { return a.Fresh }

// MutationFreshWorkers runs a counted run's trials in them, a slice's Ports
// method.
func (a adapters) MutationFreshWorkers() mutation.Workers { return a.FreshWorkers }

// CoverageMutationMeasurer is the coverage domain's MutationMeasurer, the
// listed evidence and executable inventories mutation run measures by, a
// slice's Ports method.
func (a adapters) CoverageMutationMeasurer() coverage.MutationMeasurer { return a.MutationCoverage }

// MutationPlatform is the system itos-cc runs on, which mutation run's
// --count and --fail-fast check, a slice's Ports method.
func (a adapters) MutationPlatform() string { return a.Platform }

// WorkingDir is the working directory, absolute, "" when it cannot be told,
// a slice's Ports method.
func (a adapters) WorkingDir() string { return a.Dir }

// MutationExceptions makes the writer of the settings at a root, a slice's
// Ports method: the field's value as it is, never a method value, so a
// binding left nil stays nil and the handler says it is missing.
func (a adapters) MutationExceptions() func(root string) mutation.Exceptions {
	return a.Exceptions
}

// MutationRepository is the top of the Git repository holding the working
// directory, physical, a slice's Ports method: the field's value as it is,
// so a binding left nil stays nil.
func (a adapters) MutationRepository() func(ctx context.Context) (string, error) {
	return a.Repository
}

// MutationPhysicalDir is the working directory, physical, a slice's Ports
// method: the field's value as it is, so a binding left nil stays nil.
func (a adapters) MutationPhysicalDir() func(ctx context.Context) (string, error) {
	return a.PhysicalDir
}

// ownedCommand is owner's Command as freshenv's preparation judges it: the
// engine's Result, which knows no domain, as mutation's CommandResult.
func ownedCommand(owner *processes.OwnedExecutor) func(context.Context, *exec.Cmd) (mutation.CommandResult, error) {
	return func(ctx context.Context, cmd *exec.Cmd) (mutation.CommandResult, error) {
		r, err := owner.Command(ctx, cmd)
		return mutation.CommandResult(r), err
	}
}

// diskAdapters are this process's: its working directory,
// internal/workdir's tree and surroundings, internal/metricsdir's store,
// internal/coveragetools' measurer, saying what it runs on log, and
// itos-cc.yaml at the project root on the disk. Every command they run in
// a process group they own marks, through hold, the group being killed
// and joined, which an interrupt waits for: the run's Interrupts', bound
// here, never held anywhere else.
func diskAdapters(log io.Writer, hold func() (release func())) adapters {
	dir, _ := os.Getwd()
	owner := &processes.OwnedExecutor{Platform: runtime.GOOS, Hold: hold}
	coverageRun := &processes.OwnedExecutor{Platform: runtime.GOOS, Hold: hold}
	measurer := coveragetools.Measurer{Log: log, Execute: coverageCommand(coverageRun)}
	tools := testtools.Toolchain{Dir: dir, Env: project.InheritedEnv()}
	fingerprints := testinputs.Fingerprints{Runtime: testinputs.CurrentRuntime()}
	fresh := freshenv.Environment{Platform: runtime.GOOS, Log: log, Execute: owner.Run, Command: ownedCommand(owner), Measure: ownedCommand(owner), Toolchain: tools, Fingerprints: fingerprints, Reach: importgraph.Reach{}, Tree: workdir.Tree{}, Surroundings: workdir.Surroundings{}}
	reader := settings.Reader{Source: diskConfig{}}
	return adapters{Settings: reader, SnapshotReaders: func(root string) mutation.MutationSnapshotReader {
		return snapshots.Mutations{Store: metricsdir.Store{RootDir: root}}
	}, Fresh: fresh, FreshWorkers: worktrees.Workers{Platform: runtime.GOOS, Owner: owner, Prepare: freshenv.CommandEnvironment}, Exceptions: func(root string) mutation.Exceptions { return exceptions.Exceptions{RootDir: root} }, SourceStatus: sources.SourceEdits{}, SnapshotReader: snapshots.Mutations{Store: metricsdir.Store{}}, Catalog: snapshots.Mutations{Store: metricsdir.Store{}}, Presence: sources.SourceEdits{}, Snapshots: snapshots.Mutations{Store: metricsdir.Store{}}, SourceEdits: sources.SourceEdits{}, TestLister: testtools.Lister{Platform: runtime.GOOS, Log: log}, History: githistory.History{Dir: dir}, Toolchain: testtools.Toolchain{Dir: dir, Env: project.InheritedEnv()}, Fingerprints: testinputs.Fingerprints{Runtime: testinputs.CurrentRuntime()}, Reach: importgraph.Reach{}, Workers: worktrees.Workers{Platform: runtime.GOOS, Hold: hold, Dir: dir, Settings: reader}, Dir: dir, Tree: workdir.Tree{}, Surroundings: workdir.Surroundings{}, Store: metricsdir.Store{},
		Coverage: measurer, MutationCoverage: measurer, Selection: reader,
		Repository: countedRoot, PhysicalDir: physicalDir, Platform: countedPlatform, Origin: project.OriginAt}
}

// diskConfig is itos-cc.yaml at the project root of the working directory.
type diskConfig struct{}

// Read is settings.Read at project.Root. Neither asks anything a context
// could cancel.
func (diskConfig) Read(context.Context) (string, []byte, error) {
	root := project.Root()
	text, err := settings.Read(root)
	return root, text, err
}

// exclusion is the sources the selection settings reader leaves out, read
// under ctx: a nil reader is a missing binding, and a failure is worded by
// settingsError.
func exclusion(ctx context.Context, reader selport.SettingsReader) (selection.Exclusion, error) {
	if reader == nil {
		return selection.Exclusion{}, errors.New("selection settings binding is missing")
	}
	settings, failure := reader.ReadSelection(ctx)
	if failure != nil {
		return selection.Exclusion{}, settingsError(failure)
	}
	return selection.Exclusion(settings), nil
}

// settingsError is failure, from reading itos-cc.yaml, as a person reads it:
// a file that cannot be read, or read as a config, is config.invalid, with
// the refusal's own fix, else saying what each key needs, and a read that
// failed short of it or was cancelled is its cause.
func settingsError(failure selport.SettingsFailure) error {
	switch failure := failure.(type) {
	case *selport.SettingsInvalid:
		fix := failure.Fix
		if fix == "" {
			fix = "Fix it: exclude lists globs of sources from the project root, such as **/porttest/**; each entry under mutation.exceptions needs file, function, hash, line_in_function, column, original, replacement, and reason, and 'itos-cc mutation except' writes one; mutation.tests needs list, run with {pattern}, ids_pattern with {ids}, and join with each, holding {id}, and sep."
		}
		p := cli.NewProblem(cli.CodeUsage, "config.invalid", settings.File+": "+failure.Reason, fix).With("file", settings.File)
		if failure.Key != "" {
			p = p.With("key", failure.Key)
		}
		return p
	case *selport.SettingsFailed:
		return failure.Cause
	case *selport.Cancelled:
		return failure.Cause
	}
	return failure
}
