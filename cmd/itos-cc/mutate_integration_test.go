package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/metrics"
)

// The scenarios of "Rule: Coverage from tests that run the built binary" in
// features/mutate.feature. Each runs mutate in a Go module whose binary,
// greet, is tested only by e2e/e2e_test.go, in another package: the test
// builds the binary and runs it with one argument, so it reaches "one" and
// never "many", which it would not notice anyway, and checks stdout only, so
// what main writes to stderr is never checked. --all-tests runs that test for
// coverage and for every mutant, each mutant's from the worker's copy, which
// the test builds the binary from.

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

// The lines of main.go that hold mutants: those of main and greet's if the
// binary runs, those of the branch it never takes and of half it does not.
const (
	greetArgsLine   = 9  // n := len(os.Args) - 1: - → +, 1 → 0
	greetStderrLine = 11 // n*2, never checked: * → /
	greetIfLine     = 16 // n > 1: > → >=, 1 → 0
	greetManyLine   = 17 // n-1, a branch the binary never takes: - → +, 1 → 0
	greetHalfLine   = 24 // i > 2, never run: > → >=
)

// greetMutants is how many mutants main.go holds.
const greetMutants = 8

// greetRepo makes the module in a git repository and makes it the working
// directory. Without cover, its test builds the binary without -cover
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

// moduleRepo writes files in a git repository and makes it the working
// directory.
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
	t.Chdir(dir)
	return dir
}

// greetJSON runs mutation run --all-tests --json with args and coverage
// measured, and returns main.go's "mutants" as plain maps, with what was
// printed.
func greetJSON(t *testing.T, args ...string) ([]map[string]any, outcome) {
	t.Helper()
	o := mutateCovered(t, append([]string{"--all-tests", "--json"}, args...)...)
	for _, f := range o.rawFiles(t) {
		if f["file"] != greetSource {
			continue
		}
		var out []map[string]any
		list, _ := f["mutants"].([]any)
		for _, m := range list {
			out = append(out, m.(map[string]any))
		}
		if len(out) != greetMutants {
			t.Fatalf("%d mutants of %s, want %d\n%s\nstderr:\n%s", len(out), greetSource, greetMutants, o.stdout, o.stderr)
		}
		return out, o
	}
	t.Fatalf("no %s in files:\n%s\nstderr:\n%s", greetSource, o.stdout, o.stderr)
	return nil, o
}

func mutantLine(m map[string]any) int {
	line, _ := m["line"].(float64)
	return int(line)
}

func describe(m map[string]any) string {
	return fmt.Sprintf("%s:%d %v → %v %v", greetSource, mutantLine(m), m["original"], m["replacement"], m["outcome"])
}

// @ID-MUT-117
func TestLinesATestReachesThroughTheBuiltBinaryAreCovered(t *testing.T) {
	greetRepo(t, true, false)

	mutants, o := greetJSON(t, "--fail-uncovered")
	ran := []int{greetArgsLine, greetStderrLine, greetIfLine}
	for _, m := range mutants {
		if slices.Contains(ran, mutantLine(m)) && (m["outcome"] == "uncovered" || m["reused"] == true) {
			t.Fatalf("%s: want it covered and run, on a line the binary ran\nstderr:\n%s", describe(m), o.stderr)
		}
	}
	outcomes := map[string]string{}
	for _, m := range mutants {
		outcomes[fmt.Sprintf("%d %v", mutantLine(m), m["replacement"])] = m["outcome"].(string)
	}
	if got := outcomes[fmt.Sprintf("%d >=", greetIfLine)]; got != "killed" {
		t.Errorf("%s:%d > → >=: %s, want killed: the binary prints \"0 more\"", greetSource, greetIfLine, got)
	}
	if got := outcomes[fmt.Sprintf("%d /", greetStderrLine)]; got != "survived" {
		t.Errorf("%s:%d * → /: %s, want survived: the test never reads stderr", greetSource, greetStderrLine, got)
	}
	for _, m := range mutants {
		never := mutantLine(m) == greetManyLine || mutantLine(m) == greetHalfLine
		if never != (m["outcome"] == "uncovered") {
			t.Errorf("%s: want uncovered exactly on the lines the binary never ran, %d and %d",
				describe(m), greetManyLine, greetHalfLine)
		}
	}
	if t.Failed() {
		t.Logf("stdout:\n%s\nstderr:\n%s", o.stdout, o.stderr)
	}
	if o.code != 1 {
		t.Errorf("exit %d, want 1 for the uncovered mutants and the survivor", o.code)
	}
}

