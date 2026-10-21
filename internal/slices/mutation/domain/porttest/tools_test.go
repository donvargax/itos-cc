package porttest

import (
	"path/filepath"
	"strings"
	"testing"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

func TestProbeFactsRequireActualInterpreterCallerAndEnvironment(t *testing.T) {
	b := memoryInputFixture(t, map[string]string{"pyproject.toml": "", "a.py": "def a(): return 1\n", ".venv/bin/python": "", "venv/bin/python": ""}, nil, nil)
	input := b.Toolchain.(*Inputs)
	input.Dir = filepath.Join(b.Root, "nested-caller")
	request := mutants.CommandRequest{Path: filepath.Join(b.Root, "a.py"), All: true}
	interpreter := filepath.Join(b.Root, "venv", "bin", "python")
	input.Probes = map[PytestProbe]bool{ProbeKey(filepath.Join(b.Root, ".venv", "bin", "python"), input.Dir, nil): true, ProbeKey(interpreter, b.Root, nil): true}
	if _, failure := input.Observe(t.Context(), request); failure == nil {
		t.Fatal("wrong interpreter/caller probe bound")
	}
	input.Probes[ProbeKey(interpreter, input.Dir, nil)] = false
	command, failure := mutants.ObserveCommand(t.Context(), input, request)
	if failure != nil || command.Args[2] != "unittest" {
		t.Fatalf("explicit failed import: %+v %v", command, failure)
	}
	input.Runtime.Env = map[string]string{"PYTHONPATH": "editable-src"}
	if _, failure := input.Observe(t.Context(), request); failure == nil {
		t.Fatal("wrong environment probe reused")
	}
	input.Probes[ProbeKey(interpreter, input.Dir, input.Runtime.Env)] = true
	command, failure = mutants.ObserveCommand(t.Context(), input, request)
	if failure != nil || command.Args[2] != "pytest" {
		t.Fatalf("actual independent import success: %+v %v", command, failure)
	}

	input.Without = []string{"python*"}
	if observed, failure := input.Observe(t.Context(), request); failure != nil || !observed.Pytest {
		t.Fatalf("explicit venv interpreter hidden by PATH filter: %+v %v", observed, failure)
	}
	request.Shell = "./run.sh"
	input.Probes = nil
	if _, failure := input.Observe(t.Context(), request); failure != nil {
		t.Fatal("shell override probed unused pytest", failure)
	}
}

func TestWorkersPathHidingPreservesExplicitExecutablesAndShellOutcomes(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "board")
	explicit := filepath.Join(root, "node_modules", ".bin", "vitest")
	workers := &Workers{Platform: "linux", Files: map[string]string{filepath.Join(root, "a.ts"): "actual bytes"}, Without: []string{"vitest", "go"}, Tests: map[string]map[string]Tests{root: {explicit + " run": {}, "go version": {}, "go version && true": {}}}}
	pool, failure := workers.Open(t.Context(), mutants.MutationRun, 1)
	if failure != nil {
		t.Fatal(failure)
	}
	defer func() {
		if failure := pool.Close(t.Context()); failure != nil {
			t.Error(failure)
		}
	}()
	if result, failure := pool.Worker(0).Run(t.Context(), mutants.Command{Root: root, Dir: root, Args: []string{explicit, "run"}}, nil, 0); failure != nil || !result.Passed {
		t.Fatalf("explicit installed entry hidden: %+v %v", result, failure)
	}
	if _, failure := pool.Worker(0).Run(t.Context(), mutants.Command{Root: root, Dir: root, Args: []string{"go", "version"}}, nil, 0); failure == nil {
		t.Fatal("bare PATH lookup silently executed")
	}
	if result, failure := pool.Worker(0).Run(t.Context(), mutants.Command{Root: root, Dir: root, Shell: "go version"}, nil, 0); failure != nil || result.Passed || result.ExitCode != 127 {
		t.Fatalf("shell absence became direct-start failure: %+v %v", result, failure)
	}

	workers.Platform = "windows"
	if _, failure := pool.Worker(0).Run(t.Context(), mutants.Command{Root: root, Dir: root, Shell: "go version"}, nil, 0); failure == nil || !strings.Contains(failure.Error(), "unsupported Windows shell") {
		t.Fatalf("POSIX shell exit invented on Windows: %v", failure)
	}
	workers.Platform = "linux"
	if _, failure := pool.Worker(0).Run(t.Context(), mutants.Command{Root: root, Dir: root, Shell: "go version && true"}, nil, 0); failure == nil {
		t.Fatal("unsupported compound shell hiding silently executed")
	}
	if !hiddenProgram("Go.EXE", []string{"go"}, "windows") || hiddenProgram("./Go.EXE", []string{"go"}, "windows") || !hiddenProgram("itos-example", []string{"itos-*"}, "linux") {
		t.Fatal("native PATH-name matching changed")
	}
}
