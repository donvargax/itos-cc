// Command nightly-survivors runs the nightly's mutation run so that a survivor alone does not fail
// the night (T-229, q-108): survivors are recorded in .metrics/mutate/, so red should mean the run
// itself or another step failed. Harvested with the owner's permission (CONTRIBUTORS.md) from
// donvargax/itos's tools/bin/nightly-survivors at v8.49.0 (its T-341), behaving as it did, less
// this comment's account of how itos's own steps start it.
//
//	"$RUNNER_TEMP/nightly-survivors" -- <command> <argument>…
//
// It runs the command with its stdin, stdout and stderr passed through and exits as it did, except
// one case: exit 1 with a stdout that is one JSON value, an itos-cc --json report, whose top-level
// problems are at least one and all of rule mutation.survived. That is exit 0, with a GitHub
// ::warning command on stderr for each survivor, its message escaped as GitHub unescapes it, after
// everything the command wrote. Stdout stays the command's, byte for byte, and GitHub reads workflow
// commands from either stream. The exit code alone could not say: tests.list-failed,
// mutation.baseline-failed and mutation.flaky are exit 1 too.
//
// A command that does not start exits as a shell says: 127 when it is not there, else 126. One a
// signal kills exits 128 and the signal's number, as a shell says too. Stdout that could not be
// passed through is exit 1 when the command exited 0, and no report otherwise.
//
// SIGINT and SIGTERM do not stop it: an interrupted itos-cc still has its report to print, through
// this program, and itos-cc is the one to act on them. They are not forwarded, as a second one ends
// itos-cc at once, and it gets them as it would without this program: when they are sent to the
// whole process group or tree, as timeout does, which is how the nightly's deadline reaches it.
//
// The nightly builds it rather than go run it, as go run exits 1 for any failure of the program,
// which would hide a cut-off run from the commit step that reads the status.
//
// It imports nothing but the standard library, as the other gate programs do.
//
// Exit status: the command's, 0 in the one case above, 2 a usage error.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

const survived = "mutation.survived"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 || args[0] != "--" {
		_, _ = fmt.Fprintln(stderr, "usage: nightly-survivors -- <command> <argument>…")
		return 2
	}
	// Caught and dropped, never ignored: a signal ignored here would stay ignored in the command,
	// where a sh cannot trap it.
	signal.Notify(make(chan os.Signal, 1), os.Interrupt, syscall.SIGTERM)
	var printed bytes.Buffer
	cmd := exec.Command(args[1], args[2:]...) //nolint:gosec // running the command it is given is what this program does
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, io.MultiWriter(stdout, &printed), stderr
	err := cmd.Run()
	if cmd.ProcessState == nil {
		_, _ = fmt.Fprintf(stderr, "nightly-survivors: %v\n", err)
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
			return 127
		}
		return 126
	}
	code := exitCode(cmd.ProcessState)
	if code == 0 && err != nil {
		_, _ = fmt.Fprintf(stderr, "nightly-survivors: its stdout could not be passed through: %v\n", err)
		return 1
	}
	if code != 1 {
		return code
	}
	messages := survivors(printed.Bytes())
	if len(messages) == 0 {
		return 1
	}
	for _, m := range messages {
		_, _ = fmt.Fprintf(stderr, "::warning::%s\n", escape.Replace(m))
	}
	return 0
}

// exitCode is how the command ended, a signal as a shell says it.
func exitCode(state *os.ProcessState) int {
	if ws, ok := state.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return state.ExitCode()
}

// survivors is the message of each problem of out, its rule when it has none, when out is one JSON
// value whose problems are all survivors; none otherwise.
func survivors(out []byte) []string {
	var report struct {
		Problems []struct {
			Rule    string `json:"rule"`
			Message string `json:"message"`
		} `json:"problems"`
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	if err := dec.Decode(&report); err != nil {
		return nil
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil
	}
	var messages []string
	for _, p := range report.Problems {
		if p.Rule != survived {
			return nil
		}
		if p.Message == "" {
			p.Message = survived
		}
		messages = append(messages, p.Message)
	}
	return messages
}

// escape keeps a workflow command's value on its line, as GitHub reads it back.
var escape = strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A")
