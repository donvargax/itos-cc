package mutation

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/cli"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// A plain line names its file slash-separated from the working directory
// on every system (bug-9): Windows names src\board.go so, and each line
// reads src/board.go. The presentations here are given Windows' separator
// as a value, so Linux holds them to it too.

// windows is a presentation of files Windows named, writing plain lines
// to stdout and stderr.
func windows(stdout, stderr *bytes.Buffer) *Presentation {
	return &Presentation{UI: &cli.UI{Stdout: stdout, Stderr: stderr}, Separator: '\\'}
}

// wantLines checks that got holds each of want, in order, as whole lines.
func wantLines(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("%s\n%s\nwant\n%s", what, strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestSlashedNamesAPathWithSlashes(t *testing.T) {
	t.Parallel()
	if got := Slashed(`src\game\board.py`, '\\'); got != "src/game/board.py" {
		t.Errorf("Windows' src\\game\\board.py is %q, want src/game/board.py", got)
	}
	if got := Slashed("src/game/board.py", '/'); got != "src/game/board.py" {
		t.Errorf("src/game/board.py is %q, want it unchanged", got)
	}
}

func TestARunsPlainLinesNameAWindowsFileWithSlashes(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	p := windows(&stdout, &bytes.Buffer{})
	survivor := mutants.Mutant{Line: 5, Column: 9, Original: ">", Replacement: ">=", Outcome: mutants.Survived}
	failed := mutants.FailedSelection{IDs: []string{"ID-A-02"}, Command: "go test -run ID-A-02 ./e2e", ExitCode: 1, Output: "FAIL"}
	moved := mutants.StaleException{Exception: mutants.Exception{Function: "board#clear", Original: "0", Replacement: "1", Column: 3},
		Why: mutants.ExceptionMoved, NewFile: `src\game\board.go`}
	found := RunFound{Results: []mutants.FileResult{
		{Rel: `src\failing.py`, BaselineFailed: true, BaselineOutput: mutants.Output{Text: "FAILED\n"}},
		{Rel: `src\listed.go`, FailedSelections: []mutants.FailedSelection{failed}},
		{Rel: `src\board.go`, Ran: 1, Snapshot: mutants.Snapshot{Units: []mutants.UnitResult{
			{Namespace: "board", Name: "place", Survived: 1, Mutants: []mutants.Mutant{survivor}}}},
			StaleExceptions: []mutants.StaleException{moved}},
	}}
	result := runResult{}
	Run{}.present(t.Context(), p, found, &result, false)
	wantLines(t, "plain output", plainLines(t, &stdout),
		"baseline tests of src/failing.py fail; snapshot not updated",
		"FAILED",
		"listed tests the mutants of src/listed.go run fail without any mutant; snapshot not updated",
		"listed tests ID-A-02 fail without any mutant",
		"FAIL",
		"0 killed, 1 survived, 0 uncovered in src/board.go (ran 1, reused 0)",
		"  survived `>` → `>=` in board#place, src/board.go line 5 column 9",
		"  exception-stale `0` → `1` in board#clear, src/board.go (moved to src/game/board.go)",
	)
	if got := result.Files[0].File; got != `src\failing.py` {
		t.Errorf("--json names the file %q, want it as the system names it, src\\failing.py", got)
	}
}

func TestAStoppedRunsPlainLinesNameAWindowsFileWithSlashes(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	p := windows(&stdout, &bytes.Buffer{})
	survivor := mutants.MutantResult{Site: mutants.Site{Line: 5, Column: 9, Original: ">", Replacement: ">="},
		Function: "board#place", Outcome: mutants.Survived, State: mutants.StateCompleted}
	blocked := mutants.MutantResult{Site: mutants.Site{Line: 2, Column: 1, Original: "+", Replacement: "-"},
		Function: "board#size", State: mutants.StateBlocked}
	found := RunFound{Results: []mutants.FileResult{
		{Rel: `src\failing.py`, BaselineFailed: true, BaselineOutput: mutants.Output{Text: "FAILED\n"}, Mutants: []mutants.MutantResult{blocked}},
		{Rel: `src\board.go`, Incomplete: true, Ran: 1, Mutants: []mutants.MutantResult{survivor}},
	}}
	result := runResult{Work: &runWork{}}
	Run{}.present(t.Context(), p, found, &result, true)
	wantLines(t, "plain output", plainLines(t, &stdout),
		"blocked, snapshot not updated: the tests of src/failing.py, or the listed tests its mutants run, fail without any mutant (ran 0, reused 0; 0 completed, 0 cancelled, 0 unattempted, 1 blocked); cache incomplete",
		"FAILED",
		"stopped early, snapshot not updated: 0 killed, 1 survived, 0 uncovered in src/board.go (ran 1, reused 0); 0 cancelled, 0 unattempted; cache incomplete",
		"  survived `>` → `>=` in board#place, src/board.go line 5 column 9",
	)
}

func TestAStopNamesAWindowsFileWithSlashes(t *testing.T) {
	t.Parallel()
	p := windows(&bytes.Buffer{}, &bytes.Buffer{})
	file := `src\board.go`
	stale := func(line int) *mutants.StaleException {
		return &mutants.StaleException{Exception: mutants.Exception{Function: "board#clear", Column: 3}, Why: mutants.ExceptionChanged, Line: line}
	}
	for _, tc := range []struct {
		stop *mutants.Stop
		want string
	}{
		{&mutants.Stop{File: file, Function: "board#place", Site: &mutants.Site{Line: 5, Column: 9, Original: ">", Replacement: ">="}},
			"`>` → `>=` in board#place, src/board.go line 5 column 9"},
		{&mutants.Stop{File: file, Exception: stale(7)}, "in board#clear, src/board.go line 7 column 3 (changed)"},
		{&mutants.Stop{File: file, Exception: stale(0)}, "src/board.go in board#clear (changed)"},
		{&mutants.Stop{File: file, Function: "board#place", Line: 4}, "src/board.go:4 in board#place"},
		{&mutants.Stop{File: file}, "in src/board.go"},
	} {
		if got := p.describeStop(tc.stop); got != tc.want {
			t.Errorf("stop %+v is described as %q, want %q", tc.stop, got, tc.want)
		}
	}
}

func TestAStoppedRunSaysWhatStoppedItNamingAWindowsFileWithSlashes(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	p := windows(&stdout, &bytes.Buffer{})
	result := runResult{Work: &runWork{Completed: 1, Cancelled: 2}}
	p.stopped(nil, &result)
	if result.Stop != nil || stdout.Len() != 0 {
		t.Fatalf("a run nothing stopped records %+v and says %q, want nothing", result.Stop, stdout.String())
	}
	stop := &mutants.Stop{Rule: "mutation.survived", File: `src\board.go`, Function: "board#place",
		Site: &mutants.Site{Line: 5, Column: 9, Original: ">", Replacement: ">="}}
	p.stopped(stop, &result)
	wantLines(t, "plain output", plainLines(t, &stdout),
		"stopped early at the first actionable failure, mutation.survived `>` → `>=` in board#place, src/board.go line 5 column 9: "+
			"1 completed, 2 cancelled, 0 unattempted, 0 blocked; a file it cut short keeps its source as it was, and its snapshot only what was judged")
	if result.Stop == nil || result.Stop.Rule != "mutation.survived" || result.Stop.Subject["file"] != `src\board.go` {
		t.Errorf("the stop recorded is %+v, want mutation.survived's, its file as the system names it", result.Stop)
	}
}

func TestACheckAndASamplesPlainLinesNameAWindowsFileWithSlashes(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	p := windows(&stdout, &bytes.Buffer{})
	rel := `src\board.go`
	p.FileChecked(rel, map[string]int{mutants.SnapshotFresh: 2}, nil)
	p.Function(rel, mutants.FunctionCheck{Function: "board#reset", StartLine: 14}, "mutation.missing", "has no mutation results")
	p.LeftOut(rel, mutants.FunctionCheck{Function: "board#clear", StartLine: 9, LeftOut: 1})
	p.CoverageProblem("go", rel, "board#place", 12, "mutation.uncovered-statement", "has an uncovered statement")
	p.Orphans([]Orphan{{File: `src\gone.go`, Snapshot: "src/gone.go.json", Why: "gone"}}, "is an orphan, which mutation run removes")
	result := sampleResult{Files: []sampleFile{}}
	mismatch := mutants.SampledMutant{Site: mutants.Site{Line: 5, Column: 9, Original: ">", Replacement: ">="},
		Function: "board#place", Recorded: mutants.Killed, Outcome: mutants.Survived, Outcomes: slices.Repeat([]string{mutants.Survived}, mutants.Reruns+1)}
	flaky := mutants.SampledMutant{Site: mutants.Site{Line: 9, Column: 3, Original: "0", Replacement: "1"},
		Function: "board#clear", Recorded: mutants.Killed, Outcome: mutants.Survived, Outcomes: []string{mutants.Survived, mutants.Killed}}
	p.sampledFile(mutants.SampledFile{Rel: rel, Mutants: []mutants.SampledMutant{mismatch, flaky}}, &result, map[string]bool{})
	p.sampledFile(mutants.SampledFile{Rel: `src\failing.py`, BaselineFailed: true, BaselineOutput: mutants.Output{Text: "FAILED\n"}}, &result, map[string]bool{})
	failed := mutants.FailedSelection{IDs: []string{"ID-A-02"}, Command: "go test -run ID-A-02 ./e2e", ExitCode: 1, Output: "FAIL"}
	p.sampledFile(mutants.SampledFile{Rel: `src\listed.go`, FailedSelections: []mutants.FailedSelection{failed}}, &result, map[string]bool{})
	wantLines(t, "plain output", plainLines(t, &stdout),
		"2 fresh, 0 stale, 0 missing in src/board.go",
		"  missing src/board.go:14 in board#reset",
		"  left out src/board.go:9 in board#clear: 1 recorded mutant at sites gone, which the next mutation run drops",
		"  uncovered-statement src/board.go:12 in board#place",
		"snapshot src/gone.go.json is an orphan, which mutation run removes: its source src/gone.go is gone",
		"2 sampled, 1 mismatched, 1 flaky in src/board.go",
		"  mismatch `>` → `>=` in board#place, src/board.go line 5 column 9: recorded killed, now survived",
		"  flaky `0` → `1` in board#clear, src/board.go line 9 column 3: recorded killed, now survived, killed",
		"baseline tests of src/failing.py fail; no sampled mutant ran",
		"FAILED",
		"listed tests the sampled mutants of src/listed.go run fail without any mutant; no sampled mutant is judged",
		"listed tests ID-A-02 fail without any mutant",
		"FAIL",
	)
}

func TestANotBuiltFileAndACountedSiteNameAWindowsFileWithSlashes(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	p := windows(&stdout, &stderr)
	notBuilt := []mutants.NotBuilt{{Path: `src\probe_windows.go`, Platform: mutants.Platform{OS: "linux", Arch: "amd64"}}}
	listed := p.sayNotBuilt(notBuilt)
	if len(listed) != 1 || listed[0].File != `src\probe_windows.go` {
		t.Errorf("--json lists %+v, want the file as the system names it, src\\probe_windows.go", listed)
	}
	(&countedProgress{Progress: Progress{Log: &stderr}, p: p}).NotBuilt(notBuilt)
	wantLines(t, "stderr", strings.Split(strings.TrimSuffix(stderr.String(), "\n"), "\n"),
		"itos-cc: not built on linux/amd64, so src/probe_windows.go is not judged here",
		"itos-cc: not built on linux/amd64, so src/probe_windows.go is not judged here",
	)
	p.printCounted(&countedResult{Sampling: countedSampling{Commit: "abc", Seed: "s", Algorithm: "a", Budget: 1, Eligible: 1, Selected: 1, Executed: 1},
		Selected: []countedSite{{File: `src\board.go`, Line: 5, Column: 9, Function: "board#place", Original: ">", Replacement: ">=",
			State: "judged", Outcome: mutants.Survived}}})
	if lines := plainLines(t, &stdout); len(lines) < 3 || lines[2] != "  survived `>` → `>=` in board#place, src/board.go line 5 column 9" {
		t.Errorf("a counted run's plain output\n%s\nwant its site named src/board.go line 5 column 9", stdout.String())
	}
}
