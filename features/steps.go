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
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	gherkin "github.com/cucumber/gherkin/go/v42"
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
	// records it, for the fake that runs a project's tests once the
	// mutation group moves. A folder with no Tests declared has no tests a
	// run may run.
	// Declarations upsert by directory and Command; other commands remain.
	Tests(dir string, t Tests) error
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
}

// TestTrace identifies an executable fixture's execution-observation file and source.
type TestTrace struct {
	File, Source string
	// Tests maps the complete runnable selector inventory to observed names.
	// Bound commands and measurement requests select executions from it; nil
	// keeps the source-only execution trace contract used by earlier fixtures.
	Tests map[string]string
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
	// CoverageExit/Output describe the actual listed collection command result.
	// CoverageMetadata, when nonnil, is written as the owned per-test covmeta
	// artifact; conversion, rather than this declaration, determines failure.
	CoverageExit     int
	CoverageOutput   string
	CoverageMetadata []byte
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
	// IntegrationReport is the actual subprocess collector report, separately
	// from Report, in the same existing formats. Empty means no subprocess data.
	IntegrationReport string
	// WithoutC8 declares TypeScript's actual missing built-in collector. Binary
	// fixtures own a facade of pinned tools excluding c8; no integration report
	// can accompany that absence. Raw reports/no-coverage remain unaffected.
	WithoutC8 bool
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
// have steps: count.feature, the template's sample, every scenario of it
// @wip (T-26), units.feature, the first command moved onto kong (T-32),
// dry.feature and scrap.feature, crap.feature, complexity.feature and
// coverage.feature, and file_selection.feature, each converted before its
// command moves so the move is held by its scenarios (T-37, T-41, T-60,
// T-61, T-62), cli.feature, whose help, version and mutation scenarios
// need commands that move later (T-62), and mutate.feature, converted in
// pieces before the mutation group moves (T-70 to T-81), the scenarios of
// the pieces not done yet listed in Unstepped. A file converts with its
// scenarios' text unchanged (q-52); the binary driver runs every one.
var Converted = []string{
	"count.feature", "units.feature", "dry.feature", "scrap.feature",
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
// complexity's and file selection's files in T-67, cli.feature in S9); until
// then the binary driver alone runs its scenarios.
var InProcess = []string{"count.feature", "units.feature", "dry.feature", "scrap.feature", "crap.feature", "complexity.feature", "coverage.feature", "file_selection.feature"}

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
	// project is the folder of the project a Given made, slash-separated
	// from the scratch folder, which a sentence names as <dir>; "" for
	// none.
	project string

	exit           int
	stdout, stderr string
	// each are the runs of a When that runs several commands, each of which
	// the exit code and stderr steps hold to; nil after a run of one.
	each []outcome

	stdin string // the text the run reads from its standard input, "" for none
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
	sc.Step(`^its JSON output names the problem "([^"]*)"$`, w.jsonNamesProblem)
	sc.Step(`^its JSON output gives file "([^"]*)", (\d+) lines and (\d+) words$`, w.jsonGivesCounts)
	sc.Step(`^its JSON output gives each problem a rule, a message and a fix$`, w.jsonProblemsComplete)
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

	// The fixtures a scenario asks for. godog takes one parameter per
	// capturing group and none variadic, so the second text is its own
	// group, empty where the scenario names one.
	sc.Step(`^the file "([^"]*)" contains "([^"]*)"(?:, "([^"]*)")?$`, func(name, first, second string) error {
		texts := []string{first}
		if second != "" {
			texts = append(texts, second)
		}
		return w.fileContains(name, texts...)
	})
	sc.Step(`^the file "([^"]*)" holds a line of (\d+) characters$`, w.fileHoldsLine)
	sc.Step(`^standard input holds "([^"]*)"$`, func(text string) error {
		w.stdin = text
		return nil
	})
	sc.Step(`^an empty folder "([^"]*)"$`, w.emptyFolder)
	// itos-cc.yaml, the project's settings at the root of the scratch
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
	w.last, w.problem, w.without, w.wrote, w.each, w.binary = nil, nil, nil, "", nil, ""
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
	result, err := w.d.Run(Run{Dir: dir, Env: env, Stdin: w.stdin, Args: args, Without: w.without, Binary: w.binary})
	return result.Exit, result.Stdout, result.Stderr, err
}

// Fixture steps.

// fileContains writes name in the scratch folder, one line per text the
// scenario named, each ending in the newline a text file's last line has.
func (w *world) fileContains(name string, texts ...string) error {
	if len(texts) == 0 || texts[0] == "" {
		return fmt.Errorf("the file %q is given no text to contain", name)
	}
	body := strings.Join(texts, "\n") + "\n"
	return w.d.Write(filepath.Join(w.dir, name), []byte(body))
}

// fileHoldsLine writes name in the scratch folder holding one line of count
// characters, long enough to beat a token limit a reader might have, and no
// other line.
func (w *world) fileHoldsLine(name string, count int) error {
	body := strings.Repeat("x", count) + "\n"
	return w.d.Write(filepath.Join(w.dir, name), []byte(body))
}

// emptyFolder makes a folder in the scratch directory.
func (w *world) emptyFolder(name string) error {
	return w.d.Mkdir(filepath.Join(w.dir, name))
}

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

// Then steps.

func (w *world) report() string {
	return fmt.Sprintf("exit %d\n--- stdout\n%s--- stderr\n%s", w.exit, w.stdout, w.stderr)
}

// stderrSays is whether stderr holds text, \" in it a quote, in each run
// the When made. In a text naming <dir>, that stands for the folder of the
// project the Given made, as itos-cc names it from the scratch folder, and
// "..." or "…" for the rest of what it says there.
func (w *world) stderrSays(text string) error {
	text = strings.ReplaceAll(strings.ReplaceAll(text, `\"`, `"`), "…", "...")
	if strings.Contains(text, "<message>") || strings.Contains(text, "<fix>") {
		return w.saysEachProblem(text)
	}
	for _, o := range w.outcomes() {
		if err := w.saysIn(o.stderr, text); err != nil {
			return fmt.Errorf("%v\n--- the run of itos-cc %s, exit %d\n--- stdout\n%s--- stderr\n%s", err, strings.Join(o.run.args, " "), o.exit, o.stdout, o.stderr)
		}
	}
	return nil
}

// saysEachProblem is whether the last run reported a problem, and its
// stderr says text of each, <message> and <fix> in it standing for the
// problem's, as the run made with --json gives them (lastObject).
func (w *world) saysEachProblem(text string) error {
	object, report, err := w.lastObject()
	if err != nil {
		return err
	}
	problems, _ := object["problems"].([]any)
	if len(problems) == 0 {
		return fmt.Errorf("the run reported no problem\n%s", report)
	}
	for _, one := range problems {
		problem, _ := one.(map[string]any)
		message, _ := problem["message"].(string)
		fix, _ := problem["fix"].(string)
		want := strings.ReplaceAll(strings.ReplaceAll(text, "<message>", message), "<fix>", fix)
		if message == "" || fix == "" || !strings.Contains(w.stderr, want) {
			return fmt.Errorf("stderr does not say %q\n%s", want, report)
		}
	}
	return nil
}

// saysIn is whether stderr, a run's, holds text, read as stderrSays reads
// it.
func (w *world) saysIn(stderr, text string) error {
	if !strings.Contains(text, "<dir>") && !strings.Contains(text, "...") {
		// A path is named in the system's form, which a scenario writes
		// with slashes.
		if !strings.Contains(stderr, text) && !strings.Contains(filepath.ToSlash(stderr), text) {
			return fmt.Errorf("stderr does not say %q", text)
		}
		return nil
	}
	if strings.Contains(text, "<dir>") && w.project == "" {
		return fmt.Errorf("%q names <dir>, and no Given made a project's folder", text)
	}
	var pattern strings.Builder
	for i, part := range strings.Split(text, "...") {
		if i > 0 {
			pattern.WriteString(".+")
		}
		pieces := strings.Split(part, "<dir>")
		for j, piece := range pieces {
			if j > 0 {
				pattern.WriteString(regexp.QuoteMeta(w.project))
			}
			pattern.WriteString(regexp.QuoteMeta(piece))
		}
	}
	if !regexp.MustCompile(pattern.String()).MatchString(filepath.ToSlash(stderr)) {
		return fmt.Errorf("stderr does not say %q, <dir> being %s", text, w.project)
	}
	return nil
}

func outputLines(output string) []string {
	return strings.Split(strings.TrimSuffix(output, "\n"), "\n")
}

// exitsWith is whether each run the When made exited with code.
func (w *world) exitsWith(code int) error {
	for _, o := range w.outcomes() {
		if o.exit != code {
			return fmt.Errorf("itos-cc %s exited %d, not %d\n--- stdout\n%s--- stderr\n%s", strings.Join(o.run.args, " "), o.exit, code, o.stdout, o.stderr)
		}
	}
	return nil
}

// secondLineIs is whether standard output's second line is text, a missing
// line read as an empty one and a line ending in \r\n as one in \n.
func (w *world) secondLineIs(text string) error {
	_, rest, _ := strings.Cut(w.stdout, "\n")
	second, _, _ := strings.Cut(rest, "\n")
	if second = strings.TrimSuffix(second, "\r"); second != text {
		return fmt.Errorf("the second line of standard output is %q, not %q\n%s", second, text, w.report())
	}
	return nil
}

// firstLineIs is whether standard output's first line is text, a line ending
// in \r\n read as one in \n.
func (w *world) firstLineIs(text string) error {
	first, _, _ := strings.Cut(w.stdout, "\n")
	if first = strings.TrimSuffix(first, "\r"); first != text {
		return fmt.Errorf("the first line of standard output is %q, not %q\n%s", first, text, w.report())
	}
	return nil
}

// The JSON steps read standard output as the one object --json prints.

// json is the object standard output holds, one line of JSON as --json
// prints it.
func (w *world) json() (map[string]any, error) {
	var object map[string]any
	if err := json.Unmarshal([]byte(w.stdout), &object); err != nil {
		return nil, fmt.Errorf("standard output is not one JSON object: %v\n%s", err, w.report())
	}
	return object, nil
}

// jsonNamesProblem is whether the object's problems list one with that rule.
func (w *world) jsonNamesProblem(rule string) error {
	object, err := w.json()
	if err != nil {
		return err
	}
	problems, _ := object["problems"].([]any)
	for _, one := range problems {
		problem, _ := one.(map[string]any)
		if problem["rule"] == rule {
			return nil
		}
	}
	return fmt.Errorf("no problem of the JSON output has the rule %q\n%s", rule, w.report())
}

// lastObject is the object of the last run, made again with --json before
// its words, as a script would make it, unless it had --json already: that
// run must exit as the plain one did. The report is both runs'.
func (w *world) lastObject() (map[string]any, string, error) {
	if w.last == nil {
		return nil, "", errors.New("no run of itos-cc to read")
	}
	stdout, report := w.stdout, w.report()
	if !slices.Contains(w.last.args, "--json") {
		code, out, errOut, err := w.capture(w.last.dir, w.last.env, append([]string{"--json"}, w.last.args...)...)
		if err != nil {
			return nil, "", err
		}
		report = fmt.Sprintf("%s\n--- with --json, exit %d\n--- stdout\n%s--- stderr\n%s", report, code, out, errOut)
		if code != w.exit {
			return nil, "", fmt.Errorf("with --json itos-cc exited %d, not %d as without it\n%s", code, w.exit, report)
		}
		stdout = out
	}
	var object map[string]any
	if err := json.Unmarshal([]byte(stdout), &object); err != nil {
		return nil, "", fmt.Errorf("the output with --json is not one JSON object: %v\n%s", err, report)
	}
	return object, report, nil
}

// problemIs is whether the last run reports a problem of that rule, whose
// key, a flag, an argument, a key of itos-cc.yaml or a file, its
// separators read as slashes, is value; any problem
// of the rule where key is "". The plain output names no rule, so the
// problem is read from the run made with --json (lastObject), and the plain
// one's error output must print its message.
func (w *world) problemIs(rule, key, value string) error {
	object, report, err := w.lastObject()
	if err != nil {
		return err
	}
	problems, _ := object["problems"].([]any)
	for _, one := range problems {
		problem, _ := one.(map[string]any)
		got := problem[key]
		if file, ok := got.(string); ok && key == "file" {
			// A file is named in the system's form, which a scenario
			// writes with slashes.
			got = filepath.ToSlash(file)
		}
		if problem["rule"] != rule || key != "" && got != value {
			continue
		}
		if message, _ := problem["message"].(string); message == "" || !strings.Contains(w.stderr, message) {
			return fmt.Errorf("the error output does not print the problem's message %q\n%s", message, report)
		}
		w.problem = problem
		return nil
	}
	if key == "" {
		return fmt.Errorf("no problem has the rule %q\n%s", rule, report)
	}
	return fmt.Errorf("no problem has the rule %q and the %s %q\n%s", rule, key, value, report)
}

// fixSuggests is whether the fix of the problem problemIs found asks
// whether the person meant flag, in the words the command line uses, and
// the error output prints that fix.
func (w *world) fixSuggests(flag string) error {
	if w.problem == nil {
		return errors.New("no problem found to read the fix of")
	}
	fix, _ := w.problem["fix"].(string)
	if want := "Did you mean " + flag + "?"; !strings.Contains(fix, want) {
		return fmt.Errorf("the problem's fix %q does not say %q\n%s", fix, want, w.report())
	}
	if !strings.Contains(w.stderr, fix) {
		return fmt.Errorf("the error output does not print the problem's fix %q\n%s", fix, w.report())
	}
	return nil
}

// jsonGivesCounts is whether the success object's file is file, its lines
// the count and its words the count.
func (w *world) jsonGivesCounts(file string, lines, words int) error {
	object, err := w.json()
	if err != nil {
		return err
	}
	if object["ok"] != true {
		return fmt.Errorf("the JSON output's ok is %v, not true\n%s", object["ok"], w.report())
	}
	for _, want := range []struct {
		key  string
		got  any
		want any
	}{
		{"file", object["file"], file},
		{"lines", numberOf(object["lines"]), lines},
		{"words", numberOf(object["words"]), words},
	} {
		if want.got != want.want {
			return fmt.Errorf("the JSON output's %s is %v, not %v\n%s", want.key, want.got, want.want, w.report())
		}
	}
	return nil
}

// jsonGivesBuild is whether the success object of version --json is schema
// 1 and ok, and gives the version and the commit the harness stamped.
func (w *world) jsonGivesBuild() error {
	object, err := w.json()
	if err != nil {
		return err
	}
	for _, want := range []struct {
		key  string
		want any
	}{
		{"schema", 1.0},
		{"ok", true},
		{"version", stampedVersion},
		{"commit", stampedCommit},
	} {
		if got := object[want.key]; got != want.want {
			return fmt.Errorf("the JSON output's %s is %v, not %v\n%s", want.key, got, want.want, w.report())
		}
	}
	return nil
}

// stdoutIsOneLine is whether standard output is exactly one line, ended by
// its newline, as --json prints its object.
func (w *world) stdoutIsOneLine() error {
	if strings.Count(w.stdout, "\n") != 1 || !strings.HasSuffix(w.stdout, "\n") {
		return fmt.Errorf("standard output is not one line\n%s", w.report())
	}
	return nil
}

// objectKeys is a list of quoted keys as a sentence writes it: "a", "b",
// and "c", or and "a" alone.
var objectKeys = regexp.MustCompile(`^(?:and )?"[^"]+"(?:(?:, and |, | and )"[^"]+")*$`)

// isOneObjectWith is whether standard output is one JSON object, schema 1,
// with ok and every key of the list ("a", "b", and "c"), none of them null.
func (w *world) isOneObjectWith(list string) error {
	fileShape := strings.HasSuffix(list, ", each with file, killed, survived, uncovered, ran, reused, and baseline")
	if fileShape {
		list = strings.TrimSuffix(list, ", each with file, killed, survived, uncovered, ran, reused, and baseline")
	}

	if !objectKeys.MatchString(list) {
		return fmt.Errorf("%s is no list of quoted keys", list)
	}
	dec := json.NewDecoder(strings.NewReader(w.stdout))
	var object map[string]any
	if err := dec.Decode(&object); err != nil {
		return fmt.Errorf("standard output is not a JSON object: %v\n%s", err, w.report())
	}
	if dec.More() {
		return fmt.Errorf("standard output holds more than one JSON value\n%s", w.report())
	}
	if _, ok := object["ok"].(bool); !ok || object["schema"] != 1.0 {
		return fmt.Errorf("the object lacks \"schema\": 1 or \"ok\"\n%s", w.report())
	}
	for _, m := range regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(list, -1) {
		if object[m[1]] == nil {
			return fmt.Errorf("the object lacks %q\n%s", m[1], w.report())
		}
	}
	if fileShape {
		files, ok := object["files"].([]any)
		if !ok || len(files) == 0 {
			return fmt.Errorf("no result files: %v", object)
		}
		for _, raw := range files {
			file, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("file is not an object: %v", raw)
			}
			for _, key := range []string{"file", "killed", "survived", "uncovered", "ran", "reused", "baseline"} {
				if file[key] == nil {
					return fmt.Errorf("file lacks %s: %v", key, file)
				}
			}
		}
	}

	return nil
}

