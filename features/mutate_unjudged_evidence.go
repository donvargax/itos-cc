package features

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

// This regression owns its fixture independently of the language conversion.
// The real custom test command is go version in every source language: no
// collector or language runtime is needed or claimed by these outcomes.
type unjudgedEvidence struct {
	m                  *mutateState
	s                  *sinceMutation
	file, test, source string
	before, after      map[string]any
	result             map[string]any
}

func unjudgedEvidenceSteps(sc *godog.ScenarioContext, m *mutateState, s *sinceMutation) {
	u := &unjudgedEvidence{m: m, s: s}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*u = unjudgedEvidence{m: m, s: s}
		return ctx, nil
	})
	sc.Step(`^a committed (Go|TypeScript|Python|Kotlin|Dart) project with two functions whose whole-suite outcomes record nonempty test and support evidence$`, u.project)
	sc.Step(`^a non-importing test and a configured support file changed since those outcomes were recorded$`, u.dependenciesChanged)
	sc.Step(`^only one of its functions changed since "base"$`, u.functionChanged)
	sc.Step(`^a --since run judges that changed function$`, u.run)
	sc.Step(`^the unjudged function keeps its original test and support evidence, recorded scope, source hash and mutant outcomes$`, u.originalKept)
	sc.Step(`^the judged function records current whole-suite evidence$`, u.currentJudged)
	sc.Step(`^mutation check reports the unjudged function stale and mutation sample excludes its outcomes$`, u.staleExcluded)
}

func (u *unjudgedEvidence) project(language string) error {
	files := map[string]string{"itos-cc.yaml": "mutation:\n  tests:\n    list: 'go version'\n    run: 'go version {pattern}'\n    ids_pattern: '{ids}'\n    join:\n      each: '{id}'\n      sep: ','\n    support: ['support.feature']\n", "support.feature": "Feature: original support\n"}
	switch language {
	case "Go":
		u.file, u.test = "src/board.go", "e2e/other_test.go"
		u.source = "package board\nfunc Changed() bool { return true }\nfunc Kept() bool { return true }\n"
		files["go.mod"] = "module example.com/evidence\n\ngo 1.23\n"
		files[u.test] = "package e2e\nimport \"testing\"\nfunc TestOther(t *testing.T) {}\n"
	case "TypeScript":
		u.file, u.test = "src/board.ts", "e2e/other.test.ts"
		u.source = "export function Changed(): boolean { return true; }\nexport function Kept(): boolean { return true; }\n"
		files["package.json"] = "{\"name\":\"evidence\",\"version\":\"1.0.0\"}\n"
		files[u.test] = "export const unrelated = 'original';\n"
	case "Python":
		u.file, u.test = "board.py", "tests/test_other.py"
		u.source = "def Changed():\n    return True\ndef Kept():\n    return True\n"
		files["pyproject.toml"] = "[project]\nname = \"evidence\"\nversion = \"1.0.0\"\n"
		files[u.test] = "def test_other():\n    pass\n"
	case "Kotlin":
		u.file, u.test = "src/main/kotlin/Board.kt", "src/test/kotlin/OtherTest.kt"
		u.source = "package evidence\nfun Changed(): Boolean { return true }\nfun Kept(): Boolean { return true }\n"
		files["build.gradle.kts"] = "plugins { kotlin(\"jvm\") version \"2.4.21\" }\n"
		files["settings.gradle.kts"] = "rootProject.name = \"evidence\"\n"
		files[u.test] = "package evidence\nclass OtherTest\n"
	case "Dart":
		u.file, u.test = "lib/board.dart", "test/other_test.dart"
		u.source = "bool Changed() {\n  return true;\n}\n\nbool Kept() {\n  return true;\n}\n"
		files["pubspec.yaml"] = "name: evidence\nenvironment:\n  sdk: ^3.6.0\n"
		files[u.test] = "void main() {}\n"
	}
	files[u.file] = u.source
	if err := u.m.writeAll(files); err != nil {
		return err
	}
	if err := u.declare(); err != nil {
		return err
	}
	w := u.m.w
	var paths []string
	for file := range files {
		paths = append(paths, file)
	}
	slices.Sort(paths)
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, append([]string{"add"}, paths...), {"commit", "-qm", "original evidence"}, {"tag", "base"}} {
		if err := w.git(args...); err != nil {
			return err
		}
	}
	if err := u.command(false); err != nil {
		return err
	}
	var err error
	u.before, err = u.s.snapshot(u.file)
	if err != nil {
		return err
	}
	for _, name := range []string{"Changed", "Kept"} {
		unit, err := evidenceUnit(u.before, name)
		if err != nil {
			return err
		}
		if err := u.evidenceMatches(unit); err != nil {
			return err
		}
	}
	return nil
}

