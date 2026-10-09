//go:build linux

package mutate

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	ownershipModeEnv    = "ITOS_OWNERSHIP_MODE"
	ownershipRoleEnv    = "ITOS_OWNERSHIP_ROLE"
	ownershipChildEnv   = "ITOS_OWNERSHIP_CHILD_PID"
	ownershipGrandEnv   = "ITOS_OWNERSHIP_GRANDCHILD_PID"
	ownershipWorkEnv    = "ITOS_OWNERSHIP_LATE_WORK"
	ownershipReleaseEnv = "ITOS_OWNERSHIP_RELEASE"
)

func TestLinuxOwnTimeoutStopsOwnedProcessTree(t *testing.T) {
	root := t.TempDir()
	childPID := filepath.Join(root, "child.pid")
	grandchildPID := filepath.Join(root, "grandchild.pid")
	t.Setenv(ownershipModeEnv, "timeout")
	t.Setenv(ownershipRoleEnv, "parent")
	t.Setenv(ownershipChildEnv, childPID)
	t.Setenv(ownershipGrandEnv, grandchildPID)
	w := newCommandRunnerWorker(t)
	cleanupOwnershipFixture(t, childPID, grandchildPID)
	cmd := Command{Root: root, Dir: root, Args: ownershipHelperArgs()}

	got, err := w.run(cmd, 300*time.Millisecond)
	if err != nil || !got.timedOut || got.passed {
		t.Fatalf("own timeout result = %+v, err = %v; want timed out", got, err)
	}
	child := readPID(t, childPID)
	grandchild := readPID(t, grandchildPID)
	if processCanRun(child) || processCanRun(grandchild) {
		t.Fatalf("owned process tree remains after timeout: child=%d grandchild=%d", child, grandchild)
	}
}

func TestLinuxNormalReturnStopsOwnedDescendant(t *testing.T) {
	root := t.TempDir()
	childPID := filepath.Join(root, "child.pid")
	grandchildPID := filepath.Join(root, "grandchild.pid")
	lateWork := filepath.Join(root, "late-work")
	release := filepath.Join(root, "release")
	t.Setenv(ownershipModeEnv, "normal")
	t.Setenv(ownershipRoleEnv, "parent")
	t.Setenv(ownershipChildEnv, childPID)
	t.Setenv(ownershipGrandEnv, grandchildPID)
	t.Setenv(ownershipWorkEnv, lateWork)
	t.Setenv(ownershipReleaseEnv, release)
	w := newCommandRunnerWorker(t)
	cleanupOwnershipFixture(t, childPID, grandchildPID)
	cmd := Command{Root: root, Dir: root, Args: ownershipHelperArgs()}

	got, err := w.run(cmd, 3*time.Second)
	if err != nil || !got.passed || got.exitCode != 0 {
		t.Fatalf("normal command result = %+v, err = %v; want successful parent exit", got, err)
	}
	child := readPID(t, childPID)
	grandchild := readPID(t, grandchildPID)
	if processCanRun(child) || processCanRun(grandchild) {
		t.Fatalf("owned descendant remains after normal return: child=%d grandchild=%d", child, grandchild)
	}
	if err := os.WriteFile(release, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lateWork); err == nil {
		t.Fatal("owned descendant continued fixture work after command return")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestLinuxOwnershipStartupFailureDoesNotRunCommand(t *testing.T) {
	root := t.TempDir()
	started := filepath.Join(root, "started")
	w := newCommandRunnerWorker(t)
	startErr := fmt.Errorf("injected ownership setup failure")
	w.runner.lifecycle.start = func(_ context.Context, _ *exec.Cmd) error {
		return startErr
	}

	_, err := w.run(Command{Root: root, Dir: root, Args: []string{"/bin/sh", "-c", "touch " + started}}, time.Second)
	if err != startErr {
		t.Fatalf("startup error = %v, want injected ownership error", err)
	}
	if _, err := os.Stat(started); !os.IsNotExist(err) {
		t.Fatalf("command ran after ownership setup failed (stat error %v)", err)
	}
}

// TestLinuxOwnershipFixtureHelper only runs as an owned child or grandchild.
func TestLinuxOwnershipFixtureHelper(t *testing.T) {
	switch os.Getenv(ownershipRoleEnv) {
	case "child":
		grandchild := exec.Command(os.Args[0], "-test.run=^TestLinuxOwnershipFixtureHelper$")
		grandchild.Env = append(os.Environ(), ownershipRoleEnv+"=grandchild")
		grandchild.Stdout, grandchild.Stderr = os.Stdout, os.Stderr
		if err := grandchild.Start(); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(os.Getenv(ownershipGrandEnv), []byte(strconv.Itoa(grandchild.Process.Pid)), 0o600); err != nil {
			t.Fatal(err)
		}
		_ = grandchild.Process.Release()
		for {
			time.Sleep(10 * time.Millisecond)
		}
	case "grandchild":
		if os.Getenv(ownershipModeEnv) == "normal" {
			for {
				if _, err := os.Stat(os.Getenv(ownershipReleaseEnv)); err == nil {
					_ = os.WriteFile(os.Getenv(ownershipWorkEnv), []byte("continued"), 0o600)
					return
				}
				time.Sleep(5 * time.Millisecond)
			}
		}
		for {
			time.Sleep(10 * time.Millisecond)
		}
	case "parent":
		child := exec.Command(os.Args[0], "-test.run=^TestLinuxOwnershipFixtureHelper$")
		child.Env = append(os.Environ(), ownershipRoleEnv+"=child")
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(os.Getenv(ownershipChildEnv), []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
			t.Fatal(err)
		}
		_ = child.Process.Release()
		grandchildDeadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(grandchildDeadline) {
			if _, err := os.Stat(os.Getenv(ownershipGrandEnv)); err == nil {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		if _, err := os.Stat(os.Getenv(ownershipGrandEnv)); err != nil {
			t.Fatalf("grandchild did not start: %v", err)
		}
		if os.Getenv(ownershipModeEnv) == "normal" {
			return
		}
		for {
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func ownershipHelperArgs() []string {
	return []string{os.Args[0], "-test.run=^TestLinuxOwnershipFixtureHelper$"}
}

func readPID(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture pid %s: %v", path, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("parse fixture pid %s: %v", path, err)
	}
	return pid
}

func processCanRun(pid int) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false
	}
	fields := strings.Fields(string(data))
	return len(fields) < 3 || fields[2] != "Z" && fields[2] != "X"
}

func cleanupOwnershipFixture(t *testing.T, childPath, grandchildPath string) {
	t.Helper()
	t.Cleanup(func() {
		for _, path := range []string{childPath, grandchildPath} {
			if data, err := os.ReadFile(path); err == nil {
				if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
					_ = syscall.Kill(pid, syscall.SIGKILL)
				}
			}
		}
	})
}
