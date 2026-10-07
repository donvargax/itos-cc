package main

import (
	"bytes"
	"io"
	"os/exec"
	"runtime"
)

// shell runs a user-supplied command line through the platform shell.
func shell(command string, log io.Writer) error {
	cmd := shellCmd(command)
	cmd.Stdout = log
	cmd.Stderr = log
	return cmd.Run()
}

// shellOutput runs a user-supplied command line through the platform shell
// in dir and returns what it printed on stdout; its stderr goes to log.
func shellOutput(command, dir string, log io.Writer) (string, error) {
	cmd := shellCmd(command)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = log
	err := cmd.Run()
	return out.String(), err
}

func shellCmd(command string) *exec.Cmd {
	name, flag := "sh", "-c"
	if runtime.GOOS == "windows" {
		name, flag = "cmd", "/C"
	}
	return exec.Command(name, flag, command)
}
