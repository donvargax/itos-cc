package mutate

import (
	"io"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/internal/mutants/porttest"
)

// The legacy owner must request fail-fast semantics before any actionable stop;
// the adapter's external-parent-cancellation policy depends on that purpose.
func TestExecutionBindsWorkerPurposeFromActualRunMode(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ff      *FailFast
		purpose mutants.WorkPurpose
	}{
		{"ordinary", nil, mutants.MutationRun}, {"fail-fast before any stop", &FailFast{}, mutants.FailFastJudgments},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "a.go")
			c := Command{Root: root, Dir: root, Args: []string{"fixture-tests"}}
			factory := &porttest.Workers{Files: map[string]string{path: "package a\nfunc A(x int) bool { return x > 1 }\n"}, Tests: map[string]map[string]porttest.Tests{root: {c.String(): {}}}}
			// The fixture supplies one actual mutation site through the parsed source below.
			state := mutants.NewExecution(parse(t, path, "package a\nfunc A(x int) bool { return x > 1 }\n"), "a.go", "a.go", c)
			state.Pending(0, nil)
			stop, err := mutants.Execute(t.Context(), []*mutants.Execution{state}, domainOptions(Options{WorkerFactory: factory, Workers: 1, Log: io.Discard}), tc.ff)
			if err != nil || stop != nil || state.Outcome(0) != Killed || state.Result().Ran != 1 {
				t.Fatalf("actual execution: %+v %v outcome%s ran%d", stop, err, state.Outcome(0), state.Result().Ran)
			}

			if len(factory.Opened) != 1 || factory.Opened[0] != tc.purpose {
				t.Fatalf("actual run requested %v, want %v", factory.Opened, tc.purpose)
			}
		})
	}
}
