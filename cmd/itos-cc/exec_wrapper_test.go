package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/covrun"
)

// itos-cc given covrun.ExecArg first is the wrapper go test -exec runs each
// test binary through for Go coverage: it runs the rest as a command and
// exits with that command's code, and given nothing to run says so and
// exits 2. It does so in init, before main and before TestMain, so only a
// built binary shows it, not this test binary: the test builds one, with
// -cover when this run measures coverage (coverDir) as features' harness
// builds its binary, and gives it that GOCOVERDIR, so what it executes is
// counted. The wrapper runs a second itos-cc, itself, to have a command
// that passes and one that fails on every system.
func TestTheBuiltBinaryRunsWhatGoTestHandsItsCoverageWrapper(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "itos-cc")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	args := []string{"build", "-o", bin}
	if coverDir != "" {
		args = append(args, "-cover", "-coverpkg=github.com/donvargax/itos-cc/cmd/itos-cc")
	}
	build := exec.Command("go", append(args, ".")...)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	run := func(args ...string) (int, string) {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Env = os.Environ()
		if coverDir != "" {
			cmd.Env = append(cmd.Env, "GOCOVERDIR="+coverDir)
		}
		out, err := cmd.CombinedOutput()
		var exit *exec.ExitError
		switch {
		case errors.As(err, &exit):
			return exit.ExitCode(), string(out)
		case err != nil:
			t.Fatalf("run %s: %v", bin, err)
		}
		return 0, string(out)
	}
	if code, out := run(covrun.ExecArg); code != 2 || !strings.Contains(out, covrun.ExecArg+" needs a command to run") {
		t.Errorf("the wrapper with nothing to run exited %d, saying %q; want 2 and that it needs a command", code, out)
	}
	if code, out := run(covrun.ExecArg, bin, "version"); code != 0 || !strings.HasPrefix(out, "itos-cc ") {
		t.Errorf("the wrapper running itos-cc version exited %d, saying %q; want 0 and the version", code, out)
	}
	if code, out := run(covrun.ExecArg, bin, covrun.ExecArg); code != 2 || !strings.Contains(out, "needs a command to run") {
		t.Errorf("the wrapper running a command that exits 2 exited %d, saying %q; want 2", code, out)
	}
}
