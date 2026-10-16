package testtools_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/internal/testtools"
)

// This helper is a real executable probe, not evidence of an installed Python
// runtime. It makes caller cwd, argv, environment and unsuccessful import
// observations independent of a requested test command.
func TestMain(m *testing.M) {
	if os.Getenv("T89_PROBE_HELPER") == "1" {
		cwd, err := os.Getwd()
		if err != nil {
			os.Exit(91)
		}
		data, err := json.Marshal(map[string]any{"cwd": cwd, "args": os.Args[1:], "pythonpath": os.Getenv("PYTHONPATH")})
		if err != nil {
			os.Exit(92)
		}
		if err := os.WriteFile(os.Getenv("T89_PROBE_LOG"), data, 0600); err != nil {
			os.Exit(93)
		}
		if _, err := os.Stat(filepath.Join(cwd, "pytest-present")); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestActualProbeUsesCallerCwdEnvironmentAndLastInterpreter(t *testing.T) {
	root := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, venv := range []string{".venv", "venv"} {
		interpreter := filepath.Join(root, venv, "bin", "python")
		if err := os.MkdirAll(filepath.Dir(interpreter), 0755); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(executable)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(interpreter, data, 0755); err != nil {
			t.Fatal(err)
		}
		// Windows resolves PATHEXT while the legacy policy observes the bare
		// marker. Keep identical native bytes at both names on every platform.
		if err := os.WriteFile(interpreter+".exe", data, 0755); err != nil {
			t.Fatal(err)
		}

	}
	if err := os.WriteFile(filepath.Join(root, "pyproject.toml"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	caller := filepath.Join(root, "nested-caller")
	if err := os.MkdirAll(caller, 0755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(root, "probe.json")
	tools := testtools.Toolchain{Dir: caller, Env: append(os.Environ(), "T89_PROBE_HELPER=1", "T89_PROBE_LOG="+log, "PYTHONPATH=independent-import-path")}
	request := mutants.CommandRequest{Path: filepath.Join(root, "a.py"), All: true}
	for _, present := range []bool{false, true} {
		if present {
			if err := os.WriteFile(filepath.Join(caller, "pytest-present"), nil, 0600); err != nil {
				t.Fatal(err)
			}
		}
		facts, failure := tools.Observe(t.Context(), request)
		if failure != nil || facts.Pytest != present || facts.Interpreter != filepath.Join(root, "venv", "bin", "python") {
			t.Fatalf("actual probe facts: %+v %v", facts, failure)
		}
		data, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		var observed struct {
			Cwd        string
			Args       []string
			Pythonpath string
		}
		if err := json.Unmarshal(data, &observed); err != nil {
			t.Fatal(err)
		}
		canonical, err := filepath.EvalSymlinks(caller)
		if err != nil {
			t.Fatal(err)
		}
		if observed.Cwd != canonical || !reflect.DeepEqual(observed.Args, []string{"-c", "import pytest"}) || observed.Pythonpath != "independent-import-path" {
			t.Fatalf("actual probe request: %+v", observed)
		}
	}
	if err := os.Remove(log); err != nil {
		t.Fatal(err)
	}
	request.Shell = "./run.sh"
	command, failure := mutants.ObserveCommand(t.Context(), tools, request)
	if failure != nil || command.Shell != "./run.sh" {
		t.Fatalf("shell override: %+v %v", command, failure)
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatalf("unused product probe executed: %v", err)
	}
	if !strings.Contains(command.Key(), "run.sh") {
		t.Fatal("shell key lost")
	}
}
