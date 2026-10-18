// The steps. itos-cc is a black box here: each scenario gets a fresh
// scratch folder, runs itos-cc in it, and reads its exit code and its
// output. The steps reach that world only through a Driver: the binary one
// (binary.go) runs the itos-cc TestFeatures built from this tree as another
// process on the disk, and cmd/itos-cc's tests run the same steps in
// process (decision 44). Nothing here imports or reads itos-cc's code, so
// the steps judge it by its command line alone. Harvested from itos's
// (github.com/donvargax/itos, features/steps_test.go) and the owner's
// itos-template's, its domain's steps left behind.
package features

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// Driver is the world the steps reach: folders and files, git, and runs of
// itos-cc. Every path it takes is absolute, in the system's form.
type Driver interface {
	// Folder makes a fresh, empty folder and gives its absolute path.
	Folder() (string, error)
	// Write writes data to the file at path, making the folders above it.
	Write(path string, data []byte) error
	// WriteExecutable writes owned fixture bytes and makes the file executable,
	// including when it replaces an ordinary non-executable file.
	WriteExecutable(path string, data []byte) error
	// Read is the content of the file at path.
	Read(path string) ([]byte, error)
	// Mkdir makes the folder at path, whose parent is there.
	Mkdir(path string) error
	// Exists is whether a file or folder is at path.
	Exists(path string) bool
	// Remove removes the file or folder at path and everything below it.
	Remove(path string) error
	// Files inventories actual regular files under dir, sorted slash paths relative
	// to dir, without following directory links.
	Files(ctx context.Context, dir string) ([]string, error)
	// Lock exclusively locks an existing file without truncating it. The returned
	// release closes the owned descriptor. Real OS locking is binary-only.
	Lock(ctx context.Context, path string) (release func() error, err error)
	// SetModTime updates an existing owned fixture entry and returns its observed
	// modification time. Cancellation is checked before I/O.
	SetModTime(ctx context.Context, path string, when time.Time) (time.Time, error)
	// Git runs git with args in the folder dir, for a fixture that needs a
	// repository: init, add and commit.
	Git(dir string, args ...string) error
	// Head observes the current Git commit independently of command output.
	Head(ctx context.Context, dir string) (string, error)
	// Run runs itos-cc as r says and gives what it did. Its error is a run
	// that could not be made, never an exit code other than 0.
	Run(r Run) (Result, error)
	// Platform is the actual host OS at the driver boundary, never a test override.
	Platform() string
	// Start runs only the product under test, returning its owned asynchronous run.
	// Binary overrides are rejected. Native Windows and the memory driver report
	// ErrPhysicalUnsupported before launch; ordinary synchronous Run is unaffected.
	Start(ctx context.Context, r Run) (Running, error)
	// FixturePeers holds a listener for controlled fixture lifetime connections.
	// Native Windows and memory return ErrPhysicalUnsupported before listening;
	// cancellation takes precedence. Runtime connection failures remain errors.
	FixturePeers(ctx context.Context) (FixturePeers, error)
	// Coverage declares what the project's own tools measure in the folder
	// dir, before a run of itos-cc measures coverage there with them. The
	// binary driver checks the tools are installed, skipping the scenario
	// (godog.ErrSkip) when they are not, or failing it where the job says
	// it installed them (ITOS_CC_LANGUAGE_TOOLS), and links the pinned
	// node_modules into dir for TypeScript; the in-process driver records
	// it. A folder with no Coverage declared has no tools that measure it.
	Coverage(dir string, c Coverage) error
	// Link makes path a symbolic link to target, a folder or a file that
	// is there. Only typed unsupported/permission failures can skip; ordinary
	// I/O errors retain their cause and fail. Installed-tools mode never skips.
	// The in-process driver makes none.
	Link(target, path string) error
	// Build builds itos-cc from this tree in the folder dir as b says, as a
	// release, go install or a person's go build makes it, and gives the
	// binary for a Run to name. The binary driver skips the scenario
	// (godog.ErrSkip) where the build b names cannot be made as it says;
	// the in-process driver builds nothing.
	Build(dir string, b Build) (Built, error)
	// Occupy holds a free port of 127.0.0.1 until release is called, so a
	// command asked to listen there finds it in use. The binary driver
	// skips the scenario (godog.ErrSkip) where the system lets a second
	// listener take a port held; the in-process driver holds none.
	Occupy() (port int, release func(), err error)
	// Tests declares what the project's own tests do in the folder dir,
	// before a run of itos-cc runs them there: mutation run's baseline and
	// its trial of each mutant. The binary driver checks the tools they run
	// with are installed, skipping the scenario (godog.ErrSkip) when they
	// are not, or failing it where the job says it installed them
	// (ITOS_CC_LANGUAGE_TOOLS), as Coverage does, and skipping tests said
	// to take under 100 milliseconds where the system runs none that fast;
	// the real tests then decide every outcome. The in-process driver
	// records it for the bound Workers fake. A folder with no Tests declared
	// has no tests a run may run. Imports alone, with no Command, declares
	// the files' import adjacency, which the world does before each run.
	// Declarations upsert by directory and Command; other commands remain.
	Tests(dir string, t Tests) error
	// Tools observes independently declared installed entries and interpreter
	// import capability; it never derives availability from test outcomes.
	Tools(ctx context.Context, dir string, setup ToolSetup) error
	// Graph reads the public /api/graph of serve in r.Dir, owning and stopping
	// its server before returning. The context bounds readiness and the request.
	Graph(ctx context.Context, r Run) ([]byte, error)
	// ObserveWrites starts observing existing paths and returns their writes
	// or removals since then, including rewrites of identical bytes.
	ObserveWrites(paths ...string) (func() ([]string, error), error)
	// ObserveReads monitors only the owned paths after verifying actual reads and
	// directory listings are observable; unsupported observation returns
	// ErrReadObservationUnsupported, never an empty successful log.
	ObserveReads(paths ...string) (func() ([]string, error), error)
	// Counted says the system makes counted runs, mutation run --count,
	// which itos-cc makes on Linux and macOS alone (#29): elsewhere both
	// drivers skip the scenario (godog.ErrSkip).
	Counted() error
}

