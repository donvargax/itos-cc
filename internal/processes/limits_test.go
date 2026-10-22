package processes

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestKeptOutputKeepsBothEndsAndCountsTheLinesBetween holds what a run
// keeps of a command's output (issue #47): all of it up to twice
// keptOutputEnd, else its first and last keptOutputEnd bytes, with the
// lines of those between counted, however the writes split it.
func TestKeptOutputKeepsBothEndsAndCountsTheLinesBetween(t *testing.T) {
	var whole bytes.Buffer
	for i := range 100 {
		fmt.Fprintf(&whole, "line %03d\n", i+1)
	}
	small := &keptOutput{}
	_, _ = small.Write(whole.Bytes())
	if text, written, _, leftOut := small.kept(); text != whole.String() || written != int64(whole.Len()) || leftOut != 0 {
		t.Fatalf("100 short lines kept as %d bytes of %d written, %d left out", len(text), written, leftOut)
	}

	line := strings.Repeat("y", 99) + "\n" // 100 bytes
	lines := 10_000                        // 1,000,000 bytes
	for _, piece := range []int{1, 7, 4096, 1 << 20} {
		k := &keptOutput{}
		all := strings.Repeat(line, lines)
		for rest := all; rest != ""; {
			n := min(piece, len(rest))
			_, _ = k.Write([]byte(rest[:n]))
			rest = rest[n:]
		}
		text, written, cut, leftOut := k.kept()
		if written != int64(len(all)) || len(text) != 2*keptOutputEnd || cut != keptOutputEnd {
			t.Fatalf("pieces of %d: kept %d bytes cut at %d of %d written", piece, len(text), cut, written)
		}
		if text[:cut] != all[:cut] || text[cut:] != all[len(all)-keptOutputEnd:] {
			t.Fatalf("pieces of %d: the ends kept are not the output's", piece)
		}
		dropped := all[keptOutputEnd : len(all)-keptOutputEnd]
		if want := strings.Count(dropped, "\n"); leftOut != want {
			t.Fatalf("pieces of %d: %d lines left out, want %d", piece, leftOut, want)
		}
	}
}

// TestKeptOutputActsOnceOverItsCap holds that the cap's action runs once,
// when the output passes its limit, counting what a caller's own writer
// took as well, and that one passed before it is armed runs on arming.
func TestKeptOutputActsOnceOverItsCap(t *testing.T) {
	calls := 0
	k := &keptOutput{limit: 10}
	k.arm(func() { calls++ })
	_, _ = k.Write([]byte("0123456789"))
	if calls != 0 {
		t.Fatal("the cap acted at its limit, not past it")
	}
	var elsewhere bytes.Buffer
	_, _ = (&countedWriter{to: &elsewhere, kept: k}).Write([]byte("x"))
	_, _ = k.Write([]byte("more"))
	if calls != 1 || elsewhere.String() != "x" {
		t.Fatalf("the cap acted %d times, the caller's writer holds %q", calls, elsewhere.String())
	}
	early := &keptOutput{limit: 1}
	_, _ = early.Write([]byte("ab"))
	armed := false
	early.arm(func() { armed = true })
	if !armed {
		t.Fatal("a cap passed before arming was not acted on")
	}
}

// TestMemoryIsMeasuredOnLinuxAlone holds the OS as a value: only Linux
// measures a command's memory, so no memory cap holds on macOS or Windows.
func TestMemoryIsMeasuredOnLinuxAlone(t *testing.T) {
	for platform, want := range map[string]bool{"linux": true, "darwin": false, "windows": false} {
		if got := MemoryMeasured(platform); got != want {
			t.Errorf("MemoryMeasured(%q) = %v, want %v", platform, got, want)
		}
	}
}

// TestStatGroupAndPagesReadsAProcessStatLine reads the group and resident
// pages after the command's name, which may hold spaces and parentheses.
func TestStatGroupAndPagesReadsAProcessStatLine(t *testing.T) {
	stat := "4242 (a (b) c) S 1 4200 4200 0 -1 4194560 100 0 0 0 1 2 0 0 20 0 3 0 5000 123456789 777 18446744073709551615 1 1 0 0 0 0 0 0 0 0 0 0 17 3 0 0 0 0 0\n"
	group, pages, err := statGroupAndPages([]byte(stat))
	if err != nil || group != 4200 || pages != 777 {
		t.Fatalf("group %d, pages %d, err %v; want 4200, 777", group, pages, err)
	}
	if _, _, err := statGroupAndPages([]byte("4242 (x) S 1 2")); err == nil {
		t.Fatal("a short stat line was read")
	}
}

// TestACommandOverItsOutputCapIsKilled holds the output cap on every
// system, the host's own platform as the runner's value: a command that
// writes without end is killed over its cap, long before its timeout, and
// judged over-limit, never timed out, its output kept bounded.
func TestACommandOverItsOutputCapIsKilled(t *testing.T) {
	t.Setenv(runnerFixtureMode, "flood")
	root := t.TempDir()
	w := &worker{runner: Runner{Platform: PlatformAtBoundary("")}}
	started := time.Now()
	r, err := w.runLimited(t.Context(), Command{Root: root, Dir: root, Args: commandRunnerHelperArgs()}, 30*time.Second, Limits{Output: 1 << 20})
	if err != nil || r.OverLimit != OverOutput || r.TimedOut || r.Passed {
		t.Fatalf("result = %+v, err = %v", summary(r), err)
	}
	if took := time.Since(started); took > 20*time.Second {
		t.Fatalf("the command over its cap ended after %v", took)
	}
	if r.OutputBytes <= 1<<20 || len(r.Output) > 2*keptOutputEnd {
		t.Fatalf("wrote %d bytes, kept %d", r.OutputBytes, len(r.Output))
	}
}

// TestNoMemoryCapHoldsWhereMemoryIsNotMeasured passes Windows as the
// platform value: its run measures no memory, so a command that holds
// more than the cap runs to its own end.
func TestNoMemoryCapHoldsWhereMemoryIsNotMeasured(t *testing.T) {
	noMemoryCap(t, "windows")
}

// noMemoryCap runs the hoarding fixture on platform under a 32 MiB memory
// cap, which no measurement there holds it to.
func noMemoryCap(t *testing.T, platform string) {
	t.Helper()
	t.Setenv(runnerFixtureMode, "hoard")
	root := t.TempDir()
	w := &worker{runner: Runner{Platform: platform}}
	r, err := w.runLimited(t.Context(), Command{Root: root, Dir: root, Args: commandRunnerHelperArgs()}, 30*time.Second, Limits{Memory: 32 << 20})
	if err != nil || r.OverLimit != "" || r.MemoryMeasured || r.ExitCode != 3 {
		t.Fatalf("on %s: result = %+v, err = %v", platform, summary(r), err)
	}
}

// summary is r without its output, for a failure's message.
func summary(r Result) Result {
	r.Output = fmt.Sprintf("<%d bytes kept>", len(r.Output))
	return r
}
