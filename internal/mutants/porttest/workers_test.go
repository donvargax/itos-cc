package porttest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/donvargax/itos-cc/internal/mutants"
)

func TestWorkersContract(t *testing.T) {
	WorkersContract(t, func(t *testing.T, baseline string) (mutants.Workers, mutants.Command) {
		root := t.TempDir()
		c := mutants.Command{Root: root, Dir: root, Args: []string{"fixture", "test"}}
		f := &Workers{Files: map[string]string{filepath.Join(root, "source.txt"): "original\n"}, Tests: map[string]map[string]Tests{root: {c.String(): {Baseline: baseline}}}}
		return f, c
	})
}

func TestDeclaredCommandsUseActualMutatedCopyAndSelectors(t *testing.T) {
	root := t.TempDir()
	trace := filepath.Join(root, "external-trace.jsonl")
	source := filepath.Join(root, "a.py")
	own := mutants.Command{Root: root, Dir: root, Args: []string{"python3", "-m", "pytest", "-q", "test_a.py"}}
	all := mutants.Command{Root: root, Dir: root, Args: []string{"python3", "-m", "pytest", "-q"}, Suite: true}
	src := []byte("return x > 0\n")
	site := mutants.Site{Start: 9, End: 10, Line: 1, Original: ">", Replacement: ">="}
	inventory := map[string]string{"test_a.py": "ATest", "test_unrelated.py": "Unrelated"}
	declaration := Tests{Elapsed: 23 * time.Millisecond, Trace: &Trace{File: trace, Source: "a.py", Tests: inventory}, Verdicts: []Verdict{{Path: "a.py", Site: site, Outcome: mutants.Survived}}}
	f := &Workers{Files: map[string]string{source: string(src)}, Tests: map[string]map[string]Tests{root: {own.String(): declaration, all.String(): declaration}}}
	p, err := f.Open(t.Context(), mutants.MutationRun, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close(t.Context())
	w := p.Worker(0)
	change := &mutants.Mutation{Path: source, Source: src, Site: site}
	result, err := w.Run(t.Context(), own, change, 0)
	if err != nil || !result.Passed || result.Elapsed != 23*time.Millisecond {
		t.Fatalf("actual command: %+v %v", result, err)
	}
	var first map[string]string
	lines := strings.Split(strings.TrimSpace(f.Files[trace]), "\n")
	if len(lines) != 1 {
		t.Fatalf("narrow command executed %d tests", len(lines))
	}
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(site.Apply(src))
	if first["test"] != "ATest" || first["source_sha256"] != hex.EncodeToString(sum[:]) || first["original_root"] != root || first["cwd"] == root {
		t.Fatalf("trace did not observe actual copy/selector: %v", first)
	}
	if _, err = w.Run(t.Context(), all, nil, 0); err != nil {
		t.Fatal(err)
	}
	lines = strings.Split(strings.TrimSpace(f.Files[trace]), "\n")
	if len(lines) != 3 {
		t.Fatalf("whole suite lost unrelated test: %s", f.Files[trace])
	}
	var names []string
	for _, line := range lines[1:] {
		var r map[string]string
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatal(err)
		}
		names = append(names, r["test"])
		sum := sha256.Sum256(src)
		if r["source_sha256"] != hex.EncodeToString(sum[:]) {
			t.Fatalf("mutant was not restored: %v", r)
		}
	}
	if !reflect.DeepEqual(names, []string{"ATest", "Unrelated"}) {
		t.Fatalf("whole suite inventory: %v", names)
	}
	// Counterfactual: another requested command cannot borrow own's declaration.
	wrong := own
	wrong.Args = []string{"python3", "-m", "pytest", "missing.py"}
	if _, err = w.Run(t.Context(), wrong, nil, 0); err == nil {
		t.Fatal("missing binding passed")
	}
	delete(f.Tests[root], own.String())
	if _, err = w.Run(t.Context(), own, nil, 0); err == nil {
		t.Fatal("omitted command binding passed")
	}
	if f.Files[source] != string(src) {
		t.Fatal("live source changed")
	}
}

func TestTraceRejectsUnknownSelectorsAndPreservesNilInventory(t *testing.T) {
	inventory := map[string]string{"own.ATest": "ATest", "own.BTest": "BTest"}
	for _, args := range [][]string{{"gradle", "test", "--tests", "wrong.Test"}, {"unrecognized", "runner"}} {
		if _, err := selectedTests(mutants.Command{Args: args}, inventory); err == nil {
			t.Fatalf("unsupported selector accepted: %v", args)
		}
	}
	names, err := selectedTests(mutants.Command{Args: []string{"arbitrary", "command"}}, nil)
	if err != nil || !reflect.DeepEqual(names, []string{""}) {
		t.Fatalf("nil inventory changed: %v %v", names, err)
	}
}
