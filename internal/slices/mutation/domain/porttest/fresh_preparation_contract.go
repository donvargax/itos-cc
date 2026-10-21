package porttest

import (
	"path/filepath"
	"testing"

	covport "github.com/donvargax/itos-cc/internal/coverage/port"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// FreshPreparationFixture independently supplies actual collector artifacts and
// command status after Plan determines the owned frozen location. The physical
// fixture uses entered commands; memory uses declared external observations.
type FreshPreparationFixture struct {
	Fresh     mutants.Fresh
	Root      string
	Collector func(*mutants.FreshPlan, int)
}

// FreshPreparationContract observes the same coarse preparation operation,
// including useful report data that must not erase a failed collector status.
func FreshPreparationContract(t *testing.T, open func(*testing.T) FreshPreparationFixture) {
	t.Helper()
	for _, exit := range []int{0, 7} {
		name := "successful built-in preparation"
		if exit != 0 {
			name = "failed collector retains partial data and blocks baseline"
		}
		t.Run(name, func(t *testing.T) {
			b := open(t)
			p, failed := b.Fresh.Plan(t.Context(), mutants.FreshRequest{Root: b.Root, Count: 1, Seed: "shared-preparation"})
			if failed != nil {
				t.Fatal(failed)
			}
			defer b.Fresh.Release(t.Context(), p)
			b.Collector(p, exit)
			prep, failed := b.Fresh.Prepare(t.Context(), p, covport.OwnTests)
			if prep == nil || prep.Commit != p.Commit || prep.Report == nil {
				t.Fatalf("actual partial evidence lost: %+v %v", prep, failed)
			}
			path := filepath.Join(p.FrozenRoot, "main.go")
			if hit, ok := prep.Report.LineCovered(path, 2); !hit || !ok {
				t.Fatalf("actual collector bytes not consumed: %+v %v", prep.Report, failed)
			}
			last := prep.Stages[len(prep.Stages)-1]
			if exit != 0 {
				if typed, ok := failed.(mutants.FreshFailed); !ok || typed.Stage != "coverage" {
					t.Fatalf("failed collector kind: %T %v", failed, failed)
				}
				if failed == nil || last.Name != "coverage" || last.State != "failed" || len(prep.Baselines) != 0 || prep.Report.Measures("go") {
					t.Fatalf("failed collector admitted baseline/proof: %+v %v", prep, failed)
				}
				for _, c := range prep.Commands {
					if len(c.Args) > 2 && c.Args[1] == "test" {
						for _, arg := range c.Args {
							if arg == "-failfast" {
								t.Fatal("baseline entered after failed collector")
							}
						}
					}
				}
			} else {
				if failed != nil || last.Name != "preparation" || last.State != "complete" || len(prep.Baselines) != 1 || len(prep.Listed) != 1 || prep.Listed[0].ID != "T-1" || !prep.Report.Measures("go") || len(prep.GoCoverage) != len(p.Units) {
					t.Fatalf("successful preparation evidence: %+v %v", prep, failed)
				}
				if ids := prep.Report.LineTests(path, 2); len(ids) != 1 || ids[0] != "T-1" {
					t.Fatalf("actual per-ID coverage: %v", ids)
				}
			}
		})
	}
}
