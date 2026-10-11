package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The scenarios of "Rule: With --since, --fail-uncovered=lines judges
// coverage only on the changed lines" in features/mutate.feature that the
// strict-changed-lines slice holds, but for the counted run of @ID-MUT-226,
// which builds only where counted mode runs
// (strict_changed_lines_counted_test.go). They drive the CLI and read its
// --json object as raw maps, so they compile against a product without the
// feature.
//
// Each example's project is committed at a base, tagged "base", whose
// function has a branch no test takes: its body, one line, never runs, and
// its one mutation site, 0 → 1, is on it, so that mutant is uncovered. The
// change since the base changes the function's first line, which the test
// runs, and the test with it; the further change adds a branch no test
// takes either, before the old one.
type changedLinesExample struct {
	language string
	file     string
	files    map[string]string // the project at the base
	// changed and changedTest change the function's first line, and what
	// the test expects of it, as the change since the base does.
	changed, changedTest [2]string
	// added adds the new branch, as [old, new]. Its first line is unlike
	// the old branch's, so git diff cannot slide its hunk onto the old one.
	added [2]string
	// oldLine is where plain --fail-uncovered reports the old branch's
	// line that never runs, once the change is committed: Go's block starts
	// at its first statement, so either language reports that line.
	oldLine int
	// addedLine is where the added branch's line that never runs is
	// reported: one of the lines the further change added.
	addedLine int
}

var changedLinesExamples = []changedLinesExample{
	{
		language: "go",
		file:     "value.go",
		files: map[string]string{
			"go.mod": "module example.com/lines\n\ngo 1.22\n",
			"value.go": "package lines\n\n// Value is the default, or zero when flag is set.\n" +
				"func Value(flag bool) int {\n\tn := 1\n\tif flag {\n\t\tn = 0\n\t}\n\treturn n\n}\n",
			"value_test.go": "package lines\n\nimport \"testing\"\n\n" +
				"func TestValue(t *testing.T) {\n\tif Value(false) != 1 {\n\t\tt.Fatal(\"Value(false) is not the default\")\n\t}\n}\n",
		},
		changed:     [2]string{"\tn := 1\n", "\tn := 2\n"},
		changedTest: [2]string{"Value(false) != 1", "Value(false) != 2"},
		added:       [2]string{"\tn := 2\n", "\tn := 2\n\tif flag { // never taken either\n\t\tn = 5\n\t}\n"},
		oldLine:     7,
		addedLine:   7,
	},
	{
		language: "python",
		file:     "calc.py",
		files: map[string]string{
			"pyproject.toml":     "[project]\nname = \"m\"\nversion = \"0.0.0\"\n",
			"calc.py":            "def value(flag):\n    n = 1\n    if flag:\n        n = 0\n    return n\n",
			"tests/test_calc.py": "from calc import value\n\n\ndef test_value():\n    assert value(False) == 1\n",
		},
		changed:     [2]string{"    n = 1\n", "    n = 2\n"},
		changedTest: [2]string{"value(False) == 1", "value(False) == 2"},
		added:       [2]string{"    n = 2\n", "    n = 2\n    if flag:  # never taken either\n        n = 5\n"},
		oldLine:     4,
		addedLine:   4,
	},
}

// changedLinesRules are the rules --fail-uncovered=lines narrows to the
// changed lines: strict coverage's, and the uncovered mutant's.
var changedLinesRules = []string{"mutation.uncovered-statement", "mutation.coverage-missing", "mutation.coverage-stale", "mutation.uncovered"}

// changedLinesFindings is each problem of a --json object, decoded as raw
// maps, of a rule of changedLinesRules, as "rule file:line", sorted, with
// file in slash form.
func changedLinesFindings(problems []map[string]any) []string {
	var out []string
	for _, p := range problems {
		rule, _ := p["rule"].(string)
		if !slices.Contains(changedLinesRules, rule) {
			continue
		}
		file, _ := p["file"].(string)
		line, _ := p["line"].(float64)
		out = append(out, fmt.Sprintf("%s %s:%d", rule, filepath.ToSlash(file), int(line)))
	}
	slices.Sort(out)
	return out
}

// rawProblems is the "problems" of o's --json object.
func rawProblems(t *testing.T, o outcome) []map[string]any {
	t.Helper()
	var out struct {
		Problems []map[string]any `json:"problems"`
	}
	if err := json.Unmarshal([]byte(o.stdout), &out); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s\nstderr:\n%s", err, o.stdout, o.stderr)
	}
	return out.Problems
}

// changedLinesRepo writes files in a git repository whose first commit is
// tagged "base", and makes it the test's directory (useDir).
func changedLinesRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	for name, text := range files {
		writeFile(t, filepath.Join(dir, filepath.FromSlash(name)), text)
	}
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-qm", "base")
	gitIn(t, dir, "tag", "base")
	useDir(t, dir)
	return dir
}

