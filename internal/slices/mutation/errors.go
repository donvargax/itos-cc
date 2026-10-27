package mutation

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/coverage"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// The UI's classification of what list's and check's handlers return: the
// slice's own failures, settings and history failures, and snapshot reads.
// Each keeps the words, subject and exit code the cmd handlers gave it.

func settingsError(failure mutants.SettingsFailure) error {
	switch failure := failure.(type) {
	case mutants.SettingsInvalid:
		return configInvalid(failure.Reason, failure.Key, failure.Fix, "Fix it: exclude lists globs of sources from the project root, such as **/porttest/**; each entry under mutation.exceptions needs file, function, hash, line_in_function, column, original, replacement, and reason, and 'itos-cc mutation except' writes one; mutation.tests needs list, run with {pattern}, ids_pattern with {ids}, and join with each, holding {id}, and sep.")
	case mutants.InputFailed:
		return failure.Cause
	case mutants.InputCancelled:
		return failure.Cause
	}
	return failure
}

// configInvalid is config.invalid for an itos-cc.yaml that cannot be read,
// for reason, with own, the refusal's own fix, else fix: its subject the
// file, and the key at fault when one is.
func configInvalid(reason, key, own, fix string) *cli.Problem {
	if own != "" {
		fix = own
	}
	p := cli.NewProblem(cli.CodeUsage, "config.invalid", "itos-cc.yaml: "+reason, fix).With("file", "itos-cc.yaml")
	if key != "" {
		p = p.With("key", key)
	}
	return p
}

// failureError words failure, naming a file as rel names it for a person.
func failureError(failure Failure, rel func(string) string) error {
	switch failure := failure.(type) {
	case LinesWithoutSince:
		return cli.NewProblem(cli.CodeUsage, "flags.conflict", "--fail-uncovered=lines judges only the lines a --since range changed, and was given without --since", "Add --since REF, such as --since origin/main, or write --fail-uncovered alone to judge whole functions.").With("flag", "--fail-uncovered=lines")
	case SinceWithChanged:
		return cli.NewProblem(cli.CodeUsage, "flags.conflict", "--since and --changed cannot be combined: --since judges the functions of commits, --changed whole files of the working tree", "Drop --changed; commit the work to judge it with --since.").With("flag", "--changed")
	case SinceUnavailable:
		return sinceError(failure.Failure, failure.Ref)
	case SeedUnavailable:
		return seedError(failure.Failure)
	case SiteExcluded:
		file := named(failure.File, failure.Path, rel)
		at := fmt.Sprintf("%s:%d:%d", file, failure.Line, failure.Column)
		if failure.Identity != "" {
			at = failure.Identity
		}
		return cli.NewProblem(cli.CodeUsage, "exception.excluded",
			fmt.Sprintf("%s is in %s, a source the exclude of %s leaves out, so none of its mutants is judged or excepted", at, file, "itos-cc.yaml"),
			fmt.Sprintf("Except a survivor of a source mutation run judges, or take %s out of exclude in %s first.", file, "itos-cc.yaml")).
			With("file", file)
	case NoSurvivor:
		file := named(failure.File, failure.Path, rel)
		message := fmt.Sprintf("%s:%d:%d is no survivor to except: %s", file, failure.Line, failure.Column, failure.Reason)
		fix := fmt.Sprintf("Except a site that mutation run lists as survived; run 'itos-cc mutation run %s' first when its results are stale or missing.", file)
		if failure.Uncovered {
			// On purpose (q-12): an exception covers survivors only.
			message += ", and an exception covers survivors only, on purpose: an uncovered line needs a test, not a reason"
			fix = fmt.Sprintf("Add a test that runs line %d, or route the branch through code a test already reaches, such as a helper the tests call; then run 'itos-cc mutation run %s'.", failure.Line, file)
		}
		return cli.NewProblem(cli.CodeUsage, "exception.no-survivor", message, fix).
			With("file", file).With("line", failure.Line).With("column", failure.Column)
	case IdentityStale:
		return identityStale(failure, named(failure.File, failure.Path, rel))
	case ReasonMissing:
		return reasonMissing()
	case StrictCoverageFlag:
		return flagConflict(failure.Flag, failure.Flag+" is not admitted with strict "+LanguageName(failure.Language)+" coverage; use a fresh built-in or listed measurement")
	case CountedNoGit:
		return countedNoGit(failure.Cause, failure.Flag)
	case OutsideRepository:
		return cli.NewProblem(cli.CodeUsage, "paths.unmatched", fmt.Sprintf("%q is outside the repository that --count judges", failure.Argument),
			"Name paths inside the repository.").With("argument", failure.Argument)
	case CountedPlanFailed:
		return countedPlanError(failure.Failure, failure.Since, failure.Flag)
	}
	return failure
}

