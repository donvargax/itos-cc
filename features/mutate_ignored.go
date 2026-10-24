package features

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

// A worker's copy leaves out what git ignores (slice-43, q-69 and q-92):
// a file a coverage run leaves in the project, or a stray build, never
// reaches a copy, the dependency folders stay linked, mutation.copy in
// itos-cc.yaml brings an ignored path back, a baseline that fails only in
// the copy names what it left out, and a nested checkout is never copied.

// ignoredState is what a slice-43 scenario's Givens wrote: the file the
// TypeScript package's test appends what it saw of its folder to, by its
// path slash-separated from the scratch folder, "" for none.
type ignoredState struct {
	m        *mutateState
	observed string
}

func ignoredSteps(sc *godog.ScenarioContext, m *mutateState) {
	s := &ignoredState{m: m}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*s = ignoredState{m: m}
		return ctx, nil
	})
	sc.Step(`^a TypeScript package, tested with Vitest, whose \.gitignore ignores node_modules/ and tmp/, with tmp/stray\.log, and whose tests pass in the project$`, s.ignoredTypeScript)
	sc.Step(`^I run "itos-cc mutation run --workers 1 --json" for its source$`, func() error {
		return m.w.runWith(m.w.env(), "mutation run --workers 1 --json "+m.file)
	})
	sc.Step(`^its baseline passed, and every mutant of it was judged$`, func() error {
		return (&workspaceCopyState{m: m}).workspaceCopyJudged(m.file)
	})
	sc.Step(`^the worker's copy held node_modules as a link to the project's, and no tmp/stray\.log$`, s.ignoredCopyHeld)
	sc.Step(`^gen/ is ignored by git and holds table\.txt, which the project generated and never committed$`, s.ignoredGen)
	sc.Step(`^its test, which checks no result, fails when ([^,]+)$`, s.ignoredTest)
	sc.Step(`^the problem is "mutation\.baseline-failed", its message naming (\S+) as ignored by git and left out of the worker's copy, and its fix naming mutation\.copy in itos-cc\.yaml$`, s.ignoredNamed)
	sc.Step(`^the module is in no git repository and holds gen/table\.txt$`, s.ignoredNoRepository)
	sc.Step(`^nested/ is a git repository of its own, holding marker\.txt, which git in the module neither tracks nor ignores$`, s.ignoredNested)
}

// ignoredObserved is where the TypeScript package's test appends a line
// for each of its runs, in a folder git ignores, so the project's commit
// never holds it.
const ignoredObserved = "tmp/copies.txt"

// ignoredTypeScript writes the package shapes at the scratch folder, its
// test killing every mutant of area and appending, on each run, its folder,
// whether node_modules there is a link and to what, and whether
// tmp/stray.log is there, to tmp/copies.txt of the project. It ignores
// node_modules/ and tmp/, holds tmp/stray.log, and commits the rest, then
// declares that Vitest measures and runs its tests, its node_modules the
// pinned tools.
func (s *ignoredState) ignoredTypeScript() error {
	s.m.file = "src/area.ts"
	s.m.language = "TypeScript"
	s.observed = ignoredObserved
	observed := filepath.Join(s.m.w.dir, filepath.FromSlash(ignoredObserved))
	test := "import { appendFileSync, existsSync, lstatSync, readlinkSync } from \"node:fs\";\n" +
		"import { expect, test } from \"vitest\";\nimport { area } from \"./area\";\n\n" +
		"test(\"area\", () => {\n  expect(area(2, 3)).toBe(6);\n});\n\n" +
		"test(\"where it runs\", () => {\n" +
		"  const link = lstatSync(\"node_modules\").isSymbolicLink();\n" +
		"  const seen = { cwd: process.cwd(), link, target: link ? readlinkSync(\"node_modules\") : \"\", stray: existsSync(\"tmp/stray.log\") };\n" +
		"  appendFileSync(" + strconv.Quote(observed) + ", JSON.stringify(seen) + \"\\n\");\n});\n"
	files := map[string]string{
		"package.json":     workspaceCopyPackage,
		"tsconfig.json":    workspaceCopyBase,
		"src/area.ts":      workspaceCopyArea,
		"src/area.test.ts": test,
		".gitignore":       "node_modules/\ntmp/\n",
		"tmp/stray.log":    "stray\n",
	}
	if err := s.m.writeProject(map[string][]string{"src/area.test.ts": {s.m.file}}, files); err != nil {
		return err
	}
	if err := s.m.w.d.Coverage(s.m.w.dir, Coverage{Language: "typescript"}); err != nil {
		return err
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-qm", "shapes"}} {
		if err := s.m.w.git(args...); err != nil {
			return err
		}
	}
	return s.m.w.d.Tests(s.m.w.dir, Tests{Language: "typescript", Command: vitestCommand + "src/area.ts"})
}

