package main

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/features"
	"github.com/donvargax/itos-cc/internal/mutants"
	mutantstest "github.com/donvargax/itos-cc/internal/mutants/porttest"
	"maps"

	"github.com/donvargax/itos-cc/config"
)

// memoryConfig observes the actual current backing document on every read.
// The same config.Reader/Parse codec serves this source and the disk source.
type memoryConfig struct {
	memory *memory
	root   string
}

func (s memoryConfig) Read(context.Context) (string, []byte, error) {
	path := filepath.Join(s.root, config.File)
	data, err := s.memory.Read(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s.root, nil, nil
	}
	if err == nil {
		s.memory.readVersions[path]++
	}
	return s.root, data, err
}

// This direct driver guard reaches the moved handlers beside the scenarios of
// mutate.feature. A contradictory physical document detects fallback:
// the process works in the physical root, so project.Root and the disk config
// binding would find the invalid document there.
func TestMemoryMutationListCheckSampleAndExceptUseCurrentIndependentBytes(t *testing.T) {
	m := newMemory(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, config.File), []byte("exclude: [\"\"]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	for name, text := range map[string]string{"go.mod": "module memory.test/board\n", "board.go": "package board\nfunc Value(x int) bool { return x > 1 }\n", "board_test.go": "package board\n"} {
		if err := m.Write(filepath.Join(root, name), []byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Tests(root, features.Tests{Imports: map[string][]string{"board.go": {}, "board_test.go": {"board.go"}}}); err != nil {
		t.Fatal(err)
	}
	invoke := func(child string, flags ...string) features.Result {
		t.Helper()
		result, err := m.Run(features.Run{Dir: root, Args: append(append([]string{"mutation", child, "--json"}, flags...), "board.go")})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	if got := invoke("list"); got.Exit != 0 || !strings.Contains(compactRead(got.Stdout), `"function":"memory.test/board#Value"`) || strings.Contains(got.Stdout, "config.invalid") {
		t.Fatalf("memory list reached physical config or ignored source: %+v", got)
	}
	if got := invoke("check"); got.Exit != 1 || !strings.Contains(got.Stdout, "mutation.missing") || !strings.Contains(compactRead(got.Stdout), `"function":"memory.test/board#Value"`) {
		t.Fatalf("memory check ignored current source/model: %+v", got)
	}
	// The memory holds no repository, so the seed must be given; the bound
	// workers and snapshots hold no cached mutant to sample.
	if got := invoke("sample"); got.Exit != 82 || !strings.Contains(got.Stdout, "sample.no-git") || strings.Contains(got.Stdout, "config.invalid") {
		t.Fatalf("memory sample read a HEAD or the physical config: %+v", got)
	}
	if got := invoke("sample", "--seed", "fixed"); got.Exit != 0 || !strings.Contains(compactRead(got.Stdout), `"files":[]`) || !strings.Contains(compactRead(got.Stdout), `"seed":"fixed"`) || !strings.Contains(got.Stderr, "no cached mutant to sample") {
		t.Fatalf("memory sample ignored current source/model: %+v", got)
	}
	// The memory holds no snapshot, so its site is no survivor; with no
	// entry in the memory's settings, a renewal returns before any binding.
	except := func(args ...string) features.Result {
		t.Helper()
		result, err := m.Run(features.Run{Dir: root, Args: append([]string{"mutation", "except", "--json"}, args...)})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	if got := except("board.go:2:35", "--reason", "reviewed"); got.Exit != 80 || !strings.Contains(got.Stdout, "exception.no-survivor") || !strings.Contains(got.Stdout, "has no mutation results") {
		t.Fatalf("memory except read the physical config or ignored the memory's source: %+v", got)
	}
	if got := except("--renew"); got.Exit != 0 || !strings.Contains(compactRead(got.Stdout), `"renewed":[]`) || strings.Contains(got.Stdout, "config.invalid") {
		t.Fatalf("memory renew read the physical config: %+v", got)
	}
	// Once the memory holds a commit, an omitted seed is its HEAD, which the
	// physical folder, outside any repository, has none of.
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"commit", "-qm", "independent base"}} {
		if err := m.Git(root, args...); err != nil {
			t.Fatal(err)
		}
	}
	head, err := m.Head(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if got := invoke("sample"); got.Exit != 0 || !strings.Contains(compactRead(got.Stdout), `"seed":"`+head+`"`) {
		t.Fatalf("memory sample did not seed with the bound HEAD %s: %+v", head, got)
	}
	path := filepath.Join(root, config.File)
	if err := m.Write(path, []byte("exclude: [board.go]\n")); err != nil {
		t.Fatal(err)
	}
	if got := invoke("list"); got.Exit != 0 || !strings.Contains(compactRead(got.Stdout), `"sites":[]`) || !strings.Contains(got.Stderr, "1 sources left out by exclude") || !strings.Contains(got.Stderr, "no source files to list") {
		t.Fatalf("current memory exclusions not observed: %+v", got)
	}
	if got := invoke("sample", "--seed", "fixed"); got.Exit != 0 || !strings.Contains(compactRead(got.Stdout), `"files":[]`) || !strings.Contains(got.Stderr, "1 sources left out by exclude") || !strings.Contains(got.Stderr, "no source files to sample") {
		t.Fatalf("current memory exclusions not observed by sample: %+v", got)
	}
	if got := except("board.go:2:35", "--reason", "reviewed"); got.Exit != 80 || !strings.Contains(got.Stdout, "exception.excluded") {
		t.Fatalf("current memory exclusions not observed by except: %+v", got)
	}
	// An entry in the memory's settings is renewed or left through the
	// memory's bindings; with no results it is left, and nothing is written.
	entry := mutants.Exception{File: "board.go", Function: "memory.test/board#Value", Hash: "earlier", LineInFunction: 1, Column: 35, Original: ">", Replacement: ">=", Reason: "reviewed"}
	document, _, err := config.ExceptDocument([]byte("exclude: [vendor/**]\n"), entry)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Write(path, document); err != nil {
		t.Fatal(err)
	}
	if got := except("--renew"); got.Exit != 1 || !strings.Contains(got.Stdout, "mutation.exception-stale") || !strings.Contains(compactRead(got.Stdout), `"why":"no-results"`) {
		t.Fatalf("memory renew ignored the memory's entry: %+v", got)
	}
	if current, err := m.Read(path); err != nil || string(current) != string(document) {
		t.Fatalf("memory renew wrote what it left: %s %v", current, err)
	}
	if m.readVersions[path] == 0 {
		t.Fatal("settings byte observation was not bound")
	}
	if err := m.Write(path, []byte("exclude: [\"\"]\n")); err != nil {
		t.Fatal(err)
	}
	if got := except("board.go:2:35", "--reason", "reviewed"); got.Exit != 80 || !strings.Contains(got.Stdout, "config.invalid") || !strings.Contains(compactRead(got.Stdout), `"key":"exclude[0]"`) {
		t.Fatalf("current invalid memory document ignored by except: %+v", got)
	}
	for _, child := range []string{"list", "check", "sample"} {
		if got := invoke(child); got.Exit != 80 || !strings.Contains(got.Stdout, "config.invalid") || !strings.Contains(compactRead(got.Stdout), `"key":"exclude[0]"`) {
			t.Fatalf("current invalid memory document ignored by %s: %+v", child, got)
		}
	}
}

// The memory tree includes implicit parent directories, as Exists already does.
// SourceStatus/Presence consume those independent facts through their own ports.
func (m *memory) sourceEdits() *mutantstest.SourceEdits {
	dirs := maps.Clone(m.dirs)
	for path := range m.files {
		for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
			dirs[dir] = true
			if parent := filepath.Dir(dir); parent == dir {
				break
			}
		}
	}
	return &mutantstest.SourceEdits{Files: m.files, Directories: dirs, Write: m.Write}
}

func TestMemorySourceStatusIncludesActualImplicitDirectories(t *testing.T) {
	m := newMemory(t)
	root := t.TempDir()
	path := filepath.Join(root, "nested", "actual.txt")
	if err := m.Write(path, []byte("actual file")); err != nil {
		t.Fatal(err)
	}
	source := m.sourceEdits()
	if kind, failure := source.Kind(t.Context(), filepath.Dir(path)); failure != nil || kind != mutants.SourceDirectory {
		t.Fatalf("implicit directory: %v %v", kind, failure)
	}
	if kind, failure := source.Kind(t.Context(), path); failure != nil || kind != mutants.SourceNonDirectory {
		t.Fatalf("actual file: %v %v", kind, failure)
	}
	if present, failure := source.Present(t.Context(), filepath.Join(root, "absent")); failure != nil || present {
		t.Fatalf("invented directory: %v %v", present, failure)
	}
}

// The physical decoy: the process works in a root whose disk holds an invalid
// document, so a run that fell through to the host would read it
// (config.invalid on stdout) or rewrite it. A mutation run and except run in
// process against the memory, wherever --json is put, reading the memory's
// settings beside the same decoy, and help and parse errors of both answer
// as ever.
func TestMemoryRunsMutationRunInProcess(t *testing.T) {
	m := newMemory(t)
	root := t.TempDir()
	physical := filepath.Join(root, config.File)
	physicalBytes := []byte("exclude: [\"\"]\n")
	if err := os.WriteFile(physical, physicalBytes, 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	if err := m.Write(filepath.Join(root, "board.go"), []byte("package board\nfunc Value() int { return 1 }\n")); err != nil {
		t.Fatal(err)
	}
	if err := m.Tests(root, features.Tests{Language: "go", Command: "go test -count=1 -failfast ./.", Imports: map[string][]string{"board.go": {}}}); err != nil {
		t.Fatal(err)
	}
	for _, child := range []string{"run", "except"} {
		t.Run(child, func(t *testing.T) {
			if child == "run" {
				for _, args := range [][]string{{"mutation", child, "--json"}, {"--json", "mutation", child}, {"mutation", "--json", child, "--count", "1"}} {
					result, err := m.Run(features.Run{Dir: root, Args: args})
					if err != nil || result.Exit == 70 || strings.Contains(result.Stdout+result.Stderr, "config.invalid") {
						t.Fatalf("%q did not run in process against the memory: %+v %v", args, result, err)
					}
				}
			} else {
				// Except runs in process: a renewal reads the memory's
				// settings, which hold no entry, never the decoy.
				if result, err := m.Run(features.Run{Dir: root, Args: []string{"mutation", child, "--renew", "--json"}}); err != nil || result.Exit != 0 || strings.Contains(result.Stdout, "config.invalid") {
					t.Fatalf("moved execution refused or fell through to the host: %+v %v", result, err)
				}
			}
			result, err := m.Run(features.Run{Dir: root, Args: []string{"mutation", child, "--help"}})
			if err != nil || result.Exit != 0 || !strings.Contains(result.Stdout, "usage: itos-cc mutation "+child) {
				t.Fatalf("genuine help rejected: %+v %v", result, err)
			}
			result, err = m.Run(features.Run{Dir: root, Args: []string{"mutation", child, "--json", "--not-a-flag"}})
			if err != nil || result.Exit != 80 || !strings.Contains(result.Stdout, "flags.unknown") {
				t.Fatalf("genuine parse error rejected: %+v %v", result, err)
			}
		})
	}
	actual, err := os.ReadFile(physical)
	if err != nil || string(actual) != string(physicalBytes) {
		t.Fatalf("physical decoy changed: %s %v", actual, err)
	}
}
