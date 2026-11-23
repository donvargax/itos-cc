//go:build linux || darwin

package processes

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// The owned command's cancel, exit, reap and cleanup machine, as
// docs/state-machines.md names it: its states, its actions and its
// invariants carry the names used below. A real shell is the leader of a
// real group, so what the command's own exit status says, and what
// os/exec's Cancel and Wait do with it, are the system's; the group's other
// members, the kernel's answers to kill(2) and the clock are a model,
// played through groupCalls, so one Linux run plays darwin's answers too.

// ownedMachineChecks bounds the runs of one test, so CI's time stays flat:
// an explicit -rapid.checks, or a smaller RAPID_CHECKS, still decides.
const ownedMachineChecks = 40

// TestOwnedCommandMachine drives one owned command through every order of
// its leader's own exit, its members' exits, init's reaping, a cancel or a
// timeout, the deadline passing and the reaping of the leader, and holds the
// invariants of docs/state-machines.md at each step and at the end.
func TestOwnedCommandMachine(t *testing.T) {
	set := false
	flag.Visit(func(f *flag.Flag) { set = set || f.Name == "rapid.checks" })
	if checks := flag.Lookup("rapid.checks"); !set && checks != nil {
		if n, err := strconv.Atoi(checks.Value.String()); err == nil && n > ownedMachineChecks {
			_ = checks.Value.Set(strconv.Itoa(ownedMachineChecks))
			defer func() { _ = checks.Value.Set(strconv.Itoa(n)) }()
		}
	}
	rapid.Check(t, func(t *rapid.T) {
		m := newOwnedMachine(t)
		defer m.shutdown()
		t.Repeat(map[string]func(*rapid.T){
			"ExitLeader":   m.exitLeader,
			"MemberExits":  m.memberExits,
			"InitReaps":    m.initReaps,
			"Cancel":       m.cancel,
			"PastDeadline": m.pastDeadline,
			"Reap":         m.reapLeader,
			"":             m.invariants,
		})
		m.reapLeader(t)
		m.judgment(t)
	})
}

// ownedProc is a process's state as the kernel's answers see it.
type ownedProc int

const (
	procAbsent ownedProc = iota
	procRunning
	procZombie
	procGone
)

func (p ownedProc) inGroup() bool { return p == procRunning || p == procZombie }

// ownedMachine is the machine's one run: the system as a value (platform
// and its answer to a signal toward a group of zombies alone) and the
// states of the leader, its two members, the context, the clock and the run.
type ownedMachine struct {
	platform   string
	zombieKill error // kill(2)'s answer toward a group of zombies alone

	mu          sync.Mutex
	pid         int
	leader      ownedProc
	killable    ownedProc // a member SIGKILL ends
	unkillable  ownedProc // a member SIGKILL does not end
	clock       time.Time
	start       time.Time
	deadline    time.Time // the cleanup deadline, once cleanup began
	ticks       int       // sleeps of the cleanup's polling
	initReapsAt int       // the tick at which init reaps the members' zombies
	violations  []string
	signals     int
	sigkills    int

	leaderEnd string // "", "exit" (its own) or "killed" (by the group kill)
	exitCode  int
	cause     error // the context's cause once it ended
	cancelled bool
	sleptPast bool

	stdin   *os.File // the command's stdin: its exit code goes in
	started chan struct{}
	gate    chan struct{} // opens Wait: the leader's reaping
	killed  chan struct{} // closed at the first SIGKILL of the group
	done    chan struct{} // closed when Run returned
	cancelF context.CancelCauseFunc
	pidLine *os.File
	gated   sync.Once
	reaped  bool

	result Result
	err    error
	out    bytes.Buffer
}

var errOwnedTimeout = ErrCommandTimeout
var errOwnedParent = errors.New("the parent stopped")

