//go:build windows

package worktrees

import "syscall"

func nativeGroupAttributes(_ bool) *syscall.SysProcAttr { return nil }
func nativeSignalGroup(_ int, _ syscall.Signal) error   { return syscall.ENOSYS }
func nativeNonblock(_ uintptr, _ bool) error            { return syscall.ENOSYS }
func nativeRead(_ uintptr, _ []byte) (int, error)       { var count int; return count, syscall.ENOSYS }
