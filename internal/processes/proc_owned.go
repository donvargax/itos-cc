package processes

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
// CleanupBudget is a run-abort-only deadline that callers may share across
// command scopes. Normal completion and mutant-local timeouts use a local
// cleanup budget and must not activate this shared run-abort deadline.
type CleanupBudget struct {
	mu              sync.Mutex
	deadline        time.Time
	localUntilAbort bool
	armed           bool
}

// NewCleanupBudget is a shared run-abort budget. With localUntilAbort, a
// cancelled command's cleanup keeps a local budget of its own until Begin
// starts the shared deadline.
func NewCleanupBudget(localUntilAbort bool) *CleanupBudget {
	return &CleanupBudget{localUntilAbort: localUntilAbort}
}

func cleanupBudgetForScope(ctx context.Context, shared *CleanupBudget) *CleanupBudget {
	if shared != nil && ctx.Err() != nil && !errors.Is(context.Cause(ctx), ErrCommandTimeout) {
		shared.mu.Lock()
		useShared := !shared.localUntilAbort || shared.armed
		shared.mu.Unlock()
		if useShared {
			return shared
		}
	}
	return &CleanupBudget{}
}

// Begin starts the shared deadline now, if nothing started it yet: a
// fail-fast stop starts it when it is published, so every command it
// cancels is killed and joined within five seconds of the stop.
func (b *CleanupBudget) Begin() {
	b.until()
	b.mu.Lock()
	b.armed = true
	b.mu.Unlock()
}

// Deadline is the shared deadline once a cleanup or Begin started it, and
// the zero time before.
func (b *CleanupBudget) Deadline() time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.deadline
}

func (b *CleanupBudget) until() time.Time {
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

// runOwnedCommand runs cmd as s holds it: under its output cap
// everywhere, and, where the platform owns process groups, in a group it
// owns, killed and joined when the command ends, its memory sampled where
// the platform measures it, under its memory cap.
func runOwnedCommand(ctx context.Context, cmd *exec.Cmd, s *supervision, budget *CleanupBudget, observe func(time.Time), hold func() func()) error {
	platform := s.platform
	if !ownedOutcomePrecedence(platform) {
		return runUnownedCommand(cmd, s)
	}
	if budget == nil {
		budget = &CleanupBudget{}
	}
	originalStdout, originalStderr := cmd.Stdout, cmd.Stderr
	defer func() { cmd.Stdout, cmd.Stderr = originalStdout, originalStderr }()
	sharedOutput := cmd.Stdout == io.Writer(s.kept) && cmd.Stderr == io.Writer(s.kept)
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
		if err := setup(s.kept, func(pipe *os.File) { cmd.Stdout, cmd.Stderr = pipe, pipe }); err != nil {
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
		captures[i].done = make(chan error, 1)
		stopReader.readers = append(stopReader.readers, captures[i].reader)
		// The kept output takes the command's bytes as they come, bounded;
		// a caller's own writer gets them once the group is joined, as
		// before, each counted against the cap as it is read.
		var into io.Writer = s.kept
		if captures[i].target != io.Writer(s.kept) {
			into = &countedWriter{to: &captures[i].buffer, kept: s.kept}
		}
		go func(capture *ownedOutputCapture, into io.Writer) {
			capture.done <- readOwnedCommandOutput(capture.reader, into)
		}(&captures[i], into)
	}

	if err := cmd.Start(); err != nil {
		for _, capture := range captures {
			_ = capture.writer.Close()
		}
		stopReader.activate((&CleanupBudget{}).until())
		for _, capture := range captures {
			readErr := <-capture.done
			_ = capture.reader.Close()
			if readErr != nil {
				err = errors.Join(err, fmt.Errorf("join command output reader: %w", readErr))
			}
		}
		return err
	}
	for _, capture := range captures {
		_ = capture.writer.Close()
	}
	pgid := cmd.Process.Pid
	// Over a cap the group is killed here, then joined below as at the
	// command's own timeout, within a local budget: its context is not
	// done, so the shared one is never taken.
	overCap := func(name string) func() {
		return func() {
			if s.over.hit(name) {
				_ = killOwnedProcessGroup(platform, pgid)
			}
		}
	}
	s.kept.arm(overCap(OverOutput))
	var memory *memoryWatch
	if s.measured = MemoryMeasured(platform); s.measured {
		memory = watchMemory(pgid, s.limits.Memory, overCap(OverMemory))
	}
	held := startHold(ctx, hold)
	defer held.end()
	waitErr := cmd.Wait()
	s.over.end()
	if memory != nil {
		s.peak = memory.stopped()
	}
	held.begin()

	deadline := cleanupBudgetForScope(ctx, budget).until()
	if observe != nil {
		observe(deadline)
	}
	cleanupErr := killOwnedProcessGroup(platform, pgid)
	stopReader.activate(deadline)
	if err := waitOwnedProcessGroup(pgid, deadline, platform); err != nil {
		cleanupErr = errors.Join(cleanupErr, err)
	}
	for i := range captures {
		capture := &captures[i]
		readErr := <-capture.done
		_ = capture.reader.Close()
		if capture.target != io.Writer(s.kept) {
			if _, err := io.Copy(capture.target, &capture.buffer); err != nil {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("preserve command output: %w", err))
			}
		}
		if readErr != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("join command output reader: %w", readErr))
		}
	}
	if cleanupErr != nil {
		return errors.Join(waitErr, &CleanupFailed{ProcessGroup: pgid, Command: strings.Join(cmd.Args, " "), Cause: cleanupErr})
	}
	return waitErr
}

// runUnownedCommand runs cmd where the platform owns no process group, as
// on Windows: over its output cap, the command's own process is killed.
// What it writes to a caller's own writer is counted against the cap as
// it passes. The command's streams are given back as they were.
func runUnownedCommand(cmd *exec.Cmd, s *supervision) error {
	originalStdout, originalStderr := cmd.Stdout, cmd.Stderr
	defer func() { cmd.Stdout, cmd.Stderr = originalStdout, originalStderr }()
	counted := func(w io.Writer) io.Writer {
		if _, file := w.(*os.File); w == nil || file || w == io.Writer(s.kept) {
			return w
		}
		return &countedWriter{to: w, kept: s.kept}
	}
	if cmd.Stdout == cmd.Stderr {
		cmd.Stdout = counted(cmd.Stdout)
		cmd.Stderr = cmd.Stdout
	} else {
		cmd.Stdout, cmd.Stderr = counted(cmd.Stdout), counted(cmd.Stderr)
	}
	killGroup(cmd, s.platform)
	if err := cmd.Start(); err != nil {
		return err
	}
	s.kept.arm(func() {
		if s.over.hit(OverOutput) {
			_ = cmd.Process.Kill()
		}
	})
	err := cmd.Wait()
	s.over.end()
	return err
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

// ownedOutputCapture is one pipe of a command's output: read into the
// kept output, or into buffer for a caller's own target.
type ownedOutputCapture struct {
	reader *os.File
	writer *os.File
	target io.Writer
	buffer bytes.Buffer
	done   chan error
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

// readOwnedCommandOutput reads reader into into, to its end, or to the
// read deadline the cleanup sets, past which the output not closed is a
// cleanup failure.
func readOwnedCommandOutput(reader *os.File, into io.Writer) error {
	buffer := make([]byte, 32*1024)
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			_, _ = into.Write(buffer[:count])
		}
		switch {
		case err == nil:
		case errors.Is(err, io.EOF):
			return nil
		case errors.Is(err, os.ErrDeadlineExceeded):
			return errors.New("owned command output did not close before cleanup deadline")
		default:
			return err
		}
	}
}
