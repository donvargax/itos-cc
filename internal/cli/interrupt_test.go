package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"testing"
	"time"
)

// These tests feed Interrupts.Stop interrupts of their own, os.Interrupt
// and SIGTERM on every system, so they show on Linux, macOS and Windows
// alike what each interrupt does. A scenario interrupting the built binary
// cannot show the second interrupt at any moment but one it can see, the
// owned processes' cleanup (ID-MUT-253): a command with no cleanup exits
// 130 within a moment of the first, and a second interrupt either finds it
// gone or ends it with the same 130 a prompt stop gives. No run can tell
// which ended it, so the second interrupt is held here.

// stopper is Stop running on fake interrupts: the signals its end was
// called with, and whether it returned. startStopper returns the context
// its cancel ends beside it, since a context is never held in a struct
// (decision 30).
type stopper struct {
	run        *Interrupts
	interrupts chan os.Signal
	ends       chan os.Signal
	returned   chan struct{}
}

func startStopper(t *testing.T) (context.Context, *stopper) {
	t.Helper()
	ctx, cancel := context.WithCancelCause(t.Context())
	t.Cleanup(func() { cancel(nil) })
	s := &stopper{
		run:        &Interrupts{},
		interrupts: make(chan os.Signal),
		ends:       make(chan os.Signal, 2),
		returned:   make(chan struct{}),
	}
	go func() {
		defer close(s.returned)
		s.run.Stop(s.interrupts, cancel, func(sig os.Signal) { s.ends <- sig })
	}()
	return ctx, s
}

// waitFor fails the test unless ch is ready within a while.
func waitFor[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(10 * time.Second):
		t.Fatalf("%s did not happen", what)
		var zero T
		return zero
	}
}

// noEnd fails the test if end was called.
func (s *stopper) noEnd(t *testing.T, when string) {
	t.Helper()
	select {
	case sig := <-s.ends:
		t.Fatalf("%s ended the command with %v, want no end", when, sig)
	default:
	}
}

// The first interrupt cancels the work, its cause the interrupt, so a
// command waiting on its context stops and reports the interrupt itself;
// it does not end the process, which would skip what the command does on
// its way out.
func TestTheFirstInterruptCancelsTheWorkAndEndsNothing(t *testing.T) {
	ctx, s := startStopper(t)
	s.interrupts <- syscall.SIGTERM
	waitFor(t, ctx.Done(), "the cancel")
	if cause := context.Cause(ctx); !errors.Is(cause, ErrInterrupted) {
		t.Errorf("cause %v, want ErrInterrupted", cause)
	}
	if got := s.run.Signal(); got != syscall.SIGTERM {
		t.Errorf("Signal() = %v, want the SIGTERM that came", got)
	}
	close(s.interrupts)
	waitFor(t, s.returned, "the return")
	s.noEnd(t, "one interrupt")
}

// A second interrupt ends the command at once, with the first interrupt,
// which cmd raises again: the person asked twice, and nothing owned is
// being killed.
func TestASecondInterruptEndsTheCommandAtOnce(t *testing.T) {
	ctx, s := startStopper(t)
	s.interrupts <- os.Interrupt
	s.interrupts <- syscall.SIGTERM
	if sig := waitFor(t, s.ends, "the end"); sig != os.Interrupt {
		t.Fatalf("a second interrupt ended with %v, want the first, %v", sig, os.Interrupt)
	}
	waitFor(t, s.returned, "the return")
	if ctx.Err() == nil {
		t.Error("the work was not cancelled before the end")
	}
}

// A second interrupt that comes while owned process groups are being
// killed waits for their cleanup and ends nothing, so the command ends on
// its own and reports, none of them outliving it (ADR-0021); a third, once
// they are done, ends it at once.
func TestASecondInterruptWaitsWhileOwnedGroupsAreKilled(t *testing.T) {
	_, s := startStopper(t)
	s.interrupts <- os.Interrupt
	release := s.run.Cleaning()
	other := s.run.Cleaning()
	sent := make(chan struct{})
	go func() { s.interrupts <- os.Interrupt; close(sent) }()
	waitFor(t, sent, "the second interrupt's delivery")
	time.Sleep(50 * time.Millisecond)
	s.noEnd(t, "a second interrupt during cleanup")
	release()
	release()
	time.Sleep(50 * time.Millisecond)
	s.noEnd(t, "a second interrupt while another group is still killed")
	other()
	third := make(chan struct{})
	go func() { s.interrupts <- os.Interrupt; close(third) }()
	waitFor(t, third, "the third interrupt's delivery, once the waiting second was spent")
	if sig := waitFor(t, s.ends, "the end"); sig != os.Interrupt {
		t.Fatalf("the third interrupt ended with %v, want %v", sig, os.Interrupt)
	}
}

// A cleanup that ends before the second interrupt comes holds nothing.
func TestACleanupThatEndedHoldsNoLaterInterrupt(t *testing.T) {
	_, s := startStopper(t)
	s.interrupts <- os.Interrupt
	s.run.Cleaning()()
	s.interrupts <- os.Interrupt
	waitFor(t, s.ends, "the end")
}

// With no interrupt at all, nothing is cancelled and nothing ends: a
// command that finishes on its own is left to.
func TestNoInterruptCancelsNothing(t *testing.T) {
	ctx, s := startStopper(t)
	close(s.interrupts)
	waitFor(t, s.returned, "the return")
	if ctx.Err() != nil {
		t.Error("the work was cancelled with no interrupt")
	}
	if s.run.Signal() != nil {
		t.Errorf("Signal() = %v with no interrupt", s.run.Signal())
	}
	s.noEnd(t, "no interrupt")
}

// A failure whose cause is the interrupt, which no classifier claims, is
// the interrupted problem, exit 130; any other cancellation is internal.
func TestAnInterruptsCauseIsTheInterruptedProblem(t *testing.T) {
	var e ends
	ui := e.ui()
	if code := ExitCode(ui.Problems(fmt.Errorf("walking: %w", ErrInterrupted))); code != CodeInterrupted {
		t.Errorf("exit %d for the interrupt's cause, want %d", code, CodeInterrupted)
	}
	if p := ui.Problems(ErrInterrupted); len(p) != 1 || p[0].Rule != "interrupted" {
		t.Errorf("problems %v, want interrupted", p)
	}
	if code := ExitCode(ui.Problems(context.Canceled)); code != CodeInternal {
		t.Errorf("exit %d for another cancellation, want %d", code, CodeInternal)
	}
}
