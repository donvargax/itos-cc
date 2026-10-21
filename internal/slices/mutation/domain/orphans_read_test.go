package domain_test

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"reflect"
	"testing"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

func TestOrphanInventoryUsesMappedPresenceBeforePermissiveMetadata(t *testing.T) {
	root := t.TempDir()
	present := filepath.Join(root, "present.go")
	catalog := &porttest.Snapshots{RootDir: root, CatalogFiles: map[string]string{"mutate/.json": "", "mutate/missing.go.json": "wrong.go", "mutate/present.go.json": ""}, CatalogFailures: map[string]snapport.Failure{"mutate/present.go.json": &snapport.Unreadable{Reason: "malformed metadata"}}}
	presence := &porttest.SourceEdits{Files: map[string]string{present: "independent source bytes"}}
	got, err := mutants.Orphans(t.Context(), catalog, presence, func(string) bool { return true }, func(string) bool { return false })
	if err != nil || len(got) != 1 || got[0].Path != filepath.Join(root, "missing.go") || got[0].Snapshot != ".metrics/mutate/missing.go.json" || got[0].Why != mutants.OrphanGone {
		t.Fatalf("actual orphan inventory: %+v %v", got, err)
	}
	if !reflect.DeepEqual(catalog.MetadataReads, []string{"mutate/missing.go.json"}) {
		t.Fatalf("metadata read before mapped presence: %v", catalog.MetadataReads)
	}
	got, err = mutants.Orphans(t.Context(), catalog, presence, func(string) bool { return true }, func(path string) bool { return path == present })
	if err != nil || len(got) != 2 || got[1].Why != mutants.OrphanExcluded {
		t.Fatalf("excluded present source: %+v %v", got, err)
	}
	if len(catalog.Written) != 0 || len(catalog.Removed) != 0 || len(presence.Writes) != 0 {
		t.Fatal("orphan observation wrote")
	}
}
func TestOrphansFollowOnlyAgreeingAbsoluteRecordedFiles(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.go")
	name := filepath.ToSlash(filepath.Join("mutate", filepath.ToSlash(outside)+".json"))
	catalog := &porttest.Snapshots{RootDir: root, CatalogFiles: map[string]string{name: filepath.ToSlash(outside)}}
	presence := &porttest.SourceEdits{Files: map[string]string{outside: "independent outside source"}}
	covered := func(path string) bool { return path == outside }
	got, err := mutants.Orphans(t.Context(), catalog, presence, covered, func(string) bool { return false })
	if err != nil || len(got) != 0 {
		t.Fatalf("present recorded outside file: %+v %v", got, err)
	}
	delete(presence.Files, outside)
	got, err = mutants.Orphans(t.Context(), catalog, presence, covered, func(string) bool { return false })
	if err != nil || len(got) != 1 || got[0].Path != outside {
		t.Fatalf("gone outside file: %+v %v", got, err)
	}
	for _, file := range []string{"relative.go", filepath.Join(t.TempDir(), "wrong.go"), ""} {
		catalog.CatalogFiles[name] = file
		got, err = mutants.Orphans(t.Context(), catalog, presence, covered, func(string) bool { return false })
		if err != nil || len(got) != 0 {
			t.Fatalf("unadmitted metadata %q followed: %+v %v", file, got, err)
		}
	}
	catalog.CatalogFailures = map[string]snapport.Failure{name: &snapport.Unreadable{Reason: "actual invalid metadata"}}
	got, err = mutants.Orphans(t.Context(), catalog, presence, covered, func(string) bool { return false })
	if err != nil || len(got) != 0 {
		t.Fatalf("malformed metadata did not fall back: %+v %v", got, err)
	}
}
func TestOrphanPresenceIsConservativeButRenameEvidenceRequiresSuccess(t *testing.T) {
	root := t.TempDir()
	at := func(name string) string { return filepath.Join(root, name) }
	catalog := &porttest.Snapshots{RootDir: root, CatalogFiles: map[string]string{"mutate/dangling.json": "", "mutate/denied.json": "", "mutate/directory.json": ""}}
	presence := &porttest.SourceEdits{Directories: map[string]bool{at("directory"): true}, Links: map[string]string{at("dangling"): "absent"}, LstatErrors: map[string]error{at("denied"): fs.ErrPermission}}
	got, err := mutants.Orphans(t.Context(), catalog, presence, func(string) bool { return true }, func(string) bool { return false })
	if err != nil || len(got) != 0 || len(catalog.MetadataReads) != 0 {
		t.Fatalf("conservative Lstat inventory: %+v %v metadata%v", got, err, catalog.MetadataReads)
	}
	since := map[string]string{at("old"): at("denied"), at("denied"): at("dangling"), at("cycle"): at("cycle2"), at("cycle2"): at("cycle")}
	renamed, err := mutants.RenamedToAFile(t.Context(), at("old"), since, map[string]string{at("old"): at("irrelevant")}, presence)
	if err != nil || !renamed || !reflect.DeepEqual(presence.TargetPaths, []string{at("denied"), at("dangling")}) {
		t.Fatalf("lazy since-first success-only traversal: %v %v paths%v", renamed, err, presence.TargetPaths)
	}
	if renamed, err := mutants.RenamedToAFile(t.Context(), at("cycle"), since, nil, presence); err != nil || renamed {
		t.Fatalf("cycle: %v %v", renamed, err)
	}
}
func TestOrphanFailuresAndHistoryRefusalKeepReadOnlyBoundaries(t *testing.T) {
	root := t.TempDir()
	catalog := &porttest.Snapshots{RootDir: root}
	presence := &porttest.SourceEdits{}
	covered := func(string) bool { return true }
	excluded := func(string) bool { return false }
	for _, failure := range []snapport.Failure{&snapport.Unlistable{Reason: "denied"}, &snapport.Unreadable{Reason: "bad root"}} {
		if _, ok := failure.(*snapport.Unlistable); ok {
			catalog.ListFailure = failure
		} else {
			catalog.ListFailure = nil
			catalog.RootFailure = failure
		}
		if _, err := mutants.Orphans(t.Context(), catalog, presence, covered, excluded); err != failure {
			t.Fatalf("catalog failure lost: %v", err)
		}
	}
	orphans := []mutants.Orphan{{Path: filepath.Join(root, "gone.go"), Why: mutants.OrphanGone}}
	history := porttest.History{Dir: root, Root: root}
	if got, err := mutants.NotRenamed(t.Context(), orphans, history, presence, "", false); err != nil || got != nil {
		t.Fatalf("ordinary no-repository must judge none: %+v %v", got, err)
	}
	if _, err := mutants.NotRenamed(t.Context(), orphans, nil, presence, "", false); err == nil {
		t.Fatal("missing history accepted")
	}
	if _, err := mutants.Orphans(t.Context(), nil, presence, covered, excluded); err == nil {
		t.Fatal("missing catalog accepted")
	}
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("orphan caller stopped")
	cancel(cause)
	if _, err := mutants.Orphans(ctx, catalog, presence, covered, excluded); !errors.Is(err, cause) {
		t.Fatalf("catalog cancellation: %v", err)
	}
	if _, err := mutants.NotRenamed(ctx, orphans, history, presence, "", false); !errors.Is(err, cause) {
		t.Fatalf("history cancellation: %v", err)
	}
	if _, err := mutants.RenamedToAFile(ctx, orphans[0].Path, nil, nil, presence); !errors.Is(err, cause) {
		t.Fatalf("target cancellation: %v", err)
	}
}

