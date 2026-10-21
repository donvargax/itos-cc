package domain

// MutantResult is how one mutant was decided in this run.
type MutantResult struct {
	Site
	Function string // namespace#name
	Outcome  string // Killed, Survived, Timeout, or Uncovered
	// Reused is true when the outcome came from the snapshot without
	// running.
	Reused bool
	// Scope is the scope of the tests that decided Outcome: this run's,
	// or, reused, the one its snapshot records. See RunScope.
	Scope string
	// Excepted is the reason itos-cc.yaml gives when it excepts the mutant
	// and the mutant survived; empty otherwise.
	Excepted string
	// Coverage names the sources of this run's coverage that executed the
	// mutant's line, "in-process", "integration", or both; empty when
	// coverage was not measured or did not execute it.
	Coverage []string
	// Tests is, with Scope ScopeListed, the IDs of the listed tests that
	// decided Outcome.
	Tests []string
	// State is, in a fail-fast run, StateCompleted when Outcome is set, and
	// otherwise why it is not; empty in other runs, where every mutant
	// listed has its outcome.
	State string
}

// skipped marks a site of a function not judged that has no outcome to keep:
// it neither runs nor goes into the snapshot.
const Skipped = "skipped"

// show renders an empty replacement, a deleted operator, visibly.
func Show(s string) string {
	if s == "" {
		return "(deleted)"
	}
	return "`" + s + "`"
}

// FileResult is the outcome for one source file.
type FileResult struct {
	Rel string
	// Snapshot is what the run records of the file, written to .metrics
	// unless the baseline failed or no function was judged.
	Snapshot Snapshot
	// Ran is how many mutants of the functions judged this run decided by
	// running them, and Reused how many it took from the snapshot: a
	// mutant blocked by a failing selection of listed tests counts in
	// neither, nor does a result only kept from before (Preserved).
	Ran, Reused    int
	BaselineFailed bool
	BaselineOutput string
	// FailedSelections is each selection of listed tests that a mutant of
	// the file needed and that failed without any mutant: such a mutant is
	// not decided, so the snapshot is not written, as when the baseline
	// failed.
	FailedSelections []FailedSelection
	// Judged holds the namespace#name of each function judged, in the
	// file's order, or is nil when every function was. Functions is how
	// many the file has.
	Judged    []string
	Functions int
	// Mutants is every mutant of the functions judged, in site order (line,
	// then column), with its outcome; empty when the baseline failed.
	Mutants []MutantResult
	// StaleExceptions is each exception of the functions judged that no
	// longer holds: those whose function changed or whose site is gone,
	// then those whose mutant the tests now kill.
	StaleExceptions []StaleException
	// Baseline says whether this run ran the file's own tests without any
	// mutant: "passed" or "failed" when it did, for this file or another
	// sharing its command, as a Go package's files do; BaselineNotRun when
	// it did not, as for a file no test reaches (Command.RunsNothing), one
	// whose mutants were all reused or otherwise decided before any ran,
	// and in a fail-fast run one the run never got to.
	Baseline string
	// Incomplete is set when a fail-fast stop left a site of a function
	// judged undecided: the run's own work on the file is not complete. The
	// summary comment is not written, and the snapshot only to keep what
	// the run judged (Preserved).
	Incomplete bool
	// Preserved is set when the file is Incomplete and its snapshot was
	// written all the same, as ADR-0022 says: it keeps every completed or
	// reused judgment with the scope and freshness evidence that decided
	// it, and for a site the stop left undecided what the snapshot before
	// recorded that still holds, if anything; such a site gets no outcome
	// of its own. A file the run judged nothing anew in is left as it was.
	Preserved bool
	// Cached is, in a fail-fast run, whether the file's snapshot, as the
	// run leaves it, holds a result that still holds for every mutant of
	// its functions judged: the cache's completeness, which a stopped run
	// can leave complete, apart from the run's own (Incomplete).
	Cached bool
	// CleanupFailures are the commands of the file's judgments whose owned
	// process group's cleanup did not finish (ADR-0021): each judgment has
	// no outcome, and the run reports each group.
	CleanupFailures []WorkerCleanupFailed
}

// BaselineNotRun is the baseline of a file whose own tests did not run
// without a mutant in this run: one no test reaches, whose tests run
// nothing, one none of whose mutants needed them, as when all were reused
// from its snapshot and no other file ran its command, and in a fail-fast
// run one the run never got to.
const BaselineNotRun = "not-run"