// stdoutShows is whether standard output is the text, line for line, with
// its last line's newline. A path printed with the system's separator reads
// as one with slashes, as the scenarios write paths.
func (w *world) stdoutShows(text *godog.DocString) error {
	got := strings.ReplaceAll(filepath.ToSlash(w.stdout), "\r\n", "\n")
	if want := text.Content + "\n"; got != want {
		return fmt.Errorf("stdout is\n%s\nnot\n%s\n%s", got, want, w.report())
	}
	return nil
}

// isEmptyList is whether the object standard output holds has key, a list
// with nothing in it, not null.
func (w *world) isEmptyList(key string) error {
	object, err := w.json()
	if err != nil {
		return err
	}
	if list, ok := object[key].([]any); !ok || len(list) != 0 {
		return fmt.Errorf("%q is %v, not an empty list\n%s", key, object[key], w.report())
	}
	return nil
}

// jsonProblemsComplete is whether the object's problems all carry a rule, a
// message and a fix, and there is one.
func (w *world) jsonProblemsComplete() error {
	object, err := w.json()
	if err != nil {
		return err
	}
	problems, _ := object["problems"].([]any)
	if len(problems) == 0 {
		return fmt.Errorf("the JSON output lists no problem\n%s", w.report())
	}
	for i, one := range problems {
		problem, _ := one.(map[string]any)
		for _, key := range []string{"rule", "message", "fix"} {
			if text, _ := problem[key].(string); text == "" {
				return fmt.Errorf("problem %d of the JSON output has no %s\n%s", i+1, key, w.report())
			}
		}
	}
	return nil
}

