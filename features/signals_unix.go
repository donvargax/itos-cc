//go:build unix

// The interrupt on Linux and macOS, one of the build-tagged files holding a
// call one system lacks (decision 26): os.Interrupt is SIGINT here, what a
// terminal sends for Ctrl-C, and any process may send it to another.
package features

import (
	"os"
	"syscall"
)

// interruptible readies cmd to be interrupted: nothing to do here, any
// process can be sent os.Interrupt.
func interruptAttributes() *syscall.SysProcAttr { return nil }

// interrupt sends the process os.Interrupt, as Ctrl-C at a terminal does.
func interrupt(p *os.Process) error {
	return p.Signal(os.Interrupt)
}

func consoleInterruptExit() uint32 { return 0 }
