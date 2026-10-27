package mutation

import mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"

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
// exclude leaves out; Identity is the identity that named it, "" for a
// site named by its line and column, and File is the identity's file, slash-separated from the project root on
// every system as the identity names it, which names the file in place of
// Path when an identity named the site, "" when a site did.
type SiteExcluded struct {
	Path         string
	Line, Column int
	Identity     string
	File         string
}

// IdentityStale is except's Identity, of Function in the file at Path,
// which names no site of the source as it is now: Why is changed, the
// function's hash now Hash, or gone. File is the identity's file, slash-separated from the project root on
// every system as the identity names it, which names the file in place of
// Path when an identity named the site, "" when a site did.
type IdentityStale struct {
	Path, Identity, Function string
	Why, Hash                string
	File                     string
}

// NoSurvivor is except's site, at Path, which no fresh snapshot records as
// a survivor: Reason says why, and Uncovered is true when its mutant is
// recorded uncovered. File is the identity's file, slash-separated from the project root on
// every system as the identity names it, which names the file in place of
// Path when an identity named the site, "" when a site did.
type NoSurvivor struct {
	Path         string
	Line, Column int
	Reason       string
	Uncovered    bool
	File         string
}

// ReasonMissing is except's site given with no --reason, when it re-points
// no stale entry whose reason it would keep.
type ReasonMissing struct{}

// StrictCoverageFlag is run's coverage Flag, which reads reports instead of
// measuring, beside strict coverage of Language, the first strict language
// of the selection.
type StrictCoverageFlag struct {
	Flag, Language string
}

// CountedNoGit is a counted run outside a Git repository: Cause says why
// the working directory's top level could not be told, and Flag is the
// run's, --count or --site.
type CountedNoGit struct {
	Cause error
	Flag  string
}

// OutsideRepository is a path given to a counted run, as written, that is
// outside the repository it judges.
type OutsideRepository struct {
	Argument string
}

// CountedPlanFailed is the fresh plan Fresh refused or failed, for the
// --since ref as written, of a run of Flag, --count or --site.
type CountedPlanFailed struct {
	Since   string
	Failure mutants.FreshFailure
	Flag    string
}

func (LinesWithoutSince) failure()  {}
func (SinceWithChanged) failure()   {}
func (SinceUnavailable) failure()   {}
func (SeedUnavailable) failure()    {}
func (SiteExcluded) failure()       {}
func (NoSurvivor) failure()         {}
func (IdentityStale) failure()      {}
func (ReasonMissing) failure()      {}
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
func (f IdentityStale) Error() string   { return f.Identity + " is " + f.Why }
func (ReasonMissing) Error() string     { return "no --reason" }
func (f StrictCoverageFlag) Error() string {
	return f.Flag + " beside strict " + f.Language + " coverage"
}
func (f CountedNoGit) Error() string      { return "no Git repository: " + f.Cause.Error() }
func (f OutsideRepository) Error() string { return f.Argument + " is outside the repository" }
func (f CountedPlanFailed) Error() string { return "fresh plan: " + f.Failure.Error() }