// Tests is what a project's own tests do in a folder, as a Given declares
// it.
type Tests struct {
	// Language is the language of the tests: go, typescript, python or
	// kotlin.
	Language string
	// Command is the command itos-cc runs them with, as it announces the
	// baseline's (itos-cc: baseline <dir>$ <command>), such as
	// "go test -count=1 -failfast ./src" or "npm run test --silent".
	Command string
	// Baseline is what they print when they fail with nothing mutated, ""
	// when they pass.
	Baseline string
	// Surviving are the sites whose mutants they pass, and TimingOut those
	// whose mutants make them run past the timeout; every other mutant
	// they run fails them, killed.
	Surviving, TimingOut []Site
	// Elapsed is how long one run of them takes: the baseline's, and each
	// trial's but a timing-out one's.
	Elapsed time.Duration
	// Listed describes the configured harness independently of its executable
	// fixture: IDs and defining files, coverage by source line, whether coverage
	// is split in one run, and each selection's timing or baseline failure.
	Listed *ListedTests
	// Trace names the fixture's actual execution observations, not scripted
	// calls. Each JSON line has canonical cwd/original_root and source_sha256.
	// Source is relative to the project; File is an absolute external path
	// embedded in the real tests.
	Trace *TestTrace
	// Imports is optional resolved file-to-file import/reference adjacency,
	// project-relative, independent of commands/outcomes. Nil leaves graph
	// setup unavailable; explicit empty lists mean known no edges.
	Imports map[string][]string
}

// ToolSetup describes setup facts beyond ordinary file contents. Python nil
// leaves a probe undeclared; Importable false is an explicit observed failure.
type ToolSetup struct {
	NodeBins []string
	Python   *PythonProbe
	Env      []string
}

type PythonProbe struct {
	Interpreter string
	Importable  bool
}

