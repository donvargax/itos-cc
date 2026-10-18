package config

import (
	"context"
	"errors"
	"io/fs"

	"github.com/donvargax/itos-cc/internal/mutants"
)

// Source supplies current raw configuration at its actual project root.
// Cmd selects the disk or memory byte source; Reader owns the single codec.
// Its error is what kept the file there from being read.
type Source interface {
	Read(ctx context.Context) (root string, text []byte, err error)
}

// Reader adapts the existing config codec to mutation's settings capability.
// It performs no eager read and stores no context or parsed snapshot. The source
// owns cancellation of its actual read; a completed read is still parsed in the
// old order, including disk reads which cannot be interrupted by a context.
// A file the source could not read is invalid settings, as one that cannot be
// parsed is (bug-4), unless the context's cancellation stopped the read.
type Reader struct{ Source Source }

func (r Reader) Read(ctx context.Context) (string, *mutants.Config, mutants.SettingsFailure) {
	if r.Source == nil {
		return "", nil, mutants.InputFailed{Cause: errors.New("mutation settings byte source binding is missing")}
	}
	root, text, err := r.Source.Read(ctx)
	if err != nil {
		if ctx.Err() != nil && errors.Is(err, context.Cause(ctx)) {
			return root, nil, mutants.InputCancelled{Cause: context.Cause(ctx)}
		}
		return root, nil, mutants.SettingsInvalid{Reason: unreadable(err)}
	}
	config, err := Parse(text)
	if err != nil {
		var invalid *InvalidError
		if errors.As(err, &invalid) {
			return root, nil, mutants.SettingsInvalid{Reason: invalid.Reason, Key: invalid.Key}
		}
		return root, nil, mutants.InputFailed{Cause: err}
	}
	return root, config, nil
}

// unreadable is the reason itos-cc.yaml could not be read, err saying why:
// the system's words without the path, which the failure names as
// itos-cc.yaml.
func unreadable(err error) string {
	var path *fs.PathError
	if errors.As(err, &path) {
		err = path.Err
	}
	return "cannot be read: " + err.Error()
}
