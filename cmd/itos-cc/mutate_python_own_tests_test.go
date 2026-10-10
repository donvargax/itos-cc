package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The scenario outline of "Rule: A file's own tests are the tests that reach
// it, in every language" in features/mutate.feature that the
// own-tests-python slice holds. Its project's tests each append their name
// and the directory they ran in to a log as they run, so the steps see which
// tests each command ran: coverage runs in the project's tree, and the
// baseline and mutants in worker copies, which are gone once the run
// returns. test_a reaches a.py through its import, test_b reaches only b.py
// and fails, and test_c reaches no module the graph sees: it loads a and u
// with importlib by names it computes, the way a test that runs code
// through a subprocess or the CLI executes code no import of its names. It needs a real python3 with
// pytest and coverage, and skips, naming the missing one, without them; CI
// runs it on ubuntu-latest (T-13).

// ownTestsRepo makes the project in a new directory, run with runner, and
// makes it the test's directory (useDir). With "unittest" the project has a
// .venv whose python sees the system's coverage but cannot import pytest,
// so itos-cc's pytest probe finds none. It returns the directory, its
// symlinks resolved, and the log the tests write.
func ownTestsRepo(t *testing.T, runner string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	runs := filepath.Join(t.TempDir(), "runs")
	literal, err := json.Marshal(runs)
	if err != nil {
		t.Fatal(err)
	}
	record := func(name string) string {
		return "import pathlib\nimport unittest\n\n" +
			"HERE = pathlib.Path(__file__).resolve().parent\n\n\n" +
			"def record():\n" +
			"    with open(" + string(literal) + ", \"a\") as f:\n" +
			"        f.write(f\"" + name + "\\t{HERE}\\n\")\n\n\n"
	}
	for name, text := range map[string]string{
		"pyproject.toml": "[project]\nname = \"board\"\nversion = \"0.0.0\"\n",
		"a.py":           "def low(i):\n    return i == 3\n\n\ndef spare(i):\n    return i == 7\n",
		"b.py":           "def high(i):\n    return i == 9\n",
		"u.py":           "def mid(i):\n    return i == 5\n",
		"test_a.py": record("test_a") + "from a import low\n\n\n" +
			"class TestA(unittest.TestCase):\n    def test_low(self):\n        record()\n" +
			"        self.assertTrue(low(3))\n        self.assertFalse(low(4))\n",
		"test_b.py": record("test_b") + "from b import high\n\n\n" +
			"class TestB(unittest.TestCase):\n    def test_high(self):\n        record()\n" +
			"        self.assertTrue(high(9))\n        self.fail(\"test_b fails\")\n",
		"test_c.py": record("test_c") + "import importlib\n\n\n" +
			"class TestC(unittest.TestCase):\n    def test_loaded(self):\n        record()\n" +
			"        a, u = (importlib.import_module(name) for name in (\"a\", \"u\"))\n" +
			"        self.assertTrue(a.spare(7))\n        self.assertFalse(a.spare(8))\n" +
			"        self.assertTrue(u.mid(5))\n        self.assertFalse(u.mid(4))\n",
	} {
		writeFile(t, filepath.Join(dir, name), text)
	}
	if runner == "unittest" {
		venv := filepath.Join(dir, ".venv")
		if out, err := exec.Command("python3", "-m", "venv", "--system-site-packages", "--without-pip", venv).CombinedOutput(); err != nil {
			t.Fatalf("python3 -m venv: %v\n%s", err, out)
		}
		python := filepath.Join(venv, "bin", "python")
		out, err := exec.Command(python, "-c", "import sysconfig; print(sysconfig.get_paths()['purelib'])").Output()
		if err != nil {
			t.Fatalf("the .venv's site-packages: %v", err)
		}
		// A .pth line that starts with import runs as python starts: no
		// pytest, wherever one is installed.
		writeFile(t, filepath.Join(strings.TrimSpace(string(out)), "no_pytest.pth"), "import sys; sys.modules[\"pytest\"] = None\n")
		if exec.Command(python, "-c", "import pytest").Run() == nil {
			t.Fatal("the .venv's python imports pytest")
		}
		if out, err := exec.Command(python, "-c", "import coverage").CombinedOutput(); err != nil {
			t.Fatalf("the .venv's python cannot import coverage: %v\n%s", err, out)
		}
	}
	useDir(t, dir)
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved, runs
}

