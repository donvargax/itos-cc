package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The project other tests share, from the outline of "Rule: A file's own
// tests are the tests that reach it, in every language" in
// features/mutate.feature, whose ID-MUT-216 holds the own-tests-python
// slice's old test whole (T-101). Its project's tests each append their name
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
