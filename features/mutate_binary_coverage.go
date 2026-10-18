package features

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Coverage from tests that run the built binary.

// The module of "Coverage from tests that run the built binary": greet,
// tested only by e2e/e2e_test.go, which builds the binary, with -cover
// when GOCOVERDIR is set, records where GOCOVERDIR points, and runs it with
// one argument, so it reaches "one" and never "many", and checks stdout
// alone, so what main writes to stderr is never checked.
const (
	greetGo = `package main

import (
	"fmt"
	"os"
)

func main() {
	n := len(os.Args) - 1
	fmt.Println(greet(n))
	fmt.Fprintln(os.Stderr, n*2)
}

// greet tells one from many.
func greet(n int) string {
	if n > 1 {
		return fmt.Sprint(n-1, " more")
	}
	return "one"
}

// half is never run.
func half(i int) bool {
	return i > 2
}
`
	greetE2E = `package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const records = %s

func TestOne(t *testing.T) {
	if dir := os.Getenv("GOCOVERDIR"); dir != "" {
		f, err := os.OpenFile(filepath.Join(records, "gocoverdir"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString(dir + "\n")
		f.Close()
	}
	bin := filepath.Join(t.TempDir(), "greet.exe")
	build := []string{"build", "-o", bin, ".."}
	%s
	if out, err := exec.Command("go", build...).CombinedOutput(); err != nil {
		t.Fatalf("go %%v: %%v\n%%s", build, err, out)
	}
	out, err := exec.Command(bin, "x").Output()
	if err != nil {
		t.Fatalf("greet x: %%v", err)
	}
	if got := strings.TrimSpace(string(out)); got != "one" {
		t.Fatalf("greet x printed %%q, want \"one\"", got)
	}
}
`
	greetCover = "if os.Getenv(\"GOCOVERDIR\") != \"\" {\n\t\tbuild = []string{\"build\", \"-cover\", \"-o\", bin, \"..\"}\n\t}"
	greetMany  = "package main\n\nimport \"testing\"\n\nfunc TestMany(t *testing.T) {\n" +
		"\tif got := greet(3); got != \"2 more\" {\n\t\tt.Fatalf(\"greet(3) = %q\", got)\n\t}\n}\n"
)

// The lines of main.go: those the binary runs, the branch it never takes
// and half, which nothing runs.
var (
	greetRan   = []int{9, 11, 16}
	greetNever = []int{17, 24}
)

// greetModule writes the module, its test building the binary with -cover
// where cover, and with a test in package main calling greet(3) where
// inProcess, and declares what its tests do and cover.
func (m *mutateState) greetModule(cover, inProcess bool) error {
	records, err := m.testRecords()
	if err != nil {
		return err
	}
	m.file = "main.go"
	flag := ""
	if cover {
		flag = greetCover
	}
	files := map[string]string{
		"go.mod": "module example.com/greet\n\ngo 1.22\n", m.file: greetGo,
		"e2e/e2e_test.go": fmt.Sprintf(greetE2E, strconv.Quote(records), flag),
	}
	if inProcess {
		files["main_test.go"] = greetMany
	}
	if err := m.writeAll(files); err != nil {
		return err
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "go"}); err != nil {
		return err
	}
	return m.w.d.Tests(m.w.dir, Tests{Language: "go", Command: "go test -count=1 -failfast ./...",
		Surviving: []Site{{m.file, 11, "*", "/"}, {m.file, 17, "-", "+"}, {m.file, 17, "1", "0"}, {m.file, 24, ">", ">="}}})
}

// greetChecksOneBranch checks the test reads the binary's "one", and never
// its other branch's "more".
func (m *mutateState) greetChecksOneBranch() error {
	test := m.read("e2e/e2e_test.go")
	if !strings.Contains(test, `got != "one"`) || strings.Contains(test, "more") {
		return errors.New("e2e/e2e_test.go does not check one branch of the binary's output alone")
	}
	return nil
}

// greetSites are the sites of main.go on lines, every one where lines is
// nil.
func (m *mutateState) greetSites(lines []int) ([]listedSite, error) {
	return m.sitesWhere(m.file, func(s listedSite) bool { return lines == nil || slices.Contains(lines, s.line) })
}

// greetRanCovered is whether the last run ran every mutant on the lines
// the binary ran, listing none of them uncovered.
func (m *mutateState) greetRanCovered() error {
	sites, err := m.greetSites(greetRan)
	if err != nil {
		return err
	}
	ran := trialsOf(m.w.stderr, m.file)
	for _, s := range sites {
		if !slices.ContainsFunc(ran, func(t trial) bool {
			return t.line == s.line && t.original == s.original && t.replacement == s.replacement
		}) {
			return fmt.Errorf("the mutant %s did not run\n%s", s, m.w.report())
		}
	}
	if listed := uncoveredListed(m.w.stdout, m.file); slices.ContainsFunc(listed, func(s listedSite) bool { return slices.Contains(greetRan, s.line) }) {
		return fmt.Errorf("a mutant on a line the binary ran is listed as uncovered: %v\n%s", listed, m.w.report())
	}
	return nil
}

// outcomeOf is what the last run said of the mutant of the file at line
// replacing original with replacement, "" when it ran none.
func (m *mutateState) outcomeOf(line int, original, replacement string) string {
	for _, t := range trialsOf(m.w.stderr, m.file) {
		if t.line == line && t.original == original && t.replacement == replacement {
			return t.outcome
		}
	}
	return ""
}

