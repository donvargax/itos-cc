package features

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"sync"
)

// Running owns a product run. Output is only a synchronized snapshot of already
// captured bytes; Wait returns its final result after the process and pipes join.
type Running interface {
	Output() (stdout, stderr string)
	Wait(context.Context) (Result, error)
	Interrupt(context.Context) error
	// Terminate sends the run SIGTERM, as CI does to cancel a job; Windows
	// sends none.
	Terminate(context.Context) error
	// Kill ends a run that would not, by the system's own kill.
	Kill(context.Context) error
	Close(context.Context) error
}

var ErrPhysicalUnsupported = errors.New("physical fixture capability is unavailable")

// capturedOutput is a running product's output, written by exec's copying
// goroutine while a step reads it. Its buffer is a field, not embedded: an
// embedded bytes.Buffer would lend it ReadFrom, which io.Copy prefers to
// Write, so the copy would fill the buffer without the lock.
type capturedOutput struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *capturedOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}
func (b *capturedOutput) text() string { b.mu.Lock(); defer b.mu.Unlock(); return b.buf.String() }

type productRun struct {
	cmd            *exec.Cmd
	stdout, stderr capturedOutput
	done           chan struct{}
	err            error
}

func (b binary) Platform() string { return runtime.GOOS }
func (b binary) Start(ctx context.Context, r Run) (Running, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if b.Platform() == "windows" {
		return nil, fmt.Errorf("%w: asynchronous product signals require a pinned native handle", ErrPhysicalUnsupported)
	}
	if r.Binary != "" {
		return nil, errors.New("Start accepts only the product under test, not a Binary override")
	}
	cmd := exec.Command(b.bin, r.Args...) //nolint:gosec // the itos-cc binary under test, with the scenario's arguments
	cmd.Dir = r.Dir
	cmd.Env = environment(r.Env)
	if len(r.Without) > 0 {
		cmd.Env = setEnv(cmd.Env, "PATH", pathHiding(append(slices.Clone(hiddenAlways), r.Without...)...))
	}
	if r.Stdin != "" {
		cmd.Stdin = strings.NewReader(r.Stdin)
	}
	p := &productRun{cmd: cmd, done: make(chan struct{})}
	cmd.Stdout = &p.stdout
	cmd.Stderr = &p.stderr
	interruptible(cmd)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting product: %w", err)
	}
	go func() { p.err = cmd.Wait(); close(p.done) }()
	return p, nil
}

func (p *productRun) Output() (string, string) { return p.stdout.text(), p.stderr.text() }
func (p *productRun) Wait(ctx context.Context) (Result, error) {
	select {
	case <-ctx.Done():
		return Result{}, ctx.Err()
	case <-p.done:
	}
	out, stderr := p.Output()
	r := Result{Stdout: out, Stderr: stderr, Signal: signalOf(p.cmd.ProcessState)}
	var exit *exec.ExitError
	if errors.As(p.err, &exit) {
		r.Exit = exit.ExitCode()
		return r, nil
	}
	return r, p.err
}
func (p *productRun) Interrupt(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-p.done:
		return nil
	default:
		err := interrupt(p.cmd.Process)
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return err
	}
}
func (p *productRun) Terminate(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-p.done:
		return nil
	default:
		err := terminate(p.cmd.Process)
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return err
	}
}
func (p *productRun) Kill(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-p.done:
		return nil
	default:
		err := p.cmd.Process.Kill()
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return err
	}
}
func (p *productRun) Close(ctx context.Context) error {
	select {
	case <-p.done:
		_, err := p.Wait(ctx)
		return err
	default:
	}
	if err := p.Interrupt(ctx); err != nil {
		return err
	}
	_, err := p.Wait(ctx)
	return err
}
