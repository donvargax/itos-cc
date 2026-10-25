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
