package features

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

// The project's coverage floor (slice-16, issue #46): Vitest's
// test.coverage.thresholds and coverage.py's fail_under hold the whole
// package to 80%, which the whole suite meets and a run of one file's tests
// does not, as every other file is then at 0%.

// floorProject is the project a Given of the floor's Rule wrote: its
// language, the file the run measures and the config holding the floor.
type floorProject struct {
	language, file, config, configText string
}

func (c *coverageState) floorSteps(sc *godog.ScenarioContext) {
	f := &floorProject{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*f = floorProject{}
		return ctx, nil
	})
	sc.Step(`^a (TypeScript|Python) project with two files, each fully covered by its own test, and (.+)$`, func(language, _ string) error {
		return c.floorProject(f, language, false)
	})
	sc.Step(`^the whole suite meets that floor, and either file's test alone does not$`, func() error {
		// The fixture's arithmetic: each file holds half the package's
		// statements, and only its own test runs them.
		if f.language == "" {
			return fmt.Errorf("no project with a coverage floor was written")
		}
		return nil
	})
	sc.Step(`^I run "itos-cc ([^"]*)" for one of the two files$`, func(args string) error {
		return c.w.runWith(c.w.env(), args+" --no-annotate "+f.file)
	})
	sc.Step(`^stderr says no coverage command failed$`, c.noCoverageCommandFailed)
	sc.Step(`^no problem is "([^"]+)" or "([^"]+)"$`, func(first, second string) error { return c.noProblemOf(first, second) })
	sc.Step(`^coverage runs jest with --findRelatedTests (\S+) and (--coverageThreshold=\S+)$`, c.jestRuns)
	sc.Step(`^the TypeScript project of ID-COV-44, its thresholds set to autoUpdate$`, func() error {
		return c.floorProject(f, "TypeScript", true)
	})
	sc.Step(`^vitest\.config\.ts is byte for byte as committed$`, func() error {
		data, err := c.w.readIn(c.w.dir, "vitest.config.ts")
		if err != nil {
			return err
		}
		if string(data) != f.configText {
			return fmt.Errorf("vitest.config.ts changed to:\n%s\n%s", data, c.w.report())
		}
		return nil
	})
	sc.Step(`^the TypeScript project of ID-COV-44, with a third file no test imports$`, func() error {
		if err := c.floorProject(f, "TypeScript", false); err != nil {
			return err
		}
		f.file = "src/c.ts"
		if err := c.w.write(f.file, floorUntested); err != nil {
			return err
		}
		// mutate.feature's step runs "for that file" without naming it
		// here, so a mutation run that names no source is given it.
		c.w.prepare = func(args []string) []string {
			if len(args) >= 2 && args[0] == "mutation" && args[1] == "run" && !slices.Contains(args, f.file) {
				args = append(args, "--no-annotate", f.file)
			}
			return args
		}
		return c.commit("a file no test imports")
	})
	sc.Step(`^its executable lines are "mutation\.uncovered-statement" and its mutants "mutation\.uncovered"$`, func() error {
		return c.untestedFileUncovered(f.file)
	})
}

// floorSource is a file whose function each test fully covers.
func floorSource(name string, language string, a, b int) string {
	if language == "Python" {
		return fmt.Sprintf("def %s(x):\n    if x > 0:\n        return %d\n    return %d\n", name, a, b)
	}
	return fmt.Sprintf("export function %s(x: number): number {\n  if (x > 0) {\n    return %d;\n  }\n  return %d;\n}\n", name, a, b)
}

// floorUntested is a TypeScript file no test imports: its lines 2, 3 and 5
// are executable.
const floorUntested = "export function c(x: number): number {\n  if (x > 0) {\n    return 5;\n  }\n  return 6;\n}\n"

// floorTest is the test of name, in name's own file, running every line.
func floorTest(name, language string, a, b int) string {
	if language == "Python" {
		return fmt.Sprintf("from %s import %s\n\n\ndef test_%s():\n    assert %s(1) == %d\n    assert %s(0) == %d\n", name, name, name, name, a, name, b)
	}
	return fmt.Sprintf("import { expect, test } from \"vitest\";\nimport { %s } from \"./%s\";\n\ntest(%q, () => {\n  expect(%s(1)).toBe(%d);\n  expect(%s(0)).toBe(%d);\n});\n", name, name, name, name, a, name, b)
}

// vitestFloor is a vitest.config.ts holding the package to 80% of its
// statements, every file of src/ counted, and with autoUpdate, writing a
// higher measured value back.
func vitestFloor(autoUpdate bool) string {
	thresholds := "{ statements: 80 }"
	if autoUpdate {
		thresholds = "{ statements: 80, autoUpdate: true }"
	}
	return "import { defineConfig } from \"vitest/config\";\n\nexport default defineConfig({\n" +
		"  test: { coverage: { include: [\"src/**\"], thresholds: " + thresholds + " } },\n});\n"
}

