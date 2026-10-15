// Differential and commit-range scenarios exercise the public mutation commands.
package features

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

type sinceMutation struct {
	m                  *mutateState
	made, ts, annotate bool
	file, place, clear string
	sourceBefore       string
	before             map[string]any
	held               map[string]string
	observe            func() ([]string, error)
	// Optional TypeScript fixture text and declared outcomes, shared by cached
	// result scenarios that use these same previous-run and rerun sentences.
	tsSource, tsTests        string
	tsReport                 string
	tsSurvivors, tsUncovered []string
}

func sinceMutationSteps(sc *godog.ScenarioContext, m *mutateState) *sinceMutation {
	s := &sinceMutation{m: m}
	sc.Before(func(ctx context.Context, scenario *godog.Scenario) (context.Context, error) {
		*s = sinceMutation{m: m}
		for _, tag := range scenario.Tags {
			switch tag.Name {
			case "@ID-MUT-27":
				if err := s.project(false); err != nil {
					return ctx, err
				}
				return ctx, s.previous()
			case "@ID-MUT-43", "@ID-MUT-45":
				return ctx, s.project(false)
			case "@ID-MUT-46":
				if err := s.project(true); err != nil {
					return ctx, err
				}
				return ctx, s.changePlace("base")
			}
		}
		return ctx, nil
	})
	sc.Step(`^a previous run killed every mutant of "Board#place"$`, s.killedBefore)
	sc.Step(`^"Board#place" has not changed since$`, s.unchangedPlace)
	sc.Step(`^"Board#place" changed since the previous run$`, s.changedBefore)
	sc.Step(`^an import was added above "f" so it moved down the file$`, s.movedFunction)
	sc.Step(`^a mutant of "Board#place" survived the previous run$`, s.survivedBefore)
	sc.Step(`^those mutants are reused without running$`, s.reusedPlace)
	sc.Step(`^every mutant of "Board#place" runs again$`, s.ranPlace)
	sc.Step(`^the killed mutants of "f" are still reused$`, s.reusedPlace)
	sc.Step(`^it runs again, since new tests may kill it$`, s.ranPlace)
	sc.Step(`^killed mutants of unchanged functions run again too$`, s.ranEvery)
	sc.Step(`^a commit after "base" changed lines of "Board#place" in src/board\.ts$`, func() error {
		if err := s.project(true); err != nil {
			return err
		}
		return s.changePlace("base")
	})
	sc.Step(`^"Board#clear" in the same file did not change$`, s.clearUnchanged)
	sc.Step(`^the mutants of "Board#place" run$`, s.ranPlace)
	sc.Step(`^no mutant of "Board#clear" runs$`, s.clearDidNotRun)
	sc.Step(`^a previous run recorded a survivor in "Board#clear"$`, func() error {
		if err := s.project(true); err != nil {
			return err
		}
		return s.previous()
	})
	sc.Step(`^only "Board#place" changed since "base"$`, func() error { return s.changePlace("base") })
	sc.Step(`^the snapshot of src/board\.ts still records that survivor in "Board#clear"$`, s.clearKept)
	sc.Step(`^it is not reported, and the exit code is 0 when every mutant of "Board#place" is killed$`, s.survivorNotReported)
	sc.Step(`^"Board#place" changed since "base" in a commit$`, func() error {
		if err := s.project(true); err != nil {
			return err
		}
		return s.changePlace("base")
	})
	sc.Step(`^"Board#clear" has an uncommitted change$`, func() error { return s.replace(s.file, "i < 3", "i < 4") })
	sc.Step(`^"Board#place" is judged$`, func() error { return s.judgedIs([]string{s.place}) })
	sc.Step(`^"Board#clear" is not$`, s.clearNotJudged)
	sc.Step(`^a commit after "base" only deleted a line inside "Board#place"$`, s.deletedLine)
	sc.Step(`^commits after "base" changed src/board\.ts and lib/util\.ts$`, s.twoChanged)
	sc.Step(`^only src/board\.ts is mutated$`, s.onlyBoard)
	sc.Step(`^no commit after "base" changed a source file$`, s.noSourceChange)
	sc.Step(`^the problem is "([^"]*)", with ref "([^"]*)"$`, func(rule, ref string) error { return m.w.problemIs(rule, "ref", ref) })
	sc.Step(`^each file in "files" has "judged", the namespace#name of each function judged$`, s.judgedJSON)
	sc.Step(`^without --since no file has "judged"$`, s.noJudgedKey)
	sc.Step(`^a commit after "base" changed only the imports of src/board\.ts$`, s.importOnly)
	sc.Step(`^neither \.metrics/mutate/src/board\.ts\.json nor src/board\.ts is written$`, s.notWritten)
	sc.Step(`^fresh results for every function of src/board\.go, all killed$`, s.renameProject)
	sc.Step(`^a commit after "base" renamed src/board\.go to src/grid\.go without changing it$`, func() error { return s.rename(false) })
	sc.Step(`^a commit after "base" renamed src/board\.go to src/grid\.go and changed "Board#Place"$`, func() error { return s.rename(true) })
	sc.Step(`^no function is judged$`, func() error { return s.judgedIs(nil) })
	sc.Step(`^\.metrics/mutate/src/grid\.go\.json holds the results \.metrics/mutate/src/board\.go\.json held, under its new path$`, s.renameKept)
	sc.Step(`^\.metrics/mutate/src/board\.go\.json is gone$`, s.oldGone)
	sc.Step(`^only "Board#Place" is judged$`, func() error { return s.judgedIs([]string{s.place}) })
	sc.Step(`^the other functions' results are kept in \.metrics/mutate/src/grid\.go\.json$`, s.renameOtherKept)
	return s
}