// ownTestRuns is each test run the log holds, as the test's name and the
// directory it ran in; none when there is no log.
func ownTestRuns(t *testing.T, runs string) [][2]string {
	t.Helper()
	data, err := os.ReadFile(runs)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out [][2]string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		name, ran, _ := strings.Cut(line, "\t")
		out = append(out, [2]string{name, ran})
	}
	return out
}

// ownMutantOf is the mutant of f in function, failing t unless there is
// exactly one.
func ownMutantOf(t *testing.T, f fileJSON, function string) mutantJSON {
	t.Helper()
	var found []mutantJSON
	if f.Mutants != nil {
		for _, m := range *f.Mutants {
			if strings.HasSuffix(m.Function, "#"+function) {
				found = append(found, m)
			}
		}
	}
	if len(found) != 1 {
		t.Errorf("%s: mutants of %s %+v, want one", f.File, function, found)
		return mutantJSON{}
	}
	return found[0]
}

// onlyTest says whether names holds name and nothing else.
func onlyTest(names []string, name string) bool {
	for _, n := range names {
		if n != name {
			return false
		}
	}
	return len(names) > 0
}

// @ID-MUT-216
func TestAPythonFilesOwnTestsAreTheTestsThatReachIt(t *testing.T) {
	t.Parallel()
	languageTools(t, "python")
	for _, runner := range []string{"pytest", "unittest"} {
		t.Run(runner, func(t *testing.T) {
			t.Parallel()
			// pytest plugins installed beside pytest are no part of the
			// scenario.
			useEnv(t, "PYTEST_DISABLE_PLUGIN_AUTOLOAD", "1")
			// Given a Python project run with <runner> where test_a reaches
			// a.py, test_b reaches only b.py, and test_b fails.
			dir, runs := ownTestsRepo(t, runner)

			// When I run "itos-cc mutation run --json" for a.py, coverage
			// measured.
			o := mutateCovered(t, "--json", "a.py")
			logPytestRun(t, &o)

			// Then a.py's baseline passes and its mutant is judged by
			// test_a alone, with coverage measured from test_a alone:
			// low's mutant test_a kills, and spare, which only test_c
			// executes, is uncovered.
			f := o.json(t).file(t, "a.py")
			if f.Baseline != "passed" {
				t.Errorf("a.py: baseline %q, want passed", f.Baseline)
			}
			if m := ownMutantOf(t, f, "low"); m.Outcome != "killed" {
				t.Errorf("low's mutant: %+v, want it killed by test_a", m)
			}
			if m := ownMutantOf(t, f, "spare"); m.Outcome != "uncovered" {
				t.Errorf("spare's mutant: %+v, want it uncovered, as test_a never executes spare", m)
			}
			var inTree, inCopies []string
			for _, run := range ownTestRuns(t, runs) {
				if run[1] == dir {
					inTree = append(inTree, run[0])
				} else {
					inCopies = append(inCopies, run[0])
				}
			}
			if !onlyTest(inTree, "test_a") {
				t.Errorf("coverage in the project's tree ran %v, want test_a alone", inTree)
			}
			if len(inCopies) < 2 || !onlyTest(inCopies, "test_a") {
				t.Errorf("the worker copies ran %v, want test_a alone, for the baseline and the mutant", inCopies)
			}

			// And test_b never runs.
			for _, run := range ownTestRuns(t, runs) {
				if run[0] == "test_b" {
					t.Errorf("test_b ran in %s", run[1])
				}
			}

			// But a Python file no test reaches has its mutants reported
			// uncovered, and no test command runs for it.
			os.Remove(runs)
			unreached := mutateCovered(t, "--json", "u.py")
			logPytestRun(t, &unreached)
			if u := unreached.json(t).file(t, "u.py"); u.Uncovered != 1 || u.Ran != 0 {
				t.Errorf("u.py: %+v, want its one mutant uncovered and none run", u)
			}
			if got := ownTestRuns(t, runs); len(got) != 0 {
				t.Errorf("tests ran for u.py: %v, want none", got)
			}
			for _, line := range strings.Split(unreached.stderr, "\n") {
				if strings.Contains(line, "$ ") {
					t.Errorf("a command ran for u.py: %s", line)
				}
			}
		})
	}
}
