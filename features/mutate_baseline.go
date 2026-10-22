package features

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Running mutants: the baseline first, the timeouts it sets, a failing
// baseline, and the workers that run mutants at once.

// The module of "The baseline proves the copy works": Count, whose
// mutants of i - 1 never end, its every other mutant killed by its test,
// which takes half a second and records where it ran and whether its
// copy of board.go is mutated.
const countGo = `package board

// Count is how many steps take i down to zero.
func Count(i int) int {
	n := 0
	for i > 0 {
		i = i - 1
		n++
	}
	return n
}
`

// aboutToRun writes the module and runs its mutants.
func (m *mutateState) aboutToRun() error {
	records, err := m.testRecords()
	if err != nil {
		return err
	}
	m.file, m.original = "src/board.go", countGo
	test := goRecorder("board", records, filepath.Join(m.w.dir, "src"), "time") + fmt.Sprintf(`
func TestCount(t *testing.T) {
	start := time.Now()
	time.Sleep(500 * time.Millisecond)
	got := Count(3)
	text, _ := os.ReadFile("board.go")
	record(t, map[string]any{"start": start.UnixMilli(), "end": time.Now().UnixMilli(), "mutated": string(text) != %s})
	if got != 3 {
		t.Fatal("Count")
	}
}
`, strconv.Quote(countGo))
	if err := m.writeAll(map[string]string{"go.mod": mutateModule, m.file: countGo, "src/board_test.go": test}); err != nil {
		return err
	}
	if err := m.w.d.Tests(m.w.dir, Tests{Language: "go", Command: goTests.Command, Elapsed: 500 * time.Millisecond,
		TimingOut: []Site{{m.file, 7, "-", "+"}, {m.file, 7, "1", "0"}}}); err != nil {
		return err
	}
	m.launched = time.Now()
	return m.w.runEnv(m.w.dir, m.w.env(), "mutation", "run", "--no-coverage", filepath.FromSlash(m.file))
}

// baselineFirst is whether the first run of the tests was unmutated, in a
// worker's copy, announced before any mutant's, and mutants' runs came
// after it.
func (m *mutateState) baselineFirst() error {
	runs, err := m.mutantRuns()
	if err != nil {
		return err
	}
	slices.SortStableFunc(runs, func(a, b testRecord) int { return int(a.Start - b.Start) })
	if runs[0].Mutated || !slices.ContainsFunc(runs[1:], func(r testRecord) bool { return r.Mutated }) {
		return fmt.Errorf("the first run of the tests was not the unmutated one before the mutants': %+v\n%s", runs, m.w.report())
	}
	lines := strings.Split(strings.ReplaceAll(m.w.stderr, "\r\n", "\n"), "\n")
	base := slices.IndexFunc(lines, func(l string) bool { return baselineLine.MatchString(l) })
	first := slices.IndexFunc(lines, func(l string) bool { return trialOutcome.MatchString(l) })
	if base < 0 || first < 0 || first < base {
		return fmt.Errorf("the run did not announce its baseline before its first mutant\n%s", m.w.report())
	}
	return nil
}

// timeoutAllowance is the time beyond the factor times its baseline a
// mutant's run takes before it times out, as the feature says it.
const timeoutAllowance = 5.0

// reaping is how much longer than its timeout a timed-out run may take to
// end on the platform: a second, and on Windows, where itos-cc ends the
// command's process but not the processes it started, the two seconds it
// lets those hold its output.
func reaping(platform string) float64 {
	if platform == "windows" {
		return 3
	}
	return 1
}

// timedOut are the last run's mutants of the file that timed out, and the
// longest of those that did not, in seconds.
func (m *mutateState) timedOut() ([]trial, float64, error) {
	var out []trial
	longest := -1.0
	for _, t := range trialsOf(m.w.stderr, m.file) {
		if t.outcome == "timeout" {
			out = append(out, t)
		} else {
			longest = max(longest, t.elapsed)
		}
	}
	if len(out) == 0 || longest < 0 {
		return nil, 0, fmt.Errorf("the run timed out no mutant of %s beside one it judged\n%s", m.file, m.w.report())
	}
	return out, longest, nil
}

