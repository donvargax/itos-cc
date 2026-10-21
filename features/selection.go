// The steps of file_selection.feature: which files a command looks at, as
// units lists them (its production code, and with --tests its test code)
// and crap scores them. "The project's files are discovered", and a command
// run with no paths or with --changed, is units --json and units --tests
// --json; a crap run as written selects the production files it lists. Like
// every step here, they read the binary's output alone, never its code.
package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

// selectionState is one file_selection.feature scenario's state beside the
// world's.
type selectionState struct {
	w *world

	// files are every file the Background and the Givens wrote, by
	// slash-separated path from the scratch folder, with their text.
	files map[string]string
	// sources and tests are the Background project's production and test
	// files: its fixture under testdata/ is neither.
	sources, tests []string
	// repo is what git reports of the scratch folder's repository.
	repo gitPlan
	// staged are the files a Given staged before any commit.
	staged []string
	// found is what the last discovery listed, made by the run last; nil
	// before one.
	found *discovery
	last  *run
	// dirs are the directories a Given filled that nothing may select.
	dirs []string
	// output is the directory a Given filled to see whether it is skipped
	// as build output: its files, the manifests beside it, one per way the
	// sentence names ("" for none), and what each discovery listed.
	output *outputDir
	// named are the files a Given named, by slash-separated path.
	named []string
}

// gitPlan is what git reports of the scratch folder: the files changed
// since its one commit, staged, untracked and deleted, by slash-separated
// path.
type gitPlan struct {
	modified, staged, untracked, deleted []string
}

// discovery is what units lists in a folder: the files of its units, the
// production ones and with --tests the test ones, slash-separated as it
// names them, and each file's language.
type discovery struct {
	sources, tests []string
	language       map[string]string
}

type outputDir struct {
	files     []string
	manifests []string
	found     []*discovery
}

// The Background project: TypeScript, Python, Kotlin and Go sources, each
// file of production code and of tests holding one function, in a Go module
// at the scratch folder and a Kotlin module, kt/, built with Maven and no
// JaCoCo, which coverage cannot measure and no tool needs to be installed
// to show. testdata/board.go is a fixture of its Go tests, not a source.
var (
	selectionSources = []string{
		"board.go", "src/billing/invoice.ts", "src/billing/ledger.py", "src/cart/total.ts",
		"src/cart/basket.py", "kt/src/main/kotlin/Shelf.kt",
	}
	selectionTests = []string{
		"board_test.go", "src/billing/invoice.test.ts", "src/billing/test_ledger.py",
		"src/cart/total.test.ts", "kt/src/test/kotlin/ShelfTest.kt",
	}
)

const selectionFixture = "testdata/board.go"

// fileSelectionFeature declares file_selection.feature's steps, selectionSteps, which
// tools/bin/assemble lists in steps_gen.go.
var fileSelectionFeature = featureFile{Name: "file_selection.feature", Steps: selectionSteps}

