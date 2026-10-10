package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The scenario of "Rule: Counted and Python runs touch only what their
// judgments need" in features/mutate.feature that the python-no-pytest-cache
// slice holds. Its project's conftest.py records, as each pytest run ends,
// the directory it ran in and whether a .pytest_cache stands beside its
// sources, so the step sees the worker copies, which are gone once the run
// returns. It needs a real python3 with pytest and coverage, and skips,
// naming the missing one, without them; CI runs it on ubuntu-latest (T-13).

// pytestCacheRepo makes the project in a new directory and makes it the
// test's directory (useDir). It returns the directory, its symlinks resolved, and
// the file conftest.py appends each run's directory and whether it left a
// .pytest_cache to.
func pytestCacheRepo(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	runs := filepath.Join(t.TempDir(), "runs")
	literal, err := json.Marshal(runs)
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		"pyproject.toml": "[project]\nname = \"board\"\nversion = \"0.0.0\"\n",
		"board.py":       "def low(i):\n    return i == 3\n",
		"test_board.py":  "from board import low\n\n\ndef test_low():\n    assert low(3)\n    assert not low(4)\n",
		// pytest_unconfigure comes after every plugin's sessionfinish, the
		// cache's included.
		"conftest.py": "import pathlib\n\n" +
			"HERE = pathlib.Path(__file__).resolve().parent\n\n\n" +
			"def pytest_unconfigure(config):\n" +
			"    with open(" + string(literal) + ", \"a\") as f:\n" +
			"        f.write(f\"{HERE}\\t{(HERE / '.pytest_cache').exists()}\\n\")\n",
	} {
		writeFile(t, filepath.Join(dir, name), text)
	}
	useDir(t, dir)
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved, runs
}

// logPytestRun logs what o printed once t has failed. The step builds on
// every OS, so it cannot use logOutcome, which only the !windows fail-fast
// steps define.
func logPytestRun(t *testing.T, o *outcome) {
	t.Helper()
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("exit %d\nstdout:\n%s\nstderr:\n%s", o.code, o.stdout, o.stderr)
		}
	})
}

// pytestRuns is each run conftest.py recorded: its directory and whether
// it left a .pytest_cache there.
func pytestRuns(t *testing.T, runs string) [][2]string {
	t.Helper()
	data, err := os.ReadFile(runs)
	if err != nil {
		t.Fatalf("no run recorded itself: %v", err)
	}
	var out [][2]string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		ran, left, _ := strings.Cut(line, "\t")
		out = append(out, [2]string{ran, left})
	}
	return out
}

// @ID-MUT-214
func TestMeasuringAndJudgingAPythonProjectLeavesNoPytestCacheInItsTree(t *testing.T) {
	languageTools(t, "python")
	// pytest plugins installed beside pytest may write files of their own;
	// the scenario is about pytest's cache, which is no plugin it autoloads.
	useEnv(t, "PYTEST_DISABLE_PLUGIN_AUTOLOAD", "1")
	// Given a Python project with a pytest test that kills its mutant and
	// no .pytest_cache directory.
	dir, runs := pytestCacheRepo(t)

	// When I run "itos-cc mutation run --json" for its file, coverage
	// measured in the project's own tree.
	o := mutateCovered(t, "--json", "board.py")
	logPytestRun(t, &o)

	// Then the mutant is killed.
	if f := o.json(t).file(t, "board.py"); f.Baseline != "passed" || f.Ran != 1 || f.Killed != 1 {
		t.Errorf("board.py: %+v, want its baseline passed and its one mutant run and killed", f)
	}

	// And no .pytest_cache directory is written in the project's tree or
	// beside the worker copy's sources.
	inTree, copies := false, 0
	for _, run := range pytestRuns(t, runs) {
		if run[0] == dir {
			inTree = true
		} else {
			copies++
		}
		if run[1] != "False" {
			t.Errorf("a pytest run in %s left a .pytest_cache beside its sources", run[0])
		}
	}
	if !inTree || copies < 2 {
		t.Errorf("pytest ran in the project's tree %v and in %d worker copies: want coverage measured in the tree, and the baseline and the mutant in copies",
			inTree, copies)
	}
	if _, err := os.Stat(filepath.Join(dir, ".pytest_cache")); err == nil {
		t.Error("the project's tree holds a .pytest_cache")
	}

	// But a run with --test-command runs that command exactly as given:
	// with pytest's cache, which it then writes beside the worker copy's
	// sources.
	os.Remove(runs)
	given := "python3 -m pytest -q"
	custom := mutateCovered(t, "--json", "--no-coverage", "--mutate-all", "--test-command", given, "board.py")
	logPytestRun(t, &custom)
	if f := custom.json(t).file(t, "board.py"); f.Killed != 1 || !strings.Contains(custom.stderr, "$ "+given+"\n") {
		t.Errorf("with --test-command: board.py %+v, stderr:\n%s\nwant its mutant killed by %q run as given", f, custom.stderr, given)
	}
	cached := false
	for _, run := range pytestRuns(t, runs) {
		cached = cached || (run[0] != dir && run[1] == "True")
	}
	if !cached {
		t.Errorf("runs of %q: %v: want pytest's cache written beside a worker copy's sources, as the command given does", given, pytestRuns(t, runs))
	}
}
