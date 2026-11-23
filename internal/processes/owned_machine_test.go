package processes

import (
	"context"
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestOwnedCommandTransitionsMatchTheDocument holds the code's transition
// table to docs/state-machines.md's: the same events, states and targets.
func TestOwnedCommandTransitionsMatchTheDocument(t *testing.T) {
	text, err := os.ReadFile("../../docs/state-machines.md")
	if err != nil {
		t.Fatal(err)
	}
	_, after, ok := strings.Cut(string(text), "<!-- transitions -->")
	if !ok {
		t.Fatal("docs/state-machines.md has no <!-- transitions --> table")
	}
	table, _, ok := strings.Cut(after, "<!-- /transitions -->")
	if !ok {
		t.Fatal("docs/state-machines.md's transitions table is not closed")
	}
	var documented []string
	for _, line := range strings.Split(table, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "| ") && !strings.HasPrefix(line, "| event ") {
			documented = append(documented, line)
		}
	}
	code := ownedTable()
	if len(documented) != len(code) {
		t.Fatalf("the document lists %d transitions, the code %d:\ndocument:\n%s\ncode:\n%s",
			len(documented), len(code), strings.Join(documented, "\n"), strings.Join(code, "\n"))
	}
	sortedDocumented := append([]string(nil), documented...)
	sortLines(sortedDocumented)
	for i := range code {
		if sortedDocumented[i] != code[i] {
			t.Errorf("the document has\n  %s\nwhere the code has\n  %s", sortedDocumented[i], code[i])
		}
	}
}

func sortLines(lines []string) {
	for i := 1; i < len(lines); i++ {
		for j := i; j > 0 && lines[j] < lines[j-1]; j-- {
			lines[j], lines[j-1] = lines[j-1], lines[j]
		}
	}
}

// TestOwnedCommandNextIsATable holds next to its table: over every state
// and event, one row at most applies, an event with no row is no
// transition, and a row sets exactly what it names.
func TestOwnedCommandNextIsATable(t *testing.T) {
	for _, run := range []ownedRun{runWaiting, runCleaning, runReturned} {
		for _, ctx := range []ownedContext{contextLive, contextByParent, contextByTimeout} {
			for _, deadline := range []ownedDeadline{deadlineBefore, deadlinePast} {
				for _, group := range []ownedGroup{groupUnknown, groupLive, groupGone} {
					state := ownedState{run, ctx, deadline, group}
					for _, event := range allOwnedEvents() {
						matching := 0
						for _, row := range ownedTransitions {
							if row.event == event && row.when.holds(state) {
								matching++
							}
						}
						got, ok := next(state, event)
						if matching > 1 {
							t.Errorf("%v: %q has %d rows", state, event, matching)
						}
						if ok != (matching == 1) {
							t.Errorf("%v: %q allowed = %v with %d rows", state, event, ok, matching)
						}
						if !ok && got != state {
							t.Errorf("%v: %q is no transition but moved the state to %v", state, event, got)
						}
					}
				}
			}
		}
	}
	got, ok := next(ownedStart, eventWaitReturns)
	if want := (ownedState{runCleaning, contextLive, deadlineBefore, groupUnknown}); !ok || got != want {
		t.Errorf("next(start, Wait reaps the leader) = %v, %v; want %v", got, ok, want)
	}
	if _, ok := next(got, eventWaitReturns); ok {
		t.Error("Wait reaped the leader twice")
	}
	if _, ok := next(ownedStart, eventCleanupSucceeds); ok {
		t.Error("a cleanup succeeded before its leader was reaped")
	}
}

func allOwnedEvents() []ownedEvent {
	return []ownedEvent{eventContextEndsByParent, eventContextEndsByTimeout, eventWaitReturns, eventGroupFoundLive,
		eventGroupFoundGone, eventDeadlinePasses, eventCleanupSucceeds, eventCleanupFails}
}

// cleaningLife is a run whose leader was reaped, as a cleanup meets it.
func cleaningLife() *ownedLife {
	life := newOwnedLife()
	_ = life.apply(eventWaitReturns)
	return life
}

