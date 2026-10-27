package features

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

// The steps of the Rule "A site a report names is judged alone, freshly,
// with the tests the nightly picks" (slice-63, issue #68). They reuse
// slice-41's fixture, the module example.com/fx whose board.Count has one
// site, `<` → `<=` at board/board.go:5:11, which its test survives, and
// read only what itos-cc prints and writes: its --json objects, its plain
// output, the trials its stderr announces and the files it leaves.

const (
	// siteIdentity is the identity a counted run reports for Count's site.
	siteIdentity = "board/board.go:example.com/fx/board#Count@4:6:b1a8f41702cd975f:35:<><="
	// siteOver is Over, below Count in board/board.go: its one site is
	// `>` at 10:11, which siteOverTest survives, never checking Over(10).
	siteOver     = "\n// Over says whether x is over ten.\nfunc Over(x int) bool {\n\treturn x > 10\n}\n"
	siteOverTest = "\nfunc TestOver(t *testing.T) {\n\tif !Over(11) || Over(3) {\n\t\tt.Fatal(\"Over\")\n\t}\n}\n"
	// siteIdle is a test of the board package that calls nothing.
	siteIdle = "package board\n\nimport \"testing\"\n\nfunc TestNothing(t *testing.T) {}\n"
	// siteCountMain is a program the listed test builds, printing whether
	// its argument's length is under five.
	siteCountMain = "package main\n\nimport (\n\t\"fmt\"\n\t\"os\"\n\n\t\"example.com/fx/board\"\n)\n\nfunc main() {\n\tfmt.Println(board.Count(len(os.Args[1])))\n}\n"
	// siteHarness is the listed tests' harness: ID-A-01 builds the program
	// and checks that a word of five letters is not under five, which
	// Count's mutant fails; with ITOS_CC_TEST_COVERDIR set, it builds the
	// program with -cover and runs it with GOCOVERDIR=<it>/ID-A-01.
	siteHarness = `package e2e

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var selected = flag.String("tests", "", "the IDs of the tests to run, by commas: all of them when empty")

func TestListed(t *testing.T) {
	if *selected != "" && *selected != "ID-A-01" {
		t.Fatalf("unexpected tests %q", *selected)
	}
	split := os.Getenv("ITOS_CC_TEST_COVERDIR")
	bin := filepath.Join(t.TempDir(), "count.exe")
	build := []string{"build", "-o", bin, "../cmd/count"}
	if split != "" || os.Getenv("GOCOVERDIR") != "" {
		build = []string{"build", "-cover", "-o", bin, "../cmd/count"}
	}
	if out, err := exec.Command("go", build...).CombinedOutput(); err != nil {
		t.Fatalf("go %v: %v\n%s", build, err, out)
	}
	cmd := exec.Command(bin, "fives")
	if split != "" {
		dir := filepath.Join(split, "ID-A-01")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		cmd.Env = append(os.Environ(), "GOCOVERDIR="+dir)
	}
	out, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(out)) != "false" {
		t.Fatalf("ID-A-01: %v, printed %q, want false", err, out)
	}
}
`
	siteListedConfig = `mutation:
  tests:
    list: printf "ID-A-01\n"
    run: go test -count=1 ./e2e -args -tests={pattern}
    ids_pattern: "{ids}"
    join:
      each: "{id}"
      sep: ","
`
	siteListedCommand = "go test -count=1 ./e2e -args -tests={pattern}"
)

// namedSite is the state the steps keep: whether board/board.go holds
// Over, and the source HEAD commits.
type namedSite struct {
	i      *identityException
	over   bool
	source string
}

