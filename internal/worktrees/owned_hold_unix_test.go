//go:build linux || darwin

package worktrees

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"sync"
	"testing"
	"time"
)

// holds counts the owned cleanups an executor marks under way, which an
// interrupt waits for (decision 38).
type holds struct {
	mu            sync.Mutex
	taken, active int
}

func (h *holds) hold() func() {
	h.mu.Lock()
	h.taken++
	h.active++
	h.mu.Unlock()
	return func() { h.mu.Lock(); h.active--; h.mu.Unlock() }
}

func (h *holds) counts() (taken, active int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.taken, h.active
}

// A command an abort cancels marks its owned group's kill and join under
// way from the cancel to the join, and no longer once it returns, so a
// second interrupt waits only for that. A command that ends on its own
// marks its brief cleanup too. What the scenarios cannot see is whether
// the mark spans the kill: they see only how long a second interrupt
// waits (ID-MUT-253).
func TestAnOwnedCleanupIsMarkedFromTheAbortToTheJoin(t *testing.T) {
	var h holds
	owner := &OwnedExecutor{Platform: platformAtBoundary(""), Hold: h.hold}
	if err := owner.Run(t.Context(), exec.CommandContext(t.Context(), "sh", "-c", "exit 0")); err != nil {
		t.Fatal(err)
	}
	if taken, active := h.counts(); taken != 1 || active != 0 {
		t.Errorf("a normal end: %d taken, %d active; want one taken and released", taken, active)
	}
	ctx, cancel := context.WithCancelCause(t.Context())
	stopped := errors.New("stopped")
	go func() { time.Sleep(100 * time.Millisecond); cancel(stopped) }()
	err := owner.Run(ctx, exec.CommandContext(ctx, "sh", "-c", "sleep 30"))
	if !errors.Is(err, stopped) {
		t.Errorf("an aborted command: %v, want its cause", err)
	}
	if taken, active := h.counts(); taken != 2 || active != 0 {
		t.Errorf("an abort: %d taken, %d active; want the second taken and released", taken, active)
	}
}

// A process that leaves the owned group holding its output open fails the
// cleanup at the deadline, as a CleanupFailed naming the group and the
// command, never by waiting for ever on the read.
func TestOutputHeldOpenFailsTheCleanupAtItsDeadline(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for the five-second cleanup deadline")
	}
	owner := &OwnedExecutor{Platform: platformAtBoundary("")}
	// The sleep leaves the group, job control giving it its own, and ends
	// on its own soon after the deadline.
	cmd := exec.CommandContext(t.Context(), "sh", "-c", "(set -m; sleep 7 & ) ; true")
	var out bytes.Buffer
	cmd.Stdout = &out
	r, err := owner.Command(t.Context(), cmd)
	var failed *CleanupFailed
	if !errors.As(err, &failed) || failed.ProcessGroup != cmd.Process.Pid || failed.Command != "sh -c (set -m; sleep 7 & ) ; true" {
		t.Fatalf("result %+v, %v; want CleanupFailed naming the group and the command", r, err)
	}
}