func (u *unjudgedEvidence) declare() error {
	sites, err := u.m.sitesOf(u.file)
	if err != nil {
		return err
	}
	if len(sites) != 2 {
		return fmt.Errorf("fixture lists %d sites, want two", len(sites))
	}
	t := Tests{Language: "go", Command: "go version"}
	for _, site := range sites {
		t.Surviving = append(t.Surviving, Site{File: u.file, Line: site.line, Original: site.original, Replacement: site.replacement})
	}
	return u.m.w.d.Tests(u.m.w.dir, t)
}

func (u *unjudgedEvidence) dependenciesChanged() error {
	if err := u.m.w.write(u.test, u.m.read(u.test)+"\n"); err != nil {
		return err
	}
	return u.m.w.write("support.feature", "Feature: changed support\n")
}

func (u *unjudgedEvidence) functionChanged() error {
	source := strings.Replace(u.source, "true", "false", 1)
	if source == u.source {
		source = strings.Replace(u.source, "True", "False", 1)
	}
	if source == u.source {
		return fmt.Errorf("fixture has no changed literal")
	}
	if err := u.m.w.write(u.file, source); err != nil {
		return err
	}
	if err := u.declare(); err != nil {
		return err
	}
	if err := u.m.w.git("add", u.file, u.test, "support.feature"); err != nil {
		return err
	}
	return u.m.w.git("commit", "-qm", "change one function and suite inputs")
}

func (u *unjudgedEvidence) command(partial bool) error {
	args := []string{"mutation", "run", "--json", "--no-coverage", "--no-annotate", "--workers", "1", "--test-command", "go version"}
	if partial {
		args = append(args, "--since", "base")
	}
	args = append(args, u.file)
	code, stdout, stderr, err := u.m.w.capture(u.m.w.dir, u.m.w.env(), args...)
	if err != nil {
		return err
	}
	if code != 1 {
		return fmt.Errorf("mutation run exit%d, want survived exit1: %s%s", code, stdout, stderr)
	}
	if err := json.Unmarshal([]byte(stdout), &u.result); err != nil {
		return err
	}
	return nil
}

func (u *unjudgedEvidence) run() error {
	if err := u.command(true); err != nil {
		return err
	}
	files, _ := u.result["files"].([]any)
	if len(files) != 1 {
		return fmt.Errorf("partial run has %d files", len(files))
	}
	file := files[0].(map[string]any)
	judged, _ := file["judged"].([]any)
	if len(judged) != 1 || !strings.HasSuffix(fmt.Sprint(judged[0]), "#Changed") {
		return fmt.Errorf("partial judged population: %v", judged)
	}
	var err error
	u.after, err = u.s.snapshot(u.file)
	return err
}

// A required unit and mutant cannot disappear into an empty comparison.
func evidenceUnit(snapshot map[string]any, name string) (map[string]any, error) {
	units, _ := snapshot["units"].([]any)
	if len(units) != 2 {
		return nil, fmt.Errorf("snapshot has %d units, want two", len(units))
	}
	var matches []map[string]any
	for _, raw := range units {
		unit := raw.(map[string]any)
		if unit["name"] == name {
			matches = append(matches, unit)
		}
	}
	if len(matches) != 1 {
		return nil, fmt.Errorf("snapshot has %d %s units", len(matches), name)
	}
	u := matches[0]
	mutants, _ := u["mutants"].([]any)
	if u["hash"] == nil || u["hash"] == "" || len(mutants) != 1 {
		return nil, fmt.Errorf("%s lacks hash/one mutant: %v", name, u)
	}
	m := mutants[0].(map[string]any)
	e, _ := m["suite_evidence"].(map[string]any)
	tests, _ := e["tests"].(map[string]any)
	support, _ := e["support"].(map[string]any)
	if len(tests) == 0 || len(support) == 0 || m["scope"] != "go version" || m["outcome"] != "survived" {
		return nil, fmt.Errorf("%s lacks original broad judgment/evidence: %v", name, u)
	}
	return u, nil
}

