package covrun

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode"
)

// pytestRunner is how a coverage plan runs pytest: without its cache
// plugin, as every pytest command itos-cc composes runs, so measuring writes
// no .pytest_cache into the project. Plugins it autoloads stay the
// project's.
var pytestRunner = []string{"-m", "pytest", "-q", "-p", "no:cacheprovider"}

// pythonRunner is the runner arguments of a Python coverage plan: pytest's
// or unittest's, over tests, the test files PythonTests gives, or over the
// whole suite when tests is nil.
func pythonRunner(pytest bool, tests []string) []string {
	switch {
	case pytest:
		return append(slices.Clone(pytestRunner), tests...)
	case tests != nil:
		return append([]string{"-m", "unittest"}, PythonModules(tests)...)
	}
	return []string{"-m", "unittest", "discover"}
}

// PythonTests is which of tests, the test files that reach a source, a
// Python own-test run in dir passes its runner: those inside dir named as
// pytest collects test files by default, test_*.py or *_test.py, relative
// to dir with forward slashes, sorted. conftest.py, __init__.py and other
// helpers are left out: pytest applies conftest.py files on its own, and a
// file it collects no test from would fail a run of it alone.
func PythonTests(dir string, tests []string) []string {
	out := []string{}
	for _, test := range tests {
		if !PythonTestFile(test) {
			continue
		}
		rel, err := filepath.Rel(dir, test)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			continue
		}
		if rel = filepath.ToSlash(rel); !slices.Contains(out, rel) {
			out = append(out, rel)
		}
	}
	sort.Strings(out)
	return out
}

// PythonTestFile says whether the file at path is named as pytest collects
// test files by default, test_*.py or *_test.py: a runnable test, not a
// conftest.py or a helper.
func PythonTestFile(path string) bool {
	base := filepath.Base(path)
	return strings.HasSuffix(base, ".py") && (strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py"))
}

// PythonModules is tests, test files relative to the directory the
// command runs in as PythonTests gives them, as the dotted modules
// python -m unittest imports: relative to src for one beneath it, which is
// on PYTHONPATH as the project's own sources are, else relative to the
// directory. A path no module name spells is left out.
func PythonModules(tests []string) []string {
	var out []string
	for _, test := range tests {
		name := strings.TrimSuffix(strings.TrimPrefix(test, "src/"), ".py")
		parts := strings.Split(name, "/")
		valid := true
		for _, part := range parts {
			valid = valid && pythonIdentifier(part)
		}
		if valid {
			out = append(out, strings.Join(parts, "."))
		}
	}
	return out
}

func pythonIdentifier(s string) bool {
	for i, r := range s {
		if r != '_' && !unicode.IsLetter(r) && (i == 0 || !unicode.IsDigit(r)) {
			return false
		}
	}
	return s != ""
}

// pythonPlan measures dir's Python sources with tests, the test files that
// reach them as PythonTests gives them, or with the whole suite when tests
// is nil.
func pythonPlan(dir, out string, tests []string) Plan {
	py := pythonFor(dir)
	data := filepath.Join(out, ".coverage")
	report := filepath.Join(out, "lcov.info")
	runner := pythonRunner(exec.Command(py, "-c", "import pytest").Run() == nil, tests) //nolint:gosec // probe the project's own Python, never a downloaded executable
	plan := Plan{
		Language: "python",
		Dir:      dir,
		Reports:  []string{report},
		Existing: []string{report, filepath.Join(dir, "lcov.info"), filepath.Join(dir, "coverage", "lcov.info")},
	}
	if exec.Command(py, "-c", "import coverage").Run() != nil { //nolint:gosec // probe the project's own Python, never a downloaded executable
		plan.Unsupported = fmt.Sprintf("coverage.py is not installed for %s; install it there, or measure with --coverage-command", py)
		return plan
	}
	plan.Commands = pythonCommands(py, dir, data, report, runner)
	pythonIntegration(&plan, py, dir, out)
	return plan
}

// pythonCommands is the commands of a Python coverage plan: coverage run
// of runner, writing data, in dir; then the in-process data files a
// project's parallel data wrote beside data combined into it
// (pythonInProcessScript); then data's LCOV report, written to report,
// with no floor: the project's fail_under is its own test script's to
// enforce, which a run of one file's reaching tests would miss.
func pythonCommands(py, dir, data, report string, runner []string) [][]string {
	return [][]string{
		append([]string{py, "-m", "coverage", "run", "--branch", "--data-file=" + data, "--source=" + dir}, runner...),
		{py, "-c", pythonInProcessScript, data},
		{py, "-m", "coverage", "lcov", "--data-file=" + data, "-o", report, "--fail-under=0"},
	}
}

func pythonPlanSupervised(ctx context.Context, dir, out string, tests []string, execute CommandExecutor) (Plan, []CommandExecution, error) {
	py := pythonFor(dir)
	var executions []CommandExecution
	probe := func(ctx context.Context, module string) error {
		cmd := exec.CommandContext(ctx, py, "-c", "import "+module) //nolint:gosec // probe the project's own Python for a module this package names
		cmd.Dir, cmd.Stdout, cmd.Stderr = dir, io.Discard, io.Discard
		err := execute(ctx, cmd)
		executions = append(executions, CommandExecution{Args: append([]string{}, cmd.Args...), Dir: dir, Err: err})
		return err
	}
	pytestErr := probe(ctx, "pytest")
	if pytestErr != nil && ctx.Err() != nil {
		return Plan{}, executions, ctx.Err()
	}
	runner := pythonRunner(pytestErr == nil, tests)
	data := filepath.Join(out, ".coverage")
	report := filepath.Join(out, "lcov.info")
	plan := Plan{
		Language: "python", Dir: dir,
		Reports:  []string{report},
		Existing: []string{report, filepath.Join(dir, "lcov.info"), filepath.Join(dir, "coverage", "lcov.info")},
	}
	if err := probe(ctx, "coverage"); err != nil {
		if ctx.Err() != nil {
			return Plan{}, executions, ctx.Err()
		}
		plan.Unsupported = fmt.Sprintf("coverage.py is not installed for %s; install it there, or measure with --coverage-command", py)
		return plan, executions, nil
	}
	plan.Commands = pythonCommands(py, dir, data, report, runner)
	pythonIntegration(&plan, py, dir, out)
	return plan, executions, nil
}

// pythonFor prefers the project's own virtualenv, so tests run with the
// project's dependencies.
func pythonFor(dir string) string {
	for _, venv := range []string{".venv", "venv"} {
		py := filepath.Join(dir, venv, "bin", "python")
		if exists(py) {
			return py
		}
	}
	return "python3"
}
