package port

// Request is what to measure and how, as crap's coverage flags say.
type Request struct {
	// Sources are the files to measure, as the selection discovered them.
	Sources []string
	// Reports, --coverage-report, are the reports to read instead of
	// running the tests, each path absolute or from the working directory.
	// One that cannot be read is Missing as Unreadable.
	Reports []string
	// Command, --coverage-command, is a command line run through the
	// platform shell before Reports are read, which writes them: one it
	// did not write measured nothing (MeasuredNothing). It is given only
	// with Reports, which say where its reports land.
	Command string
	// Existing, --use-existing-coverage, reads the reports an earlier run
	// left on disk instead of running the tests: a build root with none is
	// Missing as NoReport.
	Existing bool
	// AllTests, --all-tests, runs each build root's whole suite, whatever
	// Scope says.
	AllTests bool
	// Scope is which tests measure the sources without AllTests.
	Scope Scope
	// Reach is the test files that reach a source, which measure Python's
	// and Kotlin's sources in OwnTests; nil measures them by the whole
	// suite.
	Reach Reach
}

// Scope is which tests measure the sources.
type Scope int

const (
	// AllTests runs each build root's whole suite.
	AllTests Scope = iota
	// RelatedTests runs, in Go and TypeScript, only the tests that load the
	// sources, which measures them the same as the whole suite: a test that
	// does not load a file cannot cover it.
	RelatedTests
	// OwnTests measures each Go package by its own tests only, the tests
	// mutation kills its mutants with; TypeScript runs the related tests, and
	// Python and Kotlin, given the tests that reach each source, those
	// tests.
	OwnTests
)

// Reach is the test files that reach a source, as graph.TestsImporting
// finds them. It answers from what it already holds: it reads nothing.
type Reach func(source string) []string

// Failure is why Measure gave no report at all, while a build root or
// report it could not measure is in the report's Missing.
//
//sumtype:decl
type Failure interface {
	error
	measureFailure()
}

// Cancelled, the gap of a measurement the context stopped, is also
// Measure's failure once ctx is done: it measured nothing, or stopped part
// way and keeps nothing it measured.
func (Cancelled) measureFailure() {}
func (g Cancelled) Error() string { return g.String() }

// Unwritable is a measurement whose run folder, where the tools write
// their reports under .metrics/coverage, could not be made. Reason is the
// operating system's error text whole.
type Unwritable struct{ Reason string }

func (Unwritable) measureFailure() {}
func (e Unwritable) Error() string { return e.Reason }