// TestTrace identifies an executable fixture's execution-observation file and source.
type TestTrace struct {
	File, Source string
	// Tests maps the complete runnable selector inventory to observed names.
	// Bound commands and measurement requests select executions from it; nil
	// keeps the source-only execution trace contract used by earlier fixtures.
	Tests map[string]string
	// Coverage says each record tells whether its run measured coverage, as
	// a Go harness checking testing.CoverMode records it.
	Coverage bool
	// CoverDir says each record tells the ITOS_CC_TEST_COVERDIR its run was
	// given, and the listed IDs it ran ("tests"), as a listed harness
	// records them.
	CoverDir bool
}

// ListedTests is a fixture's public test inventory and observable outcomes.
// Lines name source files relative to the declared project's folder.
type ListedTests struct {
	Files   map[string]string
	Lines   map[string]map[int][]string
	Split   bool
	Elapsed map[string]time.Duration
	// Failing is the exact selection of IDs that fails alone; a larger
	// selection containing those IDs may still pass.
	Failing []string
	// Surviving and TimingOut give each listed ID the sites its tests pass
	// or hang on; other reached sites are killed. ListFails is the inventory
	// command failing, before any trial.
	Surviving, TimingOut map[string][]Site
	ListFails            bool
	// ListOutput is successful raw inventory stdout; empty uses Files.
	ListOutput string
	// ListTrace is the file the list command records each of its runs in,
	// a JSON line whose "test" is "list"; "" when it records none.
	ListTrace string
	// CoverageExit/Output describe the actual listed collection command result.
	// CoverageMetadata, when nonnil, is written as the owned per-test covmeta
	// artifact; conversion, rather than this declaration, determines failure.
	CoverageExit     int
	CoverageOutput   string
	CoverageMetadata []byte
	// ConversionExit/Output are the actual external Go covdata conversion
	// result for the per-ID metadata artifact; other collectors remain valid.
	ConversionExit   int
	ConversionOutput string
}

// Site is a mutation site as a Given names it: its file, slash-separated
// from the folder its Tests were declared for, its line, and the text a
// mutant replaces and what replaces it, "" for a deletion.
type Site struct {
	File                  string
	Line                  int
	Original, Replacement string
}

// countedSystems are the systems itos-cc makes counted runs on (#29), by
// the name Go gives each.
var countedSystems = []string{"linux", "darwin"}

// CountedOn is whether itos-cc makes counted runs on the system goos, as
// Go names it.
func CountedOn(goos string) bool { return slices.Contains(countedSystems, goos) }

// Build is a way of building itos-cc, as a Given names it.
type Build struct {
	// How is the build: "release", go build stamping main.version with
	// Version as a release does; "install", go install of the module at
	// Version from a module proxy; "checkout", go build in a git checkout
	// one commit past its release Version; "no-git", go build in a copy of
	// the tree with no git information.
	How string
	// Version is the version the build is stamped or tagged with, "" for
	// a build with no git information.
	Version string
}

// Built is what a build made: the binary, and for a checkout's build the
// full id of the commit it was built from, "" for any other.
type Built struct {
	Binary, Commit string
}

// Coverage is what a project's own tools measure in a folder, as a Given
// declares it.
type Coverage struct {
	// Language is the language whose coverage command runs there: go,
	// typescript, python or kotlin.
	Language string
	// Report is the report the command writes, as LCOV, a Go coverprofile or
	// JaCoCo XML, its paths relative to the folder: what the in-process driver
	// hands its fake in place of running the tools. "" when it writes none,
	// as when the tests do not compile.
	Report string
	// CommandExit and CommandOutput are the real coverage tool's failed exit
	// and diagnostic, even when it wrote Report. Zero/empty preserve success.
	// The bound tool returns this failure; the actual Measurer classifies it.
	CommandExit   int
	CommandOutput string
	// Output is what the command prints when it succeeds, as its tools
	// print it, such as go test's ok line, which itos-cc passes on to its
	// stderr: the in-process driver hands it to its fake measurer, which
	// prints it there; the binary driver's real tools print their own.
	Output string
	// IntegrationReport is the actual subprocess collector report, separately
	// from Report, in the same existing formats. Empty means no subprocess data.
	IntegrationReport string
	// WithoutC8 declares TypeScript's actual missing built-in collector. Binary
	// fixtures own a facade of pinned tools excluding c8; no integration report
	// can accompany that absence. Raw reports/no-coverage remain unaffected.
	WithoutC8 bool
	// InventoryReport is independent static collector output for unloaded
	// Python/TypeScript files; empty means no inventory was obtained.
	InventoryReport string
	InventoryExit   int
	InventoryOutput string
	// Venv is whether the Python project keeps its own virtualenv in
	// .venv, whose python is the one with pytest and coverage: the binary
	// driver links python3 there.
	Venv bool
	// Program is a program the project's coverage runs besides the
	// language's tools, such as make for a Makefile's cover target; ""
	// for none.
	Program string
}

