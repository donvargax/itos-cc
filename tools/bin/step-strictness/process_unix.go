//go:build !windows

package main

import (
	"syscall"
)

func groupAttributes() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setpgid: true} }
func killCommandGroup(pid int) error        { return syscall.Kill(-pid, syscall.SIGKILL) }
