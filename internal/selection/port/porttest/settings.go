package porttest

import (
	"context"
	"slices"

	"github.com/donvargax/itos-cc/internal/selection/port"
)

// Settings holds the current selection settings in memory, without a wire
// codec or an expected call script: Value, what each read gives, or
// Failure, its failure when set. Each read returns its own globs.
type Settings struct {
	Value   port.Settings
	Failure port.SettingsFailure
}

var _ port.SettingsReader = &Settings{}

// ReadSelection is Value, its globs the caller's own, Failure when it is
// set, or Cancelled once ctx is done.
func (s *Settings) ReadSelection(ctx context.Context) (port.Settings, port.SettingsFailure) {
	if ctx.Err() != nil {
		return port.Settings{}, &port.Cancelled{Cause: context.Cause(ctx)}
	}
	if s.Failure != nil {
		return port.Settings{}, s.Failure
	}
	value := s.Value
	value.Exclude = slices.Clone(value.Exclude)
	return value, nil
}
