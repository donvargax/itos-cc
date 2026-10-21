// Package freshenv prepares physical immutable environments for counted judgments.
// It receives concrete observations/execution from composition, never selecting adapters.
package freshenv

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"path/filepath"

	"github.com/donvargax/itos-cc/config"
	covrun "github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/internal/coverage"
	langport "github.com/donvargax/itos-cc/internal/lang/port"
	"github.com/donvargax/itos-cc/internal/selection"
	selport "github.com/donvargax/itos-cc/internal/selection/port"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// Environment retains bindings and physical policy, never a caller context.
// Execute is the counted invocation's owned executor shared with its trial Workers.
type Environment struct {
	Platform     string
	Log          io.Writer
	Execute      covrun.CommandExecutor
	Command      func(context.Context, *exec.Cmd) (mutants.CommandResult, error)
	Toolchain    mutants.Toolchain
	Fingerprints mutants.Fingerprints
	Reach        mutants.Reach
	Tree         selport.Tree
	Surroundings langport.Surroundings
}

func (e Environment) Plan(ctx context.Context, r mutants.FreshRequest) (*mutants.FreshPlan, mutants.FreshFailure) {
	if err := context.Cause(ctx); err != nil {
		return nil, mutants.FreshCancelled{Cause: err}
	}
	if e.Command == nil || e.Tree == nil || e.Surroundings == nil {
		return nil, mutants.FreshFailed{Cause: errors.New("fresh planning bindings are missing")}
	}
	p, err := e.plan(ctx, r.Root, r.Paths, r.Since, r.Count, r.Seed)
	return p, failure(ctx, "", err)
}
func (e Environment) Prepare(ctx context.Context, p *mutants.FreshPlan, scope covrun.Scope) (*mutants.FreshPreparation, mutants.FreshFailure) {
	if err := context.Cause(ctx); err != nil {
		return nil, mutants.FreshCancelled{Cause: err}
	}
	if e.Execute == nil || e.Toolchain == nil || e.Fingerprints == nil || e.Reach == nil || e.Tree == nil || e.Surroundings == nil {
		return nil, mutants.FreshFailed{Cause: errors.New("fresh preparation bindings are missing")}
	}
	prep, err := e.prepare(ctx, p, scope)
	stage := ""
	if prep != nil && len(prep.Stages) > 0 {
		stage = prep.Stages[len(prep.Stages)-1].Name
	}
	return prep, failure(ctx, stage, err)
}
func (e Environment) Release(_ context.Context, p *mutants.FreshPlan) { _ = releasePlan(p) }

func failure(ctx context.Context, stage string, err error) mutants.FreshFailure {
	if err == nil {
		return nil
	}
	if cause := context.Cause(ctx); cause != nil {
		return mutants.FreshCancelled{Cause: cause}
	}
	var fresh mutants.FreshFailure
	if errors.As(err, &fresh) {
		return fresh
	}
	var invalid *config.InvalidError
	if errors.As(err, &invalid) {
		return mutants.FreshInvalid{Reason: invalid.Reason, Key: invalid.Key}
	}
	for _, refusal := range []struct {
		cause error
		kind  mutants.FreshRefusal
	}{
		{ErrFreshRepository, mutants.FreshRepository}, {ErrFreshSince, mutants.FreshSince}, {ErrFreshPath, mutants.FreshPath}, {ErrFreshScratch, mutants.FreshScratch}, {ErrFreshUnsupported, mutants.FreshUnsupported},
	} {
		if errors.Is(err, refusal.cause) {
			return mutants.FreshRefused{Kind: refusal.kind, Cause: err}
		}
	}
	return mutants.FreshFailed{Stage: stage, Cause: err}
}
func (e Environment) goCoverageEvidence(ctx context.Context, root string, unit mutants.FreshUnit, blocks []coverage.GoBlock, producer string, inputs map[string]string) (*mutants.GoCoverageEvidence, error) {
	file, err := selection.Parse(ctx, e.Tree, e.Surroundings, filepath.Join(root, filepath.FromSlash(unit.Path)))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return mutants.FreshGoCoverageEvidence(file, unit, blocks, producer, inputs)
}
