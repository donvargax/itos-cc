//go:build windows

package worktrees

import "syscall"

func nativeGroupAttributes() *syscall.SysProcAttr { return nil }
func nativeKillGroup(_ int) error                 { return syscall.ENOSYS }
func nativeProbeGroup(_ int) error                { return syscall.ENOSYS }
func nativeNonblock(_ uintptr) error              { return syscall.ENOSYS }
func nativeRead(_ uintptr, _ []byte) (int, error) { return 0, syscall.ENOSYS }
