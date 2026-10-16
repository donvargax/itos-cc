package porttest

import (
	"context"
	"slices"

	"github.com/donvargax/itos-cc/internal/mutants"
)

// Settings holds independent current semantic settings, without a wire codec
// or expected call script. Each read returns owned slices and test settings.
type Settings struct {
	Root    string
	Value   mutants.Config
	Failure mutants.SettingsFailure
}

func (s *Settings) Read(ctx context.Context) (string, *mutants.Config, mutants.SettingsFailure) {
	if ctx.Err() != nil {
		return s.Root, nil, mutants.InputCancelled{Cause: context.Cause(ctx)}
	}
	if s.Failure != nil {
		return s.Root, nil, s.Failure
	}
	value := s.Value
	value.Exclude = slices.Clone(value.Exclude)
	value.Exceptions = slices.Clone(value.Exceptions)
	if value.Tests != nil {
		tests := *value.Tests
		tests.Support = slices.Clone(tests.Support)
		value.Tests = &tests
	}
	return s.Root, &value, nil
}
