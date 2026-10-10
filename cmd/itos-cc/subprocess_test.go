package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// Tests run itos-cc as a subprocess: this test binary, given cliArg first,
// runs itos-cc with the rest of its arguments. The subprocess runs in a
// directory and an environment the test names with useDir and useEnv, not
// in the process's, so a test that needs its own needs neither t.Chdir nor
// t.Setenv and can call t.Parallel. Its coverage.Executable is this test
// binary, as an in-process run's is.

// cliArg, as the first argument of this test binary, makes it itos-cc.
const cliArg = "itos-cc.test-cli"

// platformEnv, when not empty, is the platform itos-cc pretends to run on
// for --count and --fail-fast, as countedPlatform holds it.
const platformEnv = "ITOS_CC_TEST_PLATFORM"

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == cliArg {
		if platform := os.Getenv(platformEnv); platform != "" {
			countedPlatform = platform
		}
		os.Exit(run(os.Args[2:]))
	}
	os.Exit(m.Run())
}

// workspace is the directory itos-cc runs in for a test, and what it adds
// to the process's environment, as KEY=value.
type workspace struct {
	dir string
	env []string
}

var (
	workspacesMu sync.Mutex
	// workspaces holds each test's workspace by its name; a subtest without
	// one of its own has its parent's.
	workspaces = map[string]workspace{}
)

// workspaceOf is t's workspace, or its nearest ancestor's.
func workspaceOf(t *testing.T) (workspace, bool) {
	workspacesMu.Lock()
	defer workspacesMu.Unlock()
	for name := t.Name(); ; {
		if ws, ok := workspaces[name]; ok {
			return ws, true
		}
		i := strings.LastIndex(name, "/")
		if i < 0 {
			return workspace{}, false
		}
		name = name[:i]
	}
}

// setWorkspace changes t's workspace with change, starting from the one it
// has, which may be its parent's.
func setWorkspace(t *testing.T, change func(*workspace)) {
	t.Helper()
	ws, _ := workspaceOf(t)
	ws.env = append([]string(nil), ws.env...)
	change(&ws)
	workspacesMu.Lock()
	_, existed := workspaces[t.Name()]
	workspaces[t.Name()] = ws
	workspacesMu.Unlock()
	if !existed {
		t.Cleanup(func() {
			workspacesMu.Lock()
			delete(workspaces, t.Name())
			workspacesMu.Unlock()
		})
	}
}

// useDir makes dir the directory itos-cc runs in for t and its subtests,
// and the one wd and inWD resolve against: what t.Chdir(dir) was.
func useDir(t *testing.T, dir string) {
	t.Helper()
	setWorkspace(t, func(ws *workspace) { ws.dir = dir })
}

// useEnv sets key to value in the environment itos-cc runs with for t and
// its subtests: what t.Setenv(key, value) was for it.
func useEnv(t *testing.T, key, value string) {
	t.Helper()
	setWorkspace(t, func(ws *workspace) { ws.env = append(ws.env, key+"="+value) })
}

// wd is the directory useDir named for t.
func wd(t *testing.T) string {
	t.Helper()
	ws, ok := workspaceOf(t)
	if !ok || ws.dir == "" {
		t.Fatal("the test named no directory with useDir")
	}
	return ws.dir
}

// inWD is name in t's directory; an absolute name is itself.
func inWD(t *testing.T, name string) string {
	t.Helper()
	if filepath.IsAbs(name) {
		return name
	}
	return filepath.Join(wd(t), name)
}

// testEnv is the environment a command runs with in t's directory: the
// process's with t's workspace's added, and PWD set to the directory, as
// t.Chdir sets it, so os.Getwd gives it in the form useDir named it.
func testEnv(t *testing.T) []string {
	t.Helper()
	ws, _ := workspaceOf(t)
	env := append(os.Environ(), ws.env...)
	if ws.dir != "" && runtime.GOOS != "windows" {
		env = append(env, "PWD="+ws.dir)
	}
	return env
}

// itosCc runs itos-cc with args in t's directory and environment and
// returns what it printed.
func itosCc(t *testing.T, args ...string) outcome {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, append([]string{cliArg}, args...)...)
	cmd.Dir, cmd.Env = wd(t), testEnv(t)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		t.Fatalf("itos-cc %q: %v", args, err)
	}
	return outcome{code: cmd.ProcessState.ExitCode(), stdout: stdout.String(), stderr: stderr.String()}
}
