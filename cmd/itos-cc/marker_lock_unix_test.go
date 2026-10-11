//go:build !windows

package main

import (
	"os"
	"syscall"
)

// lockMarker takes an exclusive flock on f, as a mutation run takes it on
// its worker directory's marker.
func lockMarker(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}
