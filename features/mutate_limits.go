package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// The steps of the Rule "A mutant's output and memory are capped, sized
// from its baseline" (slice-24) and of a failing baseline's head and tail
// (MUT-303). They read the run's --json object, its progress on stderr and
// the snapshot it writes, never its code.

// capsState is one such scenario's state beside mutateState's.
type capsState struct {
	m *mutateState
	// judged are the outcomes the fixture's tests decide for each mutant
	// no cap holds, by its site; the capped one is the fixture's
	// subject, at subject.
	judged  map[Site]string
	subject Site
	// printing is the site of the mutant whose tests pass printing 30 MiB.
	printing Site
}

func capsSteps(sc *godog.ScenarioContext, m *mutateState) {
	c := &capsState{m: m}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*c = capsState{m: m}
		return ctx, nil
	})
	sc.Step(`^a Go package whose test prints 2 MiB with nothing mutated, where one mutant makes it print 30 MiB and pass, and another makes it print without end$`, c.flooding)
	sc.Step(`^the endless printer's outcome is "over-limit", with "limit" "output", and it is counted in "killed"$`, func() error { return c.overLimit(c.subject, "output", true) })
	sc.Step(`^it ended before its timeout, and every other mutant was judged as its tests decide$`, c.endedEarly)
	sc.Step(`^the 30 MiB printer survived, under ten times the baseline's 2 MiB plus 16 MiB$`, c.printerSurvived)
	sc.Step(`^with "--output-factor 1" the 30 MiB printer is "over-limit" too, and the exit code is 0$`, c.lowerFactor)
	sc.Step(`^a Go package whose test holds little memory with nothing mutated, where one mutant makes it hold ever more, up to 2 GiB, before it fails$`, c.hoarding)
	sc.Step(`^on Linux the hoarder's outcome is "over-limit", with "limit" "memory", before it holds 2 GiB$`, c.hoarderCapped)
	sc.Step(`^every other mutant was judged as its tests decide$`, c.othersJudged)
	sc.Step(`^on macOS and Windows its outcome is "killed", and stderr says once that no memory cap holds on this system, as itos-cc cannot measure a test run's memory there$`, c.hoarderUncapped)
	sc.Step(`^the snapshot of src/board\.go records the mutant at line 7 as "over-limit", with "limit" "output", in a function unchanged since$`, c.recordedOverLimit)
	sc.Step(`^that mutant is "over-limit", with "limit" "output", reused, and counted in "killed"$`, c.reusedOverLimit)
	sc.Step(`^"itos-cc mutation check src/board\.go" counts it killed and exits 0$`, c.checkCountsKilled)
	sc.Step(`^when it now runs past its timeout, "itos-cc mutation sample --count 20 src/board\.go" reports no problem and exits 0$`, c.sampleAgrees)
	sc.Step(`^the tests of src/board\.py fail without any mutation, printing 100 numbered lines, the first naming the cause$`, c.hundredLines)
	sc.Step(`^lines 1 to 20 follow it, then "… 60 lines left out …", then lines 81 to 100$`, func() error {
		return c.headAndTail(m.w.stdout, "baseline tests of src/board.py fail; snapshot not updated")
	})
	sc.Step(`^its "output" in --json is those same lines$`, c.jsonOutput)
	sc.Step(`^"itos-cc mutation sample src/board\.py" reports the failing baseline with the same lines$`, c.sampleHeadAndTail)
}

