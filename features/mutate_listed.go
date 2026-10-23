// Listed-test scenarios observe commands, snapshots and the public graph API.
package features

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

type listedMutation struct {
	m              *mutateState
	made           bool
	fresh          bool
	original, held string
	graph          []byte
	// slice39List is the list command's output a slice-39 scenario set, ""
	// for the fixture's own; slice39B renames ID-A-01 to ID-B-01.
	slice39List string
	slice39B    bool
}

func listedMutationSteps(sc *godog.ScenarioContext, m *mutateState) {
	l := &listedMutation{m: m}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*l = listedMutation{m: m}
		return ctx, nil
	})
	sc.Step(`^itos-cc\.yaml names mutation\.tests with list, run, ids_pattern and join$`, l.project)
	sc.Step(`^the list command prints "ID-A-01<tab>a\.feature" and "ID-A-02"$`, l.project)
	sc.Step(`^a Go module whose harness, when ITOS_CC_TEST_COVERDIR is set, gives each test's binary GOCOVERDIR=<dir>/<test-id>$`, l.project)
	sc.Step(`^its tests ID-A-01 and ID-A-02 reach different lines of the binary$`, l.project)
	sc.Step(`^the module of ID-MUT-122, whose harness ignores ITOS_CC_TEST_COVERDIR$`, l.unsplit)
	sc.Step(`^a mutant its file's own tests kill, and one only ID-A-02 kills$`, l.project)
	sc.Step(`^a line only ID-A-01 reaches, and a line no test reaches$`, l.project)
	sc.Step(`^a mutant only ID-A-02 kills$`, l.project)
	sc.Step(`^a fresh outcome recorded with scope "listed" and tests \["ID-A-02"\]$`, l.recorded)
	sc.Step(`^itos-cc\.yaml has no mutation\.tests$`, l.noListed)
	sc.Step(`^the list command exits 1$`, l.listFails)
	sc.Step(`^a run recorded a mutant ID-A-02 kills, ID-A-02 defined in a\.feature$`, l.freshRun)
	sc.Step(`^nothing has changed since$`, l.nothingChanged)
	sc.Step(`^a\.feature has changed since$`, func() error { return l.edit("a.feature") })
	sc.Step(`^b\.feature, which defines only tests that do not cover it, has changed since$`, func() error { return l.edit("b.feature") })
	sc.Step(`^itos-cc\.yaml lists features/\*_test\.go under mutation\.tests\.support$`, l.freshProject)
	sc.Step(`^a run recorded listed kills and kills by own tests$`, l.freshRun)
	sc.Step(`^features/steps_test\.go has changed since$`, func() error { return l.edit("features/steps_test.go") })
	sc.Step(`^a\.feature has changed since a run recorded a kill by ID-A-02$`, func() error {
		if err := l.freshRun(); err != nil {
			return err
		}
		return l.edit("a.feature")
	})
	sc.Step(`^the graph is built$`, l.buildGraph)
	sc.Step(`^the function holding that kill is marked stale, as mutation check calls it$`, l.graphAgrees)
	sc.Step(`^listed tests ID-A-01, which runs in about a second alone, and ID-A-02, which takes about ten$`, l.slowProject)
	sc.Step(`^a mutant only ID-A-01 reaches makes the binary hang$`, l.hangProject)
	sc.Step(`^two mutants whose lines only ID-A-02 reaches, both surviving their own tests$`, func() error {
		if err := l.project(); err != nil {
			return err
		}
		return l.declare()
	})
	sc.Step(`^ID-A-02 fails when it runs alone without a mutant, though the listed tests pass together$`, l.failingSelection)
	sc.Step(`^the list command runs once$`, l.listOnce)
	sc.Step(`^the listed tests are ID-A-01, defined in a\.feature, and ID-A-02, with no file$`, l.inventory)
	sc.Step(`^the listed tests run once for coverage$`, l.splitCoverage)
	sc.Step(`^each line the binary ran is covered by the IDs of the tests that reached it$`, l.coverageIDs)
	sc.Step(`^the run command runs once per listed test for coverage, each selecting that test alone$`, l.fallbackCoverage)
	sc.Step(`^each line is covered by the same IDs as in ID-MUT-122$`, l.coverageIDs)
	sc.Step(`^the first is killed without any listed test running for it$`, l.ownFirst)
	sc.Step(`^the second runs its own tests, survives them, then runs the run command selecting ID-A-02 alone, and is killed$`, l.listedKilled)
	sc.Step(`^the first line's mutants run and are not uncovered$`, l.firstCovered)
	sc.Step(`^only the second line's mutants are uncovered$`, l.onlyTwiceUncovered)
	sc.Step(`^its snapshot entry records scope "listed" and tests \["ID-A-02"\]$`, l.snapshotListed)
	sc.Step(`^its mutant in --json has "tests": \["ID-A-02"\]$`, l.jsonListed)
	sc.Step(`^that mutant runs the run command selecting ID-A-02, after its own tests survive it$`, l.sampleListed)
	sc.Step(`^no "mutation\.mismatch" is reported$`, l.noMismatch)
	sc.Step(`^no list command runs, and ITOS_CC_TEST_COVERDIR is not set$`, l.noListRuns)
	sc.Step(`^the outcomes and the snapshot are as before$`, l.withoutListedOutcomes)
	sc.Step(`^the problem is "tests\.list-failed", with its exit code$`, l.listProblem)
	sc.Step(`^no mutant runs, no snapshot is written, and the exit code is 1$`, l.nothingJudged)
	sc.Step(`^"itos-cc mutation run" reuses that kill$`, l.reuses)
	sc.Step(`^the problem is "mutation\.stale" for that mutant's function, and its message names a\.feature$`, func() error { return l.stale([]string{"greet"}, "a.feature") })
	sc.Step(`^"itos-cc mutation run" runs that mutant again, its own tests first$`, l.reruns)
	sc.Step(`^each function holding a listed kill is "mutation\.stale", its message naming features/steps_test\.go$`, func() error { return l.stale([]string{"main", "greet"}, "features/steps_test.go") })
	sc.Step(`^the functions whose kills were all by their own tests stay fresh$`, l.ownFresh)
	sc.Step(`^ID-A-01 runs once alone without a mutant before that mutant's run$`, func() error { return l.baselineCount("ID-A-01", true) })
	sc.Step(`^the mutant times out after ten times that run plus 5 seconds, not after ten times both tests' time$`, l.selectionTimeout)
	sc.Step(`^its outcome is "timeout", counted killed$`, l.timeoutKilled)
	sc.Step(`^ID-A-02 runs alone without a mutant exactly once$`, func() error { return l.baselineCount("ID-A-02", false) })
	sc.Step(`^the problem is "tests\.selection-failed", with the IDs of the selection$`, l.selectionProblem)
	sc.Step(`^no mutant that would run ID-A-02 is recorded killed, and its file's snapshot keeps what it held$`, l.selectionUnchanged)
	sc.Step(`^the listed tests fail when they all run for coverage, though they pass together when a mutant's run selects them$`, l.failingCoverage)
	sc.Step(`^the problem is "tests\.coverage-failed", with the command that ran every listed test and its exit code$`, func() error { return l.coverageProblem("ID-A-01", "ID-A-02") })
	sc.Step(`^the problem is "tests\.coverage-failed", with the command that ran ID-A-02 alone and its exit code$`, func() error { return l.coverageProblem("ID-A-02") })
	sc.Step(`^no problem is "([^"]+)"$`, l.noProblem)
	slice39FingerprintSteps(sc, l)
	listedImporterSteps(sc, l)
}

