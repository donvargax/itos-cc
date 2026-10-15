//go:build windows

package features

import (
	"os"

	"golang.org/x/sys/windows"
)

const (
	fixtureLockFlags    = windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY
	fixtureLockReserved = 0
	fixtureLockLength   = 1
	fixtureLockHigh     = 0
)

func lockFixture(f *os.File) error {
	return windows.LockFileEx(windows.Handle(f.Fd()), fixtureLockFlags, fixtureLockReserved, fixtureLockLength, fixtureLockHigh, &windows.Overlapped{})
}
