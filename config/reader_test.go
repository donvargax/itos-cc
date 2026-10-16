package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/internal/mutants/porttest"
)

// The actual config codec and disk read are adapter obligations; the shared
// suite holds the same current-value/ownership semantics as the domain fake.
type configSource struct{ root string }

func (s configSource) Read(context.Context) (string, []byte, error) {
	text, err := Read(s.root)
	return s.root, text, err
}

func TestSettingsReaderContract(t *testing.T) {
	porttest.SettingsContract(t, func(t *testing.T) (mutants.SettingsReader, string, func()) {
		t.Helper()
		root := t.TempDir()
		path := filepath.Join(root, File)
		write := func(text string) {
			t.Helper()
			if err := os.WriteFile(path, []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
		}
		write(`exclude: [generated/**]
mutation:
  exceptions:
    - file: board.go
      function: Value
      hash: supplied-hash
      line_in_function: 1
      column: 4
      original: ">"
      replacement: ">="
      reason: independent reason
  tests:
    list: actual-list
    run: selected {pattern}
    ids_pattern: "{ids}"
    join: {each: "{id}", sep: ","}
    support: [support.txt]
`)
		return Reader{Source: configSource{root}}, root, func() { write("exclude: [replaced/**]\n") }
	})
}

func TestSettingsReaderPreservesInvalidAndReadFailures(t *testing.T) {
	root := t.TempDir()
	source := configSource{root}
	reader := Reader{Source: source}
	_, value, failure := reader.Read(t.Context())
	if failure != nil || value == nil || value.Tests != nil || value.Exclude != nil {
		t.Fatalf("absent defaults: %+v %v", value, failure)
	}
	path := filepath.Join(root, File)
	if err := os.WriteFile(path, []byte("exclude: [\"\"]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, _, failure = reader.Read(t.Context())
	var invalid mutants.SettingsInvalid
	if !errors.As(failure, &invalid) || invalid.Key != "exclude[0]" || invalid.Reason != `exclude[0] "" is empty` {
		t.Fatalf("invalid reason/key: %+v", failure)
	}
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(errors.New("caller already stopped"))
	_, _, stopped := reader.Read(ctx)
	if !errors.As(stopped, &invalid) || invalid.Key != "exclude[0]" {
		t.Fatalf("completed disk read must retain invalid-config precedence: %v", stopped)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	_, _, actual := source.Read(t.Context())
	_, _, failure = reader.Read(t.Context())
	var failed mutants.InputFailed
	if actual == nil || !errors.As(failure, &failed) || failed.Cause.Error() != actual.Error() {
		t.Fatalf("raw read error changed: %v vs %v", actual, failure)
	}
}
