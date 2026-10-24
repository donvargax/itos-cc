package mutation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/cli"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	mutantstest "github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

// No public scenario reaches a Windows drive letter on the platforms the
// binary's scenarios run on, nor the narrower malformed sites.
func TestASiteIsFileLineAndColumn(t *testing.T) {
	t.Parallel()
	for arg, want := range map[string]struct {
		path         string
		line, column int
		ok           bool
	}{
		"src/board.go:11:11":   {"src/board.go", 11, 11, true},
		`C:\src\board.go:7:19`: {`C:\src\board.go`, 7, 19, true},
		"src/board.go:11":      {},
		"src/board.go":         {},
		":1:1":                 {},
		"src/board.go:0:1":     {},
		"src/board.go:1:x":     {},
	} {
		path, line, column, ok := parseSite(arg)
		if path != want.path || line != want.line || column != want.column || ok != want.ok {
			t.Errorf("%q: %q %d %d %v, want %+v", arg, path, line, column, ok, want)
		}
	}
}

// No public scenario can cancel the caller between the survivor's judgment
// and the write: the writer's cancellation keeps the caller's cause, and its
// invalid document is config.invalid with the fix the UI gave.
func TestAnExceptionUpdateKeepsItsCause(t *testing.T) {
	cause := errors.New("update caller cancelled")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	entry := mutants.Exception{File: "board.go", Function: "p#F", Hash: "h", LineInFunction: 1, Column: 2, Original: ">", Replacement: ">=", Reason: "reviewed"}
	same := func(path string) string { return path }
	if _, failure := (&mutantstest.Exceptions{}).Except(ctx, entry); !errors.Is(exceptError(failure, same, "fix"), cause) {
		t.Fatalf("cancel cause: %v", failure)
	}
	invalid := &mutantstest.Exceptions{Failure: mutants.ExceptionInvalid{Reason: "wrong shape", Key: "mutation"}}
	_, failure := invalid.Except(t.Context(), entry)
	if got := exceptError(failure, same, "Fix it, then except the site again.").Error(); got != "itos-cc.yaml: wrong shape" {
		t.Fatalf("invalid document: %q", got)
	}
}

// No public scenario gives a malformed identity, which is told from a site
// by its @<start>:<end>:<hash>:<offset>: part alone.
func TestAnIdentityIsToldFromASite(t *testing.T) {
	t.Parallel()
	reason := "x is never 5"
	q, p := exceptQuery("board/board.go:example.com/fx/board#Count@4:6:b1a8f41702cd975f:35:<><=", &reason)
	if p != nil || q.Identity == nil || q.Identity.Name != "Count" || q.Path != "" || q.Reason != &reason {
		t.Fatalf("identity: %+v %v", q, p)
	}
	q, p = exceptQuery("src/board.go:7:19", nil)
	if p != nil || q.Identity != nil || q.Path != "src/board.go" || q.Line != 7 || q.Column != 19 {
		t.Fatalf("site: %+v %v", q, p)
	}
	for _, arg := range []string{"board/board.go:Count@4:6:b1a8f41702cd975f:35:<><=", "src/board.go:7"} {
		_, p = exceptQuery(arg, nil)
		if p == nil || p.Rule != "args.invalid" || p.Subject("argument") != arg {
			t.Fatalf("%q: %v", arg, p)
		}
	}
	if _, p = exceptQuery("board.go:Count@4:6:ab:35:<><=", nil); !strings.Contains(p.Message, "identity") {
		t.Fatalf("malformed identity: %q", p.Message)
	}
}

// No public scenario excludes the file an identity names, nor reads the
// hash a changed function's refusal gives beside why.
func TestAnIdentityIsWordedWhereASiteWouldBe(t *testing.T) {
	t.Parallel()
	same := func(path string) string { return path }
	id := "board/board.go:example.com/fx/board#Count@4:6:b1a8f41702cd975f:35:<><="
	excluded := failureError(SiteExcluded{Path: "board/board.go", Identity: id}, same).(*cli.Problem)
	if !strings.HasPrefix(excluded.Message, id+" is in board/board.go") || excluded.Subject("file") != "board/board.go" {
		t.Fatalf("excluded identity: %q", excluded.Message)
	}
	site := failureError(SiteExcluded{Path: "board/board.go", Line: 5, Column: 11}, same).(*cli.Problem)
	if !strings.HasPrefix(site.Message, "board/board.go:5:11 is in") {
		t.Fatalf("excluded site: %q", site.Message)
	}
	changed := failureError(IdentityStale{Path: "board/board.go", Identity: id, Function: "example.com/fx/board#Count", Why: mutants.ExceptionChanged, Hash: "0123"}, same).(*cli.Problem)
	if changed.Rule != "exception.identity-stale" || changed.Code != cli.CodeUsage || changed.Subject("hash") != "0123" || changed.Subject("why") != "changed" ||
		changed.Subject("function") != "example.com/fx/board#Count" || !strings.Contains(changed.Message, "its hash now 0123") {
		t.Fatalf("changed: %+v", changed)
	}
	gone := failureError(IdentityStale{Path: "board/board.go", Identity: id, Function: "example.com/fx/board#Count", Why: mutants.ExceptionGone}, same).(*cli.Problem)
	if gone.Subject("hash") != nil || gone.Subject("identity") != id || !strings.Contains(gone.Message, "is gone") {
		t.Fatalf("gone: %+v", gone)
	}
}
