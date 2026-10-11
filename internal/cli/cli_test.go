package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// gone and locked are a domain's sealed failures as the UI meets them: it
// imports no domain, so its tests bring a set of their own, and classifier
// is that domain's, as cmd hands each domain's to the UI.
type gone struct{ path string }

func (e *gone) Error() string { return e.path + " is gone" }

type locked struct{ path string }

func (e *locked) Error() string { return e.path + " is locked" }

func classifier(err error) ([]*Problem, bool) {
	var g *gone
	if errors.As(err, &g) {
		return []*Problem{NewProblem(CodeEnvironment, "probe.gone", "the file "+g.path+" is not there", "Check the path.").With("file", g.path)}, true
	}
	var l *locked
	if errors.As(err, &l) {
		return []*Problem{NewProblem(CodeTemporary, "probe.locked", "the file "+l.path+" is locked", "Run it again.")}, true
	}
	return nil, false
}

// ends is a UI with the probe domain's classifier, writing where a test
// reads.
type ends struct {
	stdout, stderr strings.Builder
}

func (e *ends) ui() *UI {
	return &UI{Stdout: &e.stdout, Stderr: &e.stderr, Classifiers: []Classifier{classifier}}
}

// A failure a classifier claims is its problems and its code: the rule a
// script reads, the subject beside it, and the sentence and fix a person
// reads on stderr.
func TestAClassifiedFailureIsItsDomainsProblem(t *testing.T) {
	var e ends
	if code := e.ui().Fail(fmt.Errorf("counting: %w", &gone{"absent.txt"}), true); code != CodeEnvironment {
		t.Fatalf("Fail = %d, want %d", code, CodeEnvironment)
	}
	want := "{\n  \"ok\": false,\n  \"problems\": [\n    {\n      \"file\": \"absent.txt\",\n" +
		"      \"fix\": \"Check the path.\",\n      \"message\": \"the file absent.txt is not there\",\n" +
		"      \"rule\": \"probe.gone\"\n    }\n  ],\n  \"schema\": 1\n}\n"
	if got := e.stdout.String(); got != want {
		t.Errorf("--json is\n%s\nwant\n%s", got, want)
	}
	if e.stderr.Len() != 0 {
		t.Errorf("stderr = %q, want none with --json", e.stderr.String())
	}
	var plain ends
	plain.ui().Fail(&gone{"absent.txt"}, false)
	if got, want := plain.stderr.String(), "itos-cc: the file absent.txt is not there. Check the path.\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
	if plain.stdout.Len() != 0 {
		t.Errorf("stdout = %q, want none without --json", plain.stdout.String())
	}
}