// floorProject writes, and commits, a project of language whose two files
// a and b each have a test of their own covering every line, under a floor
// of 80% of the package.
func (c *coverageState) floorProject(f *floorProject, language string, autoUpdate bool) error {
	f.language = language
	files := map[string]string{}
	switch language {
	case "TypeScript":
		f.file, f.config, f.configText = "src/a.ts", "vitest.config.ts", vitestFloor(autoUpdate)
		files["package.json"] = `{"name": "floor", "type": "module", "devDependencies": {"vitest": "5.0.2", "@vitest/coverage-v8": "5.0.2"}}` + "\n"
		files[f.config] = f.configText
		for i, name := range []string{"a", "b"} {
			files["src/"+name+".ts"] = floorSource(name, language, 2*i+1, 2*i+2)
			files["src/"+name+".test.ts"] = floorTest(name, language, 2*i+1, 2*i+2)
		}
	case "Python":
		f.file, f.config = "a.py", "pyproject.toml"
		f.configText = "[project]\nname = \"floor\"\nversion = \"0.0.0\"\n\n[tool.coverage.report]\nfail_under = 80\n"
		files[f.config] = f.configText
		for i, name := range []string{"a", "b"} {
			files[name+".py"] = floorSource(name, language, 2*i+1, 2*i+2)
			files["test_"+name+".py"] = floorTest(name, language, 2*i+1, 2*i+2)
		}
	}
	if err := c.writeIn(".", files); err != nil {
		return err
	}
	if err := c.declare(".", Coverage{Language: strings.ToLower(language)}); err != nil {
		return err
	}
	if language == "Python" {
		if err := c.w.d.Tests(c.w.dir, Tests{Language: "python", Command: "python3 -m pytest -q -x -p no:cacheprovider"}); err != nil {
			return err
		}
	}
	if err := c.w.git("init", "-q"); err != nil {
		return err
	}
	return c.commit("a project under a coverage floor")
}

// commit commits everything in the scratch folder but the node_modules a
// TypeScript declaration links there.
func (c *coverageState) commit(message string) error {
	if err := c.w.write(".gitignore", "node_modules\n"); err != nil {
		return err
	}
	if err := c.w.git("add", "."); err != nil {
		return err
	}
	return c.w.git("-c", "user.name=fixture", "-c", "user.email=fixture@example.com", "commit", "-qm", message)
}

// failedCoverageCommand is the line the coverage runner logs when one of a
// plan's commands exits non-zero: "itos-cc: coverage: <language>: <error>".
var failedCoverageCommand = regexp.MustCompile(`(?m)^itos-cc: coverage: (go|typescript|python|kotlin|dart): `)

func (c *coverageState) noCoverageCommandFailed() error {
	plain := regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(c.w.stderr, "")
	if line := failedCoverageCommand.FindString(plain); line != "" {
		return fmt.Errorf("a coverage command failed (%q)\n%s", line, c.w.report())
	}
	if len(c.announced(c.w.stderr)) == 0 {
		return fmt.Errorf("no coverage command ran\n%s", c.w.report())
	}
	return nil
}

// problemsOf is the problems of the last run's --json object, by rule.
func (c *coverageState) problemsOf(rule string) ([]map[string]any, error) {
	object, _, err := c.w.lastObject()
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	problems, _ := object["problems"].([]any)
	for _, one := range problems {
		if p, _ := one.(map[string]any); p["rule"] == rule {
			out = append(out, p)
		}
	}
	return out, nil
}

func (c *coverageState) noProblemOf(rules ...string) error {
	for _, rule := range rules {
		found, err := c.problemsOf(rule)
		if err != nil {
			return err
		}
		if len(found) > 0 {
			return fmt.Errorf("a problem is %q: %v\n%s", rule, found[0], c.w.report())
		}
	}
	return nil
}

// jestRuns is whether a coverage command of the project runs jest, from
// its node_modules/.bin, selecting the tests related to file and with
// threshold among its words.
func (c *coverageState) jestRuns(file, threshold string) error {
	for _, a := range c.commands() {
		if program(a.words[0]) != "jest" {
			continue
		}
		i := slices.Index(a.words, "--findRelatedTests")
		if i < 0 || i+1 >= len(a.words) || filepath.ToSlash(a.words[i+1]) != file {
			return fmt.Errorf("%q selects no tests related to %s\n%s", a.command, file, c.w.report())
		}
		if !slices.Contains(a.words, threshold) {
			return fmt.Errorf("%q does not hold %s\n%s", a.command, threshold, c.w.report())
		}
		return nil
	}
	return fmt.Errorf("no coverage command runs jest\n%s", c.w.report())
}

// untestedFileUncovered is whether the run reports each executable line of
// floorUntested, file, as mutation.uncovered-statement and its mutants as
// mutation.uncovered.
func (c *coverageState) untestedFileUncovered(file string) error {
	statements, err := c.problemsOf("mutation.uncovered-statement")
	if err != nil {
		return err
	}
	lines := map[float64]bool{}
	for _, p := range statements {
		if filepath.ToSlash(fmt.Sprint(p["file"])) == file {
			lines[p["line"].(float64)] = true
		}
	}
	for _, line := range []float64{2, 3, 5} {
		if !lines[line] {
			return fmt.Errorf("line %v of %s is not mutation.uncovered-statement\n%s", line, file, c.w.report())
		}
	}
	mutants, err := c.problemsOf("mutation.uncovered")
	if err != nil {
		return err
	}
	for _, p := range mutants {
		if filepath.ToSlash(fmt.Sprint(p["file"])) == file {
			return nil
		}
	}
	return fmt.Errorf("no mutant of %s is mutation.uncovered\n%s", file, c.w.report())
}
