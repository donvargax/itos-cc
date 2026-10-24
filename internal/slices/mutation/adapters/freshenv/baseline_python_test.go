package freshenv

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/donvargax/itos-cc/internal/covrun"
)

// A selected Python file's fresh baseline probes whether pytest imports
// through the executor it is given, never by itself, records that probe,
// and runs the test files that reach the file in the project's root: with
// pytest when the probe passed, else unittest over their modules. A file
// no test reaches has no baseline and runs no probe. The probe's command is
// the frozen root's interpreter, python3 with no virtual environment.
func TestAFreshPythonBaselineProbesPytestThroughItsExecutor(t *testing.T) {
	root := t.TempDir()
	for name, text := range map[string]string{
		"pyproject.toml":      "[project]\nname = \"fixture\"\n",
		"src/mod.py":          "def one():\n    return 1\n",
		"tests/test_mod.py":   "from mod import one\n\ndef test_one():\n    assert one() == 1\n",
		"tests/helper_mod.py": "",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	plan := &FreshPlan{FrozenRoot: root, Selected: []FreshCandidate{{Path: "src/mod.py"}}}
	reaching := []string{filepath.Join(root, "tests", "helper_mod.py"), filepath.Join(root, "tests", "test_mod.py")}
	probe := []string{"python3", "-c", "import pytest"}
	noPytest := errors.New("no module named pytest")
	for _, c := range []struct {
		name     string
		reaching []string
		probe    error
		want     []string
	}{
		{"pytest", reaching, nil, []string{"python3", "-m", "pytest", "-q", "-x", "-p", "no:cacheprovider", "tests/test_mod.py"}},
		{"unittest", reaching, noPytest, []string{"python3", "-m", "unittest", "-f", "tests.test_mod"}},
		{"unreached", nil, nil, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			var ran [][]string
			execute := func(_ context.Context, cmd *exec.Cmd) error {
				if cmd.Dir != root {
					t.Errorf("the probe ran in %s, not the project's root %s", cmd.Dir, root)
				}
				ran = append(ran, cmd.Args)
				return c.probe
			}
			tests := func(path string) []string {
				if path != filepath.Join(root, "src", "mod.py") {
					t.Errorf("asked for the tests reaching %s", path)
				}
				return c.reaching
			}
			baselines, executions, err := Environment{}.baselineCommands(t.Context(), plan, false, tests, execute)
			if err != nil {
				t.Fatal(err)
			}
			if c.want == nil {
				if len(baselines) != 0 || len(executions) != 0 || len(ran) != 0 {
					t.Fatalf("a file no test reaches has baselines %v, executions %v and probes %v", baselines, executions, ran)
				}
				return
			}
			if len(ran) != 1 || !slices.Equal(ran[0], probe) {
				t.Fatalf("probes %v, want one, %v", ran, probe)
			}
			want := covrun.CommandExecution{Args: probe, Dir: root, Err: c.probe}
			if len(executions) != 1 || !slices.Equal(executions[0].Args, want.Args) || executions[0].Dir != want.Dir || !errors.Is(executions[0].Err, want.Err) {
				t.Fatalf("executions %+v, want one, %+v", executions, want)
			}
			if len(baselines) != 1 || !slices.Equal(baselines[0].Command.Args, c.want) || baselines[0].Command.Dir != root || !slices.Equal(baselines[0].Paths, []string{"src/mod.py"}) {
				t.Fatalf("baselines %+v, want one running %v in %s for src/mod.py", baselines, c.want, root)
			}
		})
	}
}
