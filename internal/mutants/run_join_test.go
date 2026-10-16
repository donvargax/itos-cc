package mutants_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/internal/mutants/porttest"
	"github.com/donvargax/itos-cc/internal/selection"
	seltest "github.com/donvargax/itos-cc/internal/selection/port/porttest"
)

func releasedAfterJoin(state porttest.InputState) error {
	if !state.JoinReturned {
		return errors.New("input release preceded actual entered-worker join")
	}
	return nil
}
func TestRunInputReleaseOracleRejectsPrematureRelease(t *testing.T) {
	if err := releasedAfterJoin(porttest.InputState{}); err == nil {
		t.Fatal("release oracle accepted premature input removal")
	}
	if err := releasedAfterJoin(porttest.InputState{JoinReturned: true}); err != nil {
		t.Fatal(err)
	}
}
func refreshRunTree(f *runFixture) {
	relative := map[string]string{}
	for name, text := range f.files {
		rel, _ := filepath.Rel(f.root, name)
		relative[rel] = text
	}
	f.options.Tree = seltest.TreeOf(f.root, relative, nil)
}
func joinFixture(t *testing.T) *runFixture {
	t.Helper()
	f := newRunFixture(t)
	f.files[f.path] = "package board\nfunc Value(x,y int) bool {return x > y}\n"
	refreshRunTree(f)
	return f
}
func receiveInvocation(t *testing.T, ctx context.Context, ch <-chan porttest.Invocation) porttest.Invocation {
	t.Helper()
	select {
	case entry := <-ch:
		return entry
	case <-ctx.Done():
		t.Fatal("actual bound worker never reached fixture barrier", context.Cause(ctx))
		return porttest.Invocation{}
	}
}
func TestRunJoinsActualHeldWorkerBeforeReleasingInputs(t *testing.T) {
	for _, failFast := range []bool{false, true} {
		name := "caller cancellation"
		if failFast {
			name = "other worker fail-fast"
		}
		t.Run(name, func(t *testing.T) {
			f := joinFixture(t)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			entered := make(chan porttest.Invocation, 1)
			cancelled := make(chan porttest.Invocation, 1)
			join := make(chan struct{})
			var joinOnce sync.Once
			held := &porttest.WorkGate{MutationsOnly: true, Entered: entered, Cancelled: cancelled, JoinAfterCancel: join}
			for command, facts := range f.workers.Tests[f.root] {
				facts.Hold = held
				f.workers.Tests[f.root][command] = facts
			}
			f.workers.OnRelease = releasedAfterJoin
			files := []string{f.path}
			var triggerEntered chan porttest.Invocation
			releaseTrigger := make(chan struct{})
			var triggerOnce sync.Once
			if failFast {
				other := filepath.Join(f.root, "other", "other.go")
				f.files[other] = "package other\nfunc Other(x,y int) bool {return x > y}\n"
				refreshRunTree(f)
				files = append(files, other)
				f.options.Workers = 2
				command, failure := mutants.ObserveCommand(ctx, f.inputs, mutants.CommandRequest{Path: other})
				if failure != nil {
					t.Fatal(failure)
				}
				parsed, err := selection.Parse(ctx, f.options.Tree, f.inputs, other)
				if err != nil {
					t.Fatal(err)
				}
				defer parsed.Close()
				sites := mutants.Sites(parsed)
				if len(sites) != 1 {
					t.Fatalf("actual trigger fixture sites: %v", sites)
				}
				triggerEntered = make(chan porttest.Invocation, 1)
				f.workers.Tests[f.root][command.String()] = porttest.Tests{Verdicts: []porttest.Verdict{{Path: "other/other.go", Site: sites[0], Outcome: mutants.Survived}}, Hold: &porttest.WorkGate{MutationsOnly: true, Entered: triggerEntered, Release: releaseTrigger}}
			}
			type outcome struct {
				results []mutants.FileResult
				stop    *mutants.Stop
				err     error
			}
			done := make(chan outcome, 1)
			go func() {
				if failFast {
					r, s, err := mutants.RunFailFast(ctx, files, f.options, mutants.FailFast{})
					done <- outcome{r, s, err}
				} else {
					r, err := mutants.Run(ctx, files, f.options)
					done <- outcome{results: r, err: err}
				}
			}()
			received := false
			defer func() {
				cancel()
				triggerOnce.Do(func() { close(releaseTrigger) })
				joinOnce.Do(func() { close(join) })
				if !received {
					cleanup, stop := context.WithTimeout(t.Context(), 5*time.Second)
					defer stop()
					select {
					case <-done:
					case <-cleanup.Done():
						t.Error("owned run did not join during fixture cleanup")
					}
				}
			}()
			actual := receiveInvocation(t, ctx, entered)
			if !actual.Changed || actual.Files[f.path] == f.files[f.path] {
				t.Fatal("barrier did not hold an actual mutated virtual source")
			}
			if failFast {
				receiveInvocation(t, ctx, triggerEntered)
				triggerOnce.Do(func() { close(releaseTrigger) })
			} else {
				cancel()
			}
			receiveInvocation(t, ctxWithoutOriginalDeadline(t), cancelled)
			if len(f.workers.Lifetimes) != 1 || f.workers.Lifetimes[0].State().Released {
				t.Fatal("held actual inputs were released")
			}
			joinOnce.Do(func() { close(join) })
			cleanup, stop := context.WithTimeout(t.Context(), 5*time.Second)
			defer stop()
			var got outcome
			select {
			case got = <-done:
				received = true
			case <-cleanup.Done():
				t.Fatal("run did not return after actual waiter joined")
			}
			if failFast {
				if got.err != nil || got.stop == nil || got.stop.Rule != "mutation.survived" {
					t.Fatalf("actual fail-fast: %+v %v", got.stop, got.err)
				}
			} else if !errors.Is(got.err, context.Canceled) {
				t.Fatalf("caller cancellation: %v", got.err)
			}
			state := f.workers.Lifetimes[0].State()
			if err := releasedAfterJoin(state); err != nil || !state.Released {
				t.Fatalf("actual release state: %+v %v", state, err)
			}
			if !failFast && len(f.snapshots.Written) != 0 {
				t.Fatalf("cancelled caller published: %v", f.snapshots.Written)
			}
		})
	}
}
func ctxWithoutOriginalDeadline(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}
func TestRunWorkerFailurePreventsPublication(t *testing.T) {
	f := newRunFixture(t)
	f.options.Annotate = true
	// Parsing sees actual selected bytes, but this independent worker tree lacks
	// the source. Baseline can enter; applying its actual mutation must fail.
	copies := map[string]string{}
	for path, text := range f.files {
		if path != f.path {
			copies[path] = text
		}
	}
	f.workers.Files = copies
	_, err := mutants.Run(t.Context(), []string{f.path}, f.options)
	var failure mutants.WorkerFailed
	if !errors.As(err, &failure) || len(f.snapshots.Written) != 0 || len(f.edits.Writes) != 0 {
		t.Fatalf("actual worker failure/publication: %v writes%v edits%v", err, f.snapshots.Written, f.edits.Writes)
	}
	if len(f.workers.Lifetimes) != 1 || !f.workers.Lifetimes[0].State().Released {
		t.Fatal(fmt.Sprint("worker failure did not release joined inputs", f.workers.Lifetimes))
	}
}
