package mutation

import (
	"encoding/json"
	"testing"

	"github.com/donvargax/itos-cc/internal/cli"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// proofJSON is a summary as the --json object holds it.
func proofJSON(t *testing.T, s *proofSummary) string {
	t.Helper()
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// The scenarios of the summary hold one file and a green run: drawing or
// recording two files, a timeout beside a kill, or a flaky mutant, which a
// green run never holds, costs a scenario far more than these say.
func TestAProofSummaryAddsUpEveryFileOfARun(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		got  *proofSummary
		want string
	}{
		{"counted", countedProof([]countedSite{
			{File: "a.go", State: "judged", Outcome: mutants.Killed},
			{File: "a.go", State: "judged", Outcome: mutants.Timeout},
			{File: "a.go", State: "judged", Outcome: mutants.OverLimit},
			{File: "b.go", State: "judged", Outcome: mutants.Survived},
			{File: "b.go", State: "judged", Outcome: mutants.Survived, Excepted: "x is never 5"},
			{File: "b.go", State: "uncovered", Outcome: mutants.Uncovered},
			{File: "c.go", State: "failed"},
		}, nil), `{"version":1,"judged":6,"unit":"mutant","files":2,"killed":3,"survived":1,"excepted":1,"uncovered":1}`},
		{"complete", runProof([]runFile{
			{File: "a.go", Killed: 3, Survived: 1},
			{File: "b.go"},
			{File: "c.go", Excepted: 2, Uncovered: 4, Flaky: 5},
		}, nil), `{"version":1,"judged":15,"unit":"mutant","files":2,"killed":3,"survived":1,"excepted":2,"uncovered":4}`},
		{"check", checkProof([]checkFile{
			{File: "a.go", Functions: []checkFunction{{Killed: 3, Survived: 1}, {Excepted: 2, Uncovered: 4}}},
			{File: "b.go"},
			{File: "c.go", Functions: []checkFunction{{Killed: 5, Survived: 6, Excepted: 7, Uncovered: 8}}},
		}, nil), `{"version":1,"judged":3,"unit":"function","files":2,"killed":8,"survived":7,"excepted":9,"uncovered":12}`},
	} {
		if got := proofJSON(t, c.got); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

// A site with no outcome, a file with no count and one with no function are
// nothing judged, which reports nothing rather than the zero itos refuses.
func TestAProofSummaryOfNothingJudgedIsAllNull(t *testing.T) {
	t.Parallel()
	const want = `{"version":1,"judged":null,"unit":null,"files":null,"killed":null,"survived":null,"excepted":null,"uncovered":null}`
	for name, got := range map[string]*proofSummary{
		"counted":  countedProof([]countedSite{{File: "a.go", State: "failed"}}, nil),
		"complete": runProof([]runFile{{File: "a.go"}}, nil),
		"check":    checkProof([]checkFile{{File: "a.go"}}, nil),
	} {
		if text := proofJSON(t, got); text != want {
			t.Errorf("%s: %s, want %s", name, text, want)
		}
	}
}

// A failed run's object is as it was, so the summary is nil whatever it judged.
func TestAFailedRunHasNoProofSummary(t *testing.T) {
	t.Parallel()
	failed := []*cli.Problem{cli.NewProblem(cli.CodeNo, "mutation.survived", "a mutant survived", "test it")}
	for name, got := range map[string]*proofSummary{
		"counted":  countedProof([]countedSite{{File: "a.go", Outcome: mutants.Killed}}, failed),
		"complete": runProof([]runFile{{File: "a.go", Killed: 1}}, failed),
		"check":    checkProof([]checkFile{{File: "a.go", Functions: []checkFunction{{Killed: 1}}}}, failed),
	} {
		if got != nil {
			t.Errorf("%s: %s, want none", name, proofJSON(t, got))
		}
	}
}