// greetNotices is whether the mutant that makes the binary print "0 more"
// was killed, and the one changing only what it writes to stderr survived.
func (m *mutateState) greetNotices() error {
	if got := m.outcomeOf(16, ">", ">="); got != "killed" {
		return fmt.Errorf("main.go:16 > → >= %q, not killed: the binary prints \"0 more\"\n%s", got, m.w.report())
	}
	if got := m.outcomeOf(11, "*", "/"); got != "survived" {
		return fmt.Errorf("main.go:11 * → / %q, not survived: the test never reads stderr\n%s", got, m.w.report())
	}
	return nil
}

// greetNeverRanUncovered is whether stdout lists as uncovered the mutants
// on the lines the binary never ran, and no other.
func (m *mutateState) greetNeverRanUncovered() error {
	never, err := m.greetSites(greetNever)
	if err != nil {
		return err
	}
	if listed := uncoveredListed(m.w.stdout, m.file); len(never) == 0 || !sameSites(listed, never) {
		return fmt.Errorf("stdout lists %v as uncovered, not %v\n%s", listed, never, m.w.report())
	}
	return nil
}

// greetAllUncovered is whether stdout lists every mutant of main.go as
// uncovered.
func (m *mutateState) greetAllUncovered() error {
	return m.everyListedUncovered(m.file)
}

// greetMutants are main.go's mutants in the last run's --json object.
func (m *mutateState) greetMutants() ([]map[string]any, error) {
	object, err := m.w.json()
	if err != nil {
		return nil, err
	}
	files, _ := object["files"].([]any)
	for _, one := range files {
		f, _ := one.(map[string]any)
		if name, _ := f["file"].(string); filepath.ToSlash(name) != m.file {
			continue
		}
		var out []map[string]any
		list, _ := f["mutants"].([]any)
		for _, mu := range list {
			if entry, ok := mu.(map[string]any); ok {
				out = append(out, entry)
			}
		}
		if len(out) > 0 {
			return out, nil
		}
	}
	return nil, fmt.Errorf("the object lists no mutant of %s\n%s", m.file, m.w.report())
}

// lineOf is a mutant's line in the --json object.
func lineOf(mu map[string]any) int {
	line, _ := mu["line"].(float64)
	return int(line)
}

// greetCoverageKinds is whether each mutant the last run covered lists,
// as "coverage", the data covering its line: integration on the lines
// only the binary runs, in-process on the branch only greet(3) takes, and
// both on the line both reach.
func (m *mutateState) greetCoverageKinds(inProcess, integration string) error {
	want := map[int][]any{9: {integration}, 11: {integration}, 16: {inProcess, integration}, 17: {inProcess}}
	mutants, err := m.greetMutants()
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, mu := range mutants {
		if mu["outcome"] == "uncovered" {
			continue
		}
		got, _ := mu["coverage"].([]any)
		if !slices.Equal(got, want[lineOf(mu)]) {
			return fmt.Errorf("the mutant at line %d has \"coverage\" %v, not %v\n%s", lineOf(mu), mu["coverage"], want[lineOf(mu)], m.w.report())
		}
		seen[fmt.Sprint(got)] = true
	}
	if len(seen) != 3 {
		return fmt.Errorf("the covered mutants list %d kinds of coverage, not in-process, integration and both\n%s", len(seen), m.w.report())
	}
	return nil
}

// uncoveredHasNoCoverage is whether the last run left a mutant uncovered,
// and no uncovered one has "coverage".
func (m *mutateState) uncoveredHasNoCoverage() error {
	mutants, err := m.greetMutants()
	if err != nil {
		return err
	}
	n := 0
	for _, mu := range mutants {
		if mu["outcome"] != "uncovered" {
			continue
		}
		n++
		if c, has := mu["coverage"]; has {
			return fmt.Errorf("the uncovered mutant at line %d has \"coverage\" %v\n%s", lineOf(mu), c, m.w.report())
		}
	}
	if n == 0 {
		return fmt.Errorf("no mutant is uncovered\n%s", m.w.report())
	}
	return nil
}

// coverDir is the GOCOVERDIR the binary was first given, slash-separated
// from the scratch folder, and the run-* folder it is under.
func (m *mutateState) coverDir() (dir, run string, err error) {
	if m.records == "" {
		return "", "", errors.New("the project's test records nothing")
	}
	data, err := m.w.d.Read(filepath.Join(m.records, "gocoverdir"))
	if err != nil {
		return "", "", fmt.Errorf("the test was given no GOCOVERDIR: %v\n%s", err, m.w.report())
	}
	first, _, _ := strings.Cut(strings.TrimSpace(string(data)), "\n")
	rel, in := (&coverageState{w: m.w}).rel(strings.TrimSpace(first))
	parts := strings.Split(rel, "/")
	if !in || len(parts) < 4 || parts[0] != ".metrics" || parts[1] != "coverage" || !strings.HasPrefix(parts[2], "run-") {
		return "", "", fmt.Errorf("GOCOVERDIR is %s, not a directory under a run-* directory of .metrics/coverage", first)
	}
	return rel, path.Join(parts[:3]...), nil
}

// coverDirUnderRun is whether the binary's GOCOVERDIR was under the run's
// own run-* directory in .metrics/coverage/.
func (m *mutateState) coverDirUnderRun() error {
	_, _, err := m.coverDir()
	return err
}

// coverDirGone is whether neither GOCOVERDIR nor its run-* directory is
// left after the run.
func (m *mutateState) coverDirGone() error {
	dir, run, err := m.coverDir()
	if err != nil {
		return err
	}
	for _, left := range []string{dir, run} {
		if m.w.d.Exists(filepath.Join(m.w.dir, filepath.FromSlash(left))) {
			return fmt.Errorf("%s is left after the run\n%s", left, m.w.report())
		}
	}
	return nil
}
