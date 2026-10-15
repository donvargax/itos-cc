// Cached-result scenarios read public commands and the snapshot JSON they write.
package features

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

type checkMutation struct {
	s       *sinceMutation
	id      string
	missing map[string]any
}

const checkBoardTestTS = `import {expect, test} from "vitest";
import {writeFileSync} from "node:fs";
import {Board} from "./board";
if (process.env.CHECK_MARKER) writeFileSync(process.env.CHECK_MARKER, "ran");
test("place", () => {
 const b = new Board();
 expect(b.place(5)).toBe(false);
 expect(b.place(6)).toBe(true);
});
test("clear", () => {
 const b = new Board();
 expect(b.clear(2)).toBe(true);
 expect(b.clear(3)).toBe(false);
});
`

func checkMutationSteps(sc *godog.ScenarioContext, m *mutateState, s *sinceMutation) {
	c := &checkMutation{s: s}
	sc.Before(func(ctx context.Context, scenario *godog.Scenario) (context.Context, error) {
		*c = checkMutation{s: s}
		for _, tag := range scenario.Tags {
			if strings.HasPrefix(tag.Name, "@ID-MUT-") {
				c.id = strings.TrimPrefix(tag.Name, "@ID-MUT-")
			}
		}
		switch c.id {
		case "66", "67":
			return ctx, c.project()
		case "64":
			if err := c.fresh(); err != nil {
				return ctx, err
			}
			if err := s.replace(s.file, "// past five", "// past five, changed"); err != nil {
				return ctx, err
			}
			return ctx, c.addMethod("reset(i: number): boolean { return i == 0; }")
		}
		return ctx, nil
	})
	sc.Step(`^a run recorded every mutant of src/board\.ts killed$`, c.fresh)
	sc.Step(`^no function of src/board\.ts has changed since$`, s.unchangedPlace)
	sc.Step(`^"Board#place" changed since the run that recorded it$`, c.changed)
	sc.Step(`^"Board#reset" was added after the last run$`, c.added)
	sc.Step(`^a fresh run recorded a survivor in "Board#place"$`, c.placeSurvivor)
	sc.Step(`^a fresh run recorded every covered mutant of src/board\.ts killed and one uncovered$`, c.uncovered)
	sc.Step(`^only "Board#place" changed since "base", and its fresh results are all killed$`, c.rangeFresh)
	sc.Step(`^"Board#clear", unchanged, has a recorded survivor$`, func() error { return s.unitOutcomes(s.before, s.clear, "survived") })
	sc.Step(`^"Board#size" has no mutation site and no entry in the snapshot$`, c.noSite)
	sc.Step(`^every other function of src/board\.ts has fresh results, all killed$`, c.otherFresh)
	sc.Step(`^the problem is "([^"]*)", with file "([^"]*)" and function "([^"]*)"$`, c.problem)
	sc.Step(`^the problem is "mutation\.survived", with its file, line, column, function, original, and replacement$`, c.survivorProblem)
	sc.Step(`^"itos-cc mutation check --fail-uncovered src/board\.ts" reports it as "mutation\.uncovered" and exits 1$`, c.uncoveredProblem)
	sc.Step(`^each file in "files" has "functions", each with function and state "fresh", "stale" or "missing"$`, c.functionStates)
	sc.Step(`^the problems are those the plain output prints$`, c.plainAgrees)
	sc.Step(`^fresh results for every function of src/board\.ts, all killed$`, c.fresh)
	sc.Step(`^the entry of "Board#place" does not record one of its sites, as when a newer itos-cc adds an operator$`, c.forgetSite)
	sc.Step(`^the entry of "Board#place" does not record one of its sites, and records every other one killed$`, func() error {
		if err := c.fresh(); err != nil {
			return err
		}
		return c.forgetSite()
	})
	sc.Step(`^the problem is "mutation\.stale", with file and function "Board#place", and its message names that site's line, column, original and replacement$`, c.missingProblem)
	sc.Step(`^only that site's mutant runs$`, c.onlyMissingRuns)
	sc.Step(`^the other mutants of "Board#place" are reused$`, c.othersReused)
	sc.Step(`^afterwards "itos-cc mutation check src/board\.ts" exits 0$`, func() error { _, _, err := c.captureCheck(0); return err })
	sc.Step(`^src/board\.test\.ts imports src/board\.ts and src/other\.test\.ts does not$`, c.imports)
	sc.Step(`^\.metrics/mutate/src/board\.ts\.json records the hash of src/board\.test\.ts$`, func() error { return c.testHashes("src/board.test.ts") })
	sc.Step(`^not of src/other\.test\.ts$`, c.notOther)
	sc.Step(`^"Board#place" has not changed since, but src/board\.test\.ts has$`, func() error {
		if err := s.unchangedPlace(); err != nil {
			return err
		}
		return c.changeTest()
	})
	sc.Step(`^src/board\.test\.ts was deleted since$`, func() error { return m.w.d.Remove(filepath.Join(m.w.dir, "src/board.test.ts")) })
	sc.Step(`^no mutant of "Board#place" is reused$`, s.ranPlace)
	sc.Step(`^src/board\.test\.ts changed since they were recorded$`, c.changeTest)
	sc.Step(`^each function of src/board\.ts is a "mutation\.stale" problem whose message names src/board\.test\.ts$`, func() error { return c.allStale("src/board.test.ts") })
	sc.Step(`^\.metrics/mutate/src/board\.ts\.json was written before snapshots recorded tests$`, c.oldSnapshot)
	sc.Step(`^each function of src/board\.ts is a "mutation\.stale" problem$`, func() error { return c.allStale("") })
	sc.Step(`^"itos-cc mutation run src/board\.ts" reuses none of its kills$`, c.noneReused)
	sc.Step(`^src/board\.go is in package board, tested by src/board_test\.go$`, c.goProject)
	sc.Step(`^package app imports board and is tested by app/app_test\.go$`, c.appImports)
	sc.Step(`^\.metrics/mutate/src/board\.go\.json records the hashes of src/board_test\.go and app/app_test\.go$`, func() error { return c.testHashes("src/board_test.go", "app/app_test.go") })
	sc.Step(`^fresh results for "Board#place" and "Board#clear", with a survivor recorded in "Board#clear"$`, c.fresh)
	sc.Step(`^a commit after "base" changed "Board#place" and src/board\.test\.ts$`, func() error {
		if err := c.changeTest(); err != nil {
			return err
		}
		return s.changePlace("base")
	})
	sc.Step(`^\.metrics/mutate/src/board\.ts\.json still holds the entry of "Board#clear" and its survivor, marked stale$`, c.keptStale)
	sc.Step(`^"itos-cc mutation check src/board\.ts" reports "Board#clear" as "mutation\.stale", not "mutation\.missing"$`, c.clearStale)
	sc.Step(`^a later "itos-cc mutation run src/board\.ts" runs the mutants of "Board#clear" again$`, c.laterClear)
}