const (
	// capsMiB is a mebibyte.
	capsMiB = 1 << 20
	// capsFlooding is the source whose tests print, 64 KiB at a time,
	// Pieces() pieces: 2 MiB, 30 MiB when more is true, and without end
	// when endless is. Piece's mutant fails its test before it prints.
	capsFlooding = `package board

// Pieces is how many 64 KiB pieces of output the test prints: 32, 2 MiB,
// unless more is asked, 30 MiB, or no end at all.
func Pieces() int {
	more := false
	endless := false
	switch {
	case endless:
		return forever
	case more:
		return many
	}
	return few
}

// Piece is how many bytes one piece holds.
func Piece() int {
	return 64 * 1024
}

const (
	forever = -1
	many    = 480
	few     = 32
)
`
	// capsFloodingTest prints the pieces with a millisecond's pause, about
	// 64 MB a second, and stops where its output is closed or at 1 GiB,
	// so a run with no cap ends it at its timeout, and an output it lost
	// ends it on every system.
	capsFloodingTest = `package board

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPrint(t *testing.T) {
	if Piece() != 64*1024 {
		t.Fatal("a piece is not 64 KiB")
	}
	piece := bytes.Repeat([]byte(strings.Repeat("x", 1023)+"\n"), 64)
	n := Pieces()
	for i := 0; n < 0 || i < n; i++ {
		if i == 16*1024 {
			t.Fatal("printed 1 GiB")
		}
		if _, err := os.Stdout.Write(piece); err != nil {
			os.Exit(2)
		}
		time.Sleep(time.Millisecond)
	}
}
`
	// capsHoarding is the source whose tests hold Blocks() blocks of
	// 64 MiB: none, 2 GiB when hoard is true, 64 MiB when its 0 is a 1.
	capsHoarding = `package board

// Blocks is how many 64 MiB blocks the test holds: none, unless it hoards.
func Blocks() int {
	hoard := false
	if hoard {
		return 32
	}
	return 0
}
`
	// capsHoardingTest takes a block every 20 ms, touching each page so
	// the system holds it, and fails holding any: at 2 GiB for the
	// hoarder, so no system holds more for long.
	capsHoardingTest = `package board

import (
	"testing"
	"time"
)

func TestHold(t *testing.T) {
	var held [][]byte
	for i := 0; i < Blocks(); i++ {
		block := make([]byte, 64<<20)
		for j := 0; j < len(block); j += 4096 {
			block[j] = 1
		}
		held = append(held, block)
		time.Sleep(20 * time.Millisecond)
	}
	if len(held) != 0 {
		t.Fatalf("held %d MiB", 64*len(held))
	}
}
`
	// capsCounting is the source whose mutant at line 7 loops for ever,
	// and whose other mutants its test kills.
	capsCounting = `package board

// Count is how many steps take i down to zero.
func Count(i int) int {
	n := 0
	for i > 0 {
		i -= 1
		n++
	}
	return n
}
`
	capsCountingTest = "package board\n\nimport \"testing\"\n\nfunc TestCount(t *testing.T) {\n\tif got := Count(3); got != 3 {\n\t\tt.Fatal(got)\n\t}\n}\n"
)

// goPackage writes the module, src/board.go holding source and its test,
// and declares what its tests do and that they cover the lines covered,
// forFile running the When's command for src/board.go.
func (c *capsState) goPackage(source, test string, tests Tests, covered ...int) error {
	m := c.m
	m.file = "src/board.go"
	if err := m.writeAll(map[string]string{"go.mod": mutateModule, m.file: source, "src/board_test.go": test}); err != nil {
		return err
	}
	var lines strings.Builder
	lines.WriteString("SF:src/board.go\n")
	for _, line := range covered {
		fmt.Fprintf(&lines, "DA:%d,1\n", line)
	}
	lines.WriteString("end_of_record\n")
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "go", Report: lines.String()}); err != nil {
		return err
	}
	tests.Language, tests.Command = "go", goTests.Command
	if err := m.w.d.Tests(m.w.dir, tests); err != nil {
		return err
	}
	m.forFile = func(args string) error { return m.w.runWith(m.w.env(), args+" "+filepath.FromSlash(m.file)) }
	return nil
}

// flooding writes the package whose mutants print 30 MiB and without end.
// go test passes a package's test output on as it comes only with -v, so
// the fixture sets it in GOFLAGS: without it, go test holds the output
// until the test ends.
func (c *capsState) flooding() error {
	file := "src/board.go"
	c.subject, c.printing = Site{file, 7, "false", "true"}, Site{file, 6, "false", "true"}
	c.judged = map[Site]string{{file, 19, "*", "/"}: "killed"}
	c.m.w.variables = append(c.m.w.variables, "GOFLAGS=-v")
	return c.goPackage(capsFlooding, capsFloodingTest, Tests{Elapsed: 100 * time.Millisecond, Output: 2 * capsMiB,
		Writing: map[Site]int64{c.printing: 30 * capsMiB}, Flooding: []Site{c.subject}})
}

// hoarding writes the package whose mutant holds ever more memory.
func (c *capsState) hoarding() error {
	file := "src/board.go"
	c.subject = Site{file, 5, "false", "true"}
	c.judged = map[Site]string{{file, 9, "0", "1"}: "killed"}
	return c.goPackage(capsHoarding, capsHoardingTest, Tests{Elapsed: 100 * time.Millisecond, Memory: 64 * capsMiB,
		Hoarding: []Site{c.subject}})
}

