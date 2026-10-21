// The tests are in the slice's own package: a slice imports no other slice,
// and depguard cannot tell a slice's own import from another's. The
// scenarios of features/dry.feature hold the command through its binary;
// these hold what Handle and Run do with what the ports give and fail
// with, through porttest's fakes, which no scenario can make a file fail to
// read or a snapshot fail to write in.
package dry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/cli"
	langport "github.com/donvargax/itos-cc/internal/lang/port"
	langporttest "github.com/donvargax/itos-cc/internal/lang/port/porttest"
	"github.com/donvargax/itos-cc/internal/selection"
	"github.com/donvargax/itos-cc/internal/selection/port"
	"github.com/donvargax/itos-cc/internal/selection/port/porttest"
	"github.com/donvargax/itos-cc/internal/slices/dry/domain"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
	snapporttest "github.com/donvargax/itos-cc/internal/snapshot/port/porttest"
)

const invoice = `def render(invoice):
    lines = []
    for item in invoice.items:
        lines.append(format_line(item.name, item.price))
    total = sum(item.price for item in invoice.items)
    lines.append(format_total(total))
    return "\n".join(lines)
`

const receipt = `def render(receipt):
    out = []
    for entry in receipt.entries:
        out.append(format_line(entry.label, entry.amount))
    paid = sum(entry.amount for entry in receipt.entries)
    out.append(format_total(paid))
    out.append(format_footer(receipt.store))
    return "\n".join(out)
`

const unrelated = `def parse(text):
    if not text:
        raise ValueError("empty")
    head, _, rest = text.partition(":")
    return {"head": head.strip(), "rest": rest.strip()}
`

// at is a path of the project at /work, as the system writes it.
func at(rel string) string { return filepath.FromSlash("/work/" + rel) }

// tree is a project at /work: two sources that are near copies, one that
// is no copy of either, and a test, held in memory.
func tree() porttest.Tree {
	return porttest.TreeOf(at(""), map[string]string{
		"src/invoice.py": invoice, "src/parse.py": unrelated, "src/receipt.py": receipt, "src/invoice_test.py": "",
	}, nil)
}

