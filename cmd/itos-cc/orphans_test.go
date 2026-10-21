package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/sources"
	mutation "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// An orphan is kept only while its renames lead to a path where something
// is: through the range's renames and then the working tree's, never round
// a cycle; a rename to a path where nothing is, or none, leaves it one.
func TestAnOrphanRenamedToAFileIsKept(t *testing.T) {
	dir := t.TempDir()
	at := func(name string) string { return filepath.Join(dir, name) }
	if err := os.WriteFile(at("c.ts"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	since := map[string]string{at("a.ts"): at("b.ts"), at("x.ts"): at("y.ts"), at("p.ts"): at("q.ts")}
	uncommitted := map[string]string{at("b.ts"): at("c.ts"), at("q.ts"): at("p.ts")}
	for _, c := range []struct {
		name string
		want bool
	}{
		{"a.ts", true},  // a → b in the range, b → c staged, and c exists
		{"b.ts", true},  // b → c staged
		{"x.ts", false}, // x → y, and nothing is at y
		{"p.ts", false}, // p → q → p, a cycle to a path where nothing is
		{"z.ts", false}, // no rename: deleted, or moved where git does not see it
	} {
		if got, err := mutation.RenamedToAFile(t.Context(), at(c.name), since, uncommitted, sources.SourceEdits{}); err != nil || got != c.want {
			t.Errorf("renamedToAFile(%s) = %v, want %v", c.name, got, c.want)
		}
	}
	if got, err := mutation.RenamedToAFile(t.Context(), at("a.ts"), nil, nil, sources.SourceEdits{}); err != nil || got {
		t.Errorf("renamedToAFile with no renames = true, want false")
	}
}