// Run is one run of itos-cc: the folder it runs in, the variables the
// scenario sets in its environment as NAME=value, the text of its standard
// input, "" for none, its arguments, the programs it finds on no folder
// of the PATH, so a command naming one never runs, as Gradle and Maven for
// a scenario that only reads which of them itos-cc chose, and the binary
// Build made that it runs, "" for the one under test.
type Run struct {
	Dir     string
	Env     []string
	Stdin   string
	Args    []string
	Without []string
	Binary  string
}

// Result is what a run of itos-cc did: its exit code and its output.
type Result struct {
	Exit           int
	Stdout, Stderr string
}

// stampedVersion is the version the binary under test is stamped with, as a
// release's build stamps the release's, so a scenario can tell the stamp
// from the version a build without one says.
const stampedVersion = "1.2.3-features"

// stampedCommit is the commit the binary under test is stamped with, as a
// release's build stamps the commit it was built from, so a scenario can
// name the commit the binary says it was built from.
const stampedCommit = "0123456789abcdef0123456789abcdef01234567"

// Converted are the feature files the steps run, those whose scenarios
// have steps: units.feature, the first command moved onto kong (T-32),
// dry.feature and scrap.feature, crap.feature, complexity.feature and
// coverage.feature, and file_selection.feature, each converted before its
// command moves so the move is held by its scenarios (T-37, T-41, T-60,
// T-61, T-62), cli.feature, whose help, version and mutation scenarios
// need commands that move later (T-62), and mutate.feature, converted in
// pieces before the mutation group moves (T-70 to T-81), the scenarios of
// the pieces not done yet listed in Unstepped. A file converts with its
// scenarios' text unchanged (q-52); the binary driver runs every one.
var Converted = []string{
	"units.feature", "dry.feature", "scrap.feature",
	"crap.feature", "complexity.feature", "coverage.feature", "file_selection.feature",
	"cli.feature", "mutate.feature", "metrics_snapshots.feature",
}

// Unstepped are the scenarios of converted files whose steps are not
// written yet, by their ID tag: mutate.feature's, too many for one task,
// whose steps come a piece at a time, the Rules of each piece in the S6
// plan's table. TagFilter treats them as scenarios of a file with no steps
// yet: no run selects them, and a selection naming them logs them. The list
// is written by hand, an entry for each piece left, in the plan's ranges,
// so a piece deletes its own entry, and the last one the list (T-81); an ID
// no converted file's scenario carries fails every selection.
var Unstepped = slices.Concat(
	// Deferred Windows ownership remains @wip, outside T-80.
	ids("MUT", "169-172"),
)

// ids are the ID tags of area's scenarios numbered as each of ranges says,
// a number or two joined by a dash, both included, each written with two
// digits at least: ids("MUT", "07-09") is @ID-MUT-07, @ID-MUT-08 and
// @ID-MUT-09.
func ids(area string, ranges ...string) []string {
	var tags []string
	for _, r := range ranges {
		first, last, _ := strings.Cut(r, "-")
		if last == "" {
			last = first
		}
		from, err := strconv.Atoi(first)
		if err != nil {
			panic(err)
		}
		to, err := strconv.Atoi(last)
		if err != nil {
			panic(err)
		}
		for n := from; n <= to; n++ {
			tags = append(tags, fmt.Sprintf("@ID-%s-%02d", area, n))
		}
	}
	return tags
}

