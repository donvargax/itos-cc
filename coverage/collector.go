package coverage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/donvargax/itos-cc/project"
)

// Integration coverage, in every language: what the processes a test starts
// execute, measured by the language's own collector into a directory of the
// run's own (Plan.CoverDir), turned into a report read beside the in-process
// one as Integration data, then removed. Go binaries built with -cover write
// to GOCOVERDIR (gocoverdir.go); Python processes start coverage.py through
// the .pth file coverage.py 7.13 and later install, which reads the rcfile
// COVERAGE_PROCESS_START names.
//
// Python: the coverage command itself runs as before, with the project's own
// configuration and itos-cc's flags. Before it, pythonStartScript reads the
// project's configuration with coverage.py's own reader, as coverage run
// reads it in the build root, and writes an rcfile in the run's directory:
// the project's [run] settings and plugin options, with itos-cc's on top
// (branch, the build root as source, parallel data, the data file in the run's
// directory), its relative paths made absolute, since the processes a test
// starts may run elsewhere. coverage.py reads one configuration file and
// includes none, so the layers are written out whole rather than included.
// The commands get COVERAGE_PROCESS_START naming it, and no COVERAGE_FILE or
// COVERAGE_PROCESS_CONFIG of the caller's, so every Python process the tests
// start writes its own data file there, apart from the in-process data
// (which coverage run writes where --data-file says, its own data file
// overriding COVERAGE_FILE): telling the two apart needs no contexts. The
// coverage run process starts a measurement from the rcfile too, as Python
// starts, which coverage run then stops from saving anything but an empty
// file. Once the commands ran, pythonCombineScript combines the files with
// the project's [paths] and writes their LCOV report with its report
// settings.
//
// [run] patch = subprocess, coverage.py's own switch since 7.10, is not used:
// it can only be set in a configuration file, which would replace the
// project's, it makes the subprocesses write beside the in-process data,
// where nothing tells them apart, and before 7.13 it writes .pth files into
// site-packages at run time.

// pythonStartScript, run with the project's python in its build root as
// python -c pythonStartScript RCFILE DATA ROOT..., writes the rcfile and
// checks that a Python process started with COVERAGE_PROCESS_START naming
// it measures itself. It exits 3 when one does not, printing why, as before
// coverage.py 7.13, which installs no .pth file.
const pythonStartScript = `import os, shutil, subprocess, sys, tempfile
import coverage

rc, data, roots = sys.argv[1], sys.argv[2], sys.argv[3:]
config = coverage.Coverage(data_file=data, branch=True, source=roots).config
mine = {"branch": True, "parallel": True, "data_file": os.path.abspath(data),
        "source": [os.path.abspath(root) for root in roots]}
paths = {"debug_file", "source_dirs"}
skip = {"command_line", "run_include", "_crash"}


def text(value):
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, (list, tuple)):
        return "".join("\n    " + text(v) for v in value)
    return str(value).replace("$", "$$")


lines = ["[run]"]
for spec in type(config).CONFIG_FILE_OPTIONS:
    attr, (section, option) = spec[0], spec[1].split(":")
    if section != "run" or attr in skip:
        continue
    value = mine.get(attr, getattr(config, attr, None))
    if attr in paths and value:
        value = [os.path.abspath(v) for v in value] if isinstance(value, (list, tuple)) else os.path.abspath(value)
    if attr == "run_omit" and value:
        value = [v if v.startswith(("*", "?")) else os.path.abspath(v) for v in value]
    if value is None or value == "" or value == []:
        continue
    lines.append(option + " = " + text(value))
for plugin, options in sorted((getattr(config, "plugin_options", None) or {}).items()):
    lines.append("[" + plugin + "]")
    lines.extend(key + " = " + text(value) for key, value in sorted(options.items()))
os.makedirs(os.path.dirname(os.path.abspath(rc)), exist_ok=True)
with open(rc, "w", encoding="utf-8") as f:
    f.write("\n".join(lines) + "\n")

probe = tempfile.mkdtemp()
env = dict(os.environ, COVERAGE_PROCESS_START=os.path.abspath(rc), COVERAGE_FILE=os.path.join(probe, ".coverage"))
env.pop("COVERAGE_PROCESS_CONFIG", None)
check = "import sys, coverage; sys.exit(0 if getattr(coverage.process_startup, 'coverage', None) is not None else 3)"
try:
    started = subprocess.run([sys.executable, "-c", check], env=env).returncode == 0
finally:
    shutil.rmtree(probe, ignore_errors=True)
if not started:
    print("coverage.py %s for %s does not start in the Python processes a test starts; "
          "subprocess measurement needs coverage.py 7.13 or later, whose .pth file starts it"
          % (coverage.__version__, sys.executable))
    sys.exit(3)
`