const sinceBoardTS = `export class Board {
  place(i: number): boolean {
    // past five
    return i > 5;
  }
  clear(i: number): boolean {
    return i < 3;
  }
}
`
const sinceBoardTestTS = `import {expect, test} from "vitest";
import {Board} from "./board";

test("place", () => {
 const b = new Board();
 expect(b.place(5)).toBe(false);
 expect(b.place(6)).toBe(true);
});
test("clear", () => { expect(new Board().clear(2)).toBe(true); });
`

func (s *sinceMutation) project(ts bool) error {
	if s.made {
		return nil
	}
	s.made = true
	s.ts = ts
	s.clear = "Board#clear"
	s.place = "Board#place"
	w := s.m.w
	w.prepare = func(args []string) []string {
		i := slices.Index(args, "mutation")
		if i < 0 || i+1 == len(args) || args[i+1] != "run" {
			return args
		}
		has := func(flag string) bool {
			return slices.ContainsFunc(args, func(arg string) bool { return arg == flag || strings.HasPrefix(arg, flag+"=") })
		}
		if !has("--workers") {
			args = append(args, "--workers", "2")
		}
		if !s.annotate && !has("--no-annotate") {
			args = append(args, "--no-annotate")
		}
		return args
	}
	files := map[string]string{}
	if ts {
		s.file = "src/board.ts"
		source, tests := s.tsSource, s.tsTests
		if source == "" {
			source = sinceBoardTS
		}
		if tests == "" {
			tests = sinceBoardTestTS
		}
		files = map[string]string{"package.json": vitestPackage, s.file: source, "src/board.test.ts": tests}
	}
	if !ts {
		s.file = "src/board.go"
		files = map[string]string{"go.mod": mutateModule, s.file: boardGo, "src/board_test.go": placeTest + clearTest}
	}
	s.m.file = s.file
	if err := s.m.writeAll(files); err != nil {
		return err
	}
	if err := s.declare(); err != nil {
		return err
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"config", "maintenance.auto", "false"}, {"config", "gc.auto", "0"}, {"add", "-A"}, {"commit", "-qm", "base"}, {"tag", "base"}} {
		if err := w.git(args...); err != nil {
			return err
		}
	}
	return nil
}
func (s *sinceMutation) declare() error {
	language, command := "go", "go test -count=1 -failfast ./src"
	report := "SF:" + s.file + "\n"
	if s.ts {
		language, command = "typescript", vitestCommand+s.file
	}
	source := s.m.read(s.file)
	uncovered := map[int]bool{}
	for _, name := range s.tsUncovered {
		sites, err := s.functionSites(name)
		if err != nil {
			return err
		}
		for _, site := range sites {
			uncovered[site.line] = true
		}
	}
	for i, line := range strings.Split(source, "\n") {
		if strings.Contains(line, "return ") {
			hits := 1
			if uncovered[i+1] {
				hits = 0
			}
			report += fmt.Sprintf("DA:%d,%d\n", i+1, hits)
		}
	}
	report += "end_of_record\n"
	if s.tsReport != "" {
		report = s.tsReport
	}
	tests := Tests{Language: language, Command: command}
	if s.ts {
		names := s.tsSurvivors
		if names == nil {
			names = []string{s.clear}
		}
		for _, name := range names {
			sites, err := s.functionSites(name)
			if err != nil {
				return err
			}
			for _, site := range sites {
				tests.Surviving = append(tests.Surviving, Site{File: site.file, Line: site.line, Original: site.original, Replacement: site.replacement})
			}
		}
	}
	if err := s.m.w.d.Coverage(s.m.w.dir, Coverage{Language: language, Report: report}); err != nil {
		return err
	}
	return s.m.w.d.Tests(s.m.w.dir, tests)
}
func (s *sinceMutation) replace(file, old, new string) error {
	text := s.m.read(file)
	if !strings.Contains(text, old) {
		return fmt.Errorf("%s lacks %q", file, old)
	}
	return s.m.w.write(file, strings.Replace(text, old, new, 1))
}
func (s *sinceMutation) commit(message string) error {
	if err := s.m.w.git("add", "-A"); err != nil {
		return err
	}
	return s.m.w.git("commit", "-qm", message)
}
func (s *sinceMutation) previous() error {
	if err := s.m.w.runWith(s.m.w.env(), "mutation run --json"); err != nil {
		return err
	}
	snap, err := s.snapshot(s.file)
	if err != nil {
		return err
	}
	s.before = snap
	s.sourceBefore = s.m.read(s.file)
	return nil
}
func (s *sinceMutation) snapshot(file string) (map[string]any, error) {
	data, err := s.m.w.readIn(s.m.w.dir, ".metrics/mutate/"+file+".json")
	if err != nil {
		return nil, err
	}
	var snap map[string]any
	err = json.Unmarshal(data, &snap)
	return snap, err
}
func (s *sinceMutation) units(snap map[string]any, name string) []map[string]any {
	var out []map[string]any
	units, _ := snap["units"].([]any)
	for _, one := range units {
		u := one.(map[string]any)
		if sinceFunction(listedSite{function: fmt.Sprint(u["namespace"]) + "#" + fmt.Sprint(u["name"])}, name) {
			out = append(out, u)
		}
	}
	return out
}
func (s *sinceMutation) unitOutcomes(snap map[string]any, name, outcome string) error {
	units := s.units(snap, name)
	if len(units) != 1 {
		return fmt.Errorf("%s has %d entries", name, len(units))
	}
	entries := snapshotEntries(map[string]any{"units": []any{units[0]}})
	if len(entries) == 0 {
		return fmt.Errorf("no recorded mutant for %s", name)
	}
	for _, e := range entries {
		if e["outcome"] != outcome {
			return fmt.Errorf("%s recorded %v, want %s", name, e, outcome)
		}
	}
	return nil
}
func (s *sinceMutation) killedBefore() error {
	if err := s.project(false); err != nil {
		return err
	}
	if err := s.previous(); err != nil {
		return err
	}
	return s.unitOutcomes(s.before, s.place, "killed")
}
func (s *sinceMutation) unchangedPlace() error {
	if s.m.read(s.file) != s.sourceBefore {
		return fmt.Errorf("place source changed since the previous run")
	}
	snap, err := s.snapshot(s.file)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(s.units(snap, s.place), s.units(s.before, s.place)) {
		return fmt.Errorf("place's snapshot changed")
	}
	return nil
}
func (s *sinceMutation) changedBefore() error {
	if err := s.killedBefore(); err != nil {
		return err
	}
	return s.replace(s.file, "// past five", "// past five, edited")
}
func (s *sinceMutation) movedFunction() error {
	if err := s.project(false); err != nil {
		return err
	}
	if err := s.m.w.write(s.file, "package board\n\nfunc f(i int) bool { return i > 2 }\n"); err != nil {
		return err
	}
	if err := s.m.w.write("src/board_test.go", "package board\nimport \"testing\"\nfunc TestF(t *testing.T) { if f(2)||!f(3){t.Fatal(\"f\")} }\n"); err != nil {
		return err
	}
	s.place = "example.com/m/src#f"
	if err := s.declare(); err != nil {
		return err
	}
	if err := s.previous(); err != nil {
		return err
	}
	if err := s.unitOutcomes(s.before, s.place, "killed"); err != nil {
		return err
	}
	return s.replace(s.file, "package board\n", "package board\n\nimport _ \"fmt\"\n")
}
func (s *sinceMutation) survivedBefore() error {
	if err := s.project(false); err != nil {
		return err
	}
	if err := s.replace("src/board_test.go", "b.place(5) || !b.place(6)", "!b.place(6)"); err != nil {
		return err
	}
	sites, err := s.m.sitesWhere(s.file, func(site listedSite) bool { return sinceFunction(site, s.place) })
	if err != nil {
		return err
	}
	tests := goTests
	tests.Surviving = nil
	for _, site := range sites {
		tests.Surviving = append(tests.Surviving, Site{File: site.file, Line: site.line, Original: site.original, Replacement: site.replacement})
	}
	if err := s.m.w.d.Tests(s.m.w.dir, tests); err != nil {
		return err
	}
	if err := s.previous(); err != nil {
		return err
	}
	return s.unitOutcomes(s.before, s.place, "survived")
}
func (s *sinceMutation) functionSites(name string) ([]listedSite, error) {
	return s.m.sitesWhere(s.file, func(site listedSite) bool { return sinceFunction(site, name) })
}
func (s *sinceMutation) ranPlace() error { return s.ranFunction(s.place) }

