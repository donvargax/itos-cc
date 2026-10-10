//go:build !windows

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/donvargax/itos-cc/mutate"
)

// The scenario of "Rule: Fresh counted mutation judges a bounded committed
// selection without claiming full proof" in features/mutate.feature that the
// mutation-counted-interrupt slice holds. It builds itos-cc and runs it as a
// subprocess, so a signal reaches a real process, over a module of two
// sites: Compare's, which its test kills at once, and Gate's, whose mutant
// makes Gate(5) call hold. The test sets hold to start owned descendants
// (a sleeping child, a shell with a sleeping grandchild, and a watcher
// that marks the worker copy's go.mod disappearing while it still runs),
// record their PIDs, mark that it started, and wait for a release that
// never comes. Every run of the test binary is recorded too. All files go
// to the directory GATE_DIR names. Only the mutant ever calls hold, so
// coverage and the clean baseline finish.

var gateFiles = map[string]string{
	"go.mod":  "module example.com/gate\n\ngo 1.22\n",
	"main.go": "package main\n\n// hold is what Gate does once n passes 5; its test sets it.\nvar hold = func() {}\n\nfunc Compare(n int) bool { return n > 5 }\n\nfunc Gate(n int) int {\n\tif n > 5 {\n\t\thold()\n\t}\n\treturn n\n}\n\nfunc main() {}\n",
	"main_test.go": `package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if dir := os.Getenv("GATE_DIR"); dir != "" {
		if f, err := os.OpenFile(filepath.Join(dir, "runs"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			fmt.Fprintf(f, "run %d\n", os.Getpid())
			f.Close()
		}
	}
	os.Exit(m.Run())
}

func TestCompare(t *testing.T) {
	if !Compare(6) || Compare(5) {
		t.Fatal("Compare")
	}
}

func TestGate(t *testing.T) {
	hold = func() {
		dir := os.Getenv("GATE_DIR")
		child := exec.Command("sleep", "300")
		shell := exec.Command("sh", "-c", "sleep 300 & echo $! > \"$GATE_DIR/grandchild\"; wait")
		watcher := exec.Command("sh", "-c", "while [ -e go.mod ]; do sleep 0.01; done; touch \"$GATE_DIR/removed-while-alive\"")
		pids := []string{fmt.Sprint(os.Getpid())}
		for _, c := range []*exec.Cmd{child, shell, watcher} {
			if err := c.Start(); err != nil {
				t.Fatal(err)
			}
			pids = append(pids, fmt.Sprint(c.Process.Pid))
		}
		for {
			data, _ := os.ReadFile(filepath.Join(dir, "grandchild"))
			if grandchild := strings.TrimSpace(string(data)); grandchild != "" {
				pids = append(pids, grandchild)
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		os.WriteFile(filepath.Join(dir, "pids"), []byte(strings.Join(pids, "\n")+"\n"), 0o644)
		os.WriteFile(filepath.Join(dir, "started"), nil, 0o644)
		for {
			if _, err := os.Stat(filepath.Join(dir, "release")); err == nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	if Gate(5) != 5 {
		t.Fatal("Gate")
	}
}
`,
}

// buildItosCc builds this command into a temporary directory and returns
// the binary's path; it must run in the package's directory.
func buildItosCc(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "itos-cc")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// alive reports whether pid is a process that has not exited: one the
// kernel still lists that is no zombie. It works the same on Linux and
// macOS, which has no /proc. A ps that cannot answer counts as alive, so a
// broken probe fails a check rather than passing it.
func alive(pid int) bool {
	if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
		return false
	}
	out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	state := strings.TrimSpace(string(out))
	var exit *exec.ExitError
	if errors.As(err, &exit) && state == "" {
		return false // ps lists no such process
	}
	if err != nil {
		return true
	}
	return !strings.HasPrefix(state, "Z") && !strings.HasPrefix(state, "X")
}

