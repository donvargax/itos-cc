package processes

import (
	"os/exec"
	"syscall"
	"time"
)

// groupCalls are the calls that end a process group, as values: the signal
// to a group, the wait for the command, the inspection for a member that can
// still run, and the clock the cleanup deadline is read by. A Runner's own
// are the system's; one test of this package gives it a script of them to
// play every order a group's end has taken on every system, on Linux.
type groupCalls struct {
	signal   func(group int, signal syscall.Signal) error
	wait     func(cmd *exec.Cmd) error
	runnable func(pgid int) (bool, error)
	now      func() time.Time
	sleep    func(time.Duration)
}

func systemGroupCalls() groupCalls {
	return groupCalls{
		signal:   nativeSignalGroup,
		wait:     (*exec.Cmd).Wait,
		runnable: ownedGroupHasRunnableProcess,
		now:      time.Now,
		sleep:    time.Sleep,
	}
}

// groupCalls gives the calls the runner ends groups by.
func (runner *Runner) groupCalls() groupCalls {
	if runner.calls != nil {
		return *runner.calls
	}
	return systemGroupCalls()
}
