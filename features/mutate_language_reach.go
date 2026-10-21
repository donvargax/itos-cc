package features

import (
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

func (l *languageMutation) reachSteps(sc *godog.ScenarioContext) {
	var project languageReachProject
	sc.Step(`^a (typescript|python|kotlin|dart) project whose test imports module A, where A imports module B and no test imports B directly$`, func(language string) error {
		l.language = language
		project = languageReachProjects[language]
		if language == "dart" {
			project = dartReachProject
		}
		return l.reachProject(project)
	})
	sc.Step(`^a (python|typescript|kotlin|dart) project whose test reaches module A only through (a helper module under tests/|a conftest\.py fixture above it|a helper module under __tests__/|a helper class in src/test|a helper library under test/|a flutter_test_config\.dart above it), which imports A$`, func(language, support string) error {
		l.language = language
		kind := "helper"
		if strings.Contains(support, "conftest") || strings.Contains(support, "flutter_test_config") {
			kind = "conftest"
		}
		project = languageSupportProjects[language+" "+kind]
		switch language + " " + kind {
		case "dart helper":
			project = dartHelperProject
		case "dart conftest":
			project = dartConfigProject
		}
		return l.reachProject(project)
	})
	sc.Step(`^a Flutter package "demo" whose test reaches lib/a\.dart only through (.+)$`, func(link string) error {
		l.language = "dart"
		project = dartLinkProjects[link]
		return l.reachProject(project)
	})
	sc.Step(`^(?:A|B)'s mutant has a recorded result$`, l.recordReach)
	sc.Step(`^a mutant of lib/a\.dart has a recorded result$`, l.recordReach)
	sc.Step(`^that test changes$`, func() error { return l.appendComment(project.reaching) })
	sc.Step(`^"itos-cc mutation check" reports (?:A|B)'s result stale, naming the test$`, func() error { return l.reachStale(project) })
	sc.Step(`^"itos-cc mutation check" reports that result stale, naming the test$`, func() error { return l.reachStale(project) })
	sc.Step(`^a test that (?:reaches neither A nor B|uses no support reaching A|reaches no library linking to lib/a\.dart) changing leaves (?:A's|B's|that) result fresh$`, func() error {
		// Restore the reaching test's actual bytes, then change only the unrelated one.
		if err := l.m.w.write(project.reaching, project.files[project.reaching]); err != nil {
			return err
		}
		if err := l.appendComment(project.others); err != nil {
			return err
		}
		return l.reachFresh(project)
	})
	sc.Step(`^in a Go module a test still reaches only its own package and the packages it imports, not a package two imports away$`, func() error {
		l.language = "go"
		p := languageReachProjects["go"]
		if err := l.reachProject(p); err != nil {
			return err
		}
		if err := l.recordReach(); err != nil {
			return err
		}
		if err := l.appendComment(p.reaching); err != nil {
			return err
		}
		return l.reachFresh(p)
	})
}
func (l *languageMutation) reachProject(p languageReachProject) error {
	if len(p.files) == 0 {
		return fmt.Errorf("missing reach fixture")
	}
	l.file = p.b
	for file, text := range p.files {
		if err := l.m.w.write(file, text); err != nil {
			return err
		}
	}
	l.m.w.imports(p.edges)
	return nil
}
func (l *languageMutation) recordReach() error {
	w := l.m.w
	// A real public run establishes the current source and importer hashes. The
	// raw fixture then represents the historical own-scope result under test.
	sites, err := l.m.sitesOf(l.file)
	if err != nil {
		return err
	}
	// Its sites are named from the folder the tests are declared for.
	root, file := l.reachRoot()
	t := Tests{Language: "go", Command: "go version"}
	for _, s := range sites {
		t.Surviving = append(t.Surviving, Site{File: file, Line: s.line, Original: s.original, Replacement: s.replacement})
	}
	if err := w.d.Tests(root, t); err != nil {
		return err
	}
	a, log, code, err := l.capture("mutation run --json --no-coverage --test-command 'go version' " + l.file)
	if err != nil {
		return err
	}
	if code != 1 {
		return fmt.Errorf("record reach result exit%d: %v\n%s", code, a, log)
	}
	snap, err := l.s.snapshot(l.file)
	if err != nil {
		return err
	}
	for _, u := range objectList(snap, "units") {
		for _, m := range objectList(u, "mutants") {
			delete(m, "scope")
			delete(m, "suite_evidence")
			delete(m, "go_evidence")
		}
	}
	return l.writeSnapshot(l.file, snap)
}

// reachRoot is where the recorded run's command runs, with the file's
// slash path from there: a Dart file's package, the folder of the nearest
// pubspec.yaml, and the scratch folder for the other languages.
func (l *languageMutation) reachRoot() (string, string) {
	w := l.m.w
	if l.language == "dart" {
		for dir := path.Dir(l.file); dir != "."; dir = path.Dir(dir) {
			if w.d.Exists(filepath.Join(w.dir, filepath.FromSlash(dir), "pubspec.yaml")) {
				return filepath.Join(w.dir, filepath.FromSlash(dir)), strings.TrimPrefix(l.file, dir+"/")
			}
		}
	}
	return w.dir, l.file
}
func (l *languageMutation) writeSnapshot(file string, a map[string]any) error {
	b, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return l.m.w.write(".metrics/mutate/"+file+".json", string(b)+"\n")
}
func (l *languageMutation) appendComment(file string) error {
	comment := "\n// changed\n"
	if strings.HasSuffix(file, ".py") {
		comment = "\n# changed\n"
	}
	return l.m.w.write(file, l.m.read(file)+comment)
}
func (l *languageMutation) reachStale(p languageReachProject) error {
	a, _, code, err := l.capture("mutation check --json " + p.b)
	if err != nil {
		return err
	}
	f, err := languageRunFile(a, p.b)
	if err != nil {
		return err
	}
	units := objectList(f, "functions")
	if len(units) == 0 || code != 1 {
		return fmt.Errorf("missing stale reach units exit%d: %v", code, a)
	}
	for _, u := range units {
		if u["state"] != "stale" {
			return fmt.Errorf("reaching test leaves non-stale result: %v", u)
		}
	}
	found := false
	for _, pbl := range checkProblems(a) {
		if pbl["rule"] == "mutation.stale" && filepath.ToSlash(fmt.Sprint(pbl["file"])) == p.b && strings.Contains(fmt.Sprint(pbl["message"]), p.reaching) {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("stale problem does not name reaching test %s: %v", p.reaching, a)
	}
	return nil
}
func (l *languageMutation) reachFresh(p languageReachProject) error {
	a, _, _, err := l.capture("mutation check --json " + p.b)
	if err != nil {
		return err
	}
	f, err := languageRunFile(a, p.b)
	if err != nil {
		return err
	}
	units := objectList(f, "functions")
	if len(units) == 0 {
		return fmt.Errorf("missing fresh reach functions: %v", f)
	}
	for _, u := range units {
		if u["state"] != "fresh" {
			return fmt.Errorf("unrelated test made function stale: %v", u)
		}
	}
	return nil
}
