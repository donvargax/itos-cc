package main

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/config"
	mutation "github.com/donvargax/itos-cc/internal/mutants"
	mutantstest "github.com/donvargax/itos-cc/internal/mutants/porttest"
)

func historyConsumerFixture(t *testing.T) (*memory, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	m := newMemory(t)
	for path, text := range map[string]string{"go.mod": "module example/binding\n", "main.go": "package board\nfunc Value() int { return 1 }\n", "gone.go": "package board\nfunc Gone() int { return 7 }\n", "old.go": "package board\nfunc Old() int { return 8 }\n", "staged.go": "package board\nfunc Staged() int { return 9 }\n"} {
		if err := m.Write(filepath.Join(root, path), []byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"commit", "-qm", "independent base"}, {"tag", "base"}} {
		if err := m.Git(root, args...); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Write(filepath.Join(root, "main.go"), []byte("package board\nfunc Value() int { return 2 }\n")); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove(filepath.Join(root, "gone.go")); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"mv", "old.go", "new.go"}, {"add", "-A"}, {"commit", "-qm", "actual committed changes"}, {"mv", "staged.go", "tracked.go"}} {
		if err := m.Git(root, args...); err != nil {
			t.Fatal(err)
		}
	}
	return m, root
}

// These are actual legacy consumer functions, not alternate query callbacks.
// Config/discovery admission remains at its legacy boundary; history and lazy
// no-follow target evidence come from independent in-memory fixture state.
func TestMutationHistoryConsumersRequireActualBindings(t *testing.T) {
	m, root := historyConsumerFixture(t)
	bound := m.history(root)
	in := &invocation{cmd: mutationRunCommand, values: map[string][]string{"since": {"base"}}, adapters: adapters{History: bound, Presence: &mutantstest.SourceEdits{Files: m.files, Directories: m.dirs}}}
	changed, renamed, err := changedSince(t.Context(), in)
	main := filepath.Join(root, "main.go")
	if err != nil || len(changed) != 2 || !changed[main].Functions["example/binding#Value"] || renamed[filepath.Join(root, "new.go")] != filepath.Join(root, "old.go") {
		t.Fatalf("actual bound since: %v %v %v", changed, renamed, err)
	}
	head, err := m.Head(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if seed, err := sampleSeed(t.Context(), in); err != nil || seed != head {
		t.Fatalf("default seed not independent HEAD: %q %v", seed, err)
	}
	exceptions := []config.Exception{{File: "gone.go", Function: "example/binding#Gone", Hash: "independent obsolete hash"}}
	stale, _, err := exceptionsElsewhere(t.Context(), in, exceptions, nil, func(string) string { return "" })
	if err != nil || len(stale) != 1 || stale[0].Why != mutation.ExceptionGone {
		t.Fatalf("bound deleted exception not judged: %v %v", stale, err)
	}
	for _, path := range []string{"new.go", "tracked.go"} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(m.files[filepath.Join(root, path)]), 0600); err != nil {
			t.Fatal(err)
		}
	}
	orphans := []mutation.Orphan{{Path: filepath.Join(root, "old.go"), Why: mutation.OrphanGone}, {Path: filepath.Join(root, "staged.go"), Why: mutation.OrphanGone}}
	if remaining, err := notRenamed(t.Context(), in, orphans); err != nil || len(remaining) != 0 {
		t.Fatalf("bound committed+working rename ignored: %v %v", remaining, err)
	}
	t.Run("missing presence", func(t *testing.T) {
		absent := *in
		absent.adapters.Presence = nil
		if _, err := notRenamed(t.Context(), &absent, orphans); err == nil || !strings.Contains(err.Error(), "source presence binding is missing") {
			t.Fatalf("missing presence silently observed disk targets: %v", err)
		}
	})
	t.Run("wrong presence", func(t *testing.T) {
		// These physical targets remain on disk, so a hidden fallback would still
		// suppress the orphans. The independent fixture has neither target now.
		files := maps.Clone(m.files)
		for _, name := range []string{"new.go", "tracked.go"} {
			path := filepath.Join(root, name)
			if _, err := os.Lstat(path); err != nil {
				t.Fatalf("physical target decoy absent: %v", err)
			}
			delete(files, path)
		}
		wrong := *in
		wrong.adapters.Presence = &mutantstest.SourceEdits{Files: files, Directories: m.dirs}
		if remaining, err := notRenamed(t.Context(), &wrong, orphans); err != nil || len(remaining) != 2 {
			t.Fatalf("wrong presence retained physical rename targets: %v %v", remaining, err)
		}
	})
	in.adapters.History = nil
	if _, _, err := changedSince(t.Context(), in); err == nil || !strings.Contains(err.Error(), "binding") {
		t.Fatalf("missing since binding: %v", err)
	}
	if _, err := sampleSeed(t.Context(), in); err == nil {
		t.Fatal("default seed silently read disk HEAD")
	}
	if _, _, err := exceptionsElsewhere(t.Context(), in, exceptions, nil, func(string) string { return "" }); err == nil {
		t.Fatal("deleted exception silently read disk Git")
	}
	if _, err := notRenamed(t.Context(), in, orphans); err == nil {
		t.Fatal("rename silently read disk Git")
	}
	in.values["seed"] = []string{"explicit independent seed"}
	if seed, err := sampleSeed(t.Context(), in); err != nil || seed != "explicit independent seed" {
		t.Fatal("explicit seed demanded unused HEAD binding")
	}
	delete(in.values, "seed")
	in.args = []string{"main.go"}
	if stale, _, err := exceptionsElsewhere(t.Context(), in, exceptions, nil, func(string) string { return "" }); err != nil || len(stale) != 0 {
		t.Fatal("explicit paths demanded unused deleted query")
	}
	in.args = nil
	wrong := mutantstest.NewHistoryRepository()
	wrong.Index = maps.Clone(m.repos[root].Commits[m.repos[root].Refs["base"]].Files)
	wrong.Commit("different independent base")
	wrong.Refs["base"] = wrong.Refs["main"]
	wrong.Commit("no changes or renames")
	in.adapters.History = mutantstest.History{Root: root, Dir: root, Repository: wrong, Working: m.files}
	if changed, _, err := changedSince(t.Context(), in); err != nil || len(changed) != 0 {
		t.Fatalf("wrong since binding retained correct changes: %v %v", changed, err)
	}
	if seed, err := sampleSeed(t.Context(), in); err != nil || seed == head {
		t.Fatal("wrong HEAD binding retained real seed")
	}
	if stale, _, err := exceptionsElsewhere(t.Context(), in, exceptions, nil, func(string) string { return "" }); err != nil || len(stale) != 0 {
		t.Fatal("wrong deleted binding retained stale exception")
	}
	// No staged move in the wrong independent tracked tree either.
	working := map[string]string{}
	for path, text := range wrong.Index {
		working[filepath.Join(root, path)] = text
	}
	in.adapters.History = mutantstest.History{Root: root, Dir: root, Repository: wrong, Working: working}
	if remaining, err := notRenamed(t.Context(), in, orphans); err != nil || len(remaining) != 2 {
		t.Fatalf("wrong rename binding retained exclusion: %v %v", remaining, err)
	}
	in.adapters.History = mutantstest.History{Dir: root}
	if remaining, err := notRenamed(t.Context(), in, orphans); err != nil || len(remaining) != 0 {
		t.Fatalf("ordinary unavailable Git judged or failed orphans: %v %v", remaining, err)
	}
	in.adapters.History = bound
	cause := errors.New("actual history consumer cancellation")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	if _, _, err := changedSince(ctx, in); !errors.Is(err, cause) {
		t.Fatalf("since lost caller cause: %v", err)
	}
	if _, err := sampleSeed(ctx, in); !errors.Is(err, cause) {
		t.Fatalf("seed lost caller cause: %v", err)
	}
	if _, _, err := exceptionsElsewhere(ctx, in, exceptions, nil, func(string) string { return "" }); !errors.Is(err, cause) {
		t.Fatalf("deletions lost caller cause: %v", err)
	}
	if _, err := notRenamed(ctx, in, orphans); !errors.Is(err, cause) {
		t.Fatalf("orphan query suppressed caller cancellation: %v", err)
	}
}

