package covrun

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// A conversion that fails ends its error with what it printed (slice-74,
// issue #84). ID-MUT-395 and ID-MUT-396 hold that through the real converter,
// and are binary-only, so the code proof, which measures in-process tests,
// reaches neither. These are unit tests for that reason, and because the real
// converter prints whenever it fails: no entry point reaches a failure that
// printed nothing.

// errConverter is how a fake conversion fails.
var errConverter = errors.New("execute covdata exited 1")

// printingThenFailing is an executor that writes out to the command's stdout
// and problem to its stderr, then fails.
func printingThenFailing(out, problem string) CommandExecutor {
	return func(_ context.Context, cmd *exec.Cmd) error {
		_, _ = fmt.Fprint(cmd.Stdout, out)
		_, _ = fmt.Fprint(cmd.Stderr, problem)
		return errConverter
	}
}

func TestAFailedConversionThatPrintedNothingKeepsItsError(t *testing.T) {
	for name, printed := range map[string]string{"nothing": "", "blank lines": " \n\t\n"} {
		t.Run(name, func(t *testing.T) {
			var log bytes.Buffer
			err := runConversion(t.Context(), exec.CommandContext(t.Context(), "covdata"), &log, printingThenFailing("", printed))
			if !errors.Is(err, errConverter) || err.Error() != errConverter.Error() {
				t.Errorf("gave %q, want %q alone", err, errConverter)
			}
		})
	}
}

func TestAFailedConversionEndsItsErrorWithWhatBothItsStreamsPrinted(t *testing.T) {
	var log bytes.Buffer
	err := runConversion(t.Context(), exec.CommandContext(t.Context(), "covdata"), &log, printingThenFailing("read covmeta.0\n", "\nerror: covmeta.0: unexpected EOF\n"))
	want := errConverter.Error() + ": read covmeta.0\n\nerror: covmeta.0: unexpected EOF"
	if !errors.Is(err, errConverter) || err.Error() != want {
		t.Errorf("gave %q, want %q", err, want)
	}
	if got, printed := log.String(), "read covmeta.0\n\nerror: covmeta.0: unexpected EOF\n"; got != printed {
		t.Errorf("the log holds %q, want all it printed, %q", got, printed)
	}
}

func TestAConversionThatSucceedsOnlyLogsWhatItPrinted(t *testing.T) {
	var log bytes.Buffer
	err := runConversion(t.Context(), exec.CommandContext(t.Context(), "covdata"), &log, func(_ context.Context, cmd *exec.Cmd) error {
		_, _ = fmt.Fprint(cmd.Stderr, "a warning\n")
		return nil
	})
	if err != nil || log.String() != "a warning\n" {
		t.Errorf("gave %v, the log holding %q, want no error and the warning", err, log.String())
	}
}

func TestAConversionsStdoutAndStderrAreOneWriter(t *testing.T) {
	var log bytes.Buffer
	_ = runConversion(t.Context(), exec.CommandContext(t.Context(), "covdata"), &log, func(_ context.Context, cmd *exec.Cmd) error {
		if cmd.Stdout != cmd.Stderr {
			t.Error("stdout and stderr are two writers, which exec would copy to in two goroutines")
		}
		return nil
	})
}

// isConversion says cmd names the output a conversion writes, which a test
// run does not.
func isConversion(cmd *exec.Cmd) bool {
	return slices.ContainsFunc(cmd.Args, func(arg string) bool { return strings.HasPrefix(arg, "-o=") })
}

func TestAFailedListedConversionSaysWhatItPrinted(t *testing.T) {
	root := t.TempDir()
	p, rec := listed(root)
	sources := goModule(t, root)
	var log bytes.Buffer
	_, _, err := MeasureTestsSupervised(t.Context(), p, filepath.Join(root, "data"), sources, &log, func(ctx context.Context, cmd *exec.Cmd) error {
		if isConversion(cmd) {
			return printingThenFailing("", "error: covmeta.0: unexpected EOF\n")(ctx, cmd)
		}
		return rec.execute(ctx, cmd)
	})
	if err == nil || !strings.HasSuffix(err.Error(), ": error: covmeta.0: unexpected EOF") || !strings.Contains(err.Error(), "convert listed test a coverage: ") {
		t.Errorf("gave %v, want the listed test's conversion ending in what it printed", err)
	}
	if !strings.Contains(log.String(), "error: covmeta.0: unexpected EOF\n") {
		t.Errorf("the log does not hold what it printed:\n%s", log.String())
	}
}

func TestAFailedIntegrationConversionSaysWhatItPrinted(t *testing.T) {
	root := t.TempDir()
	p := Plan{Language: "go", CoverDir: root, Written: "covmeta.*", Convert: [][]string{{"covdata"}}}
	if err := os.WriteFile(filepath.Join(p.CoverDir, "covmeta.0"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	_, _, _, err := p.integrateSupervised(t.Context(), &log, printingThenFailing("", "error: covmeta.0: unexpected EOF\n"))
	if err == nil || !strings.HasSuffix(err.Error(), ": error: covmeta.0: unexpected EOF") || !strings.Contains(err.Error(), "convert go integration coverage: ") {
		t.Errorf("gave %v, want the integration conversion ending in what it printed", err)
	}
	if !strings.Contains(log.String(), "error: covmeta.0: unexpected EOF\n") {
		t.Errorf("the log does not hold what it printed:\n%s", log.String())
	}
}
