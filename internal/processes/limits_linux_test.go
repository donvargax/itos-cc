package processes

import (
	"testing"
	"time"
)

// TestACommandOverItsMemoryCapIsKilledOnLinux holds the memory cap where
// the runner measures memory: a command whose group holds ever more is
// killed over its cap, before it holds what it would, judged over-limit,
// its peak measured.
func TestACommandOverItsMemoryCapIsKilledOnLinux(t *testing.T) {
	t.Setenv(runnerFixtureMode, "hoard")
	root := t.TempDir()
	w := &worker{runner: Runner{Platform: "linux"}}
	r, err := w.runLimited(t.Context(), Command{Root: root, Dir: root, Args: commandRunnerHelperArgs()}, 30*time.Second, Limits{Memory: 64 << 20})
	if err != nil || r.OverLimit != OverMemory || r.ExitCode == 3 || !r.MemoryMeasured || r.PeakMemory <= 64<<20 {
		t.Fatalf("result = %+v, err = %v", summary(r), err)
	}
}

// TestABaselinesPeakMemoryIsMeasuredOnLinux holds that a run with no cap
// gives its group's peak memory, which sizes its mutants' caps.
func TestABaselinesPeakMemoryIsMeasuredOnLinux(t *testing.T) {
	t.Setenv(runnerFixtureMode, "hoard")
	root := t.TempDir()
	w := &worker{runner: Runner{Platform: "linux"}}
	r, err := w.runLimited(t.Context(), Command{Root: root, Dir: root, Args: commandRunnerHelperArgs()}, 30*time.Second, Limits{})
	if err != nil || r.OverLimit != "" || r.ExitCode != 3 || !r.MemoryMeasured || r.PeakMemory < 128<<20 {
		t.Fatalf("result = %+v, err = %v", summary(r), err)
	}
}