// capsMutant is a mutant of a run's --json object.
type capsMutant struct {
	Line        int    `json:"line"`
	Original    string `json:"original"`
	Replacement string `json:"replacement"`
	Outcome     string `json:"outcome"`
	Limit       string `json:"limit"`
	Reused      bool   `json:"reused"`
}

// capsFile is a file of a run's --json object.
type capsFile struct {
	File    string       `json:"file"`
	Killed  int          `json:"killed"`
	Mutants []capsMutant `json:"mutants"`
}

// file is the --json object's entry for the scenario's file, from stdout,
// a run's made with --json.
func (c *capsState) file(stdout string) (capsFile, error) {
	var object struct {
		Files []capsFile `json:"files"`
	}
	if err := json.Unmarshal([]byte(stdout), &object); err != nil {
		return capsFile{}, fmt.Errorf("standard output is not one JSON object: %v\n%s", err, c.m.w.report())
	}
	for _, f := range object.Files {
		if filepath.ToSlash(f.File) == c.m.file {
			return f, nil
		}
	}
	return capsFile{}, fmt.Errorf("the run lists no %s\n%s", c.m.file, c.m.w.report())
}

// mutant is the entry of f for the mutant at site.
func (f capsFile) mutant(site Site) (capsMutant, error) {
	for _, mu := range f.Mutants {
		if mu.Line == site.Line && mu.Original == site.Original && mu.Replacement == site.Replacement {
			return mu, nil
		}
	}
	return capsMutant{}, fmt.Errorf("%s lists no mutant %s:%d %s → %s: %+v", f.File, site.File, site.Line, site.Original, site.Replacement, f.Mutants)
}

// kills is how many of f's mutants count as killed: those killed, timed
// out or over a limit.
func (f capsFile) kills() int {
	n := 0
	for _, mu := range f.Mutants {
		if mu.Outcome == "killed" || mu.Outcome == "timeout" || mu.Outcome == "over-limit" {
			n++
		}
	}
	return n
}

// overLimit is whether the last run's mutant at site is over its limit,
// and, with counted, the file's "killed" counts it.
func (c *capsState) overLimit(site Site, limit string, counted bool) error {
	f, err := c.file(c.m.w.stdout)
	if err != nil {
		return err
	}
	mu, err := f.mutant(site)
	if err != nil {
		return err
	}
	if mu.Outcome != "over-limit" || mu.Limit != limit {
		return fmt.Errorf("its outcome is %q with limit %q, not \"over-limit\" with %q\n%s", mu.Outcome, mu.Limit, limit, c.m.w.report())
	}
	if counted && f.Killed != f.kills() {
		return fmt.Errorf("\"killed\" is %d, not the %d mutants killed, timed out or over a limit\n%s", f.Killed, f.kills(), c.m.w.report())
	}
	return nil
}

// othersJudged is whether every mutant the fixture judges by its tests
// has the outcome they decide.
func (c *capsState) othersJudged() error {
	f, err := c.file(c.m.w.stdout)
	if err != nil {
		return err
	}
	for site, outcome := range c.judged {
		mu, err := f.mutant(site)
		if err != nil {
			return err
		}
		if mu.Outcome != outcome {
			return fmt.Errorf("the mutant at line %d is %q, not %q as its tests decide\n%s", site.Line, mu.Outcome, outcome, c.m.w.report())
		}
	}
	return nil
}

// endedEarly is whether the endless printer ended before its timeout,
// the baseline's time, times the factor 1, plus the allowance, and the
// others are judged as their tests decide. The baseline's time is at least
// about the quickest other mutant's, whose tests build as the baseline's
// do and fail at once: the printer ends before that plus the allowance.
func (c *capsState) endedEarly() error {
	var printer *trial
	quickest := -1.0
	trials := trialsOf(c.m.w.stderr, c.m.file)
	for i, t := range trials {
		if t.line == c.subject.Line && t.original == c.subject.Original && t.replacement == c.subject.Replacement {
			printer = &trials[i]
		} else if quickest < 0 || t.elapsed < quickest {
			quickest = t.elapsed
		}
	}
	if printer == nil || quickest < 0 {
		return fmt.Errorf("stderr says nothing of the endless printer, or of no other mutant\n%s", c.m.w.report())
	}
	if printer.elapsed >= quickest+timeoutAllowance {
		return fmt.Errorf("the endless printer took %.1fs, not under the %.1fs its timeout is at least\n%s", printer.elapsed, quickest+timeoutAllowance, c.m.w.report())
	}
	return c.othersJudged()
}

