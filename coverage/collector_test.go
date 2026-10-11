package coverage

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestSetEnvReplacesAndUnsetsVariables(t *testing.T) {
	env := []string{"PATH=/bin", "COVERAGE_FILE=/elsewhere/.coverage", "COVERAGE_PROCESS_CONFIG=abc", "HOME=/h"}
	got := setEnv(env, pythonCoverEnv("/run/rc")...)
	want := []string{"PATH=/bin", "HOME=/h", "COVERAGE_PROCESS_START=/run/rc"}
	if !slices.Equal(got, want) {
		t.Errorf("setEnv %q, want %q: the caller's data file and config gone", got, want)
	}
	if !slices.Equal(env, []string{"PATH=/bin", "COVERAGE_FILE=/elsewhere/.coverage", "COVERAGE_PROCESS_CONFIG=abc", "HOME=/h"}) {
		t.Errorf("setEnv changed its argument: %q", env)
	}
	// A run of one listed test sets its own data file after the unset.
	got = setEnv(env, append(pythonCoverEnv("/run/rc"), "COVERAGE_FILE=/run/0/.coverage")...)
	if !slices.Contains(got, "COVERAGE_FILE=/run/0/.coverage") || slices.Contains(got, "COVERAGE_FILE=/elsewhere/.coverage") {
		t.Errorf("setEnv %q, want only the test's own COVERAGE_FILE", got)
	}
	if got := unsetEnv([]string{"A=1", "COVERAGE_PROCESS_START=x", "B=2"}, pythonCoverEnv("/run/rc")...); !slices.Equal(got, []string{"A=1", "B=2"}) {
		t.Errorf("unsetEnv %q, want none of the collector's variables", got)
	}
}

func TestPythonPlansMeasureTheProcessesTheTestsStartInTheirOwnDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in python is a shell script")
	}
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"pyproject.toml":   "",
		".venv/bin/python": "#!/bin/sh\nexit 0\n",
		"a.py":             "",
	})
	out := filepath.Join(dir, ".metrics", "run-1")
	plans := Plans([]string{filepath.Join(dir, "a.py")}, out, AllTests, nil)
	if len(plans) != 1 {
		t.Fatalf("plans %+v, want one", plans)
	}
	p := plans[0]
	if rel, err := filepath.Rel(out, p.CoverDir); err != nil || strings.HasPrefix(rel, "..") || rel == "." {
		t.Errorf("CoverDir %q, want a directory under the run's %s", p.CoverDir, out)
	}
	if len(p.Prepare) < 5 || p.Prepare[2] != pythonStartScript || filepath.Dir(p.Prepare[3]) != p.CoverDir || filepath.Dir(p.Prepare[4]) != p.CoverDir {
		t.Errorf("Prepare %q, want the start script writing its rcfile and data in CoverDir", p.Prepare)
	}
	if !slices.Contains(p.CoverEnv, "COVERAGE_PROCESS_START="+p.Prepare[3]) {
		t.Errorf("CoverEnv %q, want COVERAGE_PROCESS_START naming the rcfile", p.CoverEnv)
	}
	if p.Integration == "" || filepath.Dir(p.Integration) != filepath.Dir(p.Reports[0]) || len(p.Convert) != 1 {
		t.Errorf("Integration %q, Convert %q, want one conversion to a report beside %s", p.Integration, p.Convert, p.Reports[0])
	}
	// The commands themselves are as before: the project's configuration,
	// itos-cc's flags.
	for _, args := range p.Commands {
		for _, a := range args {
			if strings.Contains(a, "rcfile") || strings.Contains(a, "COVERAGE_PROCESS") {
				t.Errorf("command %q names the subprocess configuration", args)
			}
		}
	}
}

