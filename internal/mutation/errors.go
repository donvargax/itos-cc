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
		p := cli.NewProblem(cli.CodeUsage, "config.invalid", "itos-cc.yaml: "+failure.Reason, "Fix it: exclude lists globs of sources from the project root, such as **/porttest/**; each entry under mutation.exceptions needs file, function, hash, line_in_function, column, original, replacement, and reason, and 'itos-cc mutation except' writes one; mutation.tests needs list, run with {pattern}, ids_pattern with {ids}, and join with each, holding {id}, and sep.").With("file", "itos-cc.yaml")
		if failure.Key != "" {
			p = p.With("key", failure.Key)
		}
		return p
	case mutants.InputFailed:
		return failure.Cause
	case mutants.InputCancelled:
		return failure.Cause
	}
	return failure
}

func failureError(failure Failure) error {
	switch failure := failure.(type) {
	case LinesWithoutSince:
		return cli.NewProblem(cli.CodeUsage, "flags.conflict", "--fail-uncovered=lines judges only the lines a --since range changed, and was given without --since", "Add --since REF, such as --since origin/main, or write --fail-uncovered alone to judge whole functions.").With("flag", "--fail-uncovered=lines")
	case SinceWithChanged:
		return cli.NewProblem(cli.CodeUsage, "flags.conflict", "--since and --changed cannot be combined: --since judges the functions of commits, --changed whole files of the working tree", "Drop --changed; commit the work to judge it with --since.").With("flag", "--changed")
	case SinceUnavailable:
		return sinceError(failure.Failure, failure.Ref)
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

func readError(err error) error {
	if err == nil {
		return nil
	}
	var failure Failure
	if errors.As(err, &failure) {
		return failureError(failure)
	}
	var read mutants.SnapshotReadFailure
	if errors.As(err, &read) {
		return fmt.Errorf("%s: %w", filepath.Join("mutate", filepath.ToSlash(read.Key)+".json"), read.Cause)
	}
	var settings mutants.SettingsFailure
	if errors.As(err, &settings) {
		return settingsError(settings)
	}
	return err
}