func (u *unjudgedEvidence) originalKept() error {
	before, err := evidenceUnit(u.before, "Kept")
	if err != nil {
		return err
	}
	after, err := evidenceUnit(u.after, "Kept")
	if err != nil {
		return err
	}
	if before["hash"] != after["hash"] {
		return fmt.Errorf("unjudged source hash changed: %v -> %v", before, after)
	}
	old := before["mutants"].([]any)[0].(map[string]any)
	now := after["mutants"].([]any)[0].(map[string]any)
	for _, key := range []string{"original", "replacement", "outcome", "scope", "suite_evidence", "go_evidence", "tests", "coverage"} {
		if !reflect.DeepEqual(old[key], now[key]) {
			return fmt.Errorf("unjudged %s changed: before%v after%v", key, old[key], now[key])
		}
	}
	for _, key := range []string{"sites", "killed", "survived", "uncovered", "go_coverage", "line_coverage"} {
		if !reflect.DeepEqual(before[key], after[key]) {
			return fmt.Errorf("unjudged %s changed", key)
		}
	}
	return nil
}

func (u *unjudgedEvidence) evidenceMatches(unit map[string]any) error {
	mutant := unit["mutants"].([]any)[0].(map[string]any)
	evidence := mutant["suite_evidence"].(map[string]any)
	for kind, file := range map[string]string{"tests": u.test, "support": "support.feature"} {
		expected := fmt.Sprintf("%x", sha256.Sum256([]byte(u.m.read(file))))
		values, _ := evidence[kind].(map[string]any)
		if len(values) != 1 || values[file] != expected {
			return fmt.Errorf("%s evidence%v does not describe current %s hash%s", kind, values, file, expected)
		}
	}
	return nil
}

func (u *unjudgedEvidence) currentJudged() error {
	unit, err := evidenceUnit(u.after, "Changed")
	if err != nil {
		return err
	}
	before, err := evidenceUnit(u.before, "Changed")
	if err != nil {
		return err
	}
	if before["hash"] == unit["hash"] {
		return fmt.Errorf("judged source hash did not change")
	}
	return u.evidenceMatches(unit)
}

func (u *unjudgedEvidence) staleExcluded() error {
	w := u.m.w
	code, stdout, stderr, err := w.capture(w.dir, w.env(), "mutation", "check", "--json", u.file)
	if err != nil {
		return err
	}
	var check map[string]any
	if err := json.Unmarshal([]byte(stdout), &check); err != nil {
		return err
	}
	problems, _ := check["problems"].([]any)
	stale := 0
	for _, raw := range problems {
		p := raw.(map[string]any)
		if p["rule"] == "mutation.stale" && strings.HasSuffix(fmt.Sprint(p["function"]), "#Kept") {
			stale++
		}
	}
	if code != 1 || stale != 1 {
		return fmt.Errorf("check exit%d stale%d: %s%s", code, stale, stdout, stderr)
	}
	code, stdout, stderr, err = w.capture(w.dir, w.env(), "mutation", "sample", "--json", "--count", "20", "--seed", "4813e48", u.file)
	if err != nil {
		return err
	}
	var sample map[string]any
	if err := json.Unmarshal([]byte(stdout), &sample); err != nil {
		return err
	}
	files, _ := sample["files"].([]any)
	if code != 0 || len(files) != 1 {
		return fmt.Errorf("sample exit%d: %s%s", code, stdout, stderr)
	}
	mutants, _ := files[0].(map[string]any)["mutants"].([]any)
	if len(mutants) != 1 || !strings.HasSuffix(fmt.Sprint(mutants[0].(map[string]any)["function"]), "#Changed") {
		return fmt.Errorf("sample includes wrong population: %v", mutants)
	}
	return nil
}
