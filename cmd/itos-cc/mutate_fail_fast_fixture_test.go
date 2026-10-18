//go:build !windows

package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The readers of a fail-fast run's --json object, test.sh's module and
// runs, and ffListedFiles, the listed-test module, that the fail-fast
// scenarios of mutate_fail_fast_test.go and
// mutate_fail_fast_partial_test.go share.

// ffJSON is the --json object of a mutation run, fail-fast keys included.
type ffJSON struct {
	Stop     map[string]any   `json:"stop"`
	Files    []map[string]any `json:"files"`
	Problems []map[string]any `json:"problems"`
}

func (o outcome) ff(t *testing.T) ffJSON {
	t.Helper()
	var out ffJSON
	if err := json.Unmarshal([]byte(o.stdout), &out); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s\nstderr:\n%s", err, o.stdout, o.stderr)
	}
	return out
}

func (f ffJSON) problem(rule string) map[string]any {
	for _, p := range f.Problems {
		if p["rule"] == rule {
			return p
		}
	}
	return nil
}

// file is the entry of file in "files", or nil.
func (f ffJSON) file(name string) map[string]any {
	for _, file := range f.Files {
		if file["file"] == name {
			return file
		}
	}
	return nil
}

// stopped says whether the run reports that it stopped early.
func (f ffJSON) stopped() bool {
	return f.Stop["stopped"] == true
}

// stopRule is the rule of the run's stop, or "".
func (f ffJSON) stopRule() string {
	r, _ := f.Stop["rule"].(string)
	return r
}

// stopSubject is the subject of the run's stop, or nil.
func (f ffJSON) stopSubject() map[string]any {
	s, _ := f.Stop["subject"].(map[string]any)
	return s
}

// mutantsOf is the "mutants" of a file entry, as plain maps.
func mutantsOf(file map[string]any) []map[string]any {
	var out []map[string]any
	list, _ := file["mutants"].([]any)
	for _, m := range list {
		out = append(out, m.(map[string]any))
	}
	return out
}

// ffMutant is the mutant of file in function, by the name after its "#".
func ffMutant(file map[string]any, function string) map[string]any {
	for _, m := range mutantsOf(file) {
		if f, _ := m["function"].(string); strings.HasSuffix(f, "#"+function) {
			return m
		}
	}
	return nil
}

// undecided says whether m is reported in state with no mutation outcome.
func undecided(m map[string]any, state string) bool {
	_, has := m["outcome"]
	return m != nil && m["state"] == state && !has
}

// ffScript is test.sh: each line of body after the line recording the run.
func ffScript(body ...string) string {
	return "echo run >> \"$FF_DIR/runs\"\n" + strings.Join(body, "\n") + "\n"
}

// ffRepo writes files in a Git repository, makes it the test's directory
// (useDir) and points FF_DIR at a fresh directory for test.sh's markers. It returns
// the repository and that directory.
func ffRepo(t *testing.T, files map[string]string) (string, string) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not installed")
	}
	dir := moduleRepo(t, files)
	ff := t.TempDir()
	useEnv(t, "FF_DIR", ff)
	return dir, ff
}

// shellRun runs mutation run --json with test.sh as the test command, no
// coverage, and args.
func shellRun(t *testing.T, args ...string) outcome {
	t.Helper()
	return itosCc(t, append([]string{"mutation", "run", "--json", "--no-coverage", "--test-command", "sh test.sh"}, args...)...)
}

// runsIn is how many times test.sh ran, as $FF_DIR/runs records.
func runsIn(ff string) int {
	return lineCount(filepath.Join(ff, "runs"))
}

func logOutcome(t *testing.T, o *outcome) {
	t.Helper()
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("exit %d\nstdout:\n%s\nstderr:\n%s", o.code, o.stdout, o.stderr)
		}
	})
}

// ffListedMain is the source of ffListedFiles. The binary prints shown(n)
// and label(n) for its n arguments; ID-A-01 runs it with two and wants
// "false" first, ID-A-02 with three and wants "true" first. The own tests
// check quick and spare, call wait, and never call shown, idle or label.
const ffListedMain = `package main

import (
	"flag"
	"fmt"
)

func main() {
	flag.Parse()
	n := flag.NArg()
	fmt.Println(shown(n), label(n))
}

// shown is checked by the listed tests: its mutant survives the own tests,
// and ID-A-01 kills it.
func shown(n int) bool {
	return n > 2
}

// idle is never called: its mutant is uncovered.
func idle(n int) bool {
	return n > 3
}

// quick is checked by the own test, which kills its mutant.
func quick(n int) bool {
	return n > 4
}

// wait is called by the own test, where its mutant blocks.
func wait(n int) bool {
	return n > 5
}

// label is printed and never checked: its mutant survives every test.
func label(n int) bool {
	return n > 6
}

// spare is checked by the own test, which kills its mutant.
func spare(n int) bool {
	return n > 8
}
`

