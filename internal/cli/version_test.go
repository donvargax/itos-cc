package cli

import (
	"strings"
	"testing"
)

// runVersion runs the version command, --json or not, over a build of that
// version and commit whose text is text, holds it to exit 0, and returns
// what it wrote on stdout and stderr.
func runVersion(t *testing.T, asJSON bool, text, version, commit string) (stdout, stderr string) {
	t.Helper()
	var out, errs strings.Builder
	ui := &UI{In: strings.NewReader(""), Stdout: &out, Stderr: &errs}
	if code := (Version{JSON: asJSON}).Run(ui, text, version, commit); code != 0 {
		t.Fatalf("Run exit = %d, want 0", code)
	}
	return out.String(), errs.String()
}

// Without --json, version prints the text --version prints, whole, and
// nothing on stderr.
func TestVersionPrintsTheText(t *testing.T) {
	text := "itos-cc 1.4.0\ncommit 0123abc"
	stdout, stderr := runVersion(t, false, text, "1.4.0", "0123abc")
	if want := text + "\n"; stdout != want {
		t.Errorf("standard output = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Errorf("error output = %q, want none", stderr)
	}
}

// With --json, one object on one line: schema 1, ok, the version and the
// commit, and nothing on stderr.
func TestVersionJSONIsOneObjectWithTheVersionAndCommit(t *testing.T) {
	stdout, stderr := runVersion(t, true, "itos-cc 1.4.0\ncommit 0123abc", "1.4.0", "0123abc")
	want := `{"schema":1,"ok":true,"version":"1.4.0","commit":"0123abc"}` + "\n"
	if stdout != want {
		t.Errorf("standard output = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Errorf("error output = %q, want none", stderr)
	}
}

// A build that knows no commit leaves the key out, as its text has no second
// line: never an empty commit a script would read as one.
func TestVersionJSONLeavesTheCommitOutWhenTheBuildKnowsNone(t *testing.T) {
	stdout, _ := runVersion(t, true, "itos-cc (devel)", "(devel)", "")
	if want := `{"schema":1,"ok":true,"version":"(devel)"}` + "\n"; stdout != want {
		t.Errorf("standard output = %q, want %q", stdout, want)
	}
}

// version's help gives the shape of its object and its exit codes, the
// contract a script reads.
func TestVersionHelpNamesItsJSONShapeAndExitCodes(t *testing.T) {
	help := Version{}.Help()
	for _, want := range []string{
		"--json prints one object: schema 1, ok true, version, and commit when the build knows it.",
		"0  success",
		"2  the command line is wrong",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("the help does not say %q\n%s", want, help)
		}
	}
}
