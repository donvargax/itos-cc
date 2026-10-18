package features

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// What the selection scenarios check of the files a command selected and
// analyzed.

// selected is the files the last run selected, slash-separated as it named
// them, from the working directory it ran in: what its discovery listed,
// or the production files a run of crap listed.
func (s *selectionState) selected() ([]string, error) {
	if s.found != nil && s.last == s.w.last {
		return slices.Concat(s.found.sources, s.found.tests), nil
	}
	entries, err := (&crapState{w: s.w}).listed()
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if file := filepath.ToSlash(e.File); !slices.Contains(files, file) {
			files = append(files, file)
		}
	}
	return files, nil
}

// fromRoot is file, named from the folder the last run ran in, as a
// slash-separated path from the scratch folder.
func (s *selectionState) fromRoot(file string) string {
	if s.w.last == nil {
		return file
	}
	rel, err := filepath.Rel(s.w.dir, s.w.last.dir)
	if err != nil {
		return file
	}
	return path.Join(filepath.ToSlash(rel), file)
}

// everyFileSelected is whether the discovery listed every source and test
// of the project, each as the kind it is.
func (s *selectionState) everyFileSelected() error {
	if s.found == nil || len(s.sources) == 0 {
		return errors.New("no discovery of the project's files")
	}
	for _, want := range []struct {
		kind       string
		files, got []string
	}{{"production", s.sources, s.found.sources}, {"test", s.tests, s.found.tests}} {
		for _, file := range want.files {
			if !slices.Contains(want.got, file) {
				return fmt.Errorf("%s is not selected as %s code: %v\n%s", file, want.kind, want.got, s.w.report())
			}
		}
	}
	return nil
}

// areSelected is whether the run selected exactly what the sentence says:
// the production files under a folder and one file besides, every file
// staged, or the files it lists.
func (s *selectionState) areSelected(what string) error {
	if what == "its files" {
		return s.itsFilesAre("selected")
	}
	got, err := s.selected()
	if err != nil {
		return err
	}
	if what == "the staged files" {
		if len(s.staged) == 0 {
			return errors.New("no file was staged")
		}
		for _, file := range s.staged {
			if !slices.Contains(got, file) {
				return fmt.Errorf("%s, staged, is not selected: %v\n%s", file, got, s.w.report())
			}
		}
		return nil
	}
	var want []string
	if m := regexp.MustCompile(`^the files under (\S+) and the file (\S+)$`).FindStringSubmatch(what); m != nil {
		for _, file := range s.sources {
			if strings.HasPrefix(file, m[1]+"/") {
				want = append(want, file)
			}
		}
		want = append(want, m[2])
	} else {
		want = splitList(what)
	}
	return sameFiles(got, want, s.w.report())
}

// sameFiles is whether got and want hold the same files.
func sameFiles(got, want []string, report string) error {
	got, want = slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(want))
	if len(want) == 0 || !slices.Equal(got, want) {
		return fmt.Errorf("selected %v, not %v\n%s", got, want, report)
	}
	return nil
}

// notSelected is whether the run selected no file named name, from its
// working directory or from the scratch folder, and the project had it.
func (s *selectionState) notSelected(name string) error {
	if _, ok := s.files[name]; !ok && !slices.Contains(s.repo.deleted, name) {
		return fmt.Errorf("the project never held %s", name)
	}
	got, err := s.selected()
	if err != nil {
		return err
	}
	for _, file := range got {
		if file == name || s.fromRoot(file) == name {
			return fmt.Errorf("%s is selected\n%s", name, s.w.report())
		}
	}
	return nil
}

// everySourceContaining is whether the run selected exactly the project's
// production files whose path contains text, example among them.
func (s *selectionState) everySourceContaining(text, example string) error {
	var want []string
	for _, file := range s.sources {
		if strings.Contains(file, text) {
			want = append(want, file)
		}
	}
	if !slices.Contains(want, example) {
		return fmt.Errorf("%s is no source of the project whose path contains %q", example, text)
	}
	got, err := s.selected()
	if err != nil {
		return err
	}
	return sameFiles(got, want, s.w.report())
}