func TestPythonSubprocessLinesFoldInBesideTheInProcessReport(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "cli.py")
	// The in-process report names every executable line, the module's
	// import-time lines executed; the subprocess's names what it ran.
	writeFiles(t, dir, map[string]string{
		"cli.py": "",
		"lcov.info": "SF:cli.py\nDA:1,1\nDA:2,0\nDA:3,0\nDA:4,0\nDA:5,1\n" +
			"BRDA:2,0,jump to line 3,0\nBRDA:2,0,jump to line 4,0\nend_of_record\n",
		"integration.info": "SF:" + filepath.ToSlash(file) + "\nDA:1,1\nDA:2,1\nDA:3,1\nDA:4,0\n" +
			"BRDA:2,0,jump to line 3,1\nBRDA:2,0,jump to line 4,0\nend_of_record\n",
	})
	var log bytes.Buffer
	r := load([]string{filepath.Join(dir, "lcov.info")}, filepath.Join(dir, "integration.info"), dir, []string{file}, &log)
	for line, want := range map[int][]string{
		1: {"in-process", "integration"}, 2: {"integration"}, 3: {"integration"}, 4: nil, 5: {"in-process"},
	} {
		if got := r.LineSources(file, line); !slices.Equal(got, want) {
			t.Errorf("line %d sources %q, want %q", line, got, want)
		}
	}
	// Coverage is shared, so crap counts the subprocess's branch too.
	if got := fraction(t, r, file, 1, 5); !approx(got, 0.5) {
		t.Errorf("fraction %v, want 1/2: the decision's branch the subprocess took counts", got)
	}
	if covered, _ := r.LineCovered(file, 4); covered {
		t.Errorf("line 4 is covered, want uncovered: no process ran it")
	}
}

// pythonWithSubprocessCoverage is a python3 whose coverage.py starts in the
// Python processes it starts, or skips t.
func pythonWithSubprocessCoverage(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the rcfile is checked with a POSIX python3")
	}
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not installed")
	}
	if exec.Command(py, "-c", "import coverage, sys; sys.exit(coverage.version_info < (7, 13))").Run() != nil {
		t.Skip("coverage.py 7.13 or later is not installed for python3")
	}
	return py
}

func TestTheSubprocessRcfileLayersItosCcsSettingsOnTheProjects(t *testing.T) {
	py := pythonWithSubprocessCoverage(t)
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"pyproject.toml": "[tool.coverage.run]\nomit = [\"gen/*\", \"*_pb2.py\"]\nbranch = false\nparallel = false\n" +
			"data_file = \"project.coverage\"\n\n[tool.coverage.report]\nexclude_also = [\"never\"]\n",
		"cli.py":     "import sys\n\n\ndef twice(n):\n    return n * 2\n\n\nprint(twice(int(sys.argv[1])))\n",
		"gen/gen.py": "print('generated')\n",
	})
	out := filepath.Join(t.TempDir(), "run-1", "python")
	plan := Plan{Language: "python", Dir: dir}
	pythonIntegration(&plan, py, dir, out)
	var log bytes.Buffer
	why, _, err := plan.start(context.Background(), os.Environ(), &log, nil)
	if err != nil || why != "" {
		t.Fatalf("start: %v %q\n%s", err, why, log.String())
	}
	data, err := os.ReadFile(filepath.Join(plan.CoverDir, "coveragerc"))
	if err != nil {
		t.Fatal(err)
	}
	rc := string(data)
	for _, want := range []string{
		"branch = true", "parallel = true",
		"data_file = " + filepath.Join(plan.CoverDir, ".coverage"),
		"source = \n    " + dir,
		"omit = \n    " + filepath.Join(dir, "gen") + "/*\n    *_pb2.py",
	} {
		if !strings.Contains(rc, want) {
			t.Errorf("the rcfile holds no %q:\n%s", want, rc)
		}
	}
	if strings.Contains(rc, "project.coverage") || strings.Contains(rc, "exclude") {
		t.Errorf("the rcfile holds the project's data file or report settings:\n%s", rc)
	}
	if _, err := os.Stat(filepath.Join(dir, "project.coverage")); !os.IsNotExist(err) {
		t.Errorf("the project's data file was written (%v)", err)
	}

	// A Python process started with the plan's environment writes its data
	// to the run's directory, by the project's omit, whatever the caller's
	// COVERAGE_FILE says.
	for _, script := range []string{"cli.py", "gen/gen.py"} {
		cmd := exec.Command(py, filepath.Join(dir, script), "4")
		cmd.Dir = t.TempDir()
		cmd.Env = setEnv(append(os.Environ(), "COVERAGE_FILE="+filepath.Join(dir, "elsewhere")), plan.CoverEnv...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", script, err, output)
		}
	}
	if !plan.written() {
		t.Fatalf("nothing written to %s", plan.CoverDir)
	}
	report := plan.integrate(&log)
	if report == "" {
		t.Fatalf("no integration report:\n%s", log.String())
	}
	entries, err := Load(report)
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, e := range entries {
		files = append(files, filepath.Base(e.Path))
	}
	if !slices.Equal(files, []string{"cli.py"}) {
		t.Errorf("the integration report names %q, want cli.py alone: gen/ is omitted", files)
	}
	if _, err := os.Stat(plan.CoverDir); !os.IsNotExist(err) {
		t.Errorf("%s is still there after the conversion (%v)", plan.CoverDir, err)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".coverage*")); len(left) > 0 || fileExists(filepath.Join(dir, "elsewhere")) {
		t.Errorf("data written in the project: %q", left)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestACoveragePyThatDoesNotStartInNewProcessesSaysWhy(t *testing.T) {
	py := pythonWithSubprocessCoverage(t)
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"pyproject.toml": ""})
	plan := Plan{Language: "python", Dir: dir}
	pythonIntegration(&plan, py, dir, filepath.Join(t.TempDir(), "python"))
	// -S keeps site, and so every .pth file, from running in the process
	// the check starts.
	plan.Prepare = append([]string{py, "-c", strings.Replace(pythonStartScript, `[sys.executable, "-c", check]`, `[sys.executable, "-S", "-c", check]`, 1)}, plan.Prepare[3:]...)
	var log bytes.Buffer
	why, _, err := plan.start(context.Background(), os.Environ(), &log, nil)
	if err != nil || !strings.Contains(why, "7.13") {
		t.Errorf("start: %v, why %q, want why naming coverage.py 7.13\n%s", err, why, log.String())
	}
	missing := plan.noIntegration(why, &log)
	if missing.Cause != ToolMissing || missing.Language != "python" || !strings.Contains(log.String(), "without integration coverage") {
		t.Errorf("missing %+v, log %q, want tool-missing for python, logged", missing, log.String())
	}
}

