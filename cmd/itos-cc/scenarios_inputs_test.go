package main

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/features"
	mutation "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	mutantstest "github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

func TestMemoryReadObservationUsesActualBoundTree(t *testing.T) {
	m := newMemory(t)
	root, err := m.Folder()
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "board.go")
	outside := filepath.Join(root, "outside.txt")
	if err := m.Write(source, []byte("package board\nfunc Value() int { return 1 }\n")); err != nil {
		t.Fatal(err)
	}
	if err := m.Write(outside, []byte("nonempty external marker")); err != nil {
		t.Fatal(err)
	}
	observation, err := m.ObserveReads(source, outside, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Read(source); err != nil {
		t.Fatal(err)
	}
	if read, err := observation(); err != nil || len(read) != 0 {
		t.Fatalf("assertion read counted: %v %v", read, err)
	}
	actual, err := m.Run(features.Run{Dir: root, Args: []string{"units", "--json", "board.go"}})
	if err != nil || actual.Exit != 0 {
		t.Fatalf("actual units run: %+v %v", actual, err)
	}
	read, err := observation()
	if err != nil || !reflect.DeepEqual(read, []string{source}) {
		t.Fatalf("bound source read missing or outside admitted: %v %v", read, err)
	}
	directory, err := m.ObserveReads(root)
	if err != nil {
		t.Fatal(err)
	}
	actual, err = m.Run(features.Run{Dir: root, Args: []string{"units", "--json", "."}})
	if err != nil || actual.Exit != 0 {
		t.Fatalf("actual directory run: %+v %v", actual, err)
	}
	if read, err := directory(); err != nil || !reflect.DeepEqual(read, []string{root}) {
		t.Fatalf("bound listing missing: %v %v", read, err)
	}
	if err := m.Remove(source); err != nil {
		t.Fatal(err)
	}
	if _, err := observation(); err == nil {
		t.Fatal("removed target observation silently passed")
	}
}