// nothingInside is whether the discovery listed files, and none under the
// directories a Given filled.
func (s *selectionState) nothingInside() error {
	if s.found == nil || len(s.found.sources) == 0 || len(s.dirs) == 0 {
		return errors.New("no discovery of a project holding those directories")
	}
	for _, file := range slices.Concat(s.found.sources, s.found.tests) {
		for _, dir := range s.dirs {
			if strings.HasPrefix(file, dir+"/") {
				return fmt.Errorf("%s, in %s, is selected\n%s", file, dir, s.w.report())
			}
		}
	}
	return nil
}

// itsFilesAre is whether every discovery listed each file of the build
// output's directory, or none of them.
func (s *selectionState) itsFilesAre(selected string) error {
	if s.output == nil || len(s.output.found) == 0 || len(s.output.found) != len(s.output.manifests) {
		return errors.New("no discovery of the directory's files")
	}
	for i, found := range s.output.found {
		for _, file := range s.output.files {
			if slices.Contains(found.sources, file) != (selected == "selected") {
				return fmt.Errorf("%s is %s, beside %q: %v", file, map[bool]string{true: "selected", false: "not selected"}[selected != "selected"], s.output.manifests[i], found.sources)
			}
		}
	}
	return nil
}

// analyzed is whether the last run, of units, listed a unit of file.
func (s *selectionState) analyzed(file string) error {
	if _, ok := s.files[file]; !ok {
		return fmt.Errorf("the project holds no %s", file)
	}
	for _, line := range outputLines(filepath.ToSlash(s.w.stdout)) {
		if strings.HasPrefix(line, file+":") {
			return nil
		}
	}
	return fmt.Errorf("no unit of %s is listed\n%s", file, s.w.report())
}

// jsxFunction is a function that holds JSX and a decision: of complexity 2
// where JSX parses, as in the JavaScript grammar, and 1 in TypeScript's,
// where it does not and the decision is lost.
func jsxFunction(name string) string {
	return "export function " + name + "(n) {\n  const tag = <span className=\"held\">{n}</span>;\n" +
		"  if (n > 1) {\n    return tag;\n  }\n  return null;\n}\n"
}

// theFiles writes each file the list names, each a function holding JSX
// and a decision.
func (s *selectionState) theFiles(list string) error {
	s.named = splitList(list)
	if len(s.named) < 2 {
		return fmt.Errorf("%q names fewer than two files", list)
	}
	for _, name := range s.named {
		if err := s.write(name, jsxFunction(functionOf(name))); err != nil {
			return err
		}
	}
	return nil
}

// functionOf is the name of the one function the step wrote in file.
func functionOf(file string) string {
	return "f_" + identifier(path.Base(file))
}

// javaScript is whether the discovery listed each named file as TypeScript
// production code, and crap gives its function the complexity the
// JavaScript grammar does.
func (s *selectionState) javaScript() error {
	if s.found == nil || len(s.named) == 0 {
		return errors.New("no discovery of the named files")
	}
	for _, file := range s.named {
		if !slices.Contains(s.found.sources, file) || s.found.language[file] != "typescript" {
			return fmt.Errorf("%s is not selected as TypeScript: %s, %v", file, s.found.language[file], s.found.sources)
		}
	}
	if err := s.w.runEnv(s.w.dir, s.w.env(), append([]string{"crap", "--no-coverage", "--json"}, s.named...)...); err != nil {
		return err
	}
	entries, err := (&crapState{w: s.w}).listed()
	if err != nil {
		return err
	}
	for _, file := range s.named {
		i := slices.IndexFunc(entries, func(e crapEntry) bool { return filepath.ToSlash(e.File) == file })
		if i < 0 {
			return fmt.Errorf("crap scores no function of %s\n%s", file, s.w.report())
		}
		if e := entries[i]; e.Name != functionOf(file) || e.Complexity != 2 {
			return fmt.Errorf("%s's %s has complexity %d, not the 2 the JavaScript grammar gives it\n%s", file, e.Name, e.Complexity, s.w.report())
		}
	}
	return nil
}

// neitherSelected is whether the discovery listed files, none of them one
// the Given named.
func (s *selectionState) neitherSelected() error {
	if s.found == nil || len(s.found.sources) == 0 || len(s.named) == 0 {
		return errors.New("no discovery of a project holding the named files")
	}
	for _, file := range s.named {
		if slices.Contains(s.found.sources, file) || slices.Contains(s.found.tests, file) {
			return fmt.Errorf("%s is selected\n%s", file, s.w.report())
		}
	}
	return nil
}
