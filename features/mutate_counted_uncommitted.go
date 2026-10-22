package features

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

// The steps of ID-MUT-275 (slice-26, issue #41): a counted run judges HEAD's
// committed inputs, and warns of the uncommitted changes to them it leaves
// unjudged. They read the --json object raw, so they compile against a run
// that has no "uncommitted" yet.

// uncommittedPaths are the in-scope paths the fixture leaves uncommitted,
// sorted: the untracked test file, the source with a staged change and the
// test with an unstaged one.
var uncommittedPaths = []string{"extra_test.go", "main.go", "main_test.go"}

func (q *countedMutation) uncommittedSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a committed project whose test has an unstaged change and whose source has a staged change, beside an untracked test file and an unstaged change to a README that is neither source, test nor setting$`, q.uncommittedProject)
	sc.Step(`^stderr warns that the run judged HEAD's commit and that the test, the source and the untracked test file go unjudged, naming each$`, q.warnsUnjudged)
	sc.Step(`^the JSON's sampling object lists those three paths as "uncommitted", and not the README$`, q.listsUncommitted)
	sc.Step(`^the judgment and the exit code are those of the same run with a clean working tree$`, q.judgedAsClean)
	sc.Step(`^with no uncommitted change in scope, nothing is warned and "uncommitted" is an empty list$`, q.cleanUnwarned)
}

// uncommittedProject commits the counted fixture with a README, keeps the
// committed text of each file it then changes, and changes them: the test
// unstaged, the source staged, a new untracked test file, and the README
// unstaged, which is no input the run freezes.
func (q *countedMutation) uncommittedProject(ctx context.Context) error {
	if err := q.project(ownedOneSource); err != nil {
		return err
	}
	w := q.e.m.w
	if err := w.write("README.md", "# fixture\n"); err != nil {
		return err
	}
	if err := w.d.Git(w.dir, "add", "README.md"); err != nil {
		return err
	}
	if err := w.d.Git(w.dir, "commit", "-qm", "readme"); err != nil {
		return err
	}
	var err error
	if q.head, err = w.d.Head(ctx, w.dir); err != nil {
		return err
	}
	q.committed = map[string]string{}
	for _, file := range []string{"main.go", "main_test.go"} {
		data, err := w.d.Read(filepath.Join(w.dir, file))
		if err != nil {
			return err
		}
		q.committed[file] = string(data)
	}
	if err := w.write("main.go", ownedOneSource+"\nfunc Staged(n int) bool { return n < 0 }\n"); err != nil {
		return err
	}
	if err := w.d.Git(w.dir, "add", "main.go"); err != nil {
		return err
	}
	if err := w.write("main_test.go", q.committed["main_test.go"]+"\n// an unstaged change to the test\n"); err != nil {
		return err
	}
	if err := w.write("extra_test.go", "package main\n\nimport \"testing\"\n\nfunc TestExtra(t *testing.T) { if Compare(5) { t.Fatal(\"Compare(5)\") } }\n"); err != nil {
		return err
	}
	return w.write("README.md", "# fixture\n\nAn unstaged change to the README.\n")
}

// unjudgedLines are the lines of stderr that speak of what goes unjudged.
func unjudgedLines(stderr string) []string {
	var out []string
	for _, line := range strings.Split(stderr, "\n") {
		if strings.Contains(line, "unjudged") {
			out = append(out, line)
		}
	}
	return out
}

// namesPath reports whether one of lines names path as a word of its own.
func namesPath(lines []string, path string) bool {
	for _, line := range lines {
		for _, word := range strings.Fields(line) {
			if strings.Trim(word, `,;:.()"'`) == path {
				return true
			}
		}
	}
	return false
}

func (q *countedMutation) warnsUnjudged() error {
	w := q.e.m.w
	lines := unjudgedLines(w.stderr)
	if !slices.ContainsFunc(lines, func(line string) bool { return strings.Contains(line, q.head) }) {
		return fmt.Errorf("stderr warns of nothing unjudged at commit %s\n%s", q.head, w.report())
	}
	for _, path := range uncommittedPaths {
		if !namesPath(lines, path) {
			return fmt.Errorf("the warning does not name %s\n%s", path, w.report())
		}
	}
	if namesPath(lines, "README.md") {
		return fmt.Errorf("the warning names README.md, which is no input of the run\n%s", w.report())
	}
	return nil
}

// uncommittedOf is the sampling object's "uncommitted" of object, and
// whether it is a list of strings.
func uncommittedOf(object map[string]any) ([]string, bool) {
	list, ok := executionMap(object, "sampling")["uncommitted"].([]any)
	if !ok {
		return nil, false
	}
	out := []string{}
	for _, item := range list {
		s, ok := item.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

func (q *countedMutation) listsUncommitted() error {
	w := q.e.m.w
	object, err := w.json()
	if err != nil {
		return err
	}
	got, ok := uncommittedOf(object)
	if !ok || !slices.Equal(got, uncommittedPaths) {
		return fmt.Errorf("sampling's uncommitted is %v, want %v\n%s", executionMap(object, "sampling")["uncommitted"], uncommittedPaths, w.report())
	}
	return nil
}

// judgedAsClean undoes every in-scope change, the README's left as it is,
// runs the same command again, and compares the two runs, but for
// "uncommitted".
func (q *countedMutation) judgedAsClean() error {
	w := q.e.m.w
	dirty, err := w.json()
	if err != nil {
		return err
	}
	dirtyExit := w.exit
	for _, file := range []string{"main.go", "main_test.go"} {
		if err := w.write(file, q.committed[file]); err != nil {
			return err
		}
	}
	if err := w.d.Git(w.dir, "add", "main.go"); err != nil {
		return err
	}
	if err := w.d.Remove(filepath.Join(w.dir, "extra_test.go")); err != nil {
		return err
	}
	clean, err := q.observe("mutation", "run", "--count", "1", "--json")
	if err != nil {
		return err
	}
	q.saved["clean"] = clean
	if clean.result.Exit != dirtyExit {
		return fmt.Errorf("exit %d with uncommitted changes, %d without", dirtyExit, clean.result.Exit)
	}
	a, b := withoutUncommitted(dirty), withoutUncommitted(clean.object)
	if !reflect.DeepEqual(a, b) {
		return fmt.Errorf("the judgment differs with uncommitted changes:\n%v\nwithout:\n%v", a, b)
	}
	if selected := objectList(b, "selected"); len(selected) != 1 || selected[0]["outcome"] != "killed" {
		return fmt.Errorf("the clean run judged no committed site killed: %v", b)
	}
	return nil
}

// withoutUncommitted is object less its sampling's "uncommitted".
func withoutUncommitted(object map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range object {
		out[k] = v
	}
	sampling := map[string]any{}
	for k, v := range executionMap(object, "sampling") {
		if k != "uncommitted" {
			sampling[k] = v
		}
	}
	out["sampling"] = sampling
	return out
}

func (q *countedMutation) cleanUnwarned() error {
	clean := q.saved["clean"]
	if lines := unjudgedLines(clean.result.Stderr); len(lines) > 0 || strings.Contains(clean.result.Stderr, "uncommitted") {
		return fmt.Errorf("a run with no uncommitted change in scope warns:\n%s", clean.result.Stderr)
	}
	got, ok := uncommittedOf(clean.object)
	if !ok || len(got) != 0 {
		return fmt.Errorf("sampling's uncommitted is %v with no uncommitted change in scope, want []", executionMap(clean.object, "sampling")["uncommitted"])
	}
	return nil
}