// durationSetsTimeout is whether the mutants that timed out took the
// factor times the baseline's time, plus the allowance: the baseline no
// shorter than its test took, and no longer than from the run's launch to
// the start of the first mutant's test, with the time a timed-out run takes
// to end. The run says no baseline it measured: it began after the launch,
// and the mutants' runs began after it ended, so the run's own times bound
// it whatever the load, and a timeout not set from it, fixed or a default,
// is past them.
func (m *mutateState) durationSetsTimeout() error {
	runs, err := m.mutantRuns()
	if err != nil {
		return err
	}
	slices.SortStableFunc(runs, func(a, b testRecord) int { return int(a.Start - b.Start) })
	first := slices.IndexFunc(runs, func(r testRecord) bool { return r.Mutated })
	if first < 0 || m.launched.IsZero() {
		return fmt.Errorf("no mutant's test ran after the baseline of a run whose launch was noted: %+v\n%s", runs, m.w.report())
	}
	shortest := float64(runs[0].End-runs[0].Start) / 1000
	longest := float64(runs[first].Start-m.launched.UnixMilli()) / 1000
	out, _, err := m.timedOut()
	if err != nil {
		return err
	}
	factor := 10.0
	low, high := factor*shortest+timeoutAllowance, factor*longest+timeoutAllowance+reaping(m.w.d.Platform())
	for _, t := range out {
		if t.elapsed < low || t.elapsed > high {
			return fmt.Errorf("the mutant at line %d timed out after %.1fs, not between %.1fs and %.1fs, as a baseline of %.2fs to %.1fs sets\n%s",
				t.line, t.elapsed, low, high, shortest, longest, m.w.report())
		}
	}
	return nil
}

// The Python project of a baseline that takes seconds: wait, whose
// mutant > → >= never ends, and past, whose mutant its test kills, which
// sleeps the baseline's time first. pytest is shadowed, so its tests run
// with unittest, as quick to start as Python is.
const (
	waitPy   = "def wait(i):\n    while i > 2:\n        pass\n    return i\n\n\ndef past(i):\n    return i > 5\n"
	waitTest = "import time\nimport unittest\n\nfrom board import past, wait\n\n\nclass TestBoard(unittest.TestCase):\n" +
		"    def test_board(self):\n        time.sleep(%s)\n        self.assertEqual(wait(2), 2)\n" +
		"        self.assertFalse(past(5))\n        self.assertTrue(past(6))\n"
	// waitGo is the Go source of a baseline under a second: past, whose
	// mutant the shell command waitCommand lets run 30 seconds, and low,
	// whose mutant it passes.
	waitGo = "package board\n\n// Past is whether i is past five.\nfunc Past(i int) bool {\n\treturn i > 5\n}\n\n" +
		"// Low is whether i is under three.\nfunc Low(i int) bool {\n\treturn i < 3\n}\n"
	// It quotes nothing: cmd, the platform shell on Windows, gets the line
	// as one argument whose quotes it does not read as Go escapes them.
	// i...5 is i > 5 alone, not its mutant i >= 5.
	waitCommand = `grep -q i...5 src/board.go && sleep %s || sleep 30`
)

