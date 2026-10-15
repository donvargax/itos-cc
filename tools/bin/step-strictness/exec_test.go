package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestJoinedCommandCancellation(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("live runner execution is Linux/macOS only")
	}
	for _, deadline := range []bool{false, true} {
		t.Run(fmt.Sprint("deadline=", deadline), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if deadline {
				var stop context.CancelFunc
				ctx, stop = context.WithTimeout(ctx, 3*time.Second)
				defer stop()
			}
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCommandChild$")
			cmd.Env = childEnvironment("parent")
			input, keepOpen, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			defer keepOpen.Close()
			cmd.Stdin = input
			output, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			defer writer.Close()
			cmd.Stdout = writer
			cmd.Stderr = writer
			done := make(chan struct{})
			var runErr error
			go func() { runErr = joinedCommand(cmd); close(done) }()
			terminated := false
			defer func() {
				cancel()
				// Even a failed readiness/assertion must clean up the owned group.
				select {
				case <-done:
					if !terminated && cmd.Process != nil {
						_ = killCommandGroup(cmd.Process.Pid)
					}
				case <-time.After(10 * time.Second):
					t.Error("cleanup could not join owned helper")
				}
			}()
			ready := make(chan string, 1)
			eof := make(chan error, 1)
			go func() {
				reader := bufio.NewReader(output)
				line, _ := reader.ReadString('\n')
				ready <- line
				_, err := io.Copy(io.Discard, reader)
				eof <- err
			}()
			select {
			case line := <-ready:
				if strings.TrimSpace(line) != "ready" {
					t.Fatalf("child not ready: %q", line)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("no child readiness")
			}
			// Only the child and grandchild now own the pipe's writing descriptors.
			if err = writer.Close(); err != nil {
				t.Fatal(err)
			}
			if !deadline {
				cancel()
			}
			select {
			case <-done:
				if runErr == nil {
					t.Fatal("cancelled command passed")
				}
				if cmd.ProcessState == nil {
					t.Fatal("returned without joining owned runner")
				}
			case <-time.After(10 * time.Second):
				cancel()
				t.Fatal("owned runner did not join")
			}
			select {
			case err := <-eof:
				if err != nil {
					t.Fatal(err)
				}
				terminated = true
			case <-time.After(10 * time.Second):
				t.Fatal("grandchild kept inherited pipe open after cancellation")
			}
			if ctx.Err() == nil {
				t.Fatal("context not cancelled")
			}
		})
	}
}

func TestCommandChild(t *testing.T) {
	mode := os.Getenv("STEP_STRICTNESS_CHILD")
	if mode == "parent" {
		child := exec.Command(os.Args[0], "-test.run=^TestCommandChild$")
		child.Env = childEnvironment("grandchild")
		child.Stdin = os.Stdin
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		if err := child.Run(); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	if mode != "grandchild" {
		return
	}
	fmt.Println("ready")
	var b [1]byte
	_, err := os.Stdin.Read(b[:])
	if err != nil && !errors.Is(err, os.ErrClosed) {
		os.Exit(2)
	}
	os.Exit(0)
}

func childEnvironment(mode string) []string {
	var env []string
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "STEP_STRICTNESS_CHILD=") {
			env = append(env, entry)
		}
	}
	return append(env, "STEP_STRICTNESS_CHILD="+mode)
}
