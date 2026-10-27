package features

import (
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// A mutation run says on stderr when each long phase ends, and how long it
// took (slice-64): the listed tests' coverage run and its conversions, a
// file's own-tests baseline, and a counted mutant's listed tests. Its
// steps read stderr's lines in their order; a duration is checked by its
// shape alone, never its value.

// phaseProgress is the state of those steps: where in stderr the last
// line a step found stands, so the next can ask what comes after it.
type phaseProgress struct {
	m *mutateState
	// at is the index, among stderr's lines, of the last line a step
	// found; found is every line the listed-coverage steps found.
	at    int
	found []int
}

func phaseProgressSteps(sc *godog.ScenarioContext, m *mutateState) {
	p := &phaseProgress{m: m}
	sc.Step(`^a Go file whose tests pass, with a mutant they kill$`, p.passingGoFile)
	sc.Step(`^stderr has a line "([^"]*)" after the listed tests' coverage command$`, p.afterListedCoverage)
	sc.Step(`^then a line "([^"]*)"$`, p.thenLine)
	sc.Step(`^both come before the first line about a mutant$`, p.beforeMutants)
	sc.Step(`^stderr has, after the line "itos-cc: baseline <dir>\$ go test …", a line "([^"]*)"$`, p.afterOwnBaseline)
	sc.Step(`^stderr has, in order, the listed tests' coverage command, "([^"]*)", "([^"]*)", the own baseline's command and "([^"]*)"$`, p.countedOrder)
	sc.Step(`^its mutant's line ends "([^"]*)"$`, p.mutantLineEnds)
	sc.Step(`^stdout is the plain report, as a run before mutation-phase-progress gives it$`, p.plainUnchanged)
	sc.Step(`^stdout is the one JSON object, its keys unchanged, as a run before mutation-phase-progress gives it$`, p.jsonUnchanged)
}

// durationShape is what <duration> stands for: digits, a point, a digit
// and "s", as "%.1fs" prints a duration.
const durationShape = `[0-9]+\.[0-9]s`

// linePattern is text as a whole line, each <duration> in it standing for
// a duration's shape and each <dir> for dir.
func linePattern(text, dir string) *regexp.Regexp {
	quoted := regexp.QuoteMeta(text)
	quoted = strings.ReplaceAll(quoted, "<duration>", durationShape)
	quoted = strings.ReplaceAll(quoted, "<dir>", regexp.QuoteMeta(dir))
	return regexp.MustCompile("^" + quoted + "$")
}

// lines is stderr's lines.
func (p *phaseProgress) lines() []string {
	return outputLines(strings.ReplaceAll(p.m.w.stderr, "\r\n", "\n"))
}

// next is the index of the first line after from that pattern matches,
// -1 when none does.
func (p *phaseProgress) next(from int, pattern *regexp.Regexp) int {
	lines := p.lines()
	for i := from + 1; i < len(lines); i++ {
		if pattern.MatchString(lines[i]) {
			return i
		}
	}
	return -1
}

// listedRuns are the indexes of the lines announcing a run of the listed
// tests for coverage: the run of every test, then, where the harness does
// not split it, each test's run alone.
func (p *phaseProgress) listedRuns() []int {
	var out []int
	for i, line := range p.lines() {
		if strings.HasPrefix(line, "itos-cc: coverage ") && strings.Contains(line, "$ ") && strings.Contains(line, "-tests=") {
			out = append(out, i)
		}
	}
	return out
}

func (p *phaseProgress) afterListedCoverage(text string) error {
	runs := p.listedRuns()
	if len(runs) == 0 {
		return fmt.Errorf("stderr announces no coverage run of the listed tests\n%s", p.m.w.report())
	}
	at := p.next(runs[len(runs)-1], linePattern(text, ""))
	if at < 0 {
		return fmt.Errorf("no line %q after the listed tests' coverage command\n%s", text, p.m.w.report())
	}
	p.at, p.found = at, append(p.found, at)
	return nil
}

func (p *phaseProgress) thenLine(text string) error {
	at := p.next(p.at, linePattern(text, ""))
	if at < 0 {
		return fmt.Errorf("no line %q after line %d\n%s", text, p.at+1, p.m.w.report())
	}
	p.at, p.found = at, append(p.found, at)
	return nil
}

// mutantLine is a line about a mutant a mutation run ran.
var mutantLine = regexp.MustCompile(`^itos-cc: \[\d+/\d+\] `)

func (p *phaseProgress) beforeMutants() error {
	first := p.next(-1, mutantLine)
	if first < 0 || len(p.found) != 2 {
		return fmt.Errorf("stderr has no line about a mutant, or the lines before were not found\n%s", p.m.w.report())
	}
	for _, at := range p.found {
		if at > first {
			return fmt.Errorf("line %d comes after the first line about a mutant, line %d\n%s", at+1, first+1, p.m.w.report())
		}
	}
	return nil
}

// passingGoFile writes a Go module whose a/a.go its own tests, a/a_test.go,
// cover and pass, killing every mutant they run.
func (p *phaseProgress) passingGoFile() error {
	m := p.m
	m.file = "a/a.go"
	if err := m.writeAll(map[string]string{
		"go.mod":      mutateModule,
		"a/a.go":      "package a\n\n// Half is whether i is past two.\nfunc Half(i int) bool {\n\treturn i > 2\n}\n",
		"a/a_test.go": "package a\n\nimport \"testing\"\n\nfunc TestHalf(t *testing.T) {\n\tif Half(2) || !Half(3) {\n\t\tt.Fatal(\"Half\")\n\t}\n}\n",
	}); err != nil {
		return err
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "go", Report: "SF:a/a.go\nDA:5,1\nend_of_record\n"}); err != nil {
		return err
	}
	return m.w.d.Tests(m.w.dir, Tests{Language: "go", Command: "go test -count=1 -failfast ./a", Elapsed: 100 * time.Millisecond})
}

