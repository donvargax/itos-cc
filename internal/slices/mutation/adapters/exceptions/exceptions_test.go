package exceptions_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/exceptions"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/settings"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

func TestRealExceptionsContract(t *testing.T) {
	porttest.ExceptionsContract(t, func(t *testing.T, entries []mutants.Exception, missing bool) porttest.ExceptionFixture {
		root := t.TempDir()
		path := filepath.Join(root, settings.File)
		put := func(entries []mutants.Exception) {
			t.Helper()
			data := []byte("# independently changed setting\nexclude: [vendor/**]\n")
			for _, e := range entries {
				updated, _, err := exceptions.ExceptDocument(data, e)
				if err != nil {
					t.Fatal(err)
				}
				data = updated
			}
			// Preserve duplicate entries by encoding each into an independent sequence item.
			if len(entries) > 1 && reflect.DeepEqual(entries[0], entries[1]) {
				single, _, err := exceptions.ExceptDocument(nil, entries[0])
				if err != nil {
					t.Fatal(err)
				}
				item := strings.SplitN(string(single), "exceptions:\n", 2)[1]
				data = []byte("mutation:\n  exceptions:\n" + item + item)
				for _, e := range entries[2:] {
					updated, _, err := exceptions.ExceptDocument(data, e)
					if err != nil {
						t.Fatal(err)
					}
					data = updated
				}
			}
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
		}
		if !missing {
			put(entries)
		}
		return porttest.ExceptionFixture{Store: exceptions.Exceptions{RootDir: root}, Replace: put, Entries: func() []mutants.Exception {
			data, err := settings.Read(root)
			if err != nil {
				t.Fatal(err)
			}
			c, err := settings.Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			return c.Exceptions
		}}
	})
}
func TestExceptionsPreserveCurrentDocumentModeAndInvalidFacts(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, settings.File)
	store := exceptions.Exceptions{RootDir: root}
	if err := os.WriteFile(path, []byte("# current setting\nexclude: [vendor/**]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	e := mutants.Exception{File: "a.go", Function: "p#F", Hash: "hash", LineInFunction: 1, Column: 2, Original: ">", Replacement: ">=", Reason: "reviewed"}
	if _, failure := store.Except(t.Context(), e); failure != nil {
		t.Fatal(failure)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if before.Mode().Perm() != after.Mode().Perm() || !strings.Contains(string(data), "# current setting") || !strings.Contains(string(data), "vendor/**") {
		t.Fatalf("current metadata lost: %s", data)
	}
	bad := []byte("exclude: true\n")
	if err := os.WriteFile(path, bad, 0600); err != nil {
		t.Fatal(err)
	}
	_, failure := store.Except(t.Context(), e)
	var invalid mutants.ExceptionInvalid
	if !errors.As(failure, &invalid) || !strings.Contains(invalid.Reason, "cannot unmarshal") {
		t.Fatalf("invalid facts: %v", failure)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != string(bad) {
		t.Fatal("invalid document overwritten")
	}
	// An aliased mutation.exceptions is refused at its anchor's key, with
	// the anchor's own fix, and nothing is written (bug-8).
	aliased := []byte("{x: &e [], mutation: {exceptions: *e}}\n")
	if err := os.WriteFile(path, aliased, 0600); err != nil {
		t.Fatal(err)
	}
	_, failure = store.Except(t.Context(), e)
	if !errors.As(failure, &invalid) || invalid.Key != "x" || !strings.Contains(invalid.Fix, "in place") {
		t.Fatalf("aliased facts: %+v", failure)
	}
	if current, err = os.ReadFile(path); err != nil || string(current) != string(aliased) {
		t.Fatalf("aliased document written: %q, %v", current, err)
	}
}

// mutation except starts a file it creates with the version this itos-cc
// writes, and never gives one to a file that is there without it, which
// reads as version 1 (slice-53).
func TestExceptStartsOnlyAFileItCreatesWithVersion(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, settings.File)
	store := exceptions.Exceptions{RootDir: root}
	e := mutants.Exception{File: "a.go", Function: "p#F", Hash: "hash", LineInFunction: 1, Column: 2, Original: ">", Replacement: ">=", Reason: "reviewed"}
	if _, failure := store.Except(t.Context(), e); failure != nil {
		t.Fatal(failure)
	}
	created, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	c, err := settings.Parse(created)
	if !strings.HasPrefix(string(created), "version: 1\nmutation:\n") || err != nil || !slices.Equal(c.Exceptions, []mutants.Exception{e}) {
		t.Fatalf("created %q, %v", created, err)
	}
	for _, text := range []string{"", "exclude: [vendor/**]\n"} {
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, failure := store.Except(t.Context(), e); failure != nil {
			t.Fatal(failure)
		}
		if data, err := os.ReadFile(path); err != nil || !strings.HasPrefix(string(data), text) || strings.Contains(string(data), "version") {
			t.Errorf("%q became %q, %v; want no version given", text, data, err)
		}
	}
	// A file there that cannot be read is no file to create.
	unreadable := exceptions.Exceptions{RootDir: t.TempDir()}
	if err := os.Mkdir(filepath.Join(unreadable.RootDir, settings.File), 0o700); err != nil {
		t.Fatal(err)
	}
	var failed mutants.ExceptionFailed
	if _, failure := unreadable.Except(t.Context(), e); !errors.As(failure, &failed) {
		t.Errorf("an unreadable itos-cc.yaml: %v, want an ExceptionFailed", failure)
	}
}