func TestListedPythonTestsWriteTheirCoverageWhereTheHarnessOrItosCcSays(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"py/pyproject.toml": "",
		"py/a.py":           "",
		"go.mod":            "module m\n",
		"b.go":              "package m\n",
	})
	data := filepath.Join(dir, ".metrics", "tests")
	collectors := testCollectors(dir, data, []string{filepath.Join(dir, "py", "a.py"), filepath.Join(dir, "b.go")})
	if len(collectors) != 2 || collectors[0].language != "go" || collectors[1].language != "python" {
		t.Fatalf("collectors %+v, want Go's and Python's", collectors)
	}
	py := collectors[1]
	if py.dir != filepath.Join(dir, "py") {
		t.Errorf("Python's collector runs in %s, want the build root", py.dir)
	}
	rc := filepath.Join(data, ".python", "coveragerc")
	whole := wholeEnv(collectors, data)
	if !slices.Contains(whole, TestCoverDirEnv+"="+data) || !slices.Contains(whole, "COVERAGE_PROCESS_START="+rc) {
		t.Errorf("the run of every test gets %q, want %s and the rcfile", whole, TestCoverDirEnv)
	}
	each := setEnv(nil, eachEnv(collectors, filepath.Join(data, ".each", "0"))...)
	for _, want := range []string{
		"GOCOVERDIR=" + filepath.Join(data, ".each", "0"),
		"COVERAGE_FILE=" + filepath.Join(data, ".each", "0", ".coverage"),
		"COVERAGE_PROCESS_START=" + rc,
	} {
		if !slices.Contains(each, want) {
			t.Errorf("a run of one test gets %q, want %s", each, want)
		}
	}
	id := filepath.Join(data, "ID-A-01")
	writeFiles(t, id, map[string]string{".coverage.host.1.x": ""})
	if !anyWritten(collectors, id) || collectors[0].written(id) || !py.written(id) {
		t.Errorf("a coverage.py data file in %s is not Python's data alone", id)
	}
	if only := testCollectors(dir, data, []string{filepath.Join(dir, "b.go")}); len(only) != 1 {
		t.Errorf("collectors %+v without a Python source, want Go's alone", only)
	}
}

func TestLogLinesShowScriptsByName(t *testing.T) {
	if got := displayArgs([]string{"python3", "-c", "import os\nprint(1)\n", "a", "b"}); got != "python3 -c <script> a b" {
		t.Errorf("displayArgs %q", got)
	}
	if got := displayArgs([]string{"go", "tool", "covdata", "textfmt", "-i=d"}); got != "go tool covdata textfmt -i=d" {
		t.Errorf("displayArgs %q", got)
	}
}