// named is the file of a site except names for a person: file, the
// identity's own, slash-separated from the project root on every system,
// when an identity named the site, else path as rel names it.
func named(file, path string, rel func(string) string) string {
	if file != "" {
		return file
	}
	return rel(path)
}

// identityStale is exception.identity-stale: the identity names no site
// of file as it is now, its function changed, with its hash now, or its
// file, function or site gone.
func identityStale(failure IdentityStale, file string) *cli.Problem {
	why := fmt.Sprintf("%s, %s or its site in it is gone", file, failure.Function)
	if failure.Why == mutants.ExceptionChanged {
		why = fmt.Sprintf("%s changed since, its hash now %s", failure.Function, failure.Hash)
	}
	p := cli.NewProblem(cli.CodeUsage, "exception.identity-stale",
		fmt.Sprintf("%s names no survivor of the code as it is now: %s", failure.Identity, why),
		"Except the survivor from a report of the code as it is now, such as the code proof of the next push, or name its site, <file>:<line>:<column>, once mutation run has recorded it.").
		With("file", file).With("function", failure.Function).With("identity", failure.Identity).With("why", failure.Why)
	if failure.Why == mutants.ExceptionChanged {
		p = p.With("hash", failure.Hash)
	}
	return p
}

// reasonMissing is mutation except's site given with no --reason, which
// re-points no stale entry whose reason it would keep.
func reasonMissing() *cli.Problem {
	return cli.NewProblem(cli.CodeUsage, "flags.value-missing", "--reason is required: an exception says why no test can kill its mutant",
		"Give it, such as --reason 'the loop only counts up'.").With("flag", "--reason")
}

// flagConflict is flag given beside another it cannot be, as message says.
func flagConflict(flag, message string) *cli.Problem {
	return cli.NewProblem(cli.CodeUsage, "flags.conflict", message, "Drop the incompatible flag and run again.").With("flag", flag)
}

// countedNoGit is a counted run of flag, --count or --site, with no Git
// repository to judge, err saying why.
func countedNoGit(err error, flag string) *cli.Problem {
	return cli.NewProblem(cli.CodeEnvironment, "count.no-git", "mutation run "+flag+" needs a Git repository with a commit at HEAD: "+err.Error(),
		"Run it inside a Git repository, after committing the code to judge.")
}

// countedConfigHint opens the fix of every itos-cc.yaml a counted run
// refuses, its own fix after it where the refusal has one (slice-44): a
// counted run reads the file HEAD commits, so a fix of the working copy
// alone meets the same refusal again.
const countedConfigHint = "Fix it in a commit: a counted run reads the itos-cc.yaml HEAD holds"

// countedPlanError words the fresh plan Fresh refused, since the --since
// ref as written, of a run of flag, --count or --site; a failure or a
// cancellation is itself.
func countedPlanError(failure mutants.FreshFailure, since, flag string) error {
	switch f := failure.(type) {
	case mutants.FreshInvalid:
		own := f.Fix
		if own != "" {
			own = countedConfigHint + ". " + own
		}
		return configInvalid(f.Reason, f.Key, own, countedConfigHint+"; exclude lists globs of sources from the project root, such as **/porttest/**.")
	case mutants.FreshRefused:
		switch f.Kind {
		case mutants.FreshRepository:
			return countedNoGit(failure, flag)
		case mutants.FreshSince:
			return cli.NewProblem(cli.CodeUsage, "since.bad-ref", fmt.Sprintf("--since %s: not a commit in this repository", since),
				"Name a commit, branch, or tag that exists, such as origin/main.").With("ref", since)
		case mutants.FreshPath:
			return cli.NewProblem(cli.CodeUsage, "paths.unmatched", failure.Error(), "Name committed source paths.")
		case mutants.FreshScratch:
			return cli.NewProblem(cli.CodeNo, "count.preparation-failed", "counted preparation failed at scratch, so no mutant was judged: "+failure.Error(),
				"Let itos-cc make its private directory where git rev-parse --git-path itos names it, under this checkout or its git directory: remove a symlink or file in its place, then run again.").
				With("stage", "scratch")
		case mutants.FreshUnsupported:
			return cli.NewProblem(cli.CodeUsage, "count.unsupported-scope", "mutation run "+flag+" cannot judge this repository's committed inputs: "+failure.Error(),
				"Judge paths without it, or run mutation run without "+flag+".")
		}
	case mutants.FreshUnmatched:
		var problems []error
		for _, site := range f.Sites {
			problems = append(problems, siteUnmatched(site))
		}
		return errors.Join(problems...)
	case mutants.FreshFailed, mutants.FreshCancelled:
	}
	return failure
}

