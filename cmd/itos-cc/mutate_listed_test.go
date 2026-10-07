package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The scenarios of "Rule: Mutants run only the listed tests that reach them"
// in features/mutate.feature. Each runs mutate in a Go module whose binary,
// greet, is tested end to end by a harness in another package,
// e2e/e2e_test.go, that runs two tests by ID: ID-A-01 runs the binary with
// one argument and checks it prints "one", ID-A-02 with three and checks
// "2 more". A selection of them is passed as -tests=<IDs, by commas>; when
// ITOS_CC_TEST_COVERDIR is set, the harness gives each test's binary
// GOCOVERDIR=<that directory>/<its ID>. itos-cc.yaml lists them with
// testdata/list.go, which prints "ID-A-01<tab>a.feature" and "ID-A-02". The
// file's own test, in package main, checks half alone. Each invocation of the
// harness and of the list command is recorded, a line each, in the file
// GREET_RECORD names.

const listedMain = `package main

import (
	"fmt"
	"os"
)

func main() {
	n := len(os.Args) - 1
	fmt.Println(greet(n))
	fmt.Fprintln(os.Stderr, n*2, half(n))
}

// greet tells one from many.
func greet(n int) string {
	if n > 1 {
		return fmt.Sprint(n-1, " more")
	}
	if n == 1 {
		return "one"
	}
	return "none"
}

// half is run by the binary, and checked by the file's own test.
func half(i int) bool {
	return i > 2
}

// twice is never run.
func twice(i int) int {
	return i * 2
}
`

// The lines of listedMain that hold mutants, and the tests that reach them.
const (
	listedArgsLine   = 9  // n := len(os.Args) - 1: both tests; - → +, 1 → 0, killed
	listedStderrLine = 11 // n*2: both tests, never checked; * → /, survives
	listedIfLine     = 16 // n > 1: both tests; > → >=, 1 → 0, killed by ID-A-01
	listedManyLine   = 17 // n-1: ID-A-02 alone; - → +, 1 → 0, killed by it
	listedOneLine    = 19 // n == 1: ID-A-01 alone; == → !=, 1 → 0, killed by it
	listedHalfLine   = 27 // i > 2: both tests and the own test; > → >=, killed by the own test
	listedTwiceLine  = 32 // i * 2: no test; * → /, uncovered
)

// listedMutants is how many mutants listedMain holds.
const listedMutants = 11

var listedFiles = map[string]string{
	"go.mod":  "module example.com/greet\n\ngo 1.22\n",
	"main.go": listedMain,
	"main_test.go": `package main

import "testing"

func TestHalf(t *testing.T) {
	if !half(3) || half(2) {
		t.Fatal("half")
	}
}
`,
	"e2e/e2e_test.go": `package e2e

import (
	"flag"
	"fmt"
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
	{"ID-A-01", []string{"x"}, "one"},
	{"ID-A-02", []string{"x", "y", "z"}, "2 more"},
}

func TestFeatures(t *testing.T) {
	if record := os.Getenv("GREET_RECORD"); record != "" {
		f, err := os.OpenFile(record, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(f, "run tests=%s coverdir=%s\n", *selected, os.Getenv("ITOS_CC_TEST_COVERDIR"))
		f.Close()
	}
	split := os.Getenv("ITOS_CC_TEST_COVERDIR")
	bin := filepath.Join(t.TempDir(), "greet.exe")
	build := []string{"build", "-o", bin, ".."}
	if split != "" || os.Getenv("GOCOVERDIR") != "" {
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
		if got := strings.TrimSpace(string(out)); got != f.want {
			t.Errorf("%s: greet printed %q, want %q", f.id, got, f.want)
		}
	}
}
`,
	"testdata/list.go": `package main

import (
	"fmt"
	"os"
)

func main() {
	if record := os.Getenv("GREET_RECORD"); record != "" {
		f, err := os.OpenFile(record, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintln(f, "list")
			f.Close()
		}
	}
	if len(os.Args) > 1 && os.Args[1] == "fail" {
		fmt.Fprintln(os.Stderr, "list: failing as asked")
		os.Exit(1)
	}
	fmt.Print("ID-A-01\ta.feature\nID-A-02\n")
}
`,
	"itos-cc.yaml": `mutation:
  tests:
    list: go run ./testdata/list.go
    run: go test -count=1 ./e2e -args -tests={pattern}
    ids_pattern: "{ids}"
    join:
      each: "{id}"
      sep: ","
`,
}

