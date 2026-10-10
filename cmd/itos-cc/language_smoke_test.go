package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// One smoke test per language that mutation run measures through its real
// tools (T-13): each copies a tiny project from testdata/smoke, runs
// mutation run --json over its one source file with coverage measured, and
// checks that the tools both judged a mutant and measured coverage. Each
// source has two functions with one mutation site each, `== 3` and `== 9`:
// the test calls low, so its mutant runs, the one trial, and is killed; no
// test calls high, so its mutant is uncovered, which only a coverage report
// that ran can say. Python's may survive instead, for python-stale-bytecode.

type smokeMutant struct {
	Function string   `json:"function"`
	Outcome  string   `json:"outcome"`
	Coverage []string `json:"coverage"`
}

// smokeRepo copies testdata/smoke/<language> into a new directory, links
// node_modules into it when modules is not empty, and makes it the working
// directory.
func smokeRepo(t *testing.T, language, modules string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("testdata", "smoke", language))); err != nil {
		t.Fatal(err)
	}
	if modules != "" {
		if err := os.Symlink(modules, filepath.Join(dir, "node_modules")); err != nil {
			t.Skip("symlinks unavailable:", err)
		}
	}
	t.Chdir(dir)
}

// smoke runs mutation run --json over source and checks that low's mutant
// ran and was killed where coverage executed it, and that high's was
// uncovered. A known bug, an idea's id, lets low's mutant survive: it must
// still run and be judged.
func smoke(t *testing.T, source, bug string) {
	t.Helper()
	source = filepath.FromSlash(source)
	o := mutateCovered(t, "--json", source)
	defer func() {
		if t.Failed() {
			t.Logf("exit %d\nstdout:\n%s\nstderr:\n%s", o.code, o.stdout, o.stderr)
		}
	}()
	f := o.json(t).file(t, source)
	if f.Baseline != "passed" || f.Ran != 1 || f.Killed+f.Survived != 1 || f.Uncovered != 1 {
		t.Errorf("%s: %+v, want its baseline passed, one mutant run and judged, and one uncovered", source, f)
	}
	var out struct {
		Files []struct {
			File    string        `json:"file"`
			Mutants []smokeMutant `json:"mutants"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(o.stdout), &out); err != nil {
		t.Fatal(err)
	}
	var mutants []smokeMutant
	for _, f := range out.Files {
		if f.File == source {
			mutants = f.Mutants
		}
	}
	outcomes := map[string]smokeMutant{}
	for _, m := range mutants {
		for _, name := range []string{"low", "high"} {
			if strings.HasSuffix(strings.ToLower(m.Function), name) {
				outcomes[name] = m
			}
		}
	}
	low := outcomes["low"]
	if !slices.Contains(low.Coverage, "in-process") {
		t.Errorf("low's mutant: %+v, want coverage to have executed it", low)
	}
	killed := low.Outcome == "killed"
	switch {
	case bug != "" && low.Outcome == "survived":
		t.Logf("low's mutant survived, as %s lets it", bug)
	case !killed:
		t.Errorf("low's mutant: %+v, want killed", low)
	}
	if m := outcomes["high"]; m.Outcome != "uncovered" {
		t.Errorf("high's mutant: %+v, want uncovered", m)
	}
	if strings.Contains(o.stderr, "no coverage") {
		t.Errorf("stderr says \"no coverage\"")
	}
	if want := map[bool]int{true: 0, false: 1}[killed]; o.code != want {
		t.Errorf("exit %d, want %d, as low's mutant was %s", o.code, want, low.Outcome)
	}
}

func TestLanguageToolSmokeTypeScript(t *testing.T) {
	modules := languageTools(t, "typescript")
	smokeRepo(t, "typescript", modules)
	smoke(t, "src/board.ts", "")
}

func TestLanguageToolSmokePython(t *testing.T) {
	languageTools(t, "python")
	smokeRepo(t, "python", "")
	// python-stale-bytecode: a mutant the size of its source, written within
	// the second the worker's baseline compiled the source into
	// __pycache__, runs the unmutated bytecode and survives. Fast runners
	// hit it, so low's mutant may survive until that is fixed.
	smoke(t, "board.py", "python-stale-bytecode")
}

func TestLanguageToolSmokeKotlin(t *testing.T) {
	languageTools(t, "kotlin")
	smokeRepo(t, "kotlin", "")
	smoke(t, "src/main/kotlin/smoke/Board.kt", "")
}
