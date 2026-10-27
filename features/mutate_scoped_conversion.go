package features

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

// The listed tests' Go coverage is converted only for the packages the run
// measures (slice-65): a module whose binary links packages a and b, the
// change and its mutant in a, and a listed test that reaches only b.
// Binary-only for good: the per-test data and its conversion are the real
// collector's and covdata's.

// scopedConversion is the state of those steps.
type scopedConversion struct {
	m *mutateState
	// counted is whether the scenario's run is a counted one, which
	// itos-cc makes on Linux and macOS alone.
	counted bool
	// sited is whether the module's base commit holds a/a.go as the change
	// leaves it, its site there from the start: slice-69's strict run
	// reuses what an earlier run of a/a.go measured.
	sited bool
}

func scopedConversionSteps(sc *godog.ScenarioContext, m *mutateState) {
	s := &scopedConversion{m: m}
	sc.Before(func(ctx context.Context, scenario *godog.Scenario) (context.Context, error) {
		*s = scopedConversion{m: m}
		for _, step := range scenario.Steps {
			if strings.Contains(step.Text, "--count") {
				s.counted = true
			}
			if strings.Contains(step.Text, "--fail-uncovered a/a.go --json") {
				s.sited = true
			}
		}
		return ctx, nil
	})
	sc.Step(`^a Go module whose binary links packages a and b, with listed tests ID-A-01, which reaches a and b, and ID-A-02, which reaches only b$`, s.module)
	sc.Step(`^a committed change to a/a\.go whose mutant survives its own tests and is killed by ID-A-01$`, s.change)
	sc.Step(`^the mutant is killed with ID-A-01 alone$`, s.killedByOne)
	sc.Step(`^no line of a/a\.go that ID-A-01 reaches is uncovered$`, s.noneUncovered)
	sc.Step(`^ID-A-02, though it reaches nothing of a, is measured: the run judges, with no "count\.preparation-failed"$`, s.measured)
}

// scopedFiles are the module at its base commit: main.go, the binary,
// prints b.Name() and, given a number, a.Big of it. ID-A-01 runs it with 5,
// reaching a and b; ID-A-02 runs it bare, reaching only b. a.Big holds no
// site until the change.
var scopedFiles = map[string]string{
	"go.mod":      "module example.com/scoped\n\ngo 1.22\n",
	"a/a.go":      "package a\n\n// Big is whether n is past five.\nfunc Big(n int) bool {\n\treturn true\n}\n",
	"a/a_test.go": "package a\n\nimport \"testing\"\n\nfunc TestBig(t *testing.T) {\n\tif !Big(20) {\n\t\tt.Fatal(\"Big\")\n\t}\n}\n",
	"b/b.go":      "package b\n\n// Name is what the binary calls itself.\nfunc Name() string {\n\treturn \"scoped\"\n}\n",
	"main.go": `package main

import (
	"fmt"
	"os"
	"strconv"

	"example.com/scoped/a"
	"example.com/scoped/b"
)

func main() {
	fmt.Println(b.Name())
	if len(os.Args) > 1 {
		n, _ := strconv.Atoi(os.Args[1])
		fmt.Println(a.Big(n))
	}
}
`,
	"e2e/e2e_test.go": `package e2e

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var selected = flag.String("tests", "", "the IDs of the tests to run, by commas: all of them when empty")

var features = []struct {
	id   string
	args []string
	want string
}{
	{"ID-A-01", []string{"5"}, "scoped\nfalse"},
	{"ID-A-02", nil, "scoped"},
}

func TestFeatures(t *testing.T) {
	split := os.Getenv("ITOS_CC_TEST_COVERDIR")
	bin := filepath.Join(t.TempDir(), "scoped.exe")
	build := []string{"build", "-buildvcs=false", "-o", bin, ".."}
	if split != "" || os.Getenv("GOCOVERDIR") != "" {
		build = []string{"build", "-buildvcs=false", "-cover", "-o", bin, ".."}
	}
	if out, err := exec.Command("go", build...).CombinedOutput(); err != nil {
		t.Fatalf("go %v: %v\n%s", build, err, out)
	}
	for _, f := range features {
		if *selected != "" && !slices.Contains(strings.Split(*selected, ","), f.id) {
			continue
		}
		cmd := exec.Command(bin, f.args...)
		if split != "" {
			dir := filepath.Join(split, f.id)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			cmd.Env = append(os.Environ(), "GOCOVERDIR="+dir)
		}
		out, err := cmd.Output()
		if err != nil {
			t.Errorf("%s: %v", f.id, err)
			continue
		}
		if got := strings.TrimSpace(string(out)); got != f.want {
			t.Errorf("%s: printed %q, want %q", f.id, got, f.want)
		}
	}
}
`,
	"testdata/list.go": "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Print(\"ID-A-01\\nID-A-02\\n\")\n}\n",
	"itos-cc.yaml":     countedListedConfig,
}

