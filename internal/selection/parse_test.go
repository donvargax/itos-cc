package selection_test

import (
	"context"
	"errors"
	"testing"

	langporttest "github.com/donvargax/itos-cc/internal/lang/port/porttest"
	"github.com/donvargax/itos-cc/internal/selection"
	"github.com/donvargax/itos-cc/internal/selection/port"
	"github.com/donvargax/itos-cc/internal/selection/port/porttest"
)

// cart is a tree holding a TypeScript source with its text, a Python
// source with none, which cannot be read, and notes in no language.
func cart() porttest.Tree {
	tree := porttest.TreeOf(dir, map[string]string{
		"src/cart.ts": "export function total(p: number[]): number {\n  return p.length;\n}\n",
		"src/tax.py":  "",
		"notes.txt":   "",
	}, nil)
	delete(tree.Contents, "src/tax.py")
	delete(tree.Contents, "notes.txt")
	return tree
}

// around is what surrounds the tree's files: its directory, holding no
// project marker.
var around = langporttest.Surroundings{Dir: dir}

func TestParseReadsASelectedFileThroughTheTreeAndParsesIt(t *testing.T) {
	path := at("src/cart.ts")[0]
	f, err := selection.Parse(t.Context(), cart(), around, path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if f.Path != path || f.Spec.Name != "typescript" || len(f.Units) != 1 || f.Units[0].Name != "total" {
		t.Errorf("parsed %s as %s with units %+v, want %s, typescript and total", f.Path, f.Spec.Name, f.Units, path)
	}
}

// A language lang does not know is refused before the tree is asked, so
// an unreadable file of none is refused for its language; a file the tree
// cannot read, or a done context, is the tree's failure.
func TestParseRefusesInTheOrderOfLanguageThenRead(t *testing.T) {
	done, cancel := context.WithCancel(t.Context())
	cancel()
	notes := at("notes.txt")[0]
	for _, ctx := range []context.Context{t.Context(), done} {
		if _, err := selection.Parse(ctx, cart(), around, notes); err == nil || err.Error() != "unsupported language: "+notes {
			t.Errorf("notes: %v, want unsupported language: %s", err, notes)
		}
	}
	var unreadable *port.Unreadable
	if _, err := selection.Parse(t.Context(), cart(), around, at("src/tax.py")[0]); !errors.As(err, &unreadable) {
		t.Errorf("an unreadable source: %T %v, want Unreadable", err, err)
	}
	var cancelled *port.Cancelled
	if _, err := selection.Parse(done, cart(), around, at("src/cart.ts")[0]); !errors.As(err, &cancelled) {
		t.Errorf("a done context: %T %v, want Cancelled", err, err)
	}
}