func (c *checkMutation) project() error {
	s := c.s
	if s.made {
		return nil
	}
	s.tsTests = checkBoardTestTS
	s.tsSurvivors = []string{}
	switch c.id {
	case "60":
		s.tsTests = strings.Replace(s.tsTests, " expect(b.place(5)).toBe(false);\n", "", 1)
		s.tsSurvivors = []string{"Board#place"}
	case "61":
		s.tsTests = s.tsTests[:strings.Index(s.tsTests, `test("clear"`)]
		s.tsUncovered = []string{"Board#clear"}
	case "62", "108":
		s.tsTests = strings.Replace(s.tsTests, " expect(b.clear(3)).toBe(false);\n", "", 1)
		s.tsSurvivors = []string{"Board#clear"}
	case "106", "107":
		s.tsSource = strings.Replace(sinceBoardTS, "i > 5", "i > 5 && i < 9", 1)
		s.tsTests = strings.Replace(s.tsTests, " expect(b.place(6)).toBe(true);", " expect(b.place(6)).toBe(true);\n expect(b.place(9)).toBe(false);", 1)
	}
	if c.id == "57" {
		s.m.w.variables = append(s.m.w.variables, "CHECK_MARKER="+filepath.ToSlash(filepath.Join(s.m.w.dir, "src/test-ran")))
	}
	if err := s.project(true); err != nil {
		return err
	}
	if c.id == "67" {
		return s.m.w.write("src/remaining.test.ts", checkBoardTestTS)
	}
	return nil
}