// pythonCombineScript, run with the project's python in its build root as
// python -c pythonCombineScript DATA REPORT, combines the data files beside
// DATA into it and writes their LCOV report to REPORT, an empty one when
// they measured none of the project's files.
const pythonCombineScript = `import os, sys
import coverage
from coverage.exceptions import NoDataError

data, report = os.path.abspath(sys.argv[1]), sys.argv[2]
cov = coverage.Coverage(data_file=data, data_suffix=False)
try:
    cov.combine([os.path.dirname(data)])
    cov.save()
    cov.lcov_report(outfile=report)
except NoDataError:
    open(report, "w").close()
`

// startExit is the exit code of a Plan's Prepare command that says the
// project's collector cannot measure the processes a test starts.
const startExit = 3

// pythonCoverEnv is the environment that has the Python processes a command
// starts measured by coverage.py with the rcfile rc: none of the caller's
// COVERAGE_FILE or COVERAGE_PROCESS_CONFIG, which would send their data
// elsewhere or configure them otherwise.
func pythonCoverEnv(rc string) []string {
	return []string{"COVERAGE_PROCESS_START=" + rc, "COVERAGE_PROCESS_CONFIG=", "COVERAGE_FILE="}
}

// pythonIntegration sets plan's integration coverage: the processes its
// tests start, measured by the coverage.py of py, write under out.
func pythonIntegration(plan *Plan, py, dir, out string) {
	plan.CoverDir = filepath.Join(out, "integration")
	rc := filepath.Join(plan.CoverDir, "coveragerc")
	data := filepath.Join(plan.CoverDir, ".coverage")
	plan.Prepare = []string{py, "-c", pythonStartScript, rc, data, dir}
	plan.CoverEnv = pythonCoverEnv(rc)
	plan.Written = ".coverage.*"
	plan.Integration = filepath.Join(out, "integration.info")
	plan.Convert = [][]string{{py, "-c", pythonCombineScript, data, plan.Integration}}
}

// setEnv is env with each of vars, KEY=VALUE, set in place of any value env
// holds for KEY; an empty VALUE unsets KEY.
func setEnv(env []string, vars ...string) []string {
	out := append([]string{}, env...)
	for _, v := range vars {
		key, value, _ := strings.Cut(v, "=")
		out = deleteEnvKey(out, key)
		if value != "" {
			out = append(out, v)
		}
	}
	return out
}

// unsetEnv is env without any of the variables vars, KEY=VALUE, name.
func unsetEnv(env []string, vars ...string) []string {
	out := append([]string{}, env...)
	for _, v := range vars {
		key, _, _ := strings.Cut(v, "=")
		out = deleteEnvKey(out, key)
	}
	return out
}

