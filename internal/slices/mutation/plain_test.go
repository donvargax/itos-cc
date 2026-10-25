package mutation

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/cli"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// setupGoMatcher is setup-go's Go problem matcher, the "regexp" of its
// matchers.json at the actions/setup-go version .github/workflows/cd.yml
// pins (b7ad1dad31e06c5925ef5d2fc7ad053ef454303e, v7.0.0), which
// features/mutate_plain_matcher.go holds MUT-343 to.
var setupGoMatcher = regexp.MustCompile(`^\s*(.+\.go):(?:(\d+):(\d+):)? (.*)`)

// pathLineColumn is a Go file's path:line:column anywhere in a line.
var pathLineColumn = regexp.MustCompile(`\.go:\d+:\d+`)

// plainLines is what p printed on stdout, each line of which it checks no
// CI problem matcher reads as an error: none starts with a path or holds
// path:line:column (slice-51).
func plainLines(t *testing.T, stdout *bytes.Buffer) []string {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	for _, line := range lines {
		if setupGoMatcher.MatchString(line) || pathLineColumn.MatchString(line) {
			t.Errorf("a CI problem matcher reads %q as a Go compiler error", line)
		}
	}
	return lines
}

// No scenario the in-process driver runs reaches these plain lines: its
// --fail-fast scenarios read --json, the plain one (MUT-197) and the
// interrupts are the binary driver's alone, and no scenario samples a file
// whose listed tests fail without a mutant. MUT-343 holds the lines every
// passing run prints through both drivers.

func TestAStopNamesItsSiteAfterTheChange(t *testing.T) {
	t.Parallel()
	p := &Presentation{}
	site := &mutants.Stop{Rule: "mutation.survived", File: "src/board.go", Function: "board#place",
		Site: &mutants.Site{Line: 5, Column: 9, Original: ">", Replacement: ">="}}
	if got, want := p.describeStop(site), "`>` → `>=` in board#place, src/board.go line 5 column 9"; got != want {
		t.Errorf("a survivor's stop is described as %q, want %q", got, want)
	}
	stale := &mutants.StaleException{Exception: mutants.Exception{Function: "board#clear", Column: 3}, Why: mutants.ExceptionChanged, Line: 7}
	exception := &mutants.Stop{Rule: "mutation.exception-stale", File: "src/board.go", Exception: stale}
	if got, want := p.describeStop(exception), "in board#clear, src/board.go line 7 column 3 (changed)"; got != want {
		t.Errorf("a stale exception's stop is described as %q, want %q", got, want)
	}
}