func (i *identityException) namedSiteSteps(sc *godog.ScenarioContext) {
	n := &namedSite{i: i}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*n = namedSite{i: i, source: identitySource}
		return ctx, nil
	})
	sc.Step("^board/board\\.go also holds Over, whose `>` → `>=` at board/board\\.go:10:11 its test survives too$", n.withOver)
	sc.Step(`^a later commit's test also checks Count\(5\)$`, n.killingCommit)
	sc.Step(`^a later commit added a blank line above Count's comment, and its test also checks Count\(5\)$`, n.movedCommit)
	sc.Step(`^board/board_test\.go now also checks Count\(5\), not committed$`, n.uncommittedKill)
	sc.Step(`^a committed Go module whose "example\.com/fx/board#Count" no test calls$`, n.uncalled)
	sc.Step(`^itos-cc\.yaml excepts nothing$`, n.counted)
	sc.Step("^itos-cc\\.yaml excepts Count's `<` → `<=` with reason \"x is never 5\"$", n.excepted)
	sc.Step(`^mutation\.tests lists a test that reaches board/board\.go line 5 and kills the mutant$`, n.listed)
	sc.Step(`^nothing changed, committed$`, n.counted)
	sc.Step(`^Count's "x < 5" became "x < 6", committed$`, func() error {
		return n.commit(map[string]string{identityFile: strings.Replace(identitySource, "x < 5", "x < 6", 1)}, "Count under six")
	})
	sc.Step(`^Count was renamed Below, committed$`, func() error {
		return n.commit(map[string]string{identityFile: strings.ReplaceAll(identitySource, "Count", "Below")}, "Below")
	})
	sc.Step(`^itos-cc\.yaml excludes "board/\*\*", committed$`, func() error {
		return n.commit(map[string]string{"itos-cc.yaml": "exclude:\n  - board/**\n"}, "exclude board")
	})
	sc.Step(`^board/board\.go became board/board_plan9\.go, committed$`, n.plan9)
	sc.Step(`^"selected" lists one site, "([^"]*)", its state "([^"]*)" and its outcome "([^"]*)"$`, n.listsOne)
	sc.Step(`^exactly one mutation trial ran, and none of Over's$`, func() error { return n.trials(5) })
	sc.Step(`^exactly two mutation trials ran$`, func() error { return n.trials(5, 10) })
	sc.Step(`^no mutation trial ran$`, func() error { return n.trials() })
	sc.Step(`^neither \.metrics/mutate nor board/board\.go was written$`, n.nothingWritten)
	sc.Step(`^stdout says that only the judgments above are proven and nothing was written$`, n.provesOnly)
	sc.Step(`^no stdout line starts with a path or holds "([^"]*)"$`, n.noPathLine)
	sc.Step(`^the problem is "(mutation\.survived)", its message naming the identity, its fix "(.*)"$`, n.survived)
	sc.Step(`^the selected site is "([^"]*)", "excepted" "([^"]*)", and there is no problem$`, n.exceptedSite)
	sc.Step(`^the problem is "([^"]*)", with file "([^"]*)", line (\d+) and column (\d+)$`, n.problemAt)
	sc.Step(`^the problem is "([^"]*)", at board/board\.go line (\d+) column (\d+)$`, func(rule string, line, column int) error {
		return n.problemAt(rule, identityFile, line, column)
	})
	sc.Step(`^the only problem is "([^"]*)", at board/board\.go line (\d+) column (\d+)$`, n.onlyProblemAt)
	sc.Step(`^"selected" lists two sites: Count's "([^"]*)" and Over's "([^"]*)"$`, n.listsTwo)
	sc.Step(`^"named" lists the three arguments in order, the first and the third with Count's identity$`, n.namedArguments)
	sc.Step(`^the problem is "([^"]*)", with argument "([^"]*)" and why "([^"]*)"$`, n.unmatched)
	sc.Step(`^the problems are two "([^"]*)", with arguments "([^"]*)" and "([^"]*)"$`, n.twoUnmatched)
	sc.Step(`^no test or coverage command ran$`, func() error { return strictNoCommands(i.w.stderr) })
	sc.Step(`^the problem is "([^"]*)", with flag "(--site)" and value "([^"]*)"$`, n.flagValue)
	sc.Step(`^stderr warns that the run judged HEAD's commit and that board/board_test\.go goes unjudged$`, n.unjudged)
	sc.Step(`^the selected site's stages are "own" survived, "listed-baseline" complete and "listed" killed, its scope "listed" with that test$`, n.listedStages)
}

// counted skips the scenario where itos-cc makes no counted run: a --site
// run is one.
func (n *namedSite) counted() error { return n.i.w.d.Counted() }

// commit writes files, commits every change and skips the scenario where
// itos-cc makes no counted run.
func (n *namedSite) commit(files map[string]string, message string) error {
	w := n.i.w
	if err := n.counted(); err != nil {
		return err
	}
	for _, name := range slices.Sorted(maps.Keys(files)) {
		if err := w.write(name, files[name]); err != nil {
			return err
		}
		if name == identityFile {
			n.source = files[name]
		}
	}
	if err := w.git("add", "-A"); err != nil {
		return err
	}
	return w.git("commit", "-qm", message)
}

