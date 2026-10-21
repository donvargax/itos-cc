package exceptions_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/exceptions"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

func TestRealExceptionsContract(t *testing.T) {
	porttest.ExceptionsContract(t, func(t *testing.T, entries []mutants.Exception, missing bool) porttest.ExceptionFixture {
		root := t.TempDir()
		path := filepath.Join(root, config.File)
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
			data, err := config.Read(root)
			if err != nil {
				t.Fatal(err)
			}
			c, err := config.Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			return c.Exceptions
		}}
	})
}
func TestExceptionsPreserveCurrentDocumentModeAndInvalidFacts(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, config.File)
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
}
