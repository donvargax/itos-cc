package worktrees

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

const commandCleanupLimit = 5 * time.Second

// ownedOutcomePrecedence says the owned supervisor judges outcomes: an actual
// exit first, then the mutant's own timeout, then a parent's cancellation.
func ownedOutcomePrecedence(platform string) bool { return platform == "linux" || platform == "darwin" }

// Linux and macOS supervision owns ordinary descendants in the command's
// process group. Commands that daemonize or escape their process group or
// session are unsupported. Only the probe that a killed group has no live
// member differs by platform: ownedGroupGone, in proc_inspect.go, with the platform passed as a value.
// cleanupBudget is a run-abort-only deadline that callers may share across
// command scopes. Normal completion and mutant-local timeouts use a local
// cleanup budget and must not activate this shared run-abort deadline.
type cleanupBudget struct {
	mu              sync.Mutex
	deadline        time.Time
	localUntilAbort bool
	armed           bool
}

func cleanupBudgetForScope(ctx context.Context, shared *cleanupBudget) *cleanupBudget {
	if shared != nil && ctx.Err() != nil && !errors.Is(context.Cause(ctx), errMutationTimeout) {
		shared.mu.Lock()
		useShared := !shared.localUntilAbort || shared.armed
		shared.mu.Unlock()
		if useShared {
			return shared
		}
	}
	return &cleanupBudget{}
}

// begin starts the shared deadline now, if nothing started it yet: a
// fail-fast stop starts it when it is published, so every command it
// cancels is killed and joined within five seconds of the stop.
func (b *cleanupBudget) begin() {
	b.until()
	b.mu.Lock()
	b.armed = true
	b.mu.Unlock()
}

func (b *cleanupBudget) until() time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.deadline.IsZero() {
		b.deadline = time.Now().Add(commandCleanupLimit)
	}
	return b.deadline
}

// CleanupFailed is an owned command's process group whose cleanup did not
// finish by its deadline: a member still ran, or a process outside the group
// still held the command's output open. ProcessGroup is the group, the
// command's process ID, and Command its words, as a person reads them.
type CleanupFailed struct {
	ProcessGroup int
	Command      string
	Cause        error
}

func (c *CleanupFailed) Error() string {
	return fmt.Sprintf("cleanup of owned process group %d (%s) failed: %v", c.ProcessGroup, c.Command, c.Cause)
}
func (c *CleanupFailed) Unwrap() error { return c.Cause }

// holdOwned marks, through hold, owned process groups being killed and
// joined from when ctx is done or the command has ended until the release
// it returns is called, so a later interrupt waits for them (decision 38).
// A nil hold marks nothing.
type holdOwned struct {
	mu       sync.Mutex
	release  func()
	stop     func() bool
	hold     func() func()
	finished bool
}

func startHold(ctx context.Context, hold func() func()) *holdOwned {
	h := &holdOwned{hold: hold}
	if hold != nil {
		h.stop = context.AfterFunc(ctx, h.begin)
	}
	return h
}

// begin marks the cleanup under way, once.
func (h *holdOwned) begin() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.hold != nil && h.release == nil && !h.finished {
		h.release = h.hold()
	}
}

// end marks it done.
func (h *holdOwned) end() {
	if h.stop != nil {
		h.stop()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.finished = true
	if h.release != nil {
		h.release()
	}
}

func runOwnedCommand(ctx context.Context, cmd *exec.Cmd, output *bytes.Buffer, budget *cleanupBudget, platform string, observe func(time.Time), hold func() func()) error {
	if !ownedOutcomePrecedence(platform) {
		killGroup(cmd, platform)
		if err := cmd.Start(); err != nil {
			return err
		}
		return cmd.Wait()
	}
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
	killGroup(cmd, platform)
	cmd.WaitDelay = 0

	stopReader := &cleanupSignal{done: make(chan struct{})}
	for i := range captures {
		captures[i].done = make(chan outputResult, 1)
		stopReader.readers = append(stopReader.readers, captures[i].reader)
		go func(capture *ownedOutputCapture) {
			data, err := readOwnedCommandOutput(capture.reader)
			capture.done <- outputResult{data: data, err: err}
		}(&captures[i])
	}

	if err := cmd.Start(); err != nil {
		for _, capture := range captures {
			_ = capture.writer.Close()
		}
		stopReader.activate((&cleanupBudget{}).until())
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
	held := startHold(ctx, hold)
	defer held.end()
	waitErr := cmd.Wait()
	held.begin()

	deadline := cleanupBudgetForScope(ctx, budget).until()
	if observe != nil {
		observe(deadline)
	}
	cleanupErr := killOwnedProcessGroup(platform, cmd.Process.Pid)
	stopReader.activate(deadline)
	if err := waitOwnedProcessGroup(cmd.Process.Pid, deadline, platform); err != nil {
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
		return errors.Join(waitErr, &CleanupFailed{ProcessGroup: cmd.Process.Pid, Command: strings.Join(cmd.Args, " "), Cause: cleanupErr})
	}
	return waitErr
}

func waitOwnedProcessGroup(pgid int, deadline time.Time, platform string) error {
	for {
		gone, err := ownedGroupGone(platform, pgid)
		if err != nil {
			return fmt.Errorf("inspect owned process group %d: %w", pgid, err)
		}
		if gone {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("owned process group %d remained active at cleanup deadline", pgid)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

type outputResult struct {
	data []byte
	err  error
}

// cleanupSignal ends the reads of an owned command's output at the cleanup
// deadline, which a process that outlives the group by holding the output
// open would otherwise keep waiting for ever.
type cleanupSignal struct {
	done     chan struct{}
	deadline time.Time
	readers  []*os.File
}

func (signal *cleanupSignal) activate(deadline time.Time) {
	signal.deadline = deadline
	for _, r := range signal.readers {
		_ = r.SetReadDeadline(deadline) // a pipe the system cannot time out reads to its end, as before
	}
	close(signal.done)
}

type ownedOutputCapture struct {
	reader *os.File
	writer *os.File
	target io.Writer
	done   chan outputResult
}

func killOwnedProcessGroup(platform string, pgid int) error {
	err := nativeSignalGroup(-pgid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	if errors.Is(err, syscall.EPERM) && platform == "darwin" {
		// The probe that follows still waits for the group to be gone, so a
		// live member that refuses the signal fails cleanup at its deadline.
		return nil
	}
	if err != nil {
		return fmt.Errorf("terminate owned process group %d: %w", pgid, err)
	}
	return nil
}

// readOwnedCommandOutput reads reader to its end, or to the read deadline
// the cleanup sets, past which the output not closed is a cleanup failure.
func readOwnedCommandOutput(reader *os.File) ([]byte, error) {
	var output bytes.Buffer
	buffer := make([]byte, 32*1024)
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			_, _ = output.Write(buffer[:count])
		}
		switch {
		case err == nil:
		case errors.Is(err, io.EOF):
			return output.Bytes(), nil
		case errors.Is(err, os.ErrDeadlineExceeded):
			return output.Bytes(), errors.New("owned command output did not close before cleanup deadline")
		default:
			return output.Bytes(), err
		}
	}
}
