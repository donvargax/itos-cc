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

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
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
	defer closePool(t.Context(), t, p)
	w := p.Worker(0)
	change := &mutants.Mutation{Path: source, Source: src, Site: site}
	result, err := w.Run(t.Context(), own, change, mutants.Limits{})
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
	if _, err = w.Run(t.Context(), all, nil, mutants.Limits{}); err != nil {
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
	if _, err = w.Run(t.Context(), wrong, nil, mutants.Limits{}); err == nil {
		t.Fatal("missing binding passed")
	}
	delete(f.Tests[root], own.String())
	if _, err = w.Run(t.Context(), own, nil, mutants.Limits{}); err == nil {
		t.Fatal("omitted command binding passed")
	}
	if f.Files[source] != string(src) {
		t.Fatal("live source changed")
	}
}

func TestTraceRejectsUnknownSelectorsAndPreservesNilInventory(t *testing.T) {
	inventory := map[string]string{"own.ATest": "ATest", "own.BTest": "BTest"}
	for _, args := range [][]string{{"gradle", "test", "--tests", "wrong.Test"}, {"unrecognized", "runner"}, {"unrecognized", "runner", "own.ATest"}} {
		if _, err := SelectedTests(mutants.Command{Args: args}, inventory); err == nil {
			t.Fatalf("unsupported selector accepted: %v", args)
		}
	}
	names, err := SelectedTests(mutants.Command{Args: []string{"arbitrary", "command"}}, nil)
	if err != nil || !reflect.DeepEqual(names, []string{""}) {
		t.Fatalf("nil inventory changed: %v %v", names, err)
	}
}

func TestBoundTraceValidatesEverySelectorAndGradleWrapperSuite(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.kt")
	trace := filepath.Join(root, "trace.jsonl")
	python := mutants.Command{Root: root, Dir: root, Args: []string{"python3", "-m", "pytest", "-q", "test_a.py", "missing.py"}}
	filtered := mutants.Command{Root: root, Dir: root, Args: []string{"python3", "-m", "pytest", "-q", "-k", "test_a"}}
	wrapper := mutants.Command{Root: root, Dir: root, Args: []string{"./gradlew", "test", "--fail-fast"}}
	declarations := map[string]Tests{}
	for _, c := range []mutants.Command{python, filtered} {
		declarations[c.String()] = Tests{Trace: &Trace{File: trace, Source: "source.kt", Tests: map[string]string{"test_a.py": "ATest", "test_unrelated.py": "Unrelated"}}}
	}
	declarations[wrapper.String()] = Tests{Trace: &Trace{File: trace, Source: "source.kt", Tests: map[string]string{"own.ATest": "ATest", "own.BTest": "Unrelated"}}}
	f := &Workers{Files: map[string]string{source: "actual source\n"}, Tests: map[string]map[string]Tests{root: declarations}}
	pool, err := f.Open(t.Context(), mutants.MutationRun, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer closePool(t.Context(), t, pool)
	worker := pool.Worker(0)
	for _, c := range []mutants.Command{python, filtered} {
		if _, err := worker.Run(t.Context(), c, nil, mutants.Limits{}); err == nil {
			t.Fatalf("unsupported actual request emitted a partial trace: %s", c)
		}
		if f.Files[trace] != "" {
			t.Fatalf("rejected selectors emitted invented executions: %s", f.Files[trace])
		}
	}
	if _, err := worker.Run(t.Context(), wrapper, nil, mutants.Limits{}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(f.Files[trace]), "\n")
	var names []string
	for _, line := range lines {
		var record map[string]string
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		names = append(names, record["test"])
		sum := sha256.Sum256([]byte(f.Files[source]))
		if record["source_sha256"] != hex.EncodeToString(sum[:]) {
			t.Fatalf("wrapper trace did not read actual input: %v", record)
		}
	}
	if !reflect.DeepEqual(names, []string{"ATest", "Unrelated"}) {
		t.Fatalf("whole wrapper lost unrelated inventory: %v", names)
	}
}

func TestBoundUnittestFailFastRequestsSelectActualInventory(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "a.py")
	trace := filepath.Join(root, "trace.jsonl")
	own := mutants.Command{Root: root, Dir: root, Args: []string{filepath.Join(root, ".venv", "bin", "python"), "-m", "unittest", "-f", "test_a"}}
	all := own
	all.Args = append([]string(nil), own.Args[:3]...)
	all.Args = append(all.Args, "discover", "-f")
	filtered := own
	filtered.Args = append([]string(nil), own.Args[:3]...)
	filtered.Args = append(filtered.Args, "-k", "ATest")
	declaration := Tests{Trace: &Trace{File: trace, Source: "a.py", Tests: map[string]string{"test_a": "ATest", "test_unrelated": "Unrelated"}}}
	f := &Workers{Files: map[string]string{source: "actual unittest source\n"}, Tests: map[string]map[string]Tests{root: {own.String(): declaration, all.String(): declaration, filtered.String(): declaration}}}
	p, err := f.Open(t.Context(), mutants.MutationRun, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer closePool(t.Context(), t, p)
	for _, c := range []mutants.Command{own, all} {
		if _, err := p.Worker(0).Run(t.Context(), c, nil, mutants.Limits{}); err != nil {
			t.Fatalf("actual unittest request %s: %v", c, err)
		}
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(f.Files[trace]), "\n") {
		var record map[string]string
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		names = append(names, record["test"])
		sum := sha256.Sum256([]byte(f.Files[source]))
		if record["source_sha256"] != hex.EncodeToString(sum[:]) || record["original_root"] != root || record["cwd"] == root {
			t.Fatalf("unittest trace not actual copied input: %v", record)
		}
	}
	if !reflect.DeepEqual(names, []string{"ATest", "ATest", "Unrelated"}) {
		t.Fatalf("unittest own/discovery selection: %v", names)
	}
	before := f.Files[trace]
	if _, err := p.Worker(0).Run(t.Context(), filtered, nil, mutants.Limits{}); err == nil || f.Files[trace] != before {
		t.Fatal("unsupported -k changed trace or was accepted")
	}
}
