package main

import (
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/mutate"
)

// The scenarios of mismatch-fix-scope, at the end of the sample-recorded-scope
// scenarios in features/mutate.feature. They use the module of
// mutate_since_test.go: no test calls Board#clear, so its mutant survives
// with the file's own tests, the whole suite, and a test command alike, and
// a snapshot rewritten to record it killed holds a kill those tests no
// longer make. passing, a test command that always passes, stands for
// 'make test'.

// mismatchFix is the mutation.mismatch a sample of src/board.go with args
// reports for Board#clear's mutant, which must be there with scope; it
// returns the problem's fix.
func mismatchFix(t *testing.T, scope string, args ...string) string {
	t.Helper()
	o := mutationSample(t, append(append([]string{"--json", "--count", "100"}, args...), boardSource)...)
	p := o.json(t).problem("mutation.mismatch")
	wantProblem(t, p, map[string]any{"file": boardSource, "function": clearID, "recorded": "killed", "outcome": "survived"}, o.stdout)
	if p["scope"] != scope {
		t.Errorf("problem scope = %v, want %q\n%s", p["scope"], scope, o.stdout)
	}
	if o.code != 1 {
		t.Errorf("exit %d, want 1", o.code)
	}
	fix, _ := p["fix"].(string)
	return fix
}

// wantFixRuns fails t unless fix says to run command, quoted.
func wantFixRuns(t *testing.T, fix, command string) {
	t.Helper()
	if !strings.Contains(fix, "run '"+command+"'") {
		t.Errorf("fix %q, want it to say to run %q", fix, command)
	}
}

// @ID-MUT-147
func TestAMismatchsFixRerunsTheMutantInTheScopeItWasSampledWith(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		flags []string // what the run recorded with
		scope string
		fix   string // the flags the fix adds
	}{
		{"without flags", nil, "own", ""},
		{"with --all-tests", []string{"--all-tests"}, "all-tests", "--all-tests "},
		{"with --test-command", []string{"--test-command", passing}, passing, "--test-command '" + passing + "' "},
	} {
		t.Run(row.name, func(t *testing.T) {
			boardRepo(t, nil)
			if o := mutateRun(t, append(row.flags, boardSource)...); o.code != 1 {
				t.Fatalf("the run: exit %d, want 1 for clear's survivor\n%s%s", o.code, o.stdout, o.stderr)
			}
			recordAs(t, boardSource, clearID, "<", mutate.Killed)

			fix := mismatchFix(t, row.scope)
			wantFixRuns(t, fix, "itos-cc mutation run --mutate-all "+row.fix+boardSource)
		})
	}
}

// @ID-MUT-148
func TestAScopeGivenToSampleIsTheOneTheFixNames(t *testing.T) {
	t.Parallel()
	boardRepo(t, nil)
	if o := mutateRun(t, boardSource); o.code != 1 {
		t.Fatalf("the run: exit %d, want 1 for clear's survivor\n%s%s", o.code, o.stdout, o.stderr)
	}
	recordAs(t, boardSource, clearID, "<", mutate.Killed)

	fix := mismatchFix(t, "all-tests", "--all-tests")
	wantFixRuns(t, fix, "itos-cc mutation run --mutate-all --all-tests "+boardSource)
}
