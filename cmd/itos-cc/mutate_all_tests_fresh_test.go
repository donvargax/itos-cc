package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/metricsdir"
	mutation "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// The Go module whose binary, greet, is tested only by e2e/e2e_test.go, in
// another package: the test builds the binary and runs it with one
// argument, so it reaches "one" and never "many", which it would not notice
// anyway, and checks stdout only, so what main writes to stderr is never
// checked. mutation.feature's integration-coverage scenarios (ID-MUT-117 to
// 120) hold its coverage; the tests here use it for whole-suite freshness.

const greetSource = "main.go"

var greetFiles = map[string]string{
	"go.mod": "module example.com/greet\n\ngo 1.22\n",
	greetSource: `package main

import (
	"fmt"
	"os"
)

func main() {
	n := len(os.Args) - 1
	fmt.Println(greet(n))
	fmt.Fprintln(os.Stderr, n*2)
}

// greet tells one from many.
func greet(n int) string {
	if n > 1 {
		return fmt.Sprint(n-1, " more")
	}
	return "one"
}

// half is never run.
func half(i int) bool {
	return i > 2
}
`,
	"e2e/e2e_test.go": `package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOne(t *testing.T) {
	// Where the binary is to write its coverage, recorded for ID-MUT-120.
	if record, dir := os.Getenv("GREET_RECORD"), os.Getenv("GOCOVERDIR"); record != "" && dir != "" {
		f, err := os.OpenFile(record, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString(dir + "\n")
		f.Close()
	}
	bin := filepath.Join(t.TempDir(), "greet.exe")
	build := []string{"build", "-o", bin, ".."}
	if os.Getenv("GOCOVERDIR") != "" { // cover
		build = []string{"build", "-cover", "-o", bin, ".."}
	}
	if out, err := exec.Command("go", build...).CombinedOutput(); err != nil {
		t.Fatalf("go %v: %v\n%s", build, err, out)
	}
	out, err := exec.Command(bin, "x").Output()
	if err != nil {
		t.Fatalf("greet x: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != "one" {
		t.Fatalf("greet x printed %q, want \"one\"", got)
	}
}
`,
}

// greetIfLine is the line of main.go holding greet's if, n > 1: > → >=,
// 1 → 0, which the binary runs.
const greetIfLine = 16

// greetRepo makes the module in a git repository and makes it the test's
// directory (useDir). Without cover, its test builds the binary without -cover
// whatever GOCOVERDIR says; inProcess adds a test in package main that calls
// greet(3), so greet's other branch is covered in process.
func greetRepo(t *testing.T, cover, inProcess bool) string {
	t.Helper()
	files := map[string]string{}
	for name, text := range greetFiles {
		files[name] = text
	}
	if !cover {
		test := files["e2e/e2e_test.go"]
		start := strings.Index(test, "\tif os.Getenv(\"GOCOVERDIR\") != \"\" { // cover\n")
		end := strings.Index(test[start:], "\t}\n") + start + len("\t}\n")
		files["e2e/e2e_test.go"] = test[:start] + test[end:]
	}
	if inProcess {
		files["main_test.go"] = "package main\n\nimport \"testing\"\n\nfunc TestMany(t *testing.T) {\n" +
			"\tif got := greet(3); got != \"2 more\" {\n\t\tt.Fatalf(\"greet(3) = %q\", got)\n\t}\n}\n"
	}
	return moduleRepo(t, files)
}

// moduleRepo writes files in a git repository and makes it the test's
// directory (useDir).
func moduleRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	for _, tool := range []string{"git", "go"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " is not installed")
		}
	}
	dir := t.TempDir()
	for name, text := range files {
		writeFile(t, filepath.Join(dir, name), text)
	}
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-qm", "base")
	useDir(t, dir)
	return dir
}

func mutantLine(m map[string]any) int {
	line, _ := m["line"].(float64)
	return int(line)
}