func (c *checkMutation) fresh() error {
	s := c.s
	if err := c.project(); err != nil {
		return err
	}
	if err := s.previous(); err != nil {
		return err
	}
	for _, name := range []string{s.place, s.clear} {
		want := "killed"
		if (c.id == "62" || c.id == "108") && name == s.clear {
			want = "survived"
		}
		if err := s.unitOutcomes(s.before, name, want); err != nil {
			return err
		}
	}
	if c.id == "57" {
		m := s.m
		m.snapshot = ".metrics/mutate/" + s.file + ".json"
		m.recorded = m.read(m.snapshot)
		for _, name := range []string{"src/test-ran", ".metrics/coverage"} {
			if err := m.w.d.Remove(filepath.Join(m.w.dir, name)); err != nil {
				return err
			}
		}
		m.quietCheck = func() error {
			if m.read(m.snapshot) != m.recorded {
				return fmt.Errorf("mutation check changed the snapshot")
			}
			if m.w.d.Exists(filepath.Join(m.w.dir, ".metrics/coverage")) {
				return fmt.Errorf("mutation check wrote coverage")
			}
			return m.w.stdoutSays("src/board.ts: 2 fresh, 0 stale, 0 missing")
		}
	}
	return nil
}

func (c *checkMutation) changed() error {
	if err := c.fresh(); err != nil {
		return err
	}
	if err := c.s.m.w.write(c.s.file, "// moved down\n"+c.s.m.read(c.s.file)); err != nil {
		return err
	}
	if _, _, err := c.captureCheck(0); err != nil {
		return fmt.Errorf("moving an unchanged function: %w", err)
	}
	return c.s.replace(c.s.file, "// past five", "// past five, edited")
}

func (c *checkMutation) addMethod(method string) error {
	s := c.s
	text := s.m.read(s.file)
	at := strings.LastIndex(text, "}")
	if at < 0 {
		return fmt.Errorf("board class has no closing brace")
	}
	return s.m.w.write(s.file, text[:at]+"  "+method+"\n"+text[at:])
}

func (c *checkMutation) added() error {
	if err := c.project(); err != nil {
		return err
	}
	obj, _, err := c.captureCheck(1)
	if err != nil {
		return err
	}
	if err := c.wantProblems(obj, "mutation.missing", []string{"Board#place", "Board#clear"}, ""); err != nil {
		return fmt.Errorf("no snapshot: %w", err)
	}
	if err := c.fresh(); err != nil {
		return err
	}
	return c.addMethod("reset(i: number): boolean { return i == 0; }")
}

func (c *checkMutation) placeSurvivor() error {
	if err := c.project(); err != nil {
		return err
	}
	if err := c.s.previous(); err != nil {
		return err
	}
	return c.s.unitOutcomes(c.s.before, c.s.place, "survived")
}

func (c *checkMutation) uncovered() error {
	if err := c.project(); err != nil {
		return err
	}
	if err := c.s.previous(); err != nil {
		return err
	}
	if err := c.s.unitOutcomes(c.s.before, c.s.place, "killed"); err != nil {
		return err
	}
	return c.s.unitOutcomes(c.s.before, c.s.clear, "uncovered")
}

func (c *checkMutation) rangeFresh() error {
	if err := c.fresh(); err != nil {
		return err
	}
	if err := c.s.changePlace("base"); err != nil {
		return err
	}
	if err := c.s.m.w.runWith(c.s.m.w.env(), "mutation run --since base"); err != nil {
		return err
	}
	if err := c.s.m.w.exitsWith(0); err != nil {
		return err
	}
	var err error
	c.s.before, err = c.s.snapshot(c.s.file)
	if err != nil {
		return err
	}
	_, _, err = c.captureCheck(1)
	return err
}

func (c *checkMutation) noSite() error {
	if err := c.fresh(); err != nil {
		return err
	}
	if err := c.addMethod("size(): number { return 7; }"); err != nil {
		return err
	}
	sites, err := c.s.functionSites("Board#size")
	if err != nil {
		return err
	}
	if len(sites) != 0 || len(c.s.units(c.s.before, "Board#size")) != 0 {
		return fmt.Errorf("size has sites or a snapshot entry")
	}
	return nil
}

func (c *checkMutation) otherFresh() error {
	_, _, err := c.captureCheck(0)
	return err
}