// declare says what the board tests do, the sites in surviving surviving
// and every other mutant killed, and what they cover: profile's blocks.
func (n *namedSite) declare(surviving []Site, profile string) error {
	w := n.i.w
	if err := w.d.Tests(w.dir, Tests{Language: "go", Command: identityCommand, Surviving: surviving}); err != nil {
		return err
	}
	return w.d.Coverage(w.dir, Coverage{Language: "go", Report: profile})
}

var (
	siteCount     = Site{File: identityFile, Line: 5, Original: "<", Replacement: "<="}
	siteOverSite  = Site{File: identityFile, Line: 10, Original: ">", Replacement: ">="}
	siteOverBlock = "example.com/fx/board/board.go:10.2,11.1 1 1\n"
)

func (n *namedSite) withOver() error {
	if err := n.commit(map[string]string{identityFile: identitySource + siteOver, "board/board_test.go": identityTest + siteOverTest}, "Over"); err != nil {
		return err
	}
	n.over = true
	return n.declare([]Site{siteCount, siteOverSite}, identityProfile+siteOverBlock)
}

func (n *namedSite) killingCommit() error {
	test, surviving, profile := identityKilling, []Site{}, identityProfile
	if n.over {
		test, surviving, profile = identityKilling+siteOverTest, []Site{siteOverSite}, identityProfile+siteOverBlock
	}
	if err := n.commit(map[string]string{"board/board_test.go": test}, "check Count(5)"); err != nil {
		return err
	}
	return n.declare(surviving, profile)
}

func (n *namedSite) movedCommit() error {
	if err := n.commit(map[string]string{identityFile: strings.Replace(identitySource, "// Count", "\n// Count", 1), "board/board_test.go": identityKilling}, "move Count"); err != nil {
		return err
	}
	return n.declare(nil, "mode: set\nexample.com/fx/board/board.go:6.2,7.1 1 1\n")
}

// uncommittedKill writes the killing test without committing it: the
// tests HEAD commits still survive Count's mutant.
func (n *namedSite) uncommittedKill() error {
	if err := n.counted(); err != nil {
		return err
	}
	return n.i.w.write("board/board_test.go", identityKilling)
}

func (n *namedSite) uncalled() error {
	if err := n.i.module(siteIdle, false); err != nil {
		return err
	}
	if err := n.counted(); err != nil {
		return err
	}
	return n.declare(nil, "mode: set\nexample.com/fx/board/board.go:5.2,6.1 1 0\n")
}

// excepted excepts Count's survivor from its identity, which runs nothing
// (slice-41), and commits the itos-cc.yaml that writes.
func (n *namedSite) excepted() error {
	w := n.i.w
	if err := n.counted(); err != nil {
		return err
	}
	if err := w.runWith(w.env(), "mutation except '"+siteIdentity+"' --reason '"+identityReason+"'"); err != nil {
		return err
	}
	if w.exit != 0 {
		return fmt.Errorf("excepting Count's survivor exited %d\n%s", w.exit, w.report())
	}
	if err := w.git("add", "itos-cc.yaml"); err != nil {
		return err
	}
	return w.git("commit", "-qm", "except Count's survivor")
}

// listed commits a listed test, ID-A-01, that reaches Count's line through
// a program it builds and kills its mutant, and declares it.
func (n *namedSite) listed() error {
	w := n.i.w
	if err := n.commit(map[string]string{"cmd/count/main.go": siteCountMain, "e2e/e2e_test.go": siteHarness, "itos-cc.yaml": siteListedConfig}, "listed test"); err != nil {
		return err
	}
	listed := &ListedTests{Files: map[string]string{"ID-A-01": "e2e/e2e_test.go"}, Lines: map[string]map[int][]string{identityFile: {5: {"ID-A-01"}}}, Split: true}
	if err := w.d.Tests(w.dir, Tests{Language: "go", Command: siteListedCommand, Listed: listed}); err != nil {
		return err
	}
	return n.declare([]Site{siteCount}, identityProfile)
}

func (n *namedSite) plan9() error {
	w := n.i.w
	if err := n.counted(); err != nil {
		return err
	}
	if err := w.write("board/board_plan9.go", identitySource); err != nil {
		return err
	}
	if err := w.d.Remove(filepath.Join(w.dir, filepath.FromSlash(identityFile))); err != nil {
		return err
	}
	if err := w.git("add", "-A"); err != nil {
		return err
	}
	return w.git("commit", "-qm", "plan9 only")
}

