package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Harvested from donvargax/itos's tools/bin/nightly-survivors at v8.49.0 (its T-341), as main.go is.

// A real report: itos-cc 0.47.0's of itos's nightly's twenty, two survivors, cut to a few sites.
//
//go:embed testdata/survivors.json
var realReport string

// A real report of the shape this repository's nightly reads: a complete mutation run's, its
// mutants under files, over a fixture with one survivor.
//
//go:embed testdata/complete.json
var completeReport string

// What this test's own binary does when started as the wrapped command, or as the program.
const (
	envOut    = "NIGHTLY_SURVIVORS_FAKE_OUT"    // prints this on stdout
	envErr    = "NIGHTLY_SURVIVORS_FAKE_ERR"    // prints this on stderr
	envExit   = "NIGHTLY_SURVIVORS_FAKE_EXIT"   // exits with this
	envKill   = "NIGHTLY_SURVIVORS_FAKE_KILL"   // kills itself
	envArgs   = "NIGHTLY_SURVIVORS_FAKE_ARGS"   // prints its arguments and stdin, as JSON
	envReady  = "NIGHTLY_SURVIVORS_FAKE_READY"  // writes its pid to this file, then waits for envGo
	envGo     = "NIGHTLY_SURVIVORS_FAKE_GO"     // the file whose presence ends the wait
	envAsMain = "NIGHTLY_SURVIVORS_FAKE_ASMAIN" // is the program itself, this variable cleared
)

func TestMain(m *testing.M) {
	if os.Getenv(envAsMain) != "" {
		_ = os.Unsetenv(envAsMain)
		main()
	}
	if os.Getenv(envOut) == "" && os.Getenv(envErr) == "" && os.Getenv(envExit) == "" &&
		os.Getenv(envKill) == "" && os.Getenv(envArgs) == "" && os.Getenv(envReady) == "" {
		os.Exit(m.Run())
	}
	if file := os.Getenv(envReady); file != "" {
		if err := os.WriteFile(file, fmt.Append(nil, os.Getpid()), 0o644); err != nil {
			os.Exit(9)
		}
		for start := time.Now(); time.Since(start) < time.Minute; time.Sleep(5 * time.Millisecond) {
			if _, err := os.Stat(os.Getenv(envGo)); err == nil {
				break
			}
		}
	}
	if os.Getenv(envArgs) != "" {
		var stdin bytes.Buffer
		if _, err := stdin.ReadFrom(os.Stdin); err != nil {
			os.Exit(9)
		}
		line, _ := json.Marshal(map[string]any{"args": os.Args[1:], "stdin": stdin.String()})
		_, _ = os.Stdout.Write(line)
	}
	_, _ = os.Stdout.WriteString(os.Getenv(envOut))
	_, _ = os.Stderr.WriteString(os.Getenv(envErr))
	if os.Getenv(envKill) != "" {
		self, _ := os.FindProcess(os.Getpid())
		_ = self.Kill()
		time.Sleep(time.Minute)
	}
	var code int
	_, _ = fmt.Sscan(os.Getenv(envExit), &code)
	os.Exit(code)
}