// An error no classifier claims is a defect of ours: 70, and never some
// other code a child process might have returned.
func TestAnErrorOfNoKnownKindIsInternal(t *testing.T) {
	cause := fmt.Errorf("cannot write the report: %w", errors.New("no space"))
	var e ends
	if code := e.ui().Fail(cause, false); code != CodeInternal {
		t.Fatalf("Fail = %d, want %d", code, CodeInternal)
	}
	want := "itos-cc: cannot write the report: no space. This is a bug in itos-cc; please report it at " + Issues + ".\n"
	if got := e.stderr.String(); got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

// A problem returned as an error is itself, whichever classifiers the UI
// has.
func TestAProblemIsItself(t *testing.T) {
	p := NewProblem(CodeUsage, "flags.unknown", "crap takes no flag --bogus", "Run 'itos-cc crap --help' for its flags.")
	got := (&UI{}).Problems(p)
	if len(got) != 1 || got[0] != p {
		t.Errorf("Problems = %v, want the problem itself", got)
	}
	if (&UI{}).Problems(nil) != nil {
		t.Error("Problems(nil) is not none")
	}
}

// Failures joined are each a problem and a line of their own, under the
// code of the precedence: one joined error never hides another.
func TestJoinedFailuresAreEachAProblemUnderOneCode(t *testing.T) {
	joined := errors.Join(&locked{"a.txt"}, &gone{"b.txt"}, errors.New("stray"))
	var e ends
	if code := e.ui().Fail(joined, false); code != CodeInternal {
		t.Fatalf("Fail of three joined = %d, want %d", code, CodeInternal)
	}
	want := "itos-cc: the file a.txt is locked. Run it again.\n" +
		"itos-cc: the file b.txt is not there. Check the path.\n" +
		"itos-cc: stray. This is a bug in itos-cc; please report it at " + Issues + ".\n"
	if got := e.stderr.String(); got != want {
		t.Errorf("stderr =\n%s\nwant\n%s", got, want)
	}
}

// Errors joined that nothing claims are one internal problem, the error
// whole, as an error of no kind is.
func TestJoinedErrorsNothingClaimsAreOneInternalProblem(t *testing.T) {
	joined := errors.Join(errors.New("one"), errors.New("two"))
	problems := (&UI{Classifiers: []Classifier{classifier}}).Problems(joined)
	if len(problems) != 1 || problems[0].Rule != "internal" || problems[0].Message != "one\ntwo" {
		t.Errorf("Problems = %+v, want one internal problem of both", problems)
	}
}

// The exit code is the first of the precedence a run's problems have, not
// the highest: a usage error says more than an internal one, an internal
// one more than a missing tool.
func TestExitCodeComesFromTheMostTellingKind(t *testing.T) {
	if got, want := Precedence, []int{130, 2, 70, 3, 75, 1}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("Precedence = %v, want %v (docs/CLI.md)", got, want)
	}
	of := func(codes ...int) []*Problem {
		var problems []*Problem
		for _, c := range codes {
			problems = append(problems, NewProblem(c, "x", "", ""))
		}
		return problems
	}
	for _, c := range []struct {
		problems []*Problem
		want     int
	}{
		{nil, 0},
		{of(CodeNo), 1},
		{of(CodeNo, CodeEnvironment), 3},
		{of(CodeEnvironment, CodeUsage, CodeNo), 2},
		{of(CodeEnvironment, CodeInternal), 70},
		{of(CodeNo, CodeTemporary), 75},
		{of(CodeTemporary, CodeEnvironment), 3},
		{of(CodeUsage, CodeInterrupted), 130},
		{of(42), CodeInternal},
	} {
		if got := ExitCode(c.problems); got != c.want {
			t.Errorf("%d problems: exit %d, want %d", len(c.problems), got, c.want)
		}
	}
}

// --json is one object: the command's own keys beside schema, ok and
// problems, each problem its rule, sentences and subject keys.
func TestJSONIsOneObjectWithSchemaOkAndProblems(t *testing.T) {
	var e ends
	code := e.ui().Emit(true, struct {
		Files []string `json:"files"`
	}{[]string{"a.go"}}, []*Problem{NewProblem(CodeNo, "mutation.survived", "it survived", "Add a test.").With("line", 3)})
	if code != CodeNo {
		t.Errorf("exit %d, want %d", code, CodeNo)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(e.stdout.String()), &got); err != nil {
		t.Fatalf("%v in %q", err, e.stdout.String())
	}
	problems, _ := got["problems"].([]any)
	if got["schema"] != 1.0 || got["ok"] != false || len(problems) != 1 || got["files"] == nil {
		t.Fatalf("object %v", got)
	}
	if p := problems[0].(map[string]any); p["rule"] != "mutation.survived" || p["line"] != 3.0 || p["fix"] != "Add a test." || p["message"] != "it survived" {
		t.Errorf("problem %v, want its rule, sentences, and subject keys", p)
	}
	var ok ends
	if code := ok.ui().Emit(true, map[string]int{"total": 2}, nil); code != 0 {
		t.Errorf("exit %d with no problems, want 0", code)
	}
	if got, want := ok.stdout.String(), "{\n  \"ok\": true,\n  \"schema\": 1,\n  \"total\": 2\n}\n"; got != want {
		t.Errorf("--json with no problems is %q, want %q", got, want)
	}
}

