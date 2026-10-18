package features

import (
	"os"
	"os/exec"
	"syscall"
)

// The interrupt steps and the product-only running driver share the
// native signal operations. Keep policy and mutation out of tagged leaves.
func interruptible(cmd *exec.Cmd) { cmd.SysProcAttr = interruptAttributes() }

// signalOf is the number of the signal that ended a process, 0 when it
// exited by itself: as Linux and macOS report a process the signal's
// default action ended, which Windows never does.
func signalOf(state *os.ProcessState) int {
	if state == nil {
		return 0
	}
	status, ok := state.Sys().(interface {
		Signaled() bool
		Signal() syscall.Signal
	})
	if !ok || !status.Signaled() {
		return 0
	}
	return int(status.Signal())
}
