package mutants

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
