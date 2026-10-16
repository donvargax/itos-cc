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

// This direct driver guard reaches the moved handlers before mutate.feature is
// admitted wholesale at T100. A contradictory physical document detects fallback.
func TestMemoryMutationListAndCheckUseCurrentIndependentBytes(t *testing.T) {
	m := newMemory(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, config.File), []byte("exclude: [\"\"]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{"go.mod": "module memory.test/board\n", "board.go": "package board\nfunc Value(x int) bool { return x > 1 }\n", "board_test.go": "package board\n"} {
		if err := m.Write(filepath.Join(root, name), []byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Tests(root, features.Tests{Imports: map[string][]string{"board.go": {}, "board_test.go": {"board.go"}}}); err != nil {
		t.Fatal(err)
	}
	invoke := func(child string) features.Result {
		t.Helper()
		result, err := m.Run(features.Run{Dir: root, Args: []string{"mutation", child, "--json", "board.go"}})
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
	path := filepath.Join(root, config.File)
	if err := m.Write(path, []byte("exclude: [board.go]\n")); err != nil {
		t.Fatal(err)
	}
	if got := invoke("list"); got.Exit != 0 || !strings.Contains(compactRead(got.Stdout), `"sites":[]`) || !strings.Contains(got.Stderr, "1 sources left out by exclude") || !strings.Contains(got.Stderr, "no source files to list") {
		t.Fatalf("current memory exclusions not observed: %+v", got)
	}
	if m.readVersions[path] == 0 {
		t.Fatal("settings byte observation was not bound")
	}
	if err := m.Write(path, []byte("exclude: [\"\"]\n")); err != nil {
		t.Fatal(err)
	}
	for _, child := range []string{"list", "check"} {
		if got := invoke(child); got.Exit != 2 || !strings.Contains(got.Stdout, "config.invalid") || !strings.Contains(compactRead(got.Stdout), `"key":"exclude[0]"`) {
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

func TestMemoryRefusesOnlyParsedUnmovedMutationExecution(t *testing.T) {
	m := newMemory(t)
	root := t.TempDir()
	physical := filepath.Join(root, config.File)
	physicalBytes := []byte("exclude: [\"\"]\n")
	if err := os.WriteFile(physical, physicalBytes, 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.Write(filepath.Join(root, "board.go"), []byte("package board\nfunc Value() int { return 1 }\n")); err != nil {
		t.Fatal(err)
	}
	for _, child := range []string{"run", "sample", "except"} {
		t.Run(child, func(t *testing.T) {
			if result, err := m.Run(features.Run{Dir: root, Args: []string{"mutation", child, "--json"}}); err == nil || !strings.Contains(err.Error(), "still executes in cmd") || result.Stdout != "" {
				t.Fatalf("unmoved execution was not explicitly refused: %+v %v", result, err)
			}
			result, err := m.Run(features.Run{Dir: root, Args: []string{"mutation", child, "--help"}})
			if err != nil || result.Exit != 0 || !strings.Contains(result.Stdout, "usage: itos-cc mutation "+child) {
				t.Fatalf("genuine help rejected: %+v %v", result, err)
			}
			result, err = m.Run(features.Run{Dir: root, Args: []string{"mutation", child, "--json", "--not-a-flag"}})
			if err != nil || result.Exit != 2 || !strings.Contains(result.Stdout, "flags.unknown") {
				t.Fatalf("genuine parse error rejected: %+v %v", result, err)
			}
		})
	}
	actual, err := os.ReadFile(physical)
	if err != nil || string(actual) != string(physicalBytes) {
		t.Fatalf("physical decoy changed: %s %v", actual, err)
	}
}
