package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/past"
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
		{&port.Cancelled{Cause: cli.ErrInterrupted}, cli.CodeInterrupted, "interrupted",
			"interrupted before it finished", "Run it again and let it finish.", nil},
		// A cancellation no interrupt caused is no interrupt (decision 38).
		{&port.Cancelled{Cause: context.Canceled}, cli.CodeInternal, "internal",
			"cancelled selection", "This is a bug in itos-cc; please report it at " + cli.Issues + ".", nil},
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

// The past commit's failures are problems of their own rules: a revision git
// cannot name is 80, no repository 82, an itos-cc.yaml the commit holds that
// cannot be read the settings' own config.invalid, 80; any other error is not
// the classifier's.
func TestEachPastFailureHasItsProblem(t *testing.T) {
	for _, c := range []struct {
		err  error
		rule string
		code int
	}{
		{&past.BadRev{Rev: "nosuch"}, "at.bad-rev", cli.CodeUsage},
		{&past.NoRepository{Reason: "none"}, "at.no-git", cli.CodeEnvironment},
		{&port.SettingsInvalid{Reason: "exclude: not a list", Key: "exclude"}, "config.invalid", cli.CodeUsage},
	} {
		problems, ok := classifyPast(c.err)
		if !ok || len(problems) != 1 || problems[0].Rule != c.rule || problems[0].Code != c.code {
			t.Errorf("%T: %v, %v; want one problem of %s, exit %d", c.err, problems, ok, c.rule, c.code)
		}
	}
	if problems, ok := classifyPast(errors.New("plain")); ok || problems != nil {
		t.Errorf("a plain error: %v, %v; want it left alone", problems, ok)
	}
	if problems, ok := classifyPast(&port.SettingsFailed{Cause: errors.New("cause")}); ok || problems != nil {
		t.Errorf("a settings read that failed short: %v, %v; want it left to its cause", problems, ok)
	}
}
