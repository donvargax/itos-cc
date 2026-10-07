package graph

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/mutate"
)

// posModule is the node holding Pos, with Pos's unit.
func posModule(t *testing.T, g *Graph) (*Node, Unit) {
	t.Helper()
	for _, n := range g.Nodes {
		for _, u := range n.Units {
			if u.Name == "Pos" {
				return n, u
			}
		}
	}
	t.Fatal("no node holds Pos")
	return nil, Unit{}
}

// @ID-GRAPH-34
func TestMutationResultsWhoseTestsChangedAreStale(t *testing.T) {
	root := t.TempDir()
	write := func(rel, text string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/p\n\ngo 1.22\n")
	write("a.go", "package p\n\nfunc Pos(i int) bool { return i > 0 }\n")
	test := "package p\n\nimport \"testing\"\n\nfunc TestPos(t *testing.T) {\n\tif !Pos(1) {\n\t\tt.Fatal(\"Pos\")\n\t}\n}\n"
	write("a_test.go", test)

	// The snapshot records Pos as it is and a_test.go as it was written.
	f, err := lang.ParseFile(filepath.Join(root, "a.go"))
	if err != nil {
		t.Fatal(err)
	}
	hash := mutate.UnitHash(f, f.Units[0])
	f.Close()
	sum := sha256.Sum256([]byte(test))
	write(".metrics/mutate/a.go.json", fmt.Sprintf(`{"version":1,"file":"a.go","language":"go",
		"tests":{"a_test.go":%q},
		"units":[{"namespace":"example.com/p","name":"Pos","hash":%q,"killed":2,"survived":0,"uncovered":0,"sites":2,"mutants":[]}]}`,
		hex.EncodeToString(sum[:]), hash))

	b, err := NewBuilder([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	g, _, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	if _, u := posModule(t, g); !u.Mutated || u.Stale {
		t.Fatalf("Pos before its test changed: %+v, want mutated and not stale", u)
	}

	write("a_test.go", test+"\nfunc TestNegative(t *testing.T) {\n\tif Pos(-1) {\n\t\tt.Fatal(\"Pos\")\n\t}\n}\n")

	// A graph built afresh, and the one serve rebuilds as files change.
	fresh, err := NewBuilder([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if g, _, err := fresh.Build(); err != nil {
		t.Fatal(err)
	} else if _, u := posModule(t, g); !u.Stale {
		t.Errorf("Pos in a new graph after a_test.go changed: %+v, want stale", u)
	}
	g, changed, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Errorf("the graph did not change when a_test.go did")
	}
	n, u := posModule(t, g)
	if !u.Stale {
		t.Errorf("Pos after a_test.go changed: %+v, want stale", u)
	}
	if n.Metrics == nil || n.Metrics.Stale != 1 {
		t.Errorf("module %s metrics %+v, want one stale function", n.ID, n.Metrics)
	}
}
