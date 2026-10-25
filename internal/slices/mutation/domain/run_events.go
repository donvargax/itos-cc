package domain

import (
	"context"
	"slices"
	"time"

	"github.com/donvargax/itos-cc/internal/lang"
)

// Progress receives domain observations. Presentation owns their wording and
// output; events contain no already-formatted progress lines.
type Progress interface{ Report(RunEvent) }

type RunEvent struct {
	FreshTrial *FreshTrialResult
	Kind       string
	Command    Command
	File       string
	Site       Site
	IDs        []string
	Outcome    string
	// Limit names the cap an OverLimit outcome went over, and Limits are
	// the limits of the run it went over; for "selection-passed", Limits
	// are those of the selection's mutant runs.
	Limit                    string
	Limits                   Limits
	Elapsed, Listed, Timeout time.Duration
	Done, Total              int
	Failure                  error
	Stop                     *Stop
}

func report(progress Progress, event RunEvent) {
	if progress != nil {
		progress.Report(event)
	}
}

// NewExecution constructs the genuine shared scheduler state. Legacy Sample
// retains drawing/checking until T-94 and admits only its drawn sites here.
func NewExecution(file *lang.File, rel, key string, command Command) *Execution {
	sites := Sites(file)
	return &Execution{file: file, rel: rel, key: key, command: command, sites: sites,
		result: &FileResult{Rel: rel}, outcomes: slices.Repeat([]string{Skipped}, len(sites)),
		scopes: make([]string, len(sites)), limits: make([]string, len(sites)), ran: make([][]string, len(sites))}
}

func (s *Execution) Pending(site int, ids []string) {
	s.outcomes[site] = ""
	if ids != nil {
		if s.reach == nil {
			s.reach = make([][]string, len(s.sites))
		}
		s.reach[site] = ids
	}
}
func (s *Execution) Result() *FileResult     { return s.result }
func (s *Execution) Outcome(site int) string { return s.outcomes[site] }

// Recheck has a sample or a run judge site's mutant against recorded,
// the outcome its snapshot records, or Flaky for one it records flaky: it
// runs again, in the same worker and scope, while its outcomes leave it
// undecided (Classify, or Settled for Flaky), each kept in run order
// (Outcomes).
func (s *Execution) Recheck(site int, recorded string) {
	if s.recorded == nil {
		s.recorded, s.trials = make([]string, len(s.sites)), make([][]string, len(s.sites))
	}
	if s.flakes == nil {
		s.flakes = make([][]string, len(s.sites))
	}
	s.recorded[site] = recorded
}

// Outcomes is every outcome the runs of site's mutant gave in a sample,
// in run order.
func (s *Execution) Outcomes(site int) []string { return slices.Clone(s.trials[site]) }

// tried keeps outcome, of a run of site's mutant, among its trials when
// it is rechecked, and says whether it runs again: while its outcomes
// leave it undecided. Once they say it is flaky, they are its flakes. A
// mutant not rechecked keeps nothing, and never runs again.
func (s *Execution) tried(site int, outcome string) bool {
	if s.recorded == nil || s.recorded[site] == "" {
		return false
	}
	s.trials[site] = append(s.trials[site], outcome)
	verdict := Classify(s.recorded[site], s.trials[site])
	if s.recorded[site] == Flaky {
		verdict = Settled(s.trials[site])
	}
	if verdict == Flaky {
		s.flakes[site] = slices.Clone(s.trials[site])
	}
	return verdict == ""
}

// flaky is the outcomes site's mutant is flaky with, or nil.
func (s *Execution) flaky(site int) []string {
	if s.flakes == nil {
		return nil
	}
	return s.flakes[site]
}

// again says whether site's mutant ran before in this run: a run again
// only adds its outcome to the mutant's trials, the first run's outcome
// being the one it records.
func (s *Execution) again(site int) bool {
	return s.trials != nil && len(s.trials[site]) > 0
}

// Execute runs already-admitted judgments. It does no planning/publication and
// is shared with the temporary legacy sample caller.
func Execute(ctx context.Context, states []*Execution, opt Options, ff *FailFast) (*Stop, error) {
	return execute(ctx, states, opt, ff)
}