func (c *checkMutation) captureCheck(code int, flags ...string) (map[string]any, string, error) {
	w := c.s.m.w
	args := append([]string{"mutation", "check", "--json"}, flags...)
	args = append(args, c.s.file)
	got, out, stderr, err := w.capture(w.dir, w.env(), args...)
	if err != nil {
		return nil, "", err
	}
	if got != code {
		return nil, "", fmt.Errorf("mutation check exit %d, want %d\n%s%s", got, code, out, stderr)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		return nil, "", err
	}
	return obj, out, nil
}

func checkProblems(obj map[string]any) []map[string]any {
	var out []map[string]any
	problems, _ := obj["problems"].([]any)
	for _, raw := range problems {
		problem, _ := raw.(map[string]any)
		out = append(out, problem)
	}
	return out
}

func (c *checkMutation) wantProblems(obj map[string]any, rule string, names []string, message string) error {
	problems := checkProblems(obj)
	if len(problems) != len(names) {
		return fmt.Errorf("problems %v, want %s for %v", problems, rule, names)
	}
	for i, p := range problems {
		if p["rule"] != rule || filepath.ToSlash(fmt.Sprint(p["file"])) != c.s.file || !sinceFunction(listedSite{function: fmt.Sprint(p["function"])}, names[i]) || !strings.Contains(fmt.Sprint(p["message"]), message) {
			return fmt.Errorf("problem %v, want %s for %s naming %q", p, rule, names[i], message)
		}
	}
	return nil
}

func (c *checkMutation) problem(rule, file, function string) error {
	obj, _, err := c.s.m.w.lastObject()
	if err != nil {
		return err
	}
	if file != c.s.file {
		return fmt.Errorf("fixture file %s, sentence says %s", c.s.file, file)
	}
	return c.wantProblems(obj, rule, []string{function}, "")
}

func (c *checkMutation) outcomeProblem(obj map[string]any, rule, function, outcome string, shift int) error {
	if err := c.wantProblems(obj, rule, []string{function}, ""); err != nil {
		return err
	}
	problems := checkProblems(obj)
	entries := snapshotEntries(map[string]any{"units": []any{c.s.units(c.s.before, function)[0]}})
	var expected map[string]any
	for _, entry := range entries {
		if entry["outcome"] == outcome {
			expected = entry
		}
	}
	if expected == nil {
		return fmt.Errorf("no recorded %s mutant", outcome)
	}
	for _, key := range []string{"line", "column", "original", "replacement"} {
		want := expected[key]
		if key == "line" {
			want = want.(float64) + float64(shift)
		}
		if problems[0][key] != want {
			return fmt.Errorf("problem %s=%v, want %v", key, problems[0][key], want)
		}
	}
	return nil
}

func (c *checkMutation) survivorProblem() error {
	obj, _, err := c.s.m.w.lastObject()
	if err != nil {
		return err
	}
	if err := c.outcomeProblem(obj, "mutation.survived", c.s.place, "survived", 0); err != nil {
		return err
	}
	// The old holder also guards the new location of an unchanged survivor.
	if err := c.s.m.w.write(c.s.file, "// moved down\n"+c.s.m.read(c.s.file)); err != nil {
		return err
	}
	obj, _, err = c.captureCheck(1)
	if err != nil {
		return err
	}
	return c.outcomeProblem(obj, "mutation.survived", c.s.place, "survived", 1)
}

func (c *checkMutation) uncoveredProblem() error {
	obj, _, err := c.captureCheck(1, "--fail-uncovered")
	if err != nil {
		return err
	}
	// Strict TypeScript obligations may add coverage-inventory problems too;
	// this sentence holds the recorded uncovered mutant and its exact subject.
	var uncovered []any
	for _, p := range checkProblems(obj) {
		if p["rule"] == "mutation.uncovered" {
			uncovered = append(uncovered, p)
		}
	}
	obj["problems"] = uncovered
	return c.outcomeProblem(obj, "mutation.uncovered", c.s.clear, "uncovered", 0)
}

