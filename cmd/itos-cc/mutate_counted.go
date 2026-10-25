package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/donvargax/itos-cc/internal/project"
)

// countedPlatform is the platform counted mode and --fail-fast check: both
// run on Linux and macOS, which own every process tree a trial starts, so
// an interruption or a stop can end it (Windows is #29). The mutation slice
// is given it as a value (adapters' Platform).
var countedPlatform = runtime.GOOS

// hostPlatform is the system this process runs on, which says how an
// interrupted command ends (endInterrupted).
var hostPlatform = runtime.GOOS

// countedRoot is the top of the Git repository holding the working
// directory, its physical path, or why git could not tell it, which the
// slice words as count.no-git.
func countedRoot(context.Context) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Env = project.InheritedEnv()
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
