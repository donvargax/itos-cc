//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The scenario of "Rule: Counted runs work in a linked git worktree" in
// features/mutate.feature, the counted-worktree-run slice. Counted mode
// refuses Windows, so the step builds only where it runs. It drives the CLI
// and reads its --json object as raw maps, so it compiles against a product
// that refuses a linked worktree.

// worktreeFiles is compareFiles whose test also records, in $WT_DIR/cwd,
// the directory each run of it works in: where the counted run's frozen
// inputs lie.
var worktreeFiles = map[string]string{
	"go.mod":  compareFiles["go.mod"],
	"main.go": compareFiles["main.go"],
	"main_test.go": `package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompare(t *testing.T) {
	if dir := os.Getenv("WT_DIR"); dir != "" {
		cwd, _ := os.Getwd()
		f, err := os.OpenFile(filepath.Join(dir, "cwd"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			f.WriteString(cwd + "\n")
			f.Close()
		}
	}
	if !Compare(6) || Compare(5) {
		t.Fatal("Compare")
	}
}
`,
}

// @ID-MUT-212
func TestACountOneRunInALinkedWorktreeJudgesItsCommittedSelection(t *testing.T) {
	requireCountedPlatform(t)
	// Given a committed Go project and a second checkout of it made with
	// git worktree add, whose branch has a commit of its own.
	main := moduleRepo(t, worktreeFiles)
	linked := filepath.Join(t.TempDir(), "linked")
	gitIn(t, main, "worktree", "add", "-q", "-b", "linked", linked)
	writeFile(t, filepath.Join(linked, "main.go"), strings.Replace(worktreeFiles["main.go"], "func main() {}", "// The linked checkout's own commit.\nfunc main() {}", 1))
	gitIn(t, linked, "commit", "-qam", "linked")
	useDir(t, linked)
	head := gitOut(t, linked, "rev-parse", "HEAD")
	gitDir := gitOut(t, linked, "rev-parse", "--absolute-git-dir")
	physical, err := filepath.EvalSymlinks(gitDir)
	if err != nil {
		t.Fatal(err)
	}
	mainGit, _ := filepath.EvalSymlinks(filepath.Join(main, ".git"))
	if !strings.HasPrefix(physical, mainGit+string(filepath.Separator)) || head == gitOut(t, main, "rev-parse", "HEAD") {
		t.Fatalf("the linked checkout's git directory %s is not under %s, or its HEAD is main's", physical, mainGit)
	}
	mainTree, linkedTree := tree(t, main), tree(t, linked)
	wt := t.TempDir()
	useEnv(t, "WT_DIR", wt)

	// When a count-one run judges it from the linked worktree.
	o := countedRun(t, "--count", "1", "--workers", "1")
	logOutcome(t, &o)
	c := o.counted(t)

	// Then exactly one selected mutant is judged with a real outcome and
	// the report names the worktree's HEAD commit.
	if len(c.Selected) != 1 || c.Selected[0]["state"] != "judged" ||
		(c.Selected[0]["outcome"] != "killed" && c.Selected[0]["outcome"] != "survived" && c.Selected[0]["outcome"] != "timeout") {
		t.Errorf("exit %d, selected %v, problems %v: want one selected mutant judged with a real outcome", o.code, c.Selected, c.Problems)
	}
	if c.Sampling["commit"] != head {
		t.Errorf("sampling commit %v, want the linked checkout's HEAD %s", c.Sampling["commit"], head)
	}
	// And its private scratch directory lies under the repository's git
	// directory for that worktree and is removed afterwards.
	var cwds []string
	if data, err := os.ReadFile(filepath.Join(wt, "cwd")); err == nil {
		cwds = strings.Fields(string(data))
	}
	under := false
	for _, cwd := range cwds {
		for _, dir := range []string{gitDir, physical} {
			under = under || strings.HasPrefix(cwd, filepath.Join(dir, "itos", "fresh-plan-"))
		}
	}
	if !under {
		t.Errorf("the tests ran in %v: want some run in the frozen inputs under %s", cwds, filepath.Join(gitDir, "itos"))
	}
	if left, _ := filepath.Glob(filepath.Join(physical, "itos", "fresh-plan-*")); len(left) > 0 {
		t.Errorf("the frozen inputs %v were left behind", left)
	}
	// And neither checkout's working tree is changed.
	if got := tree(t, main); fmt.Sprint(got) != fmt.Sprint(mainTree) {
		t.Errorf("the main checkout changed:\n%v\nwant\n%v", got, mainTree)
	}
	if got := tree(t, linked); fmt.Sprint(got) != fmt.Sprint(linkedTree) {
		t.Errorf("the linked checkout changed:\n%v\nwant\n%v", got, linkedTree)
	}

	// But a scratch path that escapes both the checkout and its git
	// directory is refused with a problem and a fix, not an internal
	// error: the worktree's scratch directory redirected elsewhere.
	elsewhere := t.TempDir()
	os.RemoveAll(filepath.Join(physical, "itos"))
	if err := os.Symlink(elsewhere, filepath.Join(physical, "itos")); err != nil {
		t.Fatal(err)
	}
	refused := countedRun(t, "--count", "1", "--workers", "1")
	logOutcome(t, &refused)
	r := refused.counted(t)
	p := r.problem("count.preparation-failed")
	if refused.code != 1 || r.problem("internal") != nil || p == nil || p["fix"] == "" || p["fix"] == nil {
		t.Errorf("redirected scratch: exit %d, problems %v: want 1 with count.preparation-failed and a fix, no internal error", refused.code, r.Problems)
	}
	if entries, _ := os.ReadDir(elsewhere); len(entries) > 0 {
		t.Errorf("the run wrote %v where the redirected scratch path leads", entries)
	}
}