// TestWaitOwnedProcessGroupMovesTheRunByWhatTheProbeFinds holds the
// cleanup's wait to its events: a group the probe finds gone leaves the run
// with it gone, and one that stays live to its deadline leaves it live, with
// its deadline past, and fails.
func TestWaitOwnedProcessGroupMovesTheRunByWhatTheProbeFinds(t *testing.T) {
	now := time.Now()
	for _, c := range []struct {
		name      string
		probe     error
		wantError bool
		want      ownedState
	}{
		{"gone", syscall.ESRCH, false, ownedState{runCleaning, contextLive, deadlineBefore, groupGone}},
		{"live to its deadline", syscall.EPERM, true, ownedState{runCleaning, contextLive, deadlinePast, groupLive}},
	} {
		t.Run(c.name, func(t *testing.T) {
			calls := systemGroupCalls()
			calls.signal = func(int, syscall.Signal) error { return c.probe }
			calls.runnable = func(int) (bool, error) { return true, nil }
			calls.now = func() time.Time { return now }
			calls.sleep = func(d time.Duration) { now = now.Add(d) }
			life := cleaningLife()
			err := waitOwnedProcessGroup(calls, life, 4242, now.Add(20*time.Millisecond), "linux")
			if (err != nil) != c.wantError {
				t.Fatalf("error = %v; want one: %v", err, c.wantError)
			}
			if life.current() != c.want {
				t.Errorf("the run is %v; want %v", life.current(), c.want)
			}
		})
	}
}

// TestObserveContextTakesTheRunThroughTheContextsEnd holds what the run
// says of its context: live while it is, ended by its parent or by its
// timeout (the cause ErrCommandTimeout) once it ended, and ended that way
// for good.
func TestObserveContextTakesTheRunThroughTheContextsEnd(t *testing.T) {
	for _, c := range []struct {
		name  string
		cause error
		want  ownedContext
	}{
		{"live", nil, contextLive},
		{"its parent's cancel", errors.New("stop"), contextByParent},
		{"its timeout", ErrCommandTimeout, contextByTimeout},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(context.Background())
			defer cancel(nil)
			life := newOwnedLife()
			if c.cause != nil {
				cancel(c.cause)
			}
			if got := life.observeContext(ctx); got != c.want {
				t.Errorf("the context stands %q; want %q", got, c.want)
			}
			if got := life.observeContext(ctx); got != c.want {
				t.Errorf("seen again, the context stands %q; want %q", got, c.want)
			}
			if life.current().context != c.want {
				t.Errorf("the run holds %q; want %q", life.current().context, c.want)
			}
		})
	}
}

// TestOwnedLifeRefusesAnUnlistedEvent holds a run to its table without a
// panic: next says an unlisted event is none, and apply answers a
// TransitionError naming the event and the state, leaves the state where it
// was, and answers the same error to whatever comes after.
func TestOwnedLifeRefusesAnUnlistedEvent(t *testing.T) {
	if got, ok := next(ownedStart, eventCleanupSucceeds); ok || got != ownedStart {
		t.Errorf("next(start, the cleanup succeeds) = %v, %v; want no transition and the state as it was", got, ok)
	}
	life := newOwnedLife()
	err := life.apply(eventCleanupSucceeds)
	var bad *TransitionError
	if !errors.As(err, &bad) {
		t.Fatalf("apply = %v; want a TransitionError", err)
	}
	for _, want := range []string{`"the cleanup succeeds"`, "run=waiting", "context=live", "deadline=before", "group=unknown"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error %q does not name %s", err, want)
		}
	}
	if life.current() != ownedStart {
		t.Errorf("the state moved to %v", life.current())
	}
	if again := life.apply(eventWaitReturns); again != err || life.current() != ownedStart || life.failure() != err {
		t.Errorf("after its first failure a run answered %v, in %v; want the first error and no move", again, life.current())
	}
}
