package porttest

import (
	"context"
	"errors"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/lang"
	langtest "github.com/donvargax/itos-cc/internal/lang/port/porttest"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// HistoryFixture supplies actual setup operations, not scripted query answers.
// Put names root-relative files; Git executes setup at Root. At changes only the
// bound caller directory, never process cwd. Outside has no repository.
type HistoryFixture struct {
	Root    string
	At      func(string) mutants.History
	Outside mutants.History
	Put     func(context.Context, string, string) error
	Remove  func(context.Context, string) error
	Git     func(context.Context, ...string) error
}

func historyPut(t *testing.T, b HistoryFixture, path, text string) {
	t.Helper()
	if err := b.Put(t.Context(), path, text); err != nil {
		t.Fatal(err)
	}
}
func historyGit(t *testing.T, b HistoryFixture, args ...string) {
	t.Helper()
	if err := b.Git(t.Context(), args...); err != nil {
		t.Fatal(err)
	}
}
func historyCommit(t *testing.T, b HistoryFixture, files map[string]string) {
	t.Helper()
	for p, text := range files {
		historyPut(t, b, p, text)
	}
	historyGit(t, b, "add", "-A")
	historyGit(t, b, "commit", "-qm", "actual fixture")
}

// HistoryContract checks immutable blobs, branch merge bases, changed functions,
// both rename directions, tracked working moves, caller paths/namespaces and
// each operation's cancellation. Physical Git/export guards remain at adapters.
func HistoryContract(t *testing.T, open func(*testing.T) HistoryFixture) {
	t.Helper()
	t.Run("immutable committed bytes and independent HEAD", func(t *testing.T) {
		b := open(t)
		h := b.At(b.Root)
		if _, failure := h.Head(t.Context()); failure == nil {
			t.Fatal("unborn repository supplied HEAD")
		}
		historyCommit(t, b, map[string]string{"go.mod": "module example/history\n", "sub/value.go": "package sub\nfunc Value() int { return 1 }\n"})
		head, failure := h.Head(t.Context())
		if failure != nil || len(head) != 40 {
			t.Fatalf("HEAD identity: %q %v", head, failure)
		}
		historyGit(t, b, "tag", "base")
		if id, failure := h.Resolve(t.Context(), "base"); failure != nil || id != head {
			t.Fatalf("independent tagged HEAD %q %v", id, failure)
		}
		historyPut(t, b, "sub/value.go", "package sub\nfunc Value() int { return 9 }\n")
		historyGit(t, b, "add", "sub/value.go")
		blob, failure := b.At(filepath.Join(b.Root, "sub")).ReadCommitted(t.Context(), head, "sub/value.go")
		want := "package sub\nfunc Value() int { return 1 }\n"
		if failure != nil || string(blob) != want {
			t.Fatalf("committed bytes changed with stage: %q %v", blob, failure)
		}
		blob[0] = 'X'
		again, failure := h.ReadCommitted(t.Context(), "base", "sub/value.go")
		if failure != nil || string(again) != want {
			t.Fatalf("caller changed immutable blob: %q %v", again, failure)
		}
		if id, failure := h.Head(t.Context()); failure != nil || id != head {
			t.Fatal("staging changed HEAD identity")
		}
	})

	t.Run("commit all preserves tracked staging without adding untracked files", func(t *testing.T) {
		b := open(t)
		historyCommit(t, b, map[string]string{"tracked.py": "def tracked(): return 1\n", "deleted.py": "def deleted(): return 1\n"})
		historyPut(t, b, "tracked.py", "def tracked(): return 2\n")
		historyPut(t, b, "staged.py", "def staged(): return 3\n")
		historyGit(t, b, "add", "staged.py")
		historyPut(t, b, "untracked.py", "def untracked(): return 4\n")
		if err := b.Remove(t.Context(), "deleted.py"); err != nil {
			t.Fatal(err)
		}
		historyGit(t, b, "commit", "-qam", "tracked only")
		historyGit(t, b, "tag", "current")
		historyGit(t, b, "tag", "-f", "current", "HEAD~1")
		if current, failure := b.At(b.Root).Resolve(t.Context(), "current"); failure != nil {
			t.Fatal(failure)
		} else if previous, failure := b.At(b.Root).Resolve(t.Context(), "HEAD~1"); failure != nil || current != previous {
			t.Fatalf("forced ref not observed independently: %q %q %v", current, previous, failure)
		}
		h := b.At(b.Root)
		if blob, failure := h.ReadCommitted(t.Context(), "HEAD", "tracked.py"); failure != nil || string(blob) != "def tracked(): return 2\n" {
			t.Fatalf("tracked edit omitted: %q %v", blob, failure)
		}
		if _, failure := h.ReadCommitted(t.Context(), "HEAD", "staged.py"); failure != nil {
			t.Fatalf("explicit staged addition lost: %v", failure)
		}
		for _, path := range []string{"untracked.py", "deleted.py"} {
			if _, failure := h.ReadCommitted(t.Context(), "HEAD", path); failure == nil {
				t.Fatalf("commit -a invented/retained %s", path)
			}
		}
	})

	t.Run("checkout of a tag detaches and leaves immutable ref", func(t *testing.T) {
		b := open(t)
		original := "def value(): return 1\n"
		historyCommit(t, b, map[string]string{"value.py": original})
		historyGit(t, b, "tag", "base")
		h := b.At(b.Root)
		base, failure := h.Resolve(t.Context(), "base")
		if failure != nil {
			t.Fatal(failure)
		}
		historyGit(t, b, "checkout", "-q", "base")
		historyPut(t, b, "value.py", "def value(): return 2\n")
		historyGit(t, b, "commit", "-qam", "detached actual edit")
		head, failure := h.Head(t.Context())
		if failure != nil || head == base {
			t.Fatalf("detached HEAD did not advance: %q %v", head, failure)
		}
		if tagged, failure := h.Resolve(t.Context(), "base"); failure != nil || tagged != base {
			t.Fatalf("detached commit moved tag: %q %v", tagged, failure)
		}
		if blob, failure := h.ReadCommitted(t.Context(), "base", "value.py"); failure != nil || string(blob) != original {
			t.Fatalf("detached commit changed tagged bytes: %q %v", blob, failure)
		}
		if blob, failure := h.ReadCommitted(t.Context(), "HEAD", "value.py"); failure != nil || string(blob) != "def value(): return 2\n" {
			t.Fatalf("detached commit lost actual new bytes: %q %v", blob, failure)
		}
		historyGit(t, b, "checkout", "main")
		if current, failure := h.Head(t.Context()); failure != nil || current != base {
			t.Fatalf("detached commit moved branch: %q %v", current, failure)
		}
	})
	t.Run("branch three-dot uses HEAD bytes and caller namespace", func(t *testing.T) {
		b := open(t)
		base := "package sub\nfunc First() int { return 1 }\nfunc Second() int { return 2 }\n"
		historyCommit(t, b, map[string]string{"go.mod": "module example/history\n", "sub/value.go": base, "top.go": "package top\nfunc Top() int { return 1 }\n"})
		historyGit(t, b, "branch", "base")
		historyGit(t, b, "checkout", "-qb", "work")
		historyCommit(t, b, map[string]string{"sub/value.go": strings.Replace(base, "return 2", "return 3", 1)})
		historyGit(t, b, "checkout", "main")
		historyCommit(t, b, map[string]string{"sub/value.go": strings.Replace(base, "return 1", "return 9", 1), "top.go": "package top\nfunc Top() int { return 9 }\n"})
		historyGit(t, b, "checkout", "work")
		historyPut(t, b, "sub/value.go", "// uncommitted lines\n"+strings.Replace(base, "return 1", "return 8", 1))
		h := b.At(filepath.Join(b.Root, "sub"))
		changed, renamed, failure := h.ChangedSince(t.Context(), "main")
		path := filepath.Join(b.Root, "sub", "value.go")
		if failure != nil || len(changed) != 1 || len(renamed) != 0 {
			t.Fatalf("caller range population: %v %v %v", changed, renamed, failure)
		}
		c := changed[path]
		if !reflect.DeepEqual(c.Functions, map[string]bool{"example/history/sub#Second": true}) || !reflect.DeepEqual(c.Lines, []mutants.LineRange{{Start: 3, End: 3}}) {
			t.Fatalf("three-dot/HEAD namespace/ranges: %+v", c)
		}
	})
	t.Run("rename directions unchanged and edited and deletions", func(t *testing.T) {
		b := open(t)
		original := "def first(x):\n    return x > 0\n\ndef second(x):\n    return x > 1\n"
		historyCommit(t, b, map[string]string{"sub/old.py": strings.ReplaceAll(original, "first", "nested_first"), "other.py": original, "gone.py": "def deleted():\n    return 'gone'\n"})
		historyGit(t, b, "tag", "base")
		historyGit(t, b, "mv", "sub/old.py", "sub/new.py")
		historyGit(t, b, "mv", "other.py", "edited.py")
		historyPut(t, b, "edited.py", strings.Replace(original, "x > 1", "x > 2", 1))
		if err := b.Remove(t.Context(), "gone.py"); err != nil {
			t.Fatal(err)
		}
		historyGit(t, b, "add", "-A")
		historyGit(t, b, "commit", "-qm", "actual renames and deletion")
		changed, prior, failure := b.At(filepath.Join(b.Root, "sub")).ChangedSince(t.Context(), "base")
		old, next := filepath.Join(b.Root, "sub", "old.py"), filepath.Join(b.Root, "sub", "new.py")
		if failure != nil || len(changed) != 1 || len(changed[next].Functions) != 0 || !maps.Equal(prior, map[string]string{next: old}) {
			t.Fatalf("unchanged caller rename: %v %v %v", changed, prior, failure)
		}
		whole, failure := b.At(filepath.Join(b.Root, "sub")).RenamesSince(t.Context(), "base")
		want := map[string]string{old: next, filepath.Join(b.Root, "other.py"): filepath.Join(b.Root, "edited.py")}
		if failure != nil || !maps.Equal(whole, want) {
			t.Fatalf("whole-repository prior->current: %v %v", whole, failure)
		}
		all, _, failure := b.At(b.Root).ChangedSince(t.Context(), "base")
		if failure != nil || len(all[filepath.Join(b.Root, "edited.py")].Functions) != 1 {
			t.Fatalf("edited rename lost actual change: %v %v", all, failure)
		}
		deleted, failure := b.At(b.Root).DeletedSince(t.Context(), "base")
		if failure != nil || !slices.Equal(deleted, []string{filepath.Join(b.Root, "gone.py")}) {
			t.Fatalf("renames became deletions: %v %v", deleted, failure)
		}
	})
	t.Run("tracked working rename excludes untracked move", func(t *testing.T) {
		b := open(t)
		original := "def value(x):\n    return x > 0\n"
		historyCommit(t, b, map[string]string{"old.py": original})
		if err := b.Remove(t.Context(), "old.py"); err != nil {
			t.Fatal(err)
		}
		historyPut(t, b, "new.py", original)
		h := b.At(b.Root)
		moves, failure := h.RenamesUncommitted(t.Context())
		if failure != nil || len(moves) != 0 {
			t.Fatalf("untracked move recognized: %v %v", moves, failure)
		}
		historyGit(t, b, "add", "new.py")
		moves, failure = h.RenamesUncommitted(t.Context())
		if failure != nil || !maps.Equal(moves, map[string]string{filepath.Join(b.Root, "old.py"): filepath.Join(b.Root, "new.py")}) {
			t.Fatalf("tracked working move lost: %v %v", moves, failure)
		}
	})
	t.Run("pure deletion and same-name kept hashes", func(t *testing.T) {
		b := open(t)
		source := "package sample\nfunc init() { println(1) }\nfunc init() { println(2) }\nfunc Value() int {\n println(0)\n return 1\n}\n"
		historyCommit(t, b, map[string]string{"go.mod": "module example/history\n", "same.go": source})
		historyGit(t, b, "tag", "base")
		after := strings.Replace(strings.Replace(source, "println(2)", "println(3)", 1), " println(0)\n", "", 1)
		historyCommit(t, b, map[string]string{"same.go": after})
		changed, _, failure := b.At(b.Root).ChangedSince(t.Context(), "base")
		if failure != nil {
			t.Fatal(failure)
		}
		c := changed[filepath.Join(b.Root, "same.go")]
		if !c.Functions["example/history#init"] || !c.Functions["example/history#Value"] || len(c.Kept["example/history#init"]) != 1 || !c.Changes(3, 3) || c.Changes(4, 6) {
			t.Fatalf("deletion versus changed lines/kept init: %+v", c)
		}
		path := filepath.Join(b.Root, "same.go")
		parsed, err := lang.Parse(t.Context(), langtest.Surroundings{Dir: b.Root, Files: map[string]string{filepath.Join(b.Root, "go.mod"): "module example/history\n", path: after}}, lang.Detect(path), path, []byte(after))
		if err != nil {
			t.Fatal(err)
		}
		defer parsed.Close()
		var initHashes []string
		for _, unit := range parsed.Units {
			if unit.Name == "init" {
				initHashes = append(initHashes, mutants.UnitHash(parsed, unit))
			}
		}
		if len(initHashes) != 2 || initHashes[0] == initHashes[1] || c.Judges("example/history#init", initHashes[0]) || !c.Judges("example/history#init", initHashes[1]) || !c.Kept["example/history#init"][initHashes[0]] {
			t.Fatalf("same-name hashes mispaired: hashes=%v changes=%+v", initHashes, c)
		}

	})
	t.Run("a change to a function's comments alone is no change", func(t *testing.T) {
		b := open(t)
		source := "package sample\nfunc A() int {\n // one\n return 1\n}\nfunc B() int {\n return 2\n}\n"
		historyCommit(t, b, map[string]string{"go.mod": "module example/history\n", "comments.go": source})
		historyGit(t, b, "tag", "base")
		after := strings.Replace(strings.Replace(source, "// one", "// the first", 1), "return 2", "return 3 // changed", 1)
		historyCommit(t, b, map[string]string{"comments.go": after})
		changed, _, failure := b.At(b.Root).ChangedSince(t.Context(), "base")
		if failure != nil {
			t.Fatal(failure)
		}
		c := changed[filepath.Join(b.Root, "comments.go")]
		if c.Functions["example/history#A"] || !c.Functions["example/history#B"] || c.Changes(2, 5) || !c.Changes(7, 7) {
			t.Fatalf("comment-only versus code change: %+v", c)
		}
	})
	t.Run("typed failures and caller cause on every operation", func(t *testing.T) {
		b := open(t)
		historyCommit(t, b, map[string]string{"a.py": "def a(): return 1\n"})
		h := b.At(b.Root)
		var missing mutants.NoRepository
		if _, failure := b.Outside.Head(t.Context()); !errors.As(failure, &missing) || missing.Reason == "" {
			t.Fatalf("no repository: %v", failure)
		}
		var bad mutants.BadReference
		if _, _, failure := h.ChangedSince(t.Context(), "--not-a-ref"); !errors.As(failure, &bad) {
			t.Fatalf("bad ref: %v", failure)
		}
		var queryFailed mutants.HistoryFailed
		if _, failure := h.ReadCommitted(t.Context(), "HEAD", "missing.py"); !errors.As(failure, &queryFailed) {
			t.Fatalf("missing committed blob failure kind: %v", failure)
		}
		cause := errors.New("actual history caller stopped")
		ctx, cancel := context.WithCancelCause(t.Context())
		cancel(cause)
		checks := []func(context.Context) error{
			func(ctx context.Context) error { _, e := h.Head(ctx); return e }, func(ctx context.Context) error { _, e := h.Resolve(ctx, "HEAD"); return e }, func(ctx context.Context) error { _, e := h.ReadCommitted(ctx, "HEAD", "a.py"); return e },
			func(ctx context.Context) error { _, _, e := h.ChangedSince(ctx, "HEAD"); return e }, func(ctx context.Context) error { _, e := h.DeletedSince(ctx, "HEAD"); return e }, func(ctx context.Context) error { _, e := h.RenamesSince(ctx, "HEAD"); return e }, func(ctx context.Context) error { _, e := h.RenamesUncommitted(ctx); return e },
		}
		for i, check := range checks {
			failure := check(ctx)
			var stopped mutants.HistoryCancelled
			if !errors.As(failure, &stopped) || !errors.Is(failure, cause) {
				t.Errorf("operation %d lost actual cause: %v", i, failure)
			}
		}
	})
}
