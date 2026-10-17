package mutation

import "github.com/donvargax/itos-cc/internal/mutants"

// Failure is what the handler refuses at its original admission point. It
// carries the fact only: the UI words it and picks its exit code.
//
//sumtype:decl
type Failure interface {
	error
	failure()
}

// LinesWithoutSince is --fail-uncovered=lines without a --since range.
type LinesWithoutSince struct{}

// SinceWithChanged is --since beside --changed.
type SinceWithChanged struct{}

// SinceUnavailable is the --since range that History could not give.
type SinceUnavailable struct {
	Ref     string
	Failure mutants.HistoryFailure
}

// SeedUnavailable is sample's omitted --seed, whose HEAD commit History
// could not give.
type SeedUnavailable struct {
	Failure mutants.HistoryFailure
}

func (LinesWithoutSince) failure() {}
func (SinceWithChanged) failure()  {}
func (SinceUnavailable) failure()  {}
func (SeedUnavailable) failure()   {}

func (LinesWithoutSince) Error() string { return "--fail-uncovered=lines without --since" }
func (SinceWithChanged) Error() string  { return "--since beside --changed" }
func (f SinceUnavailable) Error() string {
	return "--since " + f.Ref + ": " + f.Failure.Error()
}
func (f SeedUnavailable) Error() string { return "HEAD for the seed: " + f.Failure.Error() }
