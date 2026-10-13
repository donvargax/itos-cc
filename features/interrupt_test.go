// The run a scenario interrupts: itos-cc started with its standard
// input left open, so nothing but the interrupt can end its wait, and the
// interrupt sent only once the binary is reading that input. How a process
// is interrupted differs by system, in the build-tagged interrupt_unix_test.go
// and interrupt_windows_test.go; what is judged of it is here. Only the
// binary driver can send a process an interrupt, so these steps are its own,
// given to the scenario's world beside the steps every driver takes.
package features

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// readiness is how much the step writes to the binary's standard input
// before interrupting it: more than any system's pipe holds unread (64 KiB
// on Linux and macOS, 4 KiB on Windows), so the write returns only once the
// binary has read most of it. The binary installs its interrupt handling
// before it parses its command line, and reads its input after, so a write
// that returned proves the handling is in place and the binary is waiting
// on the rest of its input: no sleep guesses when that is.
const readiness = 1 << 20

// startTimeout is how long the binary has to start and read what the step
// writes; stopTimeout how long it has to stop once interrupted, the
// "promptly" a scenario holds it to. Either passing fails the step.
const (
	startTimeout = 30 * time.Second
	stopTimeout  = 10 * time.Second
)

// running is a run of the binary still under way: its standard input, held
// open, and what it has written.
type running struct {
	cmd            *exec.Cmd
	input          io.WriteCloser
	stdout, stderr strings.Builder
	done           chan error // Wait's result, once the process ended
	exited         bool       // done has been read: cmd.ProcessState is the process's end
}

// interrupter is a scenario's run of the binary at bin that waits on its
// input, nil for none, and the world whose output it leaves.
type interrupter struct {
	w       *world
	bin     string
	running *running
}

// interruptSteps are the steps of a run that waits on its input until
// something ends it, and the interrupt that does, for the binary at bin.
// The run, still under way after the scenario, is stopped then, before its
// folder is removed.
func interruptSteps(bin string) func(*godog.ScenarioContext, *world) {
	return func(sc *godog.ScenarioContext, w *world) {
		i := &interrupter{w: w, bin: bin}
		sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
			i.running = nil
			return ctx, nil
		})
		sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
			i.running.stop()
			return ctx, nil
		})
		sc.Step(`^itos-cc runs with "([^"]*)" and its standard input left open$`, i.startWithInputOpen)
		sc.Step(`^it is interrupted while it waits$`, i.interruptWhileWaiting)
	}
}

// startWithInputOpen starts itos-cc in the scratch folder with
// args, its standard input a pipe the step holds open, and returns without
// waiting for it.
func (i *interrupter) startWithInputOpen(args string) error {
	fields, err := commandWords(args)
	if err != nil {
		return err
	}
	w := i.w
	r := &running{cmd: exec.Command(i.bin, fields...), done: make(chan error, 1)}
	r.cmd.Dir = w.dir
	r.cmd.Env = environment(w.env())
	r.cmd.Stdout = &r.stdout
	r.cmd.Stderr = &r.stderr
	interruptible(r.cmd)
	if r.input, err = r.cmd.StdinPipe(); err != nil {
		return err
	}
	if err := r.cmd.Start(); err != nil {
		return fmt.Errorf("starting %s: %w", i.bin, err)
	}
	go func() { r.done <- r.cmd.Wait() }()
	i.running = r
	return nil
}

// interruptWhileWaiting interrupts the run once it reads its input, and
// waits for it to stop, its exit code and output what the Then steps read.
// The step fails where the binary never reads its input, where it does not
// stop promptly, and where the interrupt killed it by the system's default
// action rather than reaching a handler of its own: that run exited by the
// signal, not by anything the binary decided.
func (i *interrupter) interruptWhileWaiting() error {
	r := i.running
	if r == nil {
		return errors.New("no run waits on its standard input: start one with its standard input left open")
	}
	written := make(chan error, 1)
	go func() {
		_, err := r.input.Write(make([]byte, readiness))
		written <- err
	}()
	select {
	case err := <-written:
		if err != nil {
			r.stop()
			return i.ended(fmt.Errorf("itos-cc stopped before it read its standard input: %w", err))
		}
	case <-time.After(startTimeout):
		r.stop()
		return i.ended(fmt.Errorf("itos-cc did not read its standard input within %s", startTimeout))
	}
	if err := interrupt(r.cmd.Process); err != nil {
		r.stop()
		return fmt.Errorf("interrupting itos-cc: %w", err)
	}
	select {
	case <-r.done:
		r.exited = true
	case <-time.After(stopTimeout):
		r.stop()
		return i.ended(fmt.Errorf("itos-cc did not stop within %s of the interrupt", stopTimeout))
	}
	if killedBySignal(r.cmd.ProcessState) {
		return i.ended(fmt.Errorf("the interrupt killed itos-cc by the system's default action (%s): it never handled it", r.cmd.ProcessState))
	}
	return i.ended(nil)
}

// ended records what the stopped run wrote and its exit code for the Then
// steps, and returns err with them, so a failing step shows the run.
func (i *interrupter) ended(err error) error {
	w, r := i.w, i.running
	i.running = nil
	w.stdout, w.stderr = r.stdout.String(), r.stderr.String()
	w.exit = -1
	if r.exited {
		w.exit = r.cmd.ProcessState.ExitCode()
	}
	if err != nil {
		return fmt.Errorf("%w\n%s", err, w.report())
	}
	return nil
}

// stop ends the run if it is still under way, by killing it, and waits for
// it, so no scenario leaves a process behind and its end is known; on a nil
// run, nothing.
func (r *running) stop() {
	if r == nil || r.exited {
		return
	}
	_ = r.cmd.Process.Kill()
	<-r.done
	r.exited = true
}