func (l *listedMutation) project() error {
	if l.made {
		return nil
	}
	l.made = true
	m, w := l.m, l.m.w
	m.file = "main.go"
	record, err := m.testRecords()
	if err != nil {
		return err
	}
	w.variables = []string{"GREET_RECORD=" + filepath.Join(record, "listed.log")}
	if err := w.d.Write(filepath.Join(record, "listed.log"), nil); err != nil {
		return err
	}
	// Keep fixture source stable for baseline hashes and snapshot comparisons.
	// These are ordinary documented options, independent of what a Then expects.
	w.prepare = func(args []string) []string {
		command := slices.Index(args, "mutation")
		if command < 0 || command+1 >= len(args) {
			return args
		}
		action := args[command+1]
		if action != "run" && action != "sample" {
			return args
		}
		has := func(name string) bool {
			return slices.ContainsFunc(args, func(arg string) bool { flag, _, _ := strings.Cut(arg, "="); return flag == name })
		}
		if !has("--workers") {
			args = append(args, "--workers", "2")
		}
		if action == "run" && !has("--no-annotate") {
			args = append(args, "--no-annotate")
		}
		return args
	}
	for name, text := range listedFiles {
		if err := w.write(name, text); err != nil {
			return err
		}
	}
	l.original = m.read("main.go")
	if err := w.d.Git(w.dir, "init", "-q"); err != nil {
		return err
	}
	if err := w.d.Git(w.dir, "add", "."); err != nil {
		return err
	}
	if err := w.d.Git(w.dir, "commit", "-qm", "fixture"); err != nil {
		return err
	}
	return l.declare()
}