func selectionSteps(sc *godog.ScenarioContext, w *world) {
	s := &selectionState{w: w}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*s = selectionState{w: w, files: map[string]string{}}
		return ctx, nil
	})

	sc.Step(`^a project with TypeScript, Python, Kotlin, and Go sources$`, s.project)

	// Paths, fragments and what a run selects.
	sc.Step(`^I run a command with no paths$`, func() error { return s.discover(w.dir) })
	sc.Step(`^I run a command with --changed$`, func() error { return s.discover(w.dir, "--changed") })
	sc.Step(`^I run "itos-cc ?([^"]*)" in (\S+)$`, s.runIn)
	sc.Step(`^I run "itos-cc ([^"]*)", "itos-cc ([^"]*)" or "itos-cc ([^"]*)"$`, func(a, b, c string) error {
		return w.runEach(a, b, c)
	})
	sc.Step(`^every supported file under the working directory is selected$`, s.everyFileSelected)
	sc.Step(`^(.+) are selected$`, s.areSelected)
	sc.Step(`^(\S+) is not selected$`, s.notSelected)
	sc.Step(`^there is no file or directory called "([^"]*)"$`, s.noneCalled)
	sc.Step(`^every source whose path contains "([^"]*)" is selected, such as (\S+)$`, s.everySourceContaining)
	sc.Step(`^no file's path under the working directory contains "([^"]*)"$`, s.noPathContains)

	// What git reports.
	sc.Step(`^git reports (\S+) as modified, (\S+) as staged, and (\S+) as untracked$`, func(m, st, u string) error {
		s.repo.modified = append(s.repo.modified, m)
		s.repo.staged = append(s.repo.staged, st)
		s.repo.untracked = append(s.repo.untracked, u)
		return s.applyRepo()
	})
	sc.Step(`^git reports ([^,]+) as modified$`, func(list string) error {
		s.repo.modified = append(s.repo.modified, splitList(list)...)
		return s.applyRepo()
	})
	sc.Step(`^([^,]+) as untracked$`, func(list string) error {
		s.repo.untracked = append(s.repo.untracked, splitList(list)...)
		return s.applyRepo()
	})
	sc.Step(`^(\S+) was deleted$`, func(name string) error {
		s.repo.deleted = append(s.repo.deleted, name)
		return s.applyRepo()
	})
	sc.Step(`^git reports no changes$`, s.applyRepo)
	sc.Step(`^a git repository with staged files and no commits$`, s.stagedNoCommits)
	sc.Step(`^the working directory is not a git repository$`, s.notARepository)

	// What is never walked.
	sc.Step(`^the project contains these directories, each holding source files:$`, s.directories)
	sc.Step(`^the project's files are discovered$`, s.discovered)
	sc.Step(`^nothing inside those directories is selected$`, s.nothingInside)
	sc.Step(`^a directory "([^"]*)" holding source files$`, s.outputDirectory)
	sc.Step(`^its files are not selected$`, func() error { return s.itsFilesAre("not selected") })
	sc.Step(`^(\S+) is analyzed$`, s.analyzed)

	// Languages and kinds.
	sc.Step(`^the files ((?:\S+, )*\S+,? and \S+)$`, s.theFiles)
	sc.Step(`^each is selected as TypeScript and parsed with the JavaScript grammar$`, s.javaScript)
	sc.Step(`^neither is selected$`, s.neitherSelected)
	sc.Step(`^the file "([^"]*)" is discovered$`, s.fileDiscovered)
	sc.Step(`^it is classified as (production|test)$`, s.classifiedAs)

	// How a report names a file.
	sc.Step(`^a command reports on a file inside the working directory$`, s.reportsInside)
	sc.Step(`^the file is named by its path relative to the working directory$`, s.namedRelative)
	sc.Step(`^a file outside the working directory keeps its absolute path$`, s.outsideAbsolute)

	// itos-cc.yaml's exclude.
	sc.Step(`^itos-cc\.yaml holds '([^']*)'$`, func(text string) error { return s.write("itos-cc.yaml", text+"\n") })
	sc.Step(`^itos-cc\.yaml is a directory, so it cannot be read$`, func() error {
		return s.w.d.Mkdir(filepath.Join(s.w.dir, "itos-cc.yaml"))
	})
	sc.Step(`^the sources (\S+) and (\S+)$`, func(a, b string) error {
		if err := s.write(a, sourceFor(a)); err != nil {
			return err
		}
		return s.write(b, sourceFor(b))
	})
	sc.Step(`^(\S+) is scored, and (\S+) is not$`, s.scoredNotOther)
}

// Fixtures.

// write writes text to name, a slash-separated path in the scratch folder,
// and remembers it as a file of the project.
func (s *selectionState) write(name, text string) error {
	s.files[name] = text
	return s.w.write(name, text)
}

// remove removes name, a slash-separated path in the scratch folder.
func (s *selectionState) remove(name string) error {
	delete(s.files, name)
	return s.w.d.Remove(filepath.Join(s.w.dir, filepath.FromSlash(name)))
}

// project writes the Background project and declares that go test measures
// its module; its TypeScript has no tools to measure it, and its Kotlin is
// a build root coverage cannot measure.
func (s *selectionState) project() error {
	files := map[string]string{"go.mod": goModule, "kt/pom.xml": unmeasuredPom, selectionFixture: sourceFor(selectionFixture)}
	for _, name := range append(slices.Clone(selectionSources), selectionTests...) {
		files[name] = sourceFor(name)
	}
	for name, text := range files {
		if err := s.write(name, text); err != nil {
			return err
		}
	}
	s.sources, s.tests = slices.Clone(selectionSources), slices.Clone(selectionTests)
	return s.w.d.Coverage(s.w.dir, Coverage{Language: "go"})
}

