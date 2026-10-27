package mutation

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/cli"
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

// A --site run on Windows is count.platform, naming --site, before
// anything is observed. ID-MUT-248 holds it natively on Windows alone, and
// the code proof measures this system's in-process tests.
func TestANamedRunOnWindowsIsRefusedNamingSite(t *testing.T) {
	var out, errOut strings.Builder
	ui := &cli.UI{Stdout: &out, Stderr: &errOut}
	code := Run{Site: []string{"a.go:3:9"}, Repeat: 1, JSON: true}.Run(t.Context(), ui, Handler{Platform: "windows"}, cli.Written{"site": {"a.go:3:9"}})
	if code != cli.CodeEnvironment || !strings.Contains(out.String(), `"count.platform"`) || !strings.Contains(out.String(), "mutation run --site supports Linux and macOS") {
		t.Errorf("exit %d\n%s%s", code, out.String(), errOut.String())
	}
}

// --own-tests narrows a named judgment and nothing else (slice-67): given
// without --site, with --count or alone, or beside --all-tests, it is
// flags.conflict before anything is observed.
func TestOwnTestsWithoutANamedJudgmentConflicts(t *testing.T) {
	for _, c := range []struct {
		run     Run
		written cli.Written
	}{
		{Run{OwnTests: true, Repeat: 1, JSON: true}, cli.Written{"own-tests": {"true"}}},
		{Run{Count: 1, OwnTests: true, Repeat: 1, JSON: true}, cli.Written{"count": {"1"}, "own-tests": {"true"}}},
		{Run{Site: []string{"a.go:3:9"}, OwnTests: true, AllTests: true, Repeat: 1, JSON: true}, cli.Written{"site": {"a.go:3:9"}, "own-tests": {"true"}, "all-tests": {"true"}}},
	} {
		var out, errOut strings.Builder
		code := c.run.Run(t.Context(), &cli.UI{Stdout: &out, Stderr: &errOut}, Handler{Platform: "linux"}, c.written)
		if code != cli.CodeUsage || !strings.Contains(out.String(), `"flags.conflict"`) || !strings.Contains(out.String(), `"flag": "--own-tests"`) {
			t.Errorf("%v: exit %d\n%s%s", c.written, code, out.String(), errOut.String())
		}
	}
}

// planKept is a Fresh that keeps the request it is given, then stops as an
// interrupt stops it.
type planKept struct {
	mutants.Fresh
	request *mutants.FreshRequest
}

func (p planKept) Plan(ctx context.Context, r mutants.FreshRequest) (*mutants.FreshPlan, mutants.FreshFailure) {
	*p.request = r
	return nil, mutants.FreshCancelled{Cause: context.Cause(ctx)}
}

// --own-tests reaches the plan, whose preparation then lists no test, and
// the report, which says the listed tests did not run.
func TestOwnTestsReachThePlanAndTheReport(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(context.Canceled)
	var request mutants.FreshRequest
	h := Handler{Repository: func(context.Context) (string, error) { return t.TempDir(), nil }, Fresh: planKept{request: &request}}
	kept := &reportKept{}
	identity := mutants.Identity{File: "a.go"}
	q := CountedQuery{Sites: []mutants.FreshSite{{Argument: "a.go:…", File: "a.go", Identity: &identity}}, OwnTestsOnly: true, Progress: kept}
	if err := h.Counted(ctx, q); err != nil {
		t.Fatal(err)
	}
	if !request.OwnTestsOnly || kept.found == nil || !kept.found.Report.OwnTests {
		t.Fatalf("request %+v, found %+v: --own-tests did not reach both", request, kept.found)
	}
	var out strings.Builder
	(&Presentation{UI: &cli.UI{Stdout: &out}}).printCounted(kept.found.Report)
	if !strings.Contains(out.String(), "no snapshot, annotation or coverage cache was written; own tests only: the tests mutation.tests lists did not run\n") {
		t.Errorf("plain report:\n%s", out.String())
	}
	q.OwnTestsOnly = false
	if err := h.Counted(ctx, q); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	(&Presentation{UI: &cli.UI{Stdout: &out}}).printCounted(kept.found.Report)
	if request.OwnTestsOnly || kept.found.Report.OwnTests || strings.Contains(out.String(), "own tests only") {
		t.Errorf("without --own-tests: request %+v\n%s", request, out.String())
	}
}
