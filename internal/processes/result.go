package processes

import (
	"os/exec"
	"time"
)

// Result is how an owned command ended: passed, timed out by its own
// deadline, cancelled by its parent, killed over a cap of its Limits, or
// its exit code, with its elapsed time, the output kept and what it used.
type Result struct {
	Passed, TimedOut, Cancelled bool
	Elapsed                     time.Duration
	// Output is what the command wrote to the output its caller captures,
	// as kept: all of it, or, once it wrote more than twice 64 KiB, its
	// first and last 64 KiB, the bytes between left out at OutputCut, with
	// OutputLeftOut the lines they held. OutputBytes is all it wrote,
	// stdout and stderr together, which exceeds Output's length only when
	// bytes were left out.
	Output        string
	OutputBytes   int64
	OutputCut     int
	OutputLeftOut int
	ExitCode      int
	// OverLimit names the cap the command went over, OverOutput or
	// OverMemory, which killed it; "" when it went over none.
	OverLimit string
	// PeakMemory is the most resident memory the command's owned process
	// group held, summed over its members, in bytes, as sampled, where
	// MemoryMeasured says the system measures it.
	PeakMemory     int64
	MemoryMeasured bool
}

func isExit(err error) bool {
	_, ok := err.(*exec.ExitError)
	return ok
}
