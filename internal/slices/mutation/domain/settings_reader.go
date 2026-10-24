package domain

import (
	"context"

	selport "github.com/donvargax/itos-cc/internal/selection/port"
)

// Config is the semantic content of the project's settings. Its codec is
// outside the domain; these are the selection settings, tests and
// exceptions of one observation of them.
type Config struct {
	// Selection is the selection engine's settings: the project root this
	// observation found the settings at and the exclude globs, the sources
	// every command leaves out of what it measures and mutates.
	Selection  selport.Settings
	Exceptions []Exception
	// Tests, when set, is how to list the project's tests and run a
	// selection of them.
	Tests *Tests
}

// SettingsReader rereads the current settings at each admission boundary.
// The root, Selection's, belongs to that actual observation, not to a cached
// CLI snapshot.
type SettingsReader interface {
	Read(ctx context.Context) (config *Config, failure SettingsFailure)
}

// SettingsFailure distinguishes settings that cannot be read, invalid, from a
// read that failed short of them, such as a missing binding, and cancellation.
//
//sumtype:decl
type SettingsFailure interface {
	error
	settingsFailure()
}

// SettingsInvalid is settings that cannot be read: a file that could not be
// read, or content that is not valid, Reason saying why, Key naming the
// key at fault where one is, and Fix how to mend it where the refusal has
// its own mending.
type SettingsInvalid struct{ Reason, Key, Fix string }

func (SettingsInvalid) settingsFailure() {}
func (f SettingsInvalid) Error() string  { return f.Reason }
func (InputFailed) settingsFailure()     {}
func (InputCancelled) settingsFailure()  {}
