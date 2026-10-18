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

// SettingsFailure distinguishes settings that cannot be read, invalid, from a
// read that failed short of them, such as a missing binding, and cancellation.
//
//sumtype:decl
type SettingsFailure interface {
	error
	settingsFailure()
}

// SettingsInvalid is settings that cannot be read: a file that could not be
// read, or content that is not valid, Reason saying why and Key naming the
// key at fault where one is.
type SettingsInvalid struct{ Reason, Key string }

func (SettingsInvalid) settingsFailure() {}
func (f SettingsInvalid) Error() string  { return f.Reason }
func (InputFailed) settingsFailure()     {}
func (InputCancelled) settingsFailure()  {}
