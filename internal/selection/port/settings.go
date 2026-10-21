package port

import "context"

// Settings is the selection engine's part of the project's settings, as one
// observation of them read it: Root, the project root they were found at,
// and Exclude, doublestar globs, slash-separated and from Root, of the
// sources every command leaves out of what it measures and mutates; never
// of the tests (itos-cc.yaml's exclude, decision 45). The zero Settings
// excludes nothing.
type Settings struct {
	Root    string
	Exclude []string
}

// SettingsReader rereads the selection settings each time it is asked, so
// a command observes the file as it is when it reads it, never a snapshot
// cached beside the command line. ctx carries only cancellation. The
// Settings returned are the caller's own: their globs share nothing with
// another read's. porttest.SettingsContract holds what every one, the fake
// and the codec's, must do.
type SettingsReader interface {
	ReadSelection(ctx context.Context) (Settings, SettingsFailure)
}

// SettingsFailure is why the selection settings could not be read:
// settings that cannot be read, invalid, a read that failed short of them,
// such as a missing binding, or cancellation.
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

func (*SettingsInvalid) settingsFailure() {}
func (e *SettingsInvalid) Error() string  { return e.Reason }

// SettingsFailed is a read that failed short of the settings, Cause saying
// why, as a missing binding does.
type SettingsFailed struct{ Cause error }

func (*SettingsFailed) settingsFailure() {}
func (e *SettingsFailed) Error() string  { return e.Cause.Error() }

// Unwrap is why the read failed.
func (e *SettingsFailed) Unwrap() error { return e.Cause }

func (*Cancelled) settingsFailure() {}
