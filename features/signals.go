package features

import (
	"os"
	"os/exec"
	"runtime"
)

// The existing interrupt fixture and the product-only running driver share the
// native signal operations. Keep policy and mutation out of tagged leaves.
func interruptible(cmd *exec.Cmd) { cmd.SysProcAttr = interruptAttributes() }

func killedBySignal(state *os.ProcessState) bool { return signalExit(state, runtime.GOOS) }
func signalExit(state *os.ProcessState, platform string) bool {
	if platform == "windows" {
		return uint32(state.ExitCode()) == consoleInterruptExit()
	}
	status, ok := state.Sys().(interface{ Signaled() bool })
	return ok && status.Signaled()
}
