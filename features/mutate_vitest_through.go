package features

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

// A package whose Vitest comes in through another package (slice-17,
// issue #45): its package.json names neither vitest nor jest, and
// node_modules/.bin holds vitest all the same.

// The package of ID-MUT-280: area, every mutant of which its test kills,
// so the run exits 0 once Vitest judges it.
const (
	vitestThroughSource = "export function area(w: number, h: number): number {\n  if (w > 2) {\n    return w * h;\n  }\n  return 1;\n}\n"
	vitestThroughTest   = "import { expect, test } from \"vitest\";\nimport { area } from \"./area\";\n\n" +
		"test(\"area\", () => {\n  expect(area(3, 5)).toBe(15);\n  expect(area(2, 5)).toBe(1);\n});\n"
)

// vitestThroughConfigs are the vite and vitest configs a package could
// keep beside its package.json, in each extension Vite and Vitest load.
var vitestThroughConfigs = func() []string {
	var names []string
	for _, base := range []string{"vite.config", "vitest.config"} {
		for _, ext := range []string{"js", "mjs", "cjs", "ts", "mts", "cts"} {
			names = append(names, base+"."+ext)
		}
	}
	return names
}()

func vitestThroughSteps(sc *godog.ScenarioContext, m *mutateState) {
	sc.Step(`^a TypeScript package whose package\.json names neither vitest nor jest, whose node_modules/\.bin holds vitest and @vitest/coverage-v8 installed through a toolchain package it depends on, whose test script is "([^"]*)", and no c8$`, m.vitestThroughPackage)
	sc.Step(`^when its test script is "([^"]*)" and no vite or vitest config sits beside its package\.json, the test command is its package manager's run test, as before$`, m.vitestThroughScriptRunsTest)
}

// vitestThroughManifest is the package's package.json, its test script
// script: it depends on @vitest/coverage-v8 and on a package named
// toolchain, which nothing installs, never on vitest or jest.
func vitestThroughManifest(script string) (string, error) {
	data, err := json.Marshal(map[string]any{
		"name": "area", "type": "module",
		"scripts":         map[string]string{"test": script},
		"devDependencies": map[string]string{"toolchain": "1.0.0", "@vitest/coverage-v8": "5.0.2"},
	})
	if err != nil {
		return "", err
	}
	var pkg struct {
		Dependencies, DevDependencies map[string]string
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return "", err
	}
	for _, name := range []string{"vitest", "jest"} {
		if pkg.Dependencies[name] != "" || pkg.DevDependencies[name] != "" {
			return "", fmt.Errorf("package.json names %s: %s", name, data)
		}
	}
	return string(data) + "\n", nil
}

// vitestThroughPackage writes the package, its test script script, and
// declares its tools: the pinned Vitest and its provider without c8, and
// what its tests do and cover when vitest related runs them.
func (m *mutateState) vitestThroughPackage(ctx context.Context, script string) error {
	m.file, m.language, m.has, m.pm = "src/area.ts", "TypeScript", "vitest through a toolchain", "npm"
	manifest, err := vitestThroughManifest(script)
	if err != nil {
		return err
	}
	if err := m.writeProject(map[string][]string{"src/area.test.ts": {m.file}}, map[string]string{
		"package.json": manifest, m.file: vitestThroughSource, "src/area.test.ts": vitestThroughTest,
	}); err != nil {
		return err
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "typescript", WithoutC8: true,
		Report: "SF:src/area.ts\nDA:2,1\nDA:3,1\nDA:5,1\nend_of_record\n"}); err != nil {
		return err
	}
	if err := m.w.d.Tests(m.w.dir, Tests{Language: "typescript", Command: vitestCommand + m.file}); err != nil {
		return err
	}
	if err := m.w.d.Tools(ctx, m.w.dir, ToolSetup{NodeBins: []string{"vitest"}, Env: m.w.env()}); err != nil {
		return err
	}
	for _, c8 := range []string{"c8", "c8.cmd"} {
		if m.w.d.Exists(filepath.Join(m.w.dir, "node_modules", ".bin", c8)) {
			return fmt.Errorf("the package has c8 installed")
		}
	}
	return m.vitestThroughNoConfig()
}

// vitestThroughNoConfig is whether no vite or vitest config sits beside
// the package's package.json.
func (m *mutateState) vitestThroughNoConfig() error {
	for _, name := range vitestThroughConfigs {
		if m.w.d.Exists(filepath.Join(m.w.dir, name)) {
			return fmt.Errorf("%s sits beside the package's package.json", name)
		}
	}
	return nil
}

// vitestThroughScriptRunsTest gives the package the test script script,
// checks no vite or vitest config sits beside its package.json, and runs
// its mutants again without coverage: the test command is its package
// manager's run test, as for a package with neither Vitest nor Jest.
func (m *mutateState) vitestThroughScriptRunsTest(script string) error {
	if strings.Contains(script, "vit") || strings.Contains(script, "vp") {
		return fmt.Errorf("the test script %q names vitest, vp or vite", script)
	}
	manifest, err := vitestThroughManifest(script)
	if err != nil {
		return err
	}
	if err := m.w.write("package.json", manifest); err != nil {
		return err
	}
	if err := m.vitestThroughNoConfig(); err != nil {
		return err
	}
	if err := m.w.d.Tests(m.w.dir, Tests{Language: "typescript", Command: m.pm + " run test"}); err != nil {
		return err
	}
	if err := m.itsMutantsRun(); err != nil {
		return err
	}
	return m.testCommandIs("<pm> run test")
}
