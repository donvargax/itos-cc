// A function's comments are no part of its hash (slice-11, issue #22): a
// commit that only touches them leaves its results fresh and --since does
// not judge it.
package features

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

type commentMutation struct {
	m *mutateState
	s *sinceMutation
	// language is the fixture a language's Given wrote, and file and
	// function its.
	language, file, function string
}

// commentLanguage is a project with one function and one mutation site,
// return i > 5, in a language other than Go.
type commentLanguage struct {
	files                    map[string]string
	edges                    map[string][]string
	file, function           string
	comment, indent, siteEnd string
}

var commentLanguages = map[string]commentLanguage{
	"typescript": {
		files: map[string]string{
			"package.json": vitestPackage,
			"src/board.ts": "export function high(i: number): boolean {\n  return i > 5;\n}\n",
			"src/board.test.ts": "import {expect, test} from \"vitest\";\nimport {high} from \"./board\";\n\n" +
				"test(\"high\", () => {\n  expect(high(6)).toBe(true);\n  expect(high(5)).toBe(false);\n});\n",
		},
		edges: map[string][]string{"src/board.test.ts": {"src/board.ts"}},
		file:  "src/board.ts", function: "high", comment: "//", indent: "  ", siteEnd: "return i > 5;",
	},
	"python": {
		files: map[string]string{
			"pyproject.toml":      "[project]\nname = \"board\"\nversion = \"0.0.0\"\n",
			"board.py":            "def high(i):\n    return i > 5\n",
			"tests/test_board.py": "from board import high\n\n\ndef test_high():\n    assert high(6) and not high(5)\n",
		},
		edges: map[string][]string{"tests/test_board.py": {"board.py"}},
		file:  "board.py", function: "high", comment: "#", indent: "    ", siteEnd: "return i > 5",
	},
	"kotlin": {
		files: map[string]string{
			"settings.gradle.kts":                "rootProject.name = \"m\"\n",
			"build.gradle.kts":                   "plugins {\n    kotlin(\"jvm\") version \"2.4.21\"\n}\n",
			"src/main/kotlin/board/Board.kt":     "package board\n\nfun high(i: Int): Boolean {\n    return i > 5\n}\n",
			"src/test/kotlin/board/BoardTest.kt": "package board\n\nclass BoardTest {\n    fun high() = high(6)\n}\n",
		},
		edges: map[string][]string{"src/test/kotlin/board/BoardTest.kt": {"src/main/kotlin/board/Board.kt"}},
		file:  "src/main/kotlin/board/Board.kt", function: "high", comment: "//", indent: "    ", siteEnd: "return i > 5",
	},
}

func commentFreeSteps(sc *godog.ScenarioContext, m *mutateState, s *sinceMutation) {
	c := &commentMutation{m: m, s: s}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*c = commentMutation{m: m, s: s}
		return ctx, nil
	})
	sc.Step(`^a commit after "base" only (.+) inside "Board#Place"$`, c.commentOnlyPlace)
	sc.Step(`^"itos-cc mutation check --json ([^"]+)" gives "([^"]+)" the state "([^"]+)"$`, c.stateOf)
	sc.Step(`^"itos-cc mutation run --since base" judges no function and runs no mutant$`, c.judgesNothing)
	sc.Step(`^after a commit that changes its code, "itos-cc mutation check --since base" reports "mutation\.stale" for "Board#Place"$`, c.codeChangeStales)
	sc.Step(`^"Board#Reset" has no mutation results, and a commit after "base" only added a comment at the end of one of its lines$`, c.resetUnrecorded)
	sc.Step(`^a (typescript|python|kotlin) function with fresh results, all killed, committed at "base"$`, c.languageFresh)
	sc.Step(`^a commit after "base" only added a comment on a line of its own above its mutation site, and another at the end of that site's line$`, c.languageComments)
	sc.Step(`^"itos-cc mutation check --json" of its file gives the function the state "fresh"$`, func() error { return c.stateOf(c.file, c.function, "fresh") })
}

// commentOnlyPlace commits a change to Board#Place's comments alone, as the
// example says.
func (c *commentMutation) commentOnlyPlace(changed string) error {
	s := c.s
	var old, new string
	switch changed {
	case "added a comment on a line of its own above its mutation site":
		old, new = "\treturn i > 5", "\t// over five\n\treturn i > 5"
	case "added a comment at the end of its mutation site's line":
		old, new = "return i > 5", "return i > 5 // over five"
	case "reworded its comment":
		old, new = "// past five", "// more than five"
	case "deleted its comment's line":
		old, new = "\t// past five\n", ""
	default:
		return fmt.Errorf("unknown comment change %q", changed)
	}
	if err := s.replace(s.file, old, new); err != nil {
		return err
	}
	return s.commit("only a comment of Place")
}

// stateOf runs mutation check --json on file and holds function to state.
func (c *commentMutation) stateOf(file, function, state string) error {
	w := c.m.w
	_, out, stderr, err := w.capture(w.dir, w.env(), "mutation", "check", "--json", filepath.FromSlash(file))
	if err != nil {
		return err
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		return fmt.Errorf("check --json: %v\n%s%s", err, out, stderr)
	}
	f, err := languageRunFile(obj, file)
	if err != nil {
		return err
	}
	for _, u := range objectList(f, "functions") {
		if name := fmt.Sprint(u["function"]); sinceFunction(listedSite{function: name}, function) || strings.HasSuffix(name, "#"+function) {
			if u["state"] != state {
				return fmt.Errorf("%s is %v, not %s: %v", function, u["state"], state, u)
			}
			return nil
		}
	}
	return fmt.Errorf("check lists no %s: %v", function, f)
}

