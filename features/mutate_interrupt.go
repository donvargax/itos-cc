package features

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// The steps of an interrupt that stops a mutation run (decision 38): a
// complete or counted run signalled while it judges, and an owned group
// whose cleanup cannot finish within the shared five-second deadline, which
// the ownership fixture's keeper makes. They share one mutationExecution
// with the ownership and fail-fast steps (mutateState.execution), whose
// sentences they reuse.

// interruptIDs are the scenarios whose steps the counted and execution
// steps share one state for.
var interruptIDs = []string{"249", "250", "251", "252", "253"}

func (q *countedMutation) runInterruptSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a complete run with one worker has one completed judgment and one active judgment$`, func(ctx context.Context) error {
		return q.e.completeOwned(ctx, false, true)
	})
	sc.Step(`^the complete run is interrupted while the second judgment is active$`, func(ctx context.Context) error {
		return q.e.stopOwned(ctx, false)
	})
	sc.Step(`^two workers, one judging a mutant whose owned cleanup cannot finish within the shared five-second deadline$`, func(ctx context.Context) error {
		if q.e.owned == nil {
			q.e.owned = &ownershipExecution{}
		}
		q.e.owned.keep = "Compare"
		if err := q.e.concurrentOwned(ctx); err != nil {
			return err
		}
		return q.e.captureKeeper(ctx, "Compare")
	})
	sc.Step(`^a (complete run|counted run) is judging a mutant whose owned cleanup cannot finish within the shared five-second deadline$`, func(ctx context.Context, run string) error {
		if run == "complete run" {
			return q.e.completeOwned(ctx, true, false)
		}
		return q.countedKept(ctx)
	})
	sc.Step(`^the run is interrupted while that judgment is active$`, func(ctx context.Context) error {
		return q.e.stopOwned(ctx, false)
	})
	sc.Step(`^the run is interrupted, and interrupted again once its owned processes have been killed$`, func(ctx context.Context) error {
		return q.e.stopOwned(ctx, true)
	})
	sc.Step(`^the judgment whose cleanup failed is cancelled, with no killed, timed-out or survived outcome$`, func() error { return q.e.keptCancelled() })
	sc.Step(`^the mutation\.cleanup-failed problem names the owned process group and its command$`, func() error { return q.e.cleanupProblem() })
	sc.Step(`^it returns only once the shared five-second cleanup deadline has passed$`, func() error { return q.e.returnedAtDeadline() })
}

// completeOwned starts a complete run with one worker of the ownership
// fixture, its two mutants judged in site order: Compare's tests kill it,
// and Gate's command and its owned child and grandchild keep running until
// the run stops them. With kept, the keeper leaves Gate's owned group's
// cleanup unable to finish; with unrelated, another product run's owned
// processes run beside it, which the stop must leave alone.
func (q *mutationExecution) completeOwned(ctx context.Context, kept, unrelated bool) error {
	if !CountedOn(q.m.w.d.Platform()) {
		return fmt.Errorf("%w: Unix process ownership", godog.ErrSkip)
	}
	q.owned = &ownershipExecution{mode: "cancel", complete: true, order: []string{"Compare", "Gate"}, block: "Gate"}
	if kept {
		q.owned.keep = "Gate"
	}
	q.command = "go test -count=1 -failfast ."
	var err error
	if q.peers, err = q.m.w.d.FixturePeers(ctx); err != nil {
		return err
	}
	if err = q.installOwned("cancel", false, true); err != nil {
		return err
	}
	if err = q.writeControl("cancel", false, false, q.owned.block); err != nil {
		return err
	}
	if err = q.declareOwned("cancel", true, q.owned.block); err != nil {
		return err
	}
	q.before = map[string]string{"main.go": q.source["main.go"]}
	if unrelated {
		if err = q.unrelated(ctx); err != nil {
			return err
		}
	}
	q.owned.run, err = q.startOwned(ctx, "mutation", "run", "--json", "--workers", "1", "--no-coverage", "--timeout-factor", "10000", "--test-command", q.command)
	if err != nil {
		return err
	}
	if err = q.captureTree(ctx, q.owned.block); err != nil {
		return err
	}
	if kept {
		return q.captureKeeper(ctx, q.owned.block)
	}
	return nil
}

// countedKept starts a count-two run of the ownership fixture whose second
// judgment, in the run's own order, keeps its owned processes running, the
// keeper leaving its group's cleanup unable to finish.
func (q *countedMutation) countedKept(ctx context.Context) error {
	if err := q.e.m.w.d.Counted(); err != nil {
		return err
	}
	q.e.owned = &ownershipExecution{keepBlock: true}
	if err := q.e.ownedProject(ctx, "cancel", false, true); err != nil {
		return err
	}
	return q.e.captureKeeper(ctx, q.e.owned.block)
}

// captureKeeper waits for the keeper of name's judgment to be in place.
func (q *mutationExecution) captureKeeper(ctx context.Context, name string) error {
	ready, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if _, err := q.peers.Capture(ready, filepath.Join(q.outside, name+"-keeper.json")); err != nil {
		return fmt.Errorf("capture %s's keeper: %w", name, err)
	}
	return nil
}

// stopOwned interrupts the run, and with again interrupts it a second time
// once the three owned processes of its active judgment have ended, unless
// the run ended first, then waits for the run to return. A run the signal
// ended before it printed its object leaves no result for the Then steps.
func (q *mutationExecution) stopOwned(ctx context.Context, again bool) error {
	if q.owned == nil || q.owned.run == nil {
		return errors.New("no owned run is under way")
	}
	q.owned.released = time.Now()
	if err := q.owned.run.Interrupt(ctx); err != nil {
		return err
	}
	bounded, cancel := context.WithDeadline(ctx, q.owned.released.Add(20*time.Second))
	defer cancel()
	returned := make(chan time.Time, 1)
	var r Result
	var err error
	go func() {
		r, err = q.owned.run.Wait(bounded)
		returned <- time.Now()
	}()
	if again {
		killed := make(chan error, 1)
		go func() {
			var err error
			for _, peer := range q.owned.peers {
				err = errors.Join(err, peer.EOF(bounded))
			}
			killed <- err
		}()
		select {
		case at := <-returned:
			// It returned before its owned processes ended: nothing waits
			// for a second interrupt.
			returned <- at
		case err := <-killed:
			if err != nil {
				return fmt.Errorf("the owned processes did not end after the interrupt: %w", err)
			}
			q.owned.interrupted = time.Now()
			if err := q.owned.run.Interrupt(ctx); err != nil {
				return err
			}
		}
	}
	at := <-returned
	return q.returned(ctx, r, err, at)
}

// returned records the end of the owned run, which Wait gave r and err at
// at, for the Then steps: its exit, the signal that ended it, its output
// and its JSON object, nil when it printed none. A run that did not return
// within the step's bound is killed and read as never returning.
func (q *mutationExecution) returned(ctx context.Context, r Result, err error, at time.Time) error {
	w := q.m.w
	if errors.Is(err, context.DeadlineExceeded) {
		if err := q.owned.run.Kill(context.WithoutCancel(ctx)); err != nil {
			return err
		}
		stdout, stderr := q.owned.run.Output()
		w.exit, w.signal, w.stdout, w.stderr, w.last = -1, 0, stdout, stderr+"\n(the run did not return, and was killed)\n", nil
		q.result = nil
		return nil
	}
	if err != nil {
		return err
	}
	w.exit, w.signal, w.stdout, w.stderr, w.last = r.Exit, r.Signal, r.Stdout, r.Stderr, nil
	q.result = nil
	if object, err := w.json(); err == nil {
		q.result = object
	}
	// A run that reported has joined its owned processes, whose ends the
	// cleanup then reads; one the signal ended first joined none.
	q.owned.returned = q.result != nil
	q.owned.joinedAt = at
	return nil
}

// keptCancelled is whether the judgment whose cleanup failed has no
// outcome: a complete run's cancelled, a counted run's cancelled or
// undecided.
func (q *mutationExecution) keptCancelled() error {
	if q.result == nil {
		return fmt.Errorf("the run printed no JSON object\n%s", q.m.w.report())
	}
	name := q.owned.keep
	if q.owned.keepBlock {
		name = q.owned.block
	}
	if q.owned.keepBlock {
		mu, err := countedExecution(q.result, name)
		if err != nil {
			return err
		}
		if mu["outcome"] != nil || !slices.Contains([]any{"cancelled", "undecided"}, mu["state"]) {
			return fmt.Errorf("the judgment whose cleanup failed: %v", mu)
		}
		return nil
	}
	mu, err := executionMutant(q.result, "main.go", name)
	if err != nil {
		return err
	}
	if mu["state"] != "cancelled" || mu["outcome"] != nil {
		return fmt.Errorf("the judgment whose cleanup failed: %v", mu)
	}
	return nil
}

// cleanupProblem is whether the run reports one mutation.cleanup-failed
// problem, naming the process group of the kept judgment's command, as its
// fixture recorded it, and that command.
func (q *mutationExecution) cleanupProblem() error {
	if q.result == nil {
		return fmt.Errorf("the run printed no JSON object\n%s", q.m.w.report())
	}
	var found []map[string]any
	for _, p := range checkProblems(q.result) {
		if p["rule"] == "mutation.cleanup-failed" {
			found = append(found, p)
		}
	}
	if len(found) != 1 || len(q.owned.records) == 0 {
		return fmt.Errorf("mutation.cleanup-failed problems: %v", q.result)
	}
	p, group := found[0], q.owned.records[0].Group
	command := q.command
	if q.owned.keepBlock {
		// A counted run chooses its own command.
		command = "go test -count=1 -failfast ./."
	}
	if group <= 0 || p["process_group"] != float64(group) || !strings.Contains(fmt.Sprint(p["command"]), command) {
		return fmt.Errorf("mutation.cleanup-failed names group %v and command %v, not the owned group %d of %s: %v", p["process_group"], p["command"], group, command, p)
	}
	return nil
}

// returnedAtDeadline is whether the run, interrupted a second time, still
// returned only once the shared five-second deadline from the first
// interrupt had passed, and not long after.
func (q *mutationExecution) returnedAtDeadline() error {
	o := q.owned
	if o == nil || o.joinedAt.IsZero() || o.interrupted.IsZero() {
		return fmt.Errorf("the run was not interrupted twice before it returned\n%s", q.m.w.report())
	}
	took := o.joinedAt.Sub(o.released)
	if took < cleanupDeadline-100*time.Millisecond || took > 10*time.Second {
		return fmt.Errorf("the run returned %s after the first interrupt, not once the five-second deadline passed\n%s", took, q.m.w.report())
	}
	return nil
}

// interruptedOutcome is whether a complete run's unfinished judgment is
// cancelled, with no outcome.
func (q *mutationExecution) interruptedOutcome() error {
	if q.result == nil {
		return fmt.Errorf("the run printed no JSON object\n%s", q.m.w.report())
	}
	mu, err := executionMutant(q.result, "main.go", q.owned.block)
	if err != nil {
		return err
	}
	if mu["state"] != "cancelled" || mu["outcome"] != nil {
		return fmt.Errorf("the unfinished judgment: %v", mu)
	}
	return nil
}

// interruptedProof is whether an interrupted complete run kept the
// judgment it finished in the file's snapshot, with the scope that decided
// it and the evidence of the tests its freshness rests on.
func (q *mutationExecution) interruptedProof() error {
	done := q.owned.order[0]
	_, entries, err := q.recordFile("main.go", done)
	if err != nil {
		return err
	}
	if len(entries) != 1 || entries[0]["outcome"] != "killed" || entries[0]["scope"] != q.command {
		return fmt.Errorf("the finished judgment of %s: %v", done, entries)
	}
	tests, err := q.m.w.d.Read(filepath.Join(q.m.w.dir, "main_test.go"))
	if err != nil {
		return err
	}
	evidence := executionMap(executionMap(entries[0], "suite_evidence"), "tests")
	if evidence["main_test.go"] != fmt.Sprintf("%x", sha256.Sum256(tests)) {
		return fmt.Errorf("the finished judgment's freshness evidence: %v", entries[0])
	}
	return nil
}

// interruptedUnfinished is whether the judgment an interrupted complete
// run cancelled has no entry in the snapshot and no outcome in its report.
func (q *mutationExecution) interruptedUnfinished() error {
	_, entries, err := q.recordFile("main.go", q.owned.block)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return fmt.Errorf("the cancelled judgment got freshness proof: %v", entries)
	}
	return q.interruptedOutcome()
}
