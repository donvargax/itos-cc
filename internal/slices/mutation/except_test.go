package mutation

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/cli"
	selport "github.com/donvargax/itos-cc/internal/selection/port"
	selporttest "github.com/donvargax/itos-cc/internal/selection/port/porttest"
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

// No in-process scenario names a site or an identity that is neither, which
// is refused before anything is observed; a missing --reason words every
// such refusal as itself.
func TestNeitherASiteNorAnIdentityIsRefusedBeforeAnythingIsObserved(t *testing.T) {
	t.Parallel()
	reason := "x is never 5"
	for given, says := range map[*string]string{&reason: "is not a mutant's identity", nil: "--reason is required"} {
		var out, errOut strings.Builder
		ui := &cli.UI{Stdout: &out, Stderr: &errOut}
		code := Except{Site: []string{"board/board.go:Count@4:6:ab:35:<><="}, Reason: given}.Run(t.Context(), ui, Handler{})
		if code != cli.CodeUsage || !strings.Contains(errOut.String(), says) || out.Len() != 0 {
			t.Fatalf("given %v: exit %d %q", given, code, errOut.String())
		}
	}
	if got := (IdentityStale{Identity: "a.go:p#F@1:2:ab:3:<><=", Why: "gone"}).Error(); got != "a.go:p#F@1:2:ab:3:<><= is gone" {
		t.Fatalf("stale identity: %q", got)
	}
}

// No public scenario reaches what stops --rehash: a missing binding, an
// itos-cc.yaml with no entry, and a writer that refuses the document, which
// the UI words as config.invalid with its own fix.
func TestRehashStopsAtWhatItCannotReadOrWrite(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	src := "package board\n\nfunc Pos(x int) bool {\n\t// positive\n\treturn x > 0\n}\n"
	path := filepath.Join(root, "board.go")
	status := &mutantstest.SourceEdits{Files: map[string]string{path: src}}
	inputs := &mutantstest.Inputs{Dir: root, Files: map[string]string{path: src}}
	tree := selporttest.TreeOf(root, map[string]string{"board.go": src}, nil)
	// Pos's hash before 0.18.0, comment included, measured with sha256sum.
	entry := mutants.Exception{File: "board.go", Function: "board#Pos", Hash: "fad22ef65cb5e4a3", LineInFunction: 3, Column: 11,
		Original: ">", Replacement: ">=", Reason: "reviewed"}
	settings := func(entries ...mutants.Exception) *mutantstest.Settings {
		return &mutantstest.Settings{Value: mutants.Config{Selection: selport.Settings{Root: root}, Exceptions: entries}}
	}
	if _, err := (Handler{}).Rehash(t.Context()); err == nil || err.Error() != "mutation settings binding is missing" {
		t.Errorf("no Settings: %v", err)
	}
	if found, err := (Handler{Settings: settings()}).Rehash(t.Context()); err != nil || found.Root != root || found.Rehashed != nil || found.Left != nil {
		t.Errorf("no entry: %+v %v", found, err)
	}
	if _, err := (Handler{Settings: settings(entry), Tree: tree, Surroundings: inputs}).Rehash(t.Context()); err == nil {
		t.Error("no Status binding accepted")
	}
	h := Handler{Settings: settings(entry), Tree: tree, Surroundings: inputs, Status: status}
	if _, err := h.Rehash(t.Context()); !errors.Is(err, errNoExceptions) {
		t.Errorf("no Exceptions: %v", err)
	}
	store := &mutantstest.Exceptions{Entries: []mutants.Exception{entry}}
	h.Exceptions = func(string) mutants.Exceptions { return store }
	found, err := h.Rehash(t.Context())
	if err != nil || len(found.Rehashed) != 1 || store.Entries[0].Hash == entry.Hash || store.Writes != 1 {
		t.Fatalf("rehash: %+v %v, entries %+v", found, err, store.Entries)
	}
	store.Entries, store.Failure = []mutants.Exception{entry}, mutants.ExceptionInvalid{Reason: "wrong shape", Key: "mutation"}
	var out, errOut strings.Builder
	ui := &cli.UI{Stdout: &out, Stderr: &errOut}
	if code := (Except{Rehash: true}).Run(t.Context(), ui, h); code != cli.CodeUsage || !strings.Contains(errOut.String(), "wrong shape. Fix it, then rehash again.") || out.Len() != 0 {
		t.Errorf("refused document: exit %d, %q %q", code, out.String(), errOut.String())
	}
}
