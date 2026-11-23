// The tests are in the slice's own package: a slice imports no other slice,
// and depguard cannot tell a slice's own import from another's. The
// scenarios of features/scrap.feature hold the command through its binary;
// these hold what Handle and Run do with what the ports give and fail
// with, through porttest's fakes, which no scenario can make a file fail to
// read or a snapshot fail to write in.
package scrap

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/donvargax/itos-cc/internal/selection/port"
	"github.com/donvargax/itos-cc/internal/selection/port/porttest"
	"github.com/donvargax/itos-cc/internal/slices/scrap/domain"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
	snapporttest "github.com/donvargax/itos-cc/internal/snapshot/port/porttest"
)

const cartTest = `describe("cart", () => {
  it("adds", () => { expect(add(1)).toBe(1); });
  it("removes", () => { expect(remove(1)).toBe(0); });
});
`

// at is a path of the project at /work, as the system writes it.
func at(rel string) string { return filepath.FromSlash("/work/" + rel) }

// tree is a project at /work: a source and its test, held in memory.
func tree() porttest.Tree {
	return porttest.TreeOf(at(""), map[string]string{"src/cart.ts": "", "src/cart.test.ts": cartTest}, nil)
}

// handler is a handler over t, its snapshots kept in store.
func handler(t porttest.Tree, store *snapporttest.Store) Handler {
	return Handler{Tree: t, Surroundings: langporttest.Surroundings{Dir: "/work/src"}, Store: store}
}

// store is a store at /work holding scrap.json as given, none when "".
func store(scrapJSON string) *snapporttest.Store {
	s := &snapporttest.Store{RootDir: at("")}
	if scrapJSON != "" {
		s.Snapshots = map[string]string{"scrap.json": scrapJSON}
	}
	return s
}

// recordedFiles are the files scrap.json records in s.
func recordedFiles(t *testing.T, s *snapporttest.Store) []string {
	t.Helper()
	var r recorded
	if err := json.Unmarshal([]byte(s.Snapshots["scrap.json"]), &r); err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, f := range r.Files {
		files = append(files, f.File)
	}
	return files
}

// Handle names each report by its path as selected, compares it with the
// last snapshot's report of its file, and records it from the project root
// beside the files this run did not measure.
func TestHandleMeasuresComparesAndRecords(t *testing.T) {
	s := store(`{"version": 1, "files": [{"file": "src/cart.test.ts", "pressure": 99}, {"file": "lib/old.test.ts", "pressure": 1}]}`)
	found, err := handler(tree(), s).Handle(t.Context(), Query{})
	if err != nil || found.NoTests || found.Ignored != nil || len(found.Reports) != 1 {
		t.Fatalf("Handle() = %+v, %v; want one report", found, err)
	}
	if r := found.Reports[0]; r.File != at("src/cart.test.ts") || r.Examples != 2 || r.Compare == nil || r.Compare.Verdict != "improved" {
		t.Errorf("report %+v, compare %+v; want cart.test.ts's two examples, improved", r, r.Compare)
	}
	if got := recordedFiles(t, s); !slices.Equal(got, []string{"lib/old.test.ts", "src/cart.test.ts"}) {
		t.Errorf("scrap.json records %q, want the file kept and the one measured, from the root", got)
	}
}

// A selection with no test file measures and records nothing.
func TestHandleWithNoTestFileDoesNothing(t *testing.T) {
	s := store("")
	found, err := handler(tree(), s).Handle(t.Context(), Query{Paths: []string{at("src/cart.ts")}})
	if err != nil || !found.NoTests || s.Snapshots != nil {
		t.Errorf("Handle(src/cart.ts) = %+v, %v, recorded %v; want no tests and nothing recorded", found, err, s.Snapshots)
	}
}

