package domain_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/selection"
	seltest "github.com/donvargax/itos-cc/internal/selection/port/porttest"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

func TestFreshExceptionJudgmentUsesSelectedFrozenFunctionIdentity(t *testing.T) {
	f, p := freshPython(t)
	c := p.Selected[0]
	path := filepath.Join(p.FrozenRoot, filepath.FromSlash(c.Path))
	file, err := selection.Parse(t.Context(), *f.Tree, f.Inputs, path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	u := file.Units[c.Site.Unit]
	e := mutants.Exception{File: c.Path, Function: mutants.UnitID(u.Namespace, u.Name), Hash: mutants.UnitHash(file, u), LineInFunction: c.Site.Line - u.StartLine + 1, Column: c.Site.Column, Original: c.Site.Original, Replacement: c.Site.Replacement, Reason: "independently reviewed equivalent boundary"}
	x, err := mutants.PlanExceptions(t.Context(), p, []mutants.Exception{e}, *f.Tree, f.Inputs)
	if err != nil {
		t.Fatal(err)
	}
	if len(x.Stale) != 0 {
		t.Fatalf("actual frozen exception stale: %+v", x.Stale)
	}
	if _, err := mutants.PlanExceptions(t.Context(), p, []mutants.Exception{e}, nil, f.Inputs); err == nil {
		t.Fatal("missing exception source binding was hidden")
	}
	if _, err := mutants.PlanExceptions(t.Context(), p, []mutants.Exception{e}, seltest.TreeOf(p.FrozenRoot, map[string]string{}, nil), f.Inputs); err == nil {
		t.Fatal("wrong exception source binding fell back to frozen disk")
	}
	if _, err := mutants.PlanExceptions(t.Context(), p, nil, nil, nil); err != nil {
		t.Fatal("unused exception observations were required", err)
	}
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("caller stopped exception admission")
	cancel(cause)
	if _, err := mutants.PlanExceptions(ctx, p, []mutants.Exception{e}, nil, nil); !errors.Is(err, cause) {
		t.Fatal("caller cancellation hidden by missing observations", err)
	}
	if reason, stale := x.Judge(c, mutants.Survived); reason != e.Reason || stale != nil {
		t.Fatalf("valid selected survivor: %q %+v", reason, stale)
	}
	for _, outcome := range []string{mutants.Killed, mutants.Timeout} {
		if reason, stale := x.Judge(c, outcome); reason != "" || stale == nil || stale.Why != mutants.ExceptionKilled {
			t.Fatalf("completed %s judgment: %q %+v", outcome, reason, stale)
		}
	}
	if reason, stale := x.Judge(c, mutants.Uncovered); reason != "" || stale != nil {
		t.Fatal("uncovered selection was excepted or marked killed")
	}
	other := c
	other.Identity = "independent unselected identity"
	if reason, stale := x.Judge(other, mutants.Survived); reason != "" || stale != nil {
		t.Fatal("exception applied to unselected site identity")
	}
	previous, err := lang.Parse(t.Context(), f.Inputs, lang.Detect(path), path, []byte("def value(x):\n    return x + 2\n"))
	if err != nil {
		t.Fatal(err)
	}
	defer previous.Close()
	e.Hash = mutants.UnitHash(previous, previous.Units[0])
	x, err = mutants.PlanExceptions(t.Context(), p, []mutants.Exception{e}, *f.Tree, f.Inputs)
	if err != nil {
		t.Fatal(err)
	}
	if len(x.Stale) != 1 || x.Stale[0].Why != mutants.ExceptionChanged {
		t.Fatalf("old function identity trusted: %+v", x.Stale)
	}
	if reason, stale := x.Judge(c, mutants.Survived); reason != "" || stale != nil {
		t.Fatal("changed exception trusted for selected survivor")
	}
	e.File = "unrelated.py"
	x, err = mutants.PlanExceptions(t.Context(), p, []mutants.Exception{e}, *f.Tree, f.Inputs)
	if err != nil || len(x.Stale) != 0 {
		t.Fatalf("unrelated exception judged: %+v %v", x, err)
	}
}

func TestFreshListingPreservesStrictFrozenIdentityAdmission(t *testing.T) {
	got, err := mutants.ParseFreshTests("  A \t tests/test_a.py \r\n\nB\r\n")
	want := []coverage.Test{{ID: "A", File: "tests/test_a.py"}, {ID: "B"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("actual listing rows: %+v %v", got, err)
	}
	for _, raw := range []string{"\n\r\n", "A\nA\n", "A\t../outside.py\n", "A\t.git/config\n", "A\ttests/./test_a.py\n"} {
		if rows, err := mutants.ParseFreshTests(raw); err == nil || rows != nil {
			t.Fatalf("unsupported/ambiguous frozen identity admitted: %q %+v %v", raw, rows, err)
		}
	}
}
