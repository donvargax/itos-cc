// The features: every converted *.feature file in this folder, run by godog
// through go test against the itos-cc binary built once a run from this
// tree, through the binary driver (binary.go), each scenario a subtest of
// TestFeatures. Harvested from itos's
// (github.com/donvargax/itos, features/features_test.go) and the owner's
// itos-template's, without either one's generator behaviour.
//
// The binary driver runs only where it is opted into (decision 52, T-153):
// ITOS_CC_FEATURES_BINARY_DRIVER=1, which every workflow sets in its
// top-level env, so CI and CD run it, or ITOS_CC_FEATURES_BINARY naming a
// binary (below). Anywhere else, a local go test ./... or itos.yaml's
// scenario lines among them, TestFeatures logs one line saying it ran
// nothing and why, builds nothing and passes, before it reads -scenarios:
// locally the scenarios run in process against the fakes alone
// (cmd/itos-cc's TestScenarios), and a selection naming only @binary-only
// or Deferred scenarios passes having run nothing. An agent working on a
// @binary-only scenario sets the variable itself. Every command below
// assumes it set.
//
//	go test ./features -count=1                        every live scenario
//	go test ./features -count=1 -scenarios=<regexp>    the live scenarios with a
//	                                                   tag the expression matches
//	GOCOVERDIR=<dir> go test ./features -count=1       either, itos-cc
//	                                                   built with -cover, what
//	                                                   the scenarios ran in <dir>
//	ITOS_CC_FEATURES_BINARY=<file> go test ./features -count=1
//	                                                   either, against the
//	                                                   itos-cc binary at <file>,
//	                                                   built by no one here
//	ITOS_CC_FEATURES_CONCURRENCY=<n> go test ./features -count=1
//	                                                   either, <n> scenarios at
//	                                                   once, not one a CPU
//
// -scenarios takes the regular expression itos's run templates join the IDs
// they select into, which TagFilter turns into godog's filter. A scenario
// tagged @wip never runs.
//
// Only the feature files in Converted run here: those whose scenarios have
// steps (q-52), each declared beside its steps (featureFile) and listed in
// steps_gen.go by tools/bin/assemble, the same files the in-process driver
// runs. The others are converted over time, and until then their scenarios are held
// by the Go tests of cmd/itos-cc; a selection naming one of theirs logs it
// as not run here, and a selection naming nothing else fails, as one
// matching no tag does. So are the scenarios of a converted file whose
// steps are not written yet (Unstepped), but a selection naming only them
// passes, running nothing.
//
// The scenarios run itos-cc as another process, where Go records
// coverage only for a binary built with -cover and run with GOCOVERDIR naming
// an existing folder (go.dev/doc/build-cover); go test's own -cover sees none
// of it. So with GOCOVERDIR set, the binary is built with -cover
// -covermode=atomic -coverpkg=./..., every package of the module counted, and
// what the scenarios run lands in that folder, where itos-cc merges it with go
// test's. The build keys on GOCOVERDIR alone: a -cover binary run without it
// writes "warning: GOCOVERDIR not set, no coverage data emitted" to its
// stderr, which breaks every scenario that reads stderr, and only
// GOCOVERDIR's being set guarantees every itos-cc a scenario starts has it.
// The binary driver's environment passes it on unchanged, every environment
// a step runs a command in is that one with the scenario's variables set.
// itos-cc keeps it to itself: the commands it starts never inherit it
// (slice-59, ID-MUT-352), so the projects' tests it runs write nothing
// there, and its own coverage runs set their own. Measuring the whole
// module at once, go test -cover -coverpkg=./... ./..., gives every test
// binary a GOCOVERDIR of go test's own, whatever the environment says, so
// GOCOVERDIR reaches TestFeatures only through an -exec wrapper that sets
// it for features.test alone (features/README.md, Running them).
// Never ITOS_CC_TEST_COVERDIR, which itos-cc sets alone for a listed test's
// first coverage run and the driver's environment drops with every ITOS_
// variable. With GOCOVERDIR
// unset the build is the plain one.
//
// ITOS_CC_FEATURES_BINARY names a binary built elsewhere for the scenarios
// to run instead of the one built from this tree, such as the binary of an
// older commit a refactor's proof compares (T-132). The build is then
// skipped whatever GOCOVERDIR says, and the binary runs as it is. Only the
// binary changes: the steps, the fixtures they read from this tree and the
// builds a step makes itself (Build, from this tree) stay this tree's.
// Unset or empty, the binary is built as above.
package features

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

var scenarios = flag.String("scenarios", "", "run only the live scenarios with a tag this regular expression matches")
var scenarioReport = flag.String("scenario-report", "", "also write Godog cucumber JSON to this file (test harness only)")