// baselineTook writes a project whose tests take the time said: Python's,
// for a time of seconds, else a Go source tested by a shell command, quick
// to start, its run a sleep of that time.
func (m *mutateState) baselineTook(n int, unit string) error {
	took := time.Duration(n) * time.Millisecond
	if unit == "seconds" {
		took = time.Duration(n) * time.Second
	}
	m.baseline = took
	seconds := strconv.FormatFloat(took.Seconds(), 'f', -1, 64)
	if took >= time.Second {
		// board.py is at the root, where coverage, which sets no
		// PYTHONPATH, imports it.
		m.file = "board.py"
		if err := m.writeProject(map[string][]string{"tests/test_board.py": {m.file}}, map[string]string{
			"pyproject.toml": pyproject, m.file: waitPy, "pytest.py": noPytest,
			"tests/test_board.py": fmt.Sprintf(waitTest, seconds),
		}); err != nil {
			return err
		}
		if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "python", Report: "SF:board.py\nDA:2,1\nDA:4,1\nDA:8,1\nend_of_record\n"}); err != nil {
			return err
		}
		if err := m.w.d.Tests(m.w.dir, Tests{Language: "python", Command: "python3 -m unittest -f tests.test_board", Elapsed: took,
			TimingOut: []Site{{m.file, 2, ">", ">="}}}); err != nil {
			return err
		}
		// pytest.py shadows pytest, which python3 cannot import here.
		return m.pythonProbe(false)
	}
	m.file, m.command = "src/board.go", fmt.Sprintf(waitCommand, seconds)
	if err := m.writeAll(map[string]string{"go.mod": mutateModule, m.file: waitGo}); err != nil {
		return err
	}
	return m.w.d.Tests(m.w.dir, Tests{Language: "go", Command: m.command, Elapsed: took,
		TimingOut: []Site{{m.file, 5, ">", ">="}}, Surviving: []Site{{m.file, 10, "<", "<="}}})
}

// runWithDefaultFactor runs the Given's project's mutants with no
// --timeout-factor, which the sentence says is factor.
func (m *mutateState) runWithDefaultFactor(factor int) error {
	if m.command == "" {
		return errors.New("no Given wrote a project whose tests a command runs")
	}
	m.factor = float64(factor)
	return m.w.runEnv(m.w.dir, m.w.env(), "mutation", "run", "--no-coverage", "--test-command", m.command, filepath.FromSlash(m.file))
}

// factorOf is the timeout factor of the last run: its --timeout-factor,
// else the default the When said.
func (m *mutateState) factorOf() (float64, error) {
	if i := slices.Index(m.w.last.args, "--timeout-factor"); i >= 0 && i+1 < len(m.w.last.args) {
		return strconv.ParseFloat(m.w.last.args[i+1], 64)
	}
	if m.factor == 0 {
		return 0, errors.New("the run gave no --timeout-factor, and no When said its default")
	}
	return m.factor, nil
}

// numbers are the factors a sentence writes as a word.
var factorWords = map[string]float64{"two": 2, "five": 5, "ten": 10, "twenty": 20}

// timesOutAfter is whether every mutant the last run timed out took the
// seconds said: no less, and no more than the factor times what the
// longest other mutant's run took past the Given's baseline, and the time
// a timed-out run takes to end.
// Where the sentence says how, it is the factor times the baseline, plus
// the allowance.
func (m *mutateState) timesOutAfter(text, times, allowance string) error {
	after, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return err
	}
	factor, err := m.factorOf()
	if err != nil {
		return err
	}
	if m.baseline == 0 {
		return errors.New("no Given said how long the baseline took")
	}
	if times != "" {
		plus, _ := strconv.ParseFloat(allowance, 64)
		if factorWords[times] != factor || math.Abs(factor*m.baseline.Seconds()+plus-after) > 1e-9 {
			return fmt.Errorf("%s seconds is not %s times the baseline of %s, plus %s seconds, at the run's factor of %g", text, times, m.baseline, allowance, factor)
		}
	}
	out, longest, err := m.timedOut()
	if err != nil {
		return err
	}
	high := after + factor*max(0, longest-m.baseline.Seconds()) + reaping(m.w.d.Platform())
	for _, t := range out {
		if t.elapsed < after || t.elapsed > high {
			return fmt.Errorf("the mutant at line %d timed out after %.1fs, not %ss, its baseline %s and the longest other run %.1fs\n%s",
				t.line, t.elapsed, text, m.baseline, longest, m.w.report())
		}
	}
	return nil
}

