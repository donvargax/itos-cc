package mutation

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/coverage/port"
	aggregate "github.com/donvargax/itos-cc/internal/coverage/port/porttest"
	covtest "github.com/donvargax/itos-cc/internal/coverage/porttest"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// Moved from cmd with run's coverage consumers (T-100): coverage evidence
// comes solely from the bound port. No scenario reaches a missing or wrong
// binding, nor an inventory replaced under the same report.
func TestMutationCoverageConsumersUseActualBindings(t *testing.T) {
	root := t.TempDir()
	hit, miss := filepath.Join(root, "hit.py"), filepath.Join(root, "miss.py")
	d := covtest.Measurement{Language: "python", Report: "SF:hit.py\nDA:2,1\nend_of_record\n", InventoryReport: "SF:miss.py\nDA:5,1\nend_of_record\n"}
	m := &covtest.Measurer{Aggregate: aggregate.Measurer{Dir: root, Files: map[string]string{hit: "source", miss: "source"}}, Declarations: map[coverage.Unloaded]covtest.Measurement{{Language: "python", Dir: root}: d}}
	h := Handler{Coverage: m, Dir: root}
	q := RunQuery{}
	sources := []string{hit, miss}
	r, err := h.loadCoverage(t.Context(), q, sources, port.OwnTests, nil, nil)
	if err != nil || !r.Has(hit) || r.Has(miss) {
		t.Fatalf("bound measurement: %v %v", r, err)
	}
	r, _, _, err = h.strictCoverage(t.Context(), q, root, sources, []string{"python"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	lines, complete := r.Lines(miss)
	if !complete || len(lines) != 1 || lines[0].Line != 5 || lines[0].Covered {
		t.Fatalf("actual bound inventory lost: %+v %v", lines, complete)
	}
	// Replacing the independently supplied inventory cannot retain that proof.
	wrong := *m
	wrong.Declarations = map[coverage.Unloaded]covtest.Measurement{{Language: "python", Dir: root}: {Language: "python", Report: d.Report}}
	h.Coverage = &wrong
	r, _, _, err = h.strictCoverage(t.Context(), q, root, sources, []string{"python"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, complete := r.Lines(miss); complete {
		t.Fatal("wrong inventory binding still supplied proof")
	}
	h.Coverage = nil
	if _, err := h.loadCoverage(t.Context(), q, sources, port.OwnTests, nil, nil); err == nil || !strings.Contains(err.Error(), "not bound") {
		t.Fatalf("missing binding escaped: %v", err)
	}
	h.Coverage = &covtest.Measurer{Aggregate: aggregate.Measurer{Dir: root, Files: m.Aggregate.Files}, Declarations: map[coverage.Unloaded]covtest.Measurement{}}
	r, err = h.loadCoverage(t.Context(), q, sources, port.OwnTests, nil, nil)
	if err != nil || r.Has(hit) {
		t.Fatal("wrong measurement binding returned known real evidence")
	}
	h.Coverage = nil
	if r, err := h.loadCoverage(t.Context(), RunQuery{NoCoverage: true}, sources, port.OwnTests, nil, nil); r != nil || err != nil {
		t.Fatal("unused capability demanded by no-coverage")
	}
	h.Coverage = m
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := h.loadCoverage(ctx, q, sources, port.OwnTests, nil, nil); err != context.Canceled {
		t.Fatalf("caller cancellation lost: %v", err)
	}
}

// No scenario stops a listed coverage command whose owned group's cleanup
// then fails: such a run is tests.coverage-failed with
// coverage.cleanup-failed's gap beside it, which cmd's classifier words.
func TestAFailedListedCoverageRunKeepsItsCleanupFailureBesideIt(t *testing.T) {
	cleanup := port.CleanupFailed{ProcessGroup: 42, Command: "sh -c all", Reason: "group 42 did not end"}
	err := runError(ranInto(mutants.CoverageRunFailed{Command: "all", Reason: "signal: killed", ExitCode: -1, Cleanup: &cleanup}), func(s string) string { return s })
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok || len(joined.Unwrap()) != 2 {
		t.Fatalf("worded %T %v; want tests.coverage-failed and the cleanup's gap", err, err)
	}
	p, ok := joined.Unwrap()[0].(*cli.Problem)
	if !ok || p.Rule != "tests.coverage-failed" || p.Subject("command") != "all" || p.Subject("exit_code") != -1 || p.Code != cli.CodeNo {
		t.Fatalf("first: %#v", joined.Unwrap()[0])
	}
	var gap coverage.Unmeasured
	if !errors.As(joined.Unwrap()[1], &gap) || gap.Cause != cleanup {
		t.Fatalf("second: %#v", joined.Unwrap()[1])
	}
}
