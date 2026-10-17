package main

import (
	"testing"
)

// A helper the Python tests share. ID-MUT-214 holds the
// python-no-pytest-cache slice's old test whole (T-101).

// logPytestRun logs what o printed once t has failed. The step builds on
// every OS, so it cannot use logOutcome, which only the !windows fail-fast
// steps define.
func logPytestRun(t *testing.T, o *outcome) {
	t.Helper()
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("exit %d\nstdout:\n%s\nstderr:\n%s", o.code, o.stdout, o.stderr)
		}
	})
}
