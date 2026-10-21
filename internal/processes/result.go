package processes

import (
	"os/exec"
	"time"
)

// Result is how an owned command ended: passed, timed out by its own
// deadline, cancelled by its parent, or its exit code, with its elapsed
// time and the output captured.
type Result struct {
	Passed, TimedOut, Cancelled bool
	Elapsed                     time.Duration
	Output                      string
	ExitCode                    int
}

func isExit(err error) bool {
	_, ok := err.(*exec.ExitError)
	return ok
}
