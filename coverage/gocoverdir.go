package coverage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Go's integration coverage: a binary built with go build -cover writes
// what it executed to the directory GOCOVERDIR names, and go tool covdata
// textfmt turns that into a profile. A project whose tests run its built
// binary opts in by building it with -cover when GOCOVERDIR is set.
//
// go test -cover sets each test binary's GOCOVERDIR to a directory of its
// own, whatever the environment says, and reads only the test binary's own
// data from it before deleting it, so a binary a test builds with -cover
// would write where nothing reads it. go test therefore runs each test
// binary through itos-cc (-exec), which sets GOCOVERDIR to the directory
// coverDirEnv names and runs it. The test binary's own data still goes
// where -test.gocoverdir says, so go test's profile is the same.

// ExecArg, as itos-cc's first argument, runs the rest as a command with
// GOCOVERDIR set to the directory coverDirEnv names: see ExecTest.
const ExecArg = "__exec-with-gocoverdir"

// coverDirEnv names the directory ExecTest sets GOCOVERDIR to: go test
// passes it on, while it replaces GOCOVERDIR.
const coverDirEnv = "ITOS_CC_GOCOVERDIR"

// Executable is the itos-cc go test runs each test binary through, which
// must handle ExecArg. While it is "", Go coverage reads no integration
// data.
var Executable string

// execFlag is the -exec value that runs a test binary through Executable,
// or "" when there is none. go test splits it into fields, a quoted field
// whole and without escapes.
func execFlag() string {
	switch {
	case Executable == "":
		return ""
	case !strings.Contains(Executable, `"`):
		return `"` + Executable + `" ` + ExecArg
	case !strings.Contains(Executable, "'"):
		return "'" + Executable + "' " + ExecArg
	}
	return ""
}

// ExecTest runs args, a test binary and its arguments as go test -exec
// passes them, with GOCOVERDIR set to the directory coverDirEnv names, and
// returns its exit code.
func ExecTest(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "itos-cc: "+ExecArg+" needs a command to run")
		return 2
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if dir := os.Getenv(coverDirEnv); dir != "" {
		cmd.Env = append(os.Environ(), "GOCOVERDIR="+dir)
	}
	err := cmd.Run()
	if exit := (*exec.ExitError)(nil); errors.As(err, &exit) {
		return max(exit.ExitCode(), 1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "itos-cc:", err)
		return 1
	}
	return 0
}

// integrate turns what the processes the tests started wrote to p.CoverDir
// into the report p.Integration, with p.Convert, removes the directory, and
// returns the report's path, or "" when nothing was written or the report
// could not be made.
func (p Plan) integrate(log io.Writer) string {
	if p.CoverDir == "" {
		return ""
	}
	defer p.removeCoverDir()
	if err := p.dropRunnerData(); err != nil {
		fmt.Fprintf(log, "itos-cc: coverage: %s: %v\n", p.Language, err)
		return ""
	}
	if !p.written() {
		return ""
	}
	for _, args := range p.Convert {
		fmt.Fprintf(log, "itos-cc: coverage %s$ %s\n", p.Dir, displayArgs(args))
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir, cmd.Env = p.Dir, p.convertEnv()
		cmd.Stdout = log
		cmd.Stderr = log
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(log, "itos-cc: coverage: %s: %v\n", p.Language, err)
			return ""
		}
	}
	return p.Integration
}