func (s *sinceMutation) ranEvery() error {
	for _, name := range []string{s.place, s.clear} {
		if err := s.ranFunction(name); err != nil {
			return err
		}
	}
	return nil
}

func (s *sinceMutation) ranFunction(name string) error {
	sites, err := s.functionSites(name)
	if err != nil {
		return err
	}
	ran := trialsOf(s.m.w.stderr, s.file)
	for _, site := range sites {
		if !slices.ContainsFunc(ran, func(t trial) bool {
			return t.line == site.line && t.original == site.original && t.replacement == site.replacement
		}) {
			return fmt.Errorf("%s never ran\n%s", site, s.m.w.report())
		}
	}
	if len(sites) == 0 {
		return fmt.Errorf("no sites of %s", name)
	}
	return nil
}
func (s *sinceMutation) reusedPlace() error {
	sites, err := s.functionSites(s.place)
	if err != nil {
		return err
	}
	for _, trial := range trialsOf(s.m.w.stderr, s.file) {
		for _, site := range sites {
			if trial.line == site.line {
				return fmt.Errorf("reused function ran: %v", trial)
			}
		}
	}
	obj, _, err := s.m.w.lastObject()
	if err != nil {
		return err
	}
	files, _ := obj["files"].([]any)
	n := 0
	for _, one := range files {
		f := one.(map[string]any)
		entries, _ := f["mutants"].([]any)
		for _, v := range entries {
			e := v.(map[string]any)
			if sinceFunction(listedSite{function: fmt.Sprint(e["function"])}, s.place) {
				n++
				if e["reused"] != true {
					return fmt.Errorf("not reused: %v", e)
				}
			}
		}
	}
	if n == 0 {
		return fmt.Errorf("no reused mutant of %s\n%s", s.place, s.m.w.report())
	}
	return nil
}
func (s *sinceMutation) changePlace(ref string) error {
	if ref != "base" {
		return fmt.Errorf("unknown fixture ref %s", ref)
	}
	if err := s.replace(s.file, "// past five", "// past five, changed in a commit"); err != nil {
		return err
	}
	return s.commit("change place")
}
func (s *sinceMutation) clearUnchanged() error {
	if !strings.Contains(s.m.read(s.file), "return i < 3") {
		return fmt.Errorf("clear changed")
	}
	return nil
}
func (s *sinceMutation) clearDidNotRun() error {
	sites, err := s.functionSites(s.clear)
	if err != nil {
		return err
	}
	if len(sites) == 0 {
		return fmt.Errorf("no clear sites to check")
	}
	for _, t := range trialsOf(s.m.w.stderr, s.file) {
		if slices.ContainsFunc(sites, func(site listedSite) bool { return site.line == t.line }) {
			return fmt.Errorf("clear ran: %v", t)
		}
	}
	return nil
}

