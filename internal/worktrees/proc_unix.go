//go:build !windows

package worktrees

import "syscall"

func nativeGroupAttributes(setGroup bool) *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: setGroup}
}
func nativeSignalGroup(group int, signal syscall.Signal) error { return syscall.Kill(group, signal) }
func nativeNonblock(fd uintptr, nonblocking bool) error {
	return syscall.SetNonblock(int(fd), nonblocking)
}
func nativeRead(fd uintptr, buffer []byte) (int, error) { return syscall.Read(int(fd), buffer) }