// listedRepo makes the module in a git repository and makes it the working
// directory, with each of change's replacements made in the file it names
// first. It returns the file each run of the harness or the list command
// records a line in.
func listedRepo(t *testing.T, change map[string][2]string) string {
	t.Helper()
	files := map[string]string{}
	for name, text := range listedFiles {
		files[name] = text
	}
	for name, r := range change {
		if !strings.Contains(files[name], r[0]) {
			t.Fatalf("%s holds no %q", name, r[0])
		}
		files[name] = strings.Replace(files[name], r[0], r[1], 1)
	}
	moduleRepo(t, files)
	record := filepath.Join(t.TempDir(), "record")
	t.Setenv("GREET_RECORD", record)
	return record
}

// harnessRuns is the lines the harness and the list command recorded, and
// empties the record.
func harnessRuns(t *testing.T, record string) []string {
	t.Helper()
	data, err := os.ReadFile(record)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	os.Remove(record)
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// listedJSON runs mutation run --json with args on main.go and returns its
// mutants as plain maps, with what was printed.
func listedJSON(t *testing.T, args ...string) ([]map[string]any, outcome) {
	t.Helper()
	o := mutateCovered(t, append(append([]string{"--json"}, args...), greetSource)...)
	for _, f := range o.rawFiles(t) {
		if f["file"] != greetSource {
			continue
		}
		var out []map[string]any
		list, _ := f["mutants"].([]any)
		for _, m := range list {
			out = append(out, m.(map[string]any))
		}
		if len(out) != listedMutants {
			t.Fatalf("%d mutants of %s, want %d\n%s\nstderr:\n%s", len(out), greetSource, listedMutants, o.stdout, o.stderr)
		}
		return out, o
	}
	t.Fatalf("no %s in files:\n%s\nstderr:\n%s", greetSource, o.stdout, o.stderr)
	return nil, o
}

// listedTests is the "tests" of m, or nil.
func listedTests(m map[string]any) []string {
	var out []string
	list, _ := m["tests"].([]any)
	for _, id := range list {
		s, _ := id.(string)
		out = append(out, s)
	}
	return out
}

// rawSnapshot is the snapshot of main.go as plain maps.
func rawSnapshot(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(".metrics", "mutate", "main.go.json"))
	if err != nil {
		t.Fatalf("no snapshot of main.go: %v", err)
	}
	var snap map[string]any
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatal(err)
	}
	return snap
}

// snapshotMutants is every mutant of the snapshot, as plain maps.
func snapshotMutants(snap map[string]any) []map[string]any {
	var out []map[string]any
	units, _ := snap["units"].([]any)
	for _, u := range units {
		mutants, _ := u.(map[string]any)["mutants"].([]any)
		for _, m := range mutants {
			out = append(out, m.(map[string]any))
		}
	}
	return out
}

// coverageRuns is each line of stderr on which itos-cc runs the harness for
// coverage.
func coverageRuns(stderr string) []string {
	var out []string
	for _, l := range strings.Split(stderr, "\n") {
		if strings.HasPrefix(l, "coverage: ") && strings.Contains(l, "-tests=") {
			out = append(out, l)
		}
	}
	return out
}

// wantLineTests is the IDs of the tests that reach each line whose mutants
// survive the own test.
var wantLineTests = map[int][]string{
	listedArgsLine:   {"ID-A-01", "ID-A-02"},
	listedStderrLine: {"ID-A-01", "ID-A-02"},
	listedIfLine:     {"ID-A-01", "ID-A-02"},
	listedManyLine:   {"ID-A-02"},
	listedOneLine:    {"ID-A-01"},
}