// ffListedFiles is a module whose binary an e2e harness tests by listed
// test ID, as mutate_listed_test.go's does: -tests=<IDs, by commas>, and
// with ITOS_CC_TEST_COVERDIR set, GOCOVERDIR=<it>/<ID> for each test's
// binary. With FF_FAIL_ALONE set, the harness fails whenever it runs
// without coverage, as the clean baseline of a selection runs it, once the
// file FF_FAIL_AFTER names exists, if it names one. TestWait, on wait's
// mutant, marks $FF_DIR/started and waits for a release that never comes.
var ffListedFiles = map[string]string{
	"go.mod":  "module example.com/ff\n\ngo 1.22\n",
	"main.go": ffListedMain,
	"main_test.go": `package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestQuick(t *testing.T) {
	if !quick(5) || quick(4) {
		t.Fatal("quick")
	}
}

func TestWait(t *testing.T) {
	if !wait(5) {
		return
	}
	dir := os.Getenv("FF_DIR")
	os.WriteFile(filepath.Join(dir, "started"), nil, 0o644)
	for {
		if _, err := os.Stat(filepath.Join(dir, "release")); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSpare(t *testing.T) {
	if !spare(9) || spare(8) {
		t.Fatal("spare")
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
	"time"
)

var selected = flag.String("tests", "", "the IDs of the tests to run, by commas: all of them when empty")

var features = []struct {
	id   string
	args []string
	want string
}{
	{"ID-A-01", []string{"x", "y"}, "false"},
	{"ID-A-02", []string{"x", "y", "z"}, "true"},
}

func TestFeatures(t *testing.T) {
	split := os.Getenv("ITOS_CC_TEST_COVERDIR")
	covered := split != "" || os.Getenv("GOCOVERDIR") != ""
	if !covered && os.Getenv("FF_FAIL_ALONE") != "" {
		for after := os.Getenv("FF_FAIL_AFTER"); after != ""; time.Sleep(10 * time.Millisecond) {
			if _, err := os.Stat(after); err == nil {
				break
			}
		}
		t.Fatal("the listed tests fail when they run without coverage")
	}
	bin := filepath.Join(t.TempDir(), "ff.exe")
	build := []string{"build", "-o", bin, ".."}
	if covered {
		build = []string{"build", "-cover", "-o", bin, ".."}
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
		if got := strings.Fields(string(out)); len(got) == 0 || got[0] != f.want {
			t.Errorf("%s: printed %q, want %s first", f.id, out, f.want)
		}
	}
}
`,
	"itos-cc.yaml": `mutation:
  tests:
    list: printf "ID-A-01\nID-A-02\n"
    run: go test -count=1 ./e2e -args -tests={pattern}
    ids_pattern: "{ids}"
    join:
      each: "{id}"
      sep: ","
`,
}

// ffListedRepo makes ffListedFiles a Git repository and the test's
// directory (useDir), then commits each function of judged with its
// signature split over two lines, a change of its code, as a comment's
// would be none, so a run with --since HEAD~1 judges those alone. FF_DIR points at a fresh
// directory, which it returns.
func ffListedRepo(t *testing.T, judged ...string) string {
	t.Helper()
	moduleRepo(t, ffListedFiles)
	for _, function := range judged {
		head := "func " + function + "(n int) bool {\n"
		edit(t, "main.go", head, "func "+function+"(\n\tn int) bool {\n")
	}
	commitAll(t, "judge "+strings.Join(judged, " "))
	ff := t.TempDir()
	useEnv(t, "FF_DIR", ff)
	return ff
}

// listedRun runs mutation run --json --since HEAD~1 with args.
func listedRun(t *testing.T, args ...string) outcome {
	t.Helper()
	return itosCc(t, append([]string{"mutation", "run", "--json", "--no-annotate", "--since", "HEAD~1"}, args...)...)
}
