package features

import (
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

// The mutation sites a source has, and the order they are listed and printed
// in.

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
