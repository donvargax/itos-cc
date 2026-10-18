// Command itos-cc measures code quality in TypeScript, Python, Kotlin, and Go.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/internal/cli"
)

// version is set at release time with -ldflags '-X main.version=…'. A binary
// built by `go install …@<version>` reports the module version instead.
var version = "dev"

// versionString is the version the binary says it is: the release's stamp,
// else the module version Go recorded (go install's, or a checkout's
// pseudo-version), else dev.
func versionString() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

// main makes the program's one context, which every command runs under,
// and gives run this process's standard streams and adapters. SIGINT or
// SIGTERM cancels the context, its cause cli.ErrInterrupted, and the
// command reports the interrupt, exit 130; a later one ends it at once,
// unless the process groups it owns are being killed (decision 38). The
// interrupts are this run's, bound into the adapters that kill those
// groups. It alone exits.
func main() {
	ctx, cancel := context.WithCancelCause(context.Background())
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	interrupts := &cli.Interrupts{}
	go interrupts.Stop(signals, cancel, func(sig os.Signal) { endInterrupted(hostPlatform, sig) })
	code := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, diskAdapters(os.Stderr, interrupts.Cleaning))
	if sig := interrupts.Signal(); sig != nil && code == cli.CodeInterrupted {
		endInterrupted(hostPlatform, sig)
	}
	os.Exit(code)
}

// endInterrupted ends the process as sig interrupted it, once the command
// has reported: on Linux and macOS by sig's own default action, raised
// again, so a shell reads 130 for SIGINT and 143 for SIGTERM and stops its
// loop (q-54); on Windows, which raises none, with exit 130, as it does
// should the signal not end it.
func endInterrupted(platform string, sig os.Signal) {
	if platform != "windows" && raise(sig) == nil {
		// The signal ends the process; this only bounds a wait for it.
		time.Sleep(time.Second)
	}
	os.Exit(cli.CodeInterrupted)
}

// classifiers are each domain's, which turn its sealed failures into
// problems for internal/cli: the selection domain's, which every command
// selects its files through, the snapshot domain's, which the commands read
// and write .metrics/ through, and the coverage domain's, which crap and
// mutation measure coverage through. A problem itos-cc's commands report is
// its own and needs none.
var classifiers = []cli.Classifier{classifySelection, classifySnapshot, classifyCoverage}

// stdUI is the UI of this process, which serve, the one command still on
// its legacy handler, reports through: its standard streams as they are
// when it is called.
func stdUI() *cli.UI {
	dir, _ := os.Getwd()
	return &cli.UI{In: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, Classifiers: classifiers, Dir: dir}
}

// init makes this executable the one go test runs each test binary through
// for Go coverage, and runs a test binary when go test does. An init rather
// than main, so the test binary of this package does the same and its tests
// run coverage as itos-cc does.
func init() {
	if len(os.Args) > 1 && os.Args[1] == coverage.ExecArg {
		os.Exit(coverage.ExecTest(os.Args[2:]))
	}
	coverage.Executable, _ = os.Executable()
}

// run runs args, the command line after the program's name, under ctx, and
// returns its exit code. It builds the parser of every command once, with
// fresh state, which internal/cli's Dispatch reads the line against: the
// flags before a command, the top-level help, 'help <command>' and the
// guess for an unknown command. The top-level help and problems and every
// command but serve read stdin and write stdout and stderr, and reach the
// world through a; serve keeps this process's streams and disk on its
// legacy handler. A model kong refuses is a bug, an internal failure.
func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, a adapters) int {
	ui := &cli.UI{In: stdin, Stdout: stdout, Stderr: stderr, Classifiers: classifiers, Dir: a.Dir}
	var c commandLine
	parser, err := parserOf(ctx, &c, ui, a)
	if err != nil {
		return ui.Fail(err, cli.Read(args).JSON)
	}
	route := cli.Dispatch(parser.Model, args)
	switch {
	case route.Problem != nil:
		return ui.Emit(route.JSON, nil, []*cli.Problem{route.Problem})
	case route.Usage:
		// The usage page has always exited 0 whether or not stdout takes it;
		// failing on the write would change its exit code.
		_, _ = fmt.Fprint(ui.Stdout, cli.Usage(parser.Model))
		return 0
	case route.Help != nil:
		return help(parser, ui, route.Help)
	}
	return dispatch(ctx, parser, &c, ui, a, route.Line)
}