// scopedChanged is a/a.go after the change: Big's one site, > → >=, which
// its own test, Big(20), lets survive and ID-A-01, Big(5), kills.
const scopedChanged = "package a\n\n// Big is whether n is past five.\nfunc Big(n int) bool {\n\treturn n > 5\n}\n"

// module writes the module and commits it, tagged base, the <base> a run's
// words name.
func (s *scopedConversion) module() error {
	w := s.m.w
	if s.counted {
		if err := w.d.Counted(); err != nil {
			return err
		}
	}
	if err := s.m.writeAll(scopedFiles); err != nil {
		return err
	}
	if s.sited {
		if err := w.write("a/a.go", scopedChanged); err != nil {
			return err
		}
	}
	w.prepare = func(args []string) []string {
		for i, arg := range args {
			if arg == "<base>" {
				args[i] = "base"
			}
		}
		return args
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"commit", "-qm", "base"}, {"tag", "base"}} {
		if err := w.d.Git(w.dir, args...); err != nil {
			return err
		}
	}
	return nil
}

func (s *scopedConversion) change() error {
	w := s.m.w
	if err := w.write("a/a.go", scopedChanged); err != nil {
		return err
	}
	if err := w.d.Git(w.dir, "add", "a/a.go"); err != nil {
		return err
	}
	return w.d.Git(w.dir, "commit", "-qm", "change Big")
}

// mutants are the mutants the run's --json object gives: a counted run's
// "selected", a complete run's files' "mutants".
func (s *scopedConversion) mutants() ([]map[string]any, error) {
	var object struct {
		Selected []map[string]any `json:"selected"`
		Files    []struct {
			Mutants []map[string]any `json:"mutants"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(s.m.w.stdout), &object); err != nil {
		return nil, fmt.Errorf("stdout is not one JSON object: %w\n%s", err, s.m.w.report())
	}
	out := object.Selected
	for _, f := range object.Files {
		out = append(out, f.Mutants...)
	}
	return out, nil
}

func (s *scopedConversion) killedByOne() error {
	mutants, err := s.mutants()
	if err != nil {
		return err
	}
	if len(mutants) != 1 || mutants[0]["outcome"] != "killed" || !slices.Equal(entryIDs(mutants[0]), []string{"ID-A-01"}) {
		return fmt.Errorf("the run's mutants are %v, not one killed with ID-A-01 alone\n%s", mutants, s.m.w.report())
	}
	return nil
}

func (s *scopedConversion) noneUncovered() error {
	mutants, err := s.mutants()
	if err != nil {
		return err
	}
	for _, m := range mutants {
		if m["outcome"] == "uncovered" {
			return fmt.Errorf("a mutant of a/a.go is uncovered: %v\n%s", m, s.m.w.report())
		}
	}
	for _, code := range []string{"mutation.uncovered-statement", "mutation.coverage-missing", "mutation.coverage-stale"} {
		if strings.Contains(s.m.w.stdout+s.m.w.stderr, code) {
			return fmt.Errorf("the run reports %q\n%s", code, s.m.w.report())
		}
	}
	return nil
}

func (s *scopedConversion) measured() error {
	w := s.m.w
	if strings.Contains(w.stdout+w.stderr, "count.preparation-failed") {
		return fmt.Errorf("the run reports \"count.preparation-failed\"\n%s", w.report())
	}
	var object struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal([]byte(w.stdout), &object); err != nil || !object.OK {
		return fmt.Errorf("the run did not judge, ok: %v (%v)\n%s", object.OK, err, w.report())
	}
	return w.exitsWith(0)
}
