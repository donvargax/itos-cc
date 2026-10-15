//go:build !windows

package worktrees

import "syscall"

func nativeGroupAttributes() *syscall.SysProcAttr       { return &syscall.SysProcAttr{Setpgid: true} }
func nativeKillGroup(pgid int) error                    { return syscall.Kill(-pgid, syscall.SIGKILL) }
func nativeProbeGroup(pgid int) error                   { return syscall.Kill(-pgid, 0) }
func nativeNonblock(fd uintptr) error                   { return syscall.SetNonblock(int(fd), true) }
func nativeRead(fd uintptr, buffer []byte) (int, error) { return syscall.Read(int(fd), buffer) }