// A last snapshot that cannot be read is ignored, and the run goes on and
// records what it measured; any other failure to read it stops the run.
func TestAnUnreadableSnapshotIsIgnored(t *testing.T) {
	s := store("{")
	found, err := handler(tree(), s).Handle(t.Context(), Query{})
	if err != nil || found.Ignored == nil || found.Ignored.Reason != "it does not decode" || len(found.Reports) != 1 {
		t.Fatalf("Handle() = %+v, %v; want the report and the snapshot ignored", found, err)
	}
	if got := recordedFiles(t, s); !slices.Equal(got, []string{"src/cart.test.ts"}) {
		t.Errorf("scrap.json records %q, want the file measured", got)
	}
	stopped := &snapporttest.Store{Failures: map[string]snapport.Failure{"scrap.json": &snapport.Cancelled{}}}
	var cancelled *snapport.Cancelled
	if found, err := handler(tree(), stopped).Handle(t.Context(), Query{}); !errors.As(err, &cancelled) || found.Ignored != nil {
		t.Errorf("a read that was cancelled: %+v, %T %v; want Cancelled", found, err, err)
	}
}

// A test file that cannot be read, a language lang does not know, a done
// context, a root the store cannot give and a snapshot that cannot be
// written stop Handle with no report, the ignored snapshot still said.
func TestHandleStopsAtAFailure(t *testing.T) {
	locked := tree()
	delete(locked.Contents, "src/cart.test.ts")
	found, err := handler(locked, store("{")).Handle(t.Context(), Query{})
	var unreadable *port.Unreadable
	if !errors.As(err, &unreadable) || found.Ignored == nil || found.Reports != nil {
		t.Errorf("an unreadable test: %+v, %T %v; want Unreadable, the snapshot ignored", found, err, err)
	}
	strange := tree()
	strange.Tests = append(strange.Tests, "src/notes_test.txt")
	if _, err := handler(strange, store("")).Handle(t.Context(), Query{}); err == nil || err.Error() != "unsupported language: "+at("src/notes_test.txt") {
		t.Errorf("a language lang does not know: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var stopped *port.Cancelled
	if _, err := handler(tree(), &snapporttest.Store{}).Handle(ctx, Query{}); !errors.As(err, &stopped) {
		t.Errorf("a done context: %T %v, want the selection Cancelled", err, err)
	}
	var cancelled *snapport.Cancelled
	found, err = handler(tree(), &snapporttest.Store{RootFailure: &snapport.Cancelled{}}).Handle(t.Context(), Query{})
	if !errors.As(err, &cancelled) || found.Reports != nil {
		t.Errorf("a root the store cannot give: %+v, %T %v; want Cancelled and no report", found, err, err)
	}
	full := &snapport.Unwritable{Name: "scrap.json", Reason: "no space left on device"}
	found, err = handler(tree(), &snapporttest.Store{Failures: map[string]snapport.Failure{"scrap.json": full}}).Handle(t.Context(), Query{})
	if !errors.Is(err, full) || found.Reports != nil {
		t.Errorf("an unwritable snapshot: %+v, %v; want the failure and no report", found, err)
	}
}

// ports are the Ports cmd's adapters give the handler, held in memory: the
// tree and the store.
type ports struct {
	tree  porttest.Tree
	store *snapporttest.Store
}

func (p ports) SelectionTree() port.Tree { return p.tree }
func (p ports) LangSurroundings() langport.Surroundings {
	return langporttest.Surroundings{Dir: "/work/src"}
}
func (p ports) SnapshotStore() snapport.Store { return p.store }
func (p ports) PastCommits() past.Source      { return nil }
func (p ports) BuildVersion() string          { return "test" }

// NewHandler builds the handler over its Ports: the test files their tree
// holds, recorded in their store. It reads nothing, so it never fails.
func TestNewHandlerIsBuiltOverItsPorts(t *testing.T) {
	s := store("")
	h, err := NewHandler(t.Context(), ports{tree: tree(), store: s})
	if err != nil {
		t.Fatal(err)
	}
	found, err := h.Handle(t.Context(), Query{})
	if err != nil || len(found.Reports) != 1 || found.Reports[0].File != at("src/cart.test.ts") {
		t.Errorf("Handle() = %+v, %v; want cart.test.ts's report, by its path", found, err)
	}
	if got := recordedFiles(t, s); !slices.Equal(got, []string{"src/cart.test.ts"}) {
		t.Errorf("scrap.json records %q, want the file measured, in the Ports' store", got)
	}
}

// run is c run over h from /work/src, below the project root /work, so
// the names it prints differ from scrap.json's keys, and what it printed on
// stdout and stderr.
func run(t *testing.T, c CLI, h Handler) (code int, stdout, stderr string) {
	t.Helper()
	var out, errs bytes.Buffer
	code = c.Run(t.Context(), &cli.UI{Stdout: &out, Stderr: &errs, Dir: at("src")}, h)
	return code, out.String(), errs.String()
}

func TestRunPrintsEachReport(t *testing.T) {
	code, stdout, stderr := run(t, CLI{}, handler(tree(), store("")))
	if code != 0 || stderr != "" || !strings.HasPrefix(stdout, "cart.test.ts  "+domain.LeaveAlone+"  pressure=") ||
		!strings.Contains(stdout, "  examples=2  ") || strings.Count(stdout, "\n") != 1 {
		t.Errorf("exit %d, stdout\n%s\nstderr %q", code, stdout, stderr)
	}
	code, stdout, _ = run(t, CLI{Verbose: true}, handler(tree(), store("")))
	if code != 0 || !strings.Contains(stdout, `cart.test.ts:2-2 "adds" lines=1 assertions=1 decisions=0 mocks=0 []`) {
		t.Errorf("--verbose: exit %d, stdout\n%s", code, stdout)
	}
}

// A file the last change made worse says so under its summary.
func TestRunWarnsOfAFileMadeWorse(t *testing.T) {
	_, stdout, _ := run(t, CLI{}, handler(tree(), store(`{"version": 1, "files": [{"file": "src/cart.test.ts", "pressure": -9}]}`)))
	if !strings.Contains(stdout, "  worse from -9.0\n  the last change made this file worse: check new helpers and duplication before keeping it\n") {
		t.Errorf("stdout\n%s", stdout)
	}
}

// The lines for people on stderr are said with --json too.
func TestRunSaysOnStderrWhatItSkipped(t *testing.T) {
	code, stdout, stderr := run(t, CLI{JSON: true}, handler(tree(), store("{")))
	if code != 0 || stderr != "itos-cc: ignoring unreadable .metrics/scrap.json: it does not decode\n" || !strings.Contains(stdout, `"file": "cart.test.ts"`) {
		t.Errorf("exit %d, stdout\n%s\nstderr %q", code, stdout, stderr)
	}
	code, stdout, stderr = run(t, CLI{Selection: cli.Selection{Paths: []string{at("src/cart.ts")}}, JSON: true}, handler(tree(), store("")))
	if code != 0 || stderr != "itos-cc: no test files to measure\n" || !strings.Contains(stdout, `"files": []`) {
		t.Errorf("no tests: exit %d, stdout\n%s\nstderr %q", code, stdout, stderr)
	}
}

// A failure is reported with no report in the object.
func TestRunReportsAFailure(t *testing.T) {
	full := &snapport.Unwritable{Name: "scrap.json", Reason: "no space left on device"}
	code, stdout, _ := run(t, CLI{JSON: true}, handler(tree(), &snapporttest.Store{Failures: map[string]snapport.Failure{"scrap.json": full}}))
	if code != cli.CodeInternal || !strings.Contains(stdout, `"files": []`) || !strings.Contains(stdout, `"no space left on device"`) {
		t.Errorf("exit %d, stdout\n%s", code, stdout)
	}
}

// A plain line names its file slash-separated on every system (bug-9), and
// --json in the system's form. The UI is given Windows' separator as a
// value, and the tree a file Windows names src\cart.test.ts, so Linux
// holds the lines to it too.
func TestRunsPlainLinesNameAWindowsFileWithSlashes(t *testing.T) {
	windows := porttest.TreeOf(at(""), map[string]string{windowsName("src/cart.test.ts"): cartTest}, nil)
	for _, c := range []struct {
		command CLI
		want    string
	}{
		{CLI{Verbose: true}, "src/cart.test.ts  "},
		{CLI{Verbose: true}, ` src/cart.test.ts:2-2 "adds" `},
		{CLI{JSON: true}, `"file": "src\\cart.test.ts"`},
	} {
		var stdout, stderr bytes.Buffer
		ui := &cli.UI{Stdout: &stdout, Stderr: &stderr, Dir: at(""), Separator: '\\'}
		h := handler(windows, store(""))
		if code := c.command.Run(t.Context(), ui, h); code != 0 || !strings.Contains(stdout.String(), c.want) {
			t.Errorf("%+v: exit %d, stdout %q; want it to hold %s\n%s", c.command, code, stdout.String(), c.want, stderr.String())
		}
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

// pastOf is a source of one commit, "old", holding tree, its results kept
// in memory.
func pastOf(tree porttest.Tree) *pasttest.Source {
	return &pasttest.Source{
		Results: &pasttest.Results{},
		Commits: map[string]past.Commit{"old": {ID: "abc123", Root: at(""), Dir: at("src"), Tree: tree,
			Surroundings: langporttest.Surroundings{Dir: "/work/src"}, Store: past.Nothing{RootDir: at("")}}},
	}
}

// With At the test files are the commit's, measured through the ports it
// gives, compared with no snapshot, and nothing is recorded in the
// handler's own store; the reports are kept, so the same question asked of a
// commit whose tree is empty gives them back.
func TestHandleAtMeasuresTheCommitAndKeepsTheReports(t *testing.T) {
	s := store(`{"version":1,"files":[]}`)
	source := pastOf(tree())
	h := handler(porttest.TreeOf(at(""), map[string]string{}, nil), s)
	h.Past, h.Version = source, "test"
	found, err := h.Handle(t.Context(), Query{At: "old"})
	if err != nil || len(found.Reports) != 1 || found.Reports[0].File != at("src/cart.test.ts") || found.Reports[0].Compare != nil || s.Snapshots["scrap.json"] != `{"version":1,"files":[]}` {
		t.Fatalf("at old: %+v, %v, recorded %v; want cart.test.ts alone, compared with nothing, scrap.json as it was", found, err, s.Snapshots)
	}
	again := pastOf(porttest.TreeOf(at(""), map[string]string{}, nil))
	again.Results = source.Results
	h.Past = again
	if kept, err := h.Handle(t.Context(), Query{At: "old"}); err != nil || len(kept.Reports) != 1 || kept.Reports[0].File != at("src/cart.test.ts") {
		t.Errorf("a kept answer: %+v, %v; want the first run's report", kept, err)
	}
	for name := range source.Results.Held {
		source.Results.Held[name] = []byte("{not json")
	}
	h.Past = source
	if again, err := h.Handle(t.Context(), Query{At: "old"}); err != nil || len(again.Reports) != 1 {
		t.Errorf("a corrupt kept answer: %+v, %v; want it measured again", again, err)
	}
	var bad *past.BadRev
	if _, err := h.Handle(t.Context(), Query{At: "nosuch"}); !errors.As(err, &bad) {
		t.Errorf("an unknown revision: %v, want BadRev", err)
	}
}

// --at with --changed is refused, as the problem of its own rule.
func TestRunRefusesAtWithChanged(t *testing.T) {
	var out, errs bytes.Buffer
	c := CLI{Selection: cli.Selection{Changed: true}, Past: cli.Past{At: "old"}, JSON: true}
	code := c.Run(t.Context(), &cli.UI{Stdout: &out, Stderr: &errs, Dir: at("src")}, handler(tree(), store("")))
	if code != cli.CodeUsage || !strings.Contains(out.String(), "flags.conflict") {
		t.Errorf("--at --changed: exit %d, %s; want %d and flags.conflict", code, out.String(), cli.CodeUsage)
	}
}
