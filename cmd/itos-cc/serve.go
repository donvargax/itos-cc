package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/donvargax/itos-cc/graph"
	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/server"
)

var serveCommand = &command{
	name:     "serve",
	summary:  "live architecture graph and metrics over HTTP, for the viewer",
	synopsis: "[options] [repo ...]",
	about: `
Serves the architecture of one or more repositories for the viewer:

  GET /api/graph    repositories, directories, modules, functions, and
                    their dependencies and metrics, as JSON
  GET /api/events   server-sent events: the graph version, on every change
  GET /api/source   ?repo=NAME&file=PATH, the text of a file in the graph

Sources and .metrics snapshots are watched: saving a file updates
complexity and dependencies at once, and rerunning crap, mutate, or dry
updates their numbers. Listens on localhost only, until interrupted.`,
	flags: []flagSpec{
		opt("port", intFlag, "N", "7070", "port to listen on"),
		opt("ui", stringFlag, "DIR", "", "directory of a built viewer to serve at /, such as viewer/dist"),
		opt("interval", durationFlag, "D", "500ms", "how often to check for changes"),
	},
	json: `"address" once it stops`,
	rules: []string{
		"serve.port-in-use      the port is taken: port",
		"serve.repo-unreadable  a repository cannot be read",
	},
	exits: []exitDoc{
		{cli.CodeTemporary, "the port is in use; it may be free later"},
		{cli.CodeUsage, "a usage error: a bad flag, or a repository that cannot be read"},
	},
	examples: []string{
		"itos-cc serve --ui viewer/dist .",
		"itos-cc serve --port 8080 ../api ../web",
	},
	run: runServe,
}

// runServe serves until an interrupt cancels ctx, the command's, which
// shuts the server down: it then reports the address beside the
// interrupted problem, exit 130, as every interrupted command does
// (decision 38). Any other end of ctx's is the failure it is.
func runServe(ctx context.Context, in *invocation) (any, error) {
	roots := in.args
	if len(roots) == 0 {
		roots = []string{"."}
	}
	logger := log.New(os.Stderr, "itos-cc: ", 0)
	srv, err := server.New(ctx, roots, in.str("ui"), logger, graph.MutationInputs{Snapshots: in.adapters.SnapshotReaders, Toolchain: in.adapters.Toolchain, Fingerprints: in.adapters.Fingerprints})
	if err != nil {
		return nil, cli.NewProblem(cli.CodeUsage, "serve.repo-unreadable", err.Error(), "Name repositories that exist.")
	}
	go srv.Watch(ctx, in.duration("interval"))

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(in.integer("port")))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, cli.NewProblem(cli.CodeTemporary, "serve.port-in-use", fmt.Sprintf("cannot listen on %s: %v", addr, err),
			"Stop what uses the port, or choose another with --port.").With("port", in.integer("port"))
	}
	httpServer := &http.Server{Handler: srv.Handler()} //nolint:gosec // serves 127.0.0.1 alone, for the person's own viewer; a header timeout would change how serve treats a slow client
	go shutdownWhenDone(ctx, httpServer)
	logger.Printf("serving %v on http://%s", roots, addr)
	if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
		return nil, err
	}
	served := struct {
		Address string `json:"address"`
	}{addr}
	if ctx.Err() != nil {
		return served, context.Cause(ctx)
	}
	return served, nil
}

// shutdownWhenDone shuts srv down once ctx is done, giving what it is
// serving 2 seconds to end: the shutdown's own context is ctx's, without
// its cancellation, which has already come.
func shutdownWhenDone(ctx context.Context, srv *http.Server) {
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	// The command ends either way: what Shutdown could not end in its 2
	// seconds ends with the process.
	_ = srv.Shutdown(shutdown)
}
