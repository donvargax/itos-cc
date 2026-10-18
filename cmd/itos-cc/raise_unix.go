//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// raise sends this process sig again with its default action, which ends
// it as sig would have, the one call Windows lacks (itos-template's
// decision 26).
func raise(sig os.Signal) error {
	s, ok := sig.(syscall.Signal)
	if !ok {
		return fmt.Errorf("%v is no system signal", sig)
	}
	signal.Reset(s)
	return syscall.Kill(os.Getpid(), s)
}
