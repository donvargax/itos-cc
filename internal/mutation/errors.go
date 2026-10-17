package mutation

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/mutants"
)

// The UI's classification of what list's and check's handlers return: the
// slice's own failures, settings and history failures, and snapshot reads.
// Each keeps the words, subject and exit code the cmd handlers gave it.

func settingsError(failure mutants.SettingsFailure) error {
	switch failure := failure.(type) {
	case mutants.SettingsInvalid:
		return configInvalid(failure.Reason, failure.Key, "Fix it: exclude lists globs of sources from the project root, such as **/porttest/**; each entry under mutation.exceptions needs file, function, hash, line_in_function, column, original, replacement, and reason, and 'itos-cc mutation except' writes one; mutation.tests needs list, run with {pattern}, ids_pattern with {ids}, and join with each, holding {id}, and sep.")
	case mutants.InputFailed:
		return failure.Cause
	case mutants.InputCancelled:
		return failure.Cause
	}
	return failure
}

// configInvalid is config.invalid for an itos-cc.yaml that cannot be read,
// for reason, with fix: its subject the file, and the key at fault when one
// is.
func configInvalid(reason, key, fix string) *cli.Problem {
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
		file := rel(failure.Path)
		return cli.NewProblem(cli.CodeUsage, "exception.excluded",
			fmt.Sprintf("%s:%d:%d is in %s, a source the exclude of %s leaves out, so none of its mutants is judged or excepted", file, failure.Line, failure.Column, file, "itos-cc.yaml"),
			fmt.Sprintf("Except a survivor of a source mutation run judges, or take %s out of exclude in %s first.", file, "itos-cc.yaml")).
			With("file", file)
	case NoSurvivor:
		file := rel(failure.Path)
		return cli.NewProblem(cli.CodeUsage, "exception.no-survivor", fmt.Sprintf("%s:%d:%d is no survivor to except: %s", file, failure.Line, failure.Column, failure.Reason),
			fmt.Sprintf("Except a site that mutation run lists as survived; run 'itos-cc mutation run %s' first when its results are stale or missing.", file)).
			With("file", file).With("line", failure.Line).With("column", failure.Column)
	}
	return failure
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
// list command failing and a coverage command given no report. Any other
// error is err.
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
	var required mutants.CoverageReportRequired
	if errors.As(err, &required) {
		return cli.CommandNeedsReport()
	}
	return nil
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
		return configInvalid(failure.Reason, failure.Key, fix)
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
