//go:build !windows

package worktrees

import "syscall"

func nativeGroupAttributes(setGroup bool) *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: setGroup}
}
func nativeSignalGroup(group int, signal syscall.Signal) error { return syscall.Kill(group, signal) }
