package main

import (
	"path/filepath"
	"testing"
)

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
