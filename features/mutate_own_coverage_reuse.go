package features

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

// The steps of the Rule "A file's own tests' coverage is reused while
// nothing it was measured from changed" (slice-70, issue #67). They reuse
// slice-69's runs of a Given ("itos-cc …" has run), the named-survivor Go
// module, ID-MUT-105's TypeScript project and ID-MUT-117's module, and read
// only what itos-cc prints: the coverage commands its stderr says it runs
// and its --json objects.

func ownCoverageReuseSteps(sc *godog.ScenarioContext, m *mutateState) {
	sc.Step(`^a TypeScript project whose tests import src/board\.ts \(ID-MUT-105's\)$`, func() error { return m.boardProject() })
	sc.Step(`^the module of ID-MUT-117, whose test runs the built binary$`, func() error { return m.greetModule(true, false) })
	sc.Step(`^no coverage command ran$`, m.noCoverageCommand)
	sc.Step(`^every mutant's outcome and "coverage" are those of the first run$`, func() error { return m.sameAsMeasured(false) })
	sc.Step(`^each covered mutant's "coverage" is as the first run's, "integration" included$`, func() error { return m.sameAsMeasured(true) })
	sc.Step(`^once (board/board_test\.go|the test importing it) changed, the coverage command runs again$`, m.measuresAgain)
}

// boardProject is ID-MUT-105's TypeScript project, whose tests also run
// for src/board.ts, the file they import.
func (m *mutateState) boardProject() error {
	if err := m.unusedProject("src/unused.ts"); err != nil {
		return err
	}
	m.file = "src/board.ts"
	return m.w.d.Tests(m.w.dir, Tests{Language: "typescript", Command: vitestCommand + m.file})
}

// coverageCommandLines is each line of stderr saying a coverage command runs:
// "itos-cc: coverage <dir>$ <command>".
func coverageCommandLines(stderr string) []string {
	var out []string
	for _, line := range outputLines(strings.ReplaceAll(stderr, "\r\n", "\n")) {
		if rest, ok := strings.CutPrefix(line, "itos-cc: coverage "); ok && strings.Contains(rest, "$ ") {
			out = append(out, line)
		}
	}
	return out
}

func (m *mutateState) noCoverageCommand() error {
	if ran := coverageCommandLines(m.w.stderr); len(ran) > 0 {
		return fmt.Errorf("a coverage command ran: %q\n%s", ran, m.w.report())
	}
	return nil
}

// mutantsJudged is each mutant of a --json object's files, by its file and
// site, with its outcome and its "coverage".
func mutantsJudged(stdout string) (map[string]string, error) {
	var object struct {
		Files []struct {
			File    string           `json:"file"`
			Mutants []map[string]any `json:"mutants"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(stdout), &object); err != nil {
		return nil, fmt.Errorf("stdout is no --json object: %v", err)
	}
	out := map[string]string{}
	for _, f := range object.Files {
		for _, mu := range f.Mutants {
			site := fmt.Sprintf("%s:%v:%v %v → %v", f.File, mu["line"], mu["column"], mu["original"], mu["replacement"])
			out[site] = fmt.Sprintf("%v %v", mu["outcome"], mu["coverage"])
		}
	}
	return out, nil
}

// sameAsMeasured is whether the last run's mutants have the outcome and
// "coverage" of the same run with its coverage measured afresh, its cache
// bypassed: what the first run measured of the unchanged tree. covered
// compares the covered mutants' "coverage" alone, and wants one of them
// covered by "integration".
func (m *mutateState) sameAsMeasured(covered bool) error {
	w := m.w
	if w.last == nil {
		return fmt.Errorf("no run of itos-cc to compare")
	}
	reused, err := mutantsJudged(w.stdout)
	if err != nil {
		return fmt.Errorf("%v\n%s", err, w.report())
	}
	args := append(slices.Clone(w.last.args), "--no-coverage-cache")
	code, out, errOut, err := w.capture(w.last.dir, w.last.env, args...)
	if err != nil {
		return err
	}
	report := fmt.Sprintf("%s\n--- measured afresh, exit %d\n--- stdout\n%s--- stderr\n%s", w.report(), code, out, errOut)
	measured, err := mutantsJudged(out)
	if err != nil {
		return fmt.Errorf("%v\n%s", err, report)
	}
	if covered {
		keep := func(judged map[string]string) {
			maps.DeleteFunc(judged, func(_, v string) bool { return strings.HasPrefix(v, "uncovered ") })
			for site, v := range judged {
				_, judged[site], _ = strings.Cut(v, " ")
			}
		}
		keep(reused)
		keep(measured)
	}
	if len(measured) == 0 {
		return fmt.Errorf("the run measured afresh judged no mutant\n%s", report)
	}
	if !maps.Equal(reused, measured) {
		return fmt.Errorf("the mutants are %v, not those of a fresh measurement, %v\n%s", reused, measured, report)
	}
	if !slices.ContainsFunc(slices.Collect(maps.Values(measured)), func(v string) bool {
		return strings.Contains(v, "in-process") || strings.Contains(v, "integration")
	}) {
		return fmt.Errorf("no mutant has \"coverage\"\n%s", report)
	}
	if covered && !slices.ContainsFunc(slices.Collect(maps.Values(reused)), func(v string) bool { return strings.Contains(v, "integration") }) {
		return fmt.Errorf("no covered mutant's \"coverage\" names \"integration\"\n%s", report)
	}
	return nil
}

// measuresAgain changes the test of the file the last run mutated, runs
// it again, and is whether a coverage command ran.
func (m *mutateState) measuresAgain(test string) error {
	w := m.w
	if test == "the test importing it" {
		test = "src/board.test.ts"
	}
	if err := w.write(test, m.read(test)+"\n// changed\n"); err != nil {
		return err
	}
	if err := w.runEnv(w.last.dir, w.last.env, w.last.args...); err != nil {
		return err
	}
	if len(coverageCommandLines(w.stderr)) == 0 {
		return fmt.Errorf("no coverage command ran once %s changed\n%s", test, w.report())
	}
	return nil
}