// printerSurvived is whether the 30 MiB printer survived.
func (c *capsState) printerSurvived() error {
	f, err := c.file(c.m.w.stdout)
	if err != nil {
		return err
	}
	mu, err := f.mutant(c.printing)
	if err != nil {
		return err
	}
	if mu.Outcome != "survived" {
		return fmt.Errorf("the 30 MiB printer is %q, not \"survived\"\n%s", mu.Outcome, c.m.w.report())
	}
	return nil
}

// lowerFactor runs the When's command again with --output-factor 1.
func (c *capsState) lowerFactor() error {
	w := c.m.w
	if w.last == nil {
		return errors.New("no run to make again")
	}
	if err := w.runEnv(w.last.dir, w.last.env, append(slices.Clone(w.last.args), "--output-factor", "1")...); err != nil {
		return err
	}
	if err := c.overLimit(c.printing, "output", true); err != nil {
		return err
	}
	return w.exitsWith(0)
}

// hoarderCapped is, on Linux, whether the hoarder is over its memory
// limit: its test fails at 2 GiB, which a mutant killed by it would be.
func (c *capsState) hoarderCapped() error {
	if c.m.w.d.Platform() != "linux" {
		return nil
	}
	return c.overLimit(c.subject, "memory", true)
}

// capsNoMemoryCap is what a run says where it measures no memory.
const capsNoMemoryCap = "no memory cap holds on this system"

// hoarderUncapped is, on macOS and Windows, whether the hoarder is killed
// by its own test and stderr says once that no memory cap holds.
func (c *capsState) hoarderUncapped() error {
	if c.m.w.d.Platform() == "linux" {
		return nil
	}
	f, err := c.file(c.m.w.stdout)
	if err != nil {
		return err
	}
	mu, err := f.mutant(c.subject)
	if err != nil {
		return err
	}
	if mu.Outcome != "killed" {
		return fmt.Errorf("the hoarder is %q, not \"killed\"\n%s", mu.Outcome, c.m.w.report())
	}
	if n := strings.Count(c.m.w.stderr, capsNoMemoryCap); n != 1 {
		return fmt.Errorf("stderr says %d times, not once, %q\n%s", n, capsNoMemoryCap, c.m.w.report())
	}
	return nil
}

// recordedOverLimit writes the package whose mutant at line 7 loops for
// ever, has a run record it timed out, and turns that outcome in the
// snapshot's raw JSON into "over-limit" with "limit" "output".
func (c *capsState) recordedOverLimit() error {
	m := c.m
	c.subject = Site{"src/board.go", 7, "1", "0"}
	if err := c.goPackage(capsCounting, capsCountingTest, Tests{Elapsed: 100 * time.Millisecond, TimingOut: []Site{c.subject}}, 5, 6, 7, 8, 10); err != nil {
		return err
	}
	code, stdout, stderr, err := m.w.capture(m.w.dir, m.w.env(), "mutation", "run", "--json", "--timeout-factor", "1", filepath.FromSlash(m.file))
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("the run that records src/board.go exited %d\n%s%s", code, stdout, stderr)
	}
	m.snapshot = ".metrics/mutate/src/board.go.json"
	var snapshot map[string]any
	if err := json.Unmarshal([]byte(m.read(m.snapshot)), &snapshot); err != nil {
		return fmt.Errorf("the run recorded no %s: %v\n%s%s", m.snapshot, err, stdout, stderr)
	}
	turned := 0
	units, _ := snapshot["units"].([]any)
	for _, u := range units {
		unit, _ := u.(map[string]any)
		mutants, _ := unit["mutants"].([]any)
		for _, one := range mutants {
			mu, _ := one.(map[string]any)
			if mu["line"] == float64(c.subject.Line) && mu["original"] == c.subject.Original && mu["replacement"] == c.subject.Replacement {
				if mu["outcome"] != "timeout" {
					return fmt.Errorf("the run recorded the mutant at line 7 %v, not timed out\n%s%s", mu["outcome"], stdout, stderr)
				}
				mu["outcome"], mu["limit"] = "over-limit", "output"
				turned++
			}
		}
	}
	if turned != 1 {
		return fmt.Errorf("the snapshot records %d mutants at line 7, not one:\n%s", turned, m.read(m.snapshot))
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	if err := m.w.write(m.snapshot, string(data)+"\n"); err != nil {
		return err
	}
	return c.committed()
}