// InProcess are the converted files the in-process driver runs: those of
// the commands moved onto kong, which run against the ports' fakes. A file
// joins it in the task that moves its command (crap's, coverage's,
// complexity's and file selection's files in T-67, mutate.feature in T-125,
// metrics_snapshots.feature once mutation's commands had moved, in T-101,
// cli.feature once version and the top-level dispatch had, in T-114); until
// then the binary driver alone runs its scenarios.
var InProcess = []string{"units.feature", "dry.feature", "scrap.feature", "crap.feature", "complexity.feature", "coverage.feature", "file_selection.feature", "mutate.feature", "metrics_snapshots.feature", "cli.feature"}

// The tags of a scenario only one driver can run, for good: the binary's,
// for one that needs a process of its own, the real disk or the real
// tools, and the in-process driver's. A scenario of a command not moved
// yet carries neither: its file is not in InProcess.
const (
	BinaryOnly    = "@binary-only"
	InProcessOnly = "@in-process-only"
)

// One scenario's state.
type world struct {
	d   Driver // the world the steps reach
	dir string // the scratch folder

	// feature is the name of the scenario's feature file.
	feature string
	// scored is what "it is scored" does, and noTests what "no tests are
	// run" does, each a sentence of two files: set by the steps of the
	// scenario's file, nil in any other.
	scored, noTests func() error
	// problemSubject adds a fixture's exact subject check to the shared
	// "the problem is ... with ..." sentence, without a second step owner.
	problemSubject func() error
	// beforeRemove joins scenario-owned executions before any fixture deletion.
	beforeRemove func(context.Context) error
	// with are the words a Given adds after a scenario's own to each run
	// its When makes, as a row of an outline says "with --flag".
	with []string
	// variables are fixture-owned environment settings for every command.
	variables []string
	// prepare adds fixture run options without changing the sentence being checked.
	prepare func([]string) []string
	// edges are the import edges the fixture declared (imports), by slash
	// path from the scratch folder.
	edges map[string][]string
	// project is the folder of the project a Given made, slash-separated
	// from the scratch folder, which a sentence names as <dir>; "" for
	// none.
	project string

	exit           int
	stdout, stderr string
	// each are the runs of a When that runs several commands, each of which
	// the exit code and stderr steps hold to; nil after a run of one.
	each []outcome

	// without are the programs a Given leaves off every run's PATH.
	without []string
	// binary is the binary a Given built that every run runs, "" for the
	// one under test.
	binary string
	// wrote is the source "the <language> function:" wrote, slash-separated
	// from the scratch folder, which coverage.feature's steps measure too.
	wrote string

	// last is the last run of itos-cc, which the problem steps run again
	// with --json when it was run without, nil before one.
	last *run
	// problem is the problem of the last run the problem step found, its
	// fix what the fix step reads, nil before one.
	problem map[string]any
}

// run is one run of itos-cc: where, with the variables the scenario set
// and with which words.
type run struct {
	dir  string
	env  []string
	args []string
}

// outcome is what one of the runs of a When did, and the run it was.
type outcome struct {
	run            run
	exit           int
	stdout, stderr string
}

// outcomes are the runs the last When made: each of a When that ran
// several, else the one.
func (w *world) outcomes() []outcome {
	if w.each != nil {
		return w.each
	}
	o := outcome{exit: w.exit, stdout: w.stdout, stderr: w.stderr}
	if w.last != nil {
		o.run = *w.last
	}
	return []outcome{o}
}

// runEach runs itos-cc in the scratch folder once for each command line,
// split as a scenario wrote it, each a run the exit code and stderr steps
// hold to; the last is the run the other steps read.
func (w *world) runEach(lines ...string) error {
	var each []outcome
	for _, line := range lines {
		if err := w.runWith(w.env(), line); err != nil {
			return err
		}
		each = append(each, outcome{run: *w.last, exit: w.exit, stdout: w.stdout, stderr: w.stderr})
	}
	w.each = each
	return nil
}

