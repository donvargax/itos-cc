package config

import (
	"context"
	"errors"

	"github.com/donvargax/itos-cc/internal/mutants"
)

// Source supplies current raw configuration at its actual project root.
// Cmd selects the disk or memory byte source; Reader owns the single codec.
type Source interface {
	Read(ctx context.Context) (root string, text []byte, err error)
}

// Reader adapts the existing config codec to mutation's settings capability.
// It performs no eager read and stores no context or parsed snapshot. The source
// owns cancellation of its actual read; a completed read is still parsed in the
// old order, including disk reads which cannot be interrupted by a context.
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
		return root, nil, mutants.InputFailed{Cause: err}
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
