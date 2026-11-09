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

// No public scenario reads the words of show's refusals, nor the exclusion's
// fix, which a script is told not to read but a person acts on: they must not
// say --site, judged, committed or survivor, as the other commands' do.
func TestShowWordsItsRefusalsItself(t *testing.T) {
	t.Parallel()
	id := "board/board.go:example.com/fx/board#Count@4:6:b1a8f41702cd975f:35:<><="
	same := func(path string) string { return path }
	for why, says := range map[string]string{
		mutants.UnmatchedChanged:  "its function changed since, its hash now 0123",
		mutants.UnmatchedGone:     "its file, its function or its site is gone",
		mutants.UnmatchedExcluded: "its source is one the exclude of itos-cc.yaml leaves out",
	} {
		hash := ""
		if why == mutants.UnmatchedChanged {
			hash = "0123"
		}
		p, ok := failureError(MutantUnmatched{Argument: id, Why: why, Hash: hash}, same).(*cli.Problem)
		if !ok || p.Rule != "site.unmatched" || p.Code != cli.CodeUsage || p.Subject("argument") != id || p.Subject("why") != why ||
			p.Message != id+" names no mutant of the code as it is now: "+says {
			t.Fatalf("%s: %+v", why, p)
		}
		if (p.Subject("hash") != nil) != (why == mutants.UnmatchedChanged) {
			t.Errorf("%s: hash %v", why, p.Subject("hash"))
		}
		if strings.Contains(p.Fix, "exclude") != (why == mutants.UnmatchedExcluded) || strings.Contains(p.Fix, "--site") || strings.Contains(p.Fix, "except") {
			t.Errorf("%s: fix %q", why, p.Fix)
		}
		for _, wrong := range []string{"--site", "judged", "survivor", "commit"} {
			if strings.Contains(p.Message, wrong) {
				t.Errorf("%s: message %q says %q", why, p.Message, wrong)
			}
		}
	}
	if got := (MutantUnmatched{Argument: id, Why: "gone"}).Error(); got != id+" is gone" {
		t.Errorf("Error: %q", got)
	}
}

// No public scenario reaches what is neither a refusal nor a mutant: settings
// that cannot be read and a stop, each of which keeps its own classification.
func TestShowKeepsWhatIsNoRefusal(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "project")
	const src = "package board\n\nfunc Pos(x int) bool {\n\treturn x > 0\n}\n"
	path := filepath.Join(root, "board.go")
	h := Handler{Settings: &mutantstest.Settings{Value: mutants.Config{Selection: selport.Settings{Root: root}}},
		Tree:         selporttest.TreeOf(root, map[string]string{"board.go": src}, nil),
		Surroundings: &mutantstest.Inputs{Dir: root, Files: map[string]string{path: src}},
		Status:       &mutantstest.SourceEdits{Files: map[string]string{path: src}}}
	id := mutants.Identity{File: "board.go", Namespace: "board", Name: "Pos", StartLine: 3, EndLine: 5}
	if _, err := (Handler{}).Show(t.Context(), "arg", id); err == nil || err.Error() != "mutation settings binding is missing" {
		t.Errorf("no Settings: %v", err)
	}
	cause := errors.New("show stopped")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	if found, err := h.Show(ctx, "arg", id); !errors.Is(err, cause) || found != (mutants.ShownMutant{}) {
		t.Errorf("stopped: %+v %v", found, err)
	}
	// Past the settings, what stops the resolver is no refusal either.
	unbound := h
	unbound.Status = nil
	var unmatched MutantUnmatched
	if _, err := unbound.Show(t.Context(), "arg", id); err == nil || errors.As(err, &unmatched) || !strings.Contains(err.Error(), "status binding") {
		t.Errorf("no Status: %v", err)
	}
	var out, errOut strings.Builder
	ui := &cli.UI{Stdout: &out, Stderr: &errOut}
	invalid := Handler{Settings: &mutantstest.Settings{Failure: mutants.SettingsInvalid{Reason: "wrong shape"}}}
	code := Show{Identity: []string{"board.go:board#Pos@3:5:ab:30:>>>="}}.Run(t.Context(), ui, invalid)
	if code != cli.CodeUsage || !strings.Contains(errOut.String(), "wrong shape") || out.Len() != 0 {
		t.Errorf("settings: exit %d %q %q", code, out.String(), errOut.String())
	}
}
