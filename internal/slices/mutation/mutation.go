// Package mutation is the mutation command group, a slice named for it,
// the first that is a group. Kong owns parsing; run, list, check, sample and
// except own their application/UI here. Their handler reads settings
// through mutants' SettingsReader at each original admission point and
// returns refusals as the sealed Failure set, which Run's classifier words
// with the exit codes cmd gave them.
//
// Command declares the group to cmd, which assembles it into its one model
// (ADR-0051), and Ports are the ports its handler is built over, which
// cmd's adapters satisfy.
package mutation

import "github.com/donvargax/itos-cc/internal/cli"

// Command is the mutation group as cmd assembles it: its name, the line the
// top-level help lists it by, and a fresh command line for each parser,
// whose five children kong reads from CLI's fields.
var Command = cli.Declaration{
	Name: "mutation",
	Help: "mutation testing: do the tests notice small changes?",
	New:  func() any { return &CLI{} },
}

// CLI is one invocation's mutation group and its five child commands.
type CLI struct {
	Run    Run    `cmd:"" help:"mutate each function and run its tests: are the mutants killed?"`
	List   List   `cmd:"" help:"list the mutation sites without running tests"`
	Check  Check  `cmd:"" help:"check the cached mutation results without running tests"`
	Sample Sample `cmd:"" help:"run a sample of the cached mutants again: do their outcomes hold?"`
	Except Except `cmd:"" help:"except an equivalent mutant, a survivor no test can kill, with a reason"`
}

// Contract is the group's existing description and examples.
func (CLI) Contract() cli.Contract {
	return cli.Contract{About: `
Mutation testing: would the tests notice if this code were wrong? mutation run
changes one operator, boolean, or 0/1 at a time inside each function and runs
the tests; mutation list lists those changes, the mutation sites, without
running any test; mutation check says whether the results mutation run
cached are there, fresh, and passing, without running any test either;
mutation sample runs a few of the cached mutants again and fails when an
outcome differs from the one recorded; and mutation except excepts an
equivalent mutant, a survivor no test can kill, in itos-cc.yaml, with the
reason no test can. The sources itos-cc.yaml's exclude names, globs from
the project root such as **/porttest/**, are none of theirs, however they
are chosen; a test never is.`, Examples: []string{
		"itos-cc mutation run --changed",
		"itos-cc mutation list src/billing/invoice.ts",
		"itos-cc mutation check --since origin/main --fail-uncovered",
		"itos-cc mutation sample --count 20",
		"itos-cc mutation except src/board.ts:3:13 --reason 'c is set again before it is read'",
	}}
}

// Run declares mutation run's flags in their original help order. Workers
// comes from the process boundary, supplied as a Kong variable.
type Run struct {
	cli.Selection
	NoCoverage          bool        `name:"no-coverage" help:"skip coverage; complexity only"`
	UseExistingCoverage bool        `help:"read reports already on disk instead of running tests"`
	AllTests            bool        `help:"run the whole test suite, integration and end-to-end tests included"`
	CoverageCommand     string      `placeholder:"CMD" help:"run this shell command instead of the per-language commands (with --coverage-report)"`
	CoverageReport      []string    `placeholder:"FILE" sep:"none" help:"read this LCOV, Go cover profile, or JaCoCo XML report"`
	Workers             int         `placeholder:"N" default:"${mutation_workers}" help:"mutants run at the same time"`
	MutateAll           bool        `help:"rerun killed mutants of unchanged functions too"`
	TimeoutFactor       float64     `placeholder:"N" default:"10" help:"a mutant times out after N times the baseline duration, plus 5s"`
	OutputFactor        float64     `placeholder:"N" default:"10" help:"a mutant is killed once its tests write more than N times the baseline's output, plus 16 MiB"`
	MemoryFactor        float64     `placeholder:"N" default:"4" help:"a mutant is killed once its tests hold more than N times the baseline's peak memory, plus 256 MiB; Linux"`
	TestCommand         string      `placeholder:"CMD" help:"shell command that runs the tests, instead of the per-language default"`
	NoAnnotate          bool        `name:"no-annotate" help:"do not write the summary comment into source files"`
	Since               string      `placeholder:"REF" help:"judge only the functions the commits since REF changed (git diff REF...HEAD)"`
	FailUncovered       *cli.Choice `enum:"functions,lines" help:"fail on uncovered mutants, executable Go coverage blocks and executable Python, TypeScript, Kotlin and Dart lines; =lines, with --since, only on the lines the range changed"`
	FailFast            bool        `help:"stop at the first actionable failure, cancelling the work still running; Linux and macOS"`
	Count               int         `placeholder:"N" help:"judge at most N committed mutation sites freshly, drawn across the whole selection; Linux and macOS"`
	Seed                string      `placeholder:"TEXT" help:"with --count, seed the draw with TEXT instead of the HEAD commit's id"`
	Repeat              int         `placeholder:"N" default:"1" help:"with --count, run each trial N times; outcomes not all alike are mutation.flaky"`
	JSON                bool        `name:"json" help:"print one JSON object on stdout instead of text (shape below)"`
}

