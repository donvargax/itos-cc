//go:build windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

var procLockFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx")

// lockMarker takes an exclusive LockFileEx lock on the first byte of f, as
// a mutation run takes it on its worker directory's marker.
func lockMarker(f *os.File) error {
	const lockfileFailImmediately, lockfileExclusiveLock = 0x1, 0x2
	var ol syscall.Overlapped
	r, _, err := procLockFileEx.Call(f.Fd(), lockfileFailImmediately|lockfileExclusiveLock, 0, 1, 0, uintptr(unsafe.Pointer(&ol)))
	if r == 0 {
		return err
	}
	return nil
}
