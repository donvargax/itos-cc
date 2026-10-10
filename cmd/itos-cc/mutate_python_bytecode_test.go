package main

import (
	"encoding/json"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The scenario of "Rule: A Python mutant always runs as mutated" in
// features/mutate.feature. Python takes a module's bytecode in __pycache__
// as fresh while its source has the mtime, in whole seconds, and the size
// it was compiled from. A mutant that replaces "==" with "!=" keeps the
// size, so one written within the second its baseline compiled the source
// runs the unmutated bytecode. The project's conftest.py makes that second
// certain on any machine: before each run imports board.py it sets
// board.py's mtime to one fixed second, as if the baseline and the mutant
// were both written within it. At the end of each run it records whether a
// __pycache__ stands beside the sources it ran. It needs a real python3
// with pytest, and skips, naming it, without one.

// pinnedSecond is the mtime conftest.py gives board.py before every run.
const pinnedSecond = 1700000000

// pythonBytecodeRepo makes the project in a new directory, compiles its
// sources into its own __pycache__, as a project that ran its tests has,
// and makes it the test's directory (useDir). It returns the file conftest.py
// appends each run's directory and whether it left a __pycache__ to.
func pythonBytecodeRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runs := filepath.Join(t.TempDir(), "runs")
	literal, err := json.Marshal(runs)
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		// pytest's own cache is left out, so the project's tree holds only
		// what the scenario is about.
		"pyproject.toml": "[project]\nname = \"board\"\nversion = \"0.0.0\"\n\n" +
			"[tool.pytest.ini_options]\naddopts = \"-p no:cacheprovider\"\n",
		"board.py":      "def low(i):\n    return i == 3\n",
		"test_board.py": "from board import low\n\n\ndef test_low():\n    assert low(3)\n    assert not low(4)\n",
		"conftest.py": "import os\nimport pathlib\n\n" +
			"HERE = pathlib.Path(__file__).resolve().parent\n" +
			"os.utime(HERE / \"board.py\", (" + strconv.Itoa(pinnedSecond) + ", " + strconv.Itoa(pinnedSecond) + "))\n\n\n" +
			"def pytest_sessionfinish(session):\n" +
			"    with open(" + string(literal) + ", \"a\") as f:\n" +
			"        f.write(f\"{HERE}\\t{(HERE / '__pycache__').exists()}\\n\")\n",
	} {
		writeFile(t, filepath.Join(dir, name), text)
	}
	if out, err := exec.Command("python3", "-m", "compileall", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("python3 -m compileall: %v\n%s", err, out)
	}
	useDir(t, dir)
	return runs
}

// projectTree is every file and directory under dir, by path relative to
// it, with each file's content, leaving out .metrics, which is itos-cc's.
func projectTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		switch {
		case rel == ".":
			return nil
		case d.IsDir() && rel == ".metrics":
			return filepath.SkipDir
		case d.IsDir():
			tree[filepath.ToSlash(rel)+"/"] = ""
			return nil
		}
		data, err := os.ReadFile(path)
		tree[filepath.ToSlash(rel)] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

// @ID-MUT-210
func TestASameSizePythonMutantWrittenWithinTheBaselinesSecondIsStillKilled(t *testing.T) {
	languageTools(t, "python")
	// pytest plugins installed beside pytest, such as pytest-benchmark, write
	// their own files into the project; the scenario is about bytecode.
	useEnv(t, "PYTEST_DISABLE_PLUGIN_AUTOLOAD", "1")
	// Given a Python project whose test kills a mutant that replaces "=="
	// with "!="
	runs := pythonBytecodeRepo(t)
	dir, err := filepath.EvalSymlinks(wd(t))
	if err != nil {
		t.Fatal(err)
	}
	if sites := scanned(t, "board.py"); len(sites) != 1 || sites[0].Original != "==" || sites[0].Replacement != "!=" {
		t.Fatalf("sites of board.py: %+v, want the one \"==\" → \"!=\"", sites)
	}
	before := projectTree(t, dir)
	if _, ok := before["__pycache__/"]; !ok {
		t.Fatalf("the project has no __pycache__ of its own: %v", slices.Sorted(maps.Keys(before)))
	}

	// And the baseline and the mutant are written within the same second
	// (conftest.py pins board.py's mtime before every run)

	// When I run "itos-cc mutation run --json" for its file
	o := mutateCovered(t, "--json", "board.py")
	defer func() {
		if t.Failed() {
			t.Logf("exit %d\nstdout:\n%s\nstderr:\n%s", o.code, o.stdout, o.stderr)
		}
	}()

	// Then the mutant is killed, not survived
	f := o.json(t).file(t, "board.py")
	if f.Baseline != "passed" || f.Ran != 1 || f.Killed != 1 || f.Survived != 0 {
		t.Errorf("board.py: %+v, want its baseline passed and its one mutant run and killed", f)
	}

	// And no __pycache__ directory is written beside the worker copy's
	// sources
	data, err := os.ReadFile(runs)
	if err != nil {
		t.Fatalf("no run recorded itself: %v", err)
	}
	copies := 0
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		ran, left, _ := strings.Cut(line, "\t")
		if ran == dir {
			continue // the project's own tree, which the next step checks
		}
		copies++
		if left != "False" {
			t.Errorf("a run in %s left a __pycache__ beside its sources", ran)
		}
	}
	if copies < 2 {
		t.Errorf("runs in a worker copy: %d, want the baseline and the mutant\n%s", copies, data)
	}

	// And the project's own tree, its __pycache__ included, is left
	// unchanged
	after := projectTree(t, dir)
	for _, path := range slices.Sorted(maps.Keys(after)) {
		if text, ok := before[path]; !ok {
			t.Errorf("the project's tree gained %s", path)
		} else if text != after[path] {
			t.Errorf("the project's %s changed", path)
		}
	}
	for _, path := range slices.Sorted(maps.Keys(before)) {
		if _, ok := after[path]; !ok {
			t.Errorf("the project's tree lost %s", path)
		}
	}
}
