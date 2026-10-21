package mutation

import (
	"context"
	"errors"
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage"
	coverageporttest "github.com/donvargax/itos-cc/internal/coverage/porttest"
	langport "github.com/donvargax/itos-cc/internal/lang/port"
	langporttest "github.com/donvargax/itos-cc/internal/lang/port/porttest"
	selport "github.com/donvargax/itos-cc/internal/selection/port"
	selporttest "github.com/donvargax/itos-cc/internal/selection/port/porttest"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	mutantstest "github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
	snapporttest "github.com/donvargax/itos-cc/internal/snapshot/port/porttest"
)

// ports are the Ports cmd's adapters give the handler, held in memory, each
// a field its method gives as it is: a field left nil is a binding cmd left
// nil. Each port is a fake of its own, so a port copied to another's field
// is told apart.
type ports struct {
	tree         selport.Tree
	surroundings langport.Surroundings
	store        snapport.Store
	settings     mutants.SettingsReader
	reader       mutants.MutationSnapshotReader
	catalog      mutants.MutationCatalog
	presence     mutants.SourcePresence
	status       mutants.SourceStatus
	history      mutants.History
	toolchain    mutants.Toolchain
	fingerprints mutants.Fingerprints
	reach        mutants.Reach
	workers      mutants.Workers
	snapshots    mutants.MutationSnapshots
	edits        mutants.SourceEdits
	lister       mutants.TestLister
	fresh        mutants.Fresh
	freshWorkers mutants.Workers
	coverage     coverage.MutationMeasurer
	platform     string
	dir          string
	exceptions   func(root string) mutants.Exceptions
	repository   func(ctx context.Context) (string, error)
	physicalDir  func(ctx context.Context) (string, error)
}

func (p ports) SelectionTree() selport.Tree                    { return p.tree }
func (p ports) LangSurroundings() langport.Surroundings        { return p.surroundings }
func (p ports) SnapshotStore() snapport.Store                  { return p.store }
func (p ports) MutationSettings() mutants.SettingsReader       { return p.settings }
func (p ports) MutationCatalog() mutants.MutationCatalog       { return p.catalog }
func (p ports) MutationSourcePresence() mutants.SourcePresence { return p.presence }
func (p ports) MutationSourceStatus() mutants.SourceStatus     { return p.status }
func (p ports) MutationHistory() mutants.History               { return p.history }
func (p ports) MutationToolchain() mutants.Toolchain           { return p.toolchain }
func (p ports) MutationFingerprints() mutants.Fingerprints     { return p.fingerprints }
func (p ports) MutationReach() mutants.Reach                   { return p.reach }
func (p ports) MutationWorkers() mutants.Workers               { return p.workers }
func (p ports) MutationSnapshots() mutants.MutationSnapshots   { return p.snapshots }
func (p ports) MutationSourceEdits() mutants.SourceEdits       { return p.edits }
func (p ports) MutationTestLister() mutants.TestLister         { return p.lister }
func (p ports) MutationFresh() mutants.Fresh                   { return p.fresh }
func (p ports) MutationFreshWorkers() mutants.Workers          { return p.freshWorkers }
func (p ports) MutationPlatform() string                       { return p.platform }
func (p ports) WorkingDir() string                             { return p.dir }
func (p ports) MutationSnapshotReader() mutants.MutationSnapshotReader {
	return p.reader
}
func (p ports) CoverageMutationMeasurer() coverage.MutationMeasurer { return p.coverage }
func (p ports) MutationExceptions() func(root string) mutants.Exceptions {
	return p.exceptions
}
func (p ports) MutationRepository() func(ctx context.Context) (string, error) {
	return p.repository
}
func (p ports) MutationPhysicalDir() func(ctx context.Context) (string, error) {
	return p.physicalDir
}