// checkLineTests checks that each mutant on a line of wantLineTests ran the
// tests that reach its line.
func checkLineTests(t *testing.T, mutants []map[string]any) {
	t.Helper()
	for _, m := range mutants {
		want, ok := wantLineTests[mutantLine(m)]
		if !ok {
			continue
		}
		if got := listedTests(m); m["scope"] != "listed" || !slices.Equal(got, want) {
			t.Errorf("%s: scope %v, tests %q, want \"listed\" and %q, the tests that reach its line", describe(m), m["scope"], got, want)
		}
	}
}

// @ID-MUT-121
func TestListedTestsComeFromItosCcYaml(t *testing.T) {
	record := listedRepo(t, nil)

	_, o := listedJSON(t)
	lists := 0
	for _, l := range harnessRuns(t, record) {
		if l == "list" {
			lists++
		}
	}
	if lists != 1 {
		t.Fatalf("the list command ran %d times, want once\nstderr:\n%s", lists, o.stderr)
	}
	listed, _ := rawSnapshot(t)["listed"].(map[string]any)
	if want := map[string]any{"ID-A-01": "a.feature", "ID-A-02": ""}; fmt.Sprint(listed) != fmt.Sprint(want) {
		t.Errorf("the snapshot's listed tests %v, want %v", listed, want)
	}
}

// @ID-MUT-122
func TestPerTestCoverageTakesOneRunWhenTheHarnessSplitsIt(t *testing.T) {
	listedRepo(t, nil)

	mutants, o := listedJSON(t)
	runs := coverageRuns(o.stderr)
	if len(runs) != 1 || !strings.HasSuffix(runs[0], "-tests=ID-A-01,ID-A-02") {
		t.Fatalf("coverage ran the harness as %q, want once, with every listed test\nstderr:\n%s", runs, o.stderr)
	}
	checkLineTests(t, mutants)
	if t.Failed() {
		t.Logf("stdout:\n%s\nstderr:\n%s", o.stdout, o.stderr)
	}
}