// judgesNothing runs mutation run --since base and holds it to judging no
// function of the file and running no mutant.
func (c *commentMutation) judgesNothing() error {
	w, file := c.m.w, c.s.file
	if c.file != "" {
		file = c.file
	}
	if err := w.runWith(w.env(), "mutation run --since base --json --no-annotate"); err != nil {
		return err
	}
	obj, err := w.json()
	if err != nil {
		return err
	}
	files, _ := obj["files"].([]any)
	for _, one := range files {
		f, _ := one.(map[string]any)
		if judged, _ := f["judged"].([]any); filepath.ToSlash(fmt.Sprint(f["file"])) == file && len(judged) > 0 {
			return fmt.Errorf("judged %v\n%s", judged, w.report())
		}
	}
	if trials := trialsOf(w.stderr, file); len(trials) > 0 {
		return fmt.Errorf("mutants ran: %v\n%s", trials, w.report())
	}
	return w.exitsWith(0)
}

// codeChangeStales commits a change to Place's code, which keeps what its
// mutants do, and holds check --since base to finding its results stale.
func (c *commentMutation) codeChangeStales() error {
	s, w := c.s, c.m.w
	if err := s.replace(s.file, "i > 5", "(i > 5)"); err != nil {
		return err
	}
	if err := s.commit("change Place's code"); err != nil {
		return err
	}
	if err := w.runWith(w.env(), "mutation check --since base --json"); err != nil {
		return err
	}
	obj, err := w.json()
	if err != nil {
		return err
	}
	for _, p := range checkProblems(obj) {
		if p["rule"] == "mutation.stale" && sinceFunction(listedSite{function: fmt.Sprint(p["function"])}, s.place) {
			return w.exitsWith(1)
		}
	}
	return fmt.Errorf("no mutation.stale for %s\n%s", s.place, w.report())
}

// resetUnrecorded adds Board#Reset after the results were recorded, moves
// base past it, then commits a comment at the end of one of its lines.
func (c *commentMutation) resetUnrecorded() error {
	s, w := c.s, c.m.w
	if err := s.renameProject(); err != nil {
		return err
	}
	if err := w.write(s.file, s.m.read(s.file)+"\nfunc (b *Board) Reset(i int) bool {\n\treturn i == 0\n}\n"); err != nil {
		return err
	}
	if err := s.commit("add Reset"); err != nil {
		return err
	}
	if err := w.git("tag", "-f", "base"); err != nil {
		return err
	}
	if err := s.replace(s.file, "return i == 0", "return i == 0 // empty"); err != nil {
		return err
	}
	return s.commit("only a comment of Reset")
}

// languageFresh records the language's function's every mutant killed,
// rewriting a run's survivors as killed results of its own tests, so no
// language tool is needed, and commits everything at base.
func (c *commentMutation) languageFresh(language string) error {
	p := commentLanguages[language]
	w := c.m.w
	c.language, c.file, c.function = language, p.file, p.function
	c.m.file = p.file
	if err := c.m.writeProject(p.edges, p.files); err != nil {
		return err
	}
	sites, err := c.m.sitesOf(p.file)
	if err != nil {
		return err
	}
	if len(sites) == 0 {
		return fmt.Errorf("%s has no mutation site", p.file)
	}
	t := Tests{Language: "go", Command: "go version"}
	for _, site := range sites {
		t.Surviving = append(t.Surviving, Site{File: p.file, Line: site.line, Original: site.original, Replacement: site.replacement})
	}
	if err := w.d.Tests(w.dir, t); err != nil {
		return err
	}
	words, err := commandWords("mutation run --json --no-coverage --no-annotate --test-command 'go version' " + p.file)
	if err != nil {
		return err
	}
	code, out, log, err := w.capture(w.dir, w.env(), words...)
	if err != nil {
		return err
	}
	if code != 1 {
		return fmt.Errorf("recording %s exit %d\n%s%s", p.file, code, out, log)
	}
	snap, err := c.s.snapshot(p.file)
	if err != nil {
		return err
	}
	// The run's own scope, its suite evidence kept, every mutant killed.
	for _, u := range objectList(snap, "units") {
		for _, mu := range objectList(u, "mutants") {
			delete(mu, "scope")
			mu["outcome"] = "killed"
		}
		u["killed"], u["survived"] = len(objectList(u, "mutants")), 0
	}
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	if err := w.write(".metrics/mutate/"+p.file+".json", string(b)+"\n"); err != nil {
		return err
	}
	if err := c.stateOf(p.file, p.function, "fresh"); err != nil {
		return fmt.Errorf("the recorded results: %w", err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-qm", "base"}, {"tag", "base"}} {
		if err := w.git(args...); err != nil {
			return err
		}
	}
	return nil
}

// languageComments commits a comment above the site and one at the end of
// its line, in the language's own comment syntax.
func (c *commentMutation) languageComments() error {
	p := commentLanguages[c.language]
	text := c.m.read(c.file)
	site := p.indent + p.siteEnd
	if !strings.Contains(text, site) {
		return fmt.Errorf("%s lacks %q", c.file, site)
	}
	text = strings.Replace(text, site, p.indent+p.comment+" over five\n"+site+" "+p.comment+" over five", 1)
	if err := c.m.w.write(c.file, text); err != nil {
		return err
	}
	return c.s.commit("only comments of " + c.function)
}
