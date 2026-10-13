package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/donvargax/itos-cc/internal/cli"
	selport "github.com/donvargax/itos-cc/internal/selection/port"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// Every kind of the snapshot port's sealed set has its exit code, rule and
// sentences: a snapshot that could not be read, written or removed, or a
// folder of them listed, is the internal 70, its library's message whole
// with what a caller wrapped it in, as the commands have always reported
// it, and an interrupt 130.
func TestEachSnapshotFailureKindHasItsProblem(t *testing.T) {
	internal := "This is a bug in itos-cc; please report it at " + cli.Issues + "."
	for _, c := range []struct {
		failure snapport.Failure
		code    int
		rule    string
		message string
		fix     string
	}{
		{&snapport.Unreadable{Name: "scrap.json", Reason: "unexpected end of JSON input"}, cli.CodeInternal, "internal",
			"writing: unexpected end of JSON input", internal},
		{&snapport.Unwritable{Name: "dry.json", Reason: "mkdir .metrics: not a directory"}, cli.CodeInternal, "internal",
			"writing: mkdir .metrics: not a directory", internal},
		{&snapport.Unremovable{Name: "mutate/a.go.json", Reason: "remove .metrics/mutate/a.go.json: permission denied"}, cli.CodeInternal, "internal",
			"writing: remove .metrics/mutate/a.go.json: permission denied", internal},
		{&snapport.Unlistable{Dir: "mutate", Reason: "open .metrics/mutate/src: permission denied"}, cli.CodeInternal, "internal",
			"writing: open .metrics/mutate/src: permission denied", internal},
		{&snapport.Cancelled{}, cli.CodeInterrupted, "interrupted",
			"interrupted before it finished", "Run it again and let it finish."},
	} {
		problems, ok := classifySnapshot(fmt.Errorf("writing: %w", c.failure))
		if !ok || len(problems) != 1 {
			t.Fatalf("%T: claimed %t, %d problems; want its one", c.failure, ok, len(problems))
		}
		p := problems[0]
		if p.Code != c.code || p.Rule != c.rule || p.Message != c.message || p.Fix != c.fix {
			t.Errorf("%T: %+v, want %d %s %q %q", c.failure, p, c.code, c.rule, c.message, c.fix)
		}
	}
	unwritable := &snapport.Unwritable{Name: "dry.json", Reason: "no space left on device"}
	if problems, ok := classifySnapshot(unwritable); !ok || len(problems) != 1 || problems[0].Message != unwritable.Reason {
		t.Errorf("an unwrapped failure: %t, %+v; want its library's message alone", ok, problems)
	}
}

// Neither another domain's failure nor a plain error is the snapshot
// domain's to claim.
func TestAnErrorOfNoKindIsNotSnapshots(t *testing.T) {
	for _, err := range []error{errors.New("no space"), &selport.NoGit{Reason: "exit status 128"}} {
		if problems, ok := classifySnapshot(err); ok || problems != nil {
			t.Errorf("classifySnapshot(%v) claimed %v", err, problems)
		}
	}
}
