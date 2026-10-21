package mutants

import (
	"errors"
	"path/filepath"
	"strings"
)

func (opt Options) bound() error {
	if opt.Store == nil {
		return errors.New("mutation snapshots binding is missing")
	}
	if opt.Tree == nil || opt.Surroundings == nil {
		return errors.New("mutation source tree binding is missing")
	}
	if opt.Toolchain == nil {
		return errors.New("mutation Toolchain binding is missing")
	}
	if opt.Fingerprints == nil {
		return errors.New("mutation Fingerprints binding is missing")
	}
	if opt.WorkerFactory == nil {
		return errors.New("mutation Workers binding is missing")
	}
	if opt.Annotate && opt.Source == nil || opt.Renamed != nil && opt.Source == nil {
		return errors.New("mutation source edits binding is missing")
	}
	return nil
}

// SnapshotReadFailure preserves the admitted source key and original failure.
// Physical snapshot naming and the existing diagnostic belong to presentation.
type SnapshotReadFailure struct {
	Key   string
	Cause error
}

func (f SnapshotReadFailure) Error() string { return f.Key + ": " + f.Cause.Error() }
func (f SnapshotReadFailure) Unwrap() error { return f.Cause }

func relativeName(dir, path string) string {
	rel, err := filepath.Rel(dir, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return rel
}
