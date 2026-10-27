package worktrees

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/donvargax/itos-cc/internal/processes"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// This is the private ordering contract behind MUT166/183/188/193. A real
// command has entered its completion waiter, while source restoration is still
// pending. Close must join it before releasing that command's input tree.
func TestPoolJoinsEnteredWaiterBeforeRemovingInputs(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.txt")
	if err := os.WriteFile(path, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(runnerFixtureMode, "success")
	opened, failure := (Workers{Platform: runtime.GOOS, TempDir: t.TempDir()}).Open(t.Context(), mutants.MutationRun, 1)
	if failure != nil {
		t.Fatal(failure)
	}
	p := opened.(*pool)
	w := p.workers[0]
	entered, release := make(chan struct{}), make(chan struct{})
	joinEntered := make(chan struct{})
	releaseObserved := make(chan bool, 1)
	var joinReturned atomic.Bool
	p.lifetime = inputLifetime{
		join:    func(wait func()) { close(joinEntered); wait(); joinReturned.Store(true) },
		release: func(remove func()) { releaseObserved <- joinReturned.Load(); remove() },
	}
	var once sync.Once
	w.worker.runner.Lifecycle.Complete = func(context.Context, *exec.Cmd, error) { close(entered); <-release }
	runDone := make(chan struct{})
	var result mutants.CommandResult
	var failed mutants.WorkerFailure
	go func() {
		defer close(runDone)
		result, failed = w.Run(t.Context(), mutants.Command{Root: root, Dir: root, Args: commandRunnerHelperArgs()}, &mutants.Mutation{Path: path, Source: []byte("original\n"), Site: mutants.Site{Start: 0, End: 8, Original: "original", Replacement: "mutated"}}, mutants.Limits{})
	}()
	t.Cleanup(func() {
		once.Do(func() { close(release) })
		<-runDone
		if f := p.Close(t.Context()); f != nil {
			t.Error(f)
		}
	})
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("command never entered completion waiter")
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	closeDone := make(chan struct{})
	go func() {
		defer close(closeDone)
		if f := p.Close(cancelled); f != nil {
			t.Error(f)
		}
	}()
	select {
	case <-joinEntered:
	case joined := <-releaseObserved:
		t.Fatalf("input release preceded the actual join callback: joined=%v", joined)
	case <-time.After(5 * time.Second):
		t.Fatal("owner never entered actual join callback")
	}
	// The real command completion/restoration is still held at this point.
	if _, err := os.Stat(p.scratch.dir); err != nil {
		t.Fatalf("inputs removed while waiter held: %v", err)
	}
	copy := w.worker.copies[root].dir
	text, err := os.ReadFile(filepath.Join(copy, "source.txt"))
	if err != nil || string(text) != "mutated\n" {
		t.Fatalf("pending restoration input: %q %v", text, err)
	}
	once.Do(func() { close(release) })
	select {
	case <-closeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not join after waiter release")
	}
	select {
	case joined := <-releaseObserved:
		if err := joinedBeforeRelease(joined); err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("actual release edge was not observed")
	}
	<-runDone
	if failed != nil || !result.Passed {
		t.Fatalf("command changed: %+v %v", result, failed)
	}
	if _, err := os.Stat(p.scratch.dir); !os.IsNotExist(err) {
		t.Fatalf("joined inputs remain: %v", err)
	}
	original, err := os.ReadFile(path)
	if err != nil || string(original) != "original\n" {
		t.Fatalf("original changed: %q %v", original, err)
	}
}

// Unlike a unit comparison of cleanupBudget.until, this observes the actual
// deadlines selected inside both bound worker command supervisors.
func TestBoundWorkersShareAbortDeadline(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("owned descendant supervision is deferred outside Linux/macOS")
	}
	for _, tc := range []struct {
		name                 string
		purpose              mutants.WorkPurpose
		abort, shared, owner bool
	}{
		{"explicit fail-fast stop", mutants.FailFastJudgments, true, true, false},
		{"external cancellation in fail-fast mode before a stop", mutants.FailFastJudgments, false, true, false},
		{"ordinary external cancellation", mutants.MutationRun, false, false, false},
		{"frozen trials and preparation owner", mutants.FrozenTrials, false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) { checkBoundWorkerBudgets(t, tc.purpose, tc.abort, tc.shared, tc.owner) })
	}
}
func checkBoundWorkerBudgets(t *testing.T, purpose mutants.WorkPurpose, abort, shared, withOwner bool) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	factory := Workers{Platform: runtime.GOOS, TempDir: t.TempDir()}
	if withOwner {
		factory.Owner = &processes.OwnedExecutor{Platform: runtime.GOOS}
	}
	opened, f := factory.Open(t.Context(), purpose, 2)
	if f != nil {
		t.Fatal(f)
	}
	p := opened.(*pool)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	results := make(chan mutants.CommandResult, 2)
	failures := make(chan mutants.WorkerFailure, 2)
	deadlines := make(chan time.Time, 2)
	for i, w := range p.workers {
		ready := filepath.Join(outside, string(rune('a'+i))+".ready")
		w.worker.prepare = func(cmd *exec.Cmd) {
			cmd.Env = append(cmd.Env, runnerFixtureMode+"=barrier", "ITOS_COMMAND_RUNNER_STARTED="+ready, "ITOS_COMMAND_RUNNER_RELEASE="+filepath.Join(outside, "never-released"), "ITOS_COMMAND_RUNNER_FINISHED="+filepath.Join(outside, "never-finished"))
		}
		w.worker.runner.Lifecycle.Cleanup = func(deadline time.Time) { deadlines <- deadline }
		go func(w mutants.Worker) {
			r, f := w.Run(ctx, mutants.Command{Root: root, Dir: root, Args: commandRunnerHelperArgs()}, nil, mutants.Limits{})
			results <- r
			failures <- f
		}(w)
	}
	t.Cleanup(func() {
		cancel()
		if f := p.Close(t.Context()); f != nil {
			t.Error(f)
		}
	})
	for _, name := range []string{"a.ready", "b.ready"} {
		if err := waitForFile(filepath.Join(outside, name), 3*time.Second); err != nil {
			t.Fatal(err)
		}
	}
	if abort {
		p.Abort()
	}
	cancel()
	var observed []time.Time
	for range 2 {
		select {
		case r := <-results:
			if !r.Cancelled || r.TimedOut || r.Passed {
				t.Errorf("abort became a judgment: %+v", r)
			}
		case <-time.After(6 * time.Second):
			t.Fatal("worker failed to join")
		}
		select {
		case f := <-failures:
			if f != nil {
				t.Fatal(f)
			}
		case <-time.After(time.Second):
			t.Fatal("worker result has no failure observation")
		}
		select {
		case d := <-deadlines:
			observed = append(observed, d)
		case <-time.After(time.Second):
			t.Fatal("bound supervisor did not observe its cleanup deadline")
		}
	}
	if shared {
		if !observed[0].Equal(observed[1]) || !observed[0].Equal(p.budget.Deadline()) {
			t.Fatalf("bound scopes got independent budgets: %v vs %v", observed, p.budget.Deadline())
		}
		if withOwner && p.budget != factory.Owner.Budget() {
			t.Fatal("frozen trials lost preparation's actual shared owner")
		}
	} else {
		if !p.budget.Deadline().IsZero() {
			t.Fatalf("ordinary aggregate cancellation activated shared deadline: %v", p.budget.Deadline())
		}
		if observed[0].IsZero() || observed[1].IsZero() {
			t.Fatal("local command cleanup had no actual deadline")
		}
	}
}

// The same release-edge oracle rejects a premature-release fixture directly,
// independent of whether the test goroutine happened to run before removal.
func TestInputLifetimeRejectsPrematureRelease(t *testing.T) {
	if err := joinedBeforeRelease(false); err == nil {
		t.Fatal("premature release fixture escaped actual release-edge oracle")
	}
	var joined bool
	life := inputLifetime{join: func(wait func()) { wait(); joined = true }, release: func(remove func()) {
		if err := joinedBeforeRelease(joined); err != nil {
			t.Error(err)
		}
		remove()
	}}
	waited, removed := false, false
	life.finish(func() { waited = true }, func() {
		if !waited {
			t.Error("input release skipped join")
		}
		removed = true
	})
	if !removed {
		t.Fatal("joined inputs were not released")
	}
}

// The actual Close guard and the premature-release fixture use this identical
// synchronous oracle at the input-release edge.
func joinedBeforeRelease(joined bool) error {
	if !joined {
		return errors.New("input release preceded actual join return")
	}
	return nil
}
