package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func executeGo(ctx context.Context, root, dir, selection string) execution {
	log, err := os.OpenFile(filepath.Join(dir, "test.log"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // the test log, created exclusively in this run's own evidence folder
	if err != nil {
		return execution{-1, err}
	}
	//nolint:gosec // only the fixed go test of ./features; selection is the reviewed manifest's scenario selection
	cmd := exec.CommandContext(ctx, "go", "test", "./features", "-run", "^TestFeatures$", "-count=1", "-v", "-scenarios="+selection, "-scenario-report="+filepath.Join(dir, "cucumber.json"))
	cmd.Dir = root
	cmd.Stdout = log
	cmd.Stderr = log
	err = joinedCommand(cmd)
	closeErr := log.Close()
	code := -1
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		err = nil
	} // Nonzero exits are data; structured assertions decide validity.
	return execution{code, errors.Join(err, closeErr)}
}

// Called only on Linux/macOS at the executable boundary. Direct file streams
// avoid dangling output-copy goroutines. Run always waits, including after
// context cancellation kills the group; restoration is strictly after it returns.
func joinedCommand(cmd *exec.Cmd) error {
	cmd.SysProcAttr = groupAttributes()
	cmd.Cancel = func() error { return killCommandGroup(cmd.Process.Pid) }
	cmd.WaitDelay = 2 * time.Second
	return cmd.Run()
}