// Driver.Head and --changed consume the same captured Git state; branch setup
// changes actual virtual bytes and never reads a sample's chosen seed.
func TestMemoryGitBranchesOwnHeadAndCommittedSelection(t *testing.T) {
	m := newMemory(t)
	root, err := m.Folder()
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "main.go")
	write := func(text string) {
		t.Helper()
		if err := m.Write(file, []byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	git := func(args ...string) {
		t.Helper()
		if err := m.Git(root, args...); err != nil {
			t.Fatal(err)
		}
	}
	write("package main\nfunc Value() int { return 1 }\n")
	git("init", "-q")
	git("add", "-A")
	git("commit", "-qm", "base")
	git("tag", "base")
	base, err := m.Head(t.Context(), root)
	if err != nil || len(base) != 40 {
		t.Fatalf("independent driver HEAD: %q %v", base, err)
	}
	git("checkout", "-qb", "work")
	write("package main\nfunc Value() int { return 2 }\n")
	git("commit", "-qam", "work edit")
	work, err := m.Head(t.Context(), root)
	if err != nil || work == base {
		t.Fatalf("committed branch HEAD did not change: %q %v", work, err)
	}
	git("checkout", "main")
	if text, err := m.Read(file); err != nil || string(text) != "package main\nfunc Value() int { return 1 }\n" {
		t.Fatalf("checkout did not restore actual source: %q %v", text, err)
	}
	if head, err := m.Head(t.Context(), root); err != nil || head != base {
		t.Fatal("branch switch changed independent base identity")
	}
	write("package main\nfunc Value() int { return 3 }\n")
	git("add", "main.go")
	_, repository := m.repoOf(root)
	if changed := m.changed(root, root, repository); len(changed) != 1 || changed[0] != "main.go" {
		t.Fatalf("staged source disappeared from actual changed selection: %v", changed)
	}
	if blob, failure := m.history(root).ReadCommitted(t.Context(), "work", "main.go"); failure != nil || string(blob) != "package main\nfunc Value() int { return 2 }\n" {
		t.Fatalf("later stage changed independent work commit: %q %v", blob, failure)
	}
}
