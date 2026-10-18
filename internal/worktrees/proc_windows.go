//go:build windows

package worktrees

import "syscall"

func nativeGroupAttributes(_ bool) *syscall.SysProcAttr { return nil }
func nativeSignalGroup(_ int, _ syscall.Signal) error   { return syscall.ENOSYS }
