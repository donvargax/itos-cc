package mutation

import (
	"context"
	"errors"
	"maps"
	"path/filepath"
	"strings"
	"testing"

	selport "github.com/donvargax/itos-cc/internal/selection/port"
	seltest "github.com/donvargax/itos-cc/internal/selection/port/porttest"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	mutantstest "github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

// historyFixture is a repository whose base tag precedes a changed main.go,
// a deleted gone.go and a committed rename of old.go to new.go, with
// staged.go renamed to tracked.go in the working tree since.
func historyFixture(t *testing.T) (root string, files map[string]string, repository *mutantstest.HistoryRepository) {
	t.Helper()
	root = t.TempDir()
	files = map[string]string{}
	for name, text := range map[string]string{"go.mod": "module example/binding\n", "main.go": "package board\nfunc Value() int { return 1 }\n", "gone.go": "package board\nfunc Gone() int { return 7 }\n", "old.go": "package board\nfunc Old() int { return 8 }\n", "staged.go": "package board\nfunc Staged() int { return 9 }\n"} {
		files[filepath.Join(root, name)] = text
	}
	repository = mutantstest.NewHistoryRepository()
	git := func(args ...string) {
		t.Helper()
		if err := repository.Git(root, root, files, args...); err != nil {
			t.Fatal(err)
		}
	}
	git("add", "-A")
	git("commit", "-qm", "independent base")
	git("tag", "base")
	files[filepath.Join(root, "main.go")] = "package board\nfunc Value() int { return 2 }\n"
	delete(files, filepath.Join(root, "gone.go"))
	git("mv", "old.go", "new.go")
	git("add", "-A")
	git("commit", "-qm", "actual committed changes")
	git("mv", "staged.go", "tracked.go")
	return root, files, repository
}

// Moved from cmd with run's history consumers (T-100), now the slice's,
// which check shares: the --since range, the deleted files' exceptions and
// the orphans' renames come from the bound History and no-follow Presence,
// never the disk. No scenario reaches a missing or wrong binding.
func TestMutationHistoryConsumersRequireActualBindings(t *testing.T) {
	root, files, repository := historyFixture(t)
	relative := map[string]string{}
	for path, text := range files {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		relative[filepath.ToSlash(rel)] = text
	}
	bound := mutantstest.History{Root: root, Dir: root, Repository: repository, Working: files}
	h := Handler{Settings: &mutantstest.Settings{Value: mutants.Config{Selection: selport.Settings{Root: root}}}, Tree: seltest.TreeOf(root, relative, nil), History: bound,
		Presence: &mutantstest.SourceEdits{Files: files, Directories: map[string]bool{root: true}}, Dir: root}
	base := "base"
	q := SelectionQuery{Since: &base}
	same := func(s string) string { return s }
	main := filepath.Join(root, "main.go")
	sources, judge, renamed, _, _, err := h.selectSources(t.Context(), q)
	if err != nil || len(sources) != 2 || !judge(main, "example/binding#Value", "") || renamed(filepath.Join(root, "new.go")) != filepath.Join(root, "old.go") {
		t.Fatalf("actual bound since: %v %v", sources, err)
	}
	exceptions := []mutants.Exception{{File: "gone.go", Function: "example/binding#Gone", Hash: "independent obsolete hash"}}
	none := func(string) string { return "" }
	stale, _, err := h.elsewhere(t.Context(), q, root, exceptions, nil, none)
	if err != nil || len(stale) != 1 || stale[0].Why != mutants.ExceptionGone {
		t.Fatalf("bound deleted exception not judged: %v %v", stale, err)
	}
	orphans := []mutants.Orphan{{Path: filepath.Join(root, "old.go"), Why: mutants.OrphanGone}, {Path: filepath.Join(root, "staged.go"), Why: mutants.OrphanGone}}
	if remaining, err := h.notRenamed(t.Context(), q, orphans); err != nil || len(remaining) != 0 {
		t.Fatalf("bound committed+working rename ignored: %v %v", remaining, err)
	}
	t.Run("missing presence", func(t *testing.T) {
		absent := h
		absent.Presence = nil
		if _, err := absent.notRenamed(t.Context(), q, orphans); err == nil || !strings.Contains(err.Error(), "source presence binding is missing") {
			t.Fatalf("missing presence silently observed disk targets: %v", err)
		}
	})
	t.Run("wrong presence", func(t *testing.T) {
		// The independent fixture has neither rename target now.
		without := maps.Clone(files)
		for _, name := range []string{"new.go", "tracked.go"} {
			delete(without, filepath.Join(root, name))
		}
		wrong := h
		wrong.Presence = &mutantstest.SourceEdits{Files: without, Directories: map[string]bool{root: true}}
		if remaining, err := wrong.notRenamed(t.Context(), q, orphans); err != nil || len(remaining) != 2 {
			t.Fatalf("wrong presence retained rename targets: %v %v", remaining, err)
		}
	})
	h.History = nil
	if _, _, _, _, _, err := h.selectSources(t.Context(), q); err == nil || !strings.Contains(err.Error(), "binding") {
		t.Fatalf("missing since binding: %v", err)
	}
	if _, _, err := h.elsewhere(t.Context(), q, root, exceptions, nil, none); err == nil {
		t.Fatal("deleted exception silently read disk Git")
	}
	if _, err := h.notRenamed(t.Context(), q, orphans); err == nil {
		t.Fatal("rename silently read disk Git")
	}
	narrowed := q
	narrowed.Paths = []string{"main.go"}
	if stale, _, err := h.elsewhere(t.Context(), narrowed, root, exceptions, nil, none); err != nil || len(stale) != 0 {
		t.Fatal("explicit paths demanded unused deleted query")
	}
	wrong := mutantstest.NewHistoryRepository()
	wrong.Index = maps.Clone(repository.Commits[repository.Refs["base"]].Files)
	wrong.Commit("different independent base")
	wrong.Refs["base"] = wrong.Refs["main"]
	wrong.Commit("no changes or renames")
	h.History = mutantstest.History{Root: root, Dir: root, Repository: wrong, Working: files}
	if sources, _, _, _, _, err := h.selectSources(t.Context(), q); err != nil || len(sources) != 0 {
		t.Fatalf("wrong since binding retained correct changes: %v %v", sources, err)
	}
	if stale, _, err := h.elsewhere(t.Context(), q, root, exceptions, nil, none); err != nil || len(stale) != 0 {
		t.Fatal("wrong deleted binding retained stale exception")
	}
	// No staged move in the wrong independent tracked tree either.
	working := map[string]string{}
	for path, text := range wrong.Index {
		working[filepath.Join(root, path)] = text
	}
	h.History = mutantstest.History{Root: root, Dir: root, Repository: wrong, Working: working}
	if remaining, err := h.notRenamed(t.Context(), q, orphans); err != nil || len(remaining) != 2 {
		t.Fatalf("wrong rename binding retained exclusion: %v %v", remaining, err)
	}
	h.History = mutantstest.History{Dir: root}
	if remaining, err := h.notRenamed(t.Context(), q, orphans); err != nil || len(remaining) != 0 {
		t.Fatalf("ordinary unavailable Git judged or failed orphans: %v %v", remaining, err)
	}
	h.History = bound
	cause := errors.New("actual history consumer cancellation")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	// The UI words a range History could not give as its cause.
	if _, _, _, _, _, err := h.selectSources(ctx, q); !errors.Is(runError(err, same), cause) {
		t.Fatalf("since lost caller cause: %v", err)
	}
	if _, _, err := h.elsewhere(ctx, q, root, exceptions, nil, none); !errors.Is(err, cause) {
		t.Fatalf("deletions lost caller cause: %v", err)
	}
	if _, err := h.notRenamed(ctx, q, orphans); !errors.Is(err, cause) {
		t.Fatalf("orphan query suppressed caller cancellation: %v", err)
	}
}