func newOwnedMachine(t *rapid.T) *ownedMachine {
	m := &ownedMachine{
		leader: procRunning, killable: procAbsent, unkillable: procAbsent,
		started: make(chan struct{}), gate: make(chan struct{}), killed: make(chan struct{}), done: make(chan struct{}),
		clock: time.Now(),
	}
	m.start = m.clock
	m.platform, m.zombieKill = "linux", error(nil)
	if rapid.Bool().Draw(t, "darwin") {
		m.platform, m.zombieKill = "darwin", syscall.EPERM
	}
	if rapid.Bool().Draw(t, "killable member") {
		m.killable = procRunning
	}
	if rapid.Bool().Draw(t, "unkillable member") {
		m.unkillable = procRunning
	}
	m.initReapsAt = rapid.IntRange(1, 4).Draw(t, "init reaps at tick")
	if rapid.Bool().Draw(t, "init is slow") {
		m.initReapsAt = 1 << 30
	}

	stdinR, stdinW := mustPipe(t)
	pidR, pidW := mustPipe(t)
	m.stdin, m.pidLine = stdinW, pidR
	ctx, cancel := context.WithCancelCause(context.Background())
	m.cancelF = cancel
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", `echo out; echo $$ >&2; read code; exit "$code"`)
	cmd.Stdin, cmd.Stderr, cmd.Stdout = stdinR, pidW, &m.out

	calls := systemGroupCalls()
	calls.signal = m.signal
	calls.runnable = m.runnable
	calls.now = func() time.Time { m.mu.Lock(); defer m.mu.Unlock(); return m.clock }
	calls.sleep = m.sleep
	calls.wait = func(c *exec.Cmd) error {
		close(m.started)
		<-m.gate
		err := c.Wait()
		m.mu.Lock()
		m.leader = procGone
		m.reaped = true
		m.mu.Unlock()
		return err
	}
	runner := &Runner{Platform: m.platform, calls: &calls}
	runner.Lifecycle.Cleanup = func(d time.Time) { m.mu.Lock(); m.deadline = d; m.mu.Unlock() }
	go func() {
		m.result, m.err = runner.Run(ctx, cmd, &m.out)
		close(m.done)
	}()
	select {
	case <-m.started:
	case <-time.After(10 * time.Second):
		t.Fatal("the command did not start")
	}
	_ = stdinR.Close()
	_ = pidW.Close()
	if _, err := fmt.Fscanln(pidR, &m.pid); err != nil {
		t.Fatalf("the command's process id: %v", err)
	}
	return m
}

func mustPipe(t *rapid.T) (*os.File, *os.File) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	return r, w
}

// shutdown lets a run the machine did not finish end, whatever it was doing.
func (m *ownedMachine) shutdown() {
	m.cancelF(nil)
	m.gated.Do(func() { close(m.gate) })
	_, _ = fmt.Fprintln(m.stdin, 0)
	select {
	case <-m.done:
	case <-time.After(10 * time.Second):
	}
	_ = m.stdin.Close()
	_ = m.pidLine.Close()
}

// answer is kill(2)'s toward the group, as Linux's kill_pgrp and XNU's
// killpg1 give it: nothing in the group is ESRCH; something that runs takes
// the signal; zombies alone are the system's answer; only a member that
// cannot be signalled is EPERM.
func (m *ownedMachine) answer() error {
	switch {
	case !m.leader.inGroup() && !m.killable.inGroup() && !m.unkillable.inGroup():
		return syscall.ESRCH
	case m.leader == procRunning || m.killable == procRunning:
		return nil
	case m.leader == procZombie || m.killable == procZombie || m.unkillable == procZombie:
		return m.zombieKill
	}
	return syscall.EPERM
}