func (c *checkMutation) functionStates() error {
	obj, err := c.s.m.w.json()
	if err != nil {
		return err
	}
	files, _ := obj["files"].([]any)
	if len(files) != 1 {
		return fmt.Errorf("check files %v, want one", files)
	}
	f := files[0].(map[string]any)
	if filepath.ToSlash(fmt.Sprint(f["file"])) != c.s.file {
		return fmt.Errorf("unexpected file %v", f)
	}
	functions, _ := f["functions"].([]any)
	want := []struct {
		name, state string
		killed      float64
	}{{"Board#place", "stale", 1}, {"Board#clear", "fresh", 1}, {"Board#reset", "missing", 0}}
	if len(functions) != len(want) {
		return fmt.Errorf("functions %v, want three states", functions)
	}
	for i, raw := range functions {
		got := raw.(map[string]any)
		if !sinceFunction(listedSite{function: fmt.Sprint(got["function"])}, want[i].name) || got["state"] != want[i].state || got["killed"] != want[i].killed || got["survived"] != float64(0) || got["uncovered"] != float64(0) {
			return fmt.Errorf("function %v, want %+v", got, want[i])
		}
	}
	return nil
}

func (c *checkMutation) plainAgrees() error {
	w := c.s.m.w
	obj, err := w.json()
	if err != nil {
		return err
	}
	code, out, stderr, err := w.capture(w.dir, w.env(), "mutation", "check", c.s.file)
	if err != nil {
		return err
	}
	var got, want []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "  ") {
			got = append(got, filepath.ToSlash(line))
		}
	}
	for _, p := range checkProblems(obj) {
		want = append(want, fmt.Sprintf("  %s %s:%v in %s", strings.TrimPrefix(fmt.Sprint(p["rule"]), "mutation."), filepath.ToSlash(fmt.Sprint(p["file"])), p["line"], p["function"]))
	}
	if len(want) != 2 || !slices.Equal(got, want) || code != 1 || w.exit != 1 {
		return fmt.Errorf("plain problems %q exit %d, JSON %q exit %d\n%s%s", got, code, want, w.exit, out, stderr)
	}
	return nil
}

func (c *checkMutation) forgetSite() error {
	s := c.s
	snap, err := s.snapshot(s.file)
	if err != nil {
		return err
	}
	units := s.units(snap, s.place)
	if len(units) != 1 {
		return fmt.Errorf("place entries %v", units)
	}
	u := units[0]
	mutants := u["mutants"].([]any)
	if len(mutants) < 2 {
		return fmt.Errorf("place needs several sites, got %v", mutants)
	}
	c.missing = mutants[len(mutants)-1].(map[string]any)
	u["mutants"] = mutants[:len(mutants)-1]
	u["sites"] = u["sites"].(float64) - 1
	u["killed"] = u["killed"].(float64) - 1
	return c.writeSnapshot(snap)
}

func (c *checkMutation) writeSnapshot(snap map[string]any) error {
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	return c.s.m.w.write(".metrics/mutate/"+c.s.file+".json", string(data)+"\n")
}

func (c *checkMutation) missingProblem() error {
	obj, _, err := c.s.m.w.lastObject()
	if err != nil {
		return err
	}
	if err := c.wantProblems(obj, "mutation.stale", []string{c.s.place}, ""); err != nil {
		return err
	}
	want := fmt.Sprintf("%v:%v `%v` → `%v`", c.missing["line"], c.missing["column"], c.missing["original"], c.missing["replacement"])
	if !strings.Contains(fmt.Sprint(checkProblems(obj)[0]["message"]), want) {
		return fmt.Errorf("stale problem does not name missing site %s: %v", want, obj)
	}
	return nil
}

func (c *checkMutation) onlyMissingRuns() error {
	s := c.s
	ran := trialsOf(s.m.w.stderr, s.file)
	if len(ran) != 1 || float64(ran[0].line) != c.missing["line"] || ran[0].original != c.missing["original"] || ran[0].replacement != c.missing["replacement"] || ran[0].outcome != "killed" {
		return fmt.Errorf("trials %v, want only missing site %v killed\n%s", ran, c.missing, s.m.w.report())
	}
	return nil
}

func (c *checkMutation) othersReused() error {
	s := c.s
	entries := snapshotEntries(map[string]any{"units": []any{s.units(s.before, s.place)[0]}})
	sum, ok := summaryOf(s.m.w.stdout, s.file)
	// clear's one recorded kill is reused too.
	if !ok || sum.ran != 1 || sum.reused != len(entries) {
		return fmt.Errorf("summary %+v, want one trial and %d reused\n%s", sum, len(entries), s.m.w.report())
	}
	snap, err := s.snapshot(s.file)
	if err != nil {
		return err
	}
	return s.unitOutcomes(snap, s.place, "killed")
}

