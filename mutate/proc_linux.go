//go:build linux

package mutate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const commandCleanupLimit = 5 * time.Second

func linuxOutcomePrecedence() bool { return true }

// Linux supervision owns ordinary descendants in the command's process group.
// Commands that daemonize or escape their process group/session are unsupported.
// cleanupBudget can be shared by command scopes so one abort has one cleanup
// deadline, rather than granting every worker a fresh interval.
type cleanupBudget struct {
	mu       sync.Mutex
	deadline time.Time
}

func (b *cleanupBudget) until() time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.deadline.IsZero() {
		b.deadline = time.Now().Add(commandCleanupLimit)
	}
	return b.deadline
}

func runOwnedCommand(ctx context.Context, cmd *exec.Cmd, output *bytes.Buffer, budget *cleanupBudget) error {
	return runLinuxOwnedCommand(ctx, cmd, output, budget)
}

func runLinuxOwnedCommand(ctx context.Context, cmd *exec.Cmd, output *bytes.Buffer, budget *cleanupBudget) error {
	if budget == nil {
		budget = &cleanupBudget{}
	}
	originalStdout, originalStderr := cmd.Stdout, cmd.Stderr
	defer func() { cmd.Stdout, cmd.Stderr = originalStdout, originalStderr }()
	sharedOutput := cmd.Stdout == output && cmd.Stderr == output
	captures := make([]ownedOutputCapture, 0, 2)
	setup := func(target io.Writer, assign func(*os.File)) error {
		if target == nil {
			return nil
		}
		if _, ok := target.(*os.File); ok {
			return nil
		}
		reader, writer, err := os.Pipe()
		if err != nil {
			return fmt.Errorf("create owned command output pipe: %w", err)
		}
		if err := syscall.SetNonblock(int(reader.Fd()), true); err != nil {
			_ = reader.Close()
			_ = writer.Close()
			return fmt.Errorf("configure owned command output pipe: %w", err)
		}
		assign(writer)
		captures = append(captures, ownedOutputCapture{reader: reader, writer: writer, target: target})
		return nil
	}
	if sharedOutput {
		if err := setup(output, func(pipe *os.File) { cmd.Stdout, cmd.Stderr = pipe, pipe }); err != nil {
			return err
		}
	} else {
		stdout, stderr := cmd.Stdout, cmd.Stderr
		if err := setup(stdout, func(pipe *os.File) { cmd.Stdout = pipe }); err != nil {
			return err
		}
		if err := setup(stderr, func(pipe *os.File) { cmd.Stderr = pipe }); err != nil {
			for _, capture := range captures {
				_ = capture.reader.Close()
				_ = capture.writer.Close()
			}
			return err
		}
	}
	killGroup(cmd)
	cmd.WaitDelay = 0

	stopReader := make(chan struct{})
	for i := range captures {
		captures[i].done = make(chan outputResult, 1)
		go func(capture *ownedOutputCapture) {
			data, err := readOwnedCommandOutput(capture.reader, stopReader, budget)
			capture.done <- outputResult{data: data, err: err}
		}(&captures[i])
	}

	if err := cmd.Start(); err != nil {
		for _, capture := range captures {
			_ = capture.writer.Close()
		}
		close(stopReader)
		for _, capture := range captures {
			read := <-capture.done
			_ = capture.reader.Close()
			if read.err != nil {
				err = errors.Join(err, fmt.Errorf("join command output reader: %w", read.err))
			}
		}
		return err
	}
	for _, capture := range captures {
		_ = capture.writer.Close()
	}
	waitErr := cmd.Wait()

	cleanupErr := killOwnedProcessGroup(cmd.Process.Pid)
	close(stopReader)
	deadline := budget.until()
	if err := waitOwnedProcessGroup(cmd.Process.Pid, deadline); err != nil {
		cleanupErr = errors.Join(cleanupErr, err)
	}
	for _, capture := range captures {
		read := <-capture.done
		_ = capture.reader.Close()
		if _, err := io.Copy(capture.target, bytes.NewReader(read.data)); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("preserve command output: %w", err))
		}
		if read.err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("join command output reader: %w", read.err))
		}
	}
	if cleanupErr != nil {
		return errors.Join(waitErr, cleanupErr)
	}
	return waitErr
}

func waitOwnedProcessGroup(pgid int, deadline time.Time) error {
	for {
		err := syscall.Kill(-pgid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if err != nil && !errors.Is(err, syscall.EPERM) {
			return fmt.Errorf("inspect owned process group %d: %w", pgid, err)
		}
		running, err := ownedGroupHasRunnableProcess(pgid)
		if err != nil {
			return fmt.Errorf("inspect owned process group %d: %w", pgid, err)
		}
		if !running {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("owned process group %d remained active at cleanup deadline", pgid)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func ownedGroupHasRunnableProcess(pgid int) (bool, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ESRCH) {
				continue
			}
			return false, err
		}
		closeParen := strings.LastIndexByte(string(data), ')')
		if closeParen < 0 {
			return false, fmt.Errorf("malformed process stat for pid %s", entry.Name())
		}
		fields := strings.Fields(string(data[closeParen+1:]))
		if len(fields) < 3 {
			return false, fmt.Errorf("short process stat for pid %s", entry.Name())
		}
		group, err := strconv.Atoi(fields[2])
		if err != nil {
			return false, fmt.Errorf("parse process group for pid %s: %w", entry.Name(), err)
		}
		if group == pgid && fields[0] != "Z" && fields[0] != "X" {
			return true, nil
		}
	}
	return false, nil
}

type outputResult struct {
	data []byte
	err  error
}

type ownedOutputCapture struct {
	reader *os.File
	writer *os.File
	target io.Writer
	done   chan outputResult
}

func killOwnedProcessGroup(pgid int) error {
	err := syscall.Kill(-pgid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("terminate owned process group %d: %w", pgid, err)
	}
	return nil
}

func readOwnedCommandOutput(reader *os.File, stop <-chan struct{}, budget *cleanupBudget) ([]byte, error) {
	var output bytes.Buffer
	var cleanupDeadline time.Time
	buffer := make([]byte, 32*1024)
	for {
		count, err := syscall.Read(int(reader.Fd()), buffer)
		if count > 0 {
			_, _ = output.Write(buffer[:count])
		}
		if err == nil && count == 0 {
			return output.Bytes(), nil
		}
		if err != nil && !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			return output.Bytes(), err
		}
		if cleanupDeadline.IsZero() {
			select {
			case <-stop:
				cleanupDeadline = budget.until()
			default:
			}
		}
		if !cleanupDeadline.IsZero() && !time.Now().Before(cleanupDeadline) {
			return output.Bytes(), fmt.Errorf("owned command output did not close before cleanup deadline")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