func self(t *testing.T) string {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// wrapped is run around this binary as the command, printing out on stdout and exit code.
func wrapped(t *testing.T, out string, code int) (int, string, string) {
	t.Helper()
	t.Setenv(envOut, out)
	t.Setenv(envErr, "itos-cc: its log\n")
	t.Setenv(envExit, fmt.Sprint(code))
	var stdout, stderr bytes.Buffer
	got := run([]string{"--", self(t)}, &stdout, &stderr)
	return got, stdout.String(), stderr.String()
}

func report(rules ...string) string {
	var problems []string
	for _, r := range rules {
		problems = append(problems, fmt.Sprintf(`{"rule": %q, "message": "a %s"}`, r, r))
	}
	return "{\n  \"ok\": false,\n  \"problems\": [" + strings.Join(problems, ", ") + "],\n  \"schema\": 1\n}\n"
}

// The one case: exit 1 and survivors alone is exit 0, one annotation a survivor with its message,
// on stderr after the command's own, and stdout byte for byte what the command printed.
func TestSurvivorsAlonePass(t *testing.T) {
	code, stdout, stderr := wrapped(t, realReport, 1)
	if code != 0 {
		t.Errorf("exit %d, not 0", code)
	}
	if stdout != realReport {
		t.Errorf("stdout is not the report byte for byte:\n%s", stdout)
	}
	lines := strings.Split(strings.TrimSuffix(stderr, "\n"), "\n")
	if len(lines) != 3 || lines[0] != "itos-cc: its log" {
		t.Fatalf("stderr is not the command's line and an annotation for each of two survivors:\n%s", stderr)
	}
	for i, want := range []string{
		"::warning::internal/shell/shell.go:62:42: `!=` → `==` in github.com/donvargax/itos/v8/internal/shell.Result#Status survived, as ",
		"::warning::internal/config/defaults.go:120:70: `&&` → `||` in github.com/donvargax/itos/v8/internal/config#DefaultsFor survived, as ",
	} {
		if !strings.HasPrefix(lines[i+1], want) {
			t.Errorf("annotation %d is %q, not %q…", i, lines[i+1], want)
		}
	}
}

// A complete run says its survivors as a counted run does, its report's other keys being its own.
func TestACompleteRunsSurvivorsPass(t *testing.T) {
	code, stdout, stderr := wrapped(t, completeReport, 1)
	if code != 0 {
		t.Errorf("exit %d, not 0", code)
	}
	if stdout != completeReport {
		t.Errorf("stdout is not the report byte for byte:\n%s", stdout)
	}
	want := "itos-cc: its log\n::warning::calc.go:5:11: `>` → `>=` in example.com/fixture#Bigger survived\n"
	if stderr != want {
		t.Errorf("stderr is %q, not %q", stderr, want)
	}
}

// A workflow command's value keeps to its line: GitHub's escapes for %, CR and LF, % first.
func TestAnAnnotationIsEscaped(t *testing.T) {
	out := `{"problems": [{"rule": "mutation.survived", "message": "100% of\r\nit %0A::error::no"}]}`
	code, stdout, stderr := wrapped(t, out, 1)
	if code != 0 || stdout != out {
		t.Errorf("exit %d, stdout %q", code, stdout)
	}
	if want := "itos-cc: its log\n::warning::100%25 of%0D%0Ait %250A::error::no\n"; stderr != want {
		t.Errorf("stderr is %q, not %q", stderr, want)
	}
}

// A survivor with no message still has its annotation, by its rule.
func TestASurvivorWithNoMessage(t *testing.T) {
	code, _, stderr := wrapped(t, `{"problems": [{"rule": "mutation.survived"}]}`, 1)
	if want := "itos-cc: its log\n::warning::mutation.survived\n"; code != 0 || stderr != want {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
}

// Everything else keeps the command's exit and its output, and says nothing of its own.
func TestAnythingElseKeepsItsExit(t *testing.T) {
	for _, c := range []struct {
		name string
		out  string
		code int
	}{
		{"mutation.uncovered beside a survivor", report("mutation.survived", "mutation.uncovered"), 1},
		{"mutation.uncovered-statement beside a survivor", report("mutation.uncovered-statement", "mutation.survived"), 1},
		{"mutation.uncovered alone", report("mutation.uncovered"), 1},
		{"a stale exception beside a survivor", report("mutation.survived", "mutation.exception-stale"), 1},
		{"mutation.flaky beside a survivor", report("mutation.survived", "mutation.flaky"), 1},
		{"a rule that only starts like it", report("mutation.survived.not"), 1},
		{"an interrupted run", report("mutation.survived", "interrupted"), 130},
		{"a cleanup failure", report("mutation.survived", "mutation.cleanup-failed"), 100},
		{"survivors alone at another exit", report("mutation.survived"), 100},
		{"survivors alone at exit 0", report("mutation.survived"), 0},
		{"a provider that cannot run", "", 82},
		{"no output", "", 1},
		{"output that is not JSON", "itos-cc: it broke\n", 1},
		{"text before the report", "note\n" + report("mutation.survived"), 1},
		{"text after the report", report("mutation.survived") + "note\n", 1},
		{"two reports", report("mutation.survived") + report("mutation.survived"), 1},
		{"a report cut short", `{"problems": [{"rule": "mutation.survived"}`, 1},
		{"an array", `[{"rule": "mutation.survived"}]`, 1},
		{"a report whose problems are empty", report(), 1},
		{"a report with no problems key", `{"ok": false}`, 1},
		{"a report whose problems are null", `{"problems": null}`, 1},
		{"a problem with no rule", `{"problems": [{"message": "a mutant survived"}]}`, 1},
		{"a null problem", `{"problems": [null]}`, 1},
		{"a rule that is no string", `{"problems": [{"rule": 1}]}`, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			code, stdout, stderr := wrapped(t, c.out, c.code)
			if code != c.code {
				t.Errorf("exit %d, not %d", code, c.code)
			}
			if stdout != c.out {
				t.Errorf("stdout is %q, not %q", stdout, c.out)
			}
			if stderr != "itos-cc: its log\n" {
				t.Errorf("stderr is %q, not the command's alone", stderr)
			}
		})
	}
}

func TestAKilledCommandFails(t *testing.T) {
	t.Setenv(envKill, "1")
	want := 128 + int(syscall.SIGKILL)
	if runtime.GOOS == "windows" {
		want = 1 // Kill is TerminateProcess, with exit code 1.
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--", self(t)}, &stdout, &stderr); code != want {
		t.Errorf("exit %d, not %d", code, want)
	}
}

func TestACommandThatCannotStart(t *testing.T) {
	t.Run("not there", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		missing := filepath.Join(t.TempDir(), "missing")
		if code := run([]string{"--", missing}, &stdout, &stderr); code != 127 {
			t.Errorf("exit %d, not 127", code)
		}
		if !strings.HasPrefix(stderr.String(), "nightly-survivors: ") || !strings.Contains(stderr.String(), "missing") {
			t.Errorf("stderr does not name it: %q", stderr.String())
		}
	})
	t.Run("not on the PATH", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		t.Setenv("PATH", t.TempDir())
		if code := run([]string{"--", "nightly-survivors-no-such-command"}, &stdout, &stderr); code != 127 {
			t.Errorf("exit %d, not 127", code)
		}
	})
	t.Run("not a program", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows looks for a folder's name with its executable extensions: not there")
		}
		var stdout, stderr bytes.Buffer
		if code := run([]string{"--", t.TempDir()}, &stdout, &stderr); code != 126 {
			t.Errorf("exit %d, not 126", code)
		}
		if stderr.Len() == 0 {
			t.Error("stderr says nothing")
		}
	})
}