// ignoredSeen is one line of what the TypeScript package's test saw.
type ignoredSeen struct {
	Cwd    string `json:"cwd"`
	Link   bool   `json:"link"`
	Target string `json:"target"`
	Stray  bool   `json:"stray"`
}

// ignoredCopyHeld is whether every run of the package's test outside the
// project, in a worker's copy, saw node_modules as a link to the
// project's and no tmp/stray.log, and at least one ran there.
func (s *ignoredState) ignoredCopyHeld() error {
	if s.observed == "" {
		return fmt.Errorf("no Given wrote a test that says where it runs")
	}
	text, err := s.m.w.readIn(s.m.w.dir, s.observed)
	if err != nil {
		return fmt.Errorf("%v\n%s", err, s.m.w.report())
	}
	project := resolved(s.m.w.dir)
	modules := resolved(filepath.Join(s.m.w.dir, "node_modules"))
	copies := 0
	for _, line := range strings.Split(strings.TrimSpace(string(text)), "\n") {
		var seen ignoredSeen
		if err := json.Unmarshal([]byte(line), &seen); err != nil {
			return fmt.Errorf("%s holds %q: %v", s.observed, line, err)
		}
		if resolved(seen.Cwd) == project {
			continue
		}
		copies++
		switch {
		case !seen.Link:
			return fmt.Errorf("the copy at %s holds node_modules, not a link to the project's\n%s", seen.Cwd, s.m.w.report())
		case resolved(seen.Target) != modules:
			return fmt.Errorf("the copy at %s links node_modules to %s, not to the project's %s\n%s", seen.Cwd, seen.Target, modules, s.m.w.report())
		case seen.Stray:
			return fmt.Errorf("the copy at %s holds tmp/stray.log, which git ignores\n%s", seen.Cwd, s.m.w.report())
		}
	}
	if copies == 0 {
		return fmt.Errorf("no run of the test was in a worker's copy: %s\n%s", text, s.m.w.report())
	}
	return nil
}

// resolved is path with its links resolved, as itself when they cannot be.
func resolved(path string) string {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return filepath.Clean(path)
}

// ignoredGen writes gen/table.txt and a .gitignore that ignores gen/, then
// commits the scratch folder, gen/ left out as git ignores it.
func (s *ignoredState) ignoredGen() error {
	if err := s.m.writeAll(map[string]string{"gen/table.txt": "generated\n", ".gitignore": "gen/\n"}); err != nil {
		return err
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-qm", "leak"}} {
		if err := s.m.w.git(args...); err != nil {
			return err
		}
	}
	return nil
}

// ignoredNoRepository writes gen/table.txt in the scratch folder, which no
// git repository holds.
func (s *ignoredState) ignoredNoRepository() error {
	if s.m.w.git("rev-parse", "--git-dir") == nil {
		return fmt.Errorf("the scratch folder %s is in a git repository", s.m.w.dir)
	}
	return s.m.writeAll(map[string]string{"gen/table.txt": "generated\n"})
}

// ignoredNested commits the module's files in a repository at the scratch
// folder, and makes nested/ a repository of its own holding marker.txt,
// committed there, which the module's repository neither tracks nor
// ignores.
func (s *ignoredState) ignoredNested() error {
	nested := filepath.Join(s.m.w.dir, "nested")
	if err := s.m.w.write("nested/marker.txt", "marker\n"); err != nil {
		return err
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "marker.txt"}, {"commit", "-qm", "marker"}} {
		if err := s.m.w.d.Git(nested, args...); err != nil {
			return err
		}
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "go.mod", s.m.file}, {"commit", "-qm", "leak"}} {
		if err := s.m.w.git(args...); err != nil {
			return err
		}
	}
	return nil
}