func (m *ownedMachine) signal(group int, sig syscall.Signal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.signals++
	if group != -m.pid {
		m.violations = append(m.violations, fmt.Sprintf("signal %d toward %d; only the owned group %d, as -pgid, may be signalled", sig, group, -m.pid))
		return syscall.EPERM
	}
	if sig != syscall.SIGKILL && sig != 0 {
		m.violations = append(m.violations, fmt.Sprintf("signal %d; only SIGKILL and the probe may be sent", sig))
	}
	answer := m.answer()
	if sig == syscall.SIGKILL {
		m.sigkills++
		if !m.cancelled && !m.reaped {
			m.violations = append(m.violations, "the group was killed before its context ended or its leader was reaped")
		}
		if m.leader == procRunning {
			if err := syscall.Kill(group, syscall.SIGKILL); err != nil {
				m.violations = append(m.violations, fmt.Sprintf("the real kill failed: %v", err))
			}
			m.leader = procZombie
			m.leaderEnd = "killed"
		}
		if m.killable == procRunning {
			m.killable = procZombie
		}
		if m.sigkills == 1 {
			close(m.killed)
		}
	}
	return answer
}

func (m *ownedMachine) runnable(pgid int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if pgid != m.pid {
		m.violations = append(m.violations, fmt.Sprintf("scanned group %d; only the owned group %d may be", pgid, m.pid))
	}
	return m.killable == procRunning || m.unkillable == procRunning, nil
}

func (m *ownedMachine) sleep(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.deadline.IsZero() && !m.clock.Before(m.deadline) {
		m.sleptPast = true
	}
	m.clock = m.clock.Add(d)
	m.ticks++
	if m.ticks >= m.initReapsAt {
		m.reapMembers()
	}
}

func (m *ownedMachine) reapMembers() {
	if m.killable == procZombie {
		m.killable = procGone
	}
	if m.unkillable == procZombie {
		m.unkillable = procGone
	}
}

// skipOnceReturned ends the machine's actions with the run: once Run
// returned, nothing happens to the command any more.
func (m *ownedMachine) skipOnceReturned(t *rapid.T) {
	select {
	case <-m.done:
		t.Skip("the run returned")
	default:
	}
}

// exitLeader is the command's own exit, with its code, while it runs.
func (m *ownedMachine) exitLeader(t *rapid.T) {
	m.skipOnceReturned(t)
	m.mu.Lock()
	running := m.leader == procRunning
	m.mu.Unlock()
	if !running {
		t.Skip("the leader is not running")
	}
	code := rapid.IntRange(0, 1).Draw(t, "exit code")
	if _, err := fmt.Fprintln(m.stdin, code); err != nil {
		t.Fatalf("release the command: %v", err)
	}
	_ = m.pidLine.SetReadDeadline(time.Now().Add(10 * time.Second))
	if _, err := io.Copy(io.Discard, m.pidLine); err != nil {
		t.Fatalf("the command did not exit: %v", err)
	}
	m.mu.Lock()
	m.leader, m.leaderEnd, m.exitCode = procZombie, "exit", code
	m.mu.Unlock()
}

// memberExits is a member's own exit: it becomes a zombie until init reaps it.
func (m *ownedMachine) memberExits(t *rapid.T) {
	m.skipOnceReturned(t)
	m.mu.Lock()
	defer m.mu.Unlock()
	var running []*ownedProc
	for _, p := range []*ownedProc{&m.killable, &m.unkillable} {
		if *p == procRunning {
			running = append(running, p)
		}
	}
	if len(running) == 0 {
		t.Skip("no member runs")
	}
	*running[rapid.IntRange(0, len(running)-1).Draw(t, "member")] = procZombie
}

// initReaps is init reaping every member's zombie.
func (m *ownedMachine) initReaps(t *rapid.T) {
	m.skipOnceReturned(t)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.killable != procZombie && m.unkillable != procZombie {
		t.Skip("no member is a zombie")
	}
	m.reapMembers()
}

