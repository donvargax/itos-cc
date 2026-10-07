package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

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
		{0, "interrupted"},
		{2, "a usage error: a bad flag, or a repository that cannot be read"},
		{75, "the port is in use; it may be free later"},
	},
	examples: []string{
		"itos-cc serve --ui viewer/dist .",
		"itos-cc serve --port 8080 ../api ../web",
	},
	run: runServe,
}

func runServe(in *invocation) (any, error) {
	roots := in.args
	if len(roots) == 0 {
		roots = []string{"."}
	}
	logger := log.New(os.Stderr, "itos-cc: ", 0)
	srv, err := server.New(roots, in.str("ui"), logger)
	if err != nil {
		return nil, fail(kindUsage, "serve.repo-unreadable", err.Error(), "Name repositories that exist.")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go srv.Watch(ctx, in.duration("interval"))

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(in.integer("port")))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fail(kindTemporary, "serve.port-in-use", fmt.Sprintf("cannot listen on %s: %v", addr, err),
			"Stop what uses the port, or choose another with --port.").with("port", in.integer("port"))
	}
	httpServer := &http.Server{Handler: srv.Handler()}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		httpServer.Shutdown(shutdown)
	}()
	logger.Printf("serving %v on http://%s", roots, addr)
	if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
		return nil, err
	}
	return struct {
		Address string `json:"address"`
	}{addr}, nil
}
