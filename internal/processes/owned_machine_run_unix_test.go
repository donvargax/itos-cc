//go:build linux || darwin

package processes

import (
	"errors"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// without is the table less every row of events: a table that lacks a
// transition the run meets.
func without(events ...ownedEvent) []ownedRow {
	var rows []ownedRow
rows:
	for _, row := range ownedTransitions {
		for _, event := range events {
			if row.event == event {
				continue rows
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// ownedRunWith runs a command that exits 0 through runOwnedCommand, as one
// whose group stays live past its deadline when live is set, with life as
// its machine.
func ownedRunWith(t *testing.T, life *ownedLife, live bool) error {
	t.Helper()
	calls := systemGroupCalls()
	if live {
		clock := time.Now().Add(time.Hour)
		calls.signal = func(int, syscall.Signal) error { return nil }
		calls.runnable = func(int) (bool, error) { return true, nil }
		calls.now = func() time.Time { return clock }
		calls.sleep = func(d time.Duration) { clock = clock.Add(d) }
	}
	s := &supervision{kept: &keptOutput{}, platform: "linux", calls: calls, life: life}
	return runOwnedCommand(t.Context(), exec.CommandContext(t.Context(), "/bin/sh", "-c", "exit 0"), s, nil, nil, nil)
}

// TestOwnedRunEndsWithTheTransitionItLacked holds a run whose table lacks a
// transition it meets to end with a TransitionError naming the event, whole,
// not wrapped as a cleanup's or a command's failure, and never a panic.
func TestOwnedRunEndsWithTheTransitionItLacked(t *testing.T) {
	for _, c := range []struct {
		lacks ownedEvent
		live  bool
	}{
		{eventWaitReturns, false},
		{eventGroupFoundGone, false},
		{eventCleanupSucceeds, false},
		{eventGroupFoundLive, true},
		{eventDeadlinePasses, true},
		{eventCleanupFails, true},
	} {
		t.Run(string(c.lacks), func(t *testing.T) {
			life := &ownedLife{state: ownedStart, table: without(c.lacks)}
			err := ownedRunWith(t, life, c.live)
			var bad *TransitionError
			if !errors.As(err, &bad) || bad.Event != string(c.lacks) || err != error(bad) {
				t.Fatalf("error = %v; want a TransitionError for %q, not wrapped", err, c.lacks)
			}
		})
	}
}

// TestOwnedRunMovesThroughItsTable holds the state a run ends in: returned
// with its group gone for a command whose group ended, and returned with its
// group live and its deadline past for one whose group stayed live, the
// cleanup failed.
func TestOwnedRunMovesThroughItsTable(t *testing.T) {
	life := newOwnedLife()
	if err := ownedRunWith(t, life, false); err != nil {
		t.Fatal(err)
	}
	if want := (ownedState{runReturned, contextLive, deadlineBefore, groupGone}); life.current() != want {
		t.Errorf("a command whose group ended ends in %v; want %v", life.current(), want)
	}
	life = newOwnedLife()
	var cleanup *CleanupFailed
	if err := ownedRunWith(t, life, true); !errors.As(err, &cleanup) {
		t.Fatalf("error = %v; want a CleanupFailed", err)
	}
	if want := (ownedState{runReturned, contextLive, deadlinePast, groupLive}); life.current() != want {
		t.Errorf("a command whose group stayed live ends in %v; want %v", life.current(), want)
	}
}
