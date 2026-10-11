package main

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/metrics"
	"github.com/donvargax/itos-cc/project"
)

// The scenario of "Rule: Coverage from tests that start other processes, in
// every language" in features/mutate.feature that the
// integration-coverage-python slice holds, and the rule's missing
// collector for Python. Each project's cli.py is run by its tests only in a
// subprocess, python cli.py N, which prints describe(N): no test imports
// it. The one test of the first project runs it with 1, so it takes the
// positive branch, lines 6 and 7, and never line 8; it notices `0` → `1`,
// which turns 1 to the other branch, and `+` → `-`, and not `>` → `>=`. When
// CLI_RECORD names a file, cli.py writes there where coverage.py, started in
// it, writes its data. They need a real python3 with pytest and coverage.py
// 7.13 or later, and skip, naming the missing one, without them; CI runs
// them on ubuntu-latest, where the language tools are installed.

const (
	cliIfLine       = 6
	cliPositiveLine = 7
	cliOtherLine    = 8
)

const cliSource = `import os
import sys


def describe(n):
    if n > 0:
        return n + 100
    return n - 100


if __name__ == "__main__":
    record = os.environ.get("CLI_RECORD")
    if record:
        try:
            import coverage
        except ImportError:
            coverage = None
        cov = coverage and coverage.Coverage.current()
        if cov is not None:
            with open(record, "a") as f:
                f.write(cov.get_option("run:data_file") + "\n")
    print(describe(int(sys.argv[1])))
`

// cliRunner is the head of a test file that runs cli.py in a subprocess:
// run(test, arg) is what it prints. With a harness that splits coverage by
// test, the processes test starts write their coverage.py data to
// COVERAGE_FILE=<ITOS_CC_TEST_COVERDIR>/<test>/.coverage.
func cliRunner(split bool) string {
	harness := "    split = \"\"\n"
	if split {
		harness = "    split = os.environ.get(\"ITOS_CC_TEST_COVERDIR\")\n"
	}
	return "import os\nimport pathlib\nimport subprocess\nimport sys\n\n" +
		"CLI = pathlib.Path(__file__).resolve().parent / \"cli.py\"\n\n\n" +
		"def run(test, arg):\n" +
		"    env = dict(os.environ)\n" + harness +
		"    if split:\n" +
		"        env[\"COVERAGE_FILE\"] = os.path.join(split, test, \".coverage\")\n" +
		"    done = subprocess.run([sys.executable, str(CLI), arg], env=env, capture_output=True, text=True, check=True)\n" +
		"    return done.stdout.strip()\n\n\n"
}

var pythonIntegrationFiles = map[string]string{
	"pyproject.toml": "[project]\nname = \"cli\"\nversion = \"0.0.0\"\n",
	"cli.py":         cliSource,
	"test_cli.py":    cliRunner(false) + "def test_positive():\n    assert run(\"test_positive\", \"1\") == \"101\"\n",
}

// pythonIntegrationRepo makes a project of files in a new directory, makes
// it the test's directory (useDir), and returns the file cli.py records
// its coverage data file in.
func pythonIntegrationRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	// pytest plugins installed beside pytest are no part of the scenario.
	useEnv(t, "PYTEST_DISABLE_PLUGIN_AUTOLOAD", "1")
	dir := t.TempDir()
	for name, text := range files {
		writeFile(t, filepath.Join(dir, filepath.FromSlash(name)), text)
	}
	useDir(t, dir)
	record := filepath.Join(t.TempDir(), "record")
	useEnv(t, "CLI_RECORD", record)
	return record
}

// cliMutants is the mutants of cli.py in o's --json output, as plain maps.
func cliMutants(t *testing.T, o outcome) []map[string]any {
	t.Helper()
	for _, f := range o.rawFiles(t) {
		if f["file"] != "cli.py" {
			continue
		}
		var out []map[string]any
		list, _ := f["mutants"].([]any)
		for _, m := range list {
			out = append(out, m.(map[string]any))
		}
		if len(out) != 4 {
			t.Fatalf("%d mutants of cli.py, want 4\n%s\nstderr:\n%s", len(out), o.stdout, o.stderr)
		}
		return out
	}
	t.Fatalf("no cli.py in files:\n%s\nstderr:\n%s", o.stdout, o.stderr)
	return nil
}