// testFile is the example's test file.
func (e changedLinesExample) testFile() string {
	for name := range e.files {
		if strings.Contains(name, "test") {
			return filepath.FromSlash(name)
		}
	}
	return ""
}

// @ID-MUT-225
func TestAGoChangeIsJudgedOnTheLinesItChangedNotTheOldCodeAroundThem(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not installed")
	}
	changedLinesScenario(t, changedLinesExamples[0])
}

// @ID-MUT-225
func TestAPythonChangeIsJudgedOnTheLinesItChangedNotTheOldCodeAroundThem(t *testing.T) {
	t.Parallel()
	languageTools(t, "python")
	// pytest plugins installed beside pytest are no part of the scenario.
	useEnv(t, "PYTEST_DISABLE_PLUGIN_AUTOLOAD", "1")
	changedLinesScenario(t, changedLinesExamples[1])
}

func changedLinesScenario(t *testing.T, e changedLinesExample) {
	t.Helper()
	// Given a <language> project whose function has an executable line no
	// test runs, committed at a base
	changedLinesRepo(t, e.files)
	// And a commit since the base that changes another line of that
	// function, which a test runs
	edit(t, e.file, e.changed[0], e.changed[1])
	edit(t, e.testFile(), e.changedTest[0], e.changedTest[1])
	commitAll(t, "change the default")

	// When I run "itos-cc mutation run --since <base> --fail-uncovered=lines
	// --json" for its file
	lines := mutateCovered(t, "--since", "base", "--fail-uncovered=lines", "--json", e.file)
	logRun(t, &lines)
	// Then the old unexecuted line is not reported and the run passes
	if got := changedLinesFindings(rawProblems(t, lines)); len(got) != 0 || lines.code != 0 {
		t.Errorf("the changed-line run: exit %d, findings %q; want 0 and none: the old line is not one the range changed", lines.code, got)
	}

	// And "itos-cc mutation check --since <base> --fail-uncovered=lines"
	// gives the same verdict
	check := mutationCheck(t, "--since", "base", "--fail-uncovered=lines", "--json", e.file)
	logRun(t, &check)
	if got := changedLinesFindings(rawProblems(t, check)); len(got) != 0 || check.code != 0 {
		t.Errorf("the changed-line check: exit %d, findings %q; want 0 and none, as the run\n%s", check.code, got, check.stdout)
	}

	// But "itos-cc mutation run --since <base> --fail-uncovered" still
	// reports the old line as "mutation.uncovered-statement"
	plain := mutateCovered(t, "--since", "base", "--fail-uncovered", "--json", e.file)
	logRun(t, &plain)
	old := fmt.Sprintf("mutation.uncovered-statement %s:%d", e.file, e.oldLine)
	if got := changedLinesFindings(rawProblems(t, plain)); !slices.Contains(got, old) || plain.code != 1 {
		t.Errorf("the whole-function run: exit %d, findings %q; want 1 with %q", plain.code, got, old)
	}

	// And a commit that adds a line no test runs is reported on that line,
	// and the run fails
	edit(t, e.file, e.added[0], e.added[1])
	commitAll(t, "add a branch no test takes")
	added := mutateCovered(t, "--since", "base", "--fail-uncovered=lines", "--json", e.file)
	logRun(t, &added)
	want := []string{fmt.Sprintf("mutation.uncovered-statement %s:%d", e.file, e.addedLine)}
	if got := changedLinesFindings(rawProblems(t, added)); !slices.Equal(got, want) || added.code != 1 {
		t.Errorf("the changed-line run after the added line: exit %d, findings %q; want 1 with %q alone", added.code, got, want)
	}
}

// @ID-MUT-226
func TestChangedLineStrictnessNeedsARange(t *testing.T) {
	t.Parallel()
	inEmptyDir(t)
	// When I run "itos-cc mutation run --fail-uncovered=lines" without
	// --since
	// Then it fails as a usage error, exit 2, naming --since
	for _, args := range [][]string{
		{"mutation", "run", "--fail-uncovered=lines", "--json"},
		{"mutation", "run", "--count", "1", "--fail-uncovered=lines", "--json"},
		{"mutation", "check", "--fail-uncovered=lines", "--json"},
	} {
		o := cli(t, args...)
		named := false
		for _, p := range rawProblems(t, o) {
			message, _ := p["message"].(string)
			fix, _ := p["fix"].(string)
			named = named || strings.Contains(message+" "+fix, "--since")
		}
		if o.code != 2 {
			t.Errorf("%s: exit %d, want 2, a usage error\n%s", strings.Join(args, " "), o.code, o.stdout)
		}
		if !named {
			t.Errorf("%s: problems %v, want one naming --since", strings.Join(args, " "), rawProblems(t, o))
		}
	}
}
