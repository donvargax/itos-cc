//go:build !windows

package main

import (
	"fmt"
	"strings"
	"testing"
)

// The scenario of "Rule: mutation run --fail-fast stops at the first
// actionable failure" in features/mutation.feature, the fail-fast-run
// slice, whose holder still asserts what its scenario does not. --fail-fast
// runs on Linux and macOS only, so it builds only where Unix process groups
// exist. It drives the CLI and reads its --json object as raw maps.
//
// Its own-baseline fixture judges its mutants with a shell script, test.sh,
// given as --test-command: it runs at the module root of a worker copy,
// appends a line to $FF_DIR/runs on every run, and decides each mutant by
// grepping the source for its original text. The listed-test case uses
// ffListedFiles, a small module whose e2e harness runs its built binary by
// listed test ID.

// @ID-MUT-196. Kept: the own baseline's failure leaves its file and
// mutant blocked and the file it never reached unattempted, and the
// failing selection's baseline is never logged as passed, which no
// scenario asserts: ID-MUT-196's reads their outcomes and baselines, and
// ID-MUT-197's the listed alternative's states. ID-MUT-192 to 195, 197,
// 198 and 211's holders went, every assertion theirs or another
// scenario's (T-130).
func TestAFailingBaselineStopsTheRunWithoutInventingOutcomes(t *testing.T) {
	t.Parallel()
	t.Run("own baseline", func(t *testing.T) {
		// Two modules, so two baselines: a's test.sh fails on the source
		// as it is, which marks itself broken; b's would kill its mutant.
		_, ff := ffRepo(t, map[string]string{
			"a/go.mod":  "module example.com/a\n\ngo 1.22\n",
			"a/main.go": "package main\n\n// BROKEN\nfunc A(n int) bool { return n > 5 }\n\nfunc main() {}\n",
			"a/test.sh": ffScript("grep -q BROKEN main.go && exit 1", "exit 0"),
			"b/go.mod":  "module example.com/b\n\ngo 1.22\n",
			"b/main.go": "package main\n\nfunc B(n int) bool { return n > 6 }\n\nfunc main() {}\n",
			"b/test.sh": ffScript("grep -q 'n > 6' main.go || exit 1", "exit 0"),
		})
		o := shellRun(t, "--no-annotate", "--workers", "1", "--fail-fast", "a/main.go", "b/main.go")
		logOutcome(t, &o)
		f := o.ff(t)
		p := f.problem("mutation.baseline-failed")
		if o.code != 1 || p == nil || p["file"] != "a/main.go" {
			t.Errorf("exit %d, problems %v: want 1 with mutation.baseline-failed for a/main.go", o.code, f.Problems)
		}
		if !f.stopped() || f.stopRule() != "mutation.baseline-failed" || f.stopSubject()["file"] != "a/main.go" {
			t.Errorf("stop = %v: want the run stopped by a/main.go's baseline", f.Stop)
		}
		// The affected mutants have no invented outcome.
		a, b := f.file("a/main.go"), f.file("b/main.go")
		if a == nil || a["baseline"] != "failed" || a["state"] != "blocked" || !undecided(ffMutant(a, "A"), "blocked") {
			t.Errorf("a/main.go = %v: want its baseline failed, and the file and its mutant blocked with no outcome", a)
		}
		// No stage reports a baseline as passed that never ran: b's never did.
		if b == nil || b["baseline"] != "not-run" || b["state"] != "unattempted" || !undecided(ffMutant(b, "B"), "unattempted") {
			t.Errorf("b/main.go = %v: want its baseline not-run, and the file and its mutant unattempted", b)
		}
		// a's baseline runs in its worker's copy, then once more in the
		// project's own folder (slice-19), failing there too; b's never.
		if n := runsIn(ff); n != 2 {
			t.Errorf("test.sh ran %d times, want twice: a's baseline in its copy and in the project", n)
		}
	})

	t.Run("listed selection baseline", func(t *testing.T) {
		// label's mutant survives its own tests; the clean baseline of the
		// listed tests that reach it fails.
		ffListedRepo(t, "label")
		useEnv(t, "FF_FAIL_ALONE", "1")
		o := listedRun(t, "--workers", "1", "--fail-fast")
		logOutcome(t, &o)
		f := o.ff(t)
		p := f.problem("tests.selection-failed")
		if o.code != 1 || p == nil || fmt.Sprint(p["ids"]) != "[ID-A-01 ID-A-02]" {
			t.Errorf("exit %d, problems %v: want 1 with tests.selection-failed for [ID-A-01 ID-A-02]", o.code, f.Problems)
		}
		if !f.stopped() || f.stopRule() != "tests.selection-failed" || fmt.Sprint(f.stopSubject()["ids"]) != "[ID-A-01 ID-A-02]" {
			t.Errorf("stop = %v: want the run stopped by the failing selection", f.Stop)
		}
		m := f.file("main.go")
		if got := ffMutant(m, "label"); !undecided(got, "blocked") {
			t.Errorf("label's mutant = %v: want it blocked, with no killed, timed-out or survived outcome", got)
		}
		if strings.Contains(o.stderr, "baseline of ID-A-01 ID-A-02 passed") {
			t.Errorf("stderr reports the failing selection's baseline passed:\n%s", o.stderr)
		}
		if m == nil || m["baseline"] != "passed" || m["state"] != "blocked" {
			t.Errorf("main.go = %v: want its own baseline passed, as it ran, and the file blocked", m)
		}
	})
}
