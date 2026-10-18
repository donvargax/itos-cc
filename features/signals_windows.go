// The interrupt on Windows, one of the build-tagged files holding a call one
// system lacks (itos-template's decision 26). Go cannot send another process
// os.Interrupt here, but a console delivers CTRL_BREAK_EVENT to a process
// group, and Go reports it to signal.Notify as os.Interrupt, as Ctrl-C at a
// console is.
// The binary is started in its own process group, so the event reaches it
// and never the tests; Go's own os/signal TestCtrlBreak works this way.
package features

import (
	"errors"
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

// errNoSignals is why Windows takes no step that sends a signal other than
// the console's interrupt: no process can send another SIGTERM or SIGINT.
var errNoSignals = errors.New("windows sends a process no SIGTERM or SIGINT of another's")

// terminate cannot send SIGTERM on Windows.
func terminate(*os.Process) error { return errNoSignals }

// ownGroup starts a process in a console process group of its own.
func ownGroup() *syscall.SysProcAttr { return interruptAttributes() }

// interruptGroup cannot send SIGINT to a group on Windows.
func interruptGroup(int) error { return errNoSignals }

// killGroup kills nothing on Windows, where no step starts a group.
func killGroup(int) error { return nil }