// siteUnmatched is site.unmatched: an argument of --site that names no
// mutant of the commit, and why. Only another system building its file is
// a requirement not met, 82; every other why is the caller's, 80.
func siteUnmatched(site mutants.UnmatchedSite) *cli.Problem {
	code, why, fix := cli.CodeUsage, "", "Name a mutant of the code HEAD commits, as mutation run lists it or a report of the code as it is now names it, and commit the code first."
	switch site.Why {
	case mutants.UnmatchedNoSite:
		why = "no mutation site is at that line and column of a source"
		fix = "Name a mutation site, as mutation list prints it: its file, line and column, or its identity."
	case mutants.UnmatchedChanged:
		why = "its function changed since, its hash now " + site.Hash
	case mutants.UnmatchedExcluded:
		why = "its source is one the exclude of itos-cc.yaml leaves out"
		fix = "Name a site of a source mutation run judges, or take its file out of exclude in a commit first."
	case mutants.UnmatchedNotBuilt:
		code, why = cli.CodeEnvironment, "only another system builds its file; this is "+site.Platform.String()
		fix = "Run it where the file is built."
	default:
		why = "its file is not in the commit, or its function or site is gone"
	}
	p := cli.NewProblem(code, "site.unmatched", fmt.Sprintf("--site %s names no mutant, so nothing was judged: %s", site.Argument, why), fix).
		With("argument", site.Argument).With("why", site.Why)
	if site.Why == mutants.UnmatchedChanged {
		p = p.With("hash", site.Hash)
	}
	if site.Why == mutants.UnmatchedNotBuilt {
		p = p.With("platform", site.Platform.String())
	}
	return p
}

// freshPreparationMessage says why counted preparation failed, the
// committed config's invalid reason as cmd worded it.
func freshPreparationMessage(failure mutants.FreshFailure) string {
	switch f := failure.(type) {
	case mutants.FreshInvalid:
		return "read frozen committed config: itos-cc.yaml: " + f.Reason
	case mutants.FreshRefused, mutants.FreshFailed, mutants.FreshCancelled, mutants.FreshUnmatched:
		return f.Error()
	}
	return ""
}

// seedError words the HEAD commit sample could not seed its draw with.
func seedError(failure mutants.HistoryFailure) error {
	switch failure := failure.(type) {
	case mutants.NoRepository:
		return cli.NewProblem(cli.CodeEnvironment, "sample.no-git", "mutation sample seeds its draw with the HEAD commit's id, so with no --seed it needs a git repository with a commit: "+failure.Reason, "Run it inside a git repository, or give the seed with --seed.")
	case mutants.BadReference:
		return failure
	case mutants.HistoryCancelled:
		return failure.Cause
	case mutants.HistoryFailed:
		return failure.Cause
	}
	return failure
}

func sinceError(failure mutants.HistoryFailure, ref string) error {
	switch failure := failure.(type) {
	case mutants.BadReference:
		return cli.NewProblem(cli.CodeUsage, "since.bad-ref", fmt.Sprintf("--since %s: not a commit in this repository", ref), "Name a branch, tag, or commit, such as origin/main; fetch a remote one first.").With("ref", ref)
	case mutants.NoRepository:
		return cli.NewProblem(cli.CodeEnvironment, "since.no-git", "--since needs a git repository: "+failure.Reason, "Run it inside a git repository, or name the paths instead.")
	case mutants.HistoryCancelled:
		return failure.Cause
	case mutants.HistoryFailed:
		return failure.Cause
	}
	return failure
}

// RunError words the domain's run failures as cmd's run and sample's
// handler word them: a snapshot that cannot be read, the listed tests'
// list command failing and their coverage run failing. Any other error is
// err.
func RunError(err error) error {
	if worded := runFailure(err); worded != nil {
		return worded
	}
	return err
}

// runFailure is err worded as RunError words it, or nil when err is none of
// the run failures.
func runFailure(err error) error {
	var read mutants.SnapshotReadFailure
	if errors.As(err, &read) {
		return snapshotReadError(read)
	}
	var listing mutants.ListingFailed
	if errors.As(err, &listing) {
		return cli.NewProblem(cli.CodeNo, "tests.list-failed",
			fmt.Sprintf("the list command of mutation.tests in %s failed (%s), so no mutant was judged: %s", "itos-cc.yaml", listing.Reason, listing.Command),
			"Make the command print the tests, one a line, and exit 0, then run again.").With("command", listing.Command).With("exit_code", listing.ExitCode)
	}
	var failed mutants.CoverageRunFailed
	if errors.As(err, &failed) {
		p := cli.NewProblem(cli.CodeNo, "tests.coverage-failed",
			fmt.Sprintf("the listed tests of mutation.tests in %s failed their coverage run without any mutant (%s), so no mutant was judged: %s", "itos-cc.yaml", failed.Reason, failed.Command),
			"Make the listed tests pass when they run with ITOS_CC_TEST_COVERDIR set and GOCOVERDIR unset, then run again.").With("command", failed.Command).With("exit_code", failed.ExitCode)
		if failed.Cleanup != nil {
			// The command's owned group that did not end stands beside it.
			return errors.Join(p, coverage.Unmeasured{Cause: *failed.Cleanup, Reason: failed.Cleanup.Reason})
		}
		return p
	}
	return nil
}

