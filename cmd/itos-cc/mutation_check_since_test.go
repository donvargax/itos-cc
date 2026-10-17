package main

import (
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

// mutation check's --since admission moved into internal/mutation (T-97),
// worded by its UI, while mutation run's stays in cmd until run moves. No
// scenario runs check into these refusals, so these hold the moved copy to
// run's problems, word for word and with the same subject, and to its exit
// code and plain lines.
func TestCheckSinceRefusalsMatchRun(t *testing.T) {
	t.Parallel()
	boardRepo(t, nil)
	for _, args := range [][]string{{"--since", "nosuch"}, {"--since", "main", "--changed"}} {
		checkMatchesRun(t, args...)
	}
}

func TestCheckSinceOutsideAGitRepositoryMatchesRun(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	useEnv(t, "GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	for name, text := range boardFiles {
		writeFile(t, filepath.Join(dir, name), text)
	}
	useDir(t, dir)
	checkMatchesRun(t, "--since", "main")
}

// checkMatchesRun runs mutation run and mutation check with args, plain and
// with --json, and wants the same refusal from both.
func checkMatchesRun(t *testing.T, args ...string) {
	t.Helper()
	run := mutateRun(t, append([]string{"--json"}, args...)...)
	check := itosCc(t, append([]string{"mutation", "check", "--json"}, args...)...)
	want, got := rawProblems(t, run), rawProblems(t, check)
	if len(want) == 0 || !reflect.DeepEqual(got, want) || check.code != run.code {
		t.Errorf("%q: check exit %d, problems %v; want run's exit %d, problems %v", args, check.code, got, run.code, want)
	}
	run = mutateRun(t, args...)
	check = itosCc(t, append([]string{"mutation", "check"}, args...)...)
	if check.stderr != run.stderr || check.code != run.code {
		t.Errorf("%q plain: check exit %d, stderr:\n%s\nwant run's exit %d, stderr:\n%s", args, check.code, check.stderr, run.code, run.stderr)
	}
}
