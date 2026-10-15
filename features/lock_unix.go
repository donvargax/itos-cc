//go:build !windows

package features

import (
	"os"

	"golang.org/x/sys/unix"
)

const fixtureLockFlags = unix.LOCK_EX | unix.LOCK_NB

func lockFixture(f *os.File) error { return unix.Flock(int(f.Fd()), fixtureLockFlags) }