// A problem with no fix says only what is wrong, in --json and on stderr;
// one plain output has shown already is not repeated there.
func TestAProblemWithoutAFixOrShownAlready(t *testing.T) {
	var e ends
	shown := NewProblem(CodeNo, "x.shown", "shown above", "Fix it.")
	shown.Shown = true
	e.ui().Emit(false, nil, []*Problem{NewProblem(CodeNo, "x.bare", "no fix", ""), shown})
	if got, want := e.stderr.String(), "itos-cc: no fix\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
	raw, err := json.Marshal(NewProblem(CodeNo, "x.bare", "no fix", ""))
	if err != nil || string(raw) != `{"message":"no fix","rule":"x.bare"}` {
		t.Errorf("JSON = %s, %v; want no fix key", raw, err)
	}
}

// A subject key never takes a name --json gives the problem itself: one
// that tried would hide the rule a script reads.
func TestASubjectKeyIsNeverReserved(t *testing.T) {
	for _, key := range []string{"rule", "message", "fix", "exit"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("With(%q) did not panic", key)
				}
			}()
			_ = NewProblem(CodeNo, "x.y", "m", "f").With(key, "v")
		}()
	}
	p := (&Problem{Code: CodeNo, Rule: "x.y"}).With("file", "a.go")
	if p.Subject("file") != "a.go" || p.Subject("line") != nil || p.Error() != "" {
		t.Errorf("subject %v, %v", p.Subject("file"), p.Subject("line"))
	}
}

// A usage failure is exit 2, names the command as it was written, and says
// what to do next: a person reads it, a script reads its rule.
func TestUsageNamesTheCommandAndTheWayOut(t *testing.T) {
	var e ends
	if code := e.ui().Usage("count", errors.New(`expected "<file>"`), true); code != CodeUsage {
		t.Fatalf("Usage exit = %d, want %d", code, CodeUsage)
	}
	if e.stderr.Len() != 0 {
		t.Errorf("stderr = %q, want none with --json", e.stderr.String())
	}
	var got struct {
		Problems []map[string]string `json:"problems"`
	}
	if err := json.Unmarshal([]byte(e.stdout.String()), &got); err != nil || len(got.Problems) != 1 {
		t.Fatalf("--json = %q: %v", e.stdout.String(), err)
	}
	if p := got.Problems[0]; p["rule"] != "usage" || p["message"] != `count: expected "<file>"` ||
		p["fix"] != "Run 'itos-cc --help' for the commands and the flags each takes." {
		t.Errorf("problem = %v", p)
	}
}

// A usage error of a line that names no command says what was wrong and
// what to do: there is no command to name.
func TestUsageOfALineNamingNoCommandSaysOnlyWhatWasWrong(t *testing.T) {
	var e ends
	if code := e.ui().Usage("", errors.New("unknown flag --bogus"), false); code != CodeUsage {
		t.Fatalf("Usage exit = %d, want %d", code, CodeUsage)
	}
	if got, want := e.stderr.String(), "itos-cc: unknown flag --bogus. Run 'itos-cc --help' for the commands and the flags each takes.\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

// An interrupt is exit 130 and says so, whatever domain's work it stopped.
func TestAnInterruptIsItsOwnProblem(t *testing.T) {
	p := Interrupted()
	if p.Code != CodeInterrupted || p.Rule != "interrupted" || p.plain() != "interrupted before it finished. Run it again and let it finish." {
		t.Errorf("Interrupted = %+v", p)
	}
}

// A writer that cannot hold what it is given is a line on stderr, not a
// silent loss: --json's contract is what a script reads.
func TestAFailureToWriteJSONIsALineOnStderr(t *testing.T) {
	var stderr strings.Builder
	ui := &UI{Stdout: failingWriter{}, Stderr: &stderr}
	ui.JSON(struct {
		Schema int `json:"schema"`
	}{1})
	if got, want := stderr.String(), "itos-cc: the disk is full\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

// failingWriter is a writer that cannot be written, whatever a machine makes
// one: no disk, no permission bit.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("the disk is full") }
