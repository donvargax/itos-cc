package coveragetools

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/coverage/port"
	"github.com/donvargax/itos-cc/internal/coverage/port/porttest"
	covtest "github.com/donvargax/itos-cc/internal/coverage/porttest"
)

// workspace lays files out, their text by slash-separated path, under a
// temporary directory with its symlinks resolved, makes it the working
// directory and returns it. It holds no git repository, so it is the
// project root, and its .metrics/ is its own.
func workspace(t *testing.T, files map[string]string) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
	return dir
}

// The Measurer keeps the contract every Measurer keeps, the fake's
// included, over a temporary working directory.
func TestMeasurerKeepsTheContract(t *testing.T) {
	porttest.MeasurerContract(t, func(t *testing.T, files map[string]string) (coverage.Measurer, string) {
		dir := workspace(t, files)
		return Measurer{Log: io.Discard}, dir
	})
}

// A command says on the log what it runs, and its output goes there too.
func TestACommandRunsWithItsOutputOnTheLog(t *testing.T) {
	dir := workspace(t, map[string]string{"a.ts": "", "lcov.info": "SF:a.ts\nDA:1,1\nend_of_record\n"})
	var log bytes.Buffer
	r, failure := Measurer{Log: &log}.Measure(t.Context(), port.Request{Sources: []string{filepath.Join(dir, "a.ts")},
		Command: "echo measured", Reports: []string{"lcov.info"}})
	if failure != nil || !r.Has(filepath.Join(dir, "a.ts")) {
		t.Fatalf("Measure() = %v, %v; want the report the command left", r, failure)
	}
	if !strings.HasPrefix(log.String(), "itos-cc: coverage $ echo measured\n") || !strings.Contains(log.String(), "measured") {
		t.Errorf("the log says %q, want the command and its output", log.String())
	}
}

// A run folder that cannot be made, as when a file stands where
// .metrics/coverage goes, is unwritable, the operating system's text
// whole, and nothing is measured.
func TestARunFolderThatCannotBeMadeIsUnwritable(t *testing.T) {
	workspace(t, map[string]string{".metrics/coverage": "a file"})
	r, failure := Measurer{Log: io.Discard}.Measure(t.Context(), port.Request{})
	var unwritable port.Unwritable
	if r != nil || !errors.As(failure, &unwritable) || !strings.Contains(unwritable.Reason, "coverage") {
		t.Errorf("Measure() = %v, %v; want no report, unwritable, naming the folder", r, failure)
	}
}

// A run measures in a folder of its own, which it removes, keeps
// .metrics/coverage out of version control, and removes the folders of
// runs a day old that a killed run left, keeping a running one's and the
// reports the last run kept.
func TestARunKeepsItsFolderToItself(t *testing.T) {
	dir := workspace(t, map[string]string{
		".metrics/coverage/run-old/lcov.info": "", ".metrics/coverage/run-live/lcov.info": "", ".metrics/coverage/go-1/cover.out": "",
	})
	out := filepath.Join(dir, ".metrics", "coverage")
	old := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(filepath.Join(out, "run-old"), old, old); err != nil {
		t.Fatal(err)
	}
	if _, failure := (Measurer{Log: io.Discard}).Measure(t.Context(), port.Request{}); failure != nil {
		t.Fatalf("Measure() failed: %v", failure)
	}
	if text, err := os.ReadFile(filepath.Join(out, ".gitignore")); err != nil || string(text) != "*\n" {
		t.Errorf(".gitignore holds %q, %v; want it to ignore everything", text, err)
	}
	runs, _ := filepath.Glob(filepath.Join(out, "run-*"))
	if len(runs) != 1 || filepath.Base(runs[0]) != "run-live" {
		t.Errorf("run folders left %q, want run-live's alone", runs)
	}
	if _, err := os.Stat(filepath.Join(out, "go-1", "cover.out")); err != nil {
		t.Errorf("the last run's reports are gone: %v", err)
	}
}

// Reading what an earlier run left finds no report of a build root none
// was left of, and runs nothing.
func TestExistingCoverageWithNoReportIsMissing(t *testing.T) {
	dir := workspace(t, map[string]string{"go.mod": "module example.com/m\n\ngo 1.22\n", "a.go": "package m\n"})
	var log bytes.Buffer
	r, failure := Measurer{Log: &log}.Measure(t.Context(), port.Request{Sources: []string{filepath.Join(dir, "a.go")}, Existing: true})
	if failure != nil {
		t.Fatalf("Measure() failed: %v", failure)
	}
	if got := r.Missing(); len(got) != 1 || got[0].Cause != (port.NoReport{}) || got[0].Dir != dir || got[0].Language != "go" {
		t.Errorf("Missing() = %+v, want the Go root with no report", got)
	}
	if strings.Contains(log.String(), "$") {
		t.Errorf("the log says %q, want nothing run", log.String())
	}
}

func TestRealMutationMeasurerContract(t *testing.T) {
	covtest.MutationMeasurerContract(t, func(t *testing.T, files map[string]string) (coverage.MutationMeasurer, string) {
		dir := workspace(t, files)
		return Measurer{Log: io.Discard}, dir
	})
}

func TestRealListedContract(t *testing.T) {
	covtest.ListedContract(t, func(t *testing.T, split bool, fail string) (coverage.MutationMeasurer, string) {
		dir := workspace(t, map[string]string{"go.mod": "module example.com/t90\n\ngo 1.22\n", "main.go": covtest.ListedMain, "harness/main.go": covtest.ListedHarness})
		value := "false"
		if split {
			value = "true"
		}
		t.Setenv("T90_SPLIT", value)
		t.Setenv("T90_FAIL", fail)
		// The test owns both source and target, in this native temporary root.
		cmd := exec.CommandContext(t.Context(), "go", "build", "-buildvcs=false", "-cover", "-o", filepath.Join(dir, "app.exe"), ".")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build real covered fixture: %v %s", err, out)
		}
		return Measurer{Log: io.Discard}, dir
	})
}
func TestRealStaticInventoryContract(t *testing.T) {
	covtest.StaticInventoryContract(t, func(t *testing.T) (coverage.MutationMeasurer, string) {
		if out, err := exec.CommandContext(t.Context(), "python3", "-c", "import coverage").CombinedOutput(); err != nil {
			if os.Getenv("ITOS_CC_LANGUAGE_TOOLS") != "" {
				t.Fatalf("installed Python coverage unavailable: %v %s", err, out)
			}
			t.Skipf("Python coverage unavailable for real static inventory: %v %s", err, out)
		}
		dir := workspace(t, map[string]string{"pyproject.toml": "[project]\nname='t90'\nversion='1'\n", "hit.py": "hit=1\n", "miss.py": "def hidden():\n    return 1\n", "empty.py": "# no statements\n"})
		return Measurer{Log: io.Discard}, dir
	})
}