// @ID-MUT-123
func TestWithoutTheHarnesssHelpCoverageTakesOneRunPerTest(t *testing.T) {
	listedRepo(t, map[string][2]string{
		"e2e/e2e_test.go": {`split := os.Getenv("ITOS_CC_TEST_COVERDIR")`, `split := ""`},
	})

	mutants, o := listedJSON(t)
	runs := coverageRuns(o.stderr)
	for _, id := range []string{"ID-A-01", "ID-A-02"} {
		n := 0
		for _, r := range runs {
			if strings.HasSuffix(r, "-tests="+id) {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("coverage ran the harness as %q, want once with %s alone\nstderr:\n%s", runs, id, o.stderr)
		}
	}
	checkLineTests(t, mutants)
	if t.Failed() {
		t.Logf("stdout:\n%s\nstderr:\n%s", o.stdout, o.stderr)
	}
}

// @ID-MUT-124
func TestAMutantRunsItsOwnTestsFirstAndTheCoveringTestsOnlyIfItSurvives(t *testing.T) {
	record := listedRepo(t, nil)

	mutants, o := listedJSON(t)
	for _, m := range mutants {
		if mutantLine(m) == listedHalfLine && (m["outcome"] != "killed" || m["scope"] != "own" || m["tests"] != nil) {
			t.Fatalf("%s: scope %v, tests %v, want killed by its own tests, no listed test run", describe(m), m["scope"], m["tests"])
		}
	}
	for _, l := range strings.Split(o.stderr, "\n") {
		if strings.Contains(l, fmt.Sprintf(" %s:%d ", greetSource, listedHalfLine)) && strings.Contains(l, "ID-A-") {
			t.Errorf("stderr line %q: want no listed test run for a mutant its own tests kill", l)
		}
	}
	many := 0
	for _, m := range mutants {
		if mutantLine(m) != listedManyLine {
			continue
		}
		many++
		if m["outcome"] != "killed" || m["scope"] != "listed" || !slices.Equal(listedTests(m), []string{"ID-A-02"}) {
			t.Errorf("%s: scope %v, tests %v, want killed by ID-A-02 alone after its own tests", describe(m), m["scope"], m["tests"])
		}
		want := fmt.Sprintf(" %s:%d `%v` → `%v` survived its own tests, then killed with ID-A-02 (", greetSource, listedManyLine, m["original"], m["replacement"])
		if !strings.Contains(o.stderr, want) {
			t.Errorf("stderr lacks %q", want)
		}
	}
	if many != 2 {
		t.Errorf("%d mutants on line %d, want 2", many, listedManyLine)
	}
	if lines := harnessRuns(t, record); !slices.Contains(lines, "run tests=ID-A-02 coverdir=") {
		t.Errorf("the harness ran as %q, want a run selecting ID-A-02 alone", lines)
	}
	if t.Failed() {
		t.Logf("stderr:\n%s", o.stderr)
	}
}

// @ID-MUT-125
func TestUncoveredMeansNeitherTheOwnTestsNorAListedTestReachTheLine(t *testing.T) {
	listedRepo(t, nil)

	mutants, o := listedJSON(t, "--fail-uncovered")
	for _, m := range mutants {
		if mutantLine(m) == listedOneLine && (m["outcome"] == "uncovered" || m["reused"] == true) {
			t.Fatalf("%s: want it run, on a line ID-A-01 reaches\nstderr:\n%s", describe(m), o.stderr)
		}
	}
	for _, m := range mutants {
		if (mutantLine(m) == listedTwiceLine) != (m["outcome"] == "uncovered") {
			t.Errorf("%s: want only the mutants on line %d, which no test reaches, uncovered", describe(m), listedTwiceLine)
		}
	}
	if o.code != 1 {
		t.Errorf("exit %d, want 1 for the uncovered mutant and the survivor", o.code)
	}
	if t.Failed() {
		t.Logf("stderr:\n%s", o.stderr)
	}
}

// @ID-MUT-126
func TestAnOutcomeDecidedByListedTestsRecordsThem(t *testing.T) {
	listedRepo(t, nil)

	mutants, o := listedJSON(t)
	found := 0
	for _, m := range snapshotMutants(rawSnapshot(t)) {
		if line, _ := m["line"].(float64); int(line) != listedManyLine {
			continue
		}
		found++
		if m["scope"] != "listed" || !slices.Equal(listedTests(m), []string{"ID-A-02"}) {
			t.Errorf("the snapshot's mutant %v: want scope \"listed\" and tests [\"ID-A-02\"]", m)
		}
	}
	if found != 2 {
		t.Fatalf("%d mutants on line %d in the snapshot, want 2\nstderr:\n%s", found, listedManyLine, o.stderr)
	}
	for _, m := range mutants {
		if mutantLine(m) == listedManyLine && !slices.Equal(listedTests(m), []string{"ID-A-02"}) {
			t.Errorf("%s: --json \"tests\" %v, want [\"ID-A-02\"]", describe(m), m["tests"])
		}
	}
}

// @ID-MUT-127
func TestMutationSampleRerunsAListedOutcomeWithItsTests(t *testing.T) {
	record := listedRepo(t, nil)
	listedJSON(t)
	// What a run records of the mutants only ID-A-02 kills, as it is.
	path := filepath.Join(".metrics", "mutate", "main.go.json")
	snap := rawSnapshot(t)
	for _, m := range snapshotMutants(snap) {
		if line, _ := m["line"].(float64); int(line) == listedManyLine {
			m["outcome"], m["scope"], m["tests"] = "killed", "listed", []any{"ID-A-02"}
		}
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, string(data)+"\n")
	harnessRuns(t, record)

	o := cli(t, "mutation", "sample", "--count", "100", "--workers", "1", "--seed", "s", "--json")
	var out struct {
		Files []struct {
			Mutants []map[string]any `json:"mutants"`
		} `json:"files"`
		Problems []map[string]any `json:"problems"`
	}
	if err := json.Unmarshal([]byte(o.stdout), &out); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s\nstderr:\n%s", err, o.stdout, o.stderr)
	}
	many := 0
	for _, f := range out.Files {
		for _, m := range f.Mutants {
			if mutantLine(m) == listedManyLine {
				many++
				if m["outcome"] != "killed" || m["scope"] != "listed" {
					t.Errorf("sampled %s: scope %v, want killed in scope \"listed\"", describe(m), m["scope"])
				}
			}
		}
	}
	if many != 2 {
		t.Errorf("%d sampled mutants on line %d, want 2", many, listedManyLine)
	}
	if lines := harnessRuns(t, record); !slices.Contains(lines, "run tests=ID-A-02 coverdir=") {
		t.Errorf("the harness ran as %q, want a run selecting ID-A-02 alone", lines)
	}
	for _, p := range out.Problems {
		if p["rule"] == "mutation.mismatch" {
			t.Errorf("problem %v, want no mismatch", p)
		}
	}
	if t.Failed() {
		t.Logf("stdout:\n%s\nstderr:\n%s", o.stdout, o.stderr)
	}
}

