package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/cli"
)

// These test main's own seams, which no scenario reaches: the version run
// binds from the build, the itos-cc.yaml the adapters give failing to be
// read, and a command kong runs panicking or asking for what nothing binds.
// What a scenario reaches stays in cli.feature.
//
// They replace the template's tests of the template's main.go (go-template-itos
// v0.2.0, set aside by T-26), whose assertions are held so:
//
//   - its logger's (JSON to a writer, text to a terminal, quiet below a
//     warning) are the template's alone: itos-cc's main makes no logger and
//     asks no terminal, its commands' progress lines going to the stderr they
//     are given, so neither the tests nor golang.org/x/term come back;
//   - wantsJSON's is internal/cli's Read, which TestReadFindsJSONAndHelpBeforeDashDash
//     holds, and @ID-CLI-10 for a failure's object; --no-json and --json=true
//     are no flags of itos-cc's;
//   - commandName's is internal/cli's Dispatch, which its own tests and
//     @ID-CLI-04 and @ID-CLI-10 hold;
//   - the help of a line with no arguments is @ID-CLI-01's first row, and a
//     usage error's line and exit 2 are @ID-CLI-07's;
//   - __complete's and completion's are the template's: itos-cc has no such
//     command;
//   - count's line past a reader's token limit is the sample's, which no
//     itos-cc command runs;
//   - version's stamp is @ID-CLI-03's, from the binary as each build makes
//     it, and the commit its object carried is the template's: itos-cc's
//     prints its version alone.

// version and --version print the version main works out from the build,
// as the line and as the object of that version alone.
func TestRunPrintsTheVersionMainWorksOut(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}} {
		code, stdout, stderr := runFaked(t, fakes(nil), args...)
		if want := "itos-cc " + versionString() + "\n"; code != 0 || stdout != want || stderr != "" {
			t.Errorf("%q: exit %d, stdout %q, stderr %q; want 0, %q and nothing", args, code, stdout, stderr, want)
		}
	}
	code, stdout, _ := runFaked(t, fakes(nil), "version", "--json")
	var object map[string]any
	if err := json.Unmarshal([]byte(stdout), &object); err != nil {
		t.Fatalf("version --json printed %q: %v", stdout, err)
	}
	want := map[string]any{"schema": 1.0, "ok": true, "version": versionString()}
	if code != 0 || len(object) != len(want) {
		t.Errorf("version --json: exit %d, %v; want 0 and %v", code, object, want)
	}
	for key, value := range want {
		if object[key] != value {
			t.Errorf("version --json: %s = %v, want %v", key, object[key], value)
		}
	}
}

// unreadable is an itos-cc.yaml that cannot be read, as a file the user
// cannot open.
type unreadable struct{}

func (unreadable) Read(context.Context) (string, []byte, error) {
	return "/work", nil, errors.New("itos-cc.yaml: permission denied")
}

// crap, dry and units end with the failure of reading itos-cc.yaml, which
// nothing classifies, before measuring anything; their help, scrap and
// version never read it, so it breaks none of them.
func TestRunReportsAConfigThatCannotBeReadOnlyWhereItIsRead(t *testing.T) {
	failing := fakes(nil)
	failing.Config = unreadable{}
	for _, name := range []string{"crap", "dry", "units"} {
		code, stdout, stderr := runFaked(t, failing, name, "--json")
		if code != cli.CodeInternal || !strings.Contains(stdout, `"rule": "internal"`) || !strings.Contains(stdout, "itos-cc.yaml: permission denied") || stderr != "" {
			t.Errorf("%s --json: exit %d, stdout %q, stderr %q; want 70 and the failure as its object", name, code, stdout, stderr)
		}
		code, _, stderr = runFaked(t, failing, name)
		if code != cli.CodeInternal || !strings.Contains(stderr, "itos-cc: itos-cc.yaml: permission denied") {
			t.Errorf("%s: exit %d, stderr %q; want 70 and the failure on stderr", name, code, stderr)
		}
		code, stdout, _ = runFaked(t, failing, name, "--help")
		if code != 0 || !strings.Contains(stdout, "usage: itos-cc "+name) {
			t.Errorf("%s --help: exit %d, stdout %q; want 0 and its help", name, code, stdout)
		}
	}
	for _, args := range [][]string{{"scrap", "--json"}, {"version"}} {
		if code, stdout, stderr := runFaked(t, failing, args...); code != 0 {
			t.Errorf("%q: exit %d, stdout %q, stderr %q; want 0, the file unread", args, code, stdout, stderr)
		}
	}
}

// A command kong runs that panics, or asks for what nothing binds, is a bug:
// an internal problem and exit 70, as the --json object or a line on
// stderr, never a crash.
func TestCallSafelyReportsABugAsAnInternalFailure(t *testing.T) {
	for _, c := range []struct {
		name string
		run  any
		want string
	}{
		{"a panic", func() int { panic("oops") }, "version panicked: oops"},
		{"an unbound value", func(*testing.T) int { return 0 }, "*testing.T"},
	} {
		for _, asJSON := range []bool{true, false} {
			var stdout, stderr strings.Builder
			ui := &cli.UI{In: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			parser, err := parserOf(t.Context(), &commandLine{}, ui, fakes(nil))
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := cli.Parse(parser, []string{"version"})
			if err != nil {
				t.Fatal(err)
			}
			code := callSafely(ui, parsed, "version", c.run, asJSON)
			out := stderr.String()
			if asJSON {
				out = stdout.String()
				if !strings.Contains(out, `"rule": "internal"`) {
					t.Errorf("%s with --json: stdout %q, want the internal problem's object", c.name, out)
				}
			}
			if code != cli.CodeInternal || !strings.Contains(out, c.want) {
				t.Errorf("%s (--json %v): exit %d, output %q; want 70 and %q", c.name, asJSON, code, out, c.want)
			}
		}
	}
}