// NewHandler copies each of its Ports into the handler's field for it, the
// functions cmd binds among them, and reads none: a settings binding that
// would fail is not read, since each command reads its settings at its own
// admission point.
func TestNewHandlerIsBuiltOverItsPorts(t *testing.T) {
	writer := &mutantstest.Exceptions{}
	p := ports{
		tree: &selporttest.Tree{}, surroundings: &langporttest.Surroundings{}, store: &snapporttest.Store{},
		settings: &mutantstest.Settings{}, reader: &mutantstest.Snapshots{}, catalog: &mutantstest.Snapshots{},
		presence: &mutantstest.SourceEdits{}, status: &mutantstest.SourceEdits{}, history: &mutantstest.History{},
		toolchain: &mutantstest.Inputs{}, fingerprints: &mutantstest.Inputs{}, reach: &mutantstest.Inputs{},
		workers: &mutantstest.Workers{}, snapshots: &mutantstest.Snapshots{}, edits: &mutantstest.SourceEdits{},
		lister: &mutantstest.Lister{}, fresh: &mutantstest.Fresh{}, freshWorkers: &mutantstest.Workers{},
		coverage: &coverageporttest.Measurer{}, platform: "plan9", dir: "/work",
		exceptions:  func(root string) mutants.Exceptions { return writer },
		repository:  func(context.Context) (string, error) { return "/repository", nil },
		physicalDir: func(context.Context) (string, error) { return "/physical", nil },
	}
	h, err := NewHandler(t.Context(), p)
	if err != nil {
		t.Fatal(err)
	}
	for _, port := range []struct {
		name      string
		got, want any
	}{
		{"Tree", h.Tree, p.tree}, {"Surroundings", h.Surroundings, p.surroundings}, {"Metrics", h.Metrics, p.store},
		{"Settings", h.Settings, p.settings}, {"Store", h.Store, p.reader}, {"Catalog", h.Catalog, p.catalog},
		{"Presence", h.Presence, p.presence}, {"Status", h.Status, p.status}, {"History", h.History, p.history},
		{"Toolchain", h.Toolchain, p.toolchain}, {"Fingerprints", h.Fingerprints, p.fingerprints}, {"Reach", h.Reach, p.reach},
		{"Workers", h.Workers, p.workers}, {"Snapshots", h.Snapshots, p.snapshots}, {"SourceEdits", h.SourceEdits, p.edits},
		{"TestLister", h.TestLister, p.lister}, {"Fresh", h.Fresh, p.fresh}, {"FreshWorkers", h.FreshWorkers, p.freshWorkers},
		{"Coverage", h.Coverage, p.coverage}, {"Platform", h.Platform, p.platform}, {"Dir", h.Dir, p.dir},
	} {
		if port.got != port.want {
			t.Errorf("handler's %s = %v, want the Ports' own %v", port.name, port.got, port.want)
		}
	}
	if h.Exceptions == nil || h.Exceptions("/root") != writer {
		t.Error("handler's Exceptions does not make the Ports' writer")
	}
	if root, err := h.Repository(t.Context()); err != nil || root != "/repository" {
		t.Errorf("handler's Repository = %q, %v; want the Ports' /repository", root, err)
	}
	if dir, err := h.PhysicalDir(t.Context()); err != nil || dir != "/physical" {
		t.Errorf("handler's PhysicalDir = %q, %v; want the Ports' /physical", dir, err)
	}
	if _, err := NewHandler(t.Context(), ports{settings: failingSettings{}}); err != nil {
		t.Errorf("NewHandler read the settings: %v", err)
	}
}

// failingSettings fails if anything reads it.
type failingSettings struct{}

func (failingSettings) Read(context.Context) (*mutants.Config, mutants.SettingsFailure) {
	panic("the settings were read")
}

// The functions cmd binds reach the handler as the Ports give them, so one
// left nil stays nil, never a function that calls nil, and the handler
// still says which binding is missing, in its own words.
func TestNewHandlerKeepsANilBindingNil(t *testing.T) {
	h, err := NewHandler(t.Context(), ports{})
	if err != nil {
		t.Fatal(err)
	}
	if h.Exceptions != nil || h.Repository != nil || h.PhysicalDir != nil {
		t.Fatalf("a nil binding is no longer nil: Exceptions %v, Repository %v, PhysicalDir %v", h.Exceptions != nil, h.Repository != nil, h.PhysicalDir != nil)
	}
	if _, err := h.exceptions("/root"); !errors.Is(err, errNoExceptions) || err.Error() != "mutation Exceptions binding is missing" {
		t.Errorf("no Exceptions: %v", err)
	}
	if err := h.Counted(t.Context(), CountedQuery{}); err == nil || err.Error() != "mutation repository binding is missing" {
		t.Errorf("no Repository: %v", err)
	}
	if _, err := h.countedPaths(t.Context(), "/root", []string{"board.go"}); err == nil || err.Error() != "mutation working directory binding is missing" {
		t.Errorf("no PhysicalDir: %v", err)
	}
}
