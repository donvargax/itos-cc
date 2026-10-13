// The steps of mutate.feature: mutation list, run, check, sample and except
// over projects the Givens write. Like every step here, they read the
// binary's output and the files it leaves alone, never its code. Its
// scenarios gain steps a piece at a time (Unstepped, steps.go); those of
// the pieces not done yet are held by cmd/itos-cc's Go tests.
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
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

// mutateState is one mutate.feature scenario's state beside the world's.
type mutateState struct {
	w *world

	// file is the source the scenario's mutation list listed, as the run
	// named it, and sites what it listed.
	file  string
	sites []listedSite
	// setup is src/setup.go's text as the previous run left it.
	setup string
	// snapshot is the snapshot a Given recorded of an excluded source,
	// slash-separated from the scratch folder, and source that source.
	snapshot, source string
}

func mutateSteps(sc *godog.ScenarioContext, w *world) {
	m := &mutateState{w: w}
	sc.Before(func(ctx context.Context, s *godog.Scenario) (context.Context, error) {
		*m = mutateState{w: w}
		if filepath.Base(s.Uri) != "mutate.feature" {
			return ctx, nil
		}
		// no tests are run (steps.go) looks for a mark a test would leave,
		// .metrics and a baseline, as cli.feature's no test command runs.
		w.noTests = (&cliState{w: w}).noTestRuns
		for _, tag := range s.Tags {
			if setUp := mutateSamples[tag.Name]; setUp != nil {
				return ctx, setUp(m)
			}
		}
		return ctx, nil
	})

	// Mutation sites.
	sc.Step(`^the sites are:$`, m.sitesAre)
	sc.Step(`^code outside functions, comments, strings, and type arguments hold no sites$`, m.noSitesOutsideCode)
	sc.Step(`^a\?\.\(\) becomes a\(\), as a\?\.\[0\] becomes a\[0\]$`, m.optionalChainsLoseTheirMark)
	sc.Step(`^(\.\w+(?:, \.\w+)*,? and \.\w+) files have the same sites$`, m.sameSitesIn)
	sc.Step(`^each site is printed as "([^"]*)"$`, m.eachSitePrintedAs)
	sc.Step(`^src/app\.ts has a function with a site before, inside and after an inline callback$`, m.appWithCallback)
	sc.Step(`^its sites are listed in line and column order$`, m.inLineOrder)
	sc.Step(`^in the order "itos-cc ([^"]*)" lists the file's "([^"]*)"$`, m.inTheOrderOf)

	// Functions sharing a name.
	sc.Step(`^src/setup\.go declares two init functions with different bodies, each with a mutant the tests kill$`, m.setupProject)
	sc.Step(`^a previous run recorded both$`, m.previousRun)
	sc.Step(`^a previous run recorded both init functions of src/setup\.go, all killed$`, func() error {
		if err := m.setupProject(); err != nil {
			return err
		}
		return m.previousRun()
	})
	sc.Step(`^I run "itos-cc ?([^"]*)" with nothing changed$`, m.runUnchanged)
	sc.Step(`^the mutants of both init functions are reused, and none runs$`, m.bothReused)
	sc.Step(`^the second init function has changed since$`, m.secondChanged)
	sc.Step(`^the two have swapped places since, unchanged$`, m.initsSwapped)
	sc.Step(`^the only problem is "([^"]*)", for the second init function$`, m.onlyProblemOfSecond)
	sc.Step(`^"itos-cc ([^"]*)" runs only the second init function's mutants$`, m.runsOnlySecond)
	sc.Step(`^"itos-cc ([^"]*)" reuses every mutant$`, m.reusesEvery)

	// Sources itos-cc.yaml excludes.
	sc.Step(`^the only commit since main changes (\S+), whose mutants no test reaches$`, m.onlyCommitChanges)
	sc.Step(`^stderr says "(.*)", and the exit code is (\d+)$`, func(text string, code int) error {
		if err := w.stderrSays(text); err != nil {
			return err
		}
		return w.exitsWith(code)
	})
	sc.Step(`^"itos-cc ([^"]*)" exits (\d+)$`, func(args string, code int) error {
		if err := w.runWith(w.env(), args); err != nil {
			return err
		}
		return w.exitsWith(code)
	})
	sc.Step(`^neither "itos-cc ([^"]*)" nor "itos-cc ([^"]*)" names a site in (\S+)$`, m.neitherNames)
	sc.Step(`^a snapshot under \.metrics/mutate for (\S+), and fresh results for every other source$`, m.excludedSnapshot)
	sc.Step(`^itos-cc\.yaml then excludes "([^"]*)"$`, func(glob string) error {
		return w.write("itos-cc.yaml", "exclude: ["+strconv.Quote(glob)+"]\n")
	})
	sc.Step(`^its "([^"]*)" list (\S+) with "([^"]*)" "([^"]*)", and the exit code is (\d+)$`, func(key, file, name, value string, code int) error {
		if err := m.lists(key, file, name, value); err != nil {
			return err
		}
		return w.exitsWith(code)
	})
	sc.Step(`^the snapshot is removed, and its "([^"]*)" list it with "([^"]*)" "([^"]*)"$`, m.snapshotRemoved)
	sc.Step(`^itos-cc\.yaml excepts a survivor in (\S+) and excludes "([^"]*)"$`, m.exceptsAndExcludes)
	sc.Step(`^it reports "([^"]*)" with "([^"]*)" "([^"]*)"$`, m.reports)
	sc.Step(`^the itos-cc\.yaml HEAD commits excludes "([^"]*)"$`, m.headExcludes)
	sc.Step(`^no selected site is in (\S+)$`, m.noSelectedSiteIn)
}

