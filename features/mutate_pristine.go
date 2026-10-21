package features

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

// Each run in a worker starts from the copy as it was made (slice-13,
// issue #35): a file one test run writes, rewrites or removes in its
// worker's copy is back as copied before the next run there, so it never
// fails every later mutant of the worker.

// pristineState is what a slice-13 scenario's Givens wrote: the test data
// the project commits, by its path, slash-separated from the scratch
// folder.
type pristineState struct {
	m         *mutateState
	committed map[string]string
}

func pristineSteps(sc *godog.ScenarioContext, m *mutateState) {
	p := &pristineState{m: m}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*p = pristineState{m: m}
		return ctx, nil
	})
	sc.Step(`^testdata/ is ignored by git and holds golden\.txt, "([^"]*)", and keep\.txt, both committed$`, p.pristineTestdata)
	sc.Step(`^its test, which checks no result, fails when (.+), and then (.+)$`, p.pristineTest)
	sc.Step(`^(\S+)'s baseline passed and every mutant of (\S+) survived$`, p.pristineAllSurvived)
	sc.Step(`^the project's own testdata folder is as committed$`, p.pristineProjectUnchanged)
}

// pristineTestdata writes testdata/golden.txt holding golden and
// testdata/keep.txt, and a .gitignore that ignores testdata/, then commits
// the scratch folder, testdata/ forced in, as a project commits its
// fixtures under an ignored folder.
func (p *pristineState) pristineTestdata(golden string) error {
	p.committed = map[string]string{"testdata/golden.txt": golden + "\n", "testdata/keep.txt": "kept\n"}
	if err := p.m.writeAll(p.committed); err != nil {
		return err
	}
	if err := p.m.w.write(".gitignore", "testdata/\n"); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"}, {"add", "-A"}, {"add", "-f", "testdata"}, {"commit", "-qm", "leak"},
	} {
		if err := p.m.w.git(args...); err != nil {
			return err
		}
	}
	return nil
}

// pristineFound is the Go statement that fails a test when what a
// scenario's row says it finds is so, run from the package's folder.
var pristineFound = map[string]string{
	"testdata/left-behind is there": `if _, err := os.Stat("testdata/left-behind"); err == nil {
		t.Fatal("testdata/left-behind is there")
	}`,
	"testdata/golden.txt is not committed": `if data, err := os.ReadFile("testdata/golden.txt"); err != nil || string(data) != %q {
		t.Fatalf("testdata/golden.txt is not committed: %%q, %%v", data, err)
	}`,
	"testdata/keep.txt is gone": `if _, err := os.Stat("testdata/keep.txt"); err != nil {
		t.Fatalf("testdata/keep.txt is gone: %v", err)
	}`,
}

// pristineChange is the Go statement that makes the change a scenario's
// row names, failing the test when it cannot.
var pristineChange = map[string]string{
	"writes testdata/left-behind":  `err := os.WriteFile("testdata/left-behind", []byte("left behind\n"), 0o644)`,
	"rewrites testdata/golden.txt": `err := os.WriteFile("testdata/golden.txt", []byte("rewritten\n"), 0o644)`,
	"removes testdata/keep.txt":    `err := os.Remove("testdata/keep.txt")`,
}

// pristineTest writes the test of the Given's file, beside it: it calls
// each function and checks none of their results, fails when found is so,
// then makes change. It declares that Go runs it, as mutation run's own
// command does.
func (p *pristineState) pristineTest(found, change string) error {
	check, ok := pristineFound[found]
	if !ok {
		return fmt.Errorf("no test checks that %s", found)
	}
	if strings.Contains(check, "%q") {
		check = fmt.Sprintf(check, p.committed["testdata/golden.txt"])
	}
	makeChange, ok := pristineChange[change]
	if !ok {
		return fmt.Errorf("no test %s", change)
	}
	pkg, ok := emptyCasePackage(p.m.read(p.m.file))
	if !ok {
		return fmt.Errorf("%s declares no package", p.m.file)
	}
	test := strings.TrimSuffix(p.m.file, path.Ext(p.m.file)) + "_test.go"
	text := fmt.Sprintf("package %s\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\n"+
		"func TestLeaves(t *testing.T) {\n\t_, _, _ = A(3), B(3), C(3)\n\t%s\n\t%s\n\tif err != nil {\n\t\tt.Fatal(err)\n\t}\n}\n",
		pkg, check, makeChange)
	if err := p.m.w.write(test, text); err != nil {
		return err
	}
	if err := p.m.w.git("add", test); err != nil {
		return err
	}
	if err := p.m.w.git("commit", "-qm", "its test"); err != nil {
		return err
	}
	return p.m.w.d.Tests(p.m.w.dir, Tests{Language: "go", Command: "go test -count=1 -failfast ./."})
}

// pristineAllSurvived is whether the last run's --json object says file's
// baseline passed and lists its mutants, every one survived.
func (p *pristineState) pristineAllSurvived(file, again string) error {
	if file != again {
		return fmt.Errorf("the step names %s and %s, not one file", file, again)
	}
	p.m.file = file
	mutants, err := p.m.greetMutants()
	if err != nil {
		return err
	}
	object, err := p.m.w.json()
	if err != nil {
		return err
	}
	files, _ := object["files"].([]any)
	for _, one := range files {
		f, _ := one.(map[string]any)
		if name, _ := f["file"].(string); filepath.ToSlash(name) == file && f["baseline"] != "passed" {
			return fmt.Errorf("%s's baseline is %v, not passed\n%s", file, f["baseline"], p.m.w.report())
		}
	}
	for _, mu := range mutants {
		if mu["outcome"] != "survived" {
			return fmt.Errorf("a mutant of %s on line %d is %v, not survived\n%s", file, lineOf(mu), mu["outcome"], p.m.w.report())
		}
	}
	return nil
}

// pristineProjectUnchanged is whether the scratch folder's testdata holds
// what the Given committed and nothing else: no run of the tests reached
// the project's own folder. It is a guard, which held before slice-13 too.
func (p *pristineState) pristineProjectUnchanged() error {
	for name, text := range p.committed {
		if got := p.m.read(name); got != text {
			return fmt.Errorf("the project's %s holds %q, not %q as committed\n%s", name, got, text, p.m.w.report())
		}
	}
	if p.m.w.d.Exists(filepath.Join(p.m.w.dir, "testdata", "left-behind")) {
		return fmt.Errorf("the project's testdata/left-behind is there\n%s", p.m.w.report())
	}
	return nil
}
