package main

import (
	"context"
	"errors"
	"io"
	"os"
	"runtime"

	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
	"github.com/donvargax/itos-cc/internal/coveragetools"
	"github.com/donvargax/itos-cc/internal/freshenv"
	"github.com/donvargax/itos-cc/internal/githistory"
	"github.com/donvargax/itos-cc/internal/importgraph"
	langport "github.com/donvargax/itos-cc/internal/lang/port"
	"github.com/donvargax/itos-cc/internal/metricsdir"
	mutation "github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/internal/selection"
	selport "github.com/donvargax/itos-cc/internal/selection/port"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
	"github.com/donvargax/itos-cc/internal/testinputs"
	"github.com/donvargax/itos-cc/internal/testtools"
	"github.com/donvargax/itos-cc/internal/workdir"
	"github.com/donvargax/itos-cc/internal/worktrees"
	"github.com/donvargax/itos-cc/project"
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
	// Coverage is the coverage port, which measures coverage for crap.
	Coverage covport.Measurer[*coverage.Report]
	// MutationCoverage supplies listed evidence and executable inventories.
	MutationCoverage coverage.MutationMeasurer
	// Config is itos-cc.yaml.
	Config configFile
	// Repository and PhysicalDir are what a counted mutation run starts
	// from: the top of the Git repository holding the working directory,
	// and the working directory, both physical.
	Repository  func(ctx context.Context) (string, error)
	PhysicalDir func(ctx context.Context) (string, error)
	// Platform is the system itos-cc runs on, which mutation run's --count
	// and --fail-fast check (countedPlatform).
	Platform string
}

// configFile is itos-cc.yaml as the moved commands read it.
type configFile interface {
	// Read is the project root, where itos-cc.yaml is, and the file's text
	// there, nil when there is none, or the error that kept it from being
	// read.
	Read(ctx context.Context) (root string, text []byte, err error)
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
	owner := &worktrees.OwnedExecutor{Platform: runtime.GOOS, Hold: hold}
	coverageRun := &worktrees.OwnedExecutor{Platform: runtime.GOOS, Hold: hold}
	measurer := coveragetools.Measurer{Log: log, Execute: coverageCommand(coverageRun)}
	tools := testtools.Toolchain{Dir: dir, Env: os.Environ()}
	fingerprints := testinputs.Fingerprints{Runtime: testinputs.CurrentRuntime()}
	fresh := freshenv.Environment{Platform: runtime.GOOS, Log: log, Execute: owner.Run, Command: owner.Command, Toolchain: tools, Fingerprints: fingerprints, Reach: importgraph.Reach{}, Tree: workdir.Tree{}, Surroundings: workdir.Surroundings{}}
	return adapters{Settings: config.Reader{Source: diskConfig{}}, SnapshotReaders: func(root string) mutation.MutationSnapshotReader {
		return metricsdir.Mutations{Store: metricsdir.Store{RootDir: root}}
	}, Fresh: fresh, FreshWorkers: worktrees.Workers{Platform: runtime.GOOS, Owner: owner, Prepare: freshenv.CommandEnvironment}, Exceptions: func(root string) mutation.Exceptions { return config.Exceptions{RootDir: root} }, SourceStatus: workdir.SourceEdits{}, SnapshotReader: metricsdir.Mutations{Store: metricsdir.Store{}}, Catalog: metricsdir.Mutations{Store: metricsdir.Store{}}, Presence: workdir.SourceEdits{}, Snapshots: metricsdir.Mutations{Store: metricsdir.Store{}}, SourceEdits: workdir.SourceEdits{}, TestLister: testtools.Lister{Platform: runtime.GOOS, Log: log}, History: githistory.History{Dir: dir}, Toolchain: testtools.Toolchain{Dir: dir, Env: os.Environ()}, Fingerprints: testinputs.Fingerprints{Runtime: testinputs.CurrentRuntime()}, Reach: importgraph.Reach{}, Workers: worktrees.Workers{Platform: runtime.GOOS, Hold: hold}, Dir: dir, Tree: workdir.Tree{}, Surroundings: workdir.Surroundings{}, Store: metricsdir.Store{},
		Coverage: measurer, MutationCoverage: measurer, Config: diskConfig{},
		Repository: countedRoot, PhysicalDir: physicalDir, Platform: countedPlatform}
}

// diskConfig is itos-cc.yaml at the project root of the working directory.
type diskConfig struct{}

// Read is config.Read at project.Root. Neither asks anything a context
// could cancel.
func (diskConfig) Read(context.Context) (string, []byte, error) {
	root := project.Root()
	text, err := config.Read(root)
	return root, text, err
}

// exclusionOf is the sources itos-cc.yaml's exclude leaves out, its globs
// from the root file gives with the file's text, read under ctx: a file
// that cannot be read as a config is config.invalid.
func exclusionOf(ctx context.Context, file configFile) (selection.Exclusion, error) {
	root, text, err := file.Read(ctx)
	if err != nil {
		return selection.Exclusion{}, err
	}
	cfg, err := config.Parse(text)
	if err != nil {
		return selection.Exclusion{}, invalidConfig(err)
	}
	return selection.Exclusion{Root: root, Globs: cfg.Exclude}, nil
}

// invalidConfig is err, from reading itos-cc.yaml, as a person reads it: a
// file that cannot be read as a config is config.invalid, saying what each
// key needs, and any other error is err.
func invalidConfig(err error) error {
	var invalid *config.InvalidError
	if errors.As(err, &invalid) {
		p := cli.NewProblem(cli.CodeUsage, "config.invalid", invalid.Error(),
			"Fix it: exclude lists globs of sources from the project root, such as **/porttest/**; each entry under mutation.exceptions needs file, function, hash, line_in_function, column, original, replacement, and reason, and 'itos-cc mutation except' writes one; mutation.tests needs list, run with {pattern}, ids_pattern with {ids}, and join with each, holding {id}, and sep.").With("file", config.File)
		if invalid.Key != "" {
			p = p.With("key", invalid.Key)
		}
		return p
	}
	return err
}
