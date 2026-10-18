// The interrupt on Windows, one of the build-tagged files holding a call one
// system lacks (itos-template's decision 26). Go cannot send another process
// os.Interrupt here, but a console delivers CTRL_BREAK_EVENT to a process
// group, and Go reports it to signal.Notify as os.Interrupt, as Ctrl-C at a
// console is.
// The binary is started in its own process group, so the event reaches it
// and never the tests; Go's own os/signal TestCtrlBreak works this way.
package features

import (
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

// interruptible starts cmd in a process group of its own, which
// GenerateConsoleCtrlEvent can address by the process's ID.
func interruptAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

// interrupt sends CTRL_BREAK_EVENT to the process's group. CTRL_C_EVENT
// cannot be sent to a group of its own, which starts with Ctrl-C ignored.
func interrupt(p *os.Process) error {
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(p.Pid))
}

// killedBySignal is whether the process ended by the console's default
// handler, which exits STATUS_CONTROL_C_EXIT, so it never handled the
// interrupt it was sent.
func consoleInterruptExit() uint32 { return uint32(windows.STATUS_CONTROL_C_EXIT) }
