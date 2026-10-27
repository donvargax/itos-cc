package mutation

import (
	"context"
	"errors"
	"testing"

	covport "github.com/donvargax/itos-cc/internal/coverage/port"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// preparationFails is a Fresh whose plan is plan and whose preparation
// fails before any trial.
type preparationFails struct{ plan *mutants.FreshPlan }

func (f preparationFails) Plan(context.Context, mutants.FreshRequest) (*mutants.FreshPlan, mutants.FreshFailure) {
	return f.plan, nil
}

func (preparationFails) Prepare(context.Context, *mutants.FreshPlan, covport.Scope) (*mutants.FreshPreparation, mutants.FreshFailure) {
	return &mutants.FreshPreparation{Stages: []mutants.PreparationStage{{Name: "coverage", State: "failed"}}}, mutants.FreshFailed{Stage: "coverage", Cause: errors.New("coverage failed")}
}

func (preparationFails) Release(context.Context, *mutants.FreshPlan) {}

// A counted run whose preparation fails stopped, unless it had nothing to
// judge, which stays not applicable: strict Go coverage of a range with no
// site prepares too. No test that reaches this line told the two apart
// (slice-63's code proof found `!=` → `==` surviving there), the
// scenarios of a failing preparation reading its problem, not its
// completion.
func TestAFailedPreparationStopsARunUnlessItWasNotApplicable(t *testing.T) {
	site := mutants.FreshCandidate{Identity: "a.go:example/a#A@1:3:0:0:<><=", Path: "a.go", Function: "example/a#A"}
	unit := mutants.FreshUnit{Path: "a.go", Function: "example/a#A", StartLine: 1, EndLine: 3}
	for _, c := range []struct {
		name       string
		plan       *mutants.FreshPlan
		completion string
	}{
		{"a site drawn", &mutants.FreshPlan{Eligible: []mutants.FreshCandidate{site}, Selected: []mutants.FreshCandidate{site}, Units: []mutants.FreshUnit{unit}}, "stopped"},
		{"no site, strict Go", &mutants.FreshPlan{Units: []mutants.FreshUnit{unit}}, "not-applicable"},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := Handler{Repository: func(context.Context) (string, error) { return t.TempDir(), nil }, Fresh: preparationFails{plan: c.plan}}
			kept := &reportKept{}
			if err := h.Counted(t.Context(), CountedQuery{Count: 1, FailUncovered: true, Progress: kept}); err != nil {
				t.Fatal(err)
			}
			s := kept.found.Report.Sampling
			if s.Completion != c.completion || s.Stop != "preparation failed at coverage" || kept.found.PreparationStage != "coverage" {
				t.Errorf("sampling %+v, stage %q, want %s, stopped at coverage", s, kept.found.PreparationStage, c.completion)
			}
		})
	}
}