// The projects of the scenarios that only say When I run, by their ID.
var mutateSamples = map[string]func(*mutateState) error{
	"@ID-MUT-06": (*mutateState).pythonSample,
}

// Samples.

// The Python project of ID-MUT-06: src/board.py, its sites none a
// deletion, and a test that leaves test-ran behind if anything runs it.
var (
	pythonSample = map[string]string{
		"pyproject.toml":      "[project]\nname = \"board\"\nversion = \"0\"\n",
		"src/board.py":        "def place(row, col):\n    return row > 0 and col < 9\n",
		"tests/test_board.py": "open(\"test-ran\", \"w\").write(\"ran\")\n\nfrom src.board import place\n\n\ndef test_place():\n    assert place(1, 2)\n",
	}
	// pythonSites are src/board.py's sites as mutation list prints them.
	pythonSites = []listedSite{
		{file: "src/board.py", line: 2, column: 16, original: ">", replacement: ">=", function: "board#place"},
		{file: "src/board.py", line: 2, column: 18, original: "0", replacement: "1", function: "board#place"},
		{file: "src/board.py", line: 2, column: 20, original: "and", replacement: "or", function: "board#place"},
		{file: "src/board.py", line: 2, column: 28, original: "<", replacement: "<=", function: "board#place"},
	}
)

func (m *mutateState) pythonSample() error {
	return m.writeAll(pythonSample)
}

// writeAll writes each file of files, by its slash-separated path from
// the scratch folder.
func (m *mutateState) writeAll(files map[string]string) error {
	for name, text := range files {
		if err := m.w.write(name, text); err != nil {
			return err
		}
	}
	return nil
}

// Sites.

// listedSite is a mutation site as mutation list names it: its file as
// the run names it, slash-separated, line and column, the text a mutant
// replaces and what replaces it, "" for a deletion, and its function.
type listedSite struct {
	file                  string
	line, column          int
	original, replacement string
	function              string
}

func (s listedSite) String() string {
	replacement := "`" + s.replacement + "`"
	if s.replacement == "" {
		replacement = "(deleted)"
	}
	return fmt.Sprintf("%s:%d:%d `%s` → %s in %s", s.file, s.line, s.column, s.original, replacement, s.function)
}

// listedLine is one site of mutation list's plain output.
var listedLine = regexp.MustCompile("^(\\S+):(\\d+):(\\d+) `([^`]+)` → (?:`([^`]*)`|\\(deleted\\)) in (.+)$")

// listedSites are the sites stdout, mutation list's plain output, lists,
// every line one.
func listedSites(stdout string) ([]listedSite, error) {
	var sites []listedSite
	for _, line := range outputLines(strings.ReplaceAll(stdout, "\r\n", "\n")) {
		match := listedLine.FindStringSubmatch(line)
		if match == nil {
			return nil, fmt.Errorf("%q is no site's line", line)
		}
		lineNo, _ := strconv.Atoi(match[2])
		column, _ := strconv.Atoi(match[3])
		sites = append(sites, listedSite{file: filepath.ToSlash(match[1]), line: lineNo, column: column,
			original: match[4], replacement: match[5], function: match[6]})
	}
	return sites, nil
}

// sitesOf is the sites mutation list --json lists of file, a path from the
// scratch folder.
func (m *mutateState) sitesOf(file string) ([]listedSite, error) {
	code, stdout, stderr, err := m.w.capture(m.w.dir, m.w.env(), "mutation", "list", "--json", filepath.FromSlash(file))
	if err != nil {
		return nil, err
	}
	var object struct {
		Sites []struct {
			File, Function, Original, Replacement string
			Line, Column                          int
		}
	}
	if err := json.Unmarshal([]byte(stdout), &object); err != nil || code != 0 {
		return nil, fmt.Errorf("mutation list --json %s exited %d, its output no object of sites (%v)\n%s%s", file, code, err, stdout, stderr)
	}
	var sites []listedSite
	for _, s := range object.Sites {
		sites = append(sites, listedSite{file: filepath.ToSlash(s.File), line: s.Line, column: s.Column,
			original: s.Original, replacement: s.Replacement, function: s.Function})
	}
	return sites, nil
}

// sitesAre is whether the last run, mutation list of one file, listed
// exactly the table's sites, in its order: their line, the text replaced
// and what replaces it, "(deleted)" for nothing.
func (m *mutateState) sitesAre(table *godog.Table) error {
	if err := m.listed(); err != nil {
		return err
	}
	var want, got []string
	for _, row := range table.Rows[1:] {
		want = append(want, strings.Join([]string{row.Cells[0].Value, row.Cells[1].Value, row.Cells[2].Value}, " "))
	}
	for _, s := range m.sites {
		replacement := s.replacement
		if replacement == "" {
			replacement = "(deleted)"
		}
		got = append(got, strings.Join([]string{strconv.Itoa(s.line), s.original, replacement}, " "))
	}
	if !slices.Equal(got, want) {
		return fmt.Errorf("the sites listed are\n%s\nnot\n%s\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"), m.w.report())
	}
	return nil
}