func describe(m map[string]any) string {
	return fmt.Sprintf("%s:%d %v → %v %v", greetSource, mutantLine(m), m["original"], m["replacement"], m["outcome"])
}

func runMutantsAt(t *testing.T, result outcome, line int) (map[string]any, bool) {
	t.Helper()
	for _, file := range result.rawFiles(t) {
		if file["file"] != greetSource {
			continue
		}
		list, _ := file["mutants"].([]any)
		for _, raw := range list {
			mutant := raw.(map[string]any)
			if mutantLine(mutant) == line && mutant["replacement"] == ">=" {
				return mutant, true
			}
		}
	}
	return nil, false
}

// @ID-MUT-152. Kept: a reused kill's scope in mutation run --json, which no
// scenario asserts; the scenario reads the snapshot's. ID-MUT-150, 151 and
// 153 to 155 hold their old tests whole (T-101).
func TestPlainRunReusesBroadScopeKillsAndKeepsTheirEvidence(t *testing.T) {
	t.Parallel()
	for _, scope := range []struct {
		name          string
		args          []string
		expectedScope string
	}{
		{name: "all-tests", args: []string{"--all-tests"}},
		{name: "test-command", args: []string{"--test-command", "go test -count=1 ./..."}, expectedScope: "go test -count=1 ./..."},
	} {
		t.Run(scope.name, func(t *testing.T) {
			greetRepo(t, false, false)
			first := append(append([]string{}, scope.args...), "--no-coverage", "--json", greetSource)
			result := mutateCovered(t, first...)
			if result.code > 1 {
				t.Fatalf("initial run: exit %d\n%s%s", result.code, result.stdout, result.stderr)
			}
			initial, initialFound := runMutantsAt(t, result, greetIfLine)
			if !initialFound || initial["outcome"] != "killed" {
				t.Fatalf("initial run mutant: found=%v outcome=%v scope=%v", initialFound, initial["outcome"], initial["scope"])
			}
			beforeEvidence := broadEvidenceAt(t, ".metrics/mutate/main.go.json", greetIfLine)
			plain := mutateCovered(t, "--no-coverage", "--json", greetSource)
			mutant, found := runMutantsAt(t, plain, greetIfLine)
			expected := scope.expectedScope
			if expected == "" {
				expected = scope.name
			}
			if !found || mutant["outcome"] != "killed" || mutant["reused"] != true || mutant["scope"] != expected {
				t.Fatalf("plain run mutant: found=%v outcome=%v reused=%v scope=%v, want reused kill with scope %s", found, mutant["outcome"], mutant["reused"], mutant["scope"], expected)
			}
			afterEvidence := broadEvidenceAt(t, ".metrics/mutate/main.go.json", greetIfLine)
			if !reflect.DeepEqual(beforeEvidence, afterEvidence) {
				t.Fatalf("plain run changed reused broad evidence: before=%+v after=%+v", beforeEvidence, afterEvidence)
			}
			if states, _, check := checked(t); states[greetID] != "fresh" {
				t.Fatalf("mutation check after plain reuse: %q, want fresh\n%s", states[greetID], check.stdout)
			}
		})
	}
}

func broadEvidenceAt(t *testing.T, path string, line int) *mutation.SuiteEvidence {
	t.Helper()
	data, err := os.ReadFile(inWD(t, path))
	if err != nil {
		t.Fatal(err)
	}
	var snapshot mutation.Snapshot
	if err := metricsdir.ReadSnapshotJSON(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	for _, unit := range snapshot.Units {
		for _, mutant := range unit.Mutants {
			if mutant.Line == line && mutant.Replacement == ">=" {
				if mutant.SuiteEvidence == nil {
					t.Fatalf("broad mutant at line %d has no Go freshness evidence", line)
				}
				return mutant.SuiteEvidence
			}
		}
	}
	t.Fatalf("snapshot has no broad mutant at line %d", line)
	return nil
}
