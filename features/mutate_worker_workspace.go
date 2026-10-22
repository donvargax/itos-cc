package features

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

// A worker copies a TypeScript workspace member's whole workspace
// (slice-19, issue #44): a tsconfig.json that extends one at the
// workspace's root then loads in the copy as in the project. A baseline
// that fails only in the worker's copy, its tests passing in the
// project's own folder, says so.

// workspaceCopyState is what a slice-19 scenario's Given wrote: the
// package's folder, slash-separated from the scratch folder.
type workspaceCopyState struct {
	m   *mutateState
	pkg string
}

func workspaceCopySteps(sc *godog.ScenarioContext, m *mutateState) {
	s := &workspaceCopyState{m: m}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*s = workspaceCopyState{m: m}
		return ctx, nil
	})
	sc.Step(`^a (pnpm workspace listing packages/\* in pnpm-workspace\.yaml|npm workspace listing packages/\* in its package\.json) whose root holds tsconfig\.base\.json, with the member packages/shapes, tested with Vitest, whose tsconfig\.json extends \.\./\.\./tsconfig\.base\.json and whose tests pass in the project$`, s.workspaceCopyMember)
	sc.Step(`^(\S+)'s baseline passed, and every mutant of it was judged$`, s.workspaceCopyJudged)
	sc.Step(`^a TypeScript package in no workspace, tested with Vitest, whose tsconfig\.json extends \.\./tsconfig\.base\.json in the folder above it, and whose tests pass in the project$`, s.workspaceCopyOutside)
	sc.Step(`^I run "itos-cc mutation run --json" for its source$`, func() error { return m.w.runWith(m.w.env(), "mutation run --json "+m.file) })
	sc.Step(`^the problem is "mutation\.baseline-failed", its message saying the tests pass in the project but fail in the worker's copy of the package$`, s.workspaceCopyOnlyInCopy)
	sc.Step(`^when its tests fail in the project too, the message is "([^"]*)", as before$`, s.workspaceCopyFailsInProject)
}

const (
	// workspaceCopyBase is the workspace's shared tsconfig, which a
	// member's own extends: Vite reads the chain to transform its tests.
	workspaceCopyBase = `{"compilerOptions": {"strict": true, "target": "ES2022", "module": "ESNext", "moduleResolution": "bundler"}}` + "\n"
	workspaceCopyArea = "export function area(width: number, height: number): number {\n  return width * height;\n}\n"
	// workspaceCopyTest kills every mutant of area: it checks the area of
	// a 2 by 3 rectangle, which none of them gives.
	workspaceCopyTest = "import { expect, test } from \"vitest\";\nimport { area } from \"./area\";\n\n" +
		"test(\"area\", () => {\n  expect(area(2, 3)).toBe(6);\n});\n"
	workspaceCopyPackage = `{"name": "shapes", "type": "module", "devDependencies": {"vitest": "5.0.2", "@vitest/coverage-v8": "5.0.2"}}` + "\n"
)

// workspaceCopyPackageAt writes the package shapes at dir, its tsconfig.json
// extending base, and declares that Vitest measures and runs its tests
// there, its node_modules the pinned tools.
func (s *workspaceCopyState) workspaceCopyPackageAt(dir, base string) error {
	s.pkg = dir
	s.m.file = path.Join(dir, "src/area.ts")
	s.m.language = "TypeScript"
	files := map[string]string{
		path.Join(dir, "package.json"):     workspaceCopyPackage,
		path.Join(dir, "tsconfig.json"):    fmt.Sprintf(`{"extends": %q, "include": ["src"]}`+"\n", base),
		path.Join(dir, "src/area.ts"):      workspaceCopyArea,
		path.Join(dir, "src/area.test.ts"): workspaceCopyTest,
		"tsconfig.base.json":               workspaceCopyBase,
	}
	if err := s.m.writeProject(map[string][]string{path.Join(dir, "src/area.test.ts"): {s.m.file}}, files); err != nil {
		return err
	}
	at := filepath.Join(s.m.w.dir, filepath.FromSlash(dir))
	if err := s.m.w.d.Coverage(at, Coverage{Language: "typescript"}); err != nil {
		return err
	}
	return s.m.w.d.Tests(at, Tests{Language: "typescript", Command: vitestCommand + "src/area.ts"})
}