// listed reads the last run's sites: a mutation list of one file, which
// exited 0.
func (m *mutateState) listed() error {
	w := m.w
	if w.last == nil || len(w.last.args) == 0 || w.exit != 0 {
		return fmt.Errorf("no mutation list of a file ran and exited 0\n%s", w.report())
	}
	sites, err := listedSites(w.stdout)
	if err != nil {
		return fmt.Errorf("%v\n%s", err, w.report())
	}
	m.file, m.sites = filepath.ToSlash(w.last.args[len(w.last.args)-1]), sites
	return nil
}

// span is a piece of a line of a source: what it is, its line, and the
// columns it starts at and ends before, counted from 1.
type span struct {
	kind           string
	line, from, to int
}

// The pieces of a TypeScript source that are no code of a function, as
// MUT-01's file writes each.
var (
	commentText  = regexp.MustCompile(`//(.*)`)
	stringText   = regexp.MustCompile(`"[^"]*"`)
	typeArgument = regexp.MustCompile(`\w(<[\w, ]+>)`)
	// wouldBeSite is a text that would be a site in a function's code: a
	// comparison, a logical or arithmetic operator, a 0 or 1, or a boolean;
	// an assignment's = is none.
	wouldBeSite = regexp.MustCompile(`[<>]|[=!]==?|&&|\|\||[!+*/-]|\b[01]\b|\btrue\b|\bfalse\b`)
)

// noSitesOutsideCode is whether the file the last run listed holds code
// outside every function, a comment, a string and a type argument, each
// with a text that would be a site in a function's code, and no site the
// run listed is in one.
func (m *mutateState) noSitesOutsideCode() error {
	if m.file == "" {
		if err := m.listed(); err != nil {
			return err
		}
	}
	data, err := m.w.readIn(m.w.dir, m.file)
	if err != nil {
		return err
	}
	var spans []span
	depth := 0
	for i, line := range strings.Split(string(data), "\n") {
		n := i + 1
		trimmed := strings.TrimSpace(line)
		if depth == 0 && trimmed != "" && trimmed != "}" && !strings.Contains(line, "function ") {
			spans = append(spans, span{"code outside functions", n, 1, len(line) + 1})
		}
		depth += strings.Count(line, "{") - strings.Count(line, "}")
		for _, piece := range []struct {
			kind string
			text *regexp.Regexp
			sub  int
		}{{"comment", commentText, 1}, {"string", stringText, 0}, {"type argument", typeArgument, 1}} {
			for _, at := range piece.text.FindAllStringSubmatchIndex(line, -1) {
				spans = append(spans, span{piece.kind, n, at[2*piece.sub] + 1, at[2*piece.sub+1] + 1})
			}
		}
	}
	lines := strings.Split(string(data), "\n")
	for _, kind := range []string{"code outside functions", "comment", "string", "type argument"} {
		if !slices.ContainsFunc(spans, func(s span) bool {
			return s.kind == kind && wouldBeSite.MatchString(lines[s.line-1][s.from-1:s.to-1])
		}) {
			return fmt.Errorf("%s holds no %s with a text that would be a site in a function's code", m.file, kind)
		}
	}
	for _, site := range m.sites {
		for _, s := range spans {
			if site.line == s.line && site.column >= s.from && site.column < s.to {
				return fmt.Errorf("the site %s is in a %s of %s\n%s", site, s.kind, m.file, m.w.report())
			}
		}
	}
	return nil
}

// chains is a TypeScript file whose functions call a function and index a
// list through an optional chain, and what each line is, its ?. deleted.
const chains = "export function call(a) {\n  return a?.();\n}\n\nexport function index(a) {\n  return a?.[0];\n}\n"

var chainsMutated = map[int]string{2: "  return a();", 6: "  return a[0];"}

// optionalChainsLoseTheirMark is whether mutation list gives a?.() one
// site, the ?. deleted, making it a(), and a?.[0] one, making it a[0].
func (m *mutateState) optionalChainsLoseTheirMark() error {
	if err := m.w.write("chains.ts", chains); err != nil {
		return err
	}
	sites, err := m.sitesOf("chains.ts")
	if err != nil {
		return err
	}
	lines := strings.Split(chains, "\n")
	for line, want := range chainsMutated {
		var got []string
		for _, s := range sites {
			if s.line == line && s.original == "?." {
				text := lines[line-1]
				got = append(got, text[:s.column-1]+s.replacement+text[s.column-1+len(s.original):])
			}
		}
		if !slices.Equal(got, []string{want}) {
			return fmt.Errorf("the ?. of %q mutates to %q, not %q alone; the sites are %v", lines[line-1], got, want, sites)
		}
	}
	return nil
}

// sameSitesIn is whether a copy of the listed file under each extension
// the sentence names has the sites it has, at the same places, in the same
// functions.
func (m *mutateState) sameSitesIn(extensions string) error {
	if m.file == "" {
		return errors.New("no file's sites were listed to compare")
	}
	data, err := m.w.readIn(m.w.dir, m.file)
	if err != nil {
		return err
	}
	want, err := m.sitesOf(m.file)
	if err != nil {
		return err
	}
	stem := strings.TrimSuffix(m.file, path.Ext(m.file))
	for _, ext := range regexp.MustCompile(`\.\w+`).FindAllString(extensions, -1) {
		name := stem + ext
		if err := m.w.write(name, string(data)); err != nil {
			return err
		}
		got, err := m.sitesOf(name)
		if err != nil {
			return err
		}
		if len(got) == 0 || !slices.EqualFunc(got, want, func(a, b listedSite) bool {
			a.file, b.file = "", ""
			return a == b
		}) {
			return fmt.Errorf("%s has the sites %v, not %s's %v", name, got, m.file, want)
		}
	}
	return nil
}

