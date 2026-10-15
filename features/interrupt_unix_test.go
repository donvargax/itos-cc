//go:build unix

// The interrupt on Linux and macOS, one of the build-tagged files holding a
// call one system lacks (decision 26): os.Interrupt is SIGINT here, what a
// terminal sends for Ctrl-C, and any process may send it to another.
package features

import (
	"os"
	"os/exec"
	"syscall"
)

// interruptible readies cmd to be interrupted: nothing to do here, any
// process can be sent os.Interrupt.
func interruptible(*exec.Cmd) {}

// interrupt sends the process os.Interrupt, as Ctrl-C at a terminal does.
func interrupt(p *os.Process) error {
	return p.Signal(os.Interrupt)
}

// killedBySignal is whether the process ended by a signal's default action,
// so it never handled the interrupt it was sent.
func killedBySignal(state *os.ProcessState) bool {
	status, ok := state.Sys().(syscall.WaitStatus)
	return ok && status.Signaled()
}