// cliMutant describes a mutant of cli.py.
func cliMutant(m map[string]any) string {
	return strings.Replace(describe(m), greetSource, "cli.py", 1)
}

// cliCoverage is the "coverage" of m, or nil when it has none.
func cliCoverage(m map[string]any) []string {
	list, has := m["coverage"].([]any)
	if !has {
		return nil
	}
	out := []string{}
	for _, s := range list {
		name, _ := s.(string)
		out = append(out, name)
	}
	return out
}

// @ID-MUT-228
func TestLinesAPythonTestReachesThroughASubprocessAreCovered(t *testing.T) {
	t.Parallel()
	languageTools(t, "python")
	t.Run("whole suite", func(t *testing.T) {
		t.Parallel()
		// Given a Python project whose only test runs its module in a
		// subprocess and checks one branch of its output
		record := pythonIntegrationRepo(t, pythonIntegrationFiles)
		out := filepath.Join(metrics.DirOf(project.RootOf(wd(t))), "coverage")

		// When I run "itos-cc mutation run --all-tests --fail-uncovered --json"
		o := mutateCovered(t, "--all-tests", "--fail-uncovered", "--json", "cli.py")
		logPytestRun(t, &o)

		// Then the lines the subprocess ran are covered, with "coverage"
		// listing "integration", and their mutants run
		// And a mutant the test notices is killed, one it does not notice
		// survives, and only mutants on lines never run are uncovered
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
			if !slices.Equal(got, []string{"integration"}) {
				t.Errorf("%s: \"coverage\" %q, want [\"integration\"]: only the subprocess ran its line", cliMutant(m), got)
			}
		}
		// The strict line evidence counts the subprocess's lines too: only
		// the line no process ran is an uncovered statement.
		if got, want := strictLineFindings(t, o), []string{"mutation.uncovered-statement cli.py cli#describe:8"}; !slices.Equal(got, want) {
			t.Errorf("strict findings %q, want %q", got, want)
		}
		if o.code != 1 {
			t.Errorf("exit %d, want 1: the line never run holds an uncovered mutant", o.code)
		}

		// The subprocess's coverage data stays with the run: under the run's
		// own run-* directory of .metrics/coverage/, and gone once it ends.
		data, err := os.ReadFile(record)
		if err != nil {
			t.Fatalf("no coverage.py started in the subprocess, which recorded no data file: %v", err)
		}
		file, _, _ := strings.Cut(string(data), "\n")
		rel, err := filepath.Rel(out, file)
		first, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
		if err != nil || !strings.HasPrefix(first, "run-") || first == rel {
			t.Fatalf("the subprocess wrote its data to %s, want a file under a run-* directory of %s", file, out)
		}
		if _, err := os.Stat(filepath.Dir(file)); !os.IsNotExist(err) {
			t.Errorf("the subprocess's data directory %s is still there after the run (%v)", filepath.Dir(file), err)
		}
		if _, err := os.Stat(filepath.Join(out, first)); !os.IsNotExist(err) {
			t.Errorf("the run's directory %s is still there after the run (%v)", first, err)
		}
		filepath.WalkDir(wd(t), func(path string, d fs.DirEntry, err error) error {
			if err == nil && strings.HasPrefix(d.Name(), ".coverage.") {
				t.Errorf("%s is left in the project", path)
			}
			return nil
		})
	})
	// And with mutation.tests listing two tests that run different
	// branches, each line is covered by the IDs of the tests that reached it
	for _, split := range []bool{false, true} {
		name := "listed tests, each run alone"
		if split {
			name = "listed tests, split by the harness"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			files := map[string]string{
				"pyproject.toml": pythonIntegrationFiles["pyproject.toml"],
				"cli.py":         cliSource,
				"test_listed.py": cliRunner(split) +
					"def test_positive():\n    assert run(\"test_positive\", \"1\") == \"101\"\n\n\n" +
					"def test_other():\n    assert run(\"test_other\", \"-1\") == \"-101\"\n",
				"tests.txt": "test_positive\ttest_listed.py\ntest_other\ttest_listed.py\n",
				"itos-cc.yaml": "mutation:\n  tests:\n" +
					"    list: python3 -c \"print(open('tests.txt').read(), end='')\"\n" +
					"    run: python3 -m pytest -q -p no:cacheprovider test_listed.py -k \"{pattern}\"\n" +
					"    ids_pattern: \"{ids}\"\n" +
					"    join:\n      each: \"{id}\"\n      sep: \" or \"\n",
			}
			pythonIntegrationRepo(t, files)

			o := mutateCovered(t, "--json", "cli.py")
			logPytestRun(t, &o)
			wantTests := map[int][]string{
				cliIfLine:       {"test_positive", "test_other"},
				cliPositiveLine: {"test_positive"},
				cliOtherLine:    {"test_other"},
			}
			for _, m := range cliMutants(t, o) {
				want := wantTests[mutantLine(m)]
				if got := listedTests(m); m["scope"] != "listed" || !slices.Equal(got, want) {
					t.Errorf("%s: scope %v, tests %q, want \"listed\" and %q, the tests that reach its line", cliMutant(m), m["scope"], got, want)
				}
			}
			var runs []string
			for _, l := range strings.Split(o.stderr, "\n") {
				if strings.HasPrefix(l, "itos-cc: coverage ") && strings.Contains(l, "test_listed.py -k") {
					runs = append(runs, l)
				}
			}
			if wantRuns := map[bool]int{false: 3, true: 1}[split]; len(runs) != wantRuns {
				t.Errorf("coverage ran the listed tests as %q, want %d runs", runs, wantRuns)
			}
		})
	}
}