func TestAUsageError(t *testing.T) {
	for _, args := range [][]string{nil, {"--"}, {self(t)}, {self(t), "--"}} {
		var stdout, stderr bytes.Buffer
		t.Setenv(envExit, "0")
		if code := run(args, &stdout, &stderr); code != 2 {
			t.Errorf("%q: exit %d, not 2", args, code)
		}
		if !strings.Contains(stderr.String(), "usage: nightly-survivors -- <command>") {
			t.Errorf("%q: stderr is %q", args, stderr.String())
		}
	}
}

type failing struct{}

func (failing) Write([]byte) (int, error) { return 0, errors.New("disk full") }

// Output that could not be passed through is never a pass, and no report: the command's own
// exit 1 stays, and its exit 0 becomes a failure.
func TestStdoutThatCannotBeWritten(t *testing.T) {
	t.Setenv(envOut, report("mutation.survived"))
	for _, c := range []struct{ code, want int }{{1, 1}, {0, 1}} {
		t.Setenv(envExit, fmt.Sprint(c.code))
		var stderr bytes.Buffer
		if code := run([]string{"--", self(t)}, failing{}, &stderr); code != c.want {
			t.Errorf("a command's exit %d gave exit %d, not %d", c.code, code, c.want)
		}
		if strings.Contains(stderr.String(), "::warning::") {
			t.Errorf("a command's exit %d annotated: %q", c.code, stderr.String())
		}
	}
}

