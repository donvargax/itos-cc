package mutate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"
)

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
	// FailUncovered makes an uncovered mutant, and a strict Go coverage
	// finding, actionable: with --fail-uncovered.
	FailUncovered bool
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

// errFailFastStop is the cause of a fail-fast run's context once it stops.
var errFailFastStop = errors.New("the run stopped at its first actionable failure")

// stopper publishes a fail-fast run's one stop: the first trigger cancels
// the run's one context, which every command runs under, and starts the
// cleanup deadline every worker shares.
type stopper struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
	budget *cleanupBudget
	log    io.Writer
	mu     sync.Mutex
	stop   *Stop
}

func newStopper(log io.Writer) *stopper {
	ctx, cancel := context.WithCancelCause(context.Background())
	return &stopper{ctx: ctx, cancel: cancel, budget: &cleanupBudget{}, log: log}
}

// trigger stops the run at s, unless it stopped already.
func (st *stopper) trigger(s *Stop) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.stop != nil {
		return
	}
	st.stop = s
	st.budget.begin()
	st.cancel(errFailFastStop)
	announce(st.log, s)
}

// stopped says whether the run has stopped; a nil stopper never does.
func (st *stopper) stopped() bool {
	if st == nil {
		return false
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.stop != nil
}

func (st *stopper) result() *Stop {
	if st == nil {
		return nil
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.stop
}

// announce logs that the run stops at s.
func announce(log io.Writer, s *Stop) {
	fmt.Fprintf(log, "itos-cc: --fail-fast: stopping at %s in %s; no further command starts\n", s.Rule, s.File)
}

// cancelledBy says whether err is how a command or a selection's baseline
// ended because ctx, a fail-fast run's, stopped: a start the stop refused or
// a cancellation it caused, never a failure of the command's own.
func cancelledBy(ctx context.Context, err error) bool {
	return ctx.Err() != nil && err != nil && (errors.Is(err, context.Canceled) || errors.Is(err, errFailFastStop))
}

// knownStop is the first failure the plan already knows, before coverage:
// an exception of a function judged that no longer holds, in file order.
func knownStop(states []*fileState) *Stop {
	for _, s := range states {
		if len(s.excepted.stale) > 0 {
			e := s.excepted.stale[0]
			return &Stop{Rule: "mutation.exception-stale", File: s.rel, Exception: &e}
		}
	}
	return nil
}

// coverageStop is, with FailUncovered, the first failure coverage already
// shows before any mutant runs, file by file: an uncovered mutant of a
// function judged, in line order, then a function judged that strict Go
// coverage finds without complete evidence or with an uncovered block, as
// reportStrictGoCoverage would report it.
func coverageStop(states []*fileState, opt Options, ff *FailFast) (*Stop, error) {
	if !ff.FailUncovered {
		return nil, nil
	}
	for _, s := range states {
		order := make([]int, len(s.sites))
		for i := range order {
			order[i] = i
		}
		slices.SortStableFunc(order, func(a, b int) int { return LineOrder(s.sites[a], s.sites[b]) })
		for _, i := range order {
			site := s.sites[i]
			if s.outcomes[i] != Uncovered || (s.judged != nil && !s.judged[site.Unit]) {
				continue
			}
			u := s.file.Units[site.Unit]
			return &Stop{Rule: "mutation.uncovered", File: s.rel, Function: unitID(u.Namespace, u.Name), Site: &site}, nil
		}
		if s.file.Spec.Name != "go" || (opt.StatementCoverage == nil && opt.CachedCoverage == nil) {
			continue
		}
		evidence, err := s.goEvidence(opt)
		if err != nil {
			return nil, err
		}
		for i, u := range s.file.Units {
			if s.judged != nil && !s.judged[i] {
				continue
			}
			id := unitID(u.Namespace, u.Name)
			if e := evidence[i]; e == nil || !e.Complete {
				return &Stop{Rule: "mutation.coverage-missing", File: s.rel, Function: id, Line: u.StartLine}, nil
			}
			for _, block := range evidence[i].Blocks {
				if !block.Covered {
					return &Stop{Rule: "mutation.uncovered-statement", File: s.rel, Function: id, Line: block.Line}, nil
				}
			}
		}
	}
	return nil, nil
}

// actionable is the stop the outcome of site i makes final, or nil: an
// unexcepted survivor, or an excepted mutant the tests now kill, whose
// exception no longer holds.
func (s *fileState) actionable(i int) *Stop {
	site := s.sites[i]
	u := s.file.Units[site.Unit]
	e, excepted := s.excepted.held[i]
	switch s.outcomes[i] {
	case Survived:
		if !excepted {
			return &Stop{Rule: "mutation.survived", File: s.rel, Function: unitID(u.Namespace, u.Name), Site: &site}
		}
	case Killed, Timeout:
		if excepted {
			return &Stop{Rule: "mutation.exception-stale", File: s.rel, Exception: &StaleException{Exception: e, Why: ExceptionKilled, Line: site.Line}}
		}
	}
	return nil
}

// stateOf is the fail-fast state of site i once the run is over.
func (s *fileState) stateOf(i int) string {
	switch {
	case s.outcomes[i] != "":
		return StateCompleted
	case s.undecided != nil && s.undecided[i] != "":
		return s.undecided[i]
	case s.result.BaselineFailed:
		return StateBlocked
	}
	return StateUnattempted
}

// leave records that site i ends the run in state, with no outcome.
func (s *fileState) leave(i int, state string) {
	if s.undecided == nil {
		s.undecided = make([]string, len(s.sites))
	}
	s.undecided[i] = state
}

// incomplete says whether a site of a function judged has no outcome.
func (s *fileState) incomplete() bool {
	for _, o := range s.outcomes {
		if o == "" {
			return true
		}
	}
	return false
}

// preserved is the outcomes, scopes and listed tests a fail-fast run
// records of a file it left with undecided sites: this run's outcome of
// each site, run, measured or reused, and for a site it left undecided the
// outcome the snapshot before records that still holds, if any, with what
// decided it, so a stop never overwrites a valid result with nothing, a
// cancelled --mutate-all rerun's included. A site with neither stays
// undecided, which leaves it out of the snapshot.
func (s *fileState) preserved() (outcomes, scopes []string, ran [][]string) {
	outcomes, scopes, ran = slices.Clone(s.outcomes), slices.Clone(s.scopes), slices.Clone(s.ran)
	for i, o := range outcomes {
		if f := s.fresh[i]; o == "" && f.outcome != "" {
			outcomes[i], scopes[i], ran[i] = f.outcome, f.scope, f.tests
		}
	}
	return outcomes, scopes, ran
}

// judgedAnew says whether the run decided a site of a function judged
// itself, by running its mutant or measuring it uncovered, rather than
// reusing its outcome: only then has a file it left incomplete anything new
// to keep.
func (s *fileState) judgedAnew() bool {
	for i, o := range s.outcomes {
		if o != "" && o != skipped && !s.reused[i] && (s.judged == nil || s.judged[s.sites[i].Unit]) {
			return true
		}
	}
	return false
}

// cacheComplete says whether the file's snapshot, as a fail-fast run leaves
// it, holds a result that still holds for every site of its functions
// judged: this run's outcome, when written says the run writes it, else
// the one the snapshot before records that still holds.
func (s *fileState) cacheComplete(written bool) bool {
	for i, site := range s.sites {
		if s.judged != nil && !s.judged[site.Unit] {
			continue
		}
		if (written && s.outcomes[i] != "") || s.fresh[i].outcome != "" {
			continue
		}
		return false
	}
	return true
}

// keepCoverage gives each function of the snapshot of a file a fail-fast
// stop left incomplete that this run measured no Go coverage for the
// evidence the snapshot before records for it, while its hash is the same:
// the stop never drops evidence, whose own inputs say whether it holds.
func (s *fileState) keepCoverage() {
	if s.stored == nil {
		return
	}
	units := s.result.Snapshot.Units
	ids, hashes := fileKeys(s.file)
	for i, j := range EntriesOf(ids, hashes, s.stored.Units) {
		if j >= 0 && units[i].Coverage == nil && s.stored.Units[j].Hash == hashes[i] {
			units[i].Coverage = s.stored.Units[j].Coverage
		}
	}
}
