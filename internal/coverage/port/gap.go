package port

// Gap is why coverage that was meant to be measured was not. Its text is
// the cause --json writes.
//
//sumtype:decl
type Gap interface {
	// String is the gap's name, as --json writes it.
	String() string
	// MarshalText is String's bytes, so a Gap is its name in JSON.
	MarshalText() ([]byte, error)
	coverageGap()
}

// ToolMissing is a gap of a project that lacks a tool coverage needs, such
// as Vitest.
type ToolMissing struct{}

func (ToolMissing) coverageGap()                   {}
func (ToolMissing) String() string                 { return "tool-missing" }
func (g ToolMissing) MarshalText() ([]byte, error) { return []byte(g.String()), nil }

// MeasuredNothing is a gap of a coverage run that ran and measured none of
// its files, as when the tests do not compile.
type MeasuredNothing struct{}

func (MeasuredNothing) coverageGap()                   {}
func (MeasuredNothing) String() string                 { return "measured-nothing" }
func (g MeasuredNothing) MarshalText() ([]byte, error) { return []byte(g.String()), nil }

// NoReport is a gap of --use-existing-coverage finding no report on disk
// that measures the files.
type NoReport struct{}

func (NoReport) coverageGap()                   {}
func (NoReport) String() string                 { return "no-report" }
func (g NoReport) MarshalText() ([]byte, error) { return []byte(g.String()), nil }

// Unreadable is a gap of a report that cannot be read, as one named on the
// command line that is not there.
type Unreadable struct{}

func (Unreadable) coverageGap()                   {}
func (Unreadable) String() string                 { return "unreadable" }
func (g Unreadable) MarshalText() ([]byte, error) { return []byte(g.String()), nil }

// Cancelled is a gap of a measurement the context stopped, as an interrupt
// does, before it measured the files: Cause is the context's cause, which
// tells an interrupt from any other stop, and Cleanup the owned process
// group of the command it stopped whose cleanup did not finish, nil for
// none.
type Cancelled struct {
	Cause   error
	Cleanup *CleanupFailed
}

// Unwrap is the context's cause.
func (g Cancelled) Unwrap() error { return g.Cause }

func (Cancelled) coverageGap()                   {}
func (Cancelled) String() string                 { return "cancelled" }
func (g Cancelled) MarshalText() ([]byte, error) { return []byte(g.String()), nil }

// CleanupFailed is a gap of a measurement whose command's owned process
// group did not end by its cleanup deadline (ADR-0021): ProcessGroup and
// Command name the group and the command, and Reason why, the runner's
// text whole. It is also what running the command returns.
type CleanupFailed struct {
	ProcessGroup int
	Command      string
	Reason       string
}

func (CleanupFailed) coverageGap()                   {}
func (CleanupFailed) String() string                 { return "cleanup-failed" }
func (g CleanupFailed) MarshalText() ([]byte, error) { return []byte(g.String()), nil }
func (g CleanupFailed) Error() string                { return g.Reason }