// @ID-MUT-128
func TestWithoutMutationTestsNothingChanges(t *testing.T) {
	record := listedRepo(t, map[string][2]string{"itos-cc.yaml": {listedFiles["itos-cc.yaml"], "{}\n"}})

	mutants, o := listedJSON(t)
	for _, l := range harnessRuns(t, record) {
		if l == "list" || strings.HasPrefix(l, "run ") && !strings.HasSuffix(l, " coverdir=") {
			t.Errorf("recorded %q: want no list command and no ITOS_CC_TEST_COVERDIR", l)
		}
	}
	for _, m := range mutants {
		want := "uncovered"
		if mutantLine(m) == listedHalfLine {
			want = "killed"
		}
		if m["outcome"] != want || m["scope"] != "own" || m["tests"] != nil {
			t.Errorf("%s: scope %v, tests %v, want %s by the own test's coverage alone, as before", describe(m), m["scope"], m["tests"], want)
		}
	}
	snap := rawSnapshot(t)
	if _, ok := snap["listed"]; ok {
		t.Errorf("the snapshot has \"listed\": %v", snap["listed"])
	}
	for _, m := range snapshotMutants(snap) {
		if _, ok := m["tests"]; ok || m["scope"] != nil {
			t.Errorf("the snapshot's mutant %v: want no scope and no tests", m)
		}
	}
	if t.Failed() {
		t.Logf("stderr:\n%s", o.stderr)
	}
}

// @ID-MUT-129
func TestAListCommandThatFailsJudgesNothing(t *testing.T) {
	record := listedRepo(t, map[string][2]string{"itos-cc.yaml": {"list: go run ./testdata/list.go", "list: go run ./testdata/list.go fail"}})

	o := mutateCovered(t, "--json", greetSource)
	m := o.json(t)
	p := m.problem("tests.list-failed")
	if p == nil {
		t.Fatalf("no tests.list-failed problem in:\n%s\nstderr:\n%s", o.stdout, o.stderr)
	}
	if p["exit_code"] != float64(1) {
		t.Errorf("problem %v, want its exit_code 1", p)
	}
	if strings.Contains(o.stderr, "itos-cc: [1/") || slices.ContainsFunc(harnessRuns(t, record), func(l string) bool { return strings.HasPrefix(l, "run ") }) {
		t.Errorf("a mutant ran:\n%s", o.stderr)
	}
	if _, err := os.Stat(filepath.Join(".metrics", "mutate", "main.go.json")); !os.IsNotExist(err) {
		t.Errorf("a snapshot of main.go was written (%v)", err)
	}
	if o.code != 1 {
		t.Errorf("exit %d, want 1", o.code)
	}
}
