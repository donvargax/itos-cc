package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/donvargax/itos-cc/internal/metricsdir"
	mutation "github.com/donvargax/itos-cc/internal/mutants"
	snapporttest "github.com/donvargax/itos-cc/internal/snapshot/port/porttest"
	"github.com/donvargax/itos-cc/internal/testinputs"
	"github.com/donvargax/itos-cc/internal/testtools"
	"github.com/donvargax/itos-cc/internal/workdir"
)

// Strict Go coverage judges Go files alone: a TypeScript, Python or Kotlin
// file of the selection, beneath the Go module or outside any, gets no
// verdict, and its inputs are never fingerprinted as a Go module's.
func TestCheckGoCoverageJudgesGoFilesAlone(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"mod/go.mod":           "module example.com/mod\n\ngo 1.22\n",
		"mod/entry.go":         "package mod\n\nfunc Entry() string { return \"entry\" }\n",
		"mod/web/label.ts":     "export function label(name: string): string {\n  return name;\n}\n",
		"mod/tools/label.py":   "def label(name):\n    return name\n",
		"mod/app/Label.kt":     "fun label(name: String): String {\n    return name\n}\n",
		"outside/label.ts":     "export function label(name: string): string {\n  return name;\n}\n",
		"outside/label.py":     "def label(name):\n    return name\n",
		"outside/app/Label.kt": "fun label(name: String): String {\n    return name\n}\n",
	}
	var selection []string
	for name, text := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		if filepath.Base(name) != "go.mod" {
			selection = append(selection, filepath.FromSlash(name))
		}
	}
	slices.Sort(selection)
	t.Chdir(dir)

	var asked []string
	fp := &retainedGoInputs{Fingerprints: testinputs.Fingerprints{Runtime: testinputs.CurrentRuntime()}, root: dir, asked: &asked}
	checks, err := mutation.CheckCoverage(t.Context(), selection, mutation.CheckOptions{Root: dir, Dir: dir, Store: metricsdir.Mutations{Store: &snapporttest.Store{RootDir: dir}}, Tree: workdir.Tree{}, Surroundings: workdir.Surroundings{}, Toolchain: testtools.Toolchain{}, Fingerprints: fp}, map[string]bool{"go": true}, false, false)

	if err != nil {
		t.Fatalf("a mixed selection is an error: %v", err)
	}
	if !slices.Equal(asked, []string{"mod/entry.go"}) {
		t.Errorf("Go coverage inputs asked of %q, want of mod/entry.go alone", asked)
	}
	if len(checks) != 1 || filepath.ToSlash(checks[0].File) != "mod/entry.go" || checks[0].Function != "example.com/mod#Entry" || checks[0].State != "missing" {
		t.Errorf("checks %+v, want Entry alone, missing", checks)
	}
}

type retainedGoInputs struct {
	mutation.Fingerprints
	root  string
	asked *[]string
}

func (f *retainedGoInputs) Coverage(ctx context.Context, source, _, _ string, _ map[string]string) (map[string]string, mutation.InputFailure) {
	*f.asked = append(*f.asked, filepath.ToSlash(source))
	hashes, err := testinputs.GoCoverageInputs(ctx, testinputs.CurrentRuntime(), source, f.root, "producer", nil)
	if err != nil {
		return nil, mutation.InputFailed{Cause: err}
	}
	return hashes, nil
}