// committed commits the scratch folder in a repository of its own, as
// mutation sample seeds its draw with the HEAD commit's id.
func (c *capsState) committed() error {
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"},
		{"commit", "-qm", "fixture"}} {
		if err := c.m.w.git(args...); err != nil {
			return err
		}
	}
	return nil
}

// reusedOverLimit is whether the run reused the mutant over its limit and
// counts it killed.
func (c *capsState) reusedOverLimit() error {
	if err := c.overLimit(c.subject, "output", true); err != nil {
		return err
	}
	f, _ := c.file(c.m.w.stdout)
	if mu, _ := f.mutant(c.subject); !mu.Reused {
		return fmt.Errorf("the mutant over its limit ran again, not reused\n%s", c.m.w.report())
	}
	return nil
}

// checkCountsKilled is whether mutation check counts every mutant of
// Count killed, the one over its limit among them, and exits 0.
func (c *capsState) checkCountsKilled() error {
	w := c.m.w
	if err := w.runEnv(w.dir, w.env(), "mutation", "check", "--json", filepath.FromSlash(c.m.file)); err != nil {
		return err
	}
	var object struct {
		Files []struct {
			Functions []struct {
				Function string `json:"function"`
				Killed   int    `json:"killed"`
				Survived int    `json:"survived"`
			} `json:"functions"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(w.stdout), &object); err != nil {
		return fmt.Errorf("standard output is not one JSON object: %v\n%s", err, w.report())
	}
	if len(object.Files) != 1 || len(object.Files[0].Functions) != 1 {
		return fmt.Errorf("check lists no one function\n%s", w.report())
	}
	if fn := object.Files[0].Functions[0]; fn.Killed != 4 || fn.Survived != 0 {
		return fmt.Errorf("check counts %d killed and %d survived, not every 4 mutants killed\n%s", fn.Killed, fn.Survived, w.report())
	}
	return w.exitsWith(0)
}

// sampleAgrees is whether a sample of every mutant runs the one recorded
// over its limit to its timeout, now, and reports no problem: read with
// --json, and with --timeout-factor 1, as the Given's run, so the timeout
// comes in seconds.
func (c *capsState) sampleAgrees() error {
	w := c.m.w
	if err := w.runEnv(w.dir, w.env(), "mutation", "sample", "--json", "--timeout-factor", "1", "--count", "20", filepath.FromSlash(c.m.file)); err != nil {
		return err
	}
	var object struct {
		Files []struct {
			Mutants []struct {
				Line     int    `json:"line"`
				Recorded string `json:"recorded"`
				Outcome  string `json:"outcome"`
			} `json:"mutants"`
		} `json:"files"`
		Problems []any `json:"problems"`
	}
	if err := json.Unmarshal([]byte(w.stdout), &object); err != nil {
		return fmt.Errorf("standard output is not one JSON object: %v\n%s", err, w.report())
	}
	if len(object.Problems) != 0 {
		return fmt.Errorf("the sample reports problems\n%s", w.report())
	}
	ran := false
	for _, f := range object.Files {
		for _, mu := range f.Mutants {
			if mu.Line == c.subject.Line {
				ran = mu.Recorded == "over-limit" && mu.Outcome == "timeout"
			}
		}
	}
	if !ran {
		return fmt.Errorf("the sample did not run the mutant recorded over its limit to its timeout\n%s", w.report())
	}
	return w.exitsWith(0)
}

// capsHundred are the 100 numbered lines a failing baseline prints, the
// first naming the cause.
func capsHundred() []string {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %03d", i+1)
	}
	lines[0] += " names the cause"
	return lines
}

// capsPrinting is the project's pytest settings: pytest's own report off,
// and the test's output not captured, so the tests print what the test
// prints alone.
const capsPrinting = pyproject + "\n[tool.pytest.ini_options]\naddopts = \"-p no:terminal -s\"\n"

// capsPrintingTest kills one mutant of src/board.py and lets its "> →
// >=" survive, so a run retries that one, after a baseline, unless
// failing.txt is beside it: then it prints that file, and fails. The file
// is no test, so the snapshot's results of src/board.py still hold with it
// there, as mutation sample needs.
const capsPrintingTest = `import os

from board import place

FAILING = os.path.join(os.path.dirname(__file__), "failing.txt")


def test_place():
    if os.path.exists(FAILING):
        with open(FAILING) as printed:
            print(printed.read(), end="")
        assert not place(1, 2)
    assert place(1, 2)
`

// hundredLines writes a Python project whose tests pass, records a run's
// results of src/board.py, a survivor among them, then makes its tests
// print the 100 lines and fail, and commits it all.
func (c *capsState) hundredLines() error {
	m := c.m
	m.file = "src/board.py"
	if err := m.writeProject(map[string][]string{"tests/test_board.py": {m.file}}, map[string]string{"pyproject.toml": capsPrinting, m.file: pyBoard, "tests/test_board.py": capsPrintingTest}); err != nil {
		return err
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "python", Report: "SF:src/board.py\nDA:2,1\nend_of_record\n"}); err != nil {
		return err
	}
	command := "python3 -m pytest -q -x -p no:cacheprovider tests/test_board.py"
	if err := m.w.d.Tests(m.w.dir, Tests{Language: "python", Command: command, Surviving: []Site{{m.file, 2, ">", ">="}}}); err != nil {
		return err
	}
	if err := m.pythonProbe(true); err != nil {
		return err
	}
	code, stdout, stderr, err := m.w.capture(m.w.dir, m.w.env(), "mutation", "run", filepath.FromSlash(m.file))
	if s, ok := summaryOf(stdout, m.file); err != nil || code != 1 || !ok || s.killed != 1 || s.survived != 1 {
		return fmt.Errorf("the run that records %s did not kill one mutant and leave one surviving: exit %d, %v\n%s%s", m.file, code, err, stdout, stderr)
	}
	printed := strings.Join(capsHundred(), "\n") + "\n"
	if err := m.w.write("tests/failing.txt", printed); err != nil {
		return err
	}
	if err := m.w.d.Tests(m.w.dir, Tests{Language: "python", Command: command, Baseline: printed}); err != nil {
		return err
	}
	return c.committed()
}

// capsReport is the head and the tail of the 100 lines, as a report gives
// them.
func capsReport() []string {
	lines := capsHundred()
	return slices.Concat(lines[:20], []string{"… 60 lines left out …"}, lines[80:])
}

// headAndTail is whether the lines of stdout after the one saying said
// are the first 20 of the 100, the line saying 60 were left out, and the
// last 20, then nothing but a summary.
func (c *capsState) headAndTail(stdout, said string) error {
	lines := outputLines(strings.ReplaceAll(stdout, "\r\n", "\n"))
	at := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, said) })
	if at < 0 {
		return fmt.Errorf("stdout does not say %q\n%s", said, stdout)
	}
	follow := lines[at+1:]
	if end := slices.IndexFunc(follow, func(l string) bool { return summaryLine.MatchString(l) }); end >= 0 {
		follow = follow[:end]
	}
	if want := capsReport(); !slices.Equal(follow, want) {
		return fmt.Errorf("the lines that follow are\n%s\nnot\n%s", strings.Join(follow, "\n"), strings.Join(want, "\n"))
	}
	return nil
}

// jsonOutput is whether the problem the problem step found gives the same
// lines as its "output".
func (c *capsState) jsonOutput() error {
	problem := c.m.w.problem
	if problem == nil {
		return errors.New("no problem found to read the output of")
	}
	output, _ := problem["output"].(string)
	if got, want := outputLines(strings.ReplaceAll(output, "\r\n", "\n")), capsReport(); !slices.Equal(got, want) {
		return fmt.Errorf("its \"output\" is\n%s\nnot\n%s", output, strings.Join(want, "\n"))
	}
	return nil
}

// sampleHeadAndTail is whether mutation sample reports the failing
// baseline with the same lines.
func (c *capsState) sampleHeadAndTail() error {
	w := c.m.w
	code, stdout, stderr, err := w.capture(w.dir, w.env(), "mutation", "sample", filepath.FromSlash("src/board.py"))
	if err != nil {
		return err
	}
	if err := c.headAndTail(stdout, "baseline tests of src/board.py fail; no sampled mutant ran"); err != nil {
		return fmt.Errorf("%w\nexit %d\n--- stdout\n%s--- stderr\n%s", err, code, stdout, stderr)
	}
	return nil
}
