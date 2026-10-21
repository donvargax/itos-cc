package domain

// FailFast makes a complete run stop at the first actionable final
// judgment it observes (ADR-0020): an unexcepted survivor once the listed
// tests that reach it also failed to kill it, an uncovered mutant with
// FailUncovered, an exception that no longer holds, a file's failing
// baseline, or a selection of listed tests that fails without any mutant.
// Killed, timed-out, validly excepted and listed-killed mutants never stop
// it. The stop admits no further command, cancels every judgment then
// running, with no outcome, and kills and joins every process group the
// run owns within one shared five-second deadline from the stop, before
// the worker copies are removed. A file the stop left with an undecided
// site of a function judged, cut short or blocked by a selection of listed
// tests, gets no summary comment, and its snapshot keeps what the run
// judged and nothing for the undecided sites (ADR-0022; see
// FileResult.Preserved).
type FailFast struct {
	// FailUncovered makes an uncovered mutant, and a strict coverage
	// finding, actionable: with --fail-uncovered.
	FailUncovered bool
	// Lines, with --fail-uncovered=lines, narrows what FailUncovered makes
	// actionable to the lines the --since range added or changed: an
	// uncovered mutant on one, a function with one and without complete
	// evidence, or an uncovered block or line overlapping one. Nil judges
	// whole functions.
	Lines LineScope
	// Known is a failure the caller knew before the run planned, such as an
	// exception for a file the run does not select that no longer holds:
	// when any mutant has to run, the run stops before any of them, and
	// before coverage.
	Known *Stop
}

// Stop is the judgment that stopped a fail-fast run. Rule is the rule its
// problem reports, and the rest its subject: File, the file as output names
// it; with mutation.survived and mutation.uncovered, Function and Site; with
// mutation.exception-stale, Exception; with tests.selection-failed,
// Selection; and with mutation.uncovered-statement and
// mutation.coverage-missing, Function and Line.
type Stop struct {
	Rule      string
	File      string
	Function  string
	Site      *Site
	Line      int
	Exception *StaleException
	Selection *FailedSelection
}

// The states of a selected site in a fail-fast run, in MutantResult.State.
// A completed site has its outcome, run, reused or measured uncovered; the
// others have none: cancelled, its judgment was running when the run
// stopped; unattempted, it never started; blocked, its file's baseline, or
// the selection of listed tests it needed, failed without any mutant.
const (
	StateCompleted   = "completed"
	StateCancelled   = "cancelled"
	StateUnattempted = "unattempted"
	StateBlocked     = "blocked"
)