// siteParts are the words of a site's format, each with what it matches.
var siteParts = map[string]string{
	"file": `(\S+?)`, "line": `(\d+)`, "column": `(\d+)`, "original": "([^`]+)", "replacement": "([^`]*)",
	"namespace": `([\w.$/-]+)`, "name": `(\w+)`,
}

// eachSitePrintedAs is whether every line of the last run's stdout is a
// site of ID-MUT-06's src/board.py in the format, its words file, line,
// column, original, replacement, namespace and name standing for the
// site's, and every site of the file is printed so, in order.
func (m *mutateState) eachSitePrintedAs(format string) error {
	var order []string
	words := regexp.MustCompile(`\b(?:file|line|column|original|replacement|namespace|name)\b`)
	pattern := words.ReplaceAllStringFunc(regexp.QuoteMeta(format), func(word string) string {
		order = append(order, word)
		return siteParts[word]
	})
	line := regexp.MustCompile("^" + pattern + "$")
	var got, want []string
	for _, text := range outputLines(strings.ReplaceAll(m.w.stdout, "\r\n", "\n")) {
		match := line.FindStringSubmatch(text)
		if match == nil {
			return fmt.Errorf("%q is not printed as %q\n%s", text, format, m.w.report())
		}
		fields := map[string]string{}
		for i, word := range order {
			fields[word] = match[i+1]
		}
		got = append(got, fmt.Sprintf("%s:%s:%s %s %s %s#%s", filepath.ToSlash(fields["file"]), fields["line"], fields["column"],
			fields["original"], fields["replacement"], fields["namespace"], fields["name"]))
	}
	for _, s := range pythonSites {
		want = append(want, fmt.Sprintf("%s:%d:%d %s %s %s", s.file, s.line, s.column, s.original, s.replacement, s.function))
	}
	if !slices.Equal(got, want) {
		return fmt.Errorf("the sites printed are\n%s\nnot\n%s\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"), m.w.report())
	}
	return nil
}

// The project of ID-MUT-101: src/app.ts's routes, of a site on line 2,
// before the route callback app.get("/u", …), and of two on line 7, after
// it, the callback a function of its own with a site on line 4; its test
// script passes whatever is mutated, so every mutant survives.
const appTS = `export function routes(app, a) {
  if (a > 1) {
    app.get("/u", (req, res) => {
      return req.x < 2;
    });
  }
  return a === 0;
}
`

var appSites = []Site{
	{"src/app.ts", 2, ">", ">="}, {"src/app.ts", 2, "1", "0"}, {"src/app.ts", 4, "<", "<="},
	{"src/app.ts", 7, "===", "!=="}, {"src/app.ts", 7, "0", "1"},
}

// appWithCallback writes the project, declares its tests, and checks the
// function has sites on the lines before, inside and after the callback,
// those inside in a function of their own.
func (m *mutateState) appWithCallback() error {
	if err := m.writeAll(map[string]string{
		"package.json": `{"name": "m", "scripts": {"test": "exit 0"}}` + "\n",
		"src/app.ts":   appTS,
	}); err != nil {
		return err
	}
	if err := m.w.d.Tests(m.w.dir, Tests{Language: "typescript", Command: "npm run test --silent", Surviving: appSites}); err != nil {
		return err
	}
	sites, err := m.sitesOf("src/app.ts")
	if err != nil {
		return err
	}
	byLine := map[int]string{}
	var got []Site
	for _, s := range sites {
		if f, ok := byLine[s.line]; ok && f != s.function {
			return fmt.Errorf("the sites of line %d are in two functions: %v", s.line, sites)
		}
		byLine[s.line] = s.function
		got = append(got, Site{s.file, s.line, s.original, s.replacement})
	}
	if !slices.Equal(got, appSites) || byLine[2] != byLine[7] || byLine[4] == byLine[2] {
		return fmt.Errorf("src/app.ts's sites are %v, not those of lines 2 and 7 in one function and line 4's in the callback", sites)
	}
	return nil
}

// inLineOrder is whether the last run listed every site of its file, by
// line and then column.
func (m *mutateState) inLineOrder() error {
	if err := m.listed(); err != nil {
		return err
	}
	want, err := m.sitesOf(m.file)
	if err != nil {
		return err
	}
	if len(m.sites) == 0 || len(m.sites) != len(want) {
		return fmt.Errorf("the run listed %d sites of %s's %d\n%s", len(m.sites), m.file, len(want), m.w.report())
	}
	if !slices.IsSortedFunc(m.sites, func(a, b listedSite) int {
		if a.line != b.line {
			return a.line - b.line
		}
		return a.column - b.column
	}) {
		return fmt.Errorf("the sites are not listed in line and column order\n%s", m.w.report())
	}
	return nil
}

