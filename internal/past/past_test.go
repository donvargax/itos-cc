package past_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/internal/past"
	"github.com/donvargax/itos-cc/internal/past/porttest"
)

var root = filepath.FromSlash("/repo")

// A path is named from the project root and back, in any worktree; one the
// root cannot reach stays as it is.
func TestRelAndAbsNameFilesFromTheRoot(t *testing.T) {
	c := past.Commit{Root: root, Dir: filepath.Join(root, "sub")}
	if got := c.Rel(filepath.Join(root, "sub", "a.go")); got != "sub/a.go" {
		t.Errorf("Rel = %q, want sub/a.go", got)
	}
	if got := c.Abs("sub/a.go"); got != filepath.Join(root, "sub", "a.go") {
		t.Errorf("Abs = %q", got)
	}
	outside := filepath.FromSlash("/elsewhere/a.go")
	if got := c.Rel(outside); got != "/elsewhere/a.go" {
		t.Errorf("Rel of a path outside = %q, want it as it is", got)
	}
	if got := c.Abs(outside); got != outside {
		t.Errorf("Abs of an absolute path = %q, want it as it is", got)
	}
}

// A question's key follows what changes its answer, and nothing else: the
// same question in another worktree, at another place of the same
// project, has the same key, a different one does not.
func TestKeyFollowsTheQuestion(t *testing.T) {
	type q struct{ Paths []string }
	here := past.Commit{Root: root, Dir: filepath.Join(root, "sub")}
	moved := past.Commit{Root: filepath.FromSlash("/other-worktree"), Dir: filepath.FromSlash("/other-worktree/sub")}
	if here.Key(q{[]string{"a"}}, "1") != moved.Key(q{[]string{"a"}}, "1") {
		t.Errorf("the same question has two keys across worktrees")
	}
	for name, other := range map[string]string{
		"paths":   moved.Key(q{[]string{"b"}}, "1"),
		"version": moved.Key(q{[]string{"a"}}, "2"),
		"place":   (past.Commit{Root: root, Dir: root}).Key(q{[]string{"a"}}, "1"),
	} {
		if other == here.Key(q{[]string{"a"}}, "1") {
			t.Errorf("a different %s has the same key", name)
		}
	}
	// A working directory the root cannot be related to keys by its own name.
	apart := past.Commit{Root: root, Dir: "relative"}
	again := past.Commit{Root: root, Dir: "relative"}
	if apart.Key(q{[]string{"a"}}, "1") == here.Key(q{[]string{"a"}}, "1") || apart.Key(q{[]string{"a"}}, "1") != again.Key(q{[]string{"a"}}, "1") {
		t.Errorf("a directory apart from the root should have a key of its own, the same each time")
	}
	if (past.Commit{Root: root, Dir: root}).Key(func() {}, "1") == "" {
		t.Errorf("a query that cannot be encoded still has a key")
	}
}

// Kept measures once, keeps the answer, serves it, and measures again what
// it cannot read back; what fails to measure is kept nowhere.
func TestKeptMeasuresOnceAndServesTheKeptAnswer(t *testing.T) {
	results := &porttest.Results{}
	c := past.Commit{ID: "abc", Results: results}
	measured := 0
	compute := func() (int, error) { measured++; return 7, nil }
	freeze := func(n int) ([]byte, error) { return []byte{byte(n)}, nil }
	thaw := func(b []byte) (int, error) {
		if len(b) != 1 {
			return 0, errors.New("corrupt")
		}
		return int(b[0]), nil
	}
	for range 2 {
		if got, err := past.Kept(t.Context(), c, "cmd", "k", compute, freeze, thaw); err != nil || got != 7 {
			t.Fatalf("Kept = %d, %v", got, err)
		}
	}
	if measured != 1 || results.Puts != 1 {
		t.Errorf("measured %d times and kept %d, want once each", measured, results.Puts)
	}
	results.Held["abc/cmd-k"] = []byte("garbage")
	if got, _ := past.Kept(t.Context(), c, "cmd", "k", compute, freeze, thaw); got != 7 || measured != 2 || len(results.Held["abc/cmd-k"]) != 1 {
		t.Errorf("a corrupt answer: %d, measured %d, kept %q; want it measured again and kept whole", got, measured, results.Held["abc/cmd-k"])
	}
	failing := func() (int, error) { return 0, errors.New("no") }
	if _, err := past.Kept(t.Context(), c, "cmd", "other", failing, freeze, thaw); err == nil || results.Puts != 2 {
		t.Errorf("a failed measure: %v, kept %d; want its error and nothing kept", err, results.Puts)
	}
	unfreezable := func(int) ([]byte, error) { return nil, errors.New("no") }
	if got, err := past.Kept(t.Context(), c, "cmd", "third", compute, unfreezable, thaw); err != nil || got != 7 || results.Puts != 2 {
		t.Errorf("an answer that cannot be frozen: %d, %v, kept %d; want it given, not kept", got, err, results.Puts)
	}
	if got, err := past.Kept(t.Context(), past.Commit{ID: "abc"}, "cmd", "k", compute, freeze, thaw); err != nil || got != 7 {
		t.Errorf("a commit with no results to keep: %d, %v", got, err)
	}
}

// The failures say what the person did, the Nothing store holds nothing.
func TestFailuresAndTheStoreOfNothing(t *testing.T) {
	var f past.Failure = &past.BadRev{Rev: "x"}
	if got, ok := past.IsFailure(f); !ok || got.Error() != "x names no commit of this repository" {
		t.Errorf("BadRev: %v", got)
	}
	if got, ok := past.IsFailure(&past.NoRepository{Reason: "r"}); !ok || got.Error() != "no git repository: r" {
		t.Errorf("NoRepository: %v", got)
	}
	if _, ok := past.IsFailure(errors.New("plain")); ok {
		t.Errorf("a plain error is no failure of past")
	}
	s := past.Nothing{RootDir: root}
	if got, f := s.Root(t.Context()); got != root || f != nil {
		t.Errorf("Root = %q, %v", got, f)
	}
	var v any
	if found, f := s.Read(t.Context(), "dry.json", &v); found || f != nil {
		t.Errorf("Read = %v, %v; want nothing found", found, f)
	}
	if f := s.Write(t.Context(), "dry.json", 1); f != nil {
		t.Errorf("Write = %v", f)
	}
	if f := s.Remove(t.Context(), "dry.json"); f != nil {
		t.Errorf("Remove = %v", f)
	}
	if names, f := s.List(t.Context(), ""); len(names) != 0 || f != nil {
		t.Errorf("List = %v, %v", names, f)
	}
}

// The fake source opens what it holds and refuses what it does not.
func TestTheFakeSourceOpensWhatItHolds(t *testing.T) {
	s := &porttest.Source{Commits: map[string]past.Commit{"a": {ID: "1"}}, Results: &porttest.Results{}}
	if c, err := s.Open(t.Context(), "a"); err != nil || c.ID != "1" || c.Results == nil || s.Opened != 1 {
		t.Errorf("Open(a) = %+v, %v", c, err)
	}
	var bad *past.BadRev
	if _, err := s.Open(t.Context(), "b"); !errors.As(err, &bad) {
		t.Errorf("Open(b) = %v, want BadRev", err)
	}
}