// sourceFor is the text of a file of name's language holding one function,
// named after the file: a test's function in a test file of Go, whose
// package is its folder's name, board at the root.
func sourceFor(name string) string {
	fn := "f_" + identifier(strings.TrimSuffix(path.Base(name), path.Ext(name)))
	switch path.Ext(name) {
	case ".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs":
		return "export function " + fn + "(n) {\n  return n;\n}\n"
	case ".py":
		return "def " + fn + "(n):\n    return n\n"
	case ".kt":
		return "fun " + fn + "(n: Int): Int {\n    return n\n}\n"
	case ".go":
		pkg := "board"
		if dir := path.Dir(name); dir != "." {
			if pkg = identifier(path.Base(dir)); pkg == "" {
				pkg = "p"
			}
		}
		if strings.HasSuffix(name, "_test.go") {
			return "package " + pkg + "\n\nimport \"testing\"\n\nfunc Test" + fn + "(t *testing.T) {\n\tt.Log(\"" + fn + "\")\n}\n"
		}
		return "package " + pkg + "\n\nfunc " + fn + "(n int) int {\n\treturn n\n}\n"
	}
	return "x\n"
}

// identifier is text's ASCII letters and digits, lower-cased.
func identifier(text string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// splitList is the names of a list as a sentence writes it: "a", "a and
// b", "a, b, and c".
func splitList(list string) []string {
	list = strings.ReplaceAll(strings.ReplaceAll(list, ", and ", ", "), " and ", ", ")
	var names []string
	for _, name := range strings.Split(list, ", ") {
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// applyRepo makes the scratch folder a repository whose one commit holds
// the project as it stands, each file the plan modifies, stages or deletes
// at its first text and none it leaves untracked, then changes them as the
// plan says: a modified or staged file gains a line, a staged one is
// staged, a deleted one removed and an untracked one written.
func (s *selectionState) applyRepo() error {
	p := s.repo
	for _, name := range p.untracked {
		if err := s.remove(name); err != nil {
			return err
		}
	}
	for _, name := range slices.Concat(p.modified, p.staged, p.deleted) {
		if err := s.write(name, sourceFor(name)); err != nil {
			return err
		}
	}
	if err := s.w.d.Remove(filepath.Join(s.w.dir, ".git")); err != nil {
		return err
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"commit", "-q", "-m", "the project"}} {
		if err := s.w.git(args...); err != nil {
			return err
		}
	}
	for _, name := range slices.Concat(p.modified, p.staged) {
		if err := s.write(name, sourceFor(name)+changedLine(name)); err != nil {
			return err
		}
	}
	for _, name := range p.staged {
		if err := s.w.git("add", name); err != nil {
			return err
		}
	}
	for _, name := range p.deleted {
		if err := s.remove(name); err != nil {
			return err
		}
	}
	for _, name := range p.untracked {
		if err := s.write(name, sourceFor(name)); err != nil {
			return err
		}
	}
	return nil
}

// changedLine is a comment in name's language, a change git reports.
func changedLine(name string) string {
	if path.Ext(name) == ".py" {
		return "# changed\n"
	}
	return "// changed\n"
}

// stagedNoCommits makes the scratch folder a repository with no commit,
// every source and test of the project staged.
func (s *selectionState) stagedNoCommits() error {
	if err := s.w.git("init", "-q"); err != nil {
		return err
	}
	s.staged = append(slices.Clone(s.sources), s.tests...)
	return s.w.git(append([]string{"add"}, s.staged...)...)
}

// notARepository skips the scenario where the scratch folder is in a
// repository, which no Given can undo.
func (s *selectionState) notARepository() error {
	for dir := s.w.dir; ; dir = filepath.Dir(dir) {
		if s.w.d.Exists(filepath.Join(dir, ".git")) {
			return fmt.Errorf("%s is a repository above the scratch folder: %w", dir, godog.ErrSkip)
		}
		if filepath.Dir(dir) == dir {
			return nil
		}
	}
}

// noneCalled is whether nothing in the scratch folder is called name.
func (s *selectionState) noneCalled(name string) error {
	if s.w.d.Exists(filepath.Join(s.w.dir, filepath.FromSlash(name))) {
		return fmt.Errorf("the project holds %s", name)
	}
	return nil
}

// noPathContains is whether no file the project holds has text in its
// path.
func (s *selectionState) noPathContains(text string) error {
	for name := range s.files {
		if strings.Contains(name, text) {
			return fmt.Errorf("the project's %s contains %q", name, text)
		}
	}
	return nil
}

// directories writes a TypeScript and a Python source and a TypeScript
// test into each directory of the table, at the scratch folder.
func (s *selectionState) directories(table *godog.Table) error {
	for i, row := range table.Rows {
		if i == 0 {
			continue
		}
		dir := row.Cells[0].Value
		for _, name := range []string{"held.ts", "held.py", "held.test.ts"} {
			if err := s.write(dir+"/"+name, sourceFor(name)); err != nil {
				return err
			}
		}
		s.dirs = append(s.dirs, dir)
	}
	if len(s.dirs) == 0 {
		return errors.New("the table names no directory")
	}
	return nil
}

// The directory of a row and what the row says of it: beside one manifest
// or either of two, git ignoring it, a Go package, or beside go.mod alone.
var outputRow = regexp.MustCompile(`^(\S+?)(?:, (?:beside (\S+)(?: or (\S+)| (only))?|(which git ignores)|(a Go package)))?$`)

// outputDirectory writes a TypeScript, a Python and a Go source into the
// directory the row names, and lays out what the row says is beside it: a
// manifest, a .gitignore that ignores it, the Background's go.mod alone.
func (s *selectionState) outputDirectory(text string) error {
	m := outputRow.FindStringSubmatch(text)
	if m == nil {
		return fmt.Errorf("%q is no directory this step lays out", text)
	}
	dir := m[1]
	out := &outputDir{}
	for _, name := range []string{"held.ts", "held.py", "held.go"} {
		file := dir + "/" + name
		if err := s.write(file, sourceFor(file)); err != nil {
			return err
		}
		out.files = append(out.files, file)
	}
	beside := func(manifest string) string {
		if strings.Contains(manifest, "/") {
			return manifest
		}
		return path.Join(path.Dir(dir), manifest)
	}
	switch {
	case m[4] != "":
		if m[2] != "go.mod" {
			return fmt.Errorf("%q: only go.mod is a file a directory sits beside alone here", text)
		}
		// Its folder holds go.mod and Go sources, nothing else.
		parent := path.Dir(dir)
		if _, ok := s.files[path.Join(parent, "go.mod")]; !ok {
			return fmt.Errorf("%s holds no go.mod", parent)
		}
		for name := range s.files {
			if path.Dir(name) == parent && name != path.Join(parent, "go.mod") && path.Ext(name) != ".go" {
				return fmt.Errorf("%s holds %s beside go.mod", parent, name)
			}
		}
		out.manifests = []string{""}
	case m[2] != "":
		out.manifests = []string{beside(m[2])}
		if m[3] != "" {
			out.manifests = append(out.manifests, beside(m[3]))
		}
	case m[5] != "":
		if err := s.write(".gitignore", "/"+dir+"/\n"); err != nil {
			return err
		}
		if err := s.w.git("init", "-q"); err != nil {
			return err
		}
		if err := s.w.git("check-ignore", "-q", dir); err != nil {
			return fmt.Errorf("git does not ignore %s: %v", dir, err)
		}
		out.manifests = []string{""}
	case m[6] != "":
		if _, ok := s.files["go.mod"]; !ok {
			return errors.New("the project is no Go module")
		}
		out.manifests = []string{""}
	default:
		out.manifests = []string{""}
	}
	s.output = out
	return nil
}

// manifestText is a manifest of a build tool, as little as names it.
func manifestText(name string) string {
	switch path.Base(name) {
	case "package.json":
		return "{}\n"
	case "pom.xml":
		return unmeasuredPom
	case "Cargo.toml":
		return "[package]\nname = \"held\"\nversion = \"0.1.0\"\n"
	}
	return "\n"
}

// Runs.

// discover runs units --json and units --tests --json, with args, in dir,
// and remembers what they listed.
func (s *selectionState) discover(dir string, args ...string) error {
	found := &discovery{language: map[string]string{}}
	for _, tests := range []bool{false, true} {
		words := []string{"units", "--json"}
		if tests {
			words = append(words, "--tests")
		}
		if err := s.w.runEnv(dir, s.w.env(), append(words, args...)...); err != nil {
			return err
		}
		if s.w.exit != 0 {
			return fmt.Errorf("itos-cc %s exited %d\n%s", strings.Join(s.w.last.args, " "), s.w.exit, s.w.report())
		}
		var object struct {
			Units []struct{ File, Language string }
		}
		if err := json.Unmarshal([]byte(s.w.stdout), &object); err != nil {
			return fmt.Errorf("standard output is not units' object: %v\n%s", err, s.w.report())
		}
		for _, u := range object.Units {
			file := filepath.ToSlash(u.File)
			found.language[file] = u.Language
			list := &found.sources
			if tests {
				list = &found.tests
			}
			if !slices.Contains(*list, file) {
				*list = append(*list, file)
			}
		}
	}
	s.found, s.last = found, s.w.last
	return nil
}

// discovered discovers the project's files: once for each manifest a
// build output's directory is to be beside, with that manifest alone
// written, else once.
func (s *selectionState) discovered() error {
	if s.output == nil {
		return s.discover(s.w.dir)
	}
	for _, manifest := range s.output.manifests {
		if manifest != "" {
			if err := s.write(manifest, manifestText(manifest)); err != nil {
				return err
			}
		}
		err := s.discover(s.w.dir)
		if manifest != "" {
			if err := s.remove(manifest); err != nil {
				return err
			}
		}
		if err != nil {
			return err
		}
		s.output.found = append(s.output.found, s.found)
	}
	return nil
}

// runIn runs itos-cc with args in dir, a slash-separated folder of the
// scratch folder.
func (s *selectionState) runIn(args, dir string) error {
	fields, err := commandWords(args)
	if err != nil {
		return err
	}
	return s.w.runEnv(filepath.Join(s.w.dir, filepath.FromSlash(dir)), s.w.env(), fields...)
}

// Then steps.

// fileDiscovered writes file, a function of its language, and discovers
// it alone.
func (s *selectionState) fileDiscovered(file string) error {
	if err := s.write(file, sourceFor(file)); err != nil {
		return err
	}
	s.named = []string{file}
	return s.discover(s.w.dir, file)
}

// classifiedAs is whether the discovery listed the file as that kind of
// code, and not as the other.
func (s *selectionState) classifiedAs(kind string) error {
	if s.found == nil || len(s.named) != 1 {
		return errors.New("no discovery of one file")
	}
	file := s.named[0]
	inSources, inTests := slices.Contains(s.found.sources, file), slices.Contains(s.found.tests, file)
	if kind == "production" && (!inSources || inTests) || kind == "test" && (!inTests || inSources) {
		return fmt.Errorf("%s is listed as production %t, as test %t; want %s\n%s", file, inSources, inTests, kind, s.w.report())
	}
	return nil
}

// The files of a report naming one file inside its working directory, the
// project's src, and one outside it.
const (
	insideFile  = "cart/total.ts"
	outsideFile = "../kt/src/main/kotlin/Shelf.kt"
)

// reportsInside runs units --json in src, naming a file in it and one
// outside it.
func (s *selectionState) reportsInside() error {
	return s.w.runEnv(filepath.Join(s.w.dir, "src"), s.w.env(), "units", "--json", insideFile, filepath.FromSlash(outsideFile))
}

// reportedFiles are the files of the units the last run listed, as it
// named them, slash-separated.
func (s *selectionState) reportedFiles() ([]string, error) {
	var object struct{ Units []struct{ File string } }
	if err := json.Unmarshal([]byte(s.w.stdout), &object); err != nil {
		return nil, fmt.Errorf("standard output is not units' object: %v\n%s", err, s.w.report())
	}
	var files []string
	for _, u := range object.Units {
		files = append(files, filepath.ToSlash(u.File))
	}
	return files, nil
}

// namedRelative is whether the report named the file inside its working
// directory by its path from there.
func (s *selectionState) namedRelative() error {
	files, err := s.reportedFiles()
	if err != nil {
		return err
	}
	if !slices.Contains(files, insideFile) {
		return fmt.Errorf("no unit names %s: %v\n%s", insideFile, files, s.w.report())
	}
	return nil
}

// outsideAbsolute is whether the report named the file outside its
// working directory by an absolute path, the scratch folder found in it by
// its name, as the system may name it through another path.
func (s *selectionState) outsideAbsolute() error {
	files, err := s.reportedFiles()
	if err != nil {
		return err
	}
	tail := "/" + filepath.Base(s.w.dir) + "/" + strings.TrimPrefix(outsideFile, "../")
	for _, file := range files {
		if strings.HasSuffix(file, tail) && filepath.IsAbs(filepath.FromSlash(file)) {
			return nil
		}
	}
	return fmt.Errorf("no unit names %s by an absolute path: %v\n%s", tail, files, s.w.report())
}

// scoredNotOther is whether crap scored a function of scored and none of
// other.
func (s *selectionState) scoredNotOther(scored, other string) error {
	files, err := s.selected()
	if err != nil {
		return err
	}
	if !slices.Contains(files, scored) || slices.Contains(files, other) {
		return fmt.Errorf("crap scores functions of %v; want %s and not %s\n%s", files, scored, other, s.w.report())
	}
	return nil
}
