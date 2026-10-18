//go:build linux || darwin

package worktrees

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
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
// command, never by waiting for ever on the read. The command is this test
// binary, which starts itself again in a process group of its own, holding
// the command's output, and returns.
func TestOutputHeldOpenFailsTheCleanupAtItsDeadline(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for the five-second cleanup deadline")
	}
	owner := &OwnedExecutor{Platform: platformAtBoundary("")}
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestHeldOutputHelper$")
	cmd.Env = append(os.Environ(), "ITOS_CC_HELD_OUTPUT=start")
	var out bytes.Buffer
	cmd.Stdout = &out
	r, err := owner.Command(t.Context(), cmd)
	var failed *CleanupFailed
	if !errors.As(err, &failed) || failed.ProcessGroup != cmd.Process.Pid || failed.Command != strings.Join(cmd.Args, " ") {
		t.Fatalf("result %+v, %v; want CleanupFailed naming the group and the command", r, err)
	}
}

// TestHeldOutputHelper is the command of the test above: started, it starts
// itself again in a group of its own with its output, and returns; started
// again, it holds that output for a while past the deadline, then ends.
func TestHeldOutputHelper(t *testing.T) {
	switch os.Getenv("ITOS_CC_HELD_OUTPUT") {
	case "start":
		hold := exec.Command(os.Args[0], "-test.run=^TestHeldOutputHelper$")
		hold.Env = append(os.Environ(), "ITOS_CC_HELD_OUTPUT=hold")
		hold.Stdout = os.Stdout
		hold.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := hold.Start(); err != nil {
			t.Fatal(err)
		}
	case "hold":
		time.Sleep(7 * time.Second)
	}
}
