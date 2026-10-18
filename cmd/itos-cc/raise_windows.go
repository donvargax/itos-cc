//go:build windows

package main

import (
	"errors"
	"os"
)

// raise raises nothing on Windows, where no process ends by a signal's
// default action a shell reads; the command exits 130 instead.
func raise(os.Signal) error { return errors.New("windows raises no signal") }
