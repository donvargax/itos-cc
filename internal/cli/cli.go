// Package cli is the UI every command shares: where a command reads and
// writes, the problems it reports, the lines on stderr, the --json object,
// and the one place a run's problems become its exit code.
//
// It imports no domain. A domain's errors are a sealed set of plain types
// carrying no exit code, rule ID or wording; cmd hands the UI one
// Classifier per domain, each turning that domain's failures into problems
// in one switch naming every kind. An error no classifier claims is the 100
// of an unclassified bug, never a child process's status passed through.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
)

// The exit codes, as docs/CLI.md's table gives them: square/exit's, with
// 75 kept for a failure that may pass when run again (decision 36).
const (
	CodeNo             = 1   // a check said no
	CodeTemporary      = 75  // may pass when run again unchanged
	CodeUsage          = 80  // a usage or config error
	CodeUnknownCommand = 81  // a command or subcommand that is none
	CodeEnvironment    = 82  // a requirement not met: the environment lacks something
	CodeInternal       = 100 // an error nothing classified
	CodeInterrupted    = 130 // an interrupt stopped it (decision 38)
)

// Precedence picks the exit code of a run with problems of several codes:
// the first of these it has, not the highest, the one that says the most
// about why nothing else could be trusted (q-54, decision 36, docs/CLI.md's
// exit codes). The interrupt is first.
var Precedence = []int{CodeInterrupted, CodeUsage, CodeUnknownCommand, CodeInternal, CodeEnvironment, CodeTemporary, CodeNo}

// Issues is where a person reports a bug.
const Issues = "https://github.com/donvargax/itos-cc/issues"

// UI is a command's ends: what it reads and where it writes, and the
// classifiers that turn its domains' failures into problems. Nothing here
// asks a question, so there is no terminal to know of.
type UI struct {
	In     io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// Classifiers are cmd's, one per domain, tried in turn.
	Classifiers []Classifier
	// Dir is the working directory, which the paths a person reads are
	// named from; "" when it is not known.
	Dir string
}

// Rel is path as a person reads it: from the working directory when it is
// in it, and as it is otherwise, so output reads like the command line
// that produced it (Rel, from u.Dir).
func (u *UI) Rel(path string) string {
	return Rel(u.Dir, path)
}

// Rel is path named from dir, the working directory, when it is in it, and
// as it is otherwise or when dir is "", not known: the one copy of how a
// person reads a path, which the UI names paths by and cmd gives the
// slices whose handlers name files, from the same directory.
func Rel(dir, path string) string {
	if dir == "" {
		return path
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return rel
}

// Classifier turns err, a failure of one domain's sealed set, into its
// problems, each with its code, and says whether err was of that set. A
// classifier names every kind of its set in one switch, so gochecksumtype
// refuses it when a kind is added and left out.
type Classifier func(err error) ([]*Problem, bool)

// Fail reports err, a command failing, and returns its exit code: with
// withJSON the failure's object on stdout, otherwise each problem a line on
// stderr.
func (u *UI) Fail(err error, withJSON bool) int {
	return u.Emit(withJSON, nil, u.Problems(err))
}

// Usage reports err, a command line kong could not parse, as Fail does: a
// usage error, exit 80, in the words of the person who wrote the line. The
// command is named as it was written, so the sentence says what was read,
// never kong's own such as "EOL".
func (u *UI) Usage(command string, err error, withJSON bool) int {
	message := err.Error()
	if command != "" {
		message = command + ": " + message
	}
	return u.Emit(withJSON, nil, []*Problem{NewProblem(CodeUsage, "usage", message,
		"Run 'itos-cc --help' for the commands and the flags each takes.")})
}

// Emit prints a run's outcome and returns its exit code: with asJSON the
// object {"schema": 1, "ok": …, data's keys…, "problems": […]} on stdout,
// "problems" there only when ok is false; otherwise each problem plain
// output has not shown already on stderr, as "itos-cc: <message>. <fix>".
func (u *UI) Emit(asJSON bool, data any, problems []*Problem) int {
	if asJSON {
		out := map[string]any{}
		if data != nil {
			raw, err := json.Marshal(data)
			if err == nil {
				_ = json.Unmarshal(raw, &out)
			}
		}
		out["schema"] = 1
		out["ok"] = len(problems) == 0
		if len(problems) > 0 {
			out["problems"] = problems
		}
		u.JSON(out)
	} else {
		for _, p := range problems {
			if !p.Shown {
				u.Line(p.plain())
			}
		}
	}
	return ExitCode(problems)
}

// ExitCode is the exit code of a run with problems: 0 with none, else the
// first code of Precedence one of them has. A problem of a code the list
// lacks is a defect of ours, so with no listed code it is 100.
func ExitCode(problems []*Problem) int {
	for _, code := range Precedence {
		if slices.ContainsFunc(problems, func(p *Problem) bool { return p.Code == code }) {
			return code
		}
	}
	if len(problems) > 0 {
		return CodeInternal
	}
	return 0
}

// Problems is err as the problems it reports. A problem is itself; a
// failure a classifier claims is that classifier's problems; a cause no
// classifier claims that an interrupt gave, ErrInterrupted, is the
// interrupt; anything else is one internal problem, err whole. Errors joined (errors.Join) are each
// their own problems once one of them is claimed, so one never hides
// another.
func (u *UI) Problems(err error) []*Problem {
	if err == nil {
		return nil
	}
	all := leaves(err)
	var problems []*Problem
	claimed := false
	for _, leaf := range all {
		p, ok := u.classify(leaf)
		if !ok && errors.Is(leaf, ErrInterrupted) {
			p, ok = []*Problem{Interrupted()}, true
		}
		claimed = claimed || ok
		if !ok {
			p = []*Problem{Internal(leaf)}
		}
		problems = append(problems, p...)
	}
	if !claimed {
		return []*Problem{Internal(err)}
	}
	return problems
}

// classify is err's problems, from the first that claims it: err itself
// when it is a problem, or a classifier's.
func (u *UI) classify(err error) ([]*Problem, bool) {
	if p, ok := err.(*Problem); ok {
		return []*Problem{p}, true
	}
	for _, c := range u.Classifiers {
		if problems, ok := c(err); ok {
			return problems, true
		}
	}
	return nil, false
}

// leaves are the errors err joins, each in turn, or err.
func leaves(err error) []error {
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return []error{err}
	}
	var all []error
	for _, e := range joined.Unwrap() {
		all = append(all, leaves(e)...)
	}
	return all
}

// Internal is err as an internal problem, a defect of ours: exit 100.
func Internal(err error) *Problem {
	return NewProblem(CodeInternal, "internal", err.Error(), "This is a bug in itos-cc; please report it at "+Issues+".")
}

// JSON writes v on stdout as --json prints it, one object indented by two
// spaces; a failure to is a line on stderr.
func (u *UI) JSON(v any) {
	enc := json.NewEncoder(u.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		u.Line(err.Error())
	}
}

// Line writes message on stderr as a line for people, itos-cc: first. A
// stderr that cannot be written leaves nowhere else to say so.
func (u *UI) Line(message string) {
	_, _ = fmt.Fprintf(u.Stderr, "itos-cc: %s\n", message)
}