func (s *sinceMutation) clearKept() error {
	snap, err := s.snapshot(s.file)
	if err != nil {
		return err
	}
	if err := s.unitOutcomes(snap, s.clear, "survived"); err != nil {
		return err
	}
	if !reflect.DeepEqual(s.units(snap, s.clear), s.units(s.before, s.clear)) {
		return fmt.Errorf("clear's recorded survivor changed")
	}
	return nil
}
func (s *sinceMutation) survivorNotReported() error {
	if strings.Contains(s.m.w.stdout, "survived "+s.file) || strings.Contains(s.m.w.stdout, "#clear") {
		return fmt.Errorf("unjudged survivor reported\n%s", s.m.w.report())
	}
	return s.m.w.exitsWith(0)
}
func (s *sinceMutation) judged() ([]string, error) {
	obj, _, err := s.m.w.lastObject()
	if err != nil {
		return nil, err
	}
	files, _ := obj["files"].([]any)
	var names []string
	found := false
	for _, one := range files {
		f := one.(map[string]any)
		if filepath.ToSlash(fmt.Sprint(f["file"])) != s.file {
			continue
		}
		found = true
		raw, exists := f["judged"]
		if !exists {
			return nil, fmt.Errorf("no judged key: %v", f)
		}
		for _, v := range raw.([]any) {
			names = append(names, fmt.Sprint(v))
		}
	}
	if !found {
		return nil, fmt.Errorf("no judged file %s", s.file)
	}
	return names, nil
}
func (s *sinceMutation) judgedIs(want []string) error {
	names, err := s.judged()
	if err != nil {
		return err
	}
	if len(names) != len(want) {
		return fmt.Errorf("judged %v, want %v", names, want)
	}
	for i, name := range names {
		if !sinceFunction(listedSite{function: name}, want[i]) {
			return fmt.Errorf("judged %v, want %v", names, want)
		}
	}
	return nil
}
func (s *sinceMutation) clearNotJudged() error {
	names, err := s.judged()
	if err != nil {
		return err
	}
	for _, name := range names {
		if sinceFunction(listedSite{function: name}, s.clear) {
			return fmt.Errorf("clear is judged: %v", names)
		}
	}
	return nil
}
func (s *sinceMutation) deletedLine() error {
	if err := s.project(true); err != nil {
		return err
	}
	if err := s.replace(s.file, "    // past five\n", ""); err != nil {
		return err
	}
	return s.commit("delete a line in place")
}
func (s *sinceMutation) twoChanged() error {
	if err := s.project(true); err != nil {
		return err
	}
	for name, text := range map[string]string{"lib/util.ts": "export function half(i: number) { return i > 2; }\n", "src/rest.ts": "export function rest(i: number) { return i > 3; }\n"} {
		if err := s.m.w.write(name, text); err != nil {
			return err
		}
	}
	if err := s.commit("add utility and unchanged rest"); err != nil {
		return err
	}
	if err := s.m.w.git("tag", "-f", "base"); err != nil {
		return err
	}
	if err := s.replace("lib/util.ts", "i > 2", "i > 3"); err != nil {
		return err
	}
	return s.changePlace("base")
}
func (s *sinceMutation) onlyBoard() error {
	obj, _, err := s.m.w.lastObject()
	if err != nil {
		return err
	}
	files, _ := obj["files"].([]any)
	if len(files) != 1 || filepath.ToSlash(fmt.Sprint(files[0].(map[string]any)["file"])) != "src/board.ts" {
		return fmt.Errorf("files %v, want only src/board.ts", files)
	}
	return s.ranPlace()
}
func (s *sinceMutation) noSourceChange() error {
	if err := s.project(false); err != nil {
		return err
	}
	if err := s.m.w.write("README.md", "# Board\n"); err != nil {
		return err
	}
	return s.commit("describe board")
}
func (s *sinceMutation) judgedJSON() error {
	object, _, err := s.m.w.lastObject()
	if err != nil {
		return err
	}
	files, _ := object["files"].([]any)
	if len(files) != 1 {
		return fmt.Errorf("judged files %v, want the fixture's one source", files)
	}
	return s.judgedIs([]string{s.place})
}
func (s *sinceMutation) noJudgedKey() error {
	code, out, stderr, err := s.m.w.capture(s.m.w.dir, s.m.w.env(), "mutation", "run", "--json")
	if err != nil {
		return err
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		return err
	}
	files, _ := obj["files"].([]any)
	if len(files) == 0 {
		return fmt.Errorf("no files without since: %d\n%s%s", code, out, stderr)
	}
	for _, one := range files {
		if _, ok := one.(map[string]any)["judged"]; ok {
			return fmt.Errorf("judged key without since: %v", one)
		}
	}
	return nil
}
func (s *sinceMutation) importOnly() error {
	if err := s.project(true); err != nil {
		return err
	}
	s.annotate = true
	if err := s.previous(); err != nil {
		return err
	}
	if err := s.replace(s.file, "export class Board", `import type { Stats } from "node:fs";

export class Board`); err != nil {
		return err
	}
	if err := s.commit("change only imports"); err != nil {
		return err
	}
	s.held = map[string]string{}
	paths := []string{s.file, ".metrics/mutate/" + s.file + ".json"}
	var abs []string
	for _, name := range paths {
		s.held[name] = s.m.read(name)
		abs = append(abs, filepath.Join(s.m.w.dir, filepath.FromSlash(name)))
	}
	var err error
	s.observe, err = s.m.w.d.ObserveWrites(abs...)
	return err
}
func (s *sinceMutation) notWritten() error {
	for name, want := range s.held {
		if s.m.read(name) != want {
			return fmt.Errorf("%s changed bytes", name)
		}
	}
	written, err := s.observe()
	if err != nil {
		return err
	}
	if len(written) != 0 {
		return fmt.Errorf("files written: %v", written)
	}
	return nil
}
func (s *sinceMutation) renameProject() error {
	if err := s.project(false); err != nil {
		return err
	}
	s.place = "Board#Place"
	if err := s.replace(s.file, "place(", "Place("); err != nil {
		return err
	}
	if err := s.replace("src/board_test.go", "b.place(5) || !b.place(6)", "b.Place(5) || !b.Place(6)"); err != nil {
		return err
	}
	if err := s.declare(); err != nil {
		return err
	}
	if err := s.previous(); err != nil {
		return err
	}
	for _, fn := range []string{s.place, s.clear} {
		if err := s.unitOutcomes(s.before, fn, "killed"); err != nil {
			return err
		}
	}
	if err := s.commit("record fresh results"); err != nil {
		return err
	}
	return s.m.w.git("tag", "-f", "base")
}
func (s *sinceMutation) rename(edit bool) error {
	if err := s.m.w.git("mv", "src/board.go", "src/grid.go"); err != nil {
		return err
	}
	s.file = "src/grid.go"
	s.m.file = s.file
	if edit {
		if err := s.replace(s.file, "// past five", "// past five, changed"); err != nil {
			return err
		}
	}
	if err := s.commit("rename board to grid"); err != nil {
		return err
	}
	return s.declare()
}
func (s *sinceMutation) renameKept() error {
	snap, err := s.snapshot(s.file)
	if err != nil {
		return err
	}
	if snap["file"] != "src/grid.go" || !reflect.DeepEqual(snap["units"], s.before["units"]) || !reflect.DeepEqual(snap["tests"], s.before["tests"]) {
		return fmt.Errorf("renamed snapshot differs from original results: %v", snap)
	}
	return nil
}
func (s *sinceMutation) oldGone() error {
	if s.m.w.d.Exists(filepath.Join(s.m.w.dir, ".metrics", "mutate", "src", "board.go.json")) {
		return fmt.Errorf("old board snapshot remains")
	}
	return nil
}
func (s *sinceMutation) renameOtherKept() error {
	snap, err := s.snapshot(s.file)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(s.units(snap, s.clear), s.units(s.before, s.clear)) {
		return fmt.Errorf("clear's results changed during rename")
	}
	if err := s.unitOutcomes(snap, s.place, "killed"); err != nil {
		return err
	}
	return s.oldGone()
}

func sinceFunction(site listedSite, name string) bool {
	return site.function == name || ofFunction(site, name)
}
