package domain_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	covport "github.com/donvargax/itos-cc/internal/coverage/port"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

func preparedFreshTrial(t *testing.T) (*porttest.Fresh, *mutants.FreshPlan, mutants.FreshTrial, mutants.FreshTrialOptions) {
	t.Helper()
	f, p := freshPython(t)
	prep, failure := f.Prepare(t.Context(), p, covport.AllTests)
	if failure != nil {
		t.Fatal(failure)
	}
	trial := mutants.FreshTrial{Candidate: p.Selected[0], Baseline: prep.Baselines[0]}
	opt := mutants.FreshTrialOptions{WorkerFactory: f.Commands, Tree: *f.Tree, Toolchain: f.Inputs, Workers: 1, Scope: mutants.ScopeAllTests, TimeoutFactor: 10}
	return f, p, trial, opt
}

func TestFreshTrialsReadActualPostPreparationBytes(t *testing.T) {
	f, p, trial, opt := preparedFreshTrial(t)
	path := filepath.Join(p.FrozenRoot, filepath.FromSlash(trial.Candidate.Path))
	rel, err := filepath.Rel(f.Tree.Dir, path)
	if err != nil {
		t.Fatal(err)
	}
	f.Tree.Contents[filepath.ToSlash(rel)] = "def value(x):\n    return x + 99\n"
	results, err := mutants.RunFreshTrials(t.Context(), p, []mutants.FreshTrial{trial}, opt)
	if err != nil || len(results) != 1 || results[0].Err == nil || !strings.Contains(results[0].Err.Error(), "no longer holds") || results[0].Outcome != "" {
		t.Fatalf("changed frozen site admitted: %+v %v", results, err)
	}
	if len(f.Commands.Environments) != 0 {
		t.Fatal("trial command entered before actual frozen-source validation")
	}
	if len(f.Commands.Opened) != 1 || f.Commands.Opened[0] != mutants.FrozenTrials || !f.Commands.Lifetimes[0].State().Released {
		t.Fatal("actual trial lifetime was not released")
	}
}

func TestFreshTrialsShareListedBaselineAndPreserveStageScope(t *testing.T) {
	f, p, trial, opt := preparedFreshTrial(t)
	trial.Reach = []string{"A"}
	command := trial.Baseline.Command
	facts := f.Commands.Tests[command.Dir][command.String()]
	relative, err := filepath.Rel(command.Root, filepath.Join(p.FrozenRoot, filepath.FromSlash(trial.Candidate.Path)))
	if err != nil {
		t.Fatal(err)
	}
	facts.Verdicts = []porttest.Verdict{{Path: filepath.ToSlash(relative), Site: trial.Candidate.Site, Outcome: mutants.Survived}}
	f.Commands.Tests[command.Dir][command.String()] = facts
	opt.Listed = &mutants.Listed{Root: p.FrozenRoot, Select: func([]string) string { return "select A" }}
	f.Commands.Tests[p.FrozenRoot] = map[string]porttest.Tests{"select A": {Elapsed: time.Millisecond}}
	results, err := mutants.RunFreshTrials(t.Context(), p, []mutants.FreshTrial{trial, trial}, opt)
	if err != nil || len(results) != 2 {
		t.Fatalf("trials: %+v %v", results, err)
	}
	for _, r := range results {
		if r.Err != nil || r.Outcome != mutants.Killed || r.Scope != mutants.ScopeListed || len(r.Tests) != 1 || r.Tests[0] != "A" || !r.RanOwn() || len(r.Stages) != 3 {
			t.Fatalf("stage judgments: %+v", r)
		}
	}
	if got := len(f.Commands.Environments); got != 5 {
		t.Fatalf("two own + one actual clean selection + two listed commands = %d", got)
	}
	if !f.Commands.Lifetimes[0].State().Released {
		t.Fatal("trial inputs not released")
	}
}

