package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

var testCommand = &command{
	name: "probe",
	flags: []flagSpec{
		sw("changed", ""),
		opt("top", intFlag, "N", "", ""),
		opt("threshold", floatFlag, "N", "", ""),
		{name: "report", typ: stringFlag, arg: "FILE", repeat: true},
	},
}

func TestFlagsAreReadByTheirSpec(t *testing.T) {
	in, err := parse(testCommand, []string{"a", "--top", "3", "--threshold=2.5", "b", "--report", "x", "--report=y", "--changed", "--", "--top"})
	if err != nil {
		t.Fatal(err)
	}
	if in.integer("top") != 3 || in.float("threshold") != 2.5 || !in.set("changed") ||
		!slices.Equal(in.strs("report"), []string{"x", "y"}) || !slices.Equal(in.args, []string{"a", "b", "--top"}) {
		t.Errorf("values %v args %q", in.values, in.args)
	}
}

func TestBadFlagsAreUsageErrors(t *testing.T) {
	for args, rule := range map[string]string{
		"--bogus":         "flags.unknown",
		"-changed":        "flags.unknown",
		"--top":           "flags.value-missing",
		"--top --changed": "flags.value-missing",
		"--top=x":         "flags.value-invalid",
		"--changed=yes":   "flags.switch-value",
		"--top 1 --top 2": "flags.repeated",
		"--threshold 1e":  "flags.value-invalid",
	} {
		_, err := parse(testCommand, strings.Fields(args))
		p, ok := err.(*problem)
		if !ok || p.rule != rule || p.kind != kindUsage {
			t.Errorf("%s: %v, want %s, a usage error", args, err, rule)
		}
	}
	_, err := parse(testCommand, []string{"-changed"})
	if p := err.(*problem); !strings.Contains(p.fix, "Did you mean --changed?") {
		t.Errorf("fix %q, want the flag that was meant", p.fix)
	}
}

func TestHelpAnywhereButAfterDashDash(t *testing.T) {
	if _, err := parse(testCommand, []string{"a", "--top", "1", "-h"}); err != errHelp {
		t.Errorf("-h after other flags: %v, want help", err)
	}
	if _, err := parse(testCommand, []string{"--", "-h"}); err == errHelp {
		t.Error("-h after -- is an argument, not help")
	}
}

// stdout runs fn and returns what it printed on stdout. The pipe is read
// while fn writes, as a Windows pipe holds less than a long help.
func stdout(t *testing.T, fn func()) string {
	t.Helper()
	out, _ := captured(t, fn)
	return out
}

func TestExitCodeComesFromTheMostTellingKind(t *testing.T) {
	no := fail(kindNo, "x.no", "", "")
	missing := fail(kindMissing, "x.missing", "", "")
	usage := fail(kindUsage, "x.usage", "", "")
	for _, c := range []struct {
		problems []*problem
		want     int
	}{
		{nil, 0},
		{[]*problem{no}, 1},
		{[]*problem{no, missing}, 3},
		{[]*problem{missing, usage, no}, 2},
	} {
		var code int
		stdout(t, func() { code = emit(true, nil, c.problems) })
		if code != c.want {
			t.Errorf("%d problems: exit %d, want %d", len(c.problems), code, c.want)
		}
	}
}

func TestJSONIsOneObjectWithSchemaOkAndProblems(t *testing.T) {
	out := stdout(t, func() {
		emit(true, struct {
			Files []string `json:"files"`
		}{[]string{"a.go"}}, []*problem{fail(kindNo, "mutation.survived", "it survived", "Add a test.").with("line", 3)})
	})
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v in %q", err, out)
	}
	problems, _ := got["problems"].([]any)
	if got["schema"] != 1.0 || got["ok"] != false || len(problems) != 1 || got["files"] == nil {
		t.Fatalf("object %v", got)
	}
	if p := problems[0].(map[string]any); p["rule"] != "mutation.survived" || p["line"] != 3.0 || p["fix"] != "Add a test." {
		t.Errorf("problem %v, want its rule, sentences, and subject keys", p)
	}
}

