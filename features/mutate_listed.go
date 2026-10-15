// Listed-test scenarios observe commands, snapshots and the public graph API.
package features

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

type listedMutation struct {
	m              *mutateState
	made           bool
	fresh          bool
	original, held string
	unchanged      map[string]string
	graph          []byte
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
	sc.Step(`^the mutant times out after three times that run plus 5 seconds, not after three times both tests' time$`, l.selectionTimeout)
	sc.Step(`^its outcome is "timeout", counted killed$`, l.timeoutKilled)
	sc.Step(`^ID-A-02 runs alone without a mutant exactly once$`, func() error { return l.baselineCount("ID-A-02", false) })
	sc.Step(`^the problem is "tests\.selection-failed", with the IDs of the selection$`, l.selectionProblem)
	sc.Step(`^no mutant that would run ID-A-02 is recorded killed, and its file's snapshot keeps what it held$`, l.selectionUnchanged)
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
		if name == "e2e/e2e_test.go" {
			text = strings.Replace(text, "import (\n", "import (\n\t\"crypto/sha256\"\n", 1)
			text = strings.Replace(text, `fmt.Fprintf(f, "run tests=%s coverdir=%s\n", *selected, os.Getenv("ITOS_CC_TEST_COVERDIR"))`, `src, _ := os.ReadFile(filepath.Join("..", "main.go"))
        fmt.Fprintf(f, "run tests=%s coverdir=%s main=%x\n", *selected, os.Getenv("ITOS_CC_TEST_COVERDIR"), sha256.Sum256(src))`, 1)
		}
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
	t := Tests{Language: "go", Command: "go test -count=1 -failfast .", Elapsed: time.Second, Listed: &ListedTests{
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
	if strings.Contains(main, "for n > 1 {") {
		t.Listed.Elapsed["ID-A-02"] = 10 * time.Second
	}
	if slices.Contains(w.variables, "GREET_FAIL_ALONE=1") {
		t.Listed.Failing = []string{"ID-A-02"}
	}
	if strings.Contains(l.m.read("itos-cc.yaml"), "list.go fail") {
		t.Listed.ListFails = true
	}
	if !strings.Contains(l.m.read("itos-cc.yaml"), "tests:") {
		t.Listed = nil
	}
	if err := w.d.Tests(w.dir, t); err != nil {
		return err
	}
	return w.d.Coverage(w.dir, Coverage{Language: "go", Report: fmt.Sprintf("TN:\nSF:main.go\nDA:%d,1\nend_of_record\n", lineAt("return i > 2"))})
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
	l.unchanged = map[string]string{}
	for _, name := range []string{"main.go", "main_test.go", "a.feature", "b.feature", "c.feature", "features/steps_test.go", ".metrics/mutate/main.go.json"} {
		l.unchanged[name] = l.m.read(name)
	}
	return nil
}
func (l *listedMutation) nothingChanged() error {
	if len(l.unchanged) == 0 {
		return fmt.Errorf("no recorded fixture to compare")
	}
	for name, want := range l.unchanged {
		if got := l.m.read(name); got != want {
			return fmt.Errorf("%s changed since the recorded run", name)
		}
	}
	return nil
}
func (l *listedMutation) edit(name string) error {
	return l.m.w.write(name, l.m.read(name)+"\n// changed\n")
}
func (l *listedMutation) records() ([]string, error) {
	data, err := l.m.w.d.Read(filepath.Join(l.m.records, "listed.log"))
	if err != nil {
		return nil, err
	}
	return outputLines(strings.ReplaceAll(string(data), "\r\n", "\n")), nil
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
		return nil, err
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
	for _, line := range lines {
		if line == "list" {
			n++
		}
	}
	if n != 1 {
		return fmt.Errorf("list ran %d times, want once: %q", n, lines)
	}
	return nil
}
func (l *listedMutation) inventory() error {
	snap, err := l.snapshot()
	if err != nil {
		return err
	}
	got, _ := snap["listed"].(map[string]any)
	if len(got) != 2 || got["ID-A-01"] != "a.feature" || got["ID-A-02"] != "" {
		return fmt.Errorf("listed inventory %v", got)
	}
	return nil
}
func (l *listedMutation) coverageRuns() []string {
	var out []string
	for _, line := range outputLines(l.m.w.stderr) {
		if strings.HasPrefix(line, "itos-cc: coverage ") && strings.Contains(line, "-tests=") {
			out = append(out, line)
		}
	}
	return out
}
func (l *listedMutation) splitCoverage() error {
	runs := l.coverageRuns()
	if len(runs) != 1 || !strings.HasSuffix(runs[0], "-tests=ID-A-01,ID-A-02") {
		return fmt.Errorf("coverage runs %q, want one selecting both", runs)
	}
	return nil
}
func (l *listedMutation) fallbackCoverage() error {
	runs := l.coverageRuns()
	for _, id := range []string{"ID-A-01", "ID-A-02"} {
		n := 0
		for _, r := range runs {
			if strings.HasSuffix(r, "-tests="+id) {
				n++
			}
		}
		if n != 1 {
			return fmt.Errorf("coverage runs %q, want one selecting %s alone", runs, id)
		}
	}
	return nil
}
func (l *listedMutation) coverageIDs() error {
	entries, err := l.entries()
	if err != nil {
		return err
	}
	for line, ids := range listedLineTests {
		if err := l.assertLines(entries, line, func(e map[string]any) bool { return e["scope"] == "listed" && slices.Equal(entryIDs(e), ids) }); err != nil {
			return err
		}
	}
	return nil
}
func (l *listedMutation) ownFirst() error {
	entries, err := l.entries()
	if err != nil {
		return err
	}
	if err := l.assertLines(entries, listedHalfLine, func(e map[string]any) bool { return e["outcome"] == "killed" && e["scope"] == nil && e["tests"] == nil }); err != nil {
		return err
	}
	for _, t := range trialsOf(l.m.w.stderr, "main.go") {
		if t.line == listedHalfLine && strings.Contains(t.outcome, "ID-A-") {
			return fmt.Errorf("own killed mutant ran listed tests: %v", t)
		}
	}
	return nil
}
func (l *listedMutation) listedKilled() error {
	if err := l.snapshotListed(); err != nil {
		return err
	}
	return l.ranAfterOwn(listedManyLine, "ID-A-02", "killed")
}
func (l *listedMutation) ranAfterOwn(line int, id, outcome string) error {
	n := 0
	for _, t := range trialsOf(l.m.w.stderr, "main.go") {
		if t.line == line {
			n++
			if t.outcome != "survived its own tests, then "+outcome+" with "+id {
				return fmt.Errorf("trial %v did not run own then %s", t, id)
			}
		}
	}
	if n != 2 {
		return fmt.Errorf("%d trials on line %d, want two\n%s", n, line, l.m.w.report())
	}
	return nil
}
func (l *listedMutation) firstCovered() error {
	entries, err := l.entries()
	if err != nil {
		return err
	}
	if err := l.assertLines(entries, listedOneLine, func(e map[string]any) bool { return e["outcome"] != "uncovered" }); err != nil {
		return err
	}
	if !slices.Contains(trials(l.m.w.stderr, "main.go"), listedOneLine) {
		return fmt.Errorf("covered mutants never ran")
	}
	return nil
}
func (l *listedMutation) onlyTwiceUncovered() error {
	entries, err := l.entries()
	if err != nil {
		return err
	}
	n := 0
	for _, e := range entries {
		if (lineOf(e) == listedTwiceLine) != (e["outcome"] == "uncovered") {
			return fmt.Errorf("unexpected uncovered mutant %v", e)
		}
		if e["outcome"] == "uncovered" {
			n++
		}
	}
	if n != 1 {
		return fmt.Errorf("%d uncovered, want one", n)
	}
	return l.m.w.exitsWith(1)
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
	for _, line := range lines {
		if strings.HasPrefix(line, "own ") {
			own++
			if line != "own coverdir=" {
				return fmt.Errorf("own tests saw ITOS_CC_TEST_COVERDIR: %q", line)
			}
		}
		if line == "list" || strings.HasPrefix(line, "run ") && !strings.Contains(line, " coverdir= main=") {
			return fmt.Errorf("unexpected listed invocation %q", line)
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
	if len(trials(l.m.w.stderr, "main.go")) != 0 || l.m.w.d.Exists(filepath.Join(l.m.w.dir, ".metrics", "mutate", "main.go.json")) {
		return fmt.Errorf("failed list judged or wrote mutants")
	}
	lines, err := l.records()
	if err != nil {
		return err
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "run ") {
			return fmt.Errorf("harness ran after list failure")
		}
	}
	return l.m.w.exitsWith(1)
}
func (l *listedMutation) reuses() error {
	if err := l.m.w.runWith(l.m.w.env(), "mutation run --json main.go"); err != nil {
		return err
	}
	entries, err := l.m.greetMutants()
	if err != nil {
		return err
	}
	return l.assertLines(entries, listedManyLine, func(e map[string]any) bool { return e["reused"] == true && e["scope"] == "listed" })
}
func (l *listedMutation) checkObject() (map[string]any, error) {
	object, _, err := l.m.w.lastObject()
	return object, err
}
func (l *listedMutation) stale(functions []string, file string) error {
	object, err := l.checkObject()
	if err != nil {
		return err
	}
	problems, _ := object["problems"].([]any)
	for _, fn := range functions {
		found := false
		for _, p := range problems {
			entry := p.(map[string]any)
			if entry["rule"] == "mutation.stale" && entry["function"] == "example.com/greet#"+fn && strings.Contains(fmt.Sprint(entry["message"]), file) {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("no stale %s naming %s\n%s", fn, file, l.m.w.report())
		}
	}
	return nil
}
func (l *listedMutation) ownFresh() error {
	object, err := l.checkObject()
	if err != nil {
		return err
	}
	files, _ := object["files"].([]any)
	for _, f := range files {
		functions, _ := f.(map[string]any)["functions"].([]any)
		for _, fn := range functions {
			v := fn.(map[string]any)
			if v["function"] == "example.com/greet#half" && v["state"] == "fresh" {
				return nil
			}
		}
	}
	return fmt.Errorf("half is not fresh\n%s", l.m.w.report())
}
func (l *listedMutation) reruns() error {
	if err := l.m.w.runWith(l.m.w.env(), "mutation run --json main.go"); err != nil {
		return err
	}
	entries, err := l.m.greetMutants()
	if err != nil {
		return err
	}
	if err := l.assertLines(entries, listedManyLine, func(e map[string]any) bool { return e["reused"] != true && e["outcome"] == "killed" }); err != nil {
		return err
	}
	return l.ranAfterOwn(listedManyLine, "ID-A-02", "killed")
}
func (l *listedMutation) buildGraph(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var err error
	l.graph, err = l.m.w.d.Graph(ctx, Run{Dir: l.m.w.dir, Env: l.m.w.env()})
	return err
}
func (l *listedMutation) graphAgrees() error {
	var graph struct {
		Nodes []struct {
			Units []struct {
				File, Namespace, Name string
				Stale                 bool
			}
		}
	}
	if err := json.Unmarshal(l.graph, &graph); err != nil {
		return err
	}
	states := map[string]bool{}
	for _, n := range graph.Nodes {
		for _, u := range n.Units {
			if u.File == "main.go" {
				states[u.Namespace+"#"+u.Name] = u.Stale
			}
		}
	}
	if !states["example.com/greet#greet"] || states["example.com/greet#half"] {
		return fmt.Errorf("graph freshness %v", states)
	}
	if err := l.m.w.runWith(l.m.w.env(), "mutation check --json main.go"); err != nil {
		return err
	}
	if err := l.stale([]string{"greet"}, "a.feature"); err != nil {
		return err
	}
	return l.ownFresh()
}
func (l *listedMutation) slowProject() error {
	if err := l.project(); err != nil {
		return err
	}
	if err := l.replace("e2e/e2e_test.go", "\t\"testing\"\n)", "\t\"testing\"\n\t\"time\"\n)"); err != nil {
		return err
	}
	return l.replace("e2e/e2e_test.go", "\t\tcmd := exec.Command(bin, f.args...)", "\t\tif f.id == \"ID-A-02\" { time.Sleep(9*time.Second) } else { time.Sleep(700*time.Millisecond) }\n\t\tcmd := exec.Command(bin, f.args...)")
}
func (l *listedMutation) hangProject() error {
	if err := l.replace("main.go", "\t\"os\"\n)", "\t\"os\"\n\t\"time\"\n)"); err != nil {
		return err
	}
	if err := l.replace("main.go", "if n == 1 {\n", "if n == 1 {\n\t\tfor n > 1 {\n\t\t\ttime.Sleep(time.Second)\n\t\t}\n"); err != nil {
		return err
	}
	l.original = l.m.read("main.go")
	return l.declare()
}
func (l *listedMutation) baselineCount(id string, before bool) error {
	lines, err := l.records()
	if err != nil {
		return err
	}
	prefix := "run tests=" + id + " coverdir= main="
	clean := prefix + fmt.Sprintf("%x", sha256.Sum256([]byte(l.original)))
	n, at, mutated := 0, -1, -1
	for i, line := range lines {
		if line == clean {
			n++
			at = i
		} else if strings.HasPrefix(line, prefix) && mutated < 0 {
			mutated = i
		}
	}
	if n != 1 || (before && (mutated < 0 || at > mutated)) {
		return fmt.Errorf("%s unmutated runs %d at %d, first mutant %d: %q", id, n, at, mutated, lines)
	}
	if id == "ID-A-02" {
		return l.listedKilled()
	}
	return nil
}
func (l *listedMutation) selectionTimeout() error {
	re := regexp.MustCompile(`baseline of ID-A-01 passed in ([0-9.]+)s: a mutant running it times out after ([0-9.]+)s`)
	match := re.FindStringSubmatch(l.m.w.stderr)
	if match == nil {
		return fmt.Errorf("selection baseline not announced\n%s", l.m.w.report())
	}
	baseline, _ := strconv.ParseFloat(match[1], 64)
	timeout, _ := strconv.ParseFloat(match[2], 64)
	if d := timeout - (3*baseline + 5); d < -0.25 || d > .25 {
		return fmt.Errorf("baseline %.2f timeout %.2f", baseline, timeout)
	}
	re = regexp.MustCompile(`then timeout with ID-A-01 \([0-9.]+s, the listed tests ([0-9.]+)s\)`)
	runs := re.FindAllStringSubmatch(l.m.w.stderr, -1)
	if len(runs) != 2 {
		return fmt.Errorf("%d timed out trials, want two", len(runs))
	}
	for _, run := range runs {
		elapsed, _ := strconv.ParseFloat(run[1], 64)
		if elapsed < timeout-.15 || elapsed > timeout+3.5 || elapsed >= 3*(baseline+9) {
			return fmt.Errorf("trial %.2f timeout %.2f, should use selection baseline", elapsed, timeout)
		}
	}
	return nil
}
func (l *listedMutation) timeoutKilled() error {
	entries, err := l.entries()
	if err != nil {
		return err
	}
	n := 0
	for _, e := range entries {
		if e["outcome"] == "timeout" {
			n++
			if e["scope"] != "listed" || !slices.Equal(entryIDs(e), []string{"ID-A-01"}) {
				return fmt.Errorf("unexpected timeout %v", e)
			}
		}
	}
	summary, ok := summaryOf(l.m.w.stdout, "main.go")
	killed := 0
	for _, e := range entries {
		if e["outcome"] == "killed" || e["outcome"] == "timeout" {
			killed++
		}
	}
	if n != 2 || !ok || summary.killed != killed {
		return fmt.Errorf("%d timeouts, summary %v, want killed count %d", n, summary, killed)
	}
	return nil
}
func (l *listedMutation) failingSelection() error {
	if err := l.project(); err != nil {
		return err
	}
	if err := l.replace("e2e/e2e_test.go", "\tsplit := os.Getenv(\"ITOS_CC_TEST_COVERDIR\")", "\tif os.Getenv(\"GREET_FAIL_ALONE\") != \"\" && *selected == \"ID-A-02\" { t.Fatal(\"selection fails alone\") }\n\tsplit := os.Getenv(\"ITOS_CC_TEST_COVERDIR\")"); err != nil {
		return err
	}
	if err := l.declare(); err != nil {
		return err
	}
	if err := l.record(); err != nil {
		return err
	}
	snap, err := l.snapshot()
	if err != nil {
		return err
	}
	n := 0
	for _, e := range snapshotEntries(snap) {
		if lineOf(e) == listedManyLine {
			e["outcome"] = "survived"
			n++
		}
	}
	if n != 2 {
		return fmt.Errorf("%d listed mutants before retry", n)
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	l.held = string(data) + "\n"
	if err := l.m.w.write(".metrics/mutate/main.go.json", l.held); err != nil {
		return err
	}
	l.m.w.variables = append(l.m.w.variables, "GREET_FAIL_ALONE=1")
	return l.declare()
}
func (l *listedMutation) selectionProblem() error {
	if err := l.m.w.problemIs("tests.selection-failed", "", ""); err != nil {
		return err
	}
	if !slices.Equal(entryIDs(map[string]any{"tests": l.m.w.problem["ids"]}), []string{"ID-A-02"}) {
		return fmt.Errorf("selection problem %v", l.m.w.problem)
	}
	return nil
}
func (l *listedMutation) selectionUnchanged() error {
	if got := l.m.read(".metrics/mutate/main.go.json"); got != l.held {
		return fmt.Errorf("selection failure changed snapshot\n%s", got)
	}
	for _, t := range trialsOf(l.m.w.stderr, "main.go") {
		if t.line == listedManyLine && (strings.Contains(t.outcome, "killed") || strings.Contains(t.outcome, "timeout")) {
			return fmt.Errorf("failed selection decided %v", t)
		}
	}
	return nil
}
