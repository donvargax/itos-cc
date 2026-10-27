package mutation

import (
	"context"
	"path/filepath"
	"testing"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// A --site position is named from the working directory, which the
// command line always has inside the repository, so no scenario can name
// a position outside it, nor run without the working directory's binding:
// these hold the handler's side of both (slice-63). A position outside the
// repository becomes a file no commit holds, which the plan calls gone;
// an identity's file, from the repository's top already, is kept.
func TestANamedPositionIsMadeAPathFromTheRepositorysTop(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	wd := filepath.Join(root, "board")
	h := Handler{PhysicalDir: func(context.Context) (string, error) { return wd, nil }}
	id := mutants.Identity{File: "board/board.go"}
	sites := []mutants.FreshSite{
		{Argument: "board.go:5:11", File: "board.go", Line: 5, Column: 11},
		{Argument: "../../x.go:1:1", File: filepath.Join("..", "..", "x.go"), Line: 1, Column: 1},
		{Argument: "board/board.go:…", File: "board/board.go", Identity: &id},
	}
	got, err := h.namedSites(t.Context(), root, sites)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].File != "board/board.go" || got[1].File != "" || got[2].File != "board/board.go" {
		t.Errorf("files %q, %q, %q, want board/board.go, none and the identity's", got[0].File, got[1].File, got[2].File)
	}
	if sites[0].File != "board.go" {
		t.Errorf("the sites given were changed: %q", sites[0].File)
	}
}

func TestANamedPositionNeedsTheWorkingDirectory(t *testing.T) {
	h := Handler{Repository: func(context.Context) (string, error) { return t.TempDir(), nil }}
	err := h.Counted(t.Context(), CountedQuery{Sites: []mutants.FreshSite{{Argument: "a.go:1:1", File: "a.go", Line: 1, Column: 1}}})
	if err == nil || err.Error() != "mutation working directory binding is missing" {
		t.Errorf("Counted = %v, want the missing binding", err)
	}
}

// planInterrupted is a Fresh whose planning an interrupt stopped.
type planInterrupted struct{ mutants.Fresh }

func (planInterrupted) Plan(ctx context.Context, _ mutants.FreshRequest) (*mutants.FreshPlan, mutants.FreshFailure) {
	return nil, mutants.FreshCancelled{Cause: context.Cause(ctx)}
}

// reportKept keeps the report a counted run presents.
type reportKept struct {
	mutants.Progress
	found *CountedFound
}

func (*reportKept) Unjudged(string, []string)   {}
func (*reportKept) NotBuilt([]mutants.NotBuilt) {}
func (r *reportKept) Counted(_ context.Context, found CountedFound) {
	r.found = &found
}

// An interrupt while the plan is made reports what --site bounds, not what
// --count does: no scenario can interrupt a run between its start and its
// plan, which takes no time it could be stopped in.
func TestANamedRunInterruptedWhilePlanningSaysWhatSiteBounds(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(context.Canceled)
	h := Handler{Repository: func(context.Context) (string, error) { return t.TempDir(), nil }, Fresh: planInterrupted{}}
	kept := &reportKept{}
	identity := mutants.Identity{File: "a.go"}
	if err := h.Counted(ctx, CountedQuery{Sites: []mutants.FreshSite{{Argument: "a.go:…", File: "a.go", Identity: &identity}}, Progress: kept}); err != nil {
		t.Fatal(err)
	}
	if kept.found == nil || kept.found.Report.Sampling.Completion != "interrupted" || kept.found.Report.Sampling.Bounds != namedBounds {
		t.Errorf("found %+v, want an interrupted report bounded as --site bounds", kept.found)
	}
}
