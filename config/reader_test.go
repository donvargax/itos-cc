package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	// A file that cannot be read is invalid settings naming no key (bug-4),
	// in the system's words without the path, and a read the caller's
	// cancellation stopped stays a cancellation.
	if _, _, actual := source.Read(t.Context()); actual == nil {
		t.Fatal("a directory in the file's place was read")
	}
	_, _, failure = reader.Read(t.Context())
	if !errors.As(failure, &invalid) || invalid.Key != "" || !strings.HasPrefix(invalid.Reason, "cannot be read: ") || strings.Contains(invalid.Reason, root) {
		t.Fatalf("unreadable file: %+v", failure)
	}
	_, _, cancelled := Reader{Source: cancelledSource{}}.Read(ctx)
	var gone mutants.InputCancelled
	if !errors.As(cancelled, &gone) || !errors.Is(cancelled, context.Cause(ctx)) {
		t.Fatalf("cancelled read: %v", cancelled)
	}
}

// cancelledSource is a read the caller's cancellation stopped.
type cancelledSource struct{}

func (cancelledSource) Read(ctx context.Context) (string, []byte, error) {
	return "", nil, context.Cause(ctx)
}
