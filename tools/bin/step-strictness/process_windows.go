//go:build windows

package main

import (
	"errors"
	"syscall"
)

// Execution is refused at the CLI boundary, before creating recovery data or
// editing source. These leaves keep validation and recovery tests portable.
func groupAttributes() *syscall.SysProcAttr { return nil }
func killCommandGroup(_ int) error          { return errors.New("group cancellation unavailable") }