func (l *listedMutation) declare() error {
	w := l.m.w
	main := l.m.read("main.go")
	lineAt := func(text string) int {
		for i, line := range strings.Split(main, "\n") {
			if strings.Contains(line, text) {
				return i + 1
			}
		}
		return 0
	}
	lines := map[int][]string{}
	for text, ids := range map[string][]string{
		"n := len(os.Args) - 1":   {"ID-A-01", "ID-A-02"},
		"fmt.Fprintln(os.Stderr,": {"ID-A-01", "ID-A-02"},
		"if n > 1 {":              {"ID-A-01", "ID-A-02"},
		"return fmt.Sprint(n-1,":  {"ID-A-02"},
		"if n == 1 {":             {"ID-A-01"},
		"for n > 1 {":             {"ID-A-01"},
		"return i > 2":            {"ID-A-01", "ID-A-02"},
	} {
		if line := lineAt(text); line > 0 {
			lines[line] = ids
		}
	}
	t := Tests{Language: "go", Command: "go test -count=1 -failfast ./.", Elapsed: time.Second, Listed: &ListedTests{
		Files: map[string]string{"ID-A-01": "a.feature", "ID-A-02": ""}, Lines: map[string]map[int][]string{"main.go": lines}, Split: !strings.Contains(l.m.read("e2e/e2e_test.go"), `split := ""`), Elapsed: map[string]time.Duration{"ID-A-01": time.Second, "ID-A-02": time.Second},
		Surviving: map[string][]Site{}, TimingOut: map[string][]Site{},
	}}
	sites, err := l.m.sitesOf("main.go")
	if err != nil {
		return err
	}
	for _, s := range sites {
		site := Site{File: s.file, Line: s.line, Original: s.original, Replacement: s.replacement}
		if s.line != lineAt("return i > 2") {
			t.Surviving = append(t.Surviving, site)
		}
		if s.line == lineAt("fmt.Fprintln(os.Stderr,") || s.line == lineAt("return i > 2") {
			for _, id := range []string{"ID-A-01", "ID-A-02"} {
				t.Listed.Surviving[id] = append(t.Listed.Surviving[id], site)
			}
		}
		if s.line == lineAt("if n > 1 {") {
			t.Listed.Surviving["ID-A-02"] = append(t.Listed.Surviving["ID-A-02"], site)
		}
		if s.line == lineAt("for n > 1 {") {
			t.Listed.TimingOut["ID-A-01"] = append(t.Listed.TimingOut["ID-A-01"], site)
		}
	}
	if l.fresh {
		t.Listed.Files = map[string]string{"ID-A-01": "c.feature", "ID-A-02": "a.feature", "ID-A-03": "b.feature"}
	}
	if l.slice39List != "" {
		t.Listed.ListOutput = l.slice39List
	}
	if l.slice39B {
		slice39Rename(t.Listed, "ID-A-01", "ID-B-01")
	}
	if strings.Contains(main, "for n > 1 {") {
		t.Listed.Elapsed["ID-A-02"] = 10 * time.Second
	}
	if slices.Contains(w.variables, "GREET_FAIL_ALONE=1") {
		t.Listed.Failing = []string{"ID-A-02"}
	}
	if slices.Contains(w.variables, failCoverageVariable) {
		t.Listed.CoverageExit = 1
	}
	if strings.Contains(l.m.read("itos-cc.yaml"), "list.go fail") {
		t.Listed.ListFails = true
	}
	if !strings.Contains(l.m.read("itos-cc.yaml"), "tests:") {
		t.Listed = nil
	}
	// The harness records each run of its programs in listed.log: the own
	// tests' of package main, the listed tests' of e2e, the list command's.
	trace := &TestTrace{File: filepath.Join(l.m.records, "listed.log"), Source: "main.go", Tests: map[string]string{"main_test.go": "own", "e2e/e2e_test.go": "run"}, CoverDir: true}
	t.Trace = trace
	if t.Listed != nil {
		t.Listed.ListTrace = trace.File
	}
	if err := w.d.Tests(w.dir, t); err != nil {
		return err
	}
	if t.Listed != nil {
		if err := w.d.Tests(w.dir, Tests{Language: "go", Command: "go test -count=1 ./e2e -args -tests={pattern}", Trace: trace}); err != nil {
			return err
		}
	}
	// The own test, TestHalf, runs half alone: Go's coverage lists every
	// other line holding a mutant unexecuted.
	report := "TN:\nSF:main.go\n"
	for _, line := range []int{listedArgsLine, listedStderrLine, listedIfLine, listedManyLine, listedOneLine, lineAt("return i > 2"), listedTwiceLine} {
		hits := 0
		if line == lineAt("return i > 2") {
			hits = 1
		}
		report += fmt.Sprintf("DA:%d,%d\n", line, hits)
	}
	return w.d.Coverage(w.dir, Coverage{Language: "go", Report: report + "end_of_record\n"})
}