// object is the --json object of the last run, after skipping where
// itos-cc makes no counted run.
func (n *namedSite) object() (map[string]any, string, error) {
	if err := n.counted(); err != nil {
		return nil, "", err
	}
	return n.i.w.lastObject()
}

func (n *namedSite) listsOne(identity, state, outcome string) error {
	object, report, err := n.object()
	if err != nil {
		return err
	}
	selected := objectList(object, "selected")
	if len(selected) != 1 || selected[0]["identity"] != identity || selected[0]["state"] != state || selected[0]["outcome"] != outcome {
		return fmt.Errorf("\"selected\" is not one site %s, %s and %s\n%s", identity, state, outcome, report)
	}
	return nil
}

// trials is whether the mutants of board/board.go the last run's stderr
// says it ran are those of lines, in any order.
func (n *namedSite) trials(lines ...int) error {
	w := n.i.w
	var ran []int
	for _, t := range trialsOf(w.stderr, identityFile) {
		ran = append(ran, t.line)
	}
	slices.Sort(ran)
	slices.Sort(lines)
	if !slices.Equal(ran, lines) {
		return fmt.Errorf("the run's trials were of lines %v, not %v\n%s", ran, lines, w.report())
	}
	return nil
}

func (n *namedSite) nothingWritten(ctx context.Context) error {
	w := n.i.w
	files, err := w.d.Files(ctx, w.dir)
	if err != nil {
		return err
	}
	for _, file := range files {
		if strings.HasPrefix(file, ".metrics/") {
			return fmt.Errorf("the run wrote %s", file)
		}
	}
	data, err := w.readIn(w.dir, identityFile)
	if err != nil {
		return err
	}
	if string(data) != n.source {
		return fmt.Errorf("board/board.go was written\n%s", data)
	}
	return nil
}

func (n *namedSite) provesOnly() error {
	w := n.i.w
	for _, text := range []string{"only the judgments above are proven", "no snapshot, annotation or coverage cache was written"} {
		if !strings.Contains(w.stdout, text) {
			return fmt.Errorf("stdout does not say %q\n%s", text, w.report())
		}
	}
	return nil
}

func (n *namedSite) noPathLine(text string) error {
	w := n.i.w
	for _, line := range outputLines(w.stdout) {
		if strings.Contains(line, text) || setupGoPattern.MatchString(line) {
			return fmt.Errorf("the stdout line %q starts with a path or holds %q\n%s", line, text, w.report())
		}
	}
	return nil
}

func (n *namedSite) survived(rule, fix string) error {
	object, report, err := n.object()
	if err != nil {
		return err
	}
	problems := checkProblems(object)
	if len(problems) != 1 || problems[0]["rule"] != rule || !strings.Contains(fmt.Sprint(problems[0]["message"]), siteIdentity) ||
		!strings.Contains(fmt.Sprint(problems[0]["fix"]), fix) {
		return fmt.Errorf("not one %s naming %s, its fix %q\n%s", rule, siteIdentity, fix, report)
	}
	return nil
}

func (n *namedSite) exceptedSite(outcome, reason string) error {
	object, report, err := n.object()
	if err != nil {
		return err
	}
	selected := objectList(object, "selected")
	if len(selected) != 1 || selected[0]["outcome"] != outcome || selected[0]["excepted"] != reason || len(checkProblems(object)) != 0 || object["ok"] != true {
		return fmt.Errorf("the selected site is not %s, excepted %q, with no problem\n%s", outcome, reason, report)
	}
	return nil
}

// problemAt is whether the last run reports a problem of rule at file,
// line and column.
func (n *namedSite) problemAt(rule, file string, line, column int) error {
	object, report, err := n.object()
	if err != nil {
		return err
	}
	for _, p := range checkProblems(object) {
		if p["rule"] == rule && filepath.ToSlash(fmt.Sprint(p["file"])) == file && p["line"] == float64(line) && p["column"] == float64(column) {
			return nil
		}
	}
	return fmt.Errorf("no %s at %s line %d column %d\n%s", rule, file, line, column, report)
}

func (n *namedSite) onlyProblemAt(rule string, line, column int) error {
	object, report, err := n.object()
	if err != nil {
		return err
	}
	if len(checkProblems(object)) != 1 {
		return fmt.Errorf("not one problem\n%s", report)
	}
	return n.problemAt(rule, identityFile, line, column)
}

// namedSiteOf is the selected site of function, by its name after #.
func namedSiteOf(selected []map[string]any, name string) map[string]any {
	for _, s := range selected {
		if strings.HasSuffix(fmt.Sprint(s["function"]), "#"+name) {
			return s
		}
	}
	return nil
}

