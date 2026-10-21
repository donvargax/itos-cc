package porttest

import (
	"context"
	"slices"

	"github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// Settings holds independent current semantic settings, without a wire codec
// or expected call script. Each read returns owned slices and test settings.
type Settings struct {
	Value   mutants.Config
	Failure mutants.SettingsFailure
}

func (s *Settings) Read(ctx context.Context) (*mutants.Config, mutants.SettingsFailure) {
	if ctx.Err() != nil {
		return nil, mutants.InputCancelled{Cause: context.Cause(ctx)}
	}
	if s.Failure != nil {
		return nil, s.Failure
	}
	value := s.Value
	value.Selection.Exclude = slices.Clone(value.Selection.Exclude)
	value.Exceptions = slices.Clone(value.Exceptions)
	if value.Tests != nil {
		tests := *value.Tests
		tests.Support = slices.Clone(tests.Support)
		value.Tests = &tests
	}
	return &value, nil
}
