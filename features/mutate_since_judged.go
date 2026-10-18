package features

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
)

// Which functions a --since run judges, and how it carries a renamed file's
// results.

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
	if !slices.EqualFunc(names, want, func(name, w string) bool { return sinceFunction(listedSite{function: name}, w) }) {
		return fmt.Errorf("judged %v, want %v", names, want)
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
	// A line of code: deleting a comment's line is no change of its function.
	s.tsSource = strings.Replace(sinceBoardTS, "    return i > 5;\n", "    const limit = 5;\n    return i > 5;\n", 1)
	if err := s.project(true); err != nil {
		return err
	}
	if err := s.replace(s.file, "    const limit = 5;\n", ""); err != nil {
		return err
	}
	if err := s.commit("delete a line in place"); err != nil {
		return err
	}
	return s.declare()
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
		if err := s.changePlaceCode(s.file); err != nil {
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