var listedLineTests = map[int][]string{
	listedArgsLine: {"ID-A-01", "ID-A-02"}, listedStderrLine: {"ID-A-01", "ID-A-02"}, listedIfLine: {"ID-A-01", "ID-A-02"}, listedManyLine: {"ID-A-02"}, listedOneLine: {"ID-A-01"},
}

func (l *listedMutation) replace(name, old, new string) error {
	text := l.m.read(name)
	if !strings.Contains(text, old) {
		return fmt.Errorf("fixture %s lacks %q", name, old)
	}
	return l.m.w.write(name, strings.Replace(text, old, new, 1))
}
func (l *listedMutation) unsplit() error {
	if err := l.project(); err != nil {
		return err
	}
	if err := l.replace("e2e/e2e_test.go", `split := os.Getenv("ITOS_CC_TEST_COVERDIR")`, `split := ""`); err != nil {
		return err
	}
	return l.declare()
}
func (l *listedMutation) noListed() error {
	if err := l.project(); err != nil {
		return err
	}
	if err := l.m.w.write("itos-cc.yaml", "{}\n"); err != nil {
		return err
	}
	return l.declare()
}
func (l *listedMutation) listFails() error {
	if err := l.project(); err != nil {
		return err
	}
	if err := l.replace("itos-cc.yaml", "list: go run ./testdata/list.go", "list: go run ./testdata/list.go fail"); err != nil {
		return err
	}
	return l.declare()
}
func (l *listedMutation) record() error {
	if err := l.m.w.runWith(l.m.w.env(), "mutation run --json main.go"); err != nil {
		return err
	}
	if err := l.snapshotListed(); err != nil {
		return err
	}
	return l.clearRecords()
}
func (l *listedMutation) recorded() error {
	if err := l.project(); err != nil {
		return err
	}
	return l.record()
}
func (l *listedMutation) freshProject() error {
	if err := l.project(); err != nil {
		return err
	}
	if l.fresh {
		return nil
	}
	l.fresh = true
	for _, c := range [][3]string{
		{"main.go", "fmt.Fprintln(os.Stderr, n*2, half(n))", "fmt.Fprintln(os.Stderr, half(n))"},
		{"testdata/list.go", `"ID-A-01\ta.feature\nID-A-02\n"`, `"ID-A-01\tc.feature\nID-A-02\ta.feature\nID-A-03\tb.feature\n"`},
		{"itos-cc.yaml", "      sep: \",\"\n", "      sep: \",\"\n    support: [\"features/*_test.go\"]\n"},
	} {
		if err := l.replace(c[0], c[1], c[2]); err != nil {
			return err
		}
	}
	for name, text := range map[string]string{"a.feature": "Feature: a\n", "b.feature": "Feature: b\n", "c.feature": "Feature: c\n", "features/steps_test.go": "package features\n\nimport \"testing\"\n\nfunc TestSteps(t *testing.T) {}\n"} {
		if err := l.m.w.write(name, text); err != nil {
			return err
		}
	}
	return l.declare()
}
func (l *listedMutation) freshRun() error {
	if err := l.freshProject(); err != nil {
		return err
	}
	if err := l.record(); err != nil {
		return err
	}
	l.m.held = map[string]string{}
	for _, name := range []string{"main.go", "main_test.go", "a.feature", "b.feature", "c.feature", "features/steps_test.go", ".metrics/mutate/main.go.json"} {
		l.m.held[name] = l.m.read(name)
	}
	return nil
}
func (l *listedMutation) nothingChanged() error {
	return l.m.fixtureUnchanged()
}
func (m *mutateState) fixtureUnchanged() error {
	if len(m.held) == 0 {
		return fmt.Errorf("no recorded fixture to compare")
	}
	for name, want := range m.held {
		if got := m.read(name); got != want {
			return fmt.Errorf("%s changed since the recorded run", name)
		}
	}
	return nil
}
func (l *listedMutation) edit(name string) error {
	return l.m.w.write(name, l.m.read(name)+"\n// changed\n")
}

