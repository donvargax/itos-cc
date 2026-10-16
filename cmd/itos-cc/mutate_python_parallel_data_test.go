package main

import (
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	mutation "github.com/donvargax/itos-cc/internal/mutants"
)

// The scenario of "Rule: Mutation results say what happened, and leave
// nothing stale behind" in features/mutation.feature that the
// python-parallel-data-fix slice holds. A coverage.py configuration that
// sets [run] parallel = true, or patch = subprocess, which forces it, has
// coverage run write .coverage.<host>.<pid>.<random> beside the data file
// itos-cc names instead of that file. coverage.py 7.14 and later combine
// those files when they report; before 7.14 a report reads the data file
// alone, so coverage lcov found no data and the build root measured
// nothing. Each project runs with a .venv that sees the system's pytest and
// coverage.py as coverage.py before 7.14 reports
// (testdata/python/coverage_before_7_14.py, imported by a .pth file as
// Python starts). calc.py has one mutation site, mid's == 5, which its test
// notices. They need a real python3 with pytest and coverage.py 7.13 or
// later, and skip, naming the missing one, without them; CI runs them on
// ubuntu-latest (T-13).

var parallelDataFiles = map[string]string{
	".gitignore":   ".metrics\n.venv\n",
	"calc.py":      "def mid(i):\n    return i == 5\n",
	"test_calc.py": "from calc import mid\n\n\ndef test_mid():\n    assert mid(5)\n    assert not mid(4)\n",
}

// parallelDataProject writes files, and a pyproject.toml whose
// [tool.coverage.run] holds run, in dir, with a .venv whose coverage.py
// reports as before 7.14, and makes dir the test's directory.
func parallelDataProject(t *testing.T, dir, run string, files map[string]string) {
	t.Helper()
	// pytest plugins installed beside pytest are no part of the scenario.
	useEnv(t, "PYTEST_DISABLE_PLUGIN_AUTOLOAD", "1")
	useDir(t, dir)
	for name, text := range files {
		writeFile(t, filepath.Join(dir, filepath.FromSlash(name)), text)
	}
	writeFile(t, filepath.Join(dir, "pyproject.toml"),
		"[project]\nname = \"calc\"\nversion = \"0.0.0\"\n\n[tool.coverage.run]\n"+run)
	venv := filepath.Join(dir, ".venv")
	if out, err := exec.Command("python3", "-m", "venv", "--system-site-packages", "--without-pip", venv).CombinedOutput(); err != nil {
		t.Fatalf("python3 -m venv: %v\n%s", err, out)
	}
	out, err := exec.Command(filepath.Join(venv, "bin", "python"), "-c", "import sysconfig; print(sysconfig.get_paths()['purelib'])").Output()
	if err != nil {
		t.Fatalf("the .venv's site-packages: %v", err)
	}
	module, err := os.ReadFile(filepath.Join("..", "..", "testdata", "python", "coverage_before_7_14.py"))
	if err != nil {
		t.Fatal(err)
	}
	site := strings.TrimSpace(string(out))
	writeFile(t, filepath.Join(site, "coverage_before_7_14.py"), string(module))
	writeFile(t, filepath.Join(site, "coverage_before_7_14.pth"), "import coverage_before_7_14\n")
}

// coverageDataLeft is each coverage.py data file under dir other than the
// one a run keeps beside its report, .metrics/coverage/<plan>/.coverage:
// any parallel data file, .coverage.*, and a .coverage anywhere else.
func coverageDataLeft(dir string) []string {
	var left []string
	filepath.WalkDir(dir, func(file string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, file)
		rel = filepath.ToSlash(rel)
		kept := d.Name() == ".coverage" && path.Dir(path.Dir(rel)) == ".metrics/coverage"
		if strings.HasPrefix(d.Name(), ".coverage.") || d.Name() == ".coverage" && !kept {
			left = append(left, rel)
		}
		return nil
	})
	return left
}

// noMeasuredNothing fails t when o says a build root measured nothing, or
// that a file it judged has no coverage.
func noMeasuredNothing(t *testing.T, o outcome, file string) {
	t.Helper()
	if strings.Contains(o.stdout+o.stderr, "coverage.measured-nothing") {
		t.Errorf("the run reports coverage.measured-nothing")
	}
	if strings.Contains(o.stderr, "no coverage for "+file) {
		t.Errorf("the run says there is no coverage for %s", file)
	}
}

