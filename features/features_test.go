// The features: every converted *.feature file in this folder, run by godog
// through go test against the itos-cc binary built once a run from this
// tree, through the binary driver (binary.go), each scenario a subtest of
// TestFeatures. Harvested from itos's
// (github.com/donvargax/itos, features/features_test.go) and the owner's
// itos-template's, without either one's generator behaviour.
//
//	go test ./features -count=1                        every live scenario
//	go test ./features -count=1 -scenarios=<regexp>    the live scenarios with a
//	                                                   tag the expression matches
//	GOCOVERDIR=<dir> go test ./features -count=1       either, itos-cc
//	                                                   built with -cover, what
//	                                                   the scenarios ran in <dir>
//
// -scenarios takes the regular expression itos's run templates join the IDs
// they select into, which TagFilter turns into godog's filter. A scenario
// tagged @wip never runs.
//
// Only the feature files in Converted run here: those whose scenarios have
// steps (q-52), the files of commands not moved yet among them, which the
// in-process driver leaves to this one until they move (InProcess). The
// others are converted over time, and until then their scenarios are held
// by the Go tests of cmd/itos-cc; a selection naming one of theirs logs it
// as not run here, and a selection naming nothing else fails, as one
// matching no tag does.
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
// GOCOVERDIR's being set guarantees every command a scenario starts has it.
// The binary driver's environment passes it on unchanged, every environment
// a step runs a command in is that one with the scenario's variables set.
// Never ITOS_CC_TEST_COVERDIR, which itos-cc sets alone for a listed test's
// first coverage run and the driver's environment drops with every ITOS_
// variable. With GOCOVERDIR
// unset the build is the plain one.
package features

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

var scenarios = flag.String("scenarios", "", "run only the live scenarios with a tag this regular expression matches")

// versionSymbol and commitSymbol are the variables the build stamps
// (-ldflags -X).
const (
	versionSymbol = "github.com/donvargax/itos-cc/internal/version.stamp"
	commitSymbol  = "github.com/donvargax/itos-cc/internal/version.commit"
)

func TestFeatures(t *testing.T) {
	sel, err := TagFilter(*scenarios, ".", Converted, InProcessOnly)
	if err != nil {
		t.Fatal(err)
	}
	if len(sel.Unconverted) > 0 {
		t.Logf("not run here, their feature files having no steps yet (q-52): %s", strings.Join(sel.Unconverted, " "))
	}
	t.Cleanup(removeCallerPath)
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	bin, err := build(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	suite := godog.TestSuite{
		Name:                "itos-cc",
		ScenarioInitializer: func(sc *godog.ScenarioContext) { initialize(sc, binary{bin: bin, root: root}, interruptSteps(bin)) },
		Options: &godog.Options{
			Format:      "pretty",
			Paths:       Converted,
			Tags:        sel.Filter,
			Strict:      true,
			Concurrency: 1,
			TestingT:    t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("a scenario failed")
	}
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
