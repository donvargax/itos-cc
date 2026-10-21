package mutation

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/donvargax/itos-cc/internal/coverage"
	langport "github.com/donvargax/itos-cc/internal/lang/port"
	"github.com/donvargax/itos-cc/internal/selection"
	selport "github.com/donvargax/itos-cc/internal/selection/port"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// Handler holds only actual domain capabilities. Cmd selects the adapters;
// settings are reread at their existing admission points, never cached here.
type Handler struct {
	Settings     mutants.SettingsReader
	Tree         selport.Tree
	Surroundings langport.Surroundings
	Store        mutants.MutationSnapshotReader
	Catalog      mutants.MutationCatalog
	Presence     mutants.SourcePresence
	Status       mutants.SourceStatus
	History      mutants.History
	Toolchain    mutants.Toolchain
	Fingerprints mutants.Fingerprints
	Reach        mutants.Reach
	Workers      mutants.Workers
	// Exceptions makes the writer of the settings at a root, which except
	// and renew update; cmd selects it.
	Exceptions func(root string) mutants.Exceptions
	Dir        string
	// Run's own: Snapshots publishes what it judges, SourceEdits annotates
	// the sources, TestLister lists the tests itos-cc.yaml names, Coverage
	// measures, and Metrics is the store orphaned snapshots leave.
	Snapshots   mutants.MutationSnapshots
	SourceEdits mutants.SourceEdits
	TestLister  mutants.TestLister
	Coverage    coverage.MutationMeasurer
	Metrics     snapport.Store
	// A counted run's: Fresh prepares the committed inputs, FreshWorkers
	// runs the trials in them, Repository is the top of the Git repository
	// holding the working directory and PhysicalDir the working directory,
	// both physical. The run's context is the command's, which an
	// interrupt cancels (decision 38). Cmd observes and binds each.
	Fresh        mutants.Fresh
	FreshWorkers mutants.Workers
	Repository   func(ctx context.Context) (string, error)
	PhysicalDir  func(ctx context.Context) (string, error)
	// Platform is the system the run is on, a value cmd detects: --count
	// and --fail-fast run on Linux and macOS.
	Platform string
}

// Ports are what the mutation group reaches the world through, which cmd's
// adapters give it, each named for the port of the domain it answers:
// selection's tree, lang's surroundings and the snapshot store the other
// slices take too, then the mutation domain's own, the coverage measurer
// mutation run measures by, the platform and the working directory.
// MutationExceptions, MutationRepository and MutationPhysicalDir give the
// functions cmd binds, as they are: a binding left nil stays nil, so the
// handler's "binding is missing" failure still says so.
type Ports interface {
	SelectionTree() selport.Tree
	LangSurroundings() langport.Surroundings
	SnapshotStore() snapport.Store
	MutationSettings() mutants.SettingsReader
	MutationSnapshotReader() mutants.MutationSnapshotReader
	MutationCatalog() mutants.MutationCatalog
	MutationSourcePresence() mutants.SourcePresence
	MutationSourceStatus() mutants.SourceStatus
	MutationHistory() mutants.History
	MutationToolchain() mutants.Toolchain
	MutationFingerprints() mutants.Fingerprints
	MutationReach() mutants.Reach
	MutationWorkers() mutants.Workers
	MutationSnapshots() mutants.MutationSnapshots
	MutationSourceEdits() mutants.SourceEdits
	MutationTestLister() mutants.TestLister
	MutationFresh() mutants.Fresh
	MutationFreshWorkers() mutants.Workers
	CoverageMutationMeasurer() coverage.MutationMeasurer
	MutationPlatform() string
	WorkingDir() string
	MutationExceptions() func(root string) mutants.Exceptions
	MutationRepository() func(ctx context.Context) (string, error)
	MutationPhysicalDir() func(ctx context.Context) (string, error)
}

// NewHandler is the handler over p's ports, copied as they are. It reads
// nothing, the settings least of all, which each command reads at its own
// admission point, so help and a line kong refuses read nothing; its error
// is there for the one shape every slice's constructor has.
func NewHandler(_ context.Context, p Ports) (Handler, error) {
	return Handler{
		Settings: p.MutationSettings(), Tree: p.SelectionTree(), Surroundings: p.LangSurroundings(),
		Store: p.MutationSnapshotReader(), Catalog: p.MutationCatalog(), Presence: p.MutationSourcePresence(),
		Status: p.MutationSourceStatus(), History: p.MutationHistory(), Toolchain: p.MutationToolchain(),
		Fingerprints: p.MutationFingerprints(), Reach: p.MutationReach(), Workers: p.MutationWorkers(),
		Exceptions: p.MutationExceptions(), Dir: p.WorkingDir(),
		Snapshots: p.MutationSnapshots(), SourceEdits: p.MutationSourceEdits(), TestLister: p.MutationTestLister(),
		Coverage: p.CoverageMutationMeasurer(), Metrics: p.SnapshotStore(),
		Fresh: p.MutationFresh(), FreshWorkers: p.MutationFreshWorkers(),
		Repository: p.MutationRepository(), PhysicalDir: p.MutationPhysicalDir(), Platform: p.MutationPlatform(),
	}, nil
}

// SelectionQuery retains the selection's explicit optional ref, including empty.
type SelectionQuery struct {
	Paths   []string
	Changed bool
	Since   *string
}

// settings is one observation of the settings: the root its selection
// settings were found at, and the settings.
func (h Handler) settings(ctx context.Context) (string, *mutants.Config, error) {
	if h.Settings == nil {
		return "", nil, errors.New("mutation settings binding is missing")
	}
	config, failure := h.Settings.Read(ctx)
	if failure != nil {
		return "", nil, failure
	}
	if config == nil {
		return "", nil, errors.New("mutation settings binding returned no value")
	}
	return config.Selection.Root, config, nil
}
func (h Handler) exclusion(ctx context.Context) (selection.Exclusion, error) {
	_, config, err := h.settings(ctx)
	if err != nil {
		return selection.Exclusion{}, err
	}
	return selection.Exclusion(config.Selection), nil
}
func (h Handler) files(ctx context.Context, q SelectionQuery) (selection.Files, error) {
	exclude, err := h.exclusion(ctx)
	if err != nil {
		return selection.Files{}, err
	}
	if h.Tree == nil {
		return selection.Files{}, errors.New("mutation source tree binding is missing")
	}
	return selection.Select(ctx, h.Tree, q.Paths, q.Changed, exclude)
}
func sourcePath(dir, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(dir, path)
}
func (h Handler) options(root string, judge func(string, string, string) bool, tests func(string) []string, support map[string]string, exceptions []mutants.Exception) mutants.CheckOptions {
	return mutants.CheckOptions{Store: h.Store, Tree: h.Tree, Surroundings: h.Surroundings, Toolchain: h.Toolchain, Fingerprints: h.Fingerprints, Root: root, Dir: h.Dir, Judge: judge, Tests: tests, Support: support, Exceptions: exceptions}
}
func beneath(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// relTo is path from dir, as a person reads it: as it is when dir is not
// known or does not hold it.
func relTo(dir, path string) string {
	if dir == "" {
		return path
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return rel
}

// absIn is path made absolute from dir, false when dir is not known and
// path is relative.
func absIn(dir, path string) (string, bool) {
	if !filepath.IsAbs(path) && dir == "" {
		return path, false
	}
	return sourcePath(dir, path), true
}
