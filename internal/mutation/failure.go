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

// SiteExcluded is except's site, at Path, in a source the settings'
// exclude leaves out.
type SiteExcluded struct {
	Path         string
	Line, Column int
}

// NoSurvivor is except's site, at Path, which no fresh snapshot records as
// a survivor: Reason says why.
type NoSurvivor struct {
	Path         string
	Line, Column int
	Reason       string
}

func (LinesWithoutSince) failure() {}
func (SinceWithChanged) failure()  {}
func (SinceUnavailable) failure()  {}
func (SeedUnavailable) failure()   {}
func (SiteExcluded) failure()      {}
func (NoSurvivor) failure()        {}

func (LinesWithoutSince) Error() string { return "--fail-uncovered=lines without --since" }
func (SinceWithChanged) Error() string  { return "--since beside --changed" }
func (f SinceUnavailable) Error() string {
	return "--since " + f.Ref + ": " + f.Failure.Error()
}
func (f SeedUnavailable) Error() string { return "HEAD for the seed: " + f.Failure.Error() }
func (f SiteExcluded) Error() string    { return f.Path + " is excluded" }
func (f NoSurvivor) Error() string      { return f.Path + " holds no survivor: " + f.Reason }
