// The runs a scenario interrupts: itos-cc, or a shell running it, started
// as a process of its own, and signalled once its output shows it under
// way, never after a guessed delay. How a process is signalled differs by
// system, in the build-tagged signals_unix.go and signals_windows.go; what
// is judged of it is here and in the world's steps. Only the binary driver
// can send a process a signal, so these steps are its own, given to the
// scenario's world beside the steps every driver takes.
package features

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// startTimeout is how long a run has to show it is under way: serve to
// say it serves, a test to start its process; stopTimeout how long it has
// to end once signalled, the "promptly" a scenario holds it to.
const (
	startTimeout = 60 * time.Second
	stopTimeout  = 10 * time.Second
)

// serving is the line serve writes on stderr once it listens.
var serving = regexp.MustCompile(`(?m)^itos-cc: serving .* on http://`)

// running is a run still under way: itos-cc, or the shell running it, and
// what it has written.
type running struct {
	cmd            *exec.Cmd
	stdout, stderr capturedOutput
	done           chan error // Wait's result, once the process ended
	exited         bool       // done has been read: cmd.ProcessState is the process's end
	group          bool       // the process leads a process group of its own
}

// interrupter is a scenario's run of the binary at bin, nil for none, the
// world whose output it leaves, and the processes a project's tests
// started, which a step watches end.
type interrupter struct {
	w       *world
	bin     string
	running *running
	peers   FixturePeers
	started []FixturePeer
	records string
	script  string
}

// interruptSteps are the steps of a run that goes on until something ends
// it, and the signal that does, for the binary at bin. The run, still under
// way after the scenario, is stopped then, and the processes its tests
// started, before its folder is removed.
func interruptSteps(bin string) func(*godog.ScenarioContext, *world) {
	return func(sc *godog.ScenarioContext, w *world) {
		i := &interrupter{w: w, bin: bin}
		sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
			*i = interrupter{w: w, bin: bin}
			return ctx, nil
		})
		sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
			i.running.stop()
			if i.peers != nil {
				cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
				defer cancel()
				if err := i.peers.Close(cleanup); err != nil {
					return ctx, err
				}
			}
			if i.records != "" {
				return ctx, w.d.Remove(i.records)
			}
			return ctx, nil
		})
		sc.Step(`^"itos-cc ([^"]*)" is serving the scratch folder$`, i.serve)
		sc.Step(`^it is interrupted$`, func() error { return i.signal(interrupt) })
		sc.Step(`^it is sent SIGTERM$`, i.terminate)
		sc.Step(`^a Go project whose test starts a process of its own and then hangs$`, i.hangingProject)
		sc.Step(`^"itos-cc ([^"]*)" is interrupted once that process is running$`, i.interruptOnceStarted)
		sc.Step(`^no process its tests started is still running$`, i.startedEnded)
		sc.Step(`^a shell runs:$`, func(script *godog.DocString) error { i.script = script.Content; return nil })
		sc.Step(`^itos-cc alone is interrupted once it is serving$`, i.interruptItosCCAlone)
		sc.Step(`^Ctrl-C reaches the shell and itos-cc once itos-cc is serving$`, i.interruptShellAndItosCC)
		sc.Step(`^the shell prints "([^"]*)"$`, i.shellPrints)
		sc.Step(`^the shell ends interrupted, before the second itos-cc starts$`, i.shellEndedInterrupted)
	}
}

// start starts name with args in the scratch folder, the scenario's
// environment and env besides, its own process group when group is set,
// and returns without waiting for it.
func (i *interrupter) start(name string, args []string, env []string, group bool) error {
	w := i.w
	r := &running{cmd: exec.Command(name, args...), done: make(chan error, 1), group: group} //nolint:gosec // itos-cc under test, or the shell a scenario runs it in
	r.cmd.Dir = w.dir
	r.cmd.Env = append(environment(w.env()), env...)
	r.cmd.Stdout = &r.stdout
	r.cmd.Stderr = &r.stderr
	// What a process it started holds of its output never keeps the step
	// waiting once it ended.
	r.cmd.WaitDelay = 2 * time.Second
	if group {
		r.cmd.SysProcAttr = ownGroup()
	} else {
		interruptible(r.cmd)
	}
	if err := r.cmd.Start(); err != nil {
		return fmt.Errorf("starting %s: %w", name, err)
	}
	go func() { r.done <- r.cmd.Wait() }()
	i.running = r
	return nil
}