// A coverage.py that does not start in the Python processes a test starts
// is a missing collector: coverage.tool-missing under --fail-uncovered,
// where Python has mutants to judge, and otherwise a log line, the run
// going on without integration coverage. The project's .venv sees the
// system's pytest and coverage.py, with a .pth file that keeps coverage.py
// from starting in a new process, as one before 7.13, which installs no
// .pth file, never does.
func TestACoveragePyThatCannotMeasureSubprocessesIsAMissingCollector(t *testing.T) {
	t.Parallel()
	languageTools(t, "python")
	pythonIntegrationRepo(t, pythonIntegrationFiles)
	venv := filepath.Join(wd(t), ".venv")
	if out, err := exec.Command("python3", "-m", "venv", "--system-site-packages", "--without-pip", venv).CombinedOutput(); err != nil {
		t.Fatalf("python3 -m venv: %v\n%s", err, out)
	}
	python := filepath.Join(venv, "bin", "python")
	out, err := exec.Command(python, "-c", "import sysconfig; print(sysconfig.get_paths()['purelib'])").Output()
	if err != nil {
		t.Fatalf("the .venv's site-packages: %v", err)
	}
	writeFile(t, filepath.Join(strings.TrimSpace(string(out)), "no_subprocess_coverage.pth"),
		"import coverage; coverage.process_startup = lambda *args, **kwargs: None\n")
	// The test runs cli.py with the .venv's python too.

	strict := mutateCovered(t, "--all-tests", "--fail-uncovered", "--json", "cli.py")
	logPytestRun(t, &strict)
	var found bool
	for _, p := range strict.json(t).Problems {
		if p["rule"] == "coverage.tool-missing" && p["language"] == "python" {
			found = true
			if message, _ := p["message"].(string); !strings.Contains(message, "subprocess") {
				t.Errorf("coverage.tool-missing says %q, want it to name subprocess measurement", message)
			}
		}
	}
	if !found {
		t.Errorf("a strict run's problems are %q, want coverage.tool-missing for python", problemRules(t, strict))
	}
	if strict.code == 0 {
		t.Errorf("a strict run exits 0, want it to fail")
	}
	if strings.Contains(strict.stderr, "itos-cc: baseline") {
		t.Errorf("a strict run ran a mutant's baseline, want it to stop before any mutant runs")
	}

	plain := mutateCovered(t, "--all-tests", "--json", "cli.py")
	logPytestRun(t, &plain)
	if !strings.Contains(plain.stderr, "integration coverage") {
		t.Errorf("a run without --fail-uncovered logs nothing of integration coverage")
	}
	for _, p := range plain.json(t).Problems {
		if p["rule"] == "coverage.tool-missing" {
			t.Errorf("a run without --fail-uncovered reports %v", p)
		}
	}
	for _, m := range cliMutants(t, plain) {
		if m["outcome"] != "uncovered" {
			t.Errorf("%s, want uncovered, as before: no process the test started was measured", cliMutant(m))
		}
	}
}