// ignoredFound is the Go statement that fails a test when what a step
// says it finds is so, run from the package's folder.
var ignoredFound = map[string]string{
	"gen/table.txt is missing": `if _, err := os.Stat("gen/table.txt"); err != nil {
		t.Fatalf("gen/table.txt is missing: %v", err)
	}`,
	"nested/marker.txt is there": `if _, err := os.Stat("nested/marker.txt"); err == nil {
		t.Fatal("nested/marker.txt is there")
	}`,
}

// ignoredFunctions are the functions of a Given's Go file, each taking one
// int.
var ignoredFunctions = regexp.MustCompile(`(?m)^func (\w+)\(x int\)`)

// ignoredTest writes the test of the Given's file, beside it: it calls
// each function and checks none of their results, and fails when found is
// so. It commits the test where the scratch folder is a repository, and
// declares that Go measures and runs it.
func (s *ignoredState) ignoredTest(found string) error {
	check, ok := ignoredFound[found]
	if !ok {
		return fmt.Errorf("no test checks that %s", found)
	}
	source := s.m.read(s.m.file)
	pkg, ok := emptyCasePackage(source)
	if !ok {
		return fmt.Errorf("%s declares no package", s.m.file)
	}
	var calls []string
	for _, f := range ignoredFunctions.FindAllStringSubmatch(source, -1) {
		calls = append(calls, "_ = "+f[1]+"(3)")
	}
	test := strings.TrimSuffix(s.m.file, path.Ext(s.m.file)) + "_test.go"
	text := fmt.Sprintf("package %s\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\n"+
		"func TestReads(t *testing.T) {\n\t%s\n\t%s\n}\n", pkg, strings.Join(calls, "\n\t"), check)
	if err := s.m.w.write(test, text); err != nil {
		return err
	}
	if s.m.w.git("rev-parse", "--git-dir") == nil {
		if err := s.m.w.git("add", test); err != nil {
			return err
		}
		if err := s.m.w.git("commit", "-qm", "its test"); err != nil {
			return err
		}
	}
	if err := s.m.w.d.Coverage(s.m.w.dir, Coverage{Language: "go"}); err != nil {
		return err
	}
	return s.m.w.d.Tests(s.m.w.dir, Tests{Language: "go", Command: "go test -count=1 -failfast ./."})
}

// ignoredNamed is whether the last run reports the file's baseline failed
// only in the worker's copy, its message ending with left, the path git
// ignores that the copy left out, and its fix naming mutation.copy in
// itos-cc.yaml with that path as its example.
func (s *ignoredState) ignoredNamed(left string) error {
	object, err := s.m.w.json()
	if err != nil {
		return err
	}
	problems, _ := object["problems"].([]any)
	for _, one := range problems {
		problem, _ := one.(map[string]any)
		file, _ := problem["file"].(string)
		if problem["rule"] != "mutation.baseline-failed" || filepath.ToSlash(file) != s.m.file {
			continue
		}
		message, _ := problem["message"].(string)
		fix, _ := problem["fix"].(string)
		message = filepath.ToSlash(message)
		start := s.m.file + ": its tests pass in the project but fail in the worker's copy of "
		end := ", so none was judged; the copy left out what git ignores: " + left
		if !strings.HasPrefix(message, start) || !strings.HasSuffix(message, end) {
			return fmt.Errorf("the problem's message is %q, not one starting %q and ending %q\n%s", message, start, end, s.m.w.report())
		}
		example := strings.TrimSuffix(left, "/")
		if strings.HasSuffix(left, "/") {
			example += "/**"
		}
		want := "name it in mutation.copy in itos-cc.yaml, such as mutation: {copy: [" + example + "]}"
		if !strings.Contains(fix, want) {
			return fmt.Errorf("the problem's fix is %q, not one saying %q\n%s", fix, want, s.m.w.report())
		}
		return nil
	}
	return fmt.Errorf("no problem \"mutation.baseline-failed\" names %s\n%s", s.m.file, s.m.w.report())
}