// records are what the harness's programs recorded of their runs, a JSON
// object each, in order: the list command's, "test" "list", the own
// tests', "own", and the listed tests', "run", with the IDs it ran
// ("tests"), the coverage directory it was given ("coverdir") and the
// digest of the main.go it ran ("source_sha256").
func (l *listedMutation) records() ([]map[string]any, error) {
	data, err := l.m.w.d.Read(filepath.Join(l.m.records, "listed.log"))
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for _, line := range outputLines(strings.ReplaceAll(string(data), "\r\n", "\n")) {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("harness record %q: %w", line, err)
		}
		out = append(out, record)
	}
	return out, nil
}
func (l *listedMutation) clearRecords() error {
	return l.m.w.d.Write(filepath.Join(l.m.records, "listed.log"), nil)
}
func (l *listedMutation) snapshot() (map[string]any, error) {
	data, err := l.m.w.readIn(l.m.w.dir, ".metrics/mutate/main.go.json")
	if err != nil {
		return nil, err
	}
	var snap map[string]any
	err = json.Unmarshal(data, &snap)
	return snap, err
}
func snapshotEntries(snap map[string]any) []map[string]any {
	var out []map[string]any
	units, _ := snap["units"].([]any)
	for _, u := range units {
		entries, _ := u.(map[string]any)["mutants"].([]any)
		for _, entry := range entries {
			out = append(out, entry.(map[string]any))
		}
	}
	return out
}
func (l *listedMutation) entries() ([]map[string]any, error) {
	snap, err := l.snapshot()
	if err != nil {
		return nil, fmt.Errorf("listed snapshot: %w; actual run:\n%s", err, l.m.w.report())
	}
	entries := snapshotEntries(snap)
	want := listedMutants
	if l.fresh {
		want--
	}
	if strings.Contains(l.original, "for n > 1 {") {
		want += 2
	}
	if len(entries) != want {
		return nil, fmt.Errorf("snapshot has %d mutants, want %d", len(entries), want)
	}
	return entries, nil
}
func entryIDs(entry map[string]any) []string {
	var ids []string
	values, _ := entry["tests"].([]any)
	for _, v := range values {
		ids = append(ids, fmt.Sprint(v))
	}
	return ids
}
func (l *listedMutation) assertLines(entries []map[string]any, line int, check func(map[string]any) bool) error {
	n := 0
	for _, entry := range entries {
		if lineOf(entry) == line {
			n++
			if !check(entry) {
				return fmt.Errorf("line %d: unexpected mutant %v\n%s", line, entry, l.m.w.report())
			}
		}
	}
	if n == 0 {
		return fmt.Errorf("no mutant on line %d", line)
	}
	return nil
}
func (l *listedMutation) listOnce() error {
	lines, err := l.records()
	if err != nil {
		return err
	}
	n := 0
	for _, record := range lines {
		if record["test"] == "list" {
			n++
		}
	}
	if n != 1 {
		return fmt.Errorf("list ran %d times, want once: %v", n, lines)
	}
	return nil
}
func (l *listedMutation) snapshotListed() error {
	entries, err := l.entries()
	if err != nil {
		return err
	}
	return l.assertLines(entries, listedManyLine, func(e map[string]any) bool {
		return e["outcome"] == "killed" && e["scope"] == "listed" && slices.Equal(entryIDs(e), []string{"ID-A-02"})
	})
}
func (l *listedMutation) jsonListed() error {
	entries, err := l.m.greetMutants()
	if err != nil {
		return err
	}
	return l.assertLines(entries, listedManyLine, func(e map[string]any) bool { return slices.Equal(entryIDs(e), []string{"ID-A-02"}) })
}
func (l *listedMutation) sampleListed() error {
	return l.ranAfterOwn(listedManyLine, "ID-A-02", "killed")
}
func (l *listedMutation) noMismatch() error {
	if err := l.m.w.exitsWith(0); err != nil {
		return err
	}
	if strings.Contains(l.m.w.stdout+l.m.w.stderr, "mutation.mismatch") {
		return fmt.Errorf("sample reports mismatch\n%s", l.m.w.report())
	}
	return nil
}
func (l *listedMutation) noListRuns() error {
	lines, err := l.records()
	if err != nil {
		return err
	}
	own := 0
	for _, record := range lines {
		if record["test"] == "own" {
			own++
			if record["coverdir"] != "" {
				return fmt.Errorf("own tests saw ITOS_CC_TEST_COVERDIR: %v", record)
			}
		}
		if record["test"] == "list" || record["test"] == "run" && record["coverdir"] != "" {
			return fmt.Errorf("unexpected listed invocation %v", record)
		}
	}
	if own == 0 {
		return fmt.Errorf("no own test recorded its environment")
	}
	return nil
}
func (l *listedMutation) withoutListedOutcomes() error {
	snap, err := l.snapshot()
	if err != nil {
		return err
	}
	if snap["listed"] != nil {
		return fmt.Errorf("snapshot has listed inventory: %v", snap["listed"])
	}
	entries := snapshotEntries(snap)
	if len(entries) != listedMutants {
		return fmt.Errorf("%d mutants, want %d", len(entries), listedMutants)
	}
	for _, e := range entries {
		want := "uncovered"
		if lineOf(e) == listedHalfLine {
			want = "killed"
		}
		if e["outcome"] != want || e["scope"] != nil || e["tests"] != nil {
			return fmt.Errorf("changed own outcome %v", e)
		}
	}
	return nil
}
func (l *listedMutation) listProblem() error {
	if err := l.m.w.problemIs("tests.list-failed", "", ""); err != nil {
		return err
	}
	if l.m.w.problem["exit_code"] != float64(1) {
		return fmt.Errorf("list problem lacks exit_code 1: %v", l.m.w.problem)
	}
	return nil
}
func (l *listedMutation) nothingJudged() error {
	if len(trials(l.m.w.stderr, "main.go")) != 0 || !l.snapshotUnwritten() {
		return fmt.Errorf("failed run judged or wrote mutants\n%s", l.m.w.report())
	}
	lines, err := l.records()
	if err != nil {
		return err
	}
	for _, record := range lines {
		if l.mutantRan(record) {
			return fmt.Errorf("harness ran a mutant: %v", record)
		}
	}
	return l.m.w.exitsWith(1)
}
