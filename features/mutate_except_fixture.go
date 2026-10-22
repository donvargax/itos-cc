package features

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

func (e *exceptionMutation) project() error {
	source := "export class Board {\nplace(i) {\n// past five\n// site\nreturn i>5;\n}\ncount(x){ return x<5; }\n"
	tests := "import {expect,test} from \"vitest\";\nimport {Board} from \"./board\";\n" +
		"test(\"place\",()=>{const b=new Board();expect(b.place(5)).toBe(false);expect(b.place(6)).toBe(true)});\n" +
		"test(\"count\",()=>{const b=new Board();expect(b.count(4)).toBe(true);expect(b.count(6)).toBe(false)});\n"
	for i := 1; i <= 13; i++ {
		source += fmt.Sprintf("k%d(i) { return i>5; }\n", i)
		tests += fmt.Sprintf("test(\"k%d\",()=>{const b=new Board();expect(b.k%d(5)).toBe(false);expect(b.k%d(6)).toBe(true)});\n", i, i, i)
	}
	source += "}\n"
	if e.id == "144" || e.id == "146" {
		tests = strings.Replace(tests, "import {Board} from \"./board\";", "const modules=import.meta.glob([\"./*.ts\",\"!./*.test.ts\"],{eager:true});\nconst {Board}=modules[\"./board.ts\"]??modules[\"./grid.ts\"];", 1)
	}
	if err := e.r.ts("src/board.ts", source, tests, []string{"Board#count"}, nil); err != nil {
		return err
	}
	e.r.file = e.s.file
	return nil
}
func (e *exceptionMutation) record() error {
	if e.recorded {
		return nil
	}
	if err := e.run("mutation run --json src/board.ts"); err != nil {
		return err
	}
	a, err := e.object()
	if err != nil {
		return err
	}
	files, _ := a["files"].([]any)
	if len(files) != 1 {
		return fmt.Errorf("record files: %v", a)
	}
	f := files[0].(map[string]any)
	if err := exceptionFields(f, map[string]any{"killed": float64(14), "survived": float64(1), "uncovered": float64(0), "baseline": "passed"}); err != nil {
		return err
	}
	if e.s.m.w.exit != 1 {
		return fmt.Errorf("survivor record exit %d", e.s.m.w.exit)
	}
	e.recorded = true
	e.s.before, err = e.s.snapshot("src/board.ts")
	e.s.sourceBefore = e.s.m.read("src/board.ts")
	e.testBefore = e.s.m.read("src/board.test.ts")
	return err
}
func (e *exceptionMutation) review(reason string) error {
	if e.reviewed {
		return nil
	}
	if err := e.record(); err != nil {
		return err
	}
	e.reason = reason
	// The fixture's reason is data, quoted as one CLI argument, including its newline.
	quoted := "'" + strings.ReplaceAll(reason, "'", `'\''`) + "'"
	if err := e.run("mutation except src/board.ts:7:19 --reason " + quoted); err != nil {
		return err
	}
	if e.s.m.w.exit != 0 {
		return fmt.Errorf("review exit%d\n%s", e.s.m.w.exit, e.s.m.w.report())
	}
	a, err := e.entries()
	if err != nil {
		return err
	}
	if len(a) != 1 {
		return fmt.Errorf("review entries %v", a)
	}
	e.entry = a[0]
	e.written = e.s.m.read("itos-cc.yaml")
	e.reviewed = true
	return nil
}
func (e *exceptionMutation) reviewAndForget() error {
	if err := e.review(e.reason); err != nil {
		return err
	}
	return e.s.m.w.d.Remove(filepath.Join(e.s.m.w.dir, ".metrics/mutate/src/board.ts.json"))
}
func (e *exceptionMutation) recordExcepted() error {
	if err := e.review(e.reason); err != nil {
		return err
	}
	if err := e.run("mutation run --json src/board.ts"); err != nil {
		return err
	}
	return e.counted()
}
func (e *exceptionMutation) unchanged() error {
	if e.s.m.read("src/board.ts") != e.s.sourceBefore {
		return fmt.Errorf("source changed")
	}
	if e.s.m.read("src/board.test.ts") != e.testBefore {
		return fmt.Errorf("importing test changed")
	}
	snap, err := e.s.snapshot("src/board.ts")
	if err != nil {
		return err
	}
	before, err := json.Marshal(e.s.before["tests"])
	if err != nil {
		return err
	}
	after, err := json.Marshal(snap["tests"])
	if err != nil {
		return err
	}
	if string(before) != string(after) {
		return fmt.Errorf("recorded importing tests changed: %s, %s", before, after)
	}
	return nil
}
func (e *exceptionMutation) changeTests(kill bool) error {
	test := "\n// changed importing test\n"
	if kill {
		test += "test(\"count boundary\",()=>expect(new Board().count(5)).toBe(false));\n"
	}
	if err := e.s.m.w.write("src/board.test.ts", e.s.m.read("src/board.test.ts")+test); err != nil {
		return err
	}
	return e.declare("src/board.ts", !kill)
}
func (e *exceptionMutation) declare(file string, survive bool) error {
	w := e.s.m.w
	sites, err := e.s.m.sitesOf(file)
	if err != nil {
		return err
	}
	report := "SF:" + file + "\n"
	t := Tests{Language: "typescript"}
	for _, site := range sites {
		hit := 1
		if e.id == "86" || e.id == "95" {
			if site.function == "board.Board#clear" {
				hit = 0
			}
		}
		report += fmt.Sprintf("DA:%d,%d\n", site.line, hit)
		if survive && sinceFunction(site, "Board#count") {
			t.Surviving = append(t.Surviving, Site{File: file, Line: site.line, Original: site.original, Replacement: site.replacement})
		}
	}
	report += "end_of_record\n"
	// Coverage owns the fixture's tool link. Replace that link before
	// updating its declaration; a second creation must not become a skip.
	if err := w.d.Remove(filepath.Join(w.dir, "node_modules")); err != nil {
		return err
	}
	if err := w.d.Coverage(w.dir, Coverage{Language: "typescript", Report: report}); err != nil {
		return err
	}
	for _, command := range []string{vitestCommand + file, vitestCommand + filepath.ToSlash(filepath.Join(w.dir, file)), filepath.ToSlash(filepath.Join(w.dir, vitestCommand)) + file} {
		t.Command = command
		if err := w.d.Tests(w.dir, t); err != nil {
			return err
		}
	}
	return nil
}
func (e *exceptionMutation) changeCount() error {
	return e.s.replace("src/board.ts", "count(x){ return x<5; }", "count(x){ return x<5 }")
}
func (e *exceptionMutation) deleteCount() error {
	if err := e.s.replace("src/board.ts", "count(x){ return x<5; }\n", ""); err != nil {
		return err
	}
	text := e.s.m.read("src/board.test.ts")
	lines := strings.Split(text, "\n")
	var keep []string
	for _, line := range lines {
		if !strings.HasPrefix(line, "test(\"count\",") {
			keep = append(keep, line)
		}
	}
	if err := e.s.m.w.write("src/board.test.ts", strings.Join(keep, "\n")); err != nil {
		return err
	}
	return e.declare("src/board.ts", false)
}
func (e *exceptionMutation) commented() error {
	if err := e.review("first"); err != nil {
		return err
	}
	return e.s.m.w.write("itos-cc.yaml", "# a reviewed exception\nother:\n  kept: true\n"+e.s.m.read("itos-cc.yaml"))
}
func (e *exceptionMutation) invalid() error {
	if err := e.review(e.reason); err != nil {
		return err
	}
	delete(e.entry, "reason")
	return e.writeEntries([]map[string]any{e.entry})
}
func (e *exceptionMutation) refusedFixture() error {
	source := "export class Board {\nplace(i) {\n// past five\n// site\nreturn i>5;\n}\nclear(x){\n// not reached\n  !x;\n}\n}\n"
	tests := "import {expect,test} from \"vitest\";import {Board} from \"./board\";\ntest(\"place\",()=>{expect(new Board().place(5)).toBe(false);expect(new Board().place(6)).toBe(true)});\n"
	if err := e.s.m.w.write("src/board.ts", source); err != nil {
		return err
	}
	if err := e.s.m.w.write("src/board.test.ts", tests); err != nil {
		return err
	}
	if err := e.declare("src/board.ts", false); err != nil {
		return err
	}
	if err := e.run("mutation run --json src/board.ts"); err != nil {
		return err
	}
	f, err := e.r.fileObject()
	if err != nil {
		return err
	}
	if err := exceptionFields(f, map[string]any{"killed": float64(1), "uncovered": float64(1), "survived": float64(0)}); err != nil {
		return err
	}
	mutants, err := e.r.entries()
	if err != nil {
		return err
	}
	if len(mutants) != 2 {
		return fmt.Errorf("refusal fixture mutants %v", mutants)
	}
	if err := exceptionFields(mutants[0], map[string]any{"line": float64(5), "column": float64(9), "outcome": "killed"}); err != nil {
		return err
	}
	return exceptionFields(mutants[1], map[string]any{"line": float64(9), "column": float64(3), "outcome": "uncovered"})
}
func (e *exceptionMutation) uncoveredEntry() error {
	if err := e.refusedFixture(); err != nil {
		return err
	}
	snap, err := e.s.snapshot("src/board.ts")
	if err != nil {
		return err
	}
	units := e.s.units(snap, "Board#clear")
	if len(units) != 1 {
		return fmt.Errorf("no clear unit: %v", units)
	}
	e.entry = map[string]any{"file": "src/board.ts", "function": "board.Board#clear", "hash": units[0]["hash"], "line_in_function": 3, "column": 3, "original": "!", "replacement": "", "reason": "does not excuse missing coverage"}
	return e.writeEntries([]map[string]any{e.entry})
}
func (e *exceptionMutation) counterfactualStale() error {
	if err := e.run("mutation check --json src/board.ts"); err != nil {
		return err
	}
	if e.s.m.w.exit != 1 {
		return fmt.Errorf("outside-range stale counterfactual exit%d", e.s.m.w.exit)
	}
	_, err := e.stale(map[string]any{"function": "board.Board#count", "why": "changed"})
	return err
}
func (e *exceptionMutation) deleteBoard() error {
	for _, file := range []string{"src/board.ts", "src/board.test.ts"} {
		if err := e.s.m.w.d.Remove(filepath.Join(e.s.m.w.dir, file)); err != nil {
			return err
		}
	}
	return nil
}
func (e *exceptionMutation) freshAfterDeletion() error {
	if err := e.deleteBoard(); err != nil {
		return err
	}
	if err := e.s.m.w.write("src/util.ts", "export function util(x){ return x>5; }\n"); err != nil {
		return err
	}
	if err := e.s.m.w.write("src/util.test.ts", "import {expect,test} from \"vitest\";import {util} from \"./util\";test(\"util\",()=>{expect(util(5)).toBe(false);expect(util(6)).toBe(true)});\n"); err != nil {
		return err
	}
	if err := e.declare("src/util.ts", false); err != nil {
		return err
	}
	if err := e.run("mutation run --json src/util.ts"); err != nil {
		return err
	}
	if e.s.m.w.exit != 0 {
		return fmt.Errorf("remaining source record: %s", e.s.m.w.report())
	}
	return nil
}
func (e *exceptionMutation) rename(commit bool) error {
	tests := e.s.m.read("src/board.test.ts")
	if err := e.s.m.w.write("src/grid.ts", e.s.m.read("src/board.ts")); err != nil {
		return err
	}
	if err := e.s.m.w.d.Remove(filepath.Join(e.s.m.w.dir, "src/board.ts")); err != nil {
		return err
	}
	if e.s.m.read("src/board.test.ts") != tests {
		return fmt.Errorf("rename changed tests")
	}
	if commit {
		if err := e.s.commit("rename board source only"); err != nil {
			return err
		}
	}
	e.s.file = "src/grid.ts"
	e.s.m.file = e.s.file
	e.r.file = e.s.file
	return e.declare(e.s.file, true)
}
