package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/selection/port"
)

// Every kind of the selection port's sealed set has its exit code, rule,
// sentences and subject, as the command line has always reported them:
// file_selection.feature reads the first two by their code and rule.
func TestEachSelectionFailureKindHasItsProblem(t *testing.T) {
	for _, c := range []struct {
		failure  port.Failure
		code     int
		rule     string
		message  string
		fix      string
		argument any
	}{
		{&port.NoGit{Reason: "exit status 128"}, cli.CodeEnvironment, "changed.no-git",
			"--changed needs a git repository: exit status 128", "Run it inside a git repository, or name the paths instead.", nil},
		{&port.Unmatched{Fragment: "nosuch"}, cli.CodeUsage, "paths.unmatched",
			`"nosuch" is no file, and no file's path under the working directory contains it`, "Name a file, a directory, or a fragment of a source's path.", "nosuch"},
		{&port.Unwalkable{Reason: "open src: permission denied"}, cli.CodeInternal, "internal",
			"open src: permission denied", "This is a bug in itos-cc; please report it at " + cli.Issues + ".", nil},
		{&port.Cancelled{}, cli.CodeInterrupted, "interrupted",
			"interrupted before it finished", "Run it again and let it finish.", nil},
	} {
		problems, ok := classifySelection(fmt.Errorf("selecting: %w", c.failure))
		if !ok || len(problems) != 1 {
			t.Fatalf("%T: claimed %t, %d problems; want its one", c.failure, ok, len(problems))
		}
		p := problems[0]
		if p.Code != c.code || p.Rule != c.rule || p.Message != c.message || p.Fix != c.fix || p.Subject("argument") != c.argument {
			t.Errorf("%T: %+v about %v, want %d %s %q %q about %v", c.failure, p, p.Subject("argument"), c.code, c.rule, c.message, c.fix, c.argument)
		}
	}
}

func TestAnErrorOfNoKindIsNotSelections(t *testing.T) {
	if problems, ok := classifySelection(errors.New("no space")); ok || problems != nil {
		t.Errorf("classifySelection claimed %v", problems)
	}
}
