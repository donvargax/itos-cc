package covrun

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// envHelperOut names the file the helper writes the GOCOVERDIR it was
// given to, "<unset>" for none.
const envHelperOut = "ITOS_CC_ENV_HELPER_OUT"

// A GOCOVERDIR set for itos-cc never reaches the commands its coverage
// runs start, while the -exec wrapper gives the test binaries the
// directory of the run itos-cc makes itself (slice-59, q-96): a listed
// test's collector readies without it, and the wrapper replaces it with
// its own. The wrapper runs only in a built binary go test -exec starts,
// and Python's collector needs coverage.py, so neither path is in reach
// of an in-process scenario; ID-MUT-352 holds a test run through the
// built binary.
func TestCoverageCommandsNeverInheritItosCcsGOCOVERDIR(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	helper := []string{exe, "-test.run=^TestEnvHelper$"}
	dir := t.TempDir()
	out := filepath.Join(dir, "seen")
	t.Setenv(envHelperOut, out)
	t.Setenv("GOCOVERDIR", filepath.Join(dir, "inherited"))

	var log bytes.Buffer
	started, _, err := startTestCollectors(context.Background(), []testCollector{{language: "python", dir: dir, prepare: helper}}, &log, nil, true)
	if err != nil || len(started) != 1 {
		t.Fatalf("started %d collectors (%v), want the one\n%s", len(started), err, log.String())
	}
	if seen := readSeen(t, out); seen != "<unset>" {
		t.Errorf("the collector's preparation saw GOCOVERDIR=%s, want none", seen)
	}

	own := filepath.Join(dir, "own")
	t.Setenv(coverDirEnv, own)
	// The wrapper passes its own output on, which here would read as this
	// test binary's.
	output, err := os.Create(filepath.Join(dir, "output"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = output.Close() }()
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = output, output
	code := ExecTest(helper)
	os.Stdout, os.Stderr = stdout, stderr
	if code != 0 {
		t.Fatalf("the wrapper exited %d, want 0", code)
	}
	if seen := readSeen(t, out); seen != own {
		t.Errorf("the wrapper gave its command GOCOVERDIR=%s, want %s", seen, own)
	}
}

// readSeen is what the helper wrote to out.
func readSeen(t *testing.T, out string) string {
	t.Helper()
	seen, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return string(seen)
}

// TestEnvHelper, run as a command by the test above, writes the
// GOCOVERDIR it was given where envHelperOut says.
func TestEnvHelper(t *testing.T) {
	out := os.Getenv(envHelperOut)
	if out == "" {
		return
	}
	seen, ok := os.LookupEnv("GOCOVERDIR")
	if !ok {
		seen = "<unset>"
	}
	if err := os.WriteFile(out, []byte(seen), 0o600); err != nil {
		t.Fatal(err)
	}
}
