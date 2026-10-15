package mutate

import (
	"io"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/internal/lang"
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
			factory := &porttest.Workers{Files: map[string]string{path: "a"}, Tests: map[string]map[string]porttest.Tests{root: {c.String(): {}}}}
			state := &fileState{file: &lang.File{Path: path, Src: []byte("a"), Units: []lang.Unit{{Name: "A"}}}, rel: "a.go", command: c, sites: []Site{{Start: 0, End: 1, Line: 1, Original: "a", Replacement: "b"}}, outcomes: []string{""}, result: &FileResult{}}
			stop, err := execute(t.Context(), []*fileState{state}, Options{WorkerFactory: factory, Workers: 1, Log: io.Discard}, tc.ff)
			if err != nil || stop != nil || state.outcomes[0] != Killed || state.result.Ran != 1 {
				t.Fatalf("actual execution: %+v %v %v ran%d", stop, err, state.outcomes, state.result.Ran)
			}
			if len(factory.Opened) != 1 || factory.Opened[0] != tc.purpose {
				t.Fatalf("actual run requested %v, want %v", factory.Opened, tc.purpose)
			}
		})
	}
}