// inTheOrderOf is whether the command's --json object lists the listed
// file's key, its mutants, as the last run listed its sites: the same
// places, texts and functions, in the same order.
func (m *mutateState) inTheOrderOf(args, key string) error {
	words, err := commandWords(args)
	if err != nil {
		return err
	}
	code, stdout, stderr, err := m.w.capture(m.w.dir, m.w.env(), words...)
	if err != nil {
		return err
	}
	var object struct {
		Files []map[string]json.RawMessage
	}
	if err := json.Unmarshal([]byte(stdout), &object); err != nil {
		return fmt.Errorf("itos-cc %s printed no JSON object: %v\nexit %d\n%s%s", args, err, code, stdout, stderr)
	}
	for _, file := range object.Files {
		var name string
		_ = json.Unmarshal(file["file"], &name)
		if filepath.ToSlash(name) != m.file {
			continue
		}
		var mutants []struct {
			Line, Column                    int
			Original, Replacement, Function string
		}
		if err := json.Unmarshal(file[key], &mutants); err != nil {
			return fmt.Errorf("%s's %q is no list of mutants: %v\n%s", m.file, key, err, stdout)
		}
		var got []listedSite
		for _, mu := range mutants {
			got = append(got, listedSite{file: m.file, line: mu.Line, column: mu.Column, original: mu.Original,
				replacement: mu.Replacement, function: mu.Function})
		}
		if len(got) == 0 || !slices.Equal(got, m.sites) {
			return fmt.Errorf("itos-cc %s lists %s's %s\n%v\nnot as mutation list did\n%v\n%s", args, m.file, key, got, m.sites, stderr)
		}
		return nil
	}
	return fmt.Errorf("itos-cc %s lists no file %s\nexit %d\n%s%s", args, m.file, code, stdout, stderr)
}

// Functions sharing a name.

// The Go module of "Rule: Functions sharing a name": src/setup.go's two
// init functions, both example.com/m/src#init, each setting a variable its
// test checks, so every mutant of both is killed.
const (
	mutateModule = "module example.com/m\n\ngo 1.22\n"
	initID       = "example.com/m/src#init"
	firstInit    = "func init() {\n\ta = 1 + 1\n}\n"
	secondInit   = "func init() {\n\tb = 3 - 1\n}\n"
	setupTest    = "package board\n\nimport \"testing\"\n\nfunc TestSetup(t *testing.T) {\n\tif a != 2 || b != 2 {\n\t\tt.Fatal(\"setup\")\n\t}\n}\n"
	setupSource  = "src/setup.go"
)

// goTests are the tests of the Go modules here, as mutation run runs them
// over src: every mutant they run is killed.
var goTests = Tests{Language: "go", Command: "go test -count=1 -failfast ./src"}

// setupProject writes the module, declares what its tests do and cover,
// and checks each init function has a site.
func (m *mutateState) setupProject() error {
	if err := m.writeAll(map[string]string{
		"go.mod":            mutateModule,
		setupSource:         "package board\n\nvar a, b int\n\n" + firstInit + "\n" + secondInit,
		"src/setup_test.go": setupTest,
	}); err != nil {
		return err
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "go", Report: "SF:src/setup.go\nDA:6,1\nDA:10,1\nend_of_record\n"}); err != nil {
		return err
	}
	if err := m.w.d.Tests(m.w.dir, goTests); err != nil {
		return err
	}
	inits, sites, err := m.initSites()
	if err != nil {
		return err
	}
	if bodies := strings.Split(m.read(setupSource), "func init() {"); len(inits) != 2 || len(bodies) != 3 || bodies[1] == bodies[2] {
		return fmt.Errorf("src/setup.go declares no two init functions with different bodies:\n%s", m.read(setupSource))
	}
	for i, in := range sites {
		if len(in) == 0 {
			return fmt.Errorf("init function %d of src/setup.go has no site: %v", i+1, sites)
		}
	}
	return nil
}

// read is the text of name, slash-separated from the scratch folder, ""
// when there is none.
func (m *mutateState) read(name string) string {
	data, _ := m.w.readIn(m.w.dir, name)
	return string(data)
}

// lineRange is the lines a function spans, its first and its last.
type lineRange struct{ first, last int }

func (l lineRange) holds(line int) bool { return line >= l.first && line <= l.last }

// initSites are the lines each init function of src/setup.go spans now, in
// file order, and the sites mutation list gives each, every one in
// example.com/m/src#init.
func (m *mutateState) initSites() ([]lineRange, [][]listedSite, error) {
	var inits []lineRange
	for i, line := range strings.Split(m.read(setupSource), "\n") {
		switch {
		case line == "func init() {":
			inits = append(inits, lineRange{first: i + 1})
		case line == "}" && len(inits) > 0 && inits[len(inits)-1].last == 0:
			inits[len(inits)-1].last = i + 1
		}
	}
	sites, err := m.sitesOf(setupSource)
	if err != nil {
		return nil, nil, err
	}
	each := make([][]listedSite, len(inits))
	for _, s := range sites {
		i := slices.IndexFunc(inits, func(l lineRange) bool { return l.holds(s.line) })
		if i < 0 || s.function != initID {
			return nil, nil, fmt.Errorf("the site %s is in no init function", s)
		}
		each[i] = append(each[i], s)
	}
	return inits, each, nil
}

// summary is what mutation run says of a file on stdout.
type summary struct{ killed, survived, uncovered, ran, reused int }

