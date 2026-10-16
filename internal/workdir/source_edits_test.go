package workdir_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/internal/mutants/porttest"
	"github.com/donvargax/itos-cc/internal/workdir"
)

// The real writer's current bytes, permission and no-write semantics cannot be
// established by a public command's final content alone.
func TestRealSourceEditsContractAndNoIdenticalWrite(t *testing.T) {
	porttest.SourceEditsContract(t, func(t *testing.T) (mutants.SourceEdits, string, func() string, func(string)) {
		path := filepath.Join(t.TempDir(), "current.go")
		write := func(text string) {
			t.Helper()
			if err := os.WriteFile(path, []byte(text), 0640); err != nil {
				t.Fatal(err)
			}
		}
		write("original planning bytes")
		read := func() string {
			t.Helper()
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			return string(data)
		}
		return workdir.SourceEdits{}, path, read, write
	})
	path := filepath.Join(t.TempDir(), "same.go")
	if err := os.WriteFile(path, []byte("body\n\n// itos-cc mutation:\n// actual summary\n// end itos-cc mutation\n"), 0640); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if failure := (workdir.SourceEdits{}).Annotate(t.Context(), path, "//", "// itos-cc mutation:\n// actual summary\n// end itos-cc mutation\n"); failure != nil {
		t.Fatal(failure)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) || before.Mode() != after.Mode() {
		t.Fatalf("identical source rewritten: %v -> %v", before, after)
	}
	beforeBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if failure := (workdir.SourceEdits{}).Annotate(t.Context(), path, "//", "// itos-cc mutation:\n// changed current summary\n// end itos-cc mutation\n"); failure != nil {
		t.Fatal(failure)
	}
	changed, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	afterBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeBytes) == string(afterBytes) || before.Mode().Perm() != changed.Mode().Perm() {
		t.Fatalf("changed annotation did not preserve current permission bits: %v -> %v", before.Mode(), changed.Mode())
	}

}
