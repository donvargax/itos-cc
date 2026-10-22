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

// Execute runs already-admitted judgments. It does no planning/publication and
// is shared with the temporary legacy sample caller.
func Execute(ctx context.Context, states []*Execution, opt Options, ff *FailFast) (*Stop, error) {
	return execute(ctx, states, opt, ff)
}