// await waits until the run's stderr shows ready, failing when the run ends
// first or does not show it within startTimeout.
func (i *interrupter) await(ready *regexp.Regexp, what string) error {
	r := i.running
	deadline := time.After(startTimeout)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		if _, stderr := r.output(); ready.MatchString(stderr) {
			return nil
		}
		select {
		case err := <-r.done:
			r.done <- err
			return i.ended(fmt.Errorf("the run ended before %s", what))
		case <-deadline:
			r.stop()
			return i.ended(fmt.Errorf("the run did not show %s within %s", what, startTimeout))
		case <-tick.C:
		}
	}
}

// serve starts itos-cc with args, serve's command line, on a port the
// system picks unless args name one, so scenarios beside it never take
// the same, and waits until it says it serves.
func (i *interrupter) serve(args string) error {
	fields, err := commandWords(args)
	if err != nil {
		return err
	}
	if !strings.Contains(args, "--port") {
		fields = append(fields, "--port", "0")
	}
	if err := i.start(i.bin, fields, nil, false); err != nil {
		return err
	}
	return i.await(serving, "that it serves")
}

// signal sends the run send, as each system sends it, and waits for it to
// end, its exit code and output what the Then steps read. A run still
// under way stopTimeout after is killed and read as never ending.
func (i *interrupter) signal(send func(*os.Process) error) error {
	r := i.running
	if r == nil {
		return errors.New("nothing runs to signal: start it first")
	}
	if err := send(r.cmd.Process); err != nil {
		r.stop()
		return fmt.Errorf("signalling the run: %w", err)
	}
	return i.wait()
}

// wait waits for the run to end, killing it after stopTimeout.
func (i *interrupter) wait() error {
	r := i.running
	select {
	case <-r.done:
		r.exited = true
		return i.ended(nil)
	case <-time.After(stopTimeout):
		r.stop()
		_ = i.ended(nil) // nil in, nil out: it records the run for the Then steps
		i.w.exit, i.w.signal = -1, 0
		i.w.stderr += fmt.Sprintf("\n(the run did not end within %s of the signal, and was killed)\n", stopTimeout)
		return nil
	}
}

// terminate sends the run SIGTERM, which Windows sends no process.
func (i *interrupter) terminate() error {
	if i.w.d.Platform() == "windows" {
		i.running.stop()
		return fmt.Errorf("%w: Windows sends no process SIGTERM", godog.ErrSkip)
	}
	return i.signal(terminate)
}

// ended records what the stopped run wrote, its exit code and the signal
// that ended it for the Then steps, and returns err with them, so a
// failing step shows the run. A run the steps start has no last run to
// make again with --json.
func (i *interrupter) ended(err error) error {
	w, r := i.w, i.running
	w.stdout, w.stderr = r.output()
	w.exit, w.signal, w.last, w.each = -1, 0, nil, nil
	if r.exited {
		w.exit, w.signal = r.cmd.ProcessState.ExitCode(), signalOf(r.cmd.ProcessState)
	}
	if err != nil {
		return fmt.Errorf("%w\n%s", err, w.report())
	}
	return nil
}

// output is what the run has written so far.
func (r *running) output() (stdout, stderr string) { return r.stdout.text(), r.stderr.text() }

// stop ends the run if it is still under way, by killing it, its group
// too when it leads one, and waits for it, so no scenario leaves a process
// behind and its end is known; on a nil run, nothing.
func (r *running) stop() {
	if r == nil || r.exited {
		return
	}
	if r.group {
		_ = killGroup(r.cmd.Process.Pid)
	}
	_ = r.cmd.Process.Kill()
	<-r.done
	r.exited = true
}

