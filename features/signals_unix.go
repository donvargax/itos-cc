//go:build unix

// The interrupt on Linux and macOS, one of the build-tagged files holding a
// call one system lacks (itos-template's decision 26): os.Interrupt is
// SIGINT here, what a terminal sends for Ctrl-C, and any process may send
// it to another.
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

// terminate sends the process SIGTERM, as CI does to cancel a job.
func terminate(p *os.Process) error { return p.Signal(syscall.SIGTERM) }

// ownGroup starts a process as the leader of a process group of its own,
// which interruptGroup signals as a terminal signals its foreground group.
func ownGroup() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setpgid: true} }

// interruptGroup sends SIGINT to every process of the group led by pid, as
// Ctrl-C at a terminal reaches the shell and the command it runs.
func interruptGroup(pid int) error { return syscall.Kill(-pid, syscall.SIGINT) }

// killGroup kills every process of the group led by pid.
func killGroup(pid int) error { return syscall.Kill(-pid, syscall.SIGKILL) }