func deleteEnvKey(env []string, key string) []string {
	out := env[:0]
	for _, e := range env {
		k, _, _ := strings.Cut(e, "=")
		if k == key || runtime.GOOS == "windows" && strings.EqualFold(k, key) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// written says whether the processes p's tests start wrote data to
// p.CoverDir.
func (p Plan) written() bool {
	if p.CoverDir == "" || p.Written == "" {
		return false
	}
	found, _ := filepath.Glob(filepath.Join(p.CoverDir, p.Written))
	return len(found) > 0
}

// start readies p's collector of the processes its tests start: its
// Prepare command, run with env less CoverEnv's variables, through execute
// when execute is not nil. It returns why the collector cannot measure them
// ("" when it can) and an error only when Prepare failed otherwise.
func (p Plan) start(ctx context.Context, env []string, log io.Writer, execute CommandExecutor) (string, CommandExecution, error) {
	if len(p.Prepare) == 0 {
		return "", CommandExecution{}, nil
	}
	cmd := exec.CommandContext(ctx, p.Prepare[0], p.Prepare[1:]...)
	cmd.Dir, cmd.Env = p.Dir, unsetEnv(env, p.CoverEnv...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, log
	var err error
	if execute != nil {
		err = execute(ctx, cmd)
	} else {
		err = cmd.Run()
	}
	call := CommandExecution{Args: []string{p.Prepare[0], "<start integration coverage>"}, Dir: p.Dir, Err: err}
	why := strings.TrimSpace(out.String())
	if exit := (*exec.ExitError)(nil); errors.As(err, &exit) && exit.ExitCode() == startExit && why != "" {
		return why, call, nil
	}
	if err != nil {
		log.Write(out.Bytes())
		return "", call, fmt.Errorf("start %s integration coverage: %w", p.Language, err)
	}
	return "", call, nil
}

// noIntegration is the integration coverage p's collector could not
// measure, logged.
func (p Plan) noIntegration(why string, log io.Writer) Unmeasured {
	fmt.Fprintf(log, "itos-cc: coverage: %s: going on without integration coverage: %s\n", p.Dir, why)
	return Unmeasured{Dir: p.Dir, Language: p.Language, Cause: ToolMissing, Reason: "no integration coverage: " + why}
}

// IntegrationMissing is each build root whose collector could not measure
// the processes its tests start, such as a coverage.py that does not start
// in them: its in-process coverage stands, without integration coverage.
func (r *Report) IntegrationMissing() []Unmeasured {
	if r == nil {
		return nil
	}
	return r.integrationMissing
}

// removeCoverDir removes p's CoverDir, if it has one.
func (p Plan) removeCoverDir() error {
	if p.CoverDir == "" {
		return nil
	}
	return os.RemoveAll(p.CoverDir)
}

// convertEnv is the environment of p's Convert commands: none of CoverEnv's
// variables, so the conversion measures nothing itself.
func (p Plan) convertEnv() []string {
	return unsetEnv(project.NoBytecodeEnv(os.Environ()), p.CoverEnv...)
}

// displayArgs is args as a log line shows them: a script python -c runs as
// <script>.
func displayArgs(args []string) string {
	shown := append([]string{}, args...)
	for i := 1; i+1 < len(shown); i++ {
		if shown[i] == "-c" && strings.Contains(shown[i+1], "\n") {
			shown[i+1] = "<script>"
		}
	}
	return strings.Join(shown, " ")
}

// TypeScript: while the coverage command runs Vitest or Jest, NODE_V8_COVERAGE
// names a directory of the run's own, so every Node process the tests start
// writes the V8 coverage of what it ran there as it exits, and the project's
// own c8 (node_modules/.bin/c8, never fetched) reports it as LCOV, through
// the source maps Node records with it, under the project's c8
// configuration and itos-cc's flags. The test runner's own processes write
// there too, as they inherit the variable: Vitest's main process does, and
// a pool's worker or Jest's would; what they ran in-process is the runner's
// own report's to count. A raw file whose process loaded the runner's own
// modules (nodeRunnerScripts) is dropped before c8 reads the rest, so
// "integration" is only what child processes ran. No filter by process ID
// is needed, nor a wrapper that sets the variable for children alone.
// NODE_V8_COVERAGE is in every Node since 10.12, older than any Vitest or
// Jest itos-cc runs.

// nodeRunnerScripts are what a script URL of Vitest's or Jest's own
// processes holds.
var nodeRunnerScripts = []string{"/node_modules/vitest/", "/node_modules/@vitest/", "/node_modules/jest/", "/node_modules/jest-", "/node_modules/@jest/"}

// typescriptIntegration sets plan's integration coverage: the Node
// processes the tests of the package at dir start, reported by its c8,
// write under out. Without c8 it says so (CollectorMissing).
func typescriptIntegration(plan *Plan, dir, out string) {
	c8 := project.NodeBin(dir, "c8")
	if c8 == "" {
		plan.CollectorMissing = "c8 is not installed, which reads the V8 coverage of the Node processes the tests start; add c8 to devDependencies"
		return
	}
	plan.CoverDir = filepath.Join(out, "integration")
	plan.CoverEnv = []string{"NODE_V8_COVERAGE=" + plan.CoverDir}
	plan.Written = "coverage-*.json"
	plan.RunnerScripts = nodeRunnerScripts
	reports := filepath.Join(out, "integration-report")
	plan.Integration = filepath.Join(reports, "lcov.info")
	plan.Convert = [][]string{c8Report(c8, plan.CoverDir, reports)}
}

// c8Report is the command that has c8 write the LCOV report of the raw V8
// coverage in dir to reports/lcov.info.
func c8Report(c8, dir, reports string) []string {
	return []string{c8, "report", "--temp-directory=" + dir, "--reporter=lcov", "--reports-dir=" + reports}
}

// dropRunnerData removes each raw V8 coverage file in p.CoverDir that a
// process of the test runner wrote: one that loaded a script whose URL
// holds one of p.RunnerScripts.
func (p Plan) dropRunnerData() error {
	if p.CoverDir == "" || len(p.RunnerScripts) == 0 {
		return nil
	}
	files, err := filepath.Glob(filepath.Join(p.CoverDir, p.Written))
	if err != nil {
		return fmt.Errorf("inspect integration coverage data: %w", err)
	}
	for _, file := range files {
		runner, err := loadedAny(file, p.RunnerScripts)
		if err != nil {
			return fmt.Errorf("read V8 coverage %s: %w", file, err)
		}
		if runner {
			if err := os.Remove(file); err != nil {
				return err
			}
		}
	}
	return nil
}

// loadedAny says whether the raw V8 coverage file names a script whose URL
// holds one of fragments.
func loadedAny(file string, fragments []string) (bool, error) {
	f, err := os.Open(file)
	if err != nil {
		return false, err
	}
	defer f.Close()
	var raw struct {
		Result []struct {
			URL string `json:"url"`
		} `json:"result"`
	}
	if err := json.NewDecoder(f).Decode(&raw); err != nil {
		return false, err
	}
	for _, script := range raw.Result {
		for _, fragment := range fragments {
			if strings.Contains(script.URL, fragment) {
				return true, nil
			}
		}
	}
	return false, nil
}