// numberOf is the number a decoded JSON value holds, a float64 as encoding/
// json decodes every number into.
func numberOf(value any) int {
	number, _ := value.(float64)
	return int(number)
}

// Selection is what a -scenarios expression selects for one driver.
type Selection struct {
	// Paths are the driver's active feature files, with dir joined to each.
	Paths []string
	// Skipped are the feature filenames ITOS_CC_SKIP_FEATURES leaves out.
	Skipped []string
	// Filter is godog's tag filter over the driver's files, "" when the
	// expression selects none of their scenarios.
	Filter string
	// Unconverted are the tags it matches only in feature files with no
	// steps yet (q-52), held by cmd/itos-cc's Go tests.
	Unconverted []string
	// Unmoved are the tags it matches only in converted files the driver
	// does not run: the binary driver's alone until their command moves.
	Unmoved []string
	// Unstepped are the tags it matches only on scenarios Unstepped
	// lists, whose steps are not written yet, held by cmd/itos-cc's Go
	// tests.
	Unstepped []string
}

// TagFilter is what a selection selects of the feature files in dir for a
// driver that runs files of them and cannot run the scenarios tagged skip:
// every live scenario of files without one, else the live scenarios of
// files carrying a tag the expression matches, with the tags it matches
// only in other files, which the driver does not run. An expression that
// matches no tag of a converted file is an error, so a selection never
// runs nothing; one that matches only tags of converted files the driver
// does not run selects nothing for it, the binary driver running those. A
// scenario only one driver can run carries @binary-only or
// @in-process-only, never a copy, and the other driver passes it by. The
// scenarios Unstepped lists are left out of every run: a tag only they
// carry is logged, as one of a file the driver does not run is, and one
// that others carry too selects those others alone.
// ITOS_CC_SKIP_FEATURES leaves its comma-separated filenames out of Paths;
// their tags still match valid selections, and Skipped names them for the
// runners to log. Carrier validation still reads every converted file.
//
// godog's own tag filter takes exact tags joined by commas; itos's run
// templates join the IDs they select with |, as a regular expression
// (itos.yaml's tests.scenario.run), so a selection is that expression,
// turned here into godog's filter. A scenario tagged @wip never runs.
func TagFilter(selection, dir string, files []string, skip string) (Selection, error) {
	in := func(names []string) []string {
		var paths []string
		for _, name := range names {
			paths = append(paths, filepath.Join(dir, name))
		}
		return paths
	}
	omitted := map[string]bool{}
	for _, name := range strings.Split(os.Getenv("ITOS_CC_SKIP_FEATURES"), ",") {
		if name = strings.TrimSpace(name); name != "" {
			omitted[name] = true
		}
	}
	var sel Selection
	for _, name := range files {
		if !omitted[name] {
			sel.Paths = append(sel.Paths, filepath.Join(dir, name))
		}
	}
	all, err := filepath.Glob(filepath.Join(dir, "*.feature"))
	if err != nil {
		return Selection{}, err
	}
	var leftOut []string
	for _, path := range all {
		if name := filepath.Base(path); omitted[name] {
			sel.Skipped = append(sel.Skipped, name)
			leftOut = append(leftOut, path)
		}
	}
	own, err := featureTags(sel.Paths)
	if err != nil {
		return Selection{}, err
	}
	carried, err := carriers(in(Converted))
	if err != nil {
		return Selection{}, err
	}
	// leaveOut is the filter's terms leaving out the Unstepped scenarios of
	// the driver's files among ids.
	leaveOut := func(ids []string) string {
		var terms strings.Builder
		for _, id := range ids {
			if slices.Contains(own, id) {
				terms.WriteString("&&~" + id)
			}
		}
		return terms.String()
	}
	if selection == "" {
		if len(sel.Paths) > 0 {
			sel.Filter = "~@wip&&~" + skip + leaveOut(Unstepped)
		}
		return sel, nil
	}
	pattern, err := regexp.Compile(selection)
	if err != nil {
		return Selection{}, err
	}
	converted, err := featureTags(in(Converted))
	if err != nil {
		return Selection{}, err
	}
	skipped, err := featureTags(leftOut)
	if err != nil {
		return Selection{}, err
	}
	everyTag, err := featureTags(all)
	if err != nil {
		return Selection{}, err
	}
	var each []string
	matched := false
	for _, tag := range everyTag {
		if !pattern.MatchString(tag) {
			continue
		}
		c := carried[tag]
		switch {
		case c.unstepped != nil && !c.stepped:
			sel.Unstepped = append(sel.Unstepped, tag)
		case slices.Contains(own, tag):
			each = append(each, tag+"&&~@wip&&~"+skip+leaveOut(c.unstepped))
		case slices.Contains(skipped, tag):
			// A real tag whose file was deliberately left out is a valid
			// selection, even when this driver has nothing left to run.
		case slices.Contains(converted, tag):
			sel.Unmoved = append(sel.Unmoved, tag)
		default:
			sel.Unconverted = append(sel.Unconverted, tag)
			continue
		}
		matched = true
	}
	if !matched {
		return sel, &noMatch{selection, sel.Unconverted}
	}
	sel.Filter = strings.Join(each, ",")
	return sel, nil
}

