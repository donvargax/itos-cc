package count

import (
	"errors"
	"fmt"
	"testing"

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/counting/port"
)

// Every kind the counting port's sealed set has is read by a scenario by its
// exit code and its --json rule ID. Both are the contract a script relies
// on, and they are checked here whole: a kind whose code or rule changed
// would break every script that reads it, and no other check here would say
// so.
func TestEachReadFailureKindHasItsCodeRuleAndSentence(t *testing.T) {
	for _, c := range []struct {
		failure port.ReadFailure
		code    int
		rule    string
		message string
		fix     string
	}{
		{&port.Missing{Path: "absent.txt"}, cli.CodeEnvironment, "count.input-missing",
			"the file absent.txt is not there", "Check the path, or give - to count standard input."},
		{&port.Unreadable{Path: "adirectory"}, cli.CodeEnvironment, "count.input-unreadable",
			"the file adirectory cannot be read as text", "Give a file this program can read, not a folder."},
		{&port.Cancelled{Path: "-"}, cli.CodeInterrupted, "interrupted",
			"interrupted before it finished", "Run it again and let it finish."},
	} {
		problems, ok := Classify(fmt.Errorf("counting: %w", c.failure))
		if !ok || len(problems) != 1 {
			t.Fatalf("%T: claimed %t, %d problems; want its one", c.failure, ok, len(problems))
		}
		if p := problems[0]; p.Code != c.code || p.Rule != c.rule || p.Message != c.message || p.Fix != c.fix {
			t.Errorf("%T: %+v, want %d %s %q %q", c.failure, p, c.code, c.rule, c.message, c.fix)
		}
	}
}

// An error of no kind in the port's set is not counting's: the UI makes it
// the 70 of a bug.
func TestAnErrorOfNoKindIsNotCountings(t *testing.T) {
	if problems, ok := Classify(errors.New("no space")); ok || problems != nil {
		t.Errorf("Classify claimed %v", problems)
	}
}