// timeoutCountsKilled is whether the run's summary counts every mutant
// that timed out among those it killed.
func (m *mutateState) timeoutCountsKilled() error {
	out, _, err := m.timedOut()
	if err != nil {
		return err
	}
	killed := 0
	for _, t := range trialsOf(m.w.stderr, m.file) {
		if t.outcome == "killed" {
			killed++
		}
	}
	if s, ok := summaryOf(m.w.stdout, m.file); !ok || s.killed != killed+len(out) {
		return fmt.Errorf("the summary does not count the %d mutants that timed out among the %d killed\n%s", len(out), killed+len(out), m.w.report())
	}
	return nil
}

// failingTest prints thirty numbered lines, then fails.
const failingTest = "from board import place\n\n\ndef test_place():\n    for i in range(1, 31):\n" +
	"        print(\"line %02d\" % i)\n    assert not place(1, 2)\n"

// failingBaseline writes a Python project whose tests pass, records a
// run's results of the file, then makes its tests fail without any
// mutation, printing 30 lines and nothing else.
func (m *mutateState) failingBaseline(file string) error {
	if file == "src/board.ts" && m.failingSample != nil {
		return m.failingSample()
	}
	if file != "src/board.py" {
		return fmt.Errorf("no project is written for %s", file)
	}
	m.file = file
	if err := m.writeProject(map[string][]string{"tests/test_board.py": {file}}, map[string]string{"pyproject.toml": pyproject, file: pyBoard, "tests/test_board.py": pytestBoard}); err != nil {
		return err
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "python", Report: "SF:src/board.py\nDA:2,1\nend_of_record\n"}); err != nil {
		return err
	}
	command := "python3 -m pytest -q -x -p no:cacheprovider tests/test_board.py"
	if err := m.w.d.Tests(m.w.dir, Tests{Language: "python", Command: command}); err != nil {
		return err
	}
	if err := m.pythonProbe(true); err != nil {
		return err
	}
	if code, stdout, stderr, err := m.w.capture(m.w.dir, m.w.env(), "mutation", "run", filepath.FromSlash(file)); err != nil || code != 0 {
		return fmt.Errorf("the run that records %s: exit %d, %v\n%s%s", file, code, err, stdout, stderr)
	}
	m.snapshot = ".metrics/mutate/" + file + ".json"
	if m.recorded = m.read(m.snapshot); m.recorded == "" {
		return fmt.Errorf("the run recorded no %s", m.snapshot)
	}
	// pytest's own report off, and its capture with it, so the failing
	// test's lines are all the tests print.
	if err := m.writeAll(map[string]string{"tests/test_board.py": failingTest, "pyproject.toml": capsPrinting}); err != nil {
		return err
	}
	return m.w.d.Tests(m.w.dir, Tests{Language: "python", Command: command, Baseline: failingLines()})
}

// failingLines is what failingTest prints.
func failingLines() string {
	var printed strings.Builder
	for i := 1; i <= 30; i++ {
		fmt.Fprintf(&printed, "line %02d\n", i)
	}
	return printed.String()
}

// wholeOutputFollows is whether all the failing test printed follows
// stdout's line saying the baseline fails, before the next file's summary:
// its 30 lines, under the 40 a report keeps whole, with no line left out.
func (m *mutateState) wholeOutputFollows() error {
	lines := outputLines(strings.ReplaceAll(m.w.stdout, "\r\n", "\n"))
	at := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, "baseline tests fail") })
	if at < 0 {
		return fmt.Errorf("stdout says no baseline fails\n%s", m.w.report())
	}
	follow := lines[at+1:]
	if end := slices.IndexFunc(follow, func(l string) bool { return summaryLine.MatchString(l) }); end >= 0 {
		follow = follow[:end]
	}
	if want := strings.Split(strings.TrimSuffix(failingLines(), "\n"), "\n"); !slices.Equal(follow, want) {
		return fmt.Errorf("%d lines follow, not the test's whole output of %d\n%s", len(follow), len(want), m.w.report())
	}
	return nil
}

