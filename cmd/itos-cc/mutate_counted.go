package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

// countedPlatform is the platform counted mode and --fail-fast check: both
// run on Linux and macOS, which own every process tree a trial starts, so
// an interruption or a stop can end it (Windows is #29). The mutation slice
// is given it as a value (adapters' Platform).
var countedPlatform = runtime.GOOS

// errCountedInterrupted is the cause of a counted run's context once
// SIGINT or SIGTERM aborts it.
var errCountedInterrupted = errors.New("interrupted")

// countedSignalContext is the one context of a counted run, derived from
// parent: SIGINT or SIGTERM cancels it, with errCountedInterrupted as its
// cause. Every later signal is caught too, until stop, so a second one
// cannot end the process before its owned commands are joined and its
// private inputs removed.
func countedSignalContext(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(parent)
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case sig := <-signals:
				name := "SIGTERM"
				if sig == os.Interrupt {
					name = "SIGINT"
				}
				cancel(fmt.Errorf("%w by %s", errCountedInterrupted, name))
			case <-done:
				return
			}
		}
	}()
	return ctx, func() {
		signal.Stop(signals)
		close(done)
		cancel(nil)
	}
}

// countedRoot is the top of the Git repository holding the working
// directory, its physical path, or why git could not tell it, which the
// slice words as count.no-git.
func countedRoot(context.Context) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(strings.TrimSpace(string(out)))
}

// physicalDir is the working directory, its physical path when its links
// can be followed.
func physicalDir(context.Context) (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if physical, err := filepath.EvalSymlinks(wd); err == nil {
		wd = physical
	}
	return wd, nil
}
