package cli

import (
	"encoding/json"
	"fmt"
	"slices"
)

// The contract with scripts is the exit code, the --json object less its
// "message" and "fix" keys, and the files under .metrics/. Plain output is
// for people and may change in any release. See docs/CLI.md.

// Problem is one reason a run did not succeed. Code is its exit code; Rule
// is a stable, dotted lower-case ID scripts switch on; Message and Fix are
// sentences for people. Its subject holds what the problem is about, such
// as file, line and function, as keys of their own beside rule, message and
// fix in --json, so no script parses a message.
type Problem struct {
	Code    int
	Rule    string
	Message string
	Fix     string
	// Shown is set when plain output already shows the problem, so it is
	// not repeated on stderr.
	Shown   bool
	subject map[string]any
}

// reserved are the names a subject key never takes: the problem's own keys
// in --json, exit, its own exit code, among them (docs/CLI.md).
var reserved = []string{"rule", "message", "fix", "exit"}

// NewProblem is a problem of its code, rule, message and fix, about nothing
// yet.
func NewProblem(code int, rule, message, fix string) *Problem {
	return &Problem{Code: code, Rule: rule, Message: message, Fix: fix}
}

// With sets the subject key to value and returns p. A reserved key is a
// defect of ours, so it panics rather than let a subject hide the
// problem's own keys.
func (p *Problem) With(key string, value any) *Problem {
	if slices.Contains(reserved, key) {
		panic(fmt.Sprintf("cli: %q is reserved and names no subject key", key))
	}
	if p.subject == nil {
		p.subject = map[string]any{}
	}
	p.subject[key] = value
	return p
}

// Subject is the value of the subject key, or nil.
func (p *Problem) Subject(key string) any { return p.subject[key] }

// Error is the problem's message.
func (p *Problem) Error() string { return p.Message }

// MarshalJSON is the problem as --json gives it: rule, message, fix when
// there is one, its own exit code, as RFC 9457's status gives each problem
// its own (q-54), and the subject's keys beside them.
func (p *Problem) MarshalJSON() ([]byte, error) {
	out := map[string]any{"rule": p.Rule, "message": p.Message, "exit": p.Code}
	if p.Fix != "" {
		out["fix"] = p.Fix
	}
	for k, v := range p.subject {
		out[k] = v
	}
	return json.Marshal(out)
}

// plain is the problem as a line for people: its message, then its fix.
func (p *Problem) plain() string {
	if p.Fix == "" {
		return p.Message
	}
	return p.Message + ". " + p.Fix
}