func TestARepeatedFreshTrialListsEveryRunsOutcome(t *testing.T) {
	for _, c := range []struct {
		name     string
		repeat   int
		cycle    []string
		outcomes []string
		flaky    bool
	}{
		{"once", 1, []string{mutants.Killed, mutants.Survived}, nil, false},
		{"not all alike", 2, []string{mutants.Killed, mutants.Survived}, []string{mutants.Killed, mutants.Survived}, true},
		{"alike", 3, []string{mutants.Killed, mutants.Timeout}, []string{mutants.Killed, mutants.Timeout, mutants.Killed}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, p, trial, opt := preparedFreshTrial(t)
			command := trial.Baseline.Command
			facts := f.Commands.Tests[command.Dir][command.String()]
			relative, err := filepath.Rel(command.Root, filepath.Join(p.FrozenRoot, filepath.FromSlash(trial.Candidate.Path)))
			if err != nil {
				t.Fatal(err)
			}
			facts.Elapsed = time.Millisecond
			facts.Verdicts = []porttest.Verdict{{Path: filepath.ToSlash(relative), Site: trial.Candidate.Site, Outcomes: c.cycle}}
			f.Commands.Tests[command.Dir][command.String()] = facts
			opt.Repeat = c.repeat
			results, err := mutants.RunFreshTrials(t.Context(), p, []mutants.FreshTrial{trial}, opt)
			if err != nil || len(results) != 1 {
				t.Fatalf("trials: %+v %v", results, err)
			}
			r := results[0]
			if r.Err != nil || r.Outcome != mutants.Killed || fmt.Sprint(r.Outcomes) != fmt.Sprint(c.outcomes) || (r.Outcomes == nil) != (c.outcomes == nil) || r.Flaky() != c.flaky {
				t.Errorf("repeat %d: outcome %q, outcomes %v, flaky %v; want killed first, outcomes %v, flaky %v", c.repeat, r.Outcome, r.Outcomes, r.Flaky(), c.outcomes, c.flaky)
			}
			// Each run takes a millisecond at least, a timeout more.
			if least := time.Duration(c.repeat) * time.Millisecond; r.Elapsed < least {
				t.Errorf("repeat %d took %s in all, want %s at least", c.repeat, r.Elapsed, least)
			}
		})
	}
}

func TestFreshTrialCloseRetainsCancellationAndPartialResults(t *testing.T) {
	f, p, trial, opt := preparedFreshTrial(t)
	ctx, stop := context.WithTimeout(t.Context(), 5*time.Second)
	defer stop()
	ctx, cancel := context.WithCancelCause(ctx)
	entered := make(chan porttest.Invocation, 1)
	cancelled := make(chan porttest.Invocation, 1)
	join := make(chan struct{})
	var joinOnce sync.Once
	releaseJoin := func() { joinOnce.Do(func() { close(join) }) }
	facts := f.Commands.Tests[trial.Baseline.Command.Dir][trial.Baseline.Command.String()]
	facts.Hold = &porttest.WorkGate{MutationsOnly: true, Entered: entered, Cancelled: cancelled, JoinAfterCancel: join}
	f.Commands.Tests[trial.Baseline.Command.Dir][trial.Baseline.Command.String()] = facts
	f.Commands.OnRelease = releasedAfterJoin
	releaseCause := errors.New("private trial input removal unavailable")
	f.Commands.ReleaseFailure = mutants.WorkerFailed{Cause: releaseCause}
	type completion struct {
		results []mutants.FreshTrialResult
		err     error
	}
	done := make(chan completion, 1)
	completed := false
	defer func() {
		cancel(errors.New("held trial fixture completed"))
		releaseJoin()
		if !completed {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("held trial fixture could not join during cleanup")
			}
		}
	}()
	go func() {
		r, err := mutants.RunFreshTrials(ctx, p, []mutants.FreshTrial{trial}, opt)
		done <- completion{r, err}
	}()
	entry := receiveInvocation(ctx, t, entered)
	if !entry.Changed {
		t.Fatal("held request did not apply actual selected mutant")
	}
	path := filepath.Join(p.FrozenRoot, filepath.FromSlash(trial.Candidate.Path))
	before := f.Inputs.Files[path]
	if got := entry.Files[path]; got == before || got != string(trial.Candidate.Site.Apply([]byte(before))) {
		t.Fatalf("held private copy did not contain the actual selected replacement: %q", got)
	}
	cause := errors.New("caller cancelled counted trials")
	cancel(cause)
	receiveInvocation(ctxWithoutCancellation(t), t, cancelled)
	if state := f.Commands.Lifetimes[0].State(); state.JoinReturned || state.Released {
		t.Fatalf("held waiter was not joined before release: %+v", state)
	}
	releaseJoin()
	select {
	case got := <-done:
		completed = true
		if !errors.Is(got.err, cause) || !errors.Is(got.err, releaseCause) || len(got.results) != 1 || got.results[0].Err == nil || got.results[0].Outcome != "" || len(got.results[0].Stages) != 1 {
			t.Fatalf("partial results/close/cancel facts: %+v %v", got.results, got.err)
		}
		if state := f.Commands.Lifetimes[0].State(); !state.JoinReturned || state.Released {
			t.Fatalf("failed release state: %+v", state)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("joined counted trial failed to return")
	}
}

func ctxWithoutCancellation(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}