func TestAFileAStopCutShortGivesItsCountsBeforeItsName(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	p := &Presentation{UI: &cli.UI{Stdout: &stdout, Stderr: &bytes.Buffer{}}}
	survivor := mutants.MutantResult{Site: mutants.Site{Line: 5, Column: 9, Original: ">", Replacement: ">="},
		Function: "board#place", Outcome: mutants.Survived, State: mutants.StateCompleted}
	cancelled := mutants.MutantResult{Site: mutants.Site{Line: 9, Column: 3, Original: "0", Replacement: "1"},
		Function: "board#clear", State: mutants.StateCancelled}
	found := RunFound{Results: []mutants.FileResult{{Rel: "src/board.go", Incomplete: true, Ran: 1,
		Mutants: []mutants.MutantResult{survivor, cancelled}}}}
	result := runResult{Work: &runWork{}}
	Run{}.present(t.Context(), p, found, &result, true)
	want := []string{
		"stopped early, snapshot not updated: 0 killed, 1 survived, 0 uncovered in src/board.go (ran 1, reused 0); 1 cancelled, 0 unattempted; cache incomplete",
		"  survived `>` → `>=` in board#place, src/board.go line 5 column 9",
	}
	if got := plainLines(t, &stdout); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("plain output\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestASampledFileWhoseListedTestsFailNamesItAfterTheVerdict(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	p := &Presentation{UI: &cli.UI{Stdout: &stdout, Stderr: &bytes.Buffer{}}}
	failed := mutants.FailedSelection{IDs: []string{"ID-A-02"}, Command: "go test -run ID-A-02 ./e2e", ExitCode: 1, Output: "FAIL"}
	result := sampleResult{Files: []sampleFile{}}
	p.sampledFile(mutants.SampledFile{Rel: "src/board.go", FailedSelections: []mutants.FailedSelection{failed}}, &result, map[string]bool{})
	lines := plainLines(t, &stdout)
	if want := "listed tests the sampled mutants of src/board.go run fail without any mutant; no sampled mutant is judged"; lines[0] != want {
		t.Errorf("plain output's first line %q, want %q", lines[0], want)
	}
	if len(result.Files) != 1 || result.Files[0].Baseline != "failed" || len(p.Problems) != 1 || p.Problems[0].Rule != "tests.selection-failed" {
		t.Errorf("files %+v, problems %v: want src/board.go's baseline failed and tests.selection-failed", result.Files, p.Problems)
	}
}

func TestARunsFlakyMutantCountsApartAndIsListedAfterTheSurvivors(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	p := &Presentation{UI: &cli.UI{Stdout: &stdout, Stderr: &bytes.Buffer{}}}
	flaky := mutants.Mutant{Line: 5, Column: 9, Original: ">", Replacement: ">=", Outcome: mutants.Survived, Flaky: true,
		Outcomes: []string{mutants.Survived, mutants.Killed}}
	survivor := mutants.Mutant{Line: 6, Column: 3, Original: "<", Replacement: "<=", Outcome: mutants.Survived}
	found := RunFound{Results: []mutants.FileResult{{Rel: "src/board.go", Ran: 3, Snapshot: mutants.Snapshot{Units: []mutants.UnitResult{
		{Namespace: "board", Name: "place", Survived: 1, Mutants: []mutants.Mutant{flaky, survivor}}}},
		Mutants: []mutants.MutantResult{{Site: mutants.Site{Line: 5, Column: 9, Original: ">", Replacement: ">="}, Function: "board#place",
			Outcome: mutants.Survived, Outcomes: flaky.Outcomes}}}}}
	result := runResult{}
	Run{}.present(t.Context(), p, found, &result, false)
	wantLines(t, "plain output", plainLines(t, &stdout),
		"0 killed, 1 survived, 0 uncovered, 1 flaky in src/board.go (ran 3, reused 0)",
		"  survived `<` → `<=` in board#place, src/board.go line 6 column 3",
		"  flaky `>` → `>=` in board#place, src/board.go line 5 column 9: survived, killed",
	)
	if f := result.Files[0]; f.Flaky != 1 || f.Survived != 1 || !f.Mutants[0].Flaky || len(f.Mutants[0].Outcomes) != 2 {
		t.Errorf("--json file %+v, want 1 flaky beside 1 survived and the mutant flaky with its outcomes", f)
	}
	if len(p.Problems) != 2 || p.Problems[0].Rule != "mutation.survived" || p.Problems[1].Rule != "mutation.flaky" {
		t.Fatalf("problems %v, want mutation.survived, then mutation.flaky", p.Problems)
	}
	problem := p.Problems[1]
	for key, want := range map[string]any{"file": "src/board.go", "line": 5, "column": 9, "function": "board#place", "original": ">",
		"replacement": ">=", "recorded": mutants.Survived, "outcome": mutants.Survived, "scope": mutants.ScopeOwn} {
		if got := problem.Subject(key); got != want {
			t.Errorf("mutation.flaky's %s is %v, want %v", key, got, want)
		}
	}
	if problem.Fix != flakyFix {
		t.Errorf("mutation.flaky's fix %q, want sample's", problem.Fix)
	}
}

func TestAFlakyMutantAStopListsCountsApart(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	p := &Presentation{UI: &cli.UI{Stdout: &stdout, Stderr: &bytes.Buffer{}}}
	flaky := mutants.MutantResult{Site: mutants.Site{Line: 5, Column: 9, Original: ">", Replacement: ">="},
		Function: "board#place", Outcome: mutants.Killed, Outcomes: []string{mutants.Killed, mutants.Survived}, State: mutants.StateCompleted}
	found := RunFound{Results: []mutants.FileResult{{Rel: "src/board.go", Incomplete: true, Ran: 2, Mutants: []mutants.MutantResult{flaky}}}}
	result := runResult{Work: &runWork{}}
	Run{}.present(t.Context(), p, found, &result, true)
	wantLines(t, "plain output", plainLines(t, &stdout),
		"stopped early, snapshot not updated: 0 killed, 0 survived, 0 uncovered, 1 flaky in src/board.go (ran 2, reused 0); 0 cancelled, 0 unattempted; cache incomplete",
		"  flaky `>` → `>=` in board#place, src/board.go line 5 column 9: killed, survived",
	)
	if len(p.Problems) != 1 || p.Problems[0].Rule != "mutation.flaky" {
		t.Errorf("problems %v, want mutation.flaky", p.Problems)
	}
}

// MUT-351 reads a counted run's --json alone, so its flaky site's plain line
// is held here.
func TestACountedFlakySiteIsListedWithItsOutcomes(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	p := &Presentation{UI: &cli.UI{Stdout: &stdout, Stderr: &bytes.Buffer{}}}
	p.printCounted(&countedResult{Sampling: countedSampling{Commit: "abc", Seed: "s", Algorithm: "a", Budget: 1, Eligible: 1, Selected: 1, Executed: 1},
		Selected: []countedSite{{File: "board/board.go", Line: 5, Column: 11, Function: "board#Count", Original: "<", Replacement: "<=",
			State: "judged", Outcome: mutants.Killed, Outcomes: []string{mutants.Killed, mutants.Survived}, Flaky: true},
			{File: "board/board.go", Line: 6, Column: 3, Function: "board#Count", Original: "0", Replacement: "1",
				State: "judged", Outcome: mutants.Killed, Outcomes: []string{mutants.Killed, mutants.Timeout}}}})
	lines := plainLines(t, &stdout)
	if len(lines) < 4 || lines[2] != "  flaky `<` → `<=` in board#Count, board/board.go line 5 column 11: killed, survived" ||
		lines[3] != "  killed `0` → `1` in board#Count, board/board.go line 6 column 3" {
		t.Errorf("a counted run's plain output\n%s\nwant its flaky site listed with its outcomes, and the other by its outcome", stdout.String())
	}
}