// program is this binary started as the program itself around the arguments given.
func program(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(self(t), append([]string{"--"}, args...)...)
	cmd.Env = append(os.Environ(), envAsMain+"=1")
	return cmd
}

// The command's arguments reach it as they were given, flags and spaces and an empty one among
// them, and so does stdin; the whole program's streams are the ones it was started with.
func TestTheCommandGetsItsArgv(t *testing.T) {
	args := []string{"mutation", "run", "--count", "20", "--seed", "a b", "", "--", "-x", `"q"`, "$base", "--json"}
	cmd := program(t, append([]string{self(t)}, args...)...)
	cmd.Env = append(cmd.Env, envArgs+"=1")
	cmd.Stdin = strings.NewReader("its stdin")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Args  []string `json:"args"`
		Stdin string   `json:"stdin"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("%v: %q", err, out)
	}
	if strings.Join(got.Args, "\x00") != strings.Join(args, "\x00") {
		t.Errorf("the command got %q, not %q", got.Args, args)
	}
	if got.Stdin != "its stdin" {
		t.Errorf("the command read %q", got.Stdin)
	}
}

// waiting starts the program around a command that waits, and gives its pid and what ends the wait.
func waiting(t *testing.T, out string, code int) (*exec.Cmd, *bytes.Buffer, *bytes.Buffer, int, func()) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no signal to send a process")
	}
	dir := t.TempDir()
	ready, proceed := filepath.Join(dir, "ready"), filepath.Join(dir, "go")
	cmd := program(t, self(t))
	cmd.Env = append(cmd.Env, envReady+"="+ready, envGo+"="+proceed, envOut+"="+out, envExit+"="+fmt.Sprint(code))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	var pid int
	for start := time.Now(); pid == 0; time.Sleep(5 * time.Millisecond) {
		if time.Since(start) > time.Minute {
			t.Fatal("the command never started")
		}
		if data, err := os.ReadFile(ready); err == nil {
			_, _ = fmt.Sscan(string(data), &pid)
		}
	}
	return cmd, &stdout, &stderr, pid, func() {
		if err := os.WriteFile(proceed, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// A signal that reaches the program stops it from losing what the command still has to say: an
// interrupted itos-cc prints its report and exits 130 after the signal, through this program.
func TestASignalWaitsForTheCommand(t *testing.T) {
	for _, s := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(s.String(), func(t *testing.T) {
			out := report("mutation.survived", "interrupted")
			cmd, stdout, _, _, proceed := waiting(t, out, 130)
			if err := cmd.Process.Signal(s); err != nil {
				t.Fatal(err)
			}
			proceed()
			var exit *exec.ExitError
			if err := cmd.Wait(); !errors.As(err, &exit) || exit.ExitCode() != 130 {
				t.Errorf("the program ended with %v, not exit 130", err)
			}
			if stdout.String() != out {
				t.Errorf("stdout is %q, not the report", stdout.String())
			}
		})
	}
}

// The command gets the signals as it would without this program: a sh cannot trap a signal it
// starts with ignored, nor can a Go program catch SIGINT.
func TestACommandKeepsItsSignals(t *testing.T) {
	cmd, _, _, pid, proceed := waiting(t, "", 0)
	defer proceed()
	command, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	var exit *exec.ExitError
	if err := cmd.Wait(); !errors.As(err, &exit) || exit.ExitCode() != 128+int(syscall.SIGINT) {
		t.Errorf("the program ended with %v, not exit %d", err, 128+int(syscall.SIGINT))
	}
}
