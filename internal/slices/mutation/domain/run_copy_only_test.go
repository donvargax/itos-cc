package domain_test

import (
	"path/filepath"
	"testing"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

// TestABaselineFailingOnlyInTheCopySaysWhichTree holds slice-19 (issue
// #44): a baseline that fails in a worker's copy runs once more in the
// project's own folder, and the file's result names the tree the copy held
// when it passes there, and none when it fails there too.
func TestABaselineFailingOnlyInTheCopySaysWhichTree(t *testing.T) {
	for _, tc := range []struct {
		name     string
		copyOnly bool
	}{{"passes in the project", true}, {"fails in the project too", false}} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "a.go")
			source := "package a\nfunc A(x int) bool { return x > 1 }\n"
			c := mutants.Command{Root: root, Dir: root, Args: []string{"fixture-tests"}}
			factory := &porttest.Workers{Files: map[string]string{path: source},
				Tests: map[string]map[string]porttest.Tests{root: {c.String(): {Baseline: "TSCONFIG_ERROR", CopyOnly: tc.copyOnly}}}}
			state := mutants.NewExecution(parseExecutionMode(t, path, source), "a.go", "a.go", c)
			state.Pending(0, nil)
			if _, err := mutants.Execute(t.Context(), []*mutants.Execution{state}, mutants.Options{WorkerFactory: factory, Workers: 1}, nil); err != nil {
				t.Fatal(err)
			}
			r := state.Result()
			want := ""
			if tc.copyOnly {
				want = filepath.Clean(root)
			}
			if !r.BaselineFailed || r.BaselineOutput.Text != "TSCONFIG_ERROR" || r.BaselineCopy != want {
				t.Fatalf("result: failed %v, output %q, copy %q, want copy %q", r.BaselineFailed, r.BaselineOutput.Text, r.BaselineCopy, want)
			}
		})
	}
}
