package cli

import (
	"context"
	"os"
	"testing"
	"time"
)

// These tests feed StopOnInterrupt interrupts of their own, os.Interrupt
// on every system, so they show on Linux, macOS and Windows alike what each
// interrupt does. A scenario interrupting the built binary cannot show the
// second interrupt: a command with no cleanup exits 130 within a moment of
// the first, and a second interrupt either finds it gone or ends it with the
// same 130 a prompt stop gives. No run can tell which ended it, so the
// double interrupt is held here.

// stopper is StopOnInterrupt running on fake interrupts: the codes its exit
// was called with, and whether it returned. startStopper returns the context
// its cancel ends beside it, since a context is never held in a struct
// (decision 30).
type stopper struct {
	interrupts chan os.Signal
	exits      chan int
	returned   chan struct{}
}

func startStopper(t *testing.T) (context.Context, *stopper) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	s := &stopper{
		interrupts: make(chan os.Signal),
		exits:      make(chan int, 2),
		returned:   make(chan struct{}),
	}
	go func() {
		defer close(s.returned)
		StopOnInterrupt(s.interrupts, cancel, func(code int) { s.exits <- code })
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

// The first interrupt cancels the work, so a command waiting on its context
// stops and reports the interrupt itself; it does not end the process,
// which would skip what the command does on its way out.
func TestTheFirstInterruptCancelsTheWorkAndExitsNothing(t *testing.T) {
	ctx, s := startStopper(t)
	s.interrupts <- os.Interrupt
	waitFor(t, ctx.Done(), "the cancel")
	close(s.interrupts)
	waitFor(t, s.returned, "the return")
	select {
	case code := <-s.exits:
		t.Fatalf("one interrupt exited %d, want no exit", code)
	default:
	}
}

// A second interrupt ends the process at once with 130: the person asked
// twice, and the cleanup the first left running is skipped.
func TestASecondInterruptExits130AtOnce(t *testing.T) {
	ctx, s := startStopper(t)
	s.interrupts <- os.Interrupt
	s.interrupts <- os.Interrupt
	if code := waitFor(t, s.exits, "the exit"); code != 130 {
		t.Fatalf("a second interrupt exited %d, want 130", code)
	}
	waitFor(t, s.returned, "the return")
	if ctx.Err() == nil {
		t.Error("the work was not cancelled before the exit")
	}
}

// With no interrupt at all, nothing is cancelled and nothing exits: a
// command that finishes on its own is left to.
func TestNoInterruptCancelsNothing(t *testing.T) {
	ctx, s := startStopper(t)
	close(s.interrupts)
	waitFor(t, s.returned, "the return")
	if ctx.Err() != nil {
		t.Error("the work was cancelled with no interrupt")
	}
	select {
	case code := <-s.exits:
		t.Fatalf("no interrupt exited %d, want no exit", code)
	default:
	}
}