func TestDispatch(t *testing.T) {
	for _, c := range []struct {
		args []string
		code int
		want string
	}{
		{nil, 0, "Commands:"},
		{[]string{"--help"}, 0, "Commands:"},
		{[]string{"help", "crap"}, 0, "usage: itos-cc crap"},
		{[]string{"crap", "--help"}, 0, "Exit codes:"},
		{[]string{"help", "nosuch"}, 2, ""},
		{[]string{"--json", "crapp"}, 2, `"Did you mean 'crap'?`},
		{[]string{"--json", "version"}, 0, `"version"`},
		{[]string{"version", "extra"}, 2, ""},
		{nil, 0, "mutation run "},
		{[]string{"mutation", "-h"}, 0, "Subcommands:"},
		{[]string{"mutation", "--help"}, 0, "Subcommands:"},
		{[]string{"help", "mutation"}, 0, "Subcommands:"},
		{[]string{"mutation", "run", "--help"}, 0, "usage: itos-cc mutation run"},
		{[]string{"mutation", "list", "-h"}, 0, "usage: itos-cc mutation list"},
		{[]string{"help", "mutation", "run"}, 0, "usage: itos-cc mutation run"},
		{[]string{"mutation", "check", "--help"}, 0, "usage: itos-cc mutation check"},
		{[]string{"help", "mutation", "check"}, 0, "usage: itos-cc mutation check"},
		{nil, 0, "mutation check "},
		{[]string{"mutation"}, 0, "  check "},
		{[]string{"mutation", "sample", "--help"}, 0, "usage: itos-cc mutation sample"},
		{[]string{"help", "mutation", "sample"}, 0, "usage: itos-cc mutation sample"},
		{nil, 0, "mutation sample "},
		{[]string{"mutation"}, 0, "  sample "},
		{[]string{"--json", "mutation", "sampel"}, 2, `"Did you mean 'mutation sample'?`},
		{[]string{"mutation", "except", "--help"}, 0, "usage: itos-cc mutation except"},
		{[]string{"help", "mutation", "except"}, 0, "usage: itos-cc mutation except"},
		{nil, 0, "mutation except "},
		{[]string{"mutation"}, 0, "  except "},
		{[]string{"--json", "mutation", "excpet"}, 2, `"Did you mean 'mutation except'?`},
		{[]string{"--json", "mutation", "chek"}, 2, `"Did you mean 'mutation check'?`},
		{[]string{"help", "mutation", "nosuch"}, 2, ""},
		{[]string{"help", "crap", "run"}, 2, ""},
		{[]string{"--json", "mutation"}, 2, `"command.missing"`},
		{[]string{"mutation", "--json"}, 2, `"command.missing"`},
		{[]string{"--json", "mutation", "rn"}, 2, `"Did you mean 'mutation run'?`},
		{[]string{"--json", "mutation", "--changed", "run"}, 2, `"flags.unknown"`},
	} {
		o := itosCc(t, c.args...)
		code, out := o.code, o.stdout
		if code != c.code || !strings.Contains(out, c.want) {
			t.Errorf("itos-cc %q: exit %d, want %d; stdout %q, want it to contain %q", c.args, code, c.code, out, c.want)
		}
	}
}

func TestEveryCommandsHelpGivesItsContract(t *testing.T) {
	for _, c := range leaves() {
		h := c.help()
		for _, part := range []string{"usage: itos-cc " + c.name, "Options:", "--json", `"schema": 1`, "Exit codes:", "  0 ", "  2 ", "  70 ", "Examples:", issues} {
			if !strings.Contains(h, part) {
				t.Errorf("%s --help lacks %q", c.name, part)
			}
		}
	}
	for name, g := range commands {
		if g.subs == nil {
			continue
		}
		h := g.groupHelp()
		for _, part := range []string{"usage: itos-cc " + name + " <subcommand>", "Subcommands:", "--json", "command.missing", "Exit codes:", "  0 ", "  2 ", "Examples:", issues} {
			if !strings.Contains(h, part) {
				t.Errorf("%s --help lacks %q", name, part)
			}
		}
	}
}

func TestAPanicIsAnInternalError(t *testing.T) {
	boom := &command{name: "boom", run: func(*invocation) (any, error) { panic("oops") }}
	var code int
	out := stdout(t, func() { code = execute(boom, []string{"--json"}) })
	if code != 70 || !strings.Contains(out, `"rule": "internal"`) || !strings.Contains(out, "boom panicked: oops") {
		t.Errorf("exit %d, stdout %q: want exit 70 and an internal problem", code, out)
	}
}
