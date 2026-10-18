package features

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

func (q *countedMutation) interruptSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a count-two selection has one completed judgment and one active judgment$`, q.interruptProject)
	sc.Step(`^the counted run is (interrupted|sent SIGTERM) while the second judgment is active$`, q.interruptRun)
	sc.Step(`^the completed result remains visible$`, func() error { return q.e.completedFirst() })
	sc.Step(`^the unfinished judgment is cancelled or undecided, not a killed or timed-out mutant$`, func() error { return q.e.cancelOutcome() })
	sc.Step(`^no further command is admitted after the run abort$`, q.noCommandAfterAbort)
	sc.Step(`^after the interrupted run returns, its owned fixture peers have terminated and private inputs are removed$`, q.interruptedRemoved)
	sc.Step(`^one shared five-second post-abort cleanup deadline applies without killing unrelated processes$`, q.interruptBound)
}
func (q *countedMutation) interruptProject(ctx context.Context) error {
	if err := q.e.m.w.d.Counted(); err != nil {
		return err
	}
	if err := q.e.ownedProject(ctx, "cancel", false, true); err != nil {
		return err
	}
	// Read the live private-input inventory only while the owned later command
	// is held at its actual readiness barrier. This is not a removal-order probe.
	files, err := q.e.m.w.d.Files(ctx, filepath.Join(q.e.m.w.dir, ".git", "itos"))
	if err != nil {
		return err
	}
	roots := map[string]bool{}
	for _, file := range files {
		part := strings.Split(file, "/")[0]
		if strings.HasPrefix(part, "fresh-plan-") {
			roots[part] = true
		}
	}
	if len(roots) != 1 {
		return fmt.Errorf("active counted private-input root: %v", roots)
	}
	for root := range roots {
		q.worktreeParent = filepath.Join(q.e.m.w.dir, ".git", "itos", root)
	}
	if err = q.e.unrelated(ctx); err != nil {
		return err
	}
	return nil
}
func (q *countedMutation) interruptRun(ctx context.Context, how string) error {
	send := Running.Interrupt
	if how == "sent SIGTERM" {
		send = Running.Terminate
	}
	var err error
	q.traceBefore, err = q.e.m.w.d.Read(filepath.Join(q.e.outside, "trace.jsonl"))
	if err != nil {
		return err
	}
	if err := q.e.signalParent(ctx, send); err != nil {
		return err
	}
	q.primary = countedObservation{object: q.e.result, result: Result{Exit: q.e.m.w.exit, Stdout: q.e.m.w.stdout, Stderr: q.e.m.w.stderr}}
	return nil
}
func (q *countedMutation) noCommandAfterAbort() error {
	if err := q.e.cancelOutcome(); err != nil {
		return err
	}
	current, err := q.e.m.w.d.Read(filepath.Join(q.e.outside, "trace.jsonl"))
	if err != nil {
		return err
	}
	if string(current) != string(q.traceBefore) {
		return fmt.Errorf("new fixture command after abort: %s", current)
	}
	if strings.Count(q.primary.result.Stderr, "itos-cc: [") != 2 || strings.Count(q.primary.result.Stderr, "not judged: cancelled") != 1 {
		return fmt.Errorf("interrupted completed progress population: %s", q.primary.result.Stderr)
	}
	return nil
}
func (q *countedMutation) interruptedRemoved(ctx context.Context) error {
	if err := q.e.ownedRemoved(ctx); err != nil {
		return err
	}
	if q.worktreeParent == "" || q.e.m.w.d.Exists(q.worktreeParent) {
		return fmt.Errorf("private frozen inputs remain after interrupted return: %s", q.worktreeParent)
	}
	return nil
}
func (q *countedMutation) interruptBound(ctx context.Context) error {
	if q.e.owned.joinedAt.Before(q.e.owned.released) || q.e.owned.joinedAt.Sub(q.e.owned.released) > 5*time.Second {
		return fmt.Errorf("counted cancellation exceeded one shared deadline")
	}
	if err := q.e.ownedEnded(ctx); err != nil {
		return err
	}
	return q.e.unrelatedLive(ctx)
}