var summaryLine = regexp.MustCompile(`(?m)^(\S+): (\d+) killed, (\d+) survived, (\d+) uncovered \(ran (\d+), reused (\d+)\)\r?$`)

// summaryOf is what stdout, a mutation run's, says of file, and whether it
// says anything.
func summaryOf(stdout, file string) (summary, bool) {
	for _, match := range summaryLine.FindAllStringSubmatch(stdout, -1) {
		if filepath.ToSlash(match[1]) != file {
			continue
		}
		n := make([]int, 5)
		for i := range n {
			n[i], _ = strconv.Atoi(match[i+2])
		}
		return summary{n[0], n[1], n[2], n[3], n[4]}, true
	}
	return summary{}, false
}

// trialLine is mutation run's line on stderr for a mutant it ran.
var trialLine = regexp.MustCompile(`(?m)^itos-cc: \[\d+/\d+\] (\S+):(\d+) `)

// trials are the lines of the mutants of file a mutation run's stderr says
// it ran, one for each.
func trials(stderr, file string) []int {
	var out []int
	for _, match := range trialLine.FindAllStringSubmatch(stderr, -1) {
		if filepath.ToSlash(match[1]) == file {
			line, _ := strconv.Atoi(match[2])
			out = append(out, line)
		}
	}
	return out
}

// previousRun runs mutation run over src/setup.go, which must kill every
// mutant of both init functions, and keeps the text the run leaves.
func (m *mutateState) previousRun() error {
	_, each, err := m.initSites()
	if err != nil {
		return err
	}
	all := len(each[0]) + len(each[1])
	code, stdout, stderr, err := m.w.capture(m.w.dir, m.w.env(), "mutation", "run", filepath.FromSlash(setupSource))
	if err != nil {
		return err
	}
	if s, ok := summaryOf(stdout, setupSource); code != 0 || !ok || s != (summary{killed: all, ran: all}) {
		return fmt.Errorf("the previous run did not kill the %d mutants of both init functions: exit %d\n%s%s", all, code, stdout, stderr)
	}
	m.setup = m.read(setupSource)
	return nil
}

// runUnchanged runs itos-cc with args once src/setup.go is as the previous
// run left it.
func (m *mutateState) runUnchanged(args string) error {
	if m.setup == "" || m.read(setupSource) != m.setup {
		return errors.New("src/setup.go is not as a previous run left it")
	}
	return m.w.runWith(m.w.env(), args)
}

// bothReused is whether the last run reused every mutant of both init
// functions and ran none.
func (m *mutateState) bothReused() error {
	_, each, err := m.initSites()
	if err != nil {
		return err
	}
	s, ok := summaryOf(m.w.stdout, setupSource)
	if !ok || len(each[0]) == 0 || len(each[1]) == 0 || s.ran != 0 || s.reused != len(each[0])+len(each[1]) || len(trials(m.w.stderr, setupSource)) != 0 {
		return fmt.Errorf("the run did not reuse the %d and %d mutants of the two init functions alone\n%s", len(each[0]), len(each[1]), m.w.report())
	}
	return nil
}

// secondChanged changes the second init function's body.
func (m *mutateState) secondChanged() error {
	text := m.read(setupSource)
	if !strings.Contains(text, secondInit) {
		return fmt.Errorf("src/setup.go holds no second init function to change:\n%s", text)
	}
	return m.w.write(setupSource, strings.Replace(text, "b = 3 - 1", "b = 4 - 2", 1))
}

// initsSwapped swaps the two init functions, each unchanged.
func (m *mutateState) initsSwapped() error {
	text := m.read(setupSource)
	both := firstInit + "\n" + secondInit
	if !strings.Contains(text, both) {
		return fmt.Errorf("src/setup.go does not hold the two init functions in order:\n%s", text)
	}
	return m.w.write(setupSource, strings.Replace(text, both, secondInit+"\n"+firstInit, 1))
}

// onlyProblemOfSecond is whether the last run's one problem is of rule,
// naming the second init function of src/setup.go by its name and line.
func (m *mutateState) onlyProblemOfSecond(rule string) error {
	object, report, err := m.w.lastObject()
	if err != nil {
		return err
	}
	inits, _, err := m.initSites()
	if err != nil {
		return err
	}
	problems, _ := object["problems"].([]any)
	if len(problems) != 1 {
		return fmt.Errorf("the run reported %d problems, not one\n%s", len(problems), report)
	}
	p, _ := problems[0].(map[string]any)
	file, _ := p["file"].(string)
	if p["rule"] != rule || filepath.ToSlash(file) != setupSource || p["function"] != initID || p["line"] != float64(inits[1].first) {
		return fmt.Errorf("the problem is %v, not %s of %s at line %d\n%s", p, rule, initID, inits[1].first, report)
	}
	return nil
}

// runsOnlySecond is whether a run of args runs the second init function's
// mutants, every one, and no other.
func (m *mutateState) runsOnlySecond(args string) error {
	inits, each, err := m.initSites()
	if err != nil {
		return err
	}
	if err := m.w.runWith(m.w.env(), args); err != nil {
		return err
	}
	ran := trials(m.w.stderr, setupSource)
	s, _ := summaryOf(m.w.stdout, setupSource)
	if len(ran) == 0 || len(ran) != len(each[1]) || s.ran != len(ran) || slices.ContainsFunc(ran, func(l int) bool { return !inits[1].holds(l) }) {
		return fmt.Errorf("the run ran mutants on lines %v, not the %d of the second init function, lines %d to %d\n%s",
			ran, len(each[1]), inits[1].first, inits[1].last, m.w.report())
	}
	return nil
}