func (c *checkMutation) imports() error {
	if err := c.project(); err != nil {
		return err
	}
	return c.s.m.writeAll(map[string]string{
		"src/other.ts":      "export function other(i: number) { return i > 5; }\n",
		"src/other.test.ts": "import {expect,test} from 'vitest';\nimport {other} from './other';\ntest('other',()=>expect(other(6)).toBe(true));\n",
	})
}

func (c *checkMutation) testHashes(names ...string) error {
	snap, err := c.s.snapshot(c.s.file)
	if err != nil {
		return err
	}
	got, _ := snap["tests"].(map[string]any)
	want := map[string]any{}
	for _, name := range names {
		want[name] = fmt.Sprintf("%x", sha256.Sum256([]byte(c.s.m.read(name))))
	}
	if !reflect.DeepEqual(got, want) {
		return fmt.Errorf("recorded test hashes %v, want %v", got, want)
	}
	return nil
}

func (c *checkMutation) notOther() error {
	snap, err := c.s.snapshot(c.s.file)
	if err != nil {
		return err
	}
	got, _ := snap["tests"].(map[string]any)
	if _, exists := got["src/other.test.ts"]; exists {
		return fmt.Errorf("snapshot records unrelated test: %v", got)
	}
	return nil
}

func (c *checkMutation) changeTest() error {
	return c.s.m.w.write("src/board.test.ts", c.s.m.read("src/board.test.ts")+"\n// test changed\n")
}

func (c *checkMutation) allStale(message string) error {
	obj, _, err := c.s.m.w.lastObject()
	if err != nil {
		return err
	}
	return c.wantProblems(obj, "mutation.stale", []string{c.s.place, c.s.clear}, message)
}

func (c *checkMutation) oldSnapshot() error {
	if err := c.fresh(); err != nil {
		return err
	}
	snap, err := c.s.snapshot(c.s.file)
	if err != nil {
		return err
	}
	delete(snap, "tests")
	return c.writeSnapshot(snap)
}

func (c *checkMutation) noneReused() error {
	s := c.s
	if err := s.m.w.runWith(s.m.w.env(), "mutation run src/board.ts"); err != nil {
		return err
	}
	return s.ranEvery()
}

func (c *checkMutation) goProject() error {
	return c.s.project(false)
}

func (c *checkMutation) appImports() error {
	return c.s.m.writeAll(map[string]string{
		"app/app.go":      "package app\nimport board \"example.com/m/src\"\nfunc New() *board.Board { return &board.Board{} }\n",
		"app/app_test.go": "package app\nimport \"testing\"\nfunc TestNew(t *testing.T) { if New()==nil { t.Fatal(\"no board\") } }\n",
	})
}

func (c *checkMutation) keptStale() error {
	s := c.s
	snap, err := s.snapshot(s.file)
	if err != nil {
		return err
	}
	if err := s.unitOutcomes(snap, s.clear, "survived"); err != nil {
		return err
	}
	clear, place := s.units(snap, s.clear), s.units(snap, s.place)
	if len(clear) != 1 || len(place) != 1 || clear[0]["stale"] != true || place[0]["stale"] != nil {
		return fmt.Errorf("stale marks clear=%v place=%v", clear, place)
	}
	before := s.units(s.before, s.clear)[0]
	for _, key := range []string{"mutants", "survived", "sites", "hash"} {
		if !reflect.DeepEqual(clear[0][key], before[key]) {
			return fmt.Errorf("clear's %s changed: %v, before %v", key, clear[0][key], before[key])
		}
	}
	return nil
}

func (c *checkMutation) clearStale() error {
	obj, _, err := c.captureCheck(1)
	if err != nil {
		return err
	}
	return c.wantProblems(obj, "mutation.stale", []string{c.s.clear}, "tests")
}

func (c *checkMutation) laterClear() error {
	s := c.s
	if err := s.m.w.runWith(s.m.w.env(), "mutation run src/board.ts"); err != nil {
		return err
	}
	if err := s.ranFunction(s.clear); err != nil {
		return err
	}
	snap, err := s.snapshot(s.file)
	if err != nil {
		return err
	}
	units := s.units(snap, s.clear)
	if len(units) != 1 || units[0]["stale"] != nil {
		return fmt.Errorf("clear still marked stale: %v", units)
	}
	return nil
}