func TestMemoryIndependentSetupSurvivesCommandUpsert(t *testing.T) {
	m := newMemory(t)
	root, err := m.Folder()
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{"package.json": `{"devDependencies":{"vitest":"*"}}`, "a.ts": "export const a=1;", "a.test.ts": "import {a} from './a';"} {
		if err := m.Write(filepath.Join(root, name), []byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	facts := features.Tests{Imports: map[string][]string{"a.ts": {}, "a.test.ts": {"a.ts"}}}
	if err := m.Tests(root, facts); err != nil {
		t.Fatal(err)
	}
	if err := m.Tests(root, features.Tests{Language: "go", Command: "go version"}); err != nil {
		t.Fatal(err)
	}
	input := m.inputs(features.Run{Dir: root})
	source := filepath.Join(root, "a.ts")
	test := filepath.Join(root, "a.test.ts")
	reached, failure := input.TestsImporting(t.Context(), root, []string{source}, []string{test})
	if failure != nil || !reflect.DeepEqual(reached[source], []string{test}) {
		t.Fatalf("actual adjacency erased: %v %v", reached, failure)
	}
	request := mutation.CommandRequest{Path: source}
	command, failure := mutation.ObserveCommand(t.Context(), input, request)
	if failure != nil || command.Args[0] != "npm" {
		t.Fatalf("outcome inferred installed runner: %+v %v", command, failure)
	}
	if err := m.Tools(t.Context(), root, features.ToolSetup{NodeBins: []string{"vitest"}}); err != nil {
		t.Fatal(err)
	}
	command, failure = mutation.ObserveCommand(t.Context(), m.inputs(features.Run{Dir: root}), request)
	if failure != nil || !strings.Contains(command.Args[0], "vitest") {
		t.Fatalf("actual installed entry not bound: %+v %v", command, failure)
	}
	delete(m.files, command.Args[0])
	command, failure = mutation.ObserveCommand(t.Context(), m.inputs(features.Run{Dir: root}), request)
	if failure != nil || command.Args[0] != "npm" {
		t.Fatalf("removed actual entry hidden: %+v %v", command, failure)
	}
	wrong := features.Tests{Imports: map[string][]string{"a.test.ts": {}}}
	if err := m.Tests(root, wrong); err == nil {
		t.Fatal("conflicting adjacency accepted")
	}
	delete(m.imports[root], test)
	if _, failure := m.inputs(features.Run{Dir: root}).TestsImporting(t.Context(), root, []string{source}, []string{test}); failure == nil {
		t.Fatal("missing adjacency became empty reach")
	}
	invalid := features.Tests{Imports: map[string][]string{"a.ts": {}}, Baseline: "cannot execute"}
	fresh := newMemory(t)
	fresh.files = m.files
	if err := fresh.Tests(root, invalid); err == nil || len(fresh.imports) != 0 {
		t.Fatal("invalid declaration partially installed adjacency")
	}
}

func TestMemoryRuntimeInputsAreActualRunFacts(t *testing.T) {
	m := newMemory(t)
	root, err := m.Folder()
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{"go.mod": "module board\n", "go.work": "go 1.27\nuse .\n", "a.go": "package board\n", "pyproject.toml": ""} {
		if err := m.Write(filepath.Join(root, name), []byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(root, "a.go")
	normal := m.inputs(features.Run{Dir: root})
	if reason, failure := normal.Unsupported(t.Context(), "go", path, root); failure != nil || reason == "" {
		t.Fatalf("workspace control: %q %v", reason, failure)
	}
	off := m.inputs(features.Run{Dir: root, Env: []string{"GOWORK=off", "PYTEST_DISABLE_PLUGIN_AUTOLOAD=1"}, Without: []string{"python3"}})
	if reason, failure := off.Unsupported(t.Context(), "go", path, root); failure != nil || reason != "" {
		t.Fatalf("actual GOWORK=off lost: %q %v", reason, failure)
	}
	if off.Runtime.Env["PYTEST_DISABLE_PLUGIN_AUTOLOAD"] != "1" || !reflect.DeepEqual(off.Without, []string{"python3"}) {
		t.Fatal("Run facts lost")
	}
	setup := features.ToolSetup{Python: &features.PythonProbe{Interpreter: "python3", Importable: true}, Env: []string{"PYTEST_DISABLE_PLUGIN_AUTOLOAD=1"}}
	if err := m.Tools(t.Context(), root, setup); err != nil {
		t.Fatal(err)
	}
	request := mutation.CommandRequest{Path: filepath.Join(root, "a.py"), All: true}
	if _, failure := normal.Observe(t.Context(), request); failure == nil {
		t.Fatal("wrong env imported pytest")
	}
	tools := m.inputs(features.Run{Dir: root, Env: setup.Env})
	observed, failure := tools.Observe(t.Context(), request)
	if failure != nil || !observed.Pytest {
		t.Fatalf("actual env setup not bound: %+v %v", observed, failure)
	}
	cause := errors.New("caller stopped")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	if err := m.Tools(ctx, root, features.ToolSetup{}); !errors.Is(err, cause) {
		t.Fatalf("setup cancellation: %v", err)
	}
	// Missing input remains a typed observation error, never a desired verdict.
	var failed mutation.InputFailed
	_, failure = (&mutantstest.Inputs{}).TestsImporting(t.Context(), root, nil, nil)
	if !errors.As(failure, &failed) {
		t.Fatalf("missing graph setup: %v", failure)
	}
}

func TestMemoryWorkersReceiveActualRunEnvironmentAndWithout(t *testing.T) {
	m := newMemory(t)
	root, err := m.Folder()
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Write(filepath.Join(root, "a.py"), []byte("def a(): return 1\n")); err != nil {
		t.Fatal(err)
	}
	if err := m.Tests(root, features.Tests{Language: "go", Command: "python3 -m pytest -q"}); err != nil {
		t.Fatal(err)
	}
	actual := features.Run{Dir: root, Env: []string{"PYTEST_DISABLE_PLUGIN_AUTOLOAD=1", "GOWORK=off", "PYTHONPATH=editable-live-src"}}
	workers := m.workers(actual)
	pool, failure := workers.Open(t.Context(), mutation.MutationRun, 1)
	if failure != nil {
		t.Fatal(failure)
	}
	defer func() {
		if failure := pool.Close(t.Context()); failure != nil {
			t.Error(failure)
		}
	}()
	command := mutation.Command{Root: root, Dir: root, Args: []string{"python3", "-m", "pytest", "-q"}, PathEnv: "PYTHONPATH", PathDirs: []string{".", "src"}}
	if _, failure := pool.Worker(0).Run(t.Context(), command, nil, mutation.Limits{}); failure != nil {
		t.Fatal(failure)
	}
	if len(workers.Environments) != 1 {
		t.Fatal("no actual worker request")
	}
	env := workers.Environments[0]
	if env["PYTEST_DISABLE_PLUGIN_AUTOLOAD"] != "1" || env["GOWORK"] != "off" || !strings.HasSuffix(env["PYTHONPATH"], string(filepath.ListSeparator)+"editable-live-src") || strings.HasPrefix(env["PYTHONPATH"], root) {
		t.Fatalf("actual virtual-copy environment: %v", env)
	}
	missing := m.workers(features.Run{Dir: root, Without: []string{"python3"}})
	absent, failure := missing.Open(t.Context(), mutation.MutationRun, 1)
	if failure != nil {
		t.Fatal(failure)
	}
	defer func() {
		if failure := absent.Close(t.Context()); failure != nil {
			t.Error(failure)
		}
	}()
	if _, failure := absent.Worker(0).Run(t.Context(), command, nil, mutation.Limits{}); failure == nil || !strings.Contains(failure.Error(), "Run.Without") {
		t.Fatalf("missing executable silently ran declared outcome: %v", failure)
	}
}
