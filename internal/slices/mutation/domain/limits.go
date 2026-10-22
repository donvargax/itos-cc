package domain

import "time"

// A mutant's run is bounded by limits sized from its baseline's, the run
// of the same command with nothing mutated (issues #54 and #47, q-23): its
// timeout, the baseline's time times a factor plus a fixed allowance, and
// two caps, its output, the bytes it writes, stdout and stderr together,
// and its memory, the resident memory of its process group, each the
// baseline's times a factor plus an allowance of its own, so no cap sits
// below what the unmutated tests need. Where the system measures no
// memory, no memory cap holds. A run over a cap is killed, as at its own
// timeout, and its outcome is OverLimit, which, as a timeout, counts as
// killed: a runaway is the suite catching the mutant. A baseline has no
// limits.

// Limits bound one command's run: Timeout its time, Output the bytes it
// may write, and Memory the resident memory its process group may hold. A
// zero limit bounds nothing.
type Limits struct {
	Timeout        time.Duration
	Output, Memory int64
}

// The caps a run can go over, as its OverLimit outcome's Limit names them.
const (
	LimitOutput = "output"
	LimitMemory = "memory"
)

// Usage is what a baseline's run used: its time, the bytes it wrote, and
// the most memory its process group held, where MemoryMeasured says the
// system measured it.
type Usage struct {
	Elapsed        time.Duration
	Output, Memory int64
	MemoryMeasured bool
}

// outputAllowance and memoryAllowance are added to every mutant's output
// and memory caps, as timeoutAllowance is to its timeout: a mutant's run
// builds the mutated code, which its baseline may have found built, and
// the floors keep a quiet or small suite's caps from cutting a mutant's
// ordinary run short.
const (
	outputAllowance = 16 << 20
	memoryAllowance = 256 << 20
)

// Factors size the limits of a mutant's run from its baseline's Usage:
// each limit is its factor times what the baseline used, plus the
// allowance.
type Factors struct {
	Timeout, Output, Memory float64
}

// LimitsOf are the limits of the mutant runs whose baseline used u: no
// memory cap where the baseline's memory was not measured.
func (f Factors) LimitsOf(u Usage) Limits {
	l := Limits{Timeout: TimeoutOf(u.Elapsed, f.Timeout), Output: int64(float64(u.Output)*f.Output) + outputAllowance}
	if u.MemoryMeasured {
		l.Memory = int64(float64(u.Memory)*f.Memory) + memoryAllowance
	}
	return l
}

// Noticed says whether outcome means the tests noticed the mutant: killed,
// timed out or over a limit, each a kill.
func Noticed(outcome string) bool {
	return outcome == Killed || outcome == Timeout || outcome == OverLimit
}

// MemoryUnmeasured is the RunEvent kind a run reports once, the first time
// a baseline's memory was not measured: no memory cap holds on its system.
const MemoryUnmeasured = "memory-unmeasured"