// workspaceCopyMember writes the workspace the row names at the scratch
// folder, with tsconfig.base.json at its root and the member
// packages/shapes, whose tsconfig.json extends it.
func (s *workspaceCopyState) workspaceCopyMember(workspace string) error {
	root := `{"name": "shapes-workspace", "private": true}` + "\n"
	if strings.HasPrefix(workspace, "pnpm") {
		if err := s.m.w.write("pnpm-workspace.yaml", "packages:\n  - \"packages/*\"\n"); err != nil {
			return err
		}
	} else {
		root = `{"name": "shapes-workspace", "private": true, "workspaces": ["packages/*"]}` + "\n"
	}
	if err := s.m.w.write("package.json", root); err != nil {
		return err
	}
	return s.workspaceCopyPackageAt("packages/shapes", "../../tsconfig.base.json")
}

// workspaceCopyOutside writes the package shapes in no workspace, with
// tsconfig.base.json in the scratch folder above it, which its
// tsconfig.json extends.
func (s *workspaceCopyState) workspaceCopyOutside() error {
	return s.workspaceCopyPackageAt("shapes", "../tsconfig.base.json")
}

// workspaceCopyJudged is whether the last run's --json object says file's
// baseline passed and lists its mutants, every one run and judged: killed,
// survived or timed out, none uncovered or left without an outcome.
func (s *workspaceCopyState) workspaceCopyJudged(file string) error {
	s.m.file = file
	object, err := s.m.w.json()
	if err != nil {
		return err
	}
	files, _ := object["files"].([]any)
	for _, one := range files {
		f, _ := one.(map[string]any)
		if name, _ := f["file"].(string); filepath.ToSlash(name) == file && f["baseline"] != "passed" {
			return fmt.Errorf("%s's baseline is %v, not passed\n%s", file, f["baseline"], s.m.w.report())
		}
	}
	mutants, err := s.m.greetMutants()
	if err != nil {
		return err
	}
	for _, mu := range mutants {
		switch mu["outcome"] {
		case "killed", "survived", "timeout":
		default:
			return fmt.Errorf("a mutant of %s on line %d is %v, not judged\n%s", file, lineOf(mu), mu["outcome"], s.m.w.report())
		}
	}
	return nil
}

// workspaceCopyOnlyInCopy is whether the last run reports the file's
// baseline failed, its message saying its tests pass in the project but
// fail in the worker's copy of the package's folder.
func (s *workspaceCopyState) workspaceCopyOnlyInCopy() error {
	message, err := s.workspaceCopyBaselineFailed()
	if err != nil {
		return err
	}
	want := s.m.file + ": its tests pass in the project but fail in the worker's copy of " + s.pkg
	if !strings.HasPrefix(filepath.ToSlash(message), want) {
		return fmt.Errorf("the problem's message is %q, not one starting %q\n%s", message, want, s.m.w.report())
	}
	return nil
}

// workspaceCopyFailsInProject makes the package's test fail in the project
// too, expecting another area, runs the last run again, and checks its
// baseline-failed problem says message of the file, as before slice-19.
func (s *workspaceCopyState) workspaceCopyFailsInProject(message string) error {
	failing := strings.Replace(workspaceCopyTest, "toBe(6)", "toBe(7)", 1)
	if err := s.m.w.write(path.Join(s.pkg, "src/area.test.ts"), failing); err != nil {
		return err
	}
	last := s.m.w.last
	if last == nil {
		return fmt.Errorf("no run of itos-cc to repeat")
	}
	if err := s.m.w.runEnv(last.dir, last.env, last.args...); err != nil {
		return err
	}
	got, err := s.workspaceCopyBaselineFailed()
	if err != nil {
		return err
	}
	want := s.m.file + ": " + message
	if filepath.ToSlash(got) != want {
		return fmt.Errorf("the problem's message is %q, not %q\n%s", got, want, s.m.w.report())
	}
	return s.m.w.exitsWith(1)
}

// workspaceCopyBaselineFailed is the message of the problem
// "mutation.baseline-failed" of the scenario's file in the last run's
// --json object.
func (s *workspaceCopyState) workspaceCopyBaselineFailed() (string, error) {
	object, err := s.m.w.json()
	if err != nil {
		return "", err
	}
	problems, _ := object["problems"].([]any)
	for _, one := range problems {
		problem, _ := one.(map[string]any)
		file, _ := problem["file"].(string)
		if problem["rule"] == "mutation.baseline-failed" && filepath.ToSlash(file) == s.m.file {
			message, _ := problem["message"].(string)
			return message, nil
		}
	}
	return "", fmt.Errorf("no problem \"mutation.baseline-failed\" names %s\n%s", s.m.file, s.m.w.report())
}
