package porttest

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// WorkersContract exercises observable outcomes/lifetimes both implementations
// hold. open creates the file source.txt and a real executable fixture (adapter)
// or the same command declaration (fake). Native containment stays adapter-only.
// No source writes here touch the repository; all mutations are fixture inputs.
func WorkersContract(t *testing.T, open func(t *testing.T, baseline string) (mutants.Workers, mutants.Command)) {
	t.Helper()
	t.Run("baseline mutant restoration and closed pool", func(t *testing.T) {
		factory, c := open(t, "")
		p, failure := factory.Open(t.Context(), mutants.MutationRun, 1)
		if failure != nil {
			t.Fatal(failure)
		}
		t.Cleanup(func() {
			if f := p.Close(t.Context()); f != nil {
				t.Error(f)
			}
		})
		w := p.Worker(0)
		clean, f := w.Run(t.Context(), c, nil, mutants.Limits{})
		if f != nil || !clean.Passed || clean.ExitCode != 0 || clean.TimedOut || clean.Cancelled {
			t.Fatalf("clean: %+v %v", clean, f)
		}
		change := &mutants.Mutation{Path: c.Root + "/source.txt", Source: []byte("original\n"), Site: mutants.Site{Start: 0, End: 8, Line: 1, Original: "original", Replacement: "mutated"}}
		trial, f := w.Run(t.Context(), c, change, mutants.Limits{})
		if f != nil || trial.Passed || trial.ExitCode != 1 || trial.TimedOut || trial.Cancelled {
			t.Fatalf("trial: %+v %v", trial, f)
		}
		restored, f := w.Run(t.Context(), c, nil, mutants.Limits{})
		if f != nil || !restored.Passed {
			t.Fatalf("restored: %+v %v", restored, f)
		}
		if f := p.Close(t.Context()); f != nil {
			t.Fatal(f)
		}
		_, f = w.Run(t.Context(), c, nil, mutants.Limits{})
		var failed mutants.WorkerFailed
		if !errors.As(f, &failed) {
			t.Fatalf("closed pool: %v", f)
		}
	})
	t.Run("failing baseline is a command result", func(t *testing.T) {
		factory, c := open(t, "baseline-failed")
		p, f := factory.Open(t.Context(), mutants.MutationRun, 1)
		if f != nil {
			t.Fatal(f)
		}
		defer closePool(t.Context(), t, p)
		r, f := p.Worker(0).Run(t.Context(), c, nil, mutants.Limits{})
		if f != nil || r.Passed || r.ExitCode != 1 || r.Output != "baseline-failed" {
			t.Fatalf("baseline: %+v %v", r, f)
		}
	})

	t.Run("a run in the project gives its result there and names the tree a copy holds", func(t *testing.T) {
		for _, baseline := range []string{"", "baseline-failed"} {
			factory, c := open(t, baseline)
			p, f := factory.Open(t.Context(), mutants.MutationRun, 1)
			if f != nil {
				t.Fatal(f)
			}
			run, f := p.InProject(t.Context(), c)
			if f != nil || run.Result.Passed != (baseline == "") || (baseline != "" && run.Result.Output != baseline) || run.Copied != filepath.Clean(c.Root) {
				t.Errorf("in the project, baseline %q: %+v %v", baseline, run, f)
			}
			ctx, cancel := context.WithCancelCause(t.Context())
			cause := errors.New("caller stopped the run in the project")
			cancel(cause)
			_, f = p.InProject(ctx, c)
			var cancelled mutants.WorkerCancelled
			if !errors.As(f, &cancelled) || !errors.Is(f, cause) {
				t.Errorf("InProject cancelled: %v", f)
			}
			if f := p.Close(t.Context()); f != nil {
				t.Fatal(f)
			}
			_, f = p.InProject(t.Context(), c)
			var failed mutants.WorkerFailed
			if !errors.As(f, &failed) {
				t.Errorf("InProject on a closed pool: %v", f)
			}
		}
	})

	t.Run("unavailable command is a typed setup failure", func(t *testing.T) {
		factory, c := open(t, "")
		p, f := factory.Open(t.Context(), mutants.MutationRun, 1)
		if f != nil {
			t.Fatal(f)
		}
		defer closePool(t.Context(), t, p)
		c.Args = []string{filepath.Join(c.Root, "missing-command")}
		c.Shell = ""
		_, f = p.Worker(0).Run(t.Context(), c, nil, mutants.Limits{})
		var failed mutants.WorkerFailed
		if !errors.As(f, &failed) {
			t.Fatalf("setup became a judgment: %v", f)
		}
	})
	t.Run("a copy readied once runs its commands; one that cannot be readied is a setup failure", func(t *testing.T) {
		factory, c := open(t, "")
		p, f := factory.Open(t.Context(), mutants.MutationRun, 1)
		if f != nil {
			t.Fatal(f)
		}
		defer closePool(t.Context(), t, p)
		c.Ready = c.Args
		for range 2 {
			if r, f := p.Worker(0).Run(t.Context(), c, nil, mutants.Limits{}); f != nil || !r.Passed {
				t.Fatalf("readied copy: %+v %v", r, f)
			}
		}
		q, f := factory.Open(t.Context(), mutants.MutationRun, 1)
		if f != nil {
			t.Fatal(f)
		}
		defer closePool(t.Context(), t, q)
		c.Ready = []string{filepath.Join(c.Root, "missing-command")}
		_, f = q.Worker(0).Run(t.Context(), c, nil, mutants.Limits{})
		var failed mutants.WorkerFailed
		if !errors.As(f, &failed) {
			t.Fatalf("unready copy became a judgment: %v", f)
		}
	})
	t.Run("caller cancellation forwarded before any work", func(t *testing.T) {
		factory, c := open(t, "")
		ctx, cancel := context.WithCancelCause(t.Context())
		cause := errors.New("caller stopped workers")
		cancel(cause)
		_, f := factory.Open(ctx, mutants.MutationRun, 1)
		var cancelled mutants.WorkerCancelled
		if !errors.As(f, &cancelled) || !errors.Is(f, cause) {
			t.Fatalf("Open cancelled: %v", f)
		}
		p, f := factory.Open(t.Context(), mutants.MutationRun, 1)
		if f != nil {
			t.Fatal(f)
		}
		defer closePool(ctx, t, p)
		_, f = p.Worker(0).Run(ctx, c, nil, mutants.Limits{})
		if !errors.As(f, &cancelled) || !errors.Is(f, cause) {
			t.Fatalf("Run cancelled: %v", f)
		}
	})
}

func closePool(ctx context.Context, t *testing.T, p mutants.WorkerPool) {
	t.Helper()
	if f := p.Close(ctx); f != nil {
		t.Error(f)
	}
}