// waitFor polls until ok holds, failing t after limit or once exited closes.
func waitFor(t *testing.T, what string, limit time.Duration, exited <-chan struct{}, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for !ok() {
		select {
		case <-exited:
			t.Fatalf("itos-cc exited before %s", what)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("no %s within %v", what, limit)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func lineCount(path string) int {
	data, _ := os.ReadFile(path)
	return strings.Count(string(data), "\n")
}

// @ID-MUT-183
func TestInterruptionReportsPartialWorkAndCleansUpOwnedCommands(t *testing.T) {
	requireCountedPlatform(t)
	checkInterruptedCountedRun(t)
}

// checkInterruptedCountedRun interrupts a counted run while its second
// judgment is active and checks its partial report and owned cleanup.
func checkInterruptedCountedRun(t *testing.T) {
	t.Helper()
	bin := buildItosCc(t)
	dir := moduleRepo(t, gateFiles)
	gate := t.TempDir()
	// Compare's site first, then Gate's, one worker: the first judgment is
	// complete once the second one starts.
	seed := seedSelecting(t, dir, 2, func(s []mutate.FreshCandidate) bool {
		return inFunction("Compare")(s[0]) && inFunction("Gate")(s[1])
	})

	// An unrelated process the run must leave alone.
	unrelated := exec.Command("sleep", "300")
	unrelated.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := unrelated.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { unrelated.Process.Kill(); unrelated.Wait() })
	// Whatever the run leaves behind is stopped after the test.
	t.Cleanup(func() {
		data, _ := os.ReadFile(filepath.Join(gate, "pids"))
		for _, field := range strings.Fields(string(data)) {
			if pid, err := strconv.Atoi(field); err == nil && alive(pid) {
				syscall.Kill(pid, syscall.SIGKILL)
			}
		}
		os.WriteFile(filepath.Join(gate, "release"), nil, 0o644)
	})

	var stdout bytes.Buffer
	stderr, err := os.Create(filepath.Join(t.TempDir(), "stderr"))
	if err != nil {
		t.Fatal(err)
	}
	defer stderr.Close()
	run := exec.Command(bin, "mutation", "run", "--json", "--count", "2", "--seed", seed, "--workers", "1", "--timeout-factor", "100")
	run.Dir, run.Stdout, run.Stderr = dir, &stdout, stderr
	run.Env = append(os.Environ(), "GATE_DIR="+gate)
	if err := run.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	var waitErr error
	go func() { waitErr = run.Wait(); close(exited) }()
	t.Cleanup(func() {
		select {
		case <-exited:
		default:
			run.Process.Kill()
			<-exited
		}
	})
	defer func() {
		if t.Failed() {
			text, _ := os.ReadFile(stderr.Name())
			t.Logf("stdout:\n%s\nstderr:\n%s", stdout.String(), text)
		}
	}()

	// Given one completed and one active judgment.
	waitFor(t, "active second judgment", 3*time.Minute, exited, func() bool {
		_, err := os.Stat(filepath.Join(gate, "started"))
		return err == nil
	})
	text, _ := os.ReadFile(stderr.Name())
	if !strings.Contains(string(text), "[1/2]") {
		t.Fatalf("the second judgment started before the first completed:\n%s", text)
	}
	var pids []int
	data, _ := os.ReadFile(filepath.Join(gate, "pids"))
	for _, field := range strings.Fields(string(data)) {
		pid, _ := strconv.Atoi(field)
		pids = append(pids, pid)
	}
	if len(pids) != 5 {
		t.Fatalf("recorded descendants %v, want the test binary, child, shell, watcher and grandchild", pids)
	}
	runsBefore := lineCount(filepath.Join(gate, "runs"))

	// When the run is interrupted, and signalled again while it cleans up.
	interrupted := time.Now()
	if err := run.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	run.Process.Signal(syscall.SIGTERM)
	select {
	case <-exited:
	case <-time.After(time.Minute):
		t.Fatal("itos-cc did not exit within a minute of its interruption")
	}
	took := time.Since(interrupted)

	// Then the completed result remains visible.
	var c countedJSON
	if err := json.Unmarshal(stdout.Bytes(), &c); err != nil {
		t.Errorf("no partial report as one JSON object (exit %v): %v", waitErr, err)
	}
	if len(c.Selected) != 2 || c.Selected[0]["state"] != "judged" || c.Selected[0]["outcome"] != "killed" {
		t.Errorf("selected = %+v, want Compare's mutant still reported killed", c.Selected)
	}
	// The unfinished judgment is cancelled or undecided, not killed or timed out.
	if len(c.Selected) != 2 {
		t.Errorf("no report of the unfinished judgment")
	} else if active := c.Selected[1]; (active["state"] != "cancelled" && active["state"] != "undecided") || active["outcome"] != nil {
		t.Errorf("active site %+v: want cancelled or undecided with no outcome", active)
	}
	code := run.ProcessState.ExitCode()
	if p := c.problem("count.interrupted"); code != 75 || p == nil || c.Sampling["completion"] != "interrupted" {
		t.Errorf("exit %d, completion %v, problems %+v: want 75 with count.interrupted and completion interrupted",
			code, c.Sampling["completion"], c.Problems)
	}
	// No further command is admitted after the abort.
	if after := lineCount(filepath.Join(gate, "runs")); after != runsBefore {
		t.Errorf("the test binary started %d more times after the interruption", after-runsBefore)
	}
	// Every owned descendant is gone before the private inputs are removed.
	for _, pid := range pids {
		if alive(pid) {
			t.Errorf("owned process %d outlived the run", pid)
		}
	}
	if _, err := os.Stat(filepath.Join(gate, "removed-while-alive")); err == nil {
		t.Error("the worker copy was removed while an owned process still ran")
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".git", "itos", "fresh-plan-*")); len(left) > 0 {
		t.Errorf("the frozen inputs %v were left behind", left)
	}
	// One shared five-second cleanup deadline, unrelated processes untouched.
	if took > 15*time.Second {
		t.Errorf("the run took %v to finish after its interruption, want its cleanup within the five-second deadline", took)
	}
	if !alive(unrelated.Process.Pid) {
		t.Error("an unrelated process was killed")
	}
}
