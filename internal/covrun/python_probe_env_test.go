package covrun

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// Every import probe of the project's Python runs with Python told to write
// no bytecode (T-240): a project holding a pytest.py of its own would have the
// probe leave __pycache__/pytest.*.pyc in its tree.
func TestThePytestProbeOfAPlanWritesNoBytecode(t *testing.T) {
	var rec recorder
	if _, _, err := pythonPlanSupervised(t.Context(), t.TempDir(), t.TempDir(), nil, rec.execute); err != nil {
		t.Fatal(err)
	}
	if len(rec.commands) == 0 {
		t.Fatal("the plan probed nothing")
	}
	for _, cmd := range rec.commands {
		if !slices.Contains(cmd.Env, "PYTHONDONTWRITEBYTECODE=1") {
			t.Errorf("%q probes with an environment that lets Python write bytecode", cmd.Args)
		}
	}
}

func TestTheImportProbeOfTheUnsupervisedPlanWritesNoBytecode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the probe is a shell script")
	}
	dir := t.TempDir()
	seen := filepath.Join(dir, "seen")
	py := filepath.Join(dir, "python")
	if err := os.WriteFile(py, []byte("#!/bin/sh\nprintf %s \"$PYTHONDONTWRITEBYTECODE\" > "+seen+"\n"), 0o700); err != nil { //nolint:gosec // a probe the test runs
		t.Fatal(err)
	}
	if !pythonImports(py, "pytest") {
		t.Fatal("the probe failed")
	}
	if got, _ := os.ReadFile(seen); string(got) != "1" {
		t.Errorf("the probe saw PYTHONDONTWRITEBYTECODE=%q, not 1", got)
	}
}

// A project without pytest is measured with unittest, and only a run
// cancelled during the probe ends the plan with the cancellation.
func TestAPlanWithoutPytestRunsUnittestAndACancelledOneStops(t *testing.T) {
	missing := func(_ context.Context, cmd *exec.Cmd) error {
		if slices.Contains(cmd.Args, "import pytest") {
			return errors.New("no module named pytest")
		}
		return nil
	}
	plan, _, err := pythonPlanSupervised(t.Context(), t.TempDir(), t.TempDir(), nil, missing)
	if err != nil || len(plan.Commands) == 0 || !slices.Contains(plan.Commands[0], "unittest") {
		t.Fatalf("a project without pytest: plan %+v, error %v, want unittest's commands", plan, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := pythonPlanSupervised(ctx, t.TempDir(), t.TempDir(), nil, missing); !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled plan ended with %v, not the cancellation", err)
	}
}