// reusesEvery is whether a run of args reuses every mutant of src/setup.go
// and runs none.
func (m *mutateState) reusesEvery(args string) error {
	_, each, err := m.initSites()
	if err != nil {
		return err
	}
	if err := m.w.runWith(m.w.env(), args); err != nil {
		return err
	}
	all := len(each[0]) + len(each[1])
	if s, ok := summaryOf(m.w.stdout, setupSource); !ok || s.ran != 0 || s.reused != all || len(trials(m.w.stderr, setupSource)) != 0 {
		return fmt.Errorf("the run did not reuse the %d mutants of src/setup.go alone\n%s", all, m.w.report())
	}
	return nil
}

// Sources itos-cc.yaml excludes.

// The Go module of "Rule: Sources itos-cc.yaml excludes are not mutated":
// src/board.go's place, which TestPlace kills, and clear, which only
// TestClear, where a scenario has it, reaches, beside a fake in a package
// of its own, which no test reaches. Line 7 of the fake holds three sites,
// the first the 0 at column 9.
const (
	boardGo = `package board

type Board struct{}

func (b *Board) place(i int) bool {
	// past five
	return i > 5
}

func (b *Board) clear(i int) bool {
	return i < 3
}
`
	placeTest = `package board

import "testing"

func TestPlace(t *testing.T) {
	var b Board
	if b.place(5) || !b.place(6) {
		t.Fatal("place")
	}
}
`
	clearTest = `
func TestClear(t *testing.T) {
	var b Board
	if !b.clear(2) || b.clear(3) {
		t.Fatal("clear")
	}
}
`
	fakeGo = `package porttest

// Fake stands in for a board in tests.
type Fake struct{ n int }

func (f *Fake) Over(i int) bool {
	return 0 < i-f.n
}
`
	// fakeException excepts the fake's survivor at line 7, column 9, as
	// a person or mutation except writes one.
	fakeException = `mutation:
  exceptions:
    - file: %s
      function: example.com/m/src/port/porttest.Fake#Over
      hash: "0000000000000000"
      line_in_function: 2
      column: 9
      original: "0"
      replacement: "1"
      reason: the fake's bound is never read
`
)

// boardModule writes the module with the fake at fake, its tests killing
// clear's mutant too where both, and declares what they do and cover.
func (m *mutateState) boardModule(fake string, both bool) error {
	tests, report := placeTest, "SF:src/board.go\nDA:7,1\nDA:11,0\nend_of_record\n"
	if both {
		tests, report = placeTest+clearTest, "SF:src/board.go\nDA:7,1\nDA:11,1\nend_of_record\n"
	}
	if !strings.HasSuffix(fake, "/porttest/fake.go") {
		return fmt.Errorf("the fake is %s, not a porttest package's fake.go", fake)
	}
	if err := m.writeAll(map[string]string{
		"go.mod": mutateModule, "src/board.go": boardGo, "src/board_test.go": tests, fake: fakeGo,
	}); err != nil {
		return err
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "go", Report: report}); err != nil {
		return err
	}
	return m.w.d.Tests(m.w.dir, goTests)
}

// onlyCommitChanges writes the module beside the itos-cc.yaml a Given
// wrote, with the results of a run on main, the fake's mutants reached by
// no test, then commits a change to the fake alone on a branch of main.
func (m *mutateState) onlyCommitChanges(fake string) error {
	if m.read("itos-cc.yaml") == "" {
		return errors.New("no itos-cc.yaml was written before the module")
	}
	if err := m.boardModule(fake, false); err != nil {
		return err
	}
	if strings.Contains(m.read("src/board_test.go"), "porttest") {
		return errors.New("a test reaches the fake")
	}
	if err := m.w.git("init", "-q", "-b", "main"); err != nil {
		return err
	}
	if code, stdout, stderr, err := m.w.capture(m.w.dir, m.w.env(), "mutation", "run", "--no-annotate"); err != nil || code != 0 {
		return fmt.Errorf("the run on main failed: exit %d, %v\n%s%s", code, err, stdout, stderr)
	}
	for _, args := range [][]string{
		{"add", "-A"}, {"commit", "-qm", "base"}, {"checkout", "-qb", "work"},
	} {
		if err := m.w.git(args...); err != nil {
			return err
		}
	}
	if err := m.w.write(fake, strings.Replace(fakeGo, "0 < i-f.n", "1 < i-f.n", 1)); err != nil {
		return err
	}
	if err := m.w.git("add", "-A"); err != nil {
		return err
	}
	return m.w.git("commit", "-qm", "change the fake")
}

// siteMention is a site a command names: its file and line, and its column
// where it gives one, before the text a mutant replaces.
var siteMention = regexp.MustCompile("(\\S+):\\d+(?::\\d+)? `")