// Initialize gives sc every step of the converted feature files, each
// reaching the world through d.
func Initialize(sc *godog.ScenarioContext, d Driver) {
	initialize(sc, d)
}

// initialize is Initialize, and gives each of own sc and the scenario's
// world, for the steps only one driver can take, before the scratch folder
// is removed after the scenario: the binary driver's interrupt
// (interrupt_test.go).
func initialize(sc *godog.ScenarioContext, d Driver, own ...func(*godog.ScenarioContext, *world)) {
	w := &world{d: d}
	sc.Before(func(ctx context.Context, s *godog.Scenario) (context.Context, error) {
		return ctx, w.setUp(s)
	})
	for _, steps := range own {
		steps(sc, w)
	}
	// The scenario's error is godog's already: returned here, it would be
	// reported twice, as the hook's and as the step's.
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		return ctx, w.cleanUp(ctx)
	})

	// Split before anything else is expanded, so a word with a space in it
	// stays one argument. Quotes also preserve empty words for completion
	// requests.
	sc.Step(`^itos-cc runs with "([^"]*)"$`, func(args string) error {
		return w.runWith(w.env(), args)
	})
	sc.Step(`^itos-cc runs with "([^"]*)" and the environment "([^"]*)"$`, func(args, vars string) error {
		return w.runWith(w.withEnv(vars), args)
	})

	sc.Step(`^it exits with code (\d+)$`, w.exitsWith)
	sc.Step(`^the exit code is (\d+)$`, w.exitsWith)
	// The problem a run reports, by its rule, alone or with the flag, the
	// argument, the key or the file it names, and the flag its fix suggests.
	sc.Step(`^the problem is "([^"]*)", with (flag|argument|key|file) "([^"]*)"$`, w.problemIs)
	sc.Step(`^the problem is "([^"]*)"$`, func(rule string) error { return w.problemIs(rule, "", "") })
	sc.Step(`^its fix suggests "([^"]*)"$`, w.fixSuggests)
	sc.Step(`^its standard output lists "([^"]*)"$`, func(text string) error {
		if !slices.Contains(outputLines(w.stdout), text) {
			return fmt.Errorf("standard output does not list %q\n%s", text, w.report())
		}
		return nil
	})
	sc.Step(`^its standard output does not list "([^"]*)"$`, func(text string) error {
		if slices.Contains(outputLines(w.stdout), text) {
			return fmt.Errorf("standard output lists %q\n%s", text, w.report())
		}
		return nil
	})
	sc.Step(`^the last line of its standard output is "([^"]*)"$`, func(text string) error {
		lines := outputLines(w.stdout)
		last := strings.TrimSuffix(lines[len(lines)-1], "\r")
		if last != text {
			return fmt.Errorf("the last line of standard output is %q, not %q\n%s", last, text, w.report())
		}
		return nil
	})
	sc.Step(`^its standard output says "([^"]*)"$`, func(text string) error {
		if !strings.Contains(w.stdout, text) {
			return fmt.Errorf("standard output does not say %q\n%s", text, w.report())
		}
		return nil
	})
	sc.Step(`^its standard output does not say "([^"]*)"$`, func(text string) error {
		if strings.Contains(w.stdout, text) {
			return fmt.Errorf("standard output says %q\n%s", text, w.report())
		}
		return nil
	})
	sc.Step(`^its standard output is empty$`, func() error {
		if w.stdout != "" {
			return fmt.Errorf("standard output is not empty\n%s", w.report())
		}
		return nil
	})
	sc.Step(`^its error output says "([^"]*)"$`, func(text string) error {
		if !strings.Contains(w.stderr, text) {
			return fmt.Errorf("error output does not say %q\n%s", text, w.report())
		}
		return nil
	})
	sc.Step(`^its error output does not say "([^"]*)"$`, func(text string) error {
		if strings.Contains(w.stderr, text) {
			return fmt.Errorf("error output says %q\n%s", text, w.report())
		}
		return nil
	})
	sc.Step(`^the first line of its standard output is "([^"]*)" and the stamped version$`, func(name string) error {
		return w.firstLineIs(name + " " + stampedVersion)
	})
	sc.Step(`^the second line of its standard output is "([^"]*)" and the stamped commit$`, func(word string) error {
		return w.secondLineIs(word + " " + stampedCommit)
	})

	// The --json object, as a script reads it.
	sc.Step(`^its JSON output gives the stamped version and commit$`, w.jsonGivesBuild)
	sc.Step(`^its standard output is one line$`, w.stdoutIsOneLine)

	// The steps of the feature files written in the short form, a run of
	// "itos-cc ..." and what stdout and stderr show, which units.feature
	// and dry.feature share.
	sc.Step(`^I run "itos-cc ?([^"]*)"$`, func(args string) error {
		return w.runWith(w.env(), args)
	})
	sc.Step(`^I run "itos-cc ?([^"]*)" without options$`, func(args string) error {
		return w.runWith(w.env(), args)
	})
	sc.Step(`^stdout is one object with "schema": 1, "ok", (.+)$`, w.isOneObjectWith)
	sc.Step(`^stdout shows:$`, w.stdoutShows)
	// Between its first and last quote, so a text may hold quotes of its
	// own, written as they are or as \".
	sc.Step(`^stderr says "(.*)"$`, w.stderrSays)
	sc.Step(`^"([^"]*)" is an empty list$`, w.isEmptyList)
	sc.Step(`^"([^"]*)" and "([^"]*)" are empty lists, not null$`, func(first, second string) error {
		if err := w.isEmptyList(first); err != nil {
			return err
		}
		return w.isEmptyList(second)
	})

	// The fixtures a scenario asks for: itos-cc.yaml, the project's settings at the root of the scratch
	// folder, leaving out of what is measured the sources a glob names.
	sc.Step(`^itos-cc\.yaml excludes "([^"]*)"$`, func(glob string) error {
		return w.write("itos-cc.yaml", "exclude: ["+strconv.Quote(glob)+"]\n")
	})

	// scrap.feature scores an example and crap.feature a function, each
	// file's steps saying what scoring is in its scenarios, as they say what
	// running no tests is.
	sc.Step(`^it is scored$`, func() error {
		if w.scored == nil {
			return fmt.Errorf("nothing is scored in a scenario of %s", w.feature)
		}
		return w.scored()
	})
	// coverage.feature runs no test when reports are there already, and
	// mutate.feature when mutation list lists sites.
	sc.Step(`^no tests are run$`, func() error {
		if w.noTests == nil {
			return fmt.Errorf("no tests are run is no sentence of %s", w.feature)
		}
		return w.noTests()
	})

	// The steps of each converted feature file, after the world's, so a
	// file's own Before runs in the scratch folder made above.
	unitSteps(sc, w)
	drySteps(sc, w)
	scrapSteps(sc, w)
	crapSteps(sc, w)
	complexitySteps(sc, w)
	coverageSteps(sc, w)
	selectionSteps(sc, w)
	cliSteps(sc, w)
	mutateSteps(sc, w)
	snapshotSteps(sc, w)
}

