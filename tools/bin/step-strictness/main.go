package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := command(ctx, os.Args[1:], os.Stdout, os.Stderr, runtime.GOOS)
	stop()
	os.Exit(code)
}

func printJSON(out io.Writer, value any) error {
	e := json.NewEncoder(out)
	e.SetIndent("", "  ")
	return e.Encode(value)
}

// command runs step-strictness and returns its exit code, which carries the
// outcome: a line the person's terminal refuses has nowhere else to be
// reported, so its write's error is discarded.
func command(ctx context.Context, args []string, out, stderr io.Writer, platform string) int {
	flags := flag.NewFlagSet("step-strictness", flag.ContinueOnError)
	flags.SetOutput(stderr)
	path := flags.String("plan", "", "reviewed version 1 JSON manifest")
	printPlan := flags.Bool("print-plan", false, "validate and print the manifest with defaults; do not run or edit")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *path == "" || flags.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "usage: step-strictness -plan <manifest.json> [-print-plan]")
		return 2
	}
	if !*printPlan && platform != "linux" && platform != "darwin" {
		_, _ = fmt.Fprintln(stderr, "step-strictness execution requires Linux or macOS; no files were written")
		return 2
	}
	q, err := prepare(*path)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	if *printPlan {
		if err := printJSON(out, q.plan); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 2
		}
		return 0
	}
	r, err := runPlan(ctx, &q, executeGo)
	_, _ = fmt.Fprintf(out, "evidence: %s\ndetected %d, undetected %d, invalid %d (%s)\n", q.output, r.Detected, r.Undetected, r.Invalid, r.Duration)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	if r.Invalid > 0 {
		return 2
	}
	if r.Undetected > 0 {
		return 1
	}
	return 0
}