// neitherNames is whether each command names a site, and none in file,
// whose name neither says at all.
func (m *mutateState) neitherNames(first, second, file string) error {
	for _, args := range []string{first, second} {
		words, err := commandWords(args)
		if err != nil {
			return err
		}
		code, stdout, stderr, err := m.w.capture(m.w.dir, m.w.env(), words...)
		if err != nil {
			return err
		}
		output := filepath.ToSlash(stdout + stderr)
		mentions := siteMention.FindAllStringSubmatch(output, -1)
		if len(mentions) == 0 || strings.Contains(output, path.Base(file)) {
			return fmt.Errorf("itos-cc %s names no site, or names %s: exit %d\n%s", args, path.Base(file), code, output)
		}
	}
	return nil
}

// excludedSnapshot writes the module in a git repository and records a
// run's results of every source, the fake's at fake among them, all fresh.
func (m *mutateState) excludedSnapshot(fake string) error {
	if err := m.boardModule(fake, true); err != nil {
		return err
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-qm", "base"}} {
		if err := m.w.git(args...); err != nil {
			return err
		}
	}
	for _, args := range [][]string{{"mutation", "run"}, {"mutation", "check"}} {
		if code, stdout, stderr, err := m.w.capture(m.w.dir, m.w.env(), args...); err != nil || code != 0 {
			return fmt.Errorf("itos-cc %s: exit %d, %v\n%s%s", strings.Join(args, " "), code, err, stdout, stderr)
		}
	}
	m.source, m.snapshot = fake, ".metrics/mutate/"+fake+".json"
	if !m.w.d.Exists(filepath.Join(m.w.dir, filepath.FromSlash(m.snapshot))) {
		return fmt.Errorf("the run recorded no %s", m.snapshot)
	}
	return nil
}

// lists is whether the last run's --json object's key lists an entry of
// file with name value.
func (m *mutateState) lists(key, file, name, value string) error {
	object, err := m.w.json()
	if err != nil {
		return err
	}
	entries, _ := object[key].([]any)
	for _, one := range entries {
		entry, _ := one.(map[string]any)
		if got, _ := entry["file"].(string); filepath.ToSlash(got) == file {
			if entry[name] != value {
				return fmt.Errorf("%q lists %s with %q %v, not %q\n%s", key, file, name, entry[name], value, m.w.report())
			}
			return nil
		}
	}
	return fmt.Errorf("%q does not list %s\n%s", key, file, m.w.report())
}

// snapshotRemoved is whether the snapshot the Given recorded is gone, and
// the last run's key lists its source with name value.
func (m *mutateState) snapshotRemoved(key, name, value string) error {
	if m.snapshot == "" {
		return errors.New("no Given recorded a snapshot")
	}
	if m.w.d.Exists(filepath.Join(m.w.dir, filepath.FromSlash(m.snapshot))) {
		return fmt.Errorf("%s is still there\n%s", m.snapshot, m.w.report())
	}
	return m.lists(key, m.source, name, value)
}

// exceptsAndExcludes writes the module with the fake at fake, and an
// itos-cc.yaml excluding glob and excepting a survivor of the fake.
func (m *mutateState) exceptsAndExcludes(fake, glob string) error {
	if err := m.boardModule(fake, true); err != nil {
		return err
	}
	return m.w.write("itos-cc.yaml", "exclude: ["+strconv.Quote(glob)+"]\n"+fmt.Sprintf(fakeException, fake))
}

// reports is whether the last run's --json object reports a problem of
// rule with name value.
func (m *mutateState) reports(rule, name, value string) error {
	object, err := m.w.json()
	if err != nil {
		return err
	}
	problems, _ := object["problems"].([]any)
	for _, one := range problems {
		if p, _ := one.(map[string]any); p["rule"] == rule && p[name] == value {
			return nil
		}
	}
	return fmt.Errorf("no problem is %s with %q %q\n%s", rule, name, value, m.w.report())
}

// headExcludes commits the module with an itos-cc.yaml excluding glob,
// then leaves one excluding nothing in the working tree, where mutation
// list lists the fake's sites: only the file HEAD commits leaves them out.
func (m *mutateState) headExcludes(glob string) error {
	if err := m.w.d.Counted(); err != nil {
		return err
	}
	fake := "src/port/porttest/fake.go"
	if err := m.boardModule(fake, false); err != nil {
		return err
	}
	if err := m.w.write("itos-cc.yaml", "exclude: ["+strconv.Quote(glob)+"]\n"); err != nil {
		return err
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-qm", "base"}} {
		if err := m.w.git(args...); err != nil {
			return err
		}
	}
	if err := m.w.write("itos-cc.yaml", "exclude: []\n"); err != nil {
		return err
	}
	sites, err := m.sitesOf(".")
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(sites, func(s listedSite) bool { return s.file == fake }) {
		return fmt.Errorf("mutation list lists no site of %s: %v", fake, sites)
	}
	return nil
}

// noSelectedSiteIn is whether the last run's --json object selects sites,
// none in file.
func (m *mutateState) noSelectedSiteIn(file string) error {
	object, err := m.w.json()
	if err != nil {
		return err
	}
	selected, _ := object["selected"].([]any)
	if len(selected) == 0 {
		return fmt.Errorf("the run selected no site\n%s", m.w.report())
	}
	for _, one := range selected {
		site, _ := one.(map[string]any)
		if got, _ := site["file"].(string); filepath.ToSlash(got) == file {
			return fmt.Errorf("the run selected %v, in %s\n%s", site, file, m.w.report())
		}
	}
	return nil
}
