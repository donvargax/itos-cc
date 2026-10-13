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
// does, before it measured the files.
type Cancelled struct{}

func (Cancelled) coverageGap()                   {}
func (Cancelled) String() string                 { return "cancelled" }
func (g Cancelled) MarshalText() ([]byte, error) { return []byte(g.String()), nil }