// ownBaseline is the index of the first line after from announcing a
// baseline of the own tests, a go test that selects no listed test, and
// the folder it names.
func (p *phaseProgress) ownBaseline(from int) (int, string) {
	for i, line := range p.lines() {
		if i <= from {
			continue
		}
		if match := baselineLine.FindStringSubmatch(line); match != nil && strings.HasPrefix(match[2], "go test") && !strings.Contains(match[2], "-tests=") {
			return i, match[1]
		}
	}
	return -1, ""
}

func (p *phaseProgress) afterOwnBaseline(text string) error {
	at, dir := p.ownBaseline(-1)
	if at < 0 {
		return fmt.Errorf("stderr announces no baseline running go test\n%s", p.m.w.report())
	}
	if p.next(at, linePattern(text, dir)) < 0 {
		return fmt.Errorf("no line %q, <dir> %s, after the baseline's line %d\n%s", text, dir, at+1, p.m.w.report())
	}
	return nil
}

func (p *phaseProgress) countedOrder(ran, converted, passed string) error {
	runs := p.listedRuns()
	if len(runs) == 0 {
		return fmt.Errorf("stderr announces no coverage run of the listed tests\n%s", p.m.w.report())
	}
	at := runs[0]
	for _, text := range []string{ran, converted} {
		if at = p.next(at, linePattern(text, "")); at < 0 {
			return fmt.Errorf("no line %q after the listed tests' coverage command and the lines before it\n%s", text, p.m.w.report())
		}
	}
	at, dir := p.ownBaseline(at)
	if at < 0 {
		return fmt.Errorf("stderr announces no own baseline after the listed coverage's lines\n%s", p.m.w.report())
	}
	if p.next(at, linePattern(passed, dir)) < 0 {
		return fmt.Errorf("no line %q, <dir> %s, after the own baseline's line %d\n%s", passed, dir, at+1, p.m.w.report())
	}
	return nil
}

