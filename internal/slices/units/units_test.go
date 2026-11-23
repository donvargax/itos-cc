// The tests are in the slice's own package. The scenarios of
// features/units.feature hold the command through its binary; these hold
// what Handle and Run do with what the port gives and fails with, through
// porttest's fake, which no scenario can make a file fail to read in.
package units

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/cli"
	langport "github.com/donvargax/itos-cc/internal/lang/port"
	langporttest "github.com/donvargax/itos-cc/internal/lang/port/porttest"
	"github.com/donvargax/itos-cc/internal/past"
	pasttest "github.com/donvargax/itos-cc/internal/past/porttest"
	"github.com/donvargax/itos-cc/internal/selection"
	"github.com/donvargax/itos-cc/internal/selection/port"
	"github.com/donvargax/itos-cc/internal/selection/port/porttest"
)

// work is the working directory, as the system writes it.
var work = filepath.FromSlash("/work")

// tree is a working tree at work of a TypeScript source and its test, and
// a Python source, held in memory.
func tree() porttest.Tree {
	return porttest.TreeOf(work, map[string]string{
		"src/cart.ts":      "export function total(p: number[]): number {\n  return p.length;\n}\n",
		"src/price.py":     "def discount(p):\n    return p\n",
		"src/cart.test.ts": "function cartOf(n: number) {\n  return n;\n}\n",
	}, nil)
}

// around is what surrounds the tree's files: a working directory holding no
// project marker, so each file is named by its path from there.
var around = langporttest.Surroundings{Dir: "/work"}

// names are the namespace#name of each unit.
func names(t *testing.T, h Handler, q Query) ([]string, error) {
	t.Helper()
	found, err := h.Handle(t.Context(), q)
	var all []string
	for _, u := range found.Units {
		all = append(all, u.Namespace+"#"+u.Name)
	}
	return all, err
}

func TestHandleListsTheUnitsOfWhatItSelects(t *testing.T) {
	for _, c := range []struct {
		q    Query
		want []string
	}{
		{Query{}, []string{"cart#total", "price#discount"}},
		{Query{Tests: true}, []string{"cart.test#cartOf"}},
		{Query{Paths: []string{"price"}}, []string{"price#discount"}},
	} {
		got, err := names(t, Handler{Tree: tree(), Surroundings: around}, c.q)
		if err != nil || !slices.Equal(got, c.want) {
			t.Errorf("%+v: %q, %v; want %q", c.q, got, err, c.want)
		}
	}
}

// A selection that fails lists nothing; a file that cannot be read stops
// the listing with the units before it, and the port's failure.
func TestHandleStopsAtAFailureWithWhatItListed(t *testing.T) {
	_, err := names(t, Handler{Tree: tree(), Surroundings: around}, Query{Paths: []string{"nosuch"}})
	var unmatched *port.Unmatched
	if !errors.As(err, &unmatched) {
		t.Errorf("an unmatched fragment: %T %v, want Unmatched", err, err)
	}
	locked := tree()
	delete(locked.Contents, "src/price.py")
	got, err := names(t, Handler{Tree: locked, Surroundings: around}, Query{})
	var unreadable *port.Unreadable
	if !errors.As(err, &unreadable) || !slices.Equal(got, []string{"cart#total"}) {
		t.Errorf("an unreadable file: %q, %T %v; want cart#total and Unreadable", got, err, err)
	}
}

// lang names each unit through the Surroundings it is handed, not the
// disk: a go.mod they hold names a Go source by its module.
func TestHandleNamesUnitsThroughTheSurroundingsItIsGiven(t *testing.T) {
	tree := porttest.TreeOf(work, map[string]string{"billing/sum.go": "package billing\n\nfunc Sum() int { return 0 }\n"}, nil)
	module := langporttest.Surroundings{Dir: "/work", Files: map[string]string{"/work/go.mod": "module example.com/shop\n"}}
	got, err := names(t, Handler{Tree: tree, Surroundings: module}, Query{})
	if err != nil || !slices.Equal(got, []string{"example.com/shop/billing#Sum"}) {
		t.Errorf("units %q, %v; want example.com/shop/billing#Sum", got, err)
	}
}

// ports are the Ports cmd's adapters give the handler, held in memory: the
// tree, the surroundings, and the exclusion the settings give or the
// failure reading them meets.
type ports struct {
	tree    porttest.Tree
	exclude selection.Exclusion
	failure error
}

func (p ports) PastCommits() past.Source                { return nil }
func (p ports) BuildVersion() string                    { return "test" }
func (p ports) SelectionTree() port.Tree                { return p.tree }
func (p ports) LangSurroundings() langport.Surroundings { return around }
func (p ports) SelectionExclusion(context.Context) (selection.Exclusion, error) {
	return p.exclude, p.failure
}