// fromSrc names a file as a person in /work/src reads it, so names and
// dry.json's keys, from /work, differ.
func fromSrc(path string) string {
	rel, err := filepath.Rel(at("src"), path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

// handler is a handler over t, its snapshots kept in store.
func handler(t porttest.Tree, store *snapporttest.Store) Handler {
	return Handler{Tree: t, Surroundings: langporttest.Surroundings{Dir: "/work/src"}, Store: store, Name: fromSrc}
}

func query(paths ...string) Query { return Query{Paths: paths, Options: domain.Defaults} }

// The defaults help shows and the command uses are the domain's own.
func TestTheDefaultsAreTheDomains(t *testing.T) {
	cli := reflect.TypeFor[CLI]()
	for field, want := range map[string]any{
		"Threshold": domain.Defaults.Threshold,
		"MinLines":  domain.Defaults.MinLines,
		"MinNodes":  domain.Defaults.MinNodes,
	} {
		f, _ := cli.FieldByName(field)
		if got := f.Tag.Get("default"); got != fmt.Sprint(want) {
			t.Errorf("%s defaults to %q, the domain's is %v", field, got, want)
		}
	}
}

// What Handle finds names each file as Name gives it, and what it records
// names it from the project root.
func TestHandleFindsThePairAndRecordsIt(t *testing.T) {
	store := &snapporttest.Store{RootDir: at("")}
	found, err := handler(tree(), store).Handle(t.Context(), query())
	if err != nil || found.NoSources || len(found.Candidates) != 1 || len(found.Groups) != 1 {
		t.Fatalf("Handle() = %+v, %v; want the one pair", found, err)
	}
	if c := found.Candidates[0]; c.Left.File != "invoice.py" || c.Right.File != "receipt.py" {
		t.Errorf("pair %s / %s, want invoice.py / receipt.py", c.Left.File, c.Right.File)
	}
	var recorded struct {
		Version    int
		Threshold  float64
		Candidates []domain.Candidate
		Groups     []domain.Group
	}
	if err := json.Unmarshal([]byte(store.Snapshots["dry.json"]), &recorded); err != nil {
		t.Fatal(err)
	}
	if recorded.Version != 1 || recorded.Threshold != 0.82 || len(recorded.Candidates) != 1 ||
		recorded.Candidates[0].Left.File != "src/invoice.py" || recorded.Groups[0].Members[1].File != "src/receipt.py" {
		t.Errorf("dry.json holds %+v, want the pair from the root", recorded)
	}
}

// With paths, only pairs with a side among them are found, against every
// source under the working directory; paths that select no source find
// nothing and record nothing.
func TestPathsAreCheckedAgainstTheWholeProject(t *testing.T) {
	for _, c := range []struct {
		paths []string
		pairs int
	}{
		{[]string{at("src/receipt.py")}, 1},
		{[]string{"parse"}, 0},
	} {
		found, err := handler(tree(), &snapporttest.Store{RootDir: at("")}).Handle(t.Context(), query(c.paths...))
		if err != nil || found.NoSources || len(found.Candidates) != c.pairs {
			t.Errorf("%q: %+v, %v; want %d pairs", c.paths, found, err, c.pairs)
		}
	}
	store := &snapporttest.Store{RootDir: at("")}
	found, err := handler(tree(), store).Handle(t.Context(), query("invoice_test"))
	if err != nil || !found.NoSources || store.Snapshots != nil {
		t.Errorf("a test alone: %+v, %v, recorded %v; want no sources and nothing recorded", found, err, store.Snapshots)
	}
}

// A file that cannot be read, a language lang does not know, a done
// context and a root the store cannot give stop Handle with nothing found;
// a snapshot that cannot be written returns what was found beside the
// failure.
func TestHandleStopsAtAFailure(t *testing.T) {
	locked := tree()
	delete(locked.Contents, "src/parse.py")
	_, err := handler(locked, &snapporttest.Store{}).Handle(t.Context(), query())
	var unreadable *port.Unreadable
	if !errors.As(err, &unreadable) {
		t.Errorf("an unreadable file: %T %v, want Unreadable", err, err)
	}
	strange := tree()
	strange.Sources = append(strange.Sources, "src/notes.txt")
	if _, err := handler(strange, &snapporttest.Store{}).Handle(t.Context(), query()); err == nil || err.Error() != "unsupported language: "+at("src/notes.txt") {
		t.Errorf("a language lang does not know: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var stopped *port.Cancelled
	if _, err := handler(tree(), &snapporttest.Store{}).Handle(ctx, query()); !errors.As(err, &stopped) {
		t.Errorf("a done context: %T %v, want the selection Cancelled", err, err)
	}
	var cancelled *snapport.Cancelled
	if _, err := handler(tree(), &snapporttest.Store{RootFailure: &snapport.Cancelled{}}).Handle(t.Context(), query()); !errors.As(err, &cancelled) {
		t.Errorf("a root the store cannot give: %T %v, want Cancelled", err, err)
	}
	full := &snapport.Unwritable{Name: "dry.json", Reason: "no space left on device"}
	found, err := handler(tree(), &snapporttest.Store{Failures: map[string]snapport.Failure{"dry.json": full}}).Handle(t.Context(), query())
	if !errors.Is(err, full) || len(found.Candidates) != 1 {
		t.Errorf("an unwritable snapshot: %+v, %v; want the pair and the failure", found, err)
	}
}

// ports are the Ports cmd's adapters give the handler, held in memory: the
// tree, the store, files named from /work/src, and the exclusion the
// settings give or the failure reading them meets.
type ports struct {
	tree    porttest.Tree
	store   *snapporttest.Store
	exclude selection.Exclusion
	failure error
}

func (p ports) SelectionTree() port.Tree { return p.tree }
func (p ports) LangSurroundings() langport.Surroundings {
	return langporttest.Surroundings{Dir: "/work/src"}
}
func (p ports) SnapshotStore() snapport.Store { return p.store }
func (p ports) Rel(path string) string        { return fromSrc(path) }
func (p ports) SelectionExclusion(context.Context) (selection.Exclusion, error) {
	return p.exclude, p.failure
}

// NewHandler builds the handler over its Ports: the files their tree holds,
// named by their Rel, recorded in their store, the sources their settings
// exclude left out. A failure reading the settings is returned as the Ports
// gave it, never wrapped, so cmd's config.invalid is the problem the person
// reads.
func TestNewHandlerIsBuiltOverItsPorts(t *testing.T) {
	store := &snapporttest.Store{RootDir: at("")}
	h, err := NewHandler(t.Context(), ports{tree: tree(), store: store})
	if err != nil {
		t.Fatal(err)
	}
	found, err := h.Handle(t.Context(), query())
	if err != nil || len(found.Candidates) != 1 || found.Candidates[0].Left.File != "invoice.py" || store.Snapshots["dry.json"] == "" {
		t.Errorf("Handle() = %+v, %v, recorded %v; want invoice.py's pair, named by Rel, in the store", found, err, store.Snapshots)
	}
	h, err = NewHandler(t.Context(), ports{tree: tree(), store: &snapporttest.Store{RootDir: at("")}, exclude: selection.Exclusion{Root: at(""), Exclude: []string{"**/receipt.py"}}})
	if err != nil {
		t.Fatal(err)
	}
	if found, err := h.Handle(t.Context(), query()); err != nil || len(found.Candidates) != 0 || found.Excluded != 1 {
		t.Errorf("Handle() = %+v, %v; want no pair, receipt.py left out", found, err)
	}
	failure := cli.NewProblem(cli.CodeUsage, "config.invalid", "itos-cc.yaml: cannot be read", "Fix it.")
	if _, err := NewHandler(t.Context(), ports{tree: tree(), failure: failure}); err != failure {
		t.Errorf("a settings failure: %v, want the Ports' own %v", err, failure)
	}
}

// run is c run over h, and what it printed on stdout and stderr.
func run(t *testing.T, c CLI, h Handler) (code int, stdout, stderr string) {
	t.Helper()
	var out, errs bytes.Buffer
	code = c.Run(t.Context(), &cli.UI{Stdout: &out, Stderr: &errs}, h)
	return code, out.String(), errs.String()
}

var defaults = CLI{Threshold: 0.82, MinLines: 4, MinNodes: 20}

func TestRunPrintsEachGroup(t *testing.T) {
	code, stdout, stderr := run(t, defaults, handler(tree(), &snapporttest.Store{RootDir: at("")}))
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	if code != 0 || stderr != "" || len(lines) != 3 || !strings.HasPrefix(lines[0], "DUPLICATE score=0.") || !strings.HasSuffix(lines[0], " python") ||
		lines[1] != "  invoice.py:1-7  invoice#render" || lines[2] != "  receipt.py:1-8  receipt#render" {
		t.Errorf("exit %d, stdout\n%s\nstderr %q", code, stdout, stderr)
	}
}

// Paths that select no source say so on stderr and succeed, printing the
// empty object with --json.
func TestRunSaysThereIsNothingToCheck(t *testing.T) {
	c := defaults
	c.Paths, c.JSON = []string{"invoice_test"}, true
	code, stdout, stderr := run(t, c, handler(tree(), &snapporttest.Store{}))
	if code != 0 || stderr != "itos-cc: no source files to check\n" ||
		!strings.Contains(stdout, `"candidates": []`) || !strings.Contains(stdout, `"threshold": 0.82`) {
		t.Errorf("exit %d, stdout\n%s\nstderr %q", code, stdout, stderr)
	}
}

// A failure is reported with what was found before it in the object.
func TestRunReportsAFailureWithWhatItFound(t *testing.T) {
	c := defaults
	c.JSON = true
	full := &snapport.Unwritable{Name: "dry.json", Reason: "no space left on device"}
	code, stdout, _ := run(t, c, handler(tree(), &snapporttest.Store{Failures: map[string]snapport.Failure{"dry.json": full}}))
	if code != cli.CodeInternal || !strings.Contains(stdout, `"no space left on device"`) || !strings.Contains(stdout, `"file": "invoice.py"`) {
		t.Errorf("exit %d, stdout\n%s", code, stdout)
	}
}
