package cli

import (
	"context"
	"os"
)

// StopOnInterrupt stops a command on an interrupt, Ctrl-C (decision 31). The
// first cancels the command's work through cancel, so whatever waits on its
// context ends, and the command stops promptly, reporting the cancelled
// failure through Fail: exit CodeInterrupted, saying so once. A second
// calls exit with CodeInterrupted at once, skipping whatever cleanup the
// first left running.
//
// Every command shares it. cmd feeds interrupts from signal.Notify, the one
// line that differs by system, and passes os.Exit as exit, so a test feeds
// it interrupts of its own and sees what each does. It returns once
// interrupts is closed or exit returns, which os.Exit never does.
func StopOnInterrupt(interrupts <-chan os.Signal, cancel context.CancelFunc, exit func(code int)) {
	if _, ok := <-interrupts; !ok {
		return
	}
	cancel()
	if _, ok := <-interrupts; !ok {
		return
	}
	exit(CodeInterrupted)
}

// interrupted is the problem of a command an interrupt cancelled: exit 130,
// what shells report for Ctrl-C (128 and SIGINT's 2), on every system.
func interrupted() (int, []Problem) {
	return CodeInterrupted, one("INTERRUPTED", "interrupted before it finished", "run it again and let it finish")
}
