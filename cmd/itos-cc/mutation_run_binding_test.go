package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/features"
	"github.com/donvargax/itos-cc/internal/mutants"
	mutantstest "github.com/donvargax/itos-cc/internal/mutants/porttest"
	seltest "github.com/donvargax/itos-cc/internal/selection/port/porttest"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// Only the unmoved admission/config boundary uses this disposable disk fixture.
// All execution, source edits and model publication below use actual bound fakes.
type mutationRunFixture struct {
	root           string
	memory         *memory
	adapters       adapters
	snapshots      *mutantstest.Snapshots
	edits          *mutantstest.SourceEdits
	workers        *mutantstest.Workers
	stdout, stderr bytes.Buffer
}

func newMutationRunFixture(t *testing.T) *mutationRunFixture {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	f := &mutationRunFixture{root: root, memory: newMemory(t)}
	relative := map[string]string{"go.mod": "module example.test/board\n", "board.go": "package board\nfunc Value(x int) bool { return x > 1 }\n", "board_test.go": "package board\n"}
	for name, text := range relative {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		if err := f.memory.Write(path, []byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.memory.Tests(root, features.Tests{Language: "go", Command: "go version", Imports: map[string][]string{"board.go": {}, "board_test.go": {"board.go"}}}); err != nil {
		t.Fatal(err)
	}
	inputs := f.memory.inputs(features.Run{Dir: root})
	f.workers = f.memory.workers()
	f.snapshots = &mutantstest.Snapshots{RootDir: root}
	f.edits = &mutantstest.SourceEdits{Files: f.memory.files, Write: f.memory.Write}
	f.adapters = diskAdapters(&f.stderr, nil)
	f.adapters.Snapshots, f.adapters.SourceEdits = f.snapshots, f.edits
	f.adapters.Workers, f.adapters.Toolchain, f.adapters.Fingerprints, f.adapters.Reach = f.workers, inputs, inputs, inputs
	f.adapters.Tree, f.adapters.Surroundings = seltest.TreeOf(root, relative, nil), inputs
	return f
}
func (f *mutationRunFixture) invoke(t *testing.T, a adapters, flags ...string) (int, string) {
	t.Helper()
	f.stdout.Reset()
	f.stderr.Reset()
	args := append([]string{"mutation", "run", "--json", "--test-command", "go version"}, flags...)
	args = append(args, "board.go")
	code := 0
	out, log := captured(t, func() { code = run(t.Context(), args, strings.NewReader(""), &f.stdout, &f.stderr, a) })
	return code, out + f.stdout.String() + log + f.stderr.String()
}
func TestMutationRunUsesActualModelAndSourceBindings(t *testing.T) {
	f := newMutationRunFixture(t)
	if code, out := f.invoke(t, f.adapters, "--no-coverage"); code != 0 {
		t.Fatalf("bound run: %d %s", code, out)
	}
	path := filepath.Join(f.root, "board.go")
	if len(f.snapshots.Written) != 1 || len(f.edits.Writes) != 1 || len(f.workers.Environments) == 0 || !strings.Contains(f.memory.files[path], "itos-cc mutation:") {
		t.Fatalf("actual bound effects missing: snapshots%v edits%v", f.snapshots.Written, f.edits.Writes)
	}
	disk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(disk), "itos-cc mutation:") {
		t.Fatal("source binding fell back to disk")
	}
	if _, err := os.Stat(filepath.Join(f.root, ".metrics", "mutate", "board.go.json")); !os.IsNotExist(err) {
		t.Fatalf("snapshot binding fell back to disk: %v", err)
	}
	for _, binding := range []struct {
		name, token string
		change      func(*adapters)
	}{
		{"missing snapshots", "snapshots binding is missing", func(a *adapters) { a.Snapshots = nil }},
		{"missing edits", "source edits binding is missing", func(a *adapters) { a.SourceEdits = nil }},
		{"missing tree", "source tree binding is missing", func(a *adapters) { a.Tree = nil }},
		{"missing surroundings", "source tree binding is missing", func(a *adapters) { a.Surroundings = nil }},
		{"wrong snapshots", "snapshot unavailable", func(a *adapters) {
			a.Snapshots = &mutantstest.Snapshots{RootFailure: &snapport.Unreadable{Reason: "snapshot unavailable"}}
		}},
		{"wrong edits", "annotation source is absent", func(a *adapters) { a.SourceEdits = &mutantstest.SourceEdits{Files: map[string]string{}} }},
		// The selection reads the bound tree too (T-100): it lists board.go,
		// whose bytes that tree does not hold, while the disk does.
		{"wrong tree", "fake", func(a *adapters) {
			tree := seltest.TreeOf(f.root, map[string]string{"go.mod": "module example.test/board\n", "board.go": "package board\nfunc Value(x int) bool { return x > 1 }\n", "board_test.go": "package board\n"}, nil)
			delete(tree.Contents, "board.go")
			a.Tree = tree
		}},
		{"missing settings", "settings binding is missing", func(a *adapters) { a.Settings = nil }},
		{"wrong settings", "independent settings unavailable", func(a *adapters) {
			a.Settings = &mutantstest.Settings{Failure: mutants.InputFailed{Cause: errors.New("independent settings unavailable")}}
		}},
	} {
		t.Run(binding.name, func(t *testing.T) {
			a := f.adapters
			binding.change(&a)
			if code, out := f.invoke(t, a, "--no-coverage", "--mutate-all"); code == 0 || !strings.Contains(out, binding.token) {
				t.Fatalf("binding silently escaped: %d %s", code, out)
			}
		})
	}
}
func TestMutationRunCoverageCommandFallbackIsLazyAndPresented(t *testing.T) {
	f := newMutationRunFixture(t)
	if code, out := f.invoke(t, f.adapters, "--coverage-command", "actual unused command"); code != 0 || !strings.Contains(out, "coverage: --coverage-command needs --coverage-report") {
		t.Fatalf("ordinary fallback presentation: %d %s", code, out)
	}
	if len(f.snapshots.Written) != 1 || len(f.workers.Environments) == 0 {
		t.Fatal("fallback did not judge and publish")
	}
	before := len(f.workers.Environments)
	if code, out := f.invoke(t, f.adapters, "--coverage-command", "actual unused command"); code != 0 || strings.Contains(out, "coverage: --coverage-command") {
		t.Fatalf("reused run measured unnecessarily: %d %s", code, out)
	}
	if len(f.workers.Environments) != before {
		t.Fatal("reused result entered a worker")
	}
}
func TestMutationRunListingFailureUsesBoundListerBeforeTrials(t *testing.T) {
	f := newMutationRunFixture(t)
	config := `mutation:
  tests:
    list: actual-list
    run: selected {pattern}
    ids_pattern: "{ids}"
    join:
      each: "{id}"
      sep: ","
`
	if err := os.WriteFile(filepath.Join(f.root, "itos-cc.yaml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	lister := &mutantstest.Lister{Listings: map[string]map[string]mutantstest.Listing{f.root: {"actual-list": {Stdout: "independent\tboard_test.go\n", Stderr: "actual list diagnostic\n", ExitCode: 7, Reason: "exit status 7"}}}, Log: &f.stderr}
	// A shell override opts out of configured listing; invoke the actual listed policy.
	invoke := func(a adapters) (int, string) {
		f.stdout.Reset()
		f.stderr.Reset()
		code := 0
		out, log := captured(t, func() {
			code = run(t.Context(), []string{"mutation", "run", "--json", "--no-annotate", "board.go"}, strings.NewReader(""), &f.stdout, &f.stderr, a)
		})
		return code, out + f.stdout.String() + log + f.stderr.String()
	}
	f.adapters.TestLister = lister
	if code, out := invoke(f.adapters); code != 1 || !strings.Contains(out, "tests.list-failed") || !strings.Contains(out, "exit status 7") || !strings.Contains(out, "actual list diagnostic") {
		t.Fatalf("list failure classification: %d %s", code, out)
	}
	if len(f.workers.Environments) != 0 || len(f.snapshots.Written) != 0 {
		t.Fatal("listing failure entered trials/publication")
	}
	for _, wrong := range []bool{false, true} {
		a := f.adapters
		token := "mutation TestLister binding is missing"
		if wrong {
			a.TestLister = &mutantstest.Lister{}
			token = "no declared listing command"
		} else {
			a.TestLister = nil
		}
		if code, out := invoke(a); code != 100 || !strings.Contains(out, token) {
			t.Fatalf("missing/wrong actual listing: %d %s", code, out)
		}
	}
}
