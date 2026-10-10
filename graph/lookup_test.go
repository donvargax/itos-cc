package graph

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/mutate"
)

// The scenarios of graph-snapshot-lookup: each builds a Go module in a git
// repository of its own, whose lang/kotlin.go holds an init function, so
// the project root is the module's and no other .metrics is read.

// kotlinModule is the module, its git repository made, with init's entry as
// a run that killed every mutant records it.
func kotlinModule(t *testing.T) (module, []mutate.UnitResult) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	m := newModule(t)
	m.write("lang/kotlin.go", "package lang\n\nvar n int\n\nfunc init() {\n\tn = 1 + 1\n}\n")
	// A fixture repository never runs git's automatic maintenance or gc.
	for _, args := range [][]string{{"init", "-q"}, {"config", "maintenance.auto", "false"}, {"config", "gc.auto", "0"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = m.root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return m, m.recorded("lang/kotlin.go")
}

// initUnit is the init function of lang/kotlin.go in a graph of m built
// afresh.
func initUnit(t *testing.T, m module) Unit {
	t.Helper()
	for _, n := range m.build().Nodes {
		for _, u := range n.Units {
			if u.Name == "init" && u.File == "lang/kotlin.go" {
				return u
			}
		}
	}
	t.Fatal("no init in lang/kotlin.go")
	return Unit{}
}

// @ID-GRAPH-39
func TestTheGraphFindsAFilesMutationSnapshotWhereCheckDoes(t *testing.T) {
	m, units := kotlinModule(t)
	m.snapshot("lang/kotlin.go", nil, units)
	if u := initUnit(t, m); !u.Mutated || u.Killed != len(units[0].Mutants) {
		t.Errorf("init: %+v, want the %d kills its snapshot records", u, len(units[0].Mutants))
	}
}

// @ID-GRAPH-40
func TestAMutationSnapshotNamingAnotherPathDoesNotMatch(t *testing.T) {
	m, units := kotlinModule(t)
	m.snapshot("repo/lang/kotlin.go", nil, units)
	if u := initUnit(t, m); u.Mutated {
		t.Errorf("init: %+v, want no mutation results from a snapshot of repo/lang/kotlin.go", u)
	}

	t.Chdir(m.root)
	checks, err := mutate.Check([]string{filepath.Join(m.root, "lang", "kotlin.go")}, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 1 || len(checks[0].Functions) != 1 || checks[0].Functions[0].State != mutate.Missing {
		t.Errorf("mutation check: %+v, want init missing", checks)
	}
}