func TestFeatures(t *testing.T) {
	if !optedIn(os.Getenv) {
		t.Log(notOptedIn)
		return
	}
	sel, err := binarySelection(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(sel.Skipped) > 0 {
		t.Logf("not run here, left out by ITOS_CC_SKIP_FEATURES: %s", strings.Join(sel.Skipped, ", "))
	}
	if len(sel.Unconverted) > 0 {
		t.Logf("not run here, their feature files having no steps yet (q-52): %s", strings.Join(sel.Unconverted, " "))
	}
	if len(sel.Unstepped) > 0 {
		t.Logf("not run here, their steps not written yet (Unstepped): %s", strings.Join(sel.Unstepped, " "))
	}
	if sel.Filter == "" {
		return
	}
	t.Cleanup(removeCallerPath)
	t.Cleanup(removeToolCaches)
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	bin, err := underTest(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	at, err := concurrency()
	if err != nil {
		t.Fatal(err)
	}
	format := orderedFormat
	if *scenarioReport != "" {
		if strings.Contains(*scenarioReport, ",") {
			t.Fatal("scenario-report path cannot contain a Godog formatter separator (comma)")
		}
		format += ",cucumber:" + *scenarioReport
	}
	suite := godog.TestSuite{
		Name:                "itos-cc",
		ScenarioInitializer: func(sc *godog.ScenarioContext) { initialize(sc, binary{bin: bin, root: root}, interruptSteps(bin)) },
		Options: &godog.Options{
			Format:      format,
			Paths:       sel.Paths,
			Tags:        sel.Filter,
			Strict:      true,
			Concurrency: at,
			TestingT:    t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("a scenario failed")
	}
}

// binarySelection is what the binary driver runs of the feature files in
// dir: the live scenarios -scenarios selects.
func binarySelection(dir string) (Selection, error) {
	return TagFilter(*scenarios, dir, Converted, InProcessOnly)
}

// driverEnv opts into the binary driver when it is 1 (decision 52).
const driverEnv = "ITOS_CC_FEATURES_BINARY_DRIVER"

// notOptedIn is the one line TestFeatures logs when it runs nothing.
const notOptedIn = "the binary driver ran nothing: it runs only where " + driverEnv + "=1 or " +
	prebuiltEnv + " names a binary, as CI and CD set it; locally the scenarios run in process " +
	"against the fakes (decision 52)"

// optedIn says whether the binary driver runs here, reading the
// environment through getenv: driverEnv is 1, or prebuiltEnv names a
// binary, which a run against another binary (T-132) means as an opt-in.
func optedIn(getenv func(string) string) bool {
	return getenv(driverEnv) == "1" || getenv(prebuiltEnv) != ""
}

// concurrencyEnv says how many scenarios the binary driver runs at once.
const concurrencyEnv = "ITOS_CC_FEATURES_CONCURRENCY"

// concurrency is how many scenarios run at once: the number concurrencyEnv
// names, else as many as the CPUs Go runs on here (GOMAXPROCS, which
// follows the CPUs the system gives the process, a container's limit
// among them), so a runner of four CPUs runs four and one of three runs
// three. Each scenario spends most of its time waiting on the itos-cc,
// git and go processes it starts, which take the CPUs; more scenarios than
// CPUs would only make each wait longer, and stretch what the timing
// scenarios measure. Any scenario can run beside any other, each isolated
// as features/README.md's Running them says.
func concurrency() (int, error) {
	text := os.Getenv(concurrencyEnv)
	if text == "" {
		return runtime.GOMAXPROCS(0), nil
	}
	n, err := strconv.Atoi(text)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%s=%q: want a whole number of scenarios, 1 or more", concurrencyEnv, text)
	}
	return n, nil
}

// prebuiltEnv names the binary the scenarios run instead of building one.
const prebuiltEnv = "ITOS_CC_FEATURES_BINARY"

// underTest is the binary the scenarios run: the one prebuiltEnv names,
// made absolute, since each run starts in its scenario's folder, else the
// one build makes from the module at root in dir.
func underTest(root, dir string) (string, error) {
	named := os.Getenv(prebuiltEnv)
	if named == "" {
		return build(root, dir)
	}
	bin, err := filepath.Abs(named)
	if err != nil {
		return "", fmt.Errorf("%s: %w", prebuiltEnv, err)
	}
	info, err := os.Stat(bin)
	if err != nil {
		return "", fmt.Errorf("%s: %w", prebuiltEnv, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s: %s is not a regular file", prebuiltEnv, bin)
	}
	return bin, nil
}

// build builds cmd/itos-cc from the module at root into dir,
// stamped with stampedVersion and stampedCommit, once for every scenario of
// the run: itos-cc, or itos-cc.exe on windows, where a
// program is found by its extension. No VCS stamping, so the build runs no
// git of the caller's. With GOCOVERDIR set it is a -cover build (see the
// package's comment).
func build(root, dir string) (string, error) {
	name := "itos-cc"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(dir, name)
	args := []string{"build", "-trimpath", "-buildvcs=false",
		"-ldflags", "-X " + versionSymbol + "=" + stampedVersion + " -X " + commitSymbol + "=" + stampedCommit}
	if os.Getenv("GOCOVERDIR") != "" {
		args = append(args, "-cover", "-covermode=atomic", "-coverpkg=./...")
	}
	cmd := exec.Command("go", append(args, "-o", bin, "./cmd/itos-cc")...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("building itos-cc: %v\n%s", err, out)
	}
	return bin, nil
}

// The folder holding go.mod, above the working directory go test gives.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no go.mod above the working directory")
		}
		dir = parent
	}
}

// TestOptedIn holds when the binary driver runs: driverEnv at exactly 1, or
// prebuiltEnv naming any binary, whatever the other says.
func TestOptedIn(t *testing.T) {
	cases := []struct {
		driver, prebuilt string
		want             bool
	}{
		{"", "", false},
		{"0", "", false},
		{"true", "", false},
		{"1", "", true},
		{"", "/tmp/itos-cc", true},
		{"0", "/tmp/itos-cc", true},
	}
	for _, c := range cases {
		env := map[string]string{driverEnv: c.driver, prebuiltEnv: c.prebuilt}
		if got := optedIn(func(name string) string { return env[name] }); got != c.want {
			t.Errorf("%s=%q %s=%q: opted in %v, want %v", driverEnv, c.driver, prebuiltEnv, c.prebuilt, got, c.want)
		}
	}
}
