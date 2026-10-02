package main

import (
	"io"
	"os/exec"
	"runtime"
)

// shell runs a user-supplied command line through the platform shell.
func shell(command string, log io.Writer) error {
	name, flag := "sh", "-c"
	if runtime.GOOS == "windows" {
		name, flag = "cmd", "/C"
	}
	cmd := exec.Command(name, flag, command)
	cmd.Stdout = log
	cmd.Stderr = log
	return cmd.Run()
}
