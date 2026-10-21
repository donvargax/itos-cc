package mutation

import (
	"context"
	"errors"
	"testing"

	"github.com/donvargax/itos-cc/internal/slices/mutation/domain"
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