// @ID-MUT-118
func TestAProjectWhoseBinaryWritesNoCoverageBehavesAsBefore(t *testing.T) {
	greetRepo(t, false, false)

	mutants, o := greetJSON(t, "--fail-uncovered")
	for _, m := range mutants {
		if m["outcome"] != "uncovered" {
			t.Errorf("%s, want uncovered: no test executes it in process, and the binary writes no coverage", describe(m))
		}
		if _, ok := m["coverage"]; ok {
			t.Errorf("%s has \"coverage\" %v, want none: it is uncovered", describe(m), m["coverage"])
		}
	}
	if t.Failed() {
		t.Logf("stdout:\n%s\nstderr:\n%s", o.stdout, o.stderr)
	}
	if o.code != 1 {
		t.Errorf("exit %d, want 1 for the uncovered mutants", o.code)
	}
}

// @ID-MUT-119
func TestWhichCoverageReachedEachMutantAsJSON(t *testing.T) {
	greetRepo(t, true, true)

	mutants, o := greetJSON(t)
	want := map[int][]any{
		greetArgsLine:   {"integration"},
		greetStderrLine: {"integration"},
		greetIfLine:     {"in-process", "integration"},
		greetManyLine:   {"in-process"},
	}
	for _, m := range mutants {
		got, has := m["coverage"]
		if m["outcome"] == "uncovered" {
			if has {
				t.Errorf("%s has \"coverage\" %v, want none", describe(m), got)
			}
			continue
		}
		list, _ := got.([]any)
		if !has || !slices.Equal(list, want[mutantLine(m)]) {
			t.Errorf("%s: \"coverage\" %v, want %q", describe(m), got, want[mutantLine(m)])
		}
	}
	for _, m := range mutants {
		if (mutantLine(m) == greetHalfLine) != (m["outcome"] == "uncovered") {
			t.Errorf("%s: want only half's mutant, on line %d, uncovered", describe(m), greetHalfLine)
		}
	}
	if t.Failed() {
		t.Logf("stdout:\n%s\nstderr:\n%s", o.stdout, o.stderr)
	}
}

// @ID-MUT-120
func TestTheBinarysCoverageDataStaysWithTheRun(t *testing.T) {
	greetRepo(t, true, false)
	record := filepath.Join(t.TempDir(), "gocoverdir")
	t.Setenv("GREET_RECORD", record)
	out := filepath.Join(metrics.Dir(), "coverage")

	o := mutateCovered(t, "--all-tests", "--fail-uncovered")
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("the test recorded no GOCOVERDIR: %v\nstderr:\n%s", err, o.stderr)
	}
	// The coverage run comes first; mutants run without coverage.
	dir, _, _ := strings.Cut(string(data), "\n")
	rel, err := filepath.Rel(out, dir)
	first, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
	if err != nil || !strings.HasPrefix(first, "run-") || first == rel {
		t.Fatalf("GOCOVERDIR %s, want a directory under a run-* directory of %s", dir, out)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("GOCOVERDIR %s is still there after the run (%v)", dir, err)
	}
	if _, err := os.Stat(filepath.Join(out, first)); !os.IsNotExist(err) {
		t.Errorf("the run's directory %s is still there after the run (%v)", first, err)
	}
	filepath.WalkDir(out, func(path string, d fs.DirEntry, err error) error {
		if err == nil && (strings.HasPrefix(d.Name(), "covmeta.") || strings.HasPrefix(d.Name(), "covcounters.")) {
			t.Errorf("%s is left under %s", path, out)
		}
		return nil
	})
}