// noMatch is a selection matching no tag of a converted file, with the
// tags it matches in files with no steps yet.
type noMatch struct {
	selection   string
	unconverted []string
}

func (e *noMatch) Error() string {
	if len(e.unconverted) > 0 {
		return "no scenario this harness runs has a tag that " + e.selection + " matches: " +
			strings.Join(e.unconverted, " ") + " are in feature files with no steps yet (q-52), held by cmd/itos-cc's Go tests"
	}
	return "no scenario has a tag that " + e.selection + " matches"
}

// carrier is which scenarios carry a tag: the Unstepped ones, by their ID
// tag, and whether any other does.
type carrier struct {
	unstepped []string
	stepped   bool
}

// carriers is, for every tag the scenarios of files carry, theirs or one
// their feature, rule or examples carry, as godog reads them, which carry
// it. It fails when an ID Unstepped lists is no scenario's of files.
func carriers(files []string) (map[string]carrier, error) {
	carried := map[string]carrier{}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		n := 0
		newID := func() string { n++; return strconv.Itoa(n) }
		doc, err := gherkin.ParseGherkinDocument(bytes.NewReader(data), newID)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		for _, pickle := range gherkin.Pickles(*doc, file, newID) {
			id := ""
			for _, tag := range pickle.Tags {
				if slices.Contains(Unstepped, tag.Name) {
					id = tag.Name
				}
			}
			for _, tag := range pickle.Tags {
				c := carried[tag.Name]
				if id == "" {
					c.stepped = true
				} else if !slices.Contains(c.unstepped, id) {
					c.unstepped = append(c.unstepped, id)
				}
				carried[tag.Name] = c
			}
		}
	}
	for _, id := range Unstepped {
		if carried[id].unstepped == nil {
			return nil, fmt.Errorf("features.Unstepped lists %s, which no scenario of a converted file carries", id)
		}
	}
	return carried, nil
}