// The project whose test starts a process and hangs: its test, and the
// process it starts, each connect to the scenario's fixture peers and
// publish a ready record, the process once it runs, so a step knows when
// it is under way and when each has ended, and stops them after the
// scenario. Neither is itos-cc: what ends them is what crap does once
// interrupted.
const hangingTest = `package board

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHangs(t *testing.T) {
	if os.Getenv("ITOS_CC_STARTED") != "" {
		return
	}
	connect(t, "test")
	cmd := exec.Command(os.Args[0], "-test.run=^TestStarted$")
	cmd.Env = append(os.Environ(), "ITOS_CC_STARTED=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	select {}
}

func TestStarted(t *testing.T) {
	if os.Getenv("ITOS_CC_STARTED") == "" {
		return
	}
	connect(t, "started")
	select {}
}

// connect holds a connection to the scenario's peers until the process
// ends, and publishes the ready record named role.
func connect(t *testing.T, role string) {
	conn, err := net.DialTimeout("tcp", os.Getenv("T80_OWNED_ADDR"), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var b [24]byte
	if _, err = rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	nonce, pid := hex.EncodeToString(b[:]), os.Getpid()
	if err = json.NewEncoder(conn).Encode(map[string]any{"owner": os.Getenv("T80_OWNED_OWNER"), "nonce": nonce, "pid": pid}); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	if ack, err := reader.ReadString('\n'); err != nil || ack != "owned\n" {
		t.Fatalf("acknowledgment %q: %v", ack, err)
	}
	record, err := json.Marshal(map[string]any{"nonce": nonce, "pid": pid})
	if err != nil {
		t.Fatal(err)
	}
	final := filepath.Join(os.Getenv("ITOS_CC_RECORDS"), role+".json")
	if err = os.WriteFile(final+".tmp", record, 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(final+".tmp", final); err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				os.Exit(91)
			}
			switch strings.TrimSpace(line) {
			case "ping " + nonce:
				if _, err = io.WriteString(conn, "live "+nonce+"\n"); err != nil {
					os.Exit(92)
				}
			case "stop " + nonce:
				os.Exit(0)
			default:
				os.Exit(93)
			}
		}
	}()
}
`

// hangingProject writes the Go module board in the scratch folder, whose
// test starts a process of its own and then hangs, the two holding the
// scenario's fixture peers, which the system must provide.
func (i *interrupter) hangingProject(ctx context.Context) error {
	w := i.w
	peers, err := w.d.FixturePeers(ctx)
	if errors.Is(err, ErrPhysicalUnsupported) {
		return fmt.Errorf("%w: %w", godog.ErrSkip, err)
	}
	if err != nil {
		return err
	}
	i.peers = peers
	if i.records, err = w.d.Folder(); err != nil {
		return err
	}
	for name, text := range map[string]string{"go.mod": goModule, "board.go": "package board\n\nfunc Place(a int) int {\n\treturn a\n}\n", "board_test.go": hangingTest} {
		if err := w.write(name, text); err != nil {
			return err
		}
	}
	return w.d.Coverage(w.dir, Coverage{Language: "go"})
}

// interruptOnceStarted starts itos-cc with args, and interrupts it once the
// process its project's test starts is running.
func (i *interrupter) interruptOnceStarted(ctx context.Context, args string) error {
	fields, err := commandWords(args)
	if err != nil {
		return err
	}
	if i.peers == nil {
		return errors.New("no project's test starts a process: write one first")
	}
	if err = i.start(i.bin, fields, append(i.peers.Environment(), "ITOS_CC_RECORDS="+i.records), false); err != nil {
		return err
	}
	ready, cancel := context.WithTimeout(ctx, startTimeout)
	defer cancel()
	for _, role := range []string{"test", "started"} {
		peer, err := i.peers.Capture(ready, filepath.Join(i.records, role+".json"))
		if err != nil {
			i.running.stop()
			return i.ended(fmt.Errorf("the %s process did not start: %w", role, err))
		}
		i.started = append(i.started, peer)
	}
	return i.signal(interrupt)
}

