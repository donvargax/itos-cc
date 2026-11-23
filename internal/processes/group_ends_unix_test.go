//go:build linux || darwin

package processes

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"sync"
	"syscall"
	"testing"
	"time"
)

// groupEnd is one order a process group's end has taken, as a script of the
// calls: what the group signal answers, what the inspection finds and what
// the runner must give for it.
type groupEnd struct {
	name     string
	platform string
	shell    string // the command, run for real in its own group
	cancel   bool   // the context ends while the command runs
	timeout  bool   // it ends by the command's own timeout, not its parent's cancel
	// kill is the answer to a SIGKILL of the group, given after the group
	// really got it when realKill is set.
	kill     error
	realKill bool
	// probe is the answer to the signal 0 that asks whether the group is
	// there, by its call number; zombies tells the inspection of /proc that
	// only zombies are left.
	probe   func(call int) error
	running func(call int) bool

	wantPassed    bool
	wantCancelled bool
	wantCleanup   bool // CleanupFailed
}

func always(err error) func(int) error { return func(int) error { return err } }

// TestGroupEndsAreJudgedInEveryOrder plays, on any system, the orders a
// process group's end took on each system (713506df, 891f0567, 6ec9027c):
// each must give the Result and error of a command that finished as it
// did, never an internal error, and hold the cleanup deadline.
func TestGroupEndsAreJudgedInEveryOrder(t *testing.T) {
	gone := always(syscall.ESRCH)
	for _, c := range []groupEnd{
		{name: "cancelled before the command exits", platform: "linux", shell: "sleep 30", cancel: true,
			realKill: true, probe: gone, wantCancelled: true},
		{name: "cancel lands after it exited (ESRCH)", platform: "linux", shell: "sleep 0.2", cancel: true,
			kill: syscall.ESRCH, probe: gone, wantPassed: true},
		{name: "cancel lands after it exited on darwin", platform: "darwin", shell: "sleep 0.2", cancel: true,
			kill: syscall.ESRCH, probe: gone, wantPassed: true},
		{name: "group gone before the kill", platform: "linux", shell: "exit 0",
			kill: syscall.ESRCH, probe: gone, wantPassed: true},
		{name: "only zombies left on linux", platform: "linux", shell: "exit 0",
			probe: always(nil), running: func(int) bool { return false }, wantPassed: true},
		{name: "only zombies left on darwin (EPERM)", platform: "darwin", shell: "exit 0",
			kill: syscall.EPERM, wantPassed: true,
			probe: func(call int) error {
				if call < 3 {
					return syscall.EPERM
				}
				return syscall.ESRCH
			}},
		{name: "exits 0, then its timeout lands on its zombie on linux", platform: "linux", shell: "sleep 0.2",
			cancel: true, timeout: true, probe: gone, wantPassed: true},
		{name: "exits 0, then its timeout lands on its zombie on darwin", platform: "darwin", shell: "sleep 0.2",
			cancel: true, timeout: true, kill: syscall.EPERM, probe: gone, wantPassed: true},
		{name: "exits 0, then its parent's cancel lands while its member runs on linux", platform: "linux", shell: "sleep 0.2",
			cancel: true, probe: gone, wantPassed: true},
		{name: "exits 0, then its parent's cancel lands while its member runs on darwin", platform: "darwin", shell: "sleep 0.2",
			cancel: true, probe: gone, wantPassed: true},
		{name: "darwin member refusing the signal fails at the deadline", platform: "darwin", shell: "exit 0",
			kill: syscall.EPERM, probe: always(syscall.EPERM), wantCleanup: true},
		{name: "linux treats EPERM from the kill as a failure", platform: "linux", shell: "exit 0",
			kill: syscall.EPERM, probe: gone, wantCleanup: true},
		{name: "the kill lands inside the deadline", platform: "linux", shell: "exit 0",
			probe: always(nil), running: func(call int) bool { return call < 50 }, wantPassed: true},
		{name: "the kill lands past the deadline", platform: "linux", shell: "exit 0",
			probe: always(nil), running: func(int) bool { return true }, wantCleanup: true},
	} {
		t.Run(c.name, func(t *testing.T) { playGroupEnd(t, c) })
	}
}

func playGroupEnd(t *testing.T, c groupEnd) {
	t.Helper()
	ctx, stop := context.WithCancelCause(t.Context())
	defer stop(nil)
	cancel := func() { stop(nil) }
	if c.timeout {
		cancel = func() { stop(ErrCommandTimeout) }
	}
	var mu sync.Mutex
	probes, inspections := 0, 0
	clock := time.Now()
	calls := systemGroupCalls()
	calls.signal = func(group int, signal syscall.Signal) error {
		if signal == syscall.SIGKILL {
			if c.realKill {
				_ = nativeSignalGroup(group, signal)
			}
			return c.kill
		}
		mu.Lock()
		probes++
		call := probes
		mu.Unlock()
		return c.probe(call)
	}
	calls.wait = func(cmd *exec.Cmd) error {
		if c.cancel {
			cancel()
		}
		return cmd.Wait()
	}
	calls.runnable = func(int) (bool, error) {
		mu.Lock()
		inspections++
		call := inspections
		mu.Unlock()
		return c.running != nil && c.running(call), nil
	}
	calls.now = func() time.Time { mu.Lock(); defer mu.Unlock(); return clock }
	calls.sleep = func(d time.Duration) { mu.Lock(); clock = clock.Add(d); mu.Unlock() }

	runner := &Runner{Platform: c.platform, calls: &calls}
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", c.shell)
	cmd.Stdout = &out
	result, err := runner.Run(ctx, cmd, &out)

	var cleanup *CleanupFailed
	switch {
	case c.wantCleanup:
		if !errors.As(err, &cleanup) {
			t.Fatalf("error = %v; want a CleanupFailed", err)
		}
	case err != nil:
		t.Fatalf("error = %v; want the command judged as it finished", err)
	}
	if !c.wantCleanup && (result.Passed != c.wantPassed || result.Cancelled != c.wantCancelled) {
		t.Fatalf("result = %+v; want Passed %v, Cancelled %v", result, c.wantPassed, c.wantCancelled)
	}
}