// Every tag written in files, once each, sorted.
func featureTags(files []string) ([]string, error) {
	seen := map[string]bool{}
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		lines := bufio.NewScanner(f)
		for lines.Scan() {
			line := strings.TrimSpace(lines.Text())
			if !strings.HasPrefix(line, "@") {
				continue
			}
			for _, word := range strings.Fields(line) {
				if strings.HasPrefix(word, "#") {
					break
				}
				if strings.HasPrefix(word, "@") {
					seen[word] = true
				}
			}
		}
		_ = f.Close()
		if err := lines.Err(); err != nil {
			return nil, err
		}
	}
	tags := make([]string, 0, len(seen))
	for tag := range seen {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags, nil
}

// stdoutSays checks literal text, or one line whose ordered pieces surround
// the scenario's ellipsis, as mutation summaries name only their suffix.
func (w *world) stdoutSays(text string) error {
	if !strings.Contains(text, "…") {
		if strings.Contains(w.stdout, text) {
			return nil
		}
	} else {
		for _, line := range outputLines(w.stdout) {
			rest := filepath.ToSlash(line)
			matched := true
			for _, part := range strings.Split(text, "…") {
				at := strings.Index(rest, part)
				if at < 0 {
					matched = false
					break
				}
				rest = rest[at+len(part):]
			}
			if matched {
				return nil
			}
		}
	}
	return fmt.Errorf("stdout does not say %q\n%s", text, w.report())
}
