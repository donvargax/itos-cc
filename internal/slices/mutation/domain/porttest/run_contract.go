package porttest

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// ListingContract holds actual stdout/stderr separation and ordinary exit facts.
// open supplies an independently declared or genuinely executed fixture command.
func ListingContract(t *testing.T, open func(*testing.T, bool) (mutants.TestLister, mutants.Command, *bytes.Buffer)) {
	t.Helper()
	for _, fails := range []bool{false, true} {
		name := "success"
		if fails {
			name = "unsuccessful command"
		}
		t.Run(name, func(t *testing.T) {
			lister, command, diagnostics := open(t, fails)
			value, failure := lister.List(t.Context(), command)
			if failure != nil {
				t.Fatal(failure)
			}
			if !strings.Contains(diagnostics.String(), "independent diagnostic") {
				t.Fatalf("stderr lost: %q", diagnostics.String())
			}
			if strings.Contains(value.Stdout, "diagnostic") {
				t.Fatalf("stderr admitted as stdout: %q", value.Stdout)
			}
			want := []coverage.Test{{ID: "first", File: "tests/a.py"}, {ID: "second"}}
			if got := mutants.ParseListing(value.Stdout); !reflect.DeepEqual(got, want) {
				t.Fatalf("actual stdout rows: %+v", got)
			}
			if fails {
				if value.ExitCode != 7 || value.Reason == "" {
					t.Fatalf("failed command: %+v", value)
				}
			} else if value.ExitCode != 0 || value.Reason != "" {
				t.Fatalf("successful command: %+v", value)
			}
			ctx, cancel := context.WithCancelCause(t.Context())
			cause := errors.New("listing caller stopped")
			cancel(cause)
			_, failure = lister.List(ctx, command)
			if !errors.Is(failure, cause) {
				t.Fatalf("caller cause: %v", failure)
			}
		})
	}
}

// SourceEditsContract holds publication-time reread and no identical rewrite.
// read/write are fixture controls over the actual backing state, never port responses.
func SourceEditsContract(t *testing.T, open func(*testing.T) (mutants.SourceEdits, string, func() string, func(string))) {
	t.Helper()
	writer, path, read, write := open(t)
	write("// independently changed after planning\nfunc actual() {}\n")
	if failure := writer.Annotate(t.Context(), path, "//", "// itos-cc mutation:\n// actual summary\n// end itos-cc mutation\n"); failure != nil {
		t.Fatal(failure)
	}
	if got := read(); !strings.Contains(got, "independently changed") || !strings.HasSuffix(got, "// itos-cc mutation:\n// actual summary\n// end itos-cc mutation\n") {
		t.Fatalf("stale planning bytes overwrote current source: %q", got)
	}
	before := read()
	if failure := writer.Annotate(t.Context(), path, "//", "// itos-cc mutation:\n// actual summary\n// end itos-cc mutation\n"); failure != nil || read() != before {
		t.Fatalf("identical update: %v %q", failure, read())
	}
	if missing, failure := writer.Missing(t.Context(), path); failure != nil || missing {
		t.Fatalf("present: %v %v", missing, failure)
	}
	if missing, failure := writer.Missing(t.Context(), filepath.Join(filepath.Dir(path), "absent.go")); failure != nil || !missing {
		t.Fatalf("absent: %v %v", missing, failure)
	}
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("source caller stopped")
	cancel(cause)
	if failure := writer.Annotate(ctx, path, "//", "changed"); !errors.Is(failure, cause) || read() != before {
		t.Fatalf("cancelled annotation: %v", failure)
	}
	if _, failure := writer.Missing(ctx, path); !errors.Is(failure, cause) {
		t.Fatalf("cancelled stat: %v", failure)
	}
}