func TestOrphanRenameFilterUsesActualCommittedAndTrackedTrees(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "old.go")
	current := filepath.Join(root, "new.go")
	repo := porttest.NewHistoryRepository()
	repo.Index = map[string]string{"old.go": runSource}
	base := repo.Commit("actual base")
	repo.Refs["base"] = base
	repo.Index = map[string]string{"new.go": runSource}
	repo.Commit("actual rename")
	history := porttest.History{Root: root, Dir: root, Repository: repo, Working: map[string]string{current: runSource}}
	source := &porttest.SourceEdits{Files: map[string]string{current: runSource}}
	orphans := []mutants.Orphan{{Path: old, Why: mutants.OrphanGone}, {Path: old, Why: mutants.OrphanExcluded}, {Path: filepath.Join(root, "gone.go"), Why: mutants.OrphanGone}}
	got, err := mutants.NotRenamed(t.Context(), orphans, history, source, "base", true)
	if err != nil || !reflect.DeepEqual(got, orphans[1:]) {
		t.Fatalf("actual committed rename filter: %+v %v", got, err)
	}
	got, err = mutants.NotRenamed(t.Context(), orphans, history, source, "invalid", true)
	if err != nil || got != nil {
		t.Fatalf("ordinary failed history must judge none: %+v %v", got, err)
	}
	repo.Index = map[string]string{"tracked.go": runSource}
	history.Working = map[string]string{filepath.Join(root, "tracked.go"): runSource}
	source.Files = history.Working
	got, err = mutants.NotRenamed(t.Context(), []mutants.Orphan{{Path: current, Why: mutants.OrphanGone}}, history, source, "", false)
	if err != nil || len(got) != 0 {
		t.Fatalf("actual tracked rename filter: %+v %v", got, err)
	}
}