// List lists sites without running tests.
type List struct {
	cli.Selection
	JSON bool `name:"json" help:"print one JSON object on stdout instead of text (shape below)"`
}

// Check reads cached judgments, optionally requiring coverage evidence.
type Check struct {
	cli.Selection
	Since         *string     `placeholder:"REF" help:"check only the functions the commits since REF changed (git diff REF...HEAD)"`
	FailUncovered *cli.Choice `enum:"functions,lines" help:"fail on uncovered mutants, executable Go coverage blocks and executable Python, TypeScript, Kotlin and Dart lines; =lines, with --since, only on the lines the range changed"`
	JSON          bool        `name:"json" help:"print one JSON object on stdout instead of text (shape below)"`
}

// Sample declares the recorded-result sampling command. Since and Seed are
// nil when omitted, so an explicit empty one stays distinct.
type Sample struct {
	cli.Selection
	Since         *string `placeholder:"REF" help:"sample only the functions the commits since REF changed (git diff REF...HEAD)"`
	Count         int     `placeholder:"N" default:"20" help:"how many mutants to run, at least 1"`
	Seed          *string `placeholder:"TEXT" help:"seed the draw with TEXT instead of the HEAD commit's id"`
	Repeat        int     `placeholder:"N" default:"1" help:"run each sampled mutant N times; outcomes not all alike are mutation.flaky"`
	Workers       int     `placeholder:"N" default:"${mutation_workers}" help:"mutants run at the same time"`
	TimeoutFactor float64 `placeholder:"N" default:"10" help:"a mutant times out after N times the baseline duration, plus 5s"`
	OutputFactor  float64 `placeholder:"N" default:"10" help:"a mutant is killed once its tests write more than N times the baseline's output, plus 16 MiB"`
	MemoryFactor  float64 `placeholder:"N" default:"4" help:"a mutant is killed once its tests hold more than N times the baseline's peak memory, plus 256 MiB; Linux"`
	TestCommand   string  `placeholder:"CMD" help:"shell command that runs the tests for every mutant, instead of its recorded scope"`
	AllTests      bool    `help:"run the whole test suite for every mutant, instead of its recorded scope"`
	JSON          bool    `name:"json" help:"print one JSON object on stdout instead of text (shape below)"`
}

// Except accepts a site or renews the current exception entries. Reason is
// nil when omitted, so an explicit empty one stays distinct.
type Except struct {
	Reason *string  `placeholder:"TEXT" help:"why no test can kill the mutant; required, but for a site that re-points a stale entry, which keeps its reason, and with --renew or --prune"`
	Renew  bool     `help:"renew each stale entry whose mutant is unchanged, after mutation run, keeping its reason"`
	Prune  bool     `help:"remove each entry stale as gone, killed or excluded, after mutation run"`
	JSON   bool     `name:"json" help:"print one JSON object on stdout instead of text (shape below)"`
	Site   []string `arg:"" optional:"" name:"site" help:"the recorded survivor’s file, line and column, or its identity as mutation run --count reports it"`
}

// Synopsis retains except's two alternative invocation forms.
func (Except) Synopsis() string {
	return "<file>:<line>:<column>|<identity> --reason TEXT [options] | --renew [options] | --prune [options]"
}
