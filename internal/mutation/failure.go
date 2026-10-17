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

// StrictCoverageFlag is run's coverage Flag, which reads reports instead of
// measuring, beside strict coverage of Language, the first strict language
// of the selection.
type StrictCoverageFlag struct {
	Flag, Language string
}

// CountedNoGit is a counted run outside a Git repository: Cause says why
// the working directory's top level could not be told.
type CountedNoGit struct {
	Cause error
}

// OutsideRepository is a path given to a counted run, as written, that is
// outside the repository it judges.
type OutsideRepository struct {
	Argument string
}

// CountedPlanFailed is the fresh plan Fresh refused or failed, for the
// --since ref as written.
type CountedPlanFailed struct {
	Since   string
	Failure mutants.FreshFailure
}

func (LinesWithoutSince) failure()  {}
func (SinceWithChanged) failure()   {}
func (SinceUnavailable) failure()   {}
func (SeedUnavailable) failure()    {}
func (SiteExcluded) failure()       {}
func (NoSurvivor) failure()         {}
func (StrictCoverageFlag) failure() {}
func (CountedNoGit) failure()       {}
func (OutsideRepository) failure()  {}
func (CountedPlanFailed) failure()  {}

func (LinesWithoutSince) Error() string { return "--fail-uncovered=lines without --since" }
func (SinceWithChanged) Error() string  { return "--since beside --changed" }
func (f SinceUnavailable) Error() string {
	return "--since " + f.Ref + ": " + f.Failure.Error()
}
func (f SeedUnavailable) Error() string { return "HEAD for the seed: " + f.Failure.Error() }
func (f SiteExcluded) Error() string    { return f.Path + " is excluded" }
func (f NoSurvivor) Error() string      { return f.Path + " holds no survivor: " + f.Reason }
func (f StrictCoverageFlag) Error() string {
	return f.Flag + " beside strict " + f.Language + " coverage"
}
func (f CountedNoGit) Error() string      { return "no Git repository: " + f.Cause.Error() }
func (f OutsideRepository) Error() string { return f.Argument + " is outside the repository" }
func (f CountedPlanFailed) Error() string { return "fresh plan: " + f.Failure.Error() }
