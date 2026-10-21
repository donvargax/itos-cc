package domain_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
	aggregate "github.com/donvargax/itos-cc/internal/coverage/port/porttest"
	covtest "github.com/donvargax/itos-cc/internal/coverage/porttest"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

func freshPython(t *testing.T) (*porttest.Fresh, *mutants.FreshPlan) {
	t.Helper()
	b := memoryFreshFixture(t)
	for name, text := range map[string]string{"py/pyproject.toml": "[project]\nname='fresh'\n", "py/value.py": "def value(x):\n    return x + 1\n", "py/test_value.py": "def test_value():\n    assert True\n"} {
		if err := b.Put(t.Context(), name, text); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-qm", "independent Python inputs"}} {
		if err := b.Git(t.Context(), args...); err != nil {
			t.Fatal(err)
		}
	}
	f := b.Fresh.(*porttest.Fresh)
	p, failed := f.Plan(t.Context(), mutants.FreshRequest{Root: b.Root, Count: 1})
	if failed != nil {
		t.Fatal(failed)
	}
	t.Cleanup(func() { f.Release(t.Context(), p) })
	build := filepath.Join(p.FrozenRoot, "py")
	f.Inputs.Probes = map[porttest.PytestProbe]bool{
		porttest.ProbeKey("python3", p.FrozenRoot, nil): false,
		porttest.ProbeKey("python3", build, nil):        true,
	}
	f.Tools = []mutants.PreparationTool{{Name: "python3", Path: "independent-installed-python"}}
	f.Coverage = &covtest.Measurer{Aggregate: aggregate.Measurer{Dir: p.FrozenRoot, Files: f.Inputs.Files}, Declarations: map[coverage.Unloaded]covtest.Measurement{{Language: "python", Dir: build}: {Language: "python", Report: "SF:value.py\nDA:2,1\nend_of_record\n"}}}
	f.Commands = &porttest.Workers{Files: f.Inputs.Files, Tests: map[string]map[string]porttest.Tests{build: {"python3 -m pytest -q -x -p no:cacheprovider": {Elapsed: time.Millisecond}}}}
	return f, p
}

func TestMemoryFreshPreparationUsesRootProbeAndFrozenBaselineFacts(t *testing.T) {
	f, p := freshPython(t)
	build := filepath.Join(p.FrozenRoot, "py")
	prep, failed := f.Prepare(t.Context(), p, covport.AllTests)
	if failed != nil {
		t.Fatalf("prepare: %v %+v", failed, prep)
	}
	if len(prep.Baselines) != 1 || !slices.Equal(prep.Baselines[0].Command.Args, []string{"python3", "-m", "pytest", "-q", "-x", "-p", "no:cacheprovider"}) || prep.Baselines[0].Command.Dir != build {
		t.Fatalf("nested-root baseline: %+v", prep.Baselines)
	}
	if len(prep.Commands) != 2 || prep.Commands[0].Dir != build || prep.Commands[0].Err != nil || !slices.Equal(prep.Commands[0].Args, []string{"python3", "-c", "import pytest"}) {
		t.Fatalf("actual root probe: %+v", prep.Commands)
	}
	// Only the root-specific import observation changes. The repo-cwd fact and
	// command declarations cannot stand in for a successful root probe.
	f.Inputs.Probes[porttest.ProbeKey("python3", build, nil)] = false
	prep, failed = f.Prepare(t.Context(), p, covport.AllTests)
	if failed == nil || len(prep.Commands) != 1 || prep.Commands[0].Err == nil || prep.Commands[0].Dir != build {
		t.Fatalf("wrong root fact was ignored: %+v %v", prep, failed)
	}
	if !strings.Contains(failed.Error(), "unittest") {
		t.Fatalf("failed pytest import did not choose actual fallback: %v", failed)
	}
}

func TestMemoryFreshPreparationRetainsFailedCoverageWithoutBaseline(t *testing.T) {
	f, p := freshPython(t)
	group := coverage.Unloaded{Language: "python", Dir: filepath.Join(p.FrozenRoot, "py")}
	d := f.Coverage.Declarations[group]
	d.CommandExit = 7
	d.CommandOutput = "collector failed after writing"
	f.Coverage.Declarations[group] = d
	prep, failed := f.Prepare(t.Context(), p, covport.AllTests)
	if failed == nil {
		t.Fatal("failed collector passed fresh preparation")
	}
	last := prep.Stages[len(prep.Stages)-1]
	if last.Name != "coverage" || last.State != "failed" || prep.Report == nil || len(prep.Baselines) != 0 {
		t.Fatalf("partial stages lost or baseline admitted: %+v", prep)
	}
	if hit, ok := prep.Report.LineCovered(filepath.Join(p.FrozenRoot, "py", "value.py"), 2); !ok || !hit || prep.Report.Measures("python") {
		t.Fatal("failed artifact disappeared or became independent proof")
	}
	if len(prep.Commands) != 1 {
		t.Fatalf("baseline entered after failed coverage: %+v", prep.Commands)
	}
}