// cancel ends the context, by the command's own timeout or its parent's
// cancel, and waits for the group kill os/exec's Cancel makes of it.
func (m *ownedMachine) cancel(t *rapid.T) {
	m.skipOnceReturned(t)
	m.mu.Lock()
	already := m.cancelled
	m.mu.Unlock()
	if already {
		t.Skip("the context has ended")
	}
	cause := errOwnedParent
	if rapid.Bool().Draw(t, "by its timeout") {
		cause = errOwnedTimeout
	}
	m.mu.Lock()
	m.cancelled, m.cause = true, cause
	m.mu.Unlock()
	m.cancelF(cause)
	select {
	case <-m.killed:
	case <-time.After(10 * time.Second):
		t.Fatal("the cancel never reached the group")
	}
}

// pastDeadline is the clock passing the cleanup deadline.
func (m *ownedMachine) pastDeadline(t *rapid.T) {
	m.skipOnceReturned(t)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.clock.After(m.start.Add(commandCleanupLimit + time.Hour/2)) {
		t.Skip("the clock is already past")
	}
	m.clock = m.clock.Add(commandCleanupLimit + time.Minute)
}

// reapLeader is Wait reaping the leader, which starts the cleanup, and ends
// the run; a leader still running exits on its own first.
func (m *ownedMachine) reapLeader(t *rapid.T) {
	select {
	case <-m.done:
		return
	default:
	}
	m.mu.Lock()
	running := m.leader == procRunning
	m.mu.Unlock()
	if running {
		m.exitLeader(t)
	}
	m.gated.Do(func() { close(m.gate) })
	select {
	case <-m.done:
	case <-time.After(10 * time.Second):
		t.Fatal("the run did not return")
	}
}

// invariants are the ones that hold between any two actions.
func (m *ownedMachine) invariants(t *rapid.T) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.violations {
		t.Errorf("%s", v)
	}
	select {
	case <-m.done:
		if !m.reaped {
			t.Errorf("the run returned before Wait reaped its leader")
		}
	default:
	}
}

// judgment is the run's end: how it was judged against how it ended.
func (m *ownedMachine) judgment(t *rapid.T) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.violations {
		t.Errorf("%s", v)
	}
	if m.sleptPast {
		t.Errorf("the cleanup went on polling after its deadline")
	}
	var cleanup *CleanupFailed
	failed := errors.As(m.err, &cleanup)
	if m.err != nil && !failed {
		t.Fatalf("error = %v; only a CleanupFailed may be", m.err)
	}
	gone := m.groupGone()
	if failed == gone {
		t.Errorf("cleanup failed = %v with the group gone = %v (leader %d, killable %d, unkillable %d, %s)", failed, gone, m.leader, m.killable, m.unkillable, m.platform)
	}
	if failed {
		if m.clock.Before(m.start.Add(commandCleanupLimit)) {
			t.Errorf("cleanup failed before its deadline, at %v of %v", m.clock.Sub(m.start), commandCleanupLimit)
		}
		return
	}
	if want := "out\n"; m.result.Output != want {
		t.Errorf("output = %q; want %q, handed over whole", m.result.Output, want)
	}
	r := m.result
	switch m.leaderEnd {
	case "exit":
		if r.ExitCode != m.exitCode || r.Passed != (m.exitCode == 0) || r.TimedOut || r.Cancelled {
			t.Errorf("a command that exited %d is judged %+v; want its own exit status", m.exitCode, r)
		}
	case "killed":
		if r.Passed || r.ExitCode != -1 || r.TimedOut != (m.cause == errOwnedTimeout) || r.Cancelled != (m.cause == errOwnedParent) {
			t.Errorf("a command killed on %v is judged %+v", m.cause, r)
		}
	default:
		t.Errorf("the command ended in no way")
	}
}

// groupGone is what the system's probe of the group must find once the
// cleanup ended: on Linux no member that can run, on darwin nothing in it.
func (m *ownedMachine) groupGone() bool {
	if m.platform == "linux" {
		return m.killable != procRunning && m.unkillable != procRunning
	}
	return !m.leader.inGroup() && !m.killable.inGroup() && !m.unkillable.inGroup()
}
