package mutants

import "context"

// Config is the semantic content of the project's settings. Its codec is
// outside the domain; these are the existing globs, tests and exceptions.
type Config struct {
	// Exclude are doublestar globs, slash-separated and from the project
	// root, of the sources every command leaves out of what it measures and
	// mutates; never of the tests.
	Exclude    []string
	Exceptions []Exception
	// Tests, when set, is how to list the project's tests and run a
	// selection of them.
	Tests *Tests
}

// SettingsReader rereads the current settings at each admission boundary.
// The root belongs to that actual observation, not to a cached CLI snapshot.
type SettingsReader interface {
	Read(ctx context.Context) (root string, config *Config, failure SettingsFailure)
}

// SettingsFailure distinguishes invalid content from actual I/O and cancellation.
//
//sumtype:decl
type SettingsFailure interface {
	error
	settingsFailure()
}

type SettingsInvalid struct{ Reason, Key string }

func (SettingsInvalid) settingsFailure() {}
func (f SettingsInvalid) Error() string  { return f.Reason }
func (InputFailed) settingsFailure()     {}
func (InputCancelled) settingsFailure()  {}