func (p *phaseProgress) mutantLineEnds(text string) error {
	var said []string
	for _, line := range p.lines() {
		if mutantLine.MatchString(line) {
			said = append(said, line)
		}
	}
	quoted := strings.ReplaceAll(regexp.QuoteMeta(text), "<duration>", durationShape)
	if len(said) != 1 || !regexp.MustCompile(" "+quoted+"$").MatchString(said[0]) {
		return fmt.Errorf("the lines about a mutant are %q, not one ending %q\n%s", said, text, p.m.w.report())
	}
	return nil
}

// countedPlainLines are the lines a counted run's plain report printed on
// stdout for the counted listed fixture before slice-64, each value that
// changes from run to run, a seed, a commit, a path or a duration, held by
// the shape of the words around it.
var countedPlainLines = []*regexp.Regexp{
	regexp.MustCompile(`^counted mutation run at [0-9a-f]{40}: seed slice-64-2, algorithm fresh-plan-v2$`),
	regexp.MustCompile(`^budget 1: 1 of 4 eligible sites selected, 1 executed, 3 omitted$`),
	regexp.MustCompile("^  killed `>` → `>=` in example\\.com/ownership#Compare, subject\\.go line 3 column 37 by listed tests ID-A-01 ID-A-02$"),
	regexp.MustCompile(`^sampled assurance: only the judgments above are proven; this is not a complete mutation result, and no snapshot, annotation or coverage cache was written$`),
	regexp.MustCompile(`^--count bounds mutant trials, not discovery, coverage, listing, baseline commands or total latency$`),
}

func (p *phaseProgress) plainUnchanged() error {
	lines := outputLines(strings.ReplaceAll(p.m.w.stdout, "\r\n", "\n"))
	same := len(lines) == len(countedPlainLines)
	for i := 0; same && i < len(lines); i++ {
		same = countedPlainLines[i].MatchString(lines[i])
	}
	if !same {
		return fmt.Errorf("stdout is not the plain report a counted run gave before\n%s", p.m.w.report())
	}
	return nil
}

// countedJSONKeys are the keys, each by its path, of the --json object a
// counted run gave for the counted listed fixture before slice-64.
var countedJSONKeys = []string{
	".ok", ".sampling", ".sampling.algorithm", ".sampling.assurance",
	".sampling.bounds", ".sampling.budget", ".sampling.commit", ".sampling.completion",
	".sampling.eligible", ".sampling.executed", ".sampling.omitted", ".sampling.seed",
	".sampling.selected", ".sampling.uncommitted", ".schema", ".selected",
	".selected[].column", ".selected[].file", ".selected[].function", ".selected[].identity",
	".selected[].line", ".selected[].original", ".selected[].outcome", ".selected[].replacement",
	".selected[].scope", ".selected[].stages", ".selected[].stages[].name", ".selected[].stages[].outcome",
	".selected[].stages[].state", ".selected[].stages[].tests", ".selected[].state", ".selected[].tests",
	".stages", ".stages[].name", ".stages[].state", ".subjects",
	".subjects.judged", ".subjects.judged[].file", ".subjects.judged[].function", ".subjects.omitted",
	".subjects.omitted[].file", ".subjects.omitted[].function",
}

func (p *phaseProgress) jsonUnchanged() error {
	var object map[string]any
	if err := json.Unmarshal([]byte(p.m.w.stdout), &object); err != nil {
		return fmt.Errorf("stdout is not one JSON object: %w\n%s", err, p.m.w.report())
	}
	keys := map[string]bool{}
	jsonKeys(object, "", keys)
	got := slices.Sorted(maps.Keys(keys))
	if !slices.Equal(got, countedJSONKeys) {
		return fmt.Errorf("the --json object's keys are %q, not %q\n%s", got, countedJSONKeys, p.m.w.report())
	}
	return nil
}

// jsonKeys adds the path of every key of value, under path, to keys, an
// array's items sharing one path, "[]".
func jsonKeys(value any, path string, keys map[string]bool) {
	switch v := value.(type) {
	case map[string]any:
		for k, item := range v {
			keys[path+"."+k] = true
			jsonKeys(item, path+"."+k, keys)
		}
	case []any:
		for _, item := range v {
			jsonKeys(item, path+"[]", keys)
		}
	}
}
