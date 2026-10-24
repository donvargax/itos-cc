package domain

import (
	"context"
	"errors"
	"slices"
	"testing"
)

// projectPool is a WorkerPool stub whose run in the project gives run and
// failure; it holds no worker.
type projectPool struct {
	WorkerPool
	run     ProjectRun
	failure WorkerFailure
}

func (p projectPool) InProject(context.Context, Command) (ProjectRun, WorkerFailure) {
	return p.run, p.failure
}

// TestCopyOnlyKeepsWhatTheCopyLeftOut: a baseline that passes in the
// project keeps the tree the copy held and what it left out (slice-43); a
// run in the project that the caller stopped decides nothing, and one that
// failed short of the tests fails the run.
func TestCopyOnlyKeepsWhatTheCopyLeftOut(t *testing.T) {
	passed := ProjectRun{Result: CommandResult{Passed: true}, Copied: "/p", LeftOut: []string{"gen/"}}
	there, failure := copyOnly(t.Context(), projectPool{run: passed}, Command{}, nil)
	if failure != nil || there.Copied != "/p" || !slices.Equal(there.LeftOut, []string{"gen/"}) {
		t.Errorf("a baseline that passes in the project: %+v %v", there, failure)
	}
	stopped, cancel := context.WithCancel(t.Context())
	cancel()
	there, failure = copyOnly(stopped, projectPool{run: passed, failure: WorkerCancelled{Cause: context.Canceled}}, Command{}, nil)
	if failure != nil || there.Copied != "" {
		t.Errorf("a stopped run in the project: %+v %v", there, failure)
	}
	broken := WorkerFailed{Cause: errors.New("no shell")}
	if there, failure = copyOnly(t.Context(), projectPool{run: passed, failure: broken}, Command{}, nil); failure != broken || there.Copied != "" {
		t.Errorf("a run in the project that failed: %+v %v", there, failure)
	}
}
