package mutation

import (
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/internal/cli"
)

// TestABaselineFailedOnlyInTheCopyNamesTheCopy holds the wording of
// slice-19 (issue #44) for mutation run and mutation sample alike: a
// baseline that failed only in a worker's copy names the tree the copy
// held, as the working directory names it, and one that fails in the
// project too keeps its message.
func TestABaselineFailedOnlyInTheCopyNamesTheCopy(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "project")
	p := &Presentation{UI: &cli.UI{Dir: dir}}
	for _, tc := range []struct {
		copied, done, command, message, fix string
	}{
		{"", "judged", "mutation run", "shapes/src/area.ts: its tests fail before any mutant, so none was judged", "Make its tests pass, then run mutation run again."},
		{"", "run", "mutation sample", "shapes/src/area.ts: its tests fail before any mutant, so none was run", "Make its tests pass, then run mutation sample again."},
		{filepath.Join(dir, "shapes"), "judged", "mutation run", "shapes/src/area.ts: its tests pass in the project but fail in the worker's copy of shapes, so none was judged", "The copy holds shapes alone: its tests read something outside it."},
		{filepath.Join(dir, "shapes"), "run", "mutation sample", "shapes/src/area.ts: its tests pass in the project but fail in the worker's copy of shapes, so none was run", "The copy holds shapes alone: its tests read something outside it."},
		{dir, "judged", "mutation run", "shapes/src/area.ts: its tests pass in the project but fail in the worker's copy of project, so none was judged", "The copy holds project alone: its tests read something outside it."},
	} {
		got := p.baselineFailed("shapes/src/area.ts", tc.copied, nil, tc.done, tc.command)
		if got.Rule != "mutation.baseline-failed" || got.Code != cli.CodeNo || filepath.ToSlash(got.Message) != tc.message || got.Fix != tc.fix || got.Subject("file") != "shapes/src/area.ts" {
			t.Errorf("copied %q: %s %d %q %q", tc.copied, got.Rule, got.Code, got.Message, got.Fix)
		}
	}
}

// TestABaselineFailedOnlyInTheCopyNamesWhatGitIgnores holds slice-43's
// wording: a baseline that failed only in a worker's copy that left out
// what git ignores or other repositories' checkouts names the first five
// paths, then how many more, and
// its fix gives the first as mutation.copy's example, a folder's with /**.
func TestABaselineFailedOnlyInTheCopyNamesWhatGitIgnores(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "leak")
	p := &Presentation{UI: &cli.UI{Dir: dir}}
	for _, tc := range []struct {
		leftOut         []string
		listed, example string
	}{
		{[]string{"gen/"}, "gen/", "gen/**"},
		{[]string{"a.txt", "b/", "c/", "d/", "e/", "f/", "g/"}, "a.txt, b/, c/, d/, e/ and 2 more", "a.txt"},
	} {
		got := p.baselineFailed("leak.go", dir, tc.leftOut, "judged", "mutation run")
		message := "leak.go: its tests pass in the project but fail in the worker's copy of leak, so none was judged; the copy left out what git ignores and other repositories' checkouts: " + tc.listed
		fix := "If its tests read what it left out, name it in mutation.copy in itos-cc.yaml, such as mutation: {copy: [" + tc.example + "]}; else they read something outside leak, which the copy holds alone."
		if got.Rule != "mutation.baseline-failed" || got.Message != message || got.Fix != fix || got.Subject("file") != "leak.go" {
			t.Errorf("left out %q: %s %q %q", tc.leftOut, got.Rule, got.Message, got.Fix)
		}
	}
}
