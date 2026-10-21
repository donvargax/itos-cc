package mutants_test

import (
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/internal/lang"
	langtest "github.com/donvargax/itos-cc/internal/lang/port/porttest"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

// The legacy owner must request fail-fast semantics before any actionable stop;
// the adapter's external-parent-cancellation policy depends on that purpose.
func TestExecutionBindsWorkerPurposeFromActualRunMode(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ff      *mutants.FailFast
		purpose mutants.WorkPurpose
	}{
		{"ordinary", nil, mutants.MutationRun}, {"fail-fast before any stop", &mutants.FailFast{}, mutants.FailFastJudgments},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "a.go")
			c := mutants.Command{Root: root, Dir: root, Args: []string{"fixture-tests"}}
			factory := &porttest.Workers{Files: map[string]string{path: "package a\nfunc A(x int) bool { return x > 1 }\n"}, Tests: map[string]map[string]porttest.Tests{root: {c.String(): {}}}}
			// The fixture supplies one actual mutation site through the parsed source below.
			state := mutants.NewExecution(parseExecutionMode(t, path, "package a\nfunc A(x int) bool { return x > 1 }\n"), "a.go", "a.go", c)
			state.Pending(0, nil)
			stop, err := mutants.Execute(t.Context(), []*mutants.Execution{state}, mutants.Options{WorkerFactory: factory, Workers: 1}, tc.ff)
			if err != nil || stop != nil || state.Outcome(0) != mutants.Killed || state.Result().Ran != 1 {
				t.Fatalf("actual execution: %+v %v outcome%s ran%d", stop, err, state.Outcome(0), state.Result().Ran)
			}

			if len(factory.Opened) != 1 || factory.Opened[0] != tc.purpose {
				t.Fatalf("actual run requested %v, want %v", factory.Opened, tc.purpose)
			}
		})
	}
}

func parseExecutionMode(t *testing.T, path, source string) *lang.File {
	t.Helper()
	f, err := lang.Parse(t.Context(), langtest.Surroundings{Dir: filepath.Dir(path)}, lang.Detect(path), path, []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.Close)
	return f
}