// snapshotUnchanged is whether file's snapshot is as the Given's run
// recorded it.
func (m *mutateState) snapshotUnchanged(file string) error {
	if m.recorded == "" || m.snapshot != ".metrics/mutate/"+file+".json" {
		return fmt.Errorf("no Given recorded a snapshot of %s", file)
	}
	if now := m.read(m.snapshot); now != m.recorded {
		return fmt.Errorf("the snapshot of %s changed:\n%s\nwas\n%s", file, now, m.recorded)
	}
	return nil
}

// workersProject writes a module of as many functions as mutants may run
// at once, each with one mutant, and a test of them all that sleeps three
// seconds, recording when.
func (m *mutateState) workersProject() error {
	records, err := m.testRecords()
	if err != nil {
		return err
	}
	n := max(4, runtime.NumCPU()/2)
	var source, checks strings.Builder
	source.WriteString("package board\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&source, "\n// F%02d is whether i is past %d.\nfunc F%02d(i int) bool {\n\treturn i > %d\n}\n", i, i+2, i, i+2)
		fmt.Fprintf(&checks, "\tif F%02d(%d) || !F%02d(%d) {\n\t\tt.Fatal(\"F%02d\")\n\t}\n", i, i+2, i, i+3, i)
	}
	m.file = "src/board.go"
	test := goRecorder("board", records, filepath.Join(m.w.dir, "src"), "time") + `
func TestAll(t *testing.T) {
	start := time.Now()
	time.Sleep(3 * time.Second)
	record(t, map[string]any{"start": start.UnixMilli(), "end": time.Now().UnixMilli()})
` + checks.String() + "}\n"
	if err := m.writeAll(map[string]string{"go.mod": mutateModule, m.file: source.String(), "src/board_test.go": test}); err != nil {
		return err
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "go"}); err != nil {
		return err
	}
	return m.w.d.Tests(m.w.dir, Tests{Language: "go", Command: goTests.Command, Elapsed: 3 * time.Second})
}

// atOnce is how many runs of the tests in workers' copies ran at the same
// time, at most.
func (m *mutateState) atOnce() (int, error) {
	runs, err := m.mutantRuns()
	if err != nil {
		return 0, err
	}
	most := 0
	for _, r := range runs {
		n := 0
		for _, other := range runs {
			if other.Start <= r.Start && r.Start < other.End {
				n++
			}
		}
		most = max(most, n)
	}
	return most, nil
}

// upToAtOnce is whether n runs of the tests ran at once, and never more.
func (m *mutateState) upToAtOnce(n int) error {
	got, err := m.atOnce()
	if err != nil {
		return err
	}
	if got != n {
		return fmt.Errorf("%d mutants ran at once, at most, not %d\n%s", got, n, m.w.report())
	}
	return nil
}

// defaultWorkers runs the last run again without --workers, its results
// and records gone, and is whether half the CPUs, at least one, ran at
// once.
func (m *mutateState) defaultWorkers() error {
	args := slices.Clone(m.w.last.args)
	if i := slices.Index(args, "--workers"); i >= 0 {
		args = slices.Delete(args, i, i+2)
	}
	for _, gone := range []string{filepath.Join(m.w.dir, ".metrics", "mutate"), filepath.Join(m.records, "runs")} {
		if err := m.w.d.Remove(gone); err != nil {
			return err
		}
	}
	if err := m.w.runEnv(m.w.dir, m.w.env(), args...); err != nil {
		return err
	}
	return m.upToAtOnce(max(1, runtime.NumCPU()/2))
}
