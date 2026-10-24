package settings

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	selport "github.com/donvargax/itos-cc/internal/selection/port"
	selporttest "github.com/donvargax/itos-cc/internal/selection/port/porttest"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
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
		return openSettings(t)
	})
}

// The reader's selection settings are the same file's, read through the same
// codec, so crap, dry and units classify a source as mutation does.
func TestSettingsReaderSelectionContract(t *testing.T) {
	selporttest.SettingsContract(t, func(t *testing.T) (selport.SettingsReader, string, func()) {
		t.Helper()
		return openSettings(t)
	})
}

// openSettings is a Reader over an itos-cc.yaml at a root of its own, and
// replace, which rewrites it.
func openSettings(t *testing.T) (Reader, string, func()) {
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
}

func TestSettingsReaderPreservesInvalidAndReadFailures(t *testing.T) {
	root := t.TempDir()
	source := configSource{root}
	reader := Reader{Source: source}
	value, failure := reader.Read(t.Context())
	if failure != nil || value == nil || value.Tests != nil || value.Selection.Exclude != nil || value.Selection.Root != root {
		t.Fatalf("absent defaults: %+v %v", value, failure)
	}
	path := filepath.Join(root, File)
	if err := os.WriteFile(path, []byte("exclude: [\"\"]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, failure = reader.Read(t.Context())
	var invalid mutants.SettingsInvalid
	if !errors.As(failure, &invalid) || invalid.Key != "exclude[0]" || invalid.Reason != `exclude[0] "" is empty` {
		t.Fatalf("invalid reason/key: %+v", failure)
	}
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(errors.New("caller already stopped"))
	_, stopped := reader.Read(ctx)
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
	_, failure = reader.Read(t.Context())
	if !errors.As(failure, &invalid) || invalid.Key != "" || !strings.HasPrefix(invalid.Reason, "cannot be read: ") || strings.Contains(invalid.Reason, root) {
		t.Fatalf("unreadable file: %+v", failure)
	}
	_, cancelled := Reader{Source: cancelledSource{}}.Read(ctx)
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

// The selection settings fail as mutation's do, each failure the selection
// port's of the same kind: invalid with its reason and key, a cancelled read
// keeping its cause, and a missing source a read that failed short of them.
func TestSettingsReaderSelectionFailsAsMutationsDoes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, File), []byte("exclude: [\"\"]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, failure := Reader{Source: configSource{root}}.ReadSelection(t.Context())
	var invalid *selport.SettingsInvalid
	if !errors.As(failure, &invalid) || invalid.Key != "exclude[0]" || invalid.Reason != `exclude[0] "" is empty` {
		t.Fatalf("invalid reason/key: %+v", failure)
	}
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(errors.New("caller already stopped"))
	_, cancelled := Reader{Source: cancelledSource{}}.ReadSelection(ctx)
	var gone *selport.Cancelled
	if !errors.As(cancelled, &gone) || !errors.Is(cancelled, context.Cause(ctx)) {
		t.Fatalf("cancelled read: %v", cancelled)
	}
	_, missing := Reader{}.ReadSelection(t.Context())
	var failed *selport.SettingsFailed
	if !errors.As(missing, &failed) || missing.Error() != "mutation settings byte source binding is missing" {
		t.Fatalf("missing source: %v", missing)
	}
}
