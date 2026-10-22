package domain

import "context"

// CommandRequest asks for the tests of one source. Tests is the actual import
// reach inventory, not an expected execution list. Shell overrides argv only.
type CommandRequest struct {
	Path, Shell string
	All         bool
	Tests       []string
}

// ToolFacts are observations used to choose a command. Optional absent tools,
// unreadable manifests and unsuccessful probes are negative facts, not failures.
// PythonFiles/Modules, DartFiles and Classes are the existing runners'
// runnable inventory. Flutter starts the flutter that runs a Dart package's
// tests: its program, then the global flags every flutter command carries.
type ToolFacts struct {
	Language, Root, Module, Reactor string
	PackageManager, Vitest, Jest    string
	Interpreter                     string
	Pytest                          bool
	PythonFiles, PythonModules      []string
	Flutter, DartFiles              []string
	Runner                          string
	Classes                         map[string][]string
}

// Toolchain observes project manifests, installed tools, actual runnable test
// names and optional interpreter probes.
type Toolchain interface {
	Observe(ctx context.Context, request CommandRequest) (ToolFacts, InputFailure)
	// CoverageRunner observes the existing built-in collector preference separately from test argv.
	CoverageRunner(ctx context.Context, source string, related bool) (string, InputFailure)
	// GoPlatform is the GOOS and GOARCH go env reports in dir, in the
	// run's environment: the platform a Go build there is for.
	GoPlatform(ctx context.Context, dir string) (Platform, InputFailure)
}

// Fingerprints records actual content and runtime inputs. Its inventory is
// provenance, deliberately broader than the mutation sites a run may select.
type Fingerprints interface {
	// Unsupported admits no external dependency tree; ordinary in-root discovery remains separate.
	Unsupported(ctx context.Context, language, source, projectRoot string) (string, InputFailure)
	Tests(ctx context.Context, root string, tests []string) (map[string]string, InputFailure)
	Support(ctx context.Context, root string, globs []string) (map[string]string, InputFailure)
	Suite(ctx context.Context, source, root string) (map[string]string, InputFailure)
	Coverage(ctx context.Context, source, root, producer string, support map[string]string) (map[string]string, InputFailure)
}

// Reach resolves the tests importing each source using the actual supplied
// source/test inventory. Go retains its one-hop policy; other languages use
// their current transitive imports and implicit test support.
type Reach interface {
	TestsImporting(ctx context.Context, root string, sources, tests []string) (map[string][]string, InputFailure)
}

// InputFailure is a surfaced observation failure, not an optional missing tool.
//
//sumtype:decl
type InputFailure interface {
	error
	inputFailure()
}

type InputCancelled struct{ Cause error }

func (InputCancelled) inputFailure()   {}
func (f InputCancelled) Error() string { return f.Cause.Error() }
func (f InputCancelled) Unwrap() error { return f.Cause }

type InputFailed struct{ Cause error }

func (InputFailed) inputFailure()   {}
func (f InputFailed) Error() string { return f.Cause.Error() }
func (f InputFailed) Unwrap() error { return f.Cause }

// RuntimeInputs are actual runtime/environment observations at the caller's
// boundary. Fingerprints use only their existing language-specific key sets.
// Memory receives Run.Env overrides rather than consulting the host process.
// No Go version is among them: Go evidence records the project toolchain's,
// as go env resolves it in the module, never the one itos-cc was built with.
type RuntimeInputs struct {
	Platform, Architecture string
	Env                    map[string]string
}