// NewHandler builds the handler over its Ports, the sources their settings
// exclude left out, and returns a failure reading them as the Ports gave it,
// never wrapped, so cmd's config.invalid is the problem the person reads.
func TestNewHandlerLeavesOutWhatThePortsExclude(t *testing.T) {
	h, err := NewHandler(t.Context(), ports{tree: tree(), exclude: selection.Exclusion{Root: work, Exclude: []string{"**/*.py"}}})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := names(t, h, Query{}); err != nil || !slices.Equal(got, []string{"cart#total"}) {
		t.Errorf("units %q, %v; want cart#total alone", got, err)
	}
	failure := cli.NewProblem(cli.CodeUsage, "config.invalid", "itos-cc.yaml: cannot be read", "Fix it.")
	if _, err := NewHandler(t.Context(), ports{tree: tree(), failure: failure}); err != failure {
		t.Errorf("a settings failure: %v, want the Ports' own %v", err, failure)
	}
}

// ends are a run's standard output and error.
type ends struct{ stdout, stderr strings.Builder }

func run(t *testing.T, e *ends, c CLI, h Handler) int {
	t.Helper()
	ui := &cli.UI{Stdout: &e.stdout, Stderr: &e.stderr, Dir: work}
	return c.Run(t.Context(), ui, h)
}

func TestRunPrintsALineAUnit(t *testing.T) {
	var e ends
	if code := run(t, &e, CLI{}, Handler{Tree: tree(), Surroundings: around}); code != 0 {
		t.Fatalf("exit %d\n%s", code, e.stderr.String())
	}
	want := "src/cart.ts:1-3  cart#total  function\nsrc/price.py:1-2  price#discount  function\n"
	if e.stdout.String() != want || e.stderr.Len() != 0 {
		t.Errorf("stdout %q, stderr %q; want %q", e.stdout.String(), e.stderr.String(), want)
	}
}

// A plain line names its file slash-separated on every system (bug-9), and
// --json in the system's form. The UI is given Windows' separator as a
// value, and the tree a file Windows names src\cart.ts, so Linux holds the
// lines to it too.
func TestRunsPlainLinesNameAWindowsFileWithSlashes(t *testing.T) {
	windows := porttest.TreeOf(work, map[string]string{windowsName("src/cart.ts"): "export function total(p: number[]): number {\n  return p.length;\n}\n"}, nil)
	for _, c := range []struct {
		json bool
		want string
	}{
		{false, "src/cart.ts:1-3"},
		{true, `"file": "src\\cart.ts"`},
	} {
		var stdout, stderr strings.Builder
		ui := &cli.UI{Stdout: &stdout, Stderr: &stderr, Dir: work, Separator: '\\'}
		if code := (CLI{JSON: c.json}).Run(t.Context(), ui, Handler{Tree: windows, Surroundings: around}); code != 0 || !strings.Contains(stdout.String(), c.want) {
			t.Errorf("--json %v: exit %d, stdout %q; want it to hold %s\n%s", c.json, code, stdout.String(), c.want, stderr.String())
		}
	}
}