// setUp makes scenario s's scratch folder.
func (w *world) setUp(s *godog.Scenario) error {
	w.problemSubject = nil
	w.beforeRemove = nil
	w.last, w.problem, w.without, w.wrote, w.each, w.binary, w.edges = nil, nil, nil, "", nil, "", nil
	w.feature, w.scored, w.noTests, w.with, w.project = filepath.Base(s.Uri), nil, nil, nil, ""
	var err error
	w.dir, err = w.d.Folder()
	return err
}

// A failed join retains the complete fixture for safe recovery; later hooks
// cannot accidentally make scratch deletion independent of owned cleanup.
func (w *world) cleanUp(ctx context.Context) error {
	if w.beforeRemove == nil {
		_ = w.d.Remove(w.dir)
		return nil
	}
	if w.beforeRemove != nil {
		if err := w.beforeRemove(ctx); err != nil {
			return err
		}
	}
	return w.d.Remove(w.dir)
}

// env is the variables a scenario's fixture sets in every run's environment.
// The driver gives each run the rest.
func (w *world) env() []string { return slices.Clone(w.variables) }

// withEnv is the scenarios' environment with each NAME=value of vars, a
// semicolon-separated list, set as given.
func (w *world) withEnv(vars string) []string {
	env := w.env()
	for _, one := range strings.Split(vars, ";") {
		if name, value, ok := strings.Cut(one, "="); ok {
			env = setEnv(env, name, value)
		}
	}
	return env
}