func (n *namedSite) listsTwo(count, over string) error {
	object, report, err := n.object()
	if err != nil {
		return err
	}
	selected := objectList(object, "selected")
	c, o := namedSiteOf(selected, "Count"), namedSiteOf(selected, "Over")
	if len(selected) != 2 || c == nil || o == nil || c["outcome"] != count || o["outcome"] != over {
		return fmt.Errorf("\"selected\" is not Count's %s and Over's %s\n%s", count, over, report)
	}
	return nil
}

// sites are the --site arguments of the last run, in order.
func (n *namedSite) sites() []string {
	var out []string
	args := n.i.w.last.args
	for k, arg := range args {
		if arg == "--site" && k+1 < len(args) {
			out = append(out, args[k+1])
		}
	}
	return out
}

func (n *namedSite) namedArguments() error {
	object, report, err := n.object()
	if err != nil {
		return err
	}
	named := objectList(object, "named")
	sites := n.sites()
	if len(named) != 3 || len(sites) != 3 {
		return fmt.Errorf("\"named\" does not list three arguments\n%s", report)
	}
	for k, entry := range named {
		identities, _ := entry["identities"].([]any)
		if entry["argument"] != sites[k] || len(identities) != 1 {
			return fmt.Errorf("\"named\"[%d] is not %q with one identity\n%s", k, sites[k], report)
		}
		if (k == 0 || k == 2) && identities[0] != siteIdentity || k == 1 && identities[0] == siteIdentity {
			return fmt.Errorf("\"named\"[%d] names %v\n%s", k, identities, report)
		}
	}
	return nil
}

func (n *namedSite) unmatched(rule, argument, why string) error {
	if err := n.counted(); err != nil {
		return err
	}
	w := n.i.w
	if err := w.problemIs(rule, "argument", argument); err != nil {
		return err
	}
	if w.problem["why"] != why {
		return fmt.Errorf("the problem %v does not say why %q", w.problem, why)
	}
	return nil
}

func (n *namedSite) twoUnmatched(rule, first, second string) error {
	object, report, err := n.object()
	if err != nil {
		return err
	}
	problems := checkProblems(object)
	if len(problems) != 2 || problems[0]["rule"] != rule || problems[1]["rule"] != rule || problems[0]["argument"] != first || problems[1]["argument"] != second {
		return fmt.Errorf("the problems are not two %s, of %q and %q\n%s", rule, first, second, report)
	}
	return nil
}

func (n *namedSite) flagValue(rule, flag, value string) error {
	w := n.i.w
	if err := w.problemIs(rule, "flag", flag); err != nil {
		return err
	}
	if w.problem["value"] != value {
		return fmt.Errorf("the problem %v does not give the value %q", w.problem, value)
	}
	return nil
}

func (n *namedSite) unjudged() error {
	w := n.i.w
	if !strings.Contains(w.stderr, "itos-cc: warning: this run judges commit ") || !strings.Contains(w.stderr, "itos-cc: unjudged uncommitted change: board/board_test.go") {
		return fmt.Errorf("stderr does not warn that board/board_test.go goes unjudged\n%s", w.report())
	}
	return nil
}

func (n *namedSite) listedStages() error {
	object, report, err := n.object()
	if err != nil {
		return err
	}
	selected := objectList(object, "selected")
	if len(selected) != 1 {
		return fmt.Errorf("not one selected site\n%s", report)
	}
	s := selected[0]
	stages := objectList(s, "stages")
	want := []struct{ name, state, outcome string }{{"own", "complete", "survived"}, {"listed-baseline", "complete", ""}, {"listed", "complete", "killed"}}
	if len(stages) != len(want) {
		return fmt.Errorf("the stages are not own, listed-baseline and listed\n%s", report)
	}
	for k, stage := range stages {
		outcome, _ := stage["outcome"].(string)
		if stage["name"] != want[k].name || stage["state"] != want[k].state || outcome != want[k].outcome {
			return fmt.Errorf("stage %d is %v, not %v\n%s", k, stage, want[k], report)
		}
	}
	if tests := fmt.Sprint(s["tests"]); s["scope"] != "listed" || tests != "[ID-A-01]" || s["outcome"] != "killed" {
		return fmt.Errorf("the site is not killed in scope listed by ID-A-01\n%s", report)
	}
	return nil
}