// With nothing to list, the object's units are an empty list, never null.
func TestRunListsNoUnitAsAnEmptyList(t *testing.T) {
	var e ends
	if code := run(t, &e, CLI{JSON: true}, Handler{Tree: porttest.Tree{}, Surroundings: around}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if want := "{\n  \"ok\": true,\n  \"schema\": 1,\n  \"units\": []\n}\n"; e.stdout.String() != want {
		t.Errorf("stdout %q, want %q", e.stdout.String(), want)
	}
}

// A failure is reported through the UI, the object keeping the units
// listed before it, and plain output printing none of them.
func TestRunReportsAFailureWithWhatItListed(t *testing.T) {
	locked := tree()
	delete(locked.Contents, "src/price.py")
	var e ends
	if code := run(t, &e, CLI{JSON: true}, Handler{Tree: locked, Surroundings: around}); code != cli.CodeInternal {
		t.Fatalf("exit %d, want %d", code, cli.CodeInternal)
	}
	if out := e.stdout.String(); !strings.Contains(out, `"name": "total"`) || !strings.Contains(out, `"rule": "internal"`) {
		t.Errorf("stdout %q, want total and the internal problem", out)
	}
	var plain ends
	if code := run(t, &plain, CLI{}, Handler{Tree: locked, Surroundings: around}); code != cli.CodeInternal || plain.stdout.Len() != 0 {
		t.Errorf("plain: exit %d, stdout %q; want 100 and nothing", code, plain.stdout.String())
	}
}

// The help gives the contract a script reads: the units' keys and every
// exit code.
func TestTheContractGivesTheUnitsKeysAndExitCodes(t *testing.T) {
	c := CLI{}.Contract()
	var codes []int
	for _, e := range c.Exits {
		codes = append(codes, e.Code)
	}
	if !strings.Contains(c.JSON, `"start_line"`) || !slices.Equal(codes, []int{0, 80, 82}) {
		t.Errorf("JSON %q, exit codes %v", c.JSON, codes)
	}
}

// windowsName is rel as Windows names it, src\cart.ts. The fake tree keys its
// files by slash, so on a system whose separator is a backslash the name
// is the slash one already.
func windowsName(rel string) string {
	if filepath.Separator == '\\' {
		return rel
	}
	return strings.ReplaceAll(rel, "/", `\`)
}

// pastSource is a source of one commit, "old", whose tree is the one given
// and whose itos-cc.yaml excludes exclude, with its results kept in memory.
func pastSource(tree porttest.Tree, exclude selection.Exclusion) *pasttest.Source {
	return &pasttest.Source{
		Results: &pasttest.Results{},
		Commits: map[string]past.Commit{"old": {ID: "abc123", Root: work, Dir: work, Tree: tree, Surroundings: around, Settings: port.Settings(exclude)}},
	}
}

// With At the units are the commit's, read through the ports the commit
// gives, never the handler's own tree, and the sources the commit's own
// settings exclude are left out.
func TestHandleAtListsTheUnitsOfTheCommit(t *testing.T) {
	old := porttest.TreeOf(work, map[string]string{
		"src/old.ts":  "export function before(): number {\n  return 1;\n}\n",
		"src/skip.ts": "export function skipped(): number {\n  return 2;\n}\n",
	}, nil)
	source := pastSource(old, selection.Exclusion{Root: work, Exclude: []string{"**/skip.ts"}})
	h := Handler{Tree: tree(), Surroundings: around, Past: source, Version: "test"}
	found, err := h.Handle(t.Context(), Query{At: "old"})
	if err != nil || len(found.Units) != 1 || found.Units[0].Name != "before" || found.Excluded != 1 {
		t.Fatalf("at old: %+v, %v; want before alone and 1 left out", found, err)
	}
	if got := found.Units[0].File; got != filepath.Join(work, "src/old.ts") {
		t.Errorf("the unit's file is %q, want it absolute under the working directory", got)
	}
	var bad *past.BadRev
	if _, err := h.Handle(t.Context(), Query{At: "nosuch"}); !errors.As(err, &bad) || bad.Rev != "nosuch" {
		t.Errorf("an unknown revision: %v, want BadRev", err)
	}
}

// The first run keeps its answer and the next gives it back without opening
// the tree: a handler over a commit whose tree is empty, asked the same
// question, lists what the first listed; a different question is measured;
// a kept result that is corrupt is measured again and kept whole.
func TestHandleAtKeepsItsAnswerPerQuestion(t *testing.T) {
	old := porttest.TreeOf(work, map[string]string{"src/old.ts": "export function before(): number {\n  return 1;\n}\n"}, nil)
	source := pastSource(old, selection.Exclusion{})
	h := Handler{Tree: tree(), Surroundings: around, Past: source, Version: "test"}
	if got, err := names(t, h, Query{At: "old"}); err != nil || !slices.Equal(got, []string{"old#before"}) {
		t.Fatalf("first run: %q, %v", got, err)
	}
	if source.Results.Puts != 1 {
		t.Fatalf("the first run kept %d results, want 1", source.Results.Puts)
	}
	empty := pastSource(porttest.TreeOf(work, map[string]string{}, nil), selection.Exclusion{})
	empty.Results = source.Results
	h.Past = empty
	if got, err := names(t, h, Query{At: "old"}); err != nil || !slices.Equal(got, []string{"old#before"}) {
		t.Errorf("a kept answer: %q, %v; want the first run's", got, err)
	}
	if got, err := names(t, h, Query{At: "old", Tests: true}); err != nil || len(got) != 0 {
		t.Errorf("another question: %q, %v; want it measured, none", got, err)
	}
	for name := range source.Results.Held {
		source.Results.Held[name] = []byte("{not json")
	}
	h.Past = source
	if got, err := names(t, h, Query{At: "old"}); err != nil || !slices.Equal(got, []string{"old#before"}) {
		t.Errorf("a corrupt kept answer: %q, %v; want it measured again", got, err)
	}
	if _, err := names(t, h, Query{At: "old", Tests: true}); err != nil {
		t.Fatal(err)
	}
	for name, data := range source.Results.Held {
		if !strings.HasPrefix(string(data), "{") || strings.Contains(string(data), "not json") {
			t.Errorf("%s still holds %q", name, data)
		}
	}
}

// --at with --changed is refused, as the problem of its own rule.
func TestRunRefusesAtWithChanged(t *testing.T) {
	var e ends
	code := run(t, &e, CLI{Selection: cli.Selection{Changed: true}, Past: cli.Past{At: "old"}, JSON: true}, Handler{Tree: tree(), Surroundings: around})
	if code != cli.CodeUsage || !strings.Contains(e.stdout.String(), "flags.conflict") {
		t.Errorf("--at --changed: exit %d, %s; want %d and flags.conflict", code, e.stdout.String(), cli.CodeUsage)
	}
}