// When steps.

// runWith runs itos-cc in the scratch folder with args, split as a
// scenario wrote them, and the words a Given added, in the environment env.
func (w *world) runWith(env []string, args string) error {
	fields, err := commandWords(args)
	if err != nil {
		return err
	}
	return w.runEnv(w.dir, env, append(fields, w.with...)...)
}

// commandWords splits a scenario's command line without involving a shell.
// Quotes group words and preserve an explicitly empty argument, as the
// completion protocol needs for a new token. Backslashes remain literal, so
// Windows paths survive the harness.
func commandWords(line string) ([]string, error) {
	var words []string
	var word strings.Builder
	var quote rune
	started := false
	for _, r := range line {
		if quote != 0 {
			if r == quote {
				quote = 0
				continue
			}
			word.WriteRune(r)
			started = true
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
			started = true
		case ' ', '\t', '\n', '\r':
			if started {
				words = append(words, word.String())
				word.Reset()
				started = false
			}
		default:
			word.WriteRune(r)
			started = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unclosed quote in scenario command %q", line)
	}
	if started {
		words = append(words, word.String())
	}
	return words, nil
}

// setEnv is env with the variable name, in any case, set to value.
func setEnv(env []string, name, value string) []string {
	var out []string
	for _, kv := range env {
		if n, _, _ := strings.Cut(kv, "="); !strings.EqualFold(n, name) {
			out = append(out, kv)
		}
	}
	return append(out, name+"="+value)
}

// runEnv runs itos-cc with args in the folder dir with the variables env
// sets, its exit code and output what the Then steps read.
func (w *world) runEnv(dir string, env []string, args ...string) error {
	w.last, w.each = &run{dir: dir, env: env, args: args}, nil
	var err error
	w.exit, w.stdout, w.stderr, err = w.capture(dir, env, args...)
	return err
}

// capture runs itos-cc with args in the folder dir with the variables env
// sets, reading the scenario's standard input, and gives its exit code and
// output.
func (w *world) capture(dir string, env []string, args ...string) (code int, stdout, stderr string, err error) {
	if w.prepare != nil {
		args = w.prepare(slices.Clone(args))
	}
	if err := w.declareAdjacency(context.Background()); err != nil {
		return 0, "", "", err
	}
	result, err := w.d.Run(Run{Dir: dir, Env: env, Args: args, Without: w.without, Binary: w.binary})
	return result.Exit, result.Stdout, result.Stderr, err
}

// Fixture steps.

// write writes text to name, a slash-separated path in the scratch folder,
// making the folders above it.
func (w *world) write(name, text string) error {
	return w.writeIn(w.dir, name, text)
}

// writeIn writes text to name, a slash-separated path in dir, making the
// folders above it.
func (w *world) writeIn(dir, name, text string) error {
	return w.d.Write(filepath.Join(dir, filepath.FromSlash(name)), []byte(text))
}

// readIn is the text of name, a slash-separated path in dir.
func (w *world) readIn(dir, name string) ([]byte, error) {
	return w.d.Read(filepath.Join(dir, filepath.FromSlash(name)))
}

// git runs git with args in the scratch folder, for a fixture that needs a
// repository.
func (w *world) git(args ...string) error {
	return w.d.Git(w.dir, args...)
}
