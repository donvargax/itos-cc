//go:build linux || darwin

package processes

import (
	"errors"
	"os"
	"os/exec"
	"testing"
)

// bug-5: os/exec calls a command's Cancel when its context ends before Wait
// has returned, and reports a Cancel error other than os.ErrProcessDone as
// Wait's error when the command otherwise exited 0. --fail-fast's cancel can
// land after a mutant's test command has exited and its group is gone, so a
// mutant that finished must be judged as it finished, never an internal
// error. The race is about 1 run in 30 through mutation run, which no
// scenario can show reliably; this real-OS check holds its cause directly:
// killGroup's Cancel, called once the group is gone, says the process is
// done.
func TestKillGroupCancelOfAGoneGroupIsProcessDone(t *testing.T) {
	cmd := exec.CommandContext(t.Context(), "/bin/sh", "-c", "exit 0")
	killGroup(cmd, PlatformAtBoundary(""))
	if err := cmd.Run(); err != nil {
		t.Fatalf("command run = %v; want its own successful exit", err)
	}
	err := cmd.Cancel()
	if !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("Cancel after the group is gone = %v; want os.ErrProcessDone", err)
	}
}
