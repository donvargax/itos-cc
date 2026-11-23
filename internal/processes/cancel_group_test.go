package processes

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

// TestCancelGroupKillsTheGroupAndAnswersProcessDone pins what the kill is
// aimed at (the group, the negative id) and that its answer never reaches
// os/exec, whatever it is.
func TestCancelGroupKillsTheGroupAndAnswersProcessDone(t *testing.T) {
	for _, answer := range []error{nil, syscall.ESRCH, syscall.EPERM} {
		var group int
		var sent syscall.Signal
		calls := groupCalls{signal: func(g int, s syscall.Signal) error { group, sent = g, s; return answer }}
		if err := cancelGroup(calls, 4242); !errors.Is(err, os.ErrProcessDone) {
			t.Errorf("kill answered %v: cancelGroup = %v; want os.ErrProcessDone", answer, err)
		}
		if group != -4242 || sent != syscall.SIGKILL {
			t.Errorf("signalled %d with %d; want SIGKILL to -4242", group, sent)
		}
	}
}
