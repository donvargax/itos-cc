// The tests are in the slice's own package. The scenarios of
// features/units.feature hold the command through its binary; these hold
// what Handle and Run do with what the port gives and fails with, through
// porttest's fake, which no scenario can make a file fail to read in.
package units

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/cli"
	langporttest "github.com/donvargax/itos-cc/internal/lang/port/porttest"
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
	// Each path is from the working directory, as the system writes it.
	want := filepath.FromSlash("src/cart.ts") + ":1-3  cart#total  function\n" +
		filepath.FromSlash("src/price.py") + ":1-2  price#discount  function\n"
	if e.stdout.String() != want || e.stderr.Len() != 0 {
		t.Errorf("stdout %q, stderr %q; want %q", e.stdout.String(), e.stderr.String(), want)
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