// runFailed marks an error the run's UI words as a run failure (RunError),
// where cmd's handler worded it so: what the domain's check, measurement,
// listing, run and orphan removal return once the run reaches them.
type runFailed struct{ err error }

func (f runFailed) Error() string { return f.err.Error() }
func (f runFailed) Unwrap() error { return f.err }

// ranInto marks err, when there is one, as a run failure.
func ranInto(err error) error {
	if err == nil {
		return nil
	}
	return runFailed{err: err}
}

// runError words what run's handler returns: the slice's failures, then a
// run failure where the handler marked one, then settings failures, in the
// words, subject and exit code the cmd handler gave them.
func runError(err error, rel func(string) string) error {
	var failure Failure
	if errors.As(err, &failure) {
		return failureError(failure, rel)
	}
	var failed runFailed
	if errors.As(err, &failed) {
		if worded := runFailure(failed.err); worded != nil {
			return worded
		}
		err = failed.err
	}
	var settings mutants.SettingsFailure
	if errors.As(err, &settings) {
		return settingsError(settings)
	}
	return err
}

// countedError words what a counted run's handler returns: the slice's
// failures, and any other error as itself, as the cmd handler gave it.
func countedError(err error, rel func(string) string) error {
	var failure Failure
	if errors.As(err, &failure) {
		return failureError(failure, rel)
	}
	return err
}

// sampleError words what sample's handler returns: the slice's failures,
// the domain's run failures, then settings failures, in the words, subject
// and exit code the cmd handler gave them.
func sampleError(err error, rel func(string) string) error {
	var failure Failure
	if errors.As(err, &failure) {
		return failureError(failure, rel)
	}
	if worded := runFailure(err); worded != nil {
		return worded
	}
	var settings mutants.SettingsFailure
	if errors.As(err, &settings) {
		return settingsError(settings)
	}
	return err
}

func readError(err error, rel func(string) string) error {
	if err == nil {
		return nil
	}
	var failure Failure
	if errors.As(err, &failure) {
		return failureError(failure, rel)
	}
	var read mutants.SnapshotReadFailure
	if errors.As(err, &read) {
		return snapshotReadError(read)
	}
	var listing mutants.ListingFailed
	if errors.As(err, &listing) {
		// check runs the list only to date a kill by a test listed with a
		// fingerprint whose file changed.
		return cli.NewProblem(cli.CodeNo, "tests.list-failed",
			fmt.Sprintf("the list command of mutation.tests in %s failed (%s), so the kills of tests listed with a fingerprint could not be dated: %s", "itos-cc.yaml", listing.Reason, listing.Command),
			"Make the command print the tests, one a line, and exit 0, then run again.").With("command", listing.Command).With("exit_code", listing.ExitCode)
	}
	var settings mutants.SettingsFailure
	if errors.As(err, &settings) {
		return settingsError(settings)
	}
	return err
}

// exceptError words what except's and renew's handlers return: the slice's
// failures, then the writer's, an invalid document with fix, then the
// domain's run failures, then settings failures, in the words, subject and
// exit code the cmd handler gave them.
func exceptError(err error, rel func(string) string, fix string) error {
	var failure Failure
	if errors.As(err, &failure) {
		return failureError(failure, rel)
	}
	var update mutants.ExceptionFailure
	if errors.As(err, &update) {
		return updateError(update, fix)
	}
	if worded := runFailure(err); worded != nil {
		return worded
	}
	var settings mutants.SettingsFailure
	if errors.As(err, &settings) {
		return settingsError(settings)
	}
	return err
}

// updateError words the Exceptions writer's failure: an invalid document
// is config.invalid with fix, and an I/O failure or a cancellation its cause.
func updateError(failure mutants.ExceptionFailure, fix string) error {
	switch failure := failure.(type) {
	case mutants.ExceptionInvalid:
		return configInvalid(failure.Reason, failure.Key, failure.Fix, fix)
	case mutants.ExceptionFailed:
		return failure.Cause
	case mutants.ExceptionCancelled:
		return failure.Cause
	}
	return failure
}

// snapshotReadError names the snapshot that could not be read from
// .metrics, as metricsdir names it, with why.
func snapshotReadError(read mutants.SnapshotReadFailure) error {
	return fmt.Errorf("%s: %w", filepath.Join("mutate", filepath.ToSlash(read.Key)+".json"), read.Cause)
}
