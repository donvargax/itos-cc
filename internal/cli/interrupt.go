package cli

import (
	"context"
	"errors"
	"os"
	"sync"
)

// ErrInterrupted is the cause a command's context is cancelled with when an
// interrupt stops it (decision 38): SIGINT, Ctrl-C, or SIGTERM. A
// classifier maps a cancellation to the interrupted problem only when
// errors.Is reports this cause, so a fail-fast stop, a timeout or a
// parent's cancellation keeps its own classification. It names no domain.
var ErrInterrupted = errors.New("interrupted")

// Interrupts is how one run of itos-cc stops on an interrupt (decision 38):
// the first cancels the command's work with ErrInterrupted, so whatever
// waits on its context ends and the command reports the interrupt itself,
// exit CodeInterrupted, saying so once; a later one ends the command at
// once, unless the process groups the run owns are being killed then
// (ADR-0021), which finish first so none outlives it. cmd makes one per
// run and binds Cleaning to what kills those groups, so no registry holds
// it.
type Interrupts struct {
	mu       sync.Mutex
	first    os.Signal
	cleaning int
	idle     *sync.Cond
}

// Cleaning marks owned process groups being killed and joined until the
// release it returns is called, which a later interrupt waits for. It may
// be called from any goroutine, and its release once.
func (i *Interrupts) Cleaning() (release func()) {
	i.mu.Lock()
	i.cleaning++
	i.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			i.mu.Lock()
			i.cleaning--
			if i.cleaning == 0 && i.idle != nil {
				i.idle.Broadcast()
			}
			i.mu.Unlock()
		})
	}
}

// Signal is the interrupt that stopped the run, nil when none came.
func (i *Interrupts) Signal() os.Signal {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.first
}

// Stop stops a command on the interrupts it reads, until interrupts is
// closed or end returns, which cmd's never does. The first cancels the
// work through cancel, with ErrInterrupted as the cause. A later one calls
// end with the first interrupt, to end the process at once; one that comes
// while owned process groups are being killed waits for them instead and
// is spent there, the command then ending on its own once their cleanup
// is done, so the report it writes is not cut short. cmd reads interrupts
// from signal.Notify, the one line that differs by system, so a test feeds
// it interrupts of its own and sees what each does.
func (i *Interrupts) Stop(interrupts <-chan os.Signal, cancel context.CancelCauseFunc, end func(os.Signal)) {
	sig, ok := <-interrupts
	if !ok {
		return
	}
	i.mu.Lock()
	i.first = sig
	i.mu.Unlock()
	cancel(ErrInterrupted)
	for {
		if _, ok := <-interrupts; !ok {
			return
		}
		if i.waitCleaning() {
			continue
		}
		end(sig)
		return
	}
}

// waitCleaning waits while owned process groups are being killed, and says
// whether it waited.
func (i *Interrupts) waitCleaning() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.cleaning == 0 {
		return false
	}
	if i.idle == nil {
		i.idle = sync.NewCond(&i.mu)
	}
	for i.cleaning > 0 {
		i.idle.Wait()
	}
	return true
}

// Interrupted is the problem of a command an interrupt cancelled: exit 130,
// what shells report for Ctrl-C (128 and SIGINT's 2), on every system,
// SIGTERM's included. A domain's classifier gives it for its kind of a
// cancelled failure whose cause is ErrInterrupted.
func Interrupted() *Problem {
	return NewProblem(CodeInterrupted, "interrupted", "interrupted before it finished", "Run it again and let it finish.")
}