// @ID-MUT-233
func TestAPythonProjectThatWritesParallelCoverageDataIsMeasured(t *testing.T) {
	t.Parallel()
	languageTools(t, "python")
	t.Run("parallel = true", func(t *testing.T) {
		t.Parallel()
		// Given a Python project whose coverage.py configuration sets
		// parallel = true, with a test that kills its mutant
		dir := t.TempDir()
		parallelDataProject(t, dir, "parallel = true\n", parallelDataFiles)

		// When I run "itos-cc mutation run --json" for its file
		o := mutateCovered(t, "--json", "calc.py")
		logPytestRun(t, &o)

		// Then its coverage is measured, not "coverage.measured-nothing", and
		// its mutant is killed
		var mutants []map[string]any
		for _, f := range o.rawFiles(t) {
			if f["file"] == "calc.py" {
				list, _ := f["mutants"].([]any)
				for _, m := range list {
					mutants = append(mutants, m.(map[string]any))
				}
			}
		}
		if len(mutants) != 1 {
			t.Fatalf("%d mutants of calc.py, want its one", len(mutants))
		}
		m := mutants[0]
		if got := cliCoverage(m); !slices.Equal(got, []string{"in-process"}) {
			t.Errorf("calc.py:2's mutant has \"coverage\" %q, want [\"in-process\"]: its test ran its line in coverage run's own process", got)
		}
		if m["outcome"] != "killed" {
			t.Errorf("calc.py:2's mutant %v, want killed", m["outcome"])
		}
		noMeasuredNothing(t, o, "calc.py")

		// And nothing of the run's data files is left in the project's tree
		if left := coverageDataLeft(dir); len(left) > 0 {
			t.Errorf("coverage data left in the project: %q", left)
		}
	})
	// patch = subprocess forces parallel data, and has coverage run hand
	// every Python process a test starts its own configuration, which those
	// processes read before COVERAGE_PROCESS_START: they write beside the
	// in-process data, named as it is, and count as in-process.
	t.Run("patch = subprocess", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		files := map[string]string{}
		for name, text := range pythonIntegrationFiles {
			if name != "pyproject.toml" {
				files[name] = text
			}
		}
		parallelDataProject(t, dir, "patch = [\"subprocess\"]\n", files)

		o := mutateCovered(t, "--all-tests", "--json", "cli.py")
		logPytestRun(t, &o)
		want := map[string]string{">=": "survived", "1": "killed", "-": "killed", "+": "uncovered"}
		for _, m := range cliMutants(t, o) {
			replacement, _ := m["replacement"].(string)
			if m["outcome"] != want[replacement] {
				t.Errorf("%s, want %s", cliMutant(m), want[replacement])
			}
			got := cliCoverage(m)
			if mutantLine(m) == cliOtherLine {
				if got != nil {
					t.Errorf("%s has \"coverage\" %q, want none", cliMutant(m), got)
				}
				continue
			}
			if !slices.Equal(got, []string{"in-process"}) {
				t.Errorf("%s: \"coverage\" %q, want [\"in-process\"]: the project's own patch wrote the subprocess's data beside the in-process data", cliMutant(m), got)
			}
		}
		noMeasuredNothing(t, o, "cli.py")
		if left := coverageDataLeft(dir); len(left) > 0 {
			t.Errorf("coverage data left in the project: %q", left)
		}
	})
	// A counted run measures through the supervised plan, in the frozen
	// tree under .git, with the live .venv linked in.
	t.Run("counted", func(t *testing.T) {
		t.Parallel()
		requireCountedPlatform(t)
		dir := t.TempDir()
		parallelDataProject(t, dir, "parallel = true\n", parallelDataFiles)
		gitIn(t, dir, "init", "-q")
		gitIn(t, dir, "add", ".")
		gitIn(t, dir, "commit", "-qm", "calc")
		var chosen []mutation.FreshCandidate
		seed := seedSelecting(t, dir, 1, selecting(&chosen, func(c mutation.FreshCandidate) bool {
			return c.Path == "calc.py" && c.Site.Original == "==" && c.Site.Replacement == "!="
		}))

		o := countedRun(t, "--count", "1", "--seed", seed, "--workers", "1", "calc.py")
		logOnFailure(t, &o)
		c := o.counted(t)
		if len(c.Selected) != 1 {
			t.Fatalf("selected = %+v, want calc.py's one site", c.Selected)
		}
		if site := c.Selected[0]; site["state"] != "judged" || site["outcome"] != "killed" {
			t.Errorf("selected %+v, want it judged killed", site)
		}
		if o.code != 0 || c.Sampling["completion"] != "completed" {
			t.Errorf("exit %d, completion %v; want 0, completed", o.code, c.Sampling["completion"])
		}
		noMeasuredNothing(t, o, "calc.py")
		if left := coverageDataLeft(dir); len(left) > 0 {
			t.Errorf("coverage data left in the project: %q", left)
		}
	})
}
