package mutation

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/mutants"
)

func settingsError(failure mutants.SettingsFailure) error {
	switch failure := failure.(type) {
	case mutants.SettingsInvalid:
		p := cli.NewProblem(cli.CodeUsage, "config.invalid", "itos-cc.yaml: "+failure.Reason, "Fix it: exclude lists globs of sources from the project root, such as **/porttest/**; each entry under mutation.exceptions needs file, function, hash, line_in_function, column, original, replacement, and reason, and 'itos-cc mutation except' writes one; mutation.tests needs list, run with {pattern}, ids_pattern with {ids}, and join with each, holding {id}, and sep.").With("file", "itos-cc.yaml")
		if failure.Key != "" {
			p.With("key", failure.Key)
		}
		return p
	case mutants.InputFailed:
		return failure.Cause
	case mutants.InputCancelled:
		return failure.Cause
	}
	return failure
}
func readError(err error) error {
	if err == nil {
		return nil
	}
	var read mutants.SnapshotReadFailure
	if errors.As(err, &read) {
		return fmt.Errorf("%s: %w", filepath.Join("mutate", filepath.ToSlash(read.Key)+".json"), read.Cause)
	}
	var failure mutants.SettingsFailure
	if errors.As(err, &failure) {
		return settingsError(failure)
	}
	return err
}
