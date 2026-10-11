// The features: every *.feature file in this folder, run by godog through go
// test against the itos-cc binary built once a run from this tree,
// each scenario a subtest of TestFeatures. Harvested from itos's
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
// godog's own tag filter takes exact tags joined by commas; itos's run
// templates join the IDs they select with |, as a regular expression
// (itos.yaml's tests.scenario.run), so -scenarios takes that expression and
// is turned here into godog's filter. A scenario tagged @wip never runs.
//
// Only the feature files in converted run here: those whose scenarios have
// steps (q-52). The others are converted over time, each with the command
// that moves onto kong, and until then their scenarios are held by the Go
// tests of cmd/itos-cc; a selection naming one of theirs logs it as not run
// here, and a selection naming nothing else fails, as one matching no tag
// does.
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
// w.env() passes it on unchanged, every environment a step runs a command in
// is w.env() changed elsewhere (noGit, knowingNoOne, withGitConfig). Never
// ITOS_CC_TEST_COVERDIR, which itos-cc sets alone for a listed test's first
// coverage run and w.env() drops with every ITOS_ variable. With GOCOVERDIR
// unset the build is the plain one.
package features

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

var scenarios = flag.String("scenarios", "", "run only the live scenarios with a tag this regular expression matches")

// stampedVersion is the version the binary under test is stamped with, as a
// release's build stamps the release's, so a scenario can tell the stamp
// from the version a build without one says.
const stampedVersion = "1.2.3-features"

// stampedCommit is the commit the binary under test is stamped with, as a
// release's build stamps the commit it was built from, so a scenario can
// name the commit the binary says it was built from.
const stampedCommit = "0123456789abcdef0123456789abcdef01234567"

// versionSymbol and commitSymbol are the variables the build stamps
// (-ldflags -X).
const (
	versionSymbol = "github.com/donvargax/itos-cc/internal/version.stamp"
	commitSymbol  = "github.com/donvargax/itos-cc/internal/version.commit"
)

// converted are the feature files this harness runs, those whose scenarios
// have steps: count.feature, the template's sample, every scenario of it
// @wip (T-26), and units.feature, the first command moved onto kong (T-32).
// A command that moves converts its own file and adds it here, its scenarios'
// text unchanged (q-52).
var converted = []string{"count.feature", "units.feature"}

func TestFeatures(t *testing.T) {
	filter, unconverted, err := tagFilter(*scenarios, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(unconverted) > 0 {
		t.Logf("not run here, their feature files having no steps yet (q-52): %s", strings.Join(unconverted, " "))
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
		ScenarioInitializer: func(sc *godog.ScenarioContext) { initializeScenario(sc, root, bin) },
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    converted,
			Tags:     filter,
			Strict:   true,
			TestingT: t,
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

// godog's tag filter for a selection: every live scenario of the converted
// files without one, else the live scenarios of the converted files carrying
// a tag the expression matches, with the tags it matches only in other
// files, which this harness does not run. An expression that matches no tag
// of a converted file is an error, so a selection never runs nothing.
func tagFilter(selection, dir string) (filter string, unconverted []string, err error) {
	if selection == "" {
		return "~@wip", nil, nil
	}
	pattern, err := regexp.Compile(selection)
	if err != nil {
		return "", nil, err
	}
	var files []string
	for _, name := range converted {
		files = append(files, filepath.Join(dir, name))
	}
	tags, err := featureTags(files)
	if err != nil {
		return "", nil, err
	}
	var each []string
	for _, tag := range tags {
		if pattern.MatchString(tag) {
			each = append(each, tag+"&&~@wip")
		}
	}
	all, err := filepath.Glob(filepath.Join(dir, "*.feature"))
	if err != nil {
		return "", nil, err
	}
	everyTag, err := featureTags(all)
	if err != nil {
		return "", nil, err
	}
	for _, tag := range everyTag {
		if pattern.MatchString(tag) && !slices.Contains(tags, tag) {
			unconverted = append(unconverted, tag)
		}
	}
	if len(each) == 0 {
		return "", unconverted, &noMatch{selection, unconverted}
	}
	return strings.Join(each, ","), unconverted, nil
}

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
