package features

import (
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const resultBoardTS = `export class Board {
place(i: number): boolean {
// past five
// a deliberate boundary
return i>5;
}
full(i: number): boolean { return i>=9; }
clear(i: number): boolean { return i<3; }
}
`
const resultTestsTS = `import {expect,test} from "vitest";
import {Board} from "./board";
test("place",()=>{const b=new Board();expect(b.place(5)).toBe(false);expect(b.place(6)).toBe(true)});
test("full",()=>expect(new Board().full(20)).toBe(true));
`

func (r *resultMutation) ts(file, source, tests string, surviving, uncovered []string) error {
	s, w := r.s, r.s.m.w
	s.made = true
	s.ts = true
	s.file = file
	s.place = "Board#place"
	s.clear = "Board#clear"
	s.m.file = file
	r.file = file
	test := strings.TrimSuffix(file, ".ts") + ".test.ts"
	if err := s.m.writeProject(map[string][]string{test: {file}}, map[string]string{"package.json": vitestPackage, file: source, test: tests}); err != nil {
		return err
	}
	sites, err := s.m.sitesOf(file)
	if err != nil {
		return err
	}
	report := "SF:" + file + "\n"
	hits := map[int]int{}
	t := Tests{Language: "typescript", Command: vitestCommand + file}
	for _, site := range sites {
		n := 1
		if slices.ContainsFunc(uncovered, func(name string) bool { return sinceFunction(site, name) }) {
			n = 0
		}
		hits[site.line] = n
		if slices.ContainsFunc(surviving, func(name string) bool { return sinceFunction(site, name) }) {
			t.Surviving = append(t.Surviving, Site{File: file, Line: site.line, Original: site.original, Replacement: site.replacement})
		}
	}
	var lines []int
	for line := range hits {
		lines = append(lines, line)
	}
	slices.Sort(lines)
	for _, line := range lines {
		report += fmt.Sprintf("DA:%d,%d\n", line, hits[line])
	}
	report += "end_of_record\n"
	if err := w.d.Coverage(w.dir, Coverage{Language: "typescript", Report: report}); err != nil {
		return err
	}
	if err := w.d.Tests(w.dir, t); err != nil {
		return err
	}

	for _, command := range []string{vitestCommand + filepath.ToSlash(filepath.Join(w.dir, file)), filepath.ToSlash(filepath.Join(w.dir, vitestCommand)) + file} {
		t.Command = command
		if err := w.d.Tests(w.dir, t); err != nil {
			return err
		}
	}

	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "."}, {"commit", "-qm", "result fixture"}, {"tag", "base"}} {
		if err := w.git(args...); err != nil {
			return err
		}
	}
	return nil
}
func (r *resultMutation) mixed() error {
	return r.ts("src/board.ts", resultBoardTS, resultTestsTS, []string{"Board#full"}, []string{"Board#clear"})
}
func (r *resultMutation) strong() error {
	source := sinceBoardTS
	tests := strings.Replace(sinceBoardTestTS, `expect(new Board().clear(2)).toBe(true);`, `expect(new Board().clear(2)).toBe(true); expect(new Board().clear(3)).toBe(false);`, 1)
	return r.ts("src/board.ts", source, tests, nil, nil)
}
func (r *resultMutation) summaryFixture() error {
	source := "export function place(i: number): boolean {\n// past five\n// deliberate boundary\n// at the specified site\nreturn i>5;\n}\n"
	tests := `import {expect,test} from "vitest";` + "\n" + `import * as b from "./board";` + "\n" + `test("place",()=>expect(b.place(6)).toBe(true));` + "\n"
	for i := 1; i <= 14; i++ {
		name := "k" + strconv.Itoa(i)
		source += "export function " + name + "(i: number): boolean { return i>5; }\n"
		tests += fmt.Sprintf("test(%q,()=>{expect(b.%s(5)).toBe(false);expect(b.%s(6)).toBe(true)});\n", name, name, name)
	}
	for i := 1; i <= 2; i++ {
		source += fmt.Sprintf("export function u%d(i: number): boolean { return i>5; }\n", i)
	}
	if err := r.ts("src/board.ts", source, tests, []string{"board#place"}, []string{"board#u1", "board#u2"}); err != nil {
		return err
	}
	if err := r.run("mutation run --json src/board.ts"); err != nil {
		return err
	}
	snap, err := r.s.snapshot(r.file)
	if err != nil {
		return err
	}
	var keep []any
	for _, raw := range snap["units"].([]any) {
		u := raw.(map[string]any)
		for i := 1; i <= 8; i++ {
			if u["name"] == "k"+strconv.Itoa(i) {
				keep = append(keep, u)
			}
		}
	}
	if len(keep) != 8 {
		return fmt.Errorf("summary fixture has %d cached functions, want eight", len(keep))
	}
	snap["units"] = keep
	return (&checkMutation{s: r.s}).writeSnapshot(snap)
}

func (r *resultMutation) python(file, source, testfile, test string, parallel bool) error {
	w := r.s.m.w
	r.file = file
	r.s.file = file
	r.s.m.file = file
	config := "[project]\nname = \"fixture\"\nversion = \"0.0.0\"\n"
	if parallel {
		config += "\n[tool.coverage.run]\nparallel = true\n"
	}
	if err := r.s.m.writeProject(map[string][]string{testfile: {file}}, map[string]string{"pyproject.toml": config, file: source, testfile: test}); err != nil {
		return err
	}
	report := "SF:" + file + "\nDA:2,1\nend_of_record\n"
	if err := w.d.Coverage(w.dir, Coverage{Language: "python", Report: report}); err != nil {
		return err
	}
	t := Tests{Language: "python", Command: "python3 -m pytest -q -x -p no:cacheprovider " + testfile}
	if file == "x.py" {
		t.Surviving = []Site{{File: file, Line: 2, Original: ">", Replacement: ">="}}
	}
	if err := w.d.Tests(w.dir, t); err != nil {
		return err
	}
	if err := r.s.m.pythonProbe(true); err != nil {
		return err
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "."}, {"commit", "-qm", "python fixture"}} {
		if err := w.git(args...); err != nil {
			return err
		}
	}
	return nil
}