// startedEnded is whether the test and the process it started have both
// ended, within moments of itos-cc's return.
func (i *interrupter) startedEnded(ctx context.Context) error {
	if len(i.started) != 2 {
		return errors.New("no process the tests started was seen running")
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for _, peer := range i.started {
		if err := peer.EOF(bounded); err != nil {
			return fmt.Errorf("a process the tests started is still running: %w\n%s", err, i.w.report())
		}
	}
	return nil
}

// shell starts bash with the scenario's script, itos-cc the binary under
// test on its PATH, in a process group of its own, and waits until
// itos-cc says it serves. bash stops a loop as POSIX shells do: when the
// command it waits on ends by the signal that interrupted both, not when
// it exits, whatever its status. A system without bash, or Windows, which
// sends no process SIGINT, skips.
func (i *interrupter) shell() error {
	w := i.w
	if w.d.Platform() == "windows" {
		return fmt.Errorf("%w: Windows sends no process SIGINT", godog.ErrSkip)
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		return fmt.Errorf("%w: no bash: %w", godog.ErrSkip, err)
	}
	if i.script == "" {
		return errors.New("no shell script was given")
	}
	bin, err := w.d.Folder()
	if err != nil {
		return err
	}
	i.records = bin
	if err := os.Symlink(i.bin, filepath.Join(bin, "itos-cc")); err != nil {
		return err
	}
	path := bin + string(os.PathListSeparator) + os.Getenv("PATH")
	for _, kv := range environment(w.env()) {
		if name, value, _ := strings.Cut(kv, "="); name == "PATH" {
			path = bin + string(os.PathListSeparator) + value
		}
	}
	if err := i.start(bash, []string{"-c", i.script}, []string{"PATH=" + path}, true); err != nil {
		return err
	}
	return i.await(serving, "that itos-cc serves")
}

// interruptItosCCAlone interrupts the itos-cc the shell runs, and no other
// process, once it serves, and waits for the shell to end.
func (i *interrupter) interruptItosCCAlone() error {
	if err := i.shell(); err != nil {
		return err
	}
	out, err := exec.Command("pgrep", "-P", strconv.Itoa(i.running.cmd.Process.Pid)).Output() //nolint:gosec // the shell's own children, by its process ID
	if err != nil {
		i.running.stop()
		return fmt.Errorf("the shell's itos-cc: pgrep: %w", err)
	}
	pids := strings.Fields(string(out))
	if len(pids) != 1 {
		i.running.stop()
		return fmt.Errorf("the shell runs %d processes, not itos-cc alone: %v", len(pids), pids)
	}
	pid, err := strconv.Atoi(pids[0])
	if err != nil {
		return err
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := interrupt(p); err != nil {
		i.running.stop()
		return fmt.Errorf("interrupting itos-cc: %w", err)
	}
	return i.wait()
}

// interruptShellAndItosCC sends SIGINT to the shell's process group, as
// Ctrl-C at a terminal does, once itos-cc serves, and waits for the shell
// to end.
func (i *interrupter) interruptShellAndItosCC() error {
	if err := i.shell(); err != nil {
		return err
	}
	if err := interruptGroup(i.running.cmd.Process.Pid); err != nil {
		i.running.stop()
		return fmt.Errorf("interrupting the shell's group: %w", err)
	}
	return i.wait()
}

// shellPrints is whether a line of the shell's stdout is text.
func (i *interrupter) shellPrints(text string) error {
	for _, line := range outputLines(i.w.stdout) {
		if strings.TrimSuffix(line, "\r") == text {
			return nil
		}
	}
	return fmt.Errorf("the shell prints no line %q\n%s", text, i.w.report())
}

// shellEndedInterrupted is whether the shell ended by SIGINT, or with the
// 130 a shell exits with for it, and only one itos-cc ever served.
func (i *interrupter) shellEndedInterrupted() error {
	w := i.w
	if w.signal != 2 && w.exit != 130 {
		return fmt.Errorf("the shell ended with exit %d and signal %d, not interrupted\n%s", w.exit, w.signal, w.report())
	}
	if n := len(serving.FindAllString(w.stderr, -1)); n != 1 {
		return fmt.Errorf("itos-cc served %d times: the loop went on\n%s", n, w.report())
	}
	return nil
}
