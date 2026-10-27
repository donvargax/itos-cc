package covrun

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The coverage commands itos-cc starts begin from project.InheritedEnv, so
// a GOCOVERDIR set for itos-cc reaches none of them (slice-59, q-96): the
// static inventory, the Go scope listing, a supervised plan's commands,
// the listed tests' runs and their conversions, supervised or not, and a
// person's --coverage-command. Each is started here through a recording
// executor that runs nothing, where the real tools (coverage.py, Vitest, a
// listed test) are not in reach of a package test.

// recorder is a CommandExecutor that runs nothing: it keeps each command
// and, through also, does what the test needs of it.
type recorder struct {
	commands []*exec.Cmd
	also     func(*exec.Cmd)
}

func (r *recorder) execute(_ context.Context, cmd *exec.Cmd) error {
	r.commands = append(r.commands, cmd)
	if r.also != nil {
		r.also(cmd)
	}
	return nil
}

// inheritCoverDir sets GOCOVERDIR as itos-cc's own, to a folder of its own.
func inheritCoverDir(t *testing.T) {
	t.Helper()
	t.Setenv("GOCOVERDIR", filepath.Join(t.TempDir(), "inherited"))
}

// noCoverDir fails t when any of commands has a GOCOVERDIR, or when none
// was started.
func noCoverDir(t *testing.T, what string, commands []*exec.Cmd) {
	t.Helper()
	if len(commands) == 0 {
		t.Fatalf("%s started no command", what)
	}
	for _, cmd := range commands {
		if cmd.Env == nil {
			t.Errorf("%s's %q inherits itos-cc's whole environment", what, cmd.Args)
		}
		for _, kv := range cmd.Env {
			if strings.HasPrefix(kv, "GOCOVERDIR=") {
				t.Errorf("%s's %q holds %s", what, cmd.Args, kv)
			}
		}
	}
}

func TestTheConversionEnvironmentHoldsNoGOCOVERDIR(t *testing.T) {
	inheritCoverDir(t)
	env := Plan{CoverEnv: []string{"NODE_V8_COVERAGE=elsewhere"}}.convertEnv()
	noCoverDir(t, "a conversion", []*exec.Cmd{{Args: []string{"convert"}, Env: env}})
	if slices.Contains(env, "NODE_V8_COVERAGE=elsewhere") || !slices.Contains(env, "PYTHONDONTWRITEBYTECODE=1") {
		t.Errorf("the conversion environment %q keeps CoverEnv or lets Python write bytecode", env)
	}
}

func TestTheStaticInventoryHoldsNoGOCOVERDIR(t *testing.T) {
	inheritCoverDir(t)
	dir := t.TempDir()
	lonely := filepath.Join(dir, "lonely.py")
	if err := os.WriteFile(lonely, []byte("def lonely():\n    return 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := (Plan{Language: "python", Dir: dir, Sources: []string{lonely}, Unreached: true}).unreached(io.Discard)
	var rec recorder
	if _, err := Inventory(context.Background(), r, "python", io.Discard, rec.execute); err != nil {
		t.Fatal(err)
	}
	noCoverDir(t, "the inventory", rec.commands)
}

func TestTheGoScopeListingHoldsNoGOCOVERDIR(t *testing.T) {
	inheritCoverDir(t)
	var rec recorder
	// The recorder lists nothing, which fails the scope after go list.
	_, _, _, _ = goScopeSupervised(context.Background(), t.TempDir(), []string{"a.go"}, rec.execute)
	noCoverDir(t, "go list", rec.commands)
}

func TestASupervisedPlansCommandsHoldNoGOCOVERDIR(t *testing.T) {
	inheritCoverDir(t)
	dir := t.TempDir()
	plan := Plan{Language: "go", Dir: dir, Commands: [][]string{{"go", "test", "./..."}}, Reports: []string{filepath.Join(dir, "out", "coverage.out")}}
	var rec recorder
	// No report is written, which fails the run after its command.
	_, _, _ = RunSupervised(context.Background(), []Plan{plan}, nil, io.Discard, rec.execute)
	noCoverDir(t, "the plan", rec.commands)
}

// listed is a listed-tests run of one test, a, at root, and a recorder
// that writes Go coverage data where the whole run's environment says,
// and prints a covdata that is not there where it is looked up.
func listed(root string) (PerTest, *recorder) {
	p := PerTest{Root: root, Tests: []Test{{ID: "a"}}, All: "every test", Select: func([]string) string { return "one test" }}
	rec := &recorder{also: func(cmd *exec.Cmd) {
		printsCovdata(cmd, missingCovdata)
		for _, kv := range cmd.Env {
			if dir, ok := strings.CutPrefix(kv, TestCoverDirEnv+"="); ok {
				_ = os.MkdirAll(filepath.Join(dir, "a"), 0o750)
				_ = os.WriteFile(filepath.Join(dir, "a", "covmeta.0"), nil, 0o600)
			}
		}
	}}
	return p, rec
}

func TestTheListedTestsAndTheirConversionsHoldNoGOCOVERDIR(t *testing.T) {
	inheritCoverDir(t)
	root := t.TempDir()
	p, rec := listed(root)
	sources := goModule(t, root)
	// The recorder writes no profile, which fails the run after the
	// conversion.
	_, _, _ = MeasureTestsSupervised(context.Background(), p, filepath.Join(root, "data"), sources, io.Discard, rec.execute)
	noCoverDir(t, "the supervised listed tests", rec.commands)
	if len(rec.commands) != 3 {
		t.Errorf("started %d commands, want the run of every test, its lookup of covdata and its conversion", len(rec.commands))
	}

	p, rec = listed(root)
	var log bytes.Buffer
	// The conversion of what the recorder wrote, by a covdata that is not
	// there, fails, which is logged.
	_, _ = MeasureTests(context.Background(), p, filepath.Join(root, "data"), sources, &log, rec.execute)
	noCoverDir(t, "the listed tests", rec.commands)
	if !strings.Contains(log.String(), "covdata") {
		t.Errorf("the conversion was not started: %s", log.String())
	}
}

func TestACoverageCommandRuns(t *testing.T) {
	inheritCoverDir(t)
	var log bytes.Buffer
	if err := Shell("exit 0", &log); err != nil {
		t.Errorf("exit 0 failed: %v\n%s", err, log.String())
	}
}
